package engineclient

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"

	pb "github.com/dcsg/archway/internal/engineclient/pb"
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
			result.Summary = p.CheckComplete
		case *pb.EngineResponse_Error:
			return nil, fmt.Errorf("engine error: %s (%s)", p.Error.Message, p.Error.Code)
		}
	}

	if result.Summary == nil {
		return nil, fmt.Errorf("engine did not send CheckComplete")
	}
	return result, nil
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

// readMessage reads a length-prefixed protobuf message.
func readMessage(r io.Reader) (*pb.EngineResponse, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}
	msgLen := binary.LittleEndian.Uint32(lenBuf)

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
