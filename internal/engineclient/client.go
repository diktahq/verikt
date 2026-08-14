package engineclient

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"

	pb "github.com/diktahq/verikt/internal/engineclient/pb"
	"google.golang.org/protobuf/proto"
)

// Client manages the lifecycle of the Rust engine subprocess.
type Client struct {
	enginePath string
}

// New creates a client that will spawn the engine binary at the given path.
func New(enginePath string) *Client {
	return &Client{enginePath: enginePath}
}

// CheckResult holds the findings and completion summary from a Check operation.
type CheckResult struct {
	Findings []*pb.Finding
	Summary  *pb.CheckComplete
}

// Ping sends a ping request to the engine and returns the version and capabilities.
func (c *Client) Ping(ctx context.Context) (*pb.PingResult, error) {
	req := &pb.EngineRequest{
		Command: &pb.EngineRequest_Ping{Ping: &pb.PingRequest{}},
	}

	responses, err := c.execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("engine ping: %w", err)
	}

	for _, resp := range responses {
		switch p := resp.Payload.(type) {
		case *pb.EngineResponse_PingResult:
			return p.PingResult, nil
		case *pb.EngineResponse_Error:
			return nil, fmt.Errorf("engine error: %s (%s)", p.Error.Message, p.Error.Code)
		}
	}
	return nil, fmt.Errorf("no ping result in response")
}

// Check sends rules to the engine and returns findings.
func (c *Client) Check(ctx context.Context, projectPath string, rules []*pb.Rule, targetFiles []string) (*CheckResult, error) {
	req := &pb.EngineRequest{
		Command: &pb.EngineRequest_Check{Check: &pb.CheckRequest{
			ProjectPath: projectPath,
			Rules:       rules,
			TargetFiles: targetFiles,
		}},
	}

	responses, err := c.execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("engine check: %w", err)
	}

	result := &CheckResult{}
	for _, resp := range responses {
		switch p := resp.Payload.(type) {
		case *pb.EngineResponse_Finding:
			result.Findings = append(result.Findings, p.Finding)
		case *pb.EngineResponse_CheckComplete:
			result.Summary = mergeCheckComplete(result.Summary, p.CheckComplete)
		case *pb.EngineResponse_Error:
			return nil, fmt.Errorf("engine error: %s (%s)", p.Error.Message, p.Error.Code)
		}
	}

	if result.Summary == nil {
		return nil, fmt.Errorf("engine did not send CheckComplete")
	}
	return result, nil
}

// mergeCheckComplete combines one module's summary into the running total.
//
// The engine runs grep, import_graph, antipatterns and metrics for every request
// and each emits its own CheckComplete — main.rs says so, and its comment claims
// "The Go client merges them". It did not: this was a plain assignment, so every
// summary but the last was thrown away.
//
// It went unnoticed because each call site sends rules of a single engine type,
// and grep is emitted first and never early-returns, so the overwrite happened to
// land on the specialised module's summary. A request carrying grep rules *and*
// another type lost one module's rule_statuses entirely — and the proxy-rule path
// reads exactly that field, so those rules would have been reported as absent.
func mergeCheckComplete(into, next *pb.CheckComplete) *pb.CheckComplete {
	if next == nil {
		return into
	}
	if into == nil {
		return next
	}

	// Findings and time are per-module and additive.
	into.FindingsTotal += next.FindingsTotal
	into.FindingsError += next.FindingsError
	into.FindingsWarning += next.FindingsWarning
	into.FindingsInfo += next.FindingsInfo
	into.DurationMs += next.DurationMs

	// Files and rules are the same population seen by several modules, so the
	// widest view is the honest one — summing would count one file many times.
	into.FilesChecked = max(into.FilesChecked, next.FilesChecked)
	into.RulesEvaluated = max(into.RulesEvaluated, next.RulesEvaluated)

	// Each module reports statuses only for the rules it owns, so these do not
	// overlap; guard against duplicates anyway rather than assume that holds.
	seen := make(map[string]bool, len(into.RuleStatuses))
	for _, s := range into.RuleStatuses {
		seen[s.RuleId] = true
	}
	for _, s := range next.RuleStatuses {
		if !seen[s.RuleId] {
			seen[s.RuleId] = true
			into.RuleStatuses = append(into.RuleStatuses, s)
		}
	}

	return into
}

// execute spawns the engine, sends a request, reads all responses, and waits for exit.
func (c *Client) execute(ctx context.Context, req *pb.EngineRequest) ([]*pb.EngineResponse, error) {
	cmd := exec.CommandContext(ctx, c.enginePath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start engine: %w", err)
	}

	if err := writeMessage(stdin, req); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}
	if err := stdin.Close(); err != nil {
		return nil, fmt.Errorf("close stdin: %w", err)
	}

	var responses []*pb.EngineResponse
	for {
		resp, err := readMessage(stdout)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		responses = append(responses, resp)
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("engine exited: %w", err)
	}

	return responses, nil
}

// writeMessage writes a length-prefixed protobuf message.
func writeMessage(w io.Writer, msg *pb.EngineRequest) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, uint32(len(data)))

	if _, err := w.Write(lenBuf); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// maxMsgSize is the maximum protobuf message size accepted from the engine (64 MiB).
const maxMsgSize = 64 * 1024 * 1024

// readMessage reads a length-prefixed protobuf message.
func readMessage(r io.Reader) (*pb.EngineResponse, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}
	msgLen := binary.LittleEndian.Uint32(lenBuf)
	if msgLen > maxMsgSize {
		return nil, fmt.Errorf("engine message too large: %d bytes (max %d)", msgLen, maxMsgSize)
	}

	msgBuf := make([]byte, msgLen)
	if _, err := io.ReadFull(r, msgBuf); err != nil {
		return nil, err
	}

	resp := &pb.EngineResponse{}
	if err := proto.Unmarshal(msgBuf, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
