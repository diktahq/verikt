package experiment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Agent is the interface every LLM provider must satisfy for stateless experiments.
// One system prompt + one user prompt → one response. Tool-access (Mode C) is handled
// separately via callClaudeWithTools.
type Agent interface {
	ID() string // "claude-sonnet-4-6", "gpt-4o", "gemini-2.0-flash"
	Call(ctx context.Context, systemPrompt, userPrompt string) (AgentResponse, error)
}

// AgentResponse is the normalized response from any agent.
type AgentResponse struct {
	Raw          string
	InputTokens  int
	CacheTokens  int // zero if provider does not support prompt caching
	OutputTokens int
	DurationMS   int
}

// AgentFromEnv constructs an Agent from environment variables.
//
//	ARCHWAY_EXPERIMENT_VENDOR   — anthropic | openai | google | ollama (default: anthropic)
//	ARCHWAY_EXPERIMENT_MODEL    — model ID (default: claude-sonnet-4-6)
//	ARCHWAY_EXPERIMENT_BASE_URL — optional base URL override
func AgentFromEnv() (Agent, error) {
	vendor := os.Getenv("ARCHWAY_EXPERIMENT_VENDOR")
	model := os.Getenv("ARCHWAY_EXPERIMENT_MODEL")

	if vendor == "" {
		vendor = "claude-code"
	}
	if model == "" {
		model = "claude-sonnet-4-6"
	}

	// claude-code: use the Claude Code CLI (claude -p). No API key needed.
	if vendor == "claude-code" {
		return &claudeCodeAgent{model: model}, nil
	}

	baseURL := os.Getenv("ARCHWAY_EXPERIMENT_BASE_URL")
	if baseURL == "" {
		switch vendor {
		case "anthropic":
			baseURL = "https://api.anthropic.com/v1"
		case "openai":
			baseURL = "https://api.openai.com/v1"
		case "google":
			baseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
		case "ollama":
			baseURL = "http://localhost:11434/v1"
		default:
			return nil, fmt.Errorf("unknown vendor %q — set ARCHWAY_EXPERIMENT_BASE_URL", vendor)
		}
	}

	return &openAICompatAgent{
		vendor:  vendor,
		model:   model,
		baseURL: baseURL,
		apiKey:  apiKeyForVendor(vendor),
		client:  &http.Client{Timeout: 5 * time.Minute},
	}, nil
}

func apiKeyForVendor(vendor string) string {
	switch vendor {
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY")
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "google":
		return os.Getenv("GEMINI_API_KEY")
	}
	return ""
}

// openAICompatAgent calls any OpenAI-compatible chat completions endpoint.
type openAICompatAgent struct {
	vendor  string
	model   string
	baseURL string
	apiKey  string
	client  *http.Client
}

func (a *openAICompatAgent) ID() string {
	return a.model
}

func (a *openAICompatAgent) Call(ctx context.Context, systemPrompt, userPrompt string) (AgentResponse, error) {
	body, err := json.Marshal(map[string]any{
		"model": a.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	})
	if err != nil {
		return AgentResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return AgentResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	start := time.Now()
	resp, err := a.client.Do(req)
	if err != nil {
		return AgentResponse{}, fmt.Errorf("call %s: %w", a.baseURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	durationMS := int(time.Since(start).Milliseconds())

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return AgentResponse{}, fmt.Errorf("decode response: %w", err)
	}
	if result.Error != nil {
		return AgentResponse{}, fmt.Errorf("API error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return AgentResponse{}, fmt.Errorf("no choices in response")
	}

	return AgentResponse{
		Raw:          result.Choices[0].Message.Content,
		InputTokens:  result.Usage.PromptTokens,
		CacheTokens:  result.Usage.PromptTokensDetails.CachedTokens,
		OutputTokens: result.Usage.CompletionTokens,
		DurationMS:   durationMS,
	}, nil
}

// claudeCodeAgent calls the `claude -p` CLI (Claude Code pipe mode).
// No API key needed — uses the active Claude Code session/subscription.
type claudeCodeAgent struct {
	model string // e.g. "claude-sonnet-4-6"; empty = Claude Code default
}

func (a *claudeCodeAgent) ID() string {
	if a.model != "" {
		return a.model
	}
	return "claude-code"
}

func (a *claudeCodeAgent) Call(ctx context.Context, systemPrompt, userPrompt string) (AgentResponse, error) {
	args := []string{"-p", userPrompt, "--output-format", "json", "--system-prompt", systemPrompt}
	if a.model != "" {
		args = append(args, "--model", a.model)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	// Strip CLAUDE_CODE env so it doesn't interfere.
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CLAUDECODE=") && !strings.HasPrefix(e, "CLAUDE_CODE=") {
			env = append(env, e)
		}
	}
	cmd.Env = env

	start := time.Now()
	out, err := cmd.Output()
	durationMS := int(time.Since(start).Milliseconds())
	if err != nil {
		return AgentResponse{}, fmt.Errorf("claude -p: %w", err)
	}

	var result struct {
		Result string `json:"result"`
		Usage  struct {
			InputTokens          int `json:"input_tokens"`
			CacheReadInputTokens int `json:"cache_read_input_tokens"`
			OutputTokens         int `json:"output_tokens"`
		} `json:"usage"`
		DurationAPIMs int `json:"duration_api_ms"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return AgentResponse{}, fmt.Errorf("parse claude output: %w", err)
	}
	return AgentResponse{
		Raw:          result.Result,
		InputTokens:  result.Usage.InputTokens,
		CacheTokens:  result.Usage.CacheReadInputTokens,
		OutputTokens: result.Usage.OutputTokens,
		DurationMS:   durationMS,
	}, nil
}

// safeAgentID returns a filesystem-safe agent identifier for naming result directories.
// e.g. "claude-sonnet-4-6", "gpt-4o", "gemini-2-0-flash"
func safeAgentID(a Agent) string {
	return strings.NewReplacer(".", "-", "/", "-", ":", "-").Replace(a.ID())
}
