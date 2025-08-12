package bedrock

import (
	"fmt"

	"github.com/nirmata/kubectl-ai/gollm"
)

type bedrockChatResponse struct {
	content   string
	usage     any
	toolCalls []gollm.FunctionCall
	model     string
	provider  string
}

var _ gollm.ChatResponse = (*bedrockChatResponse)(nil)

func (r *bedrockChatResponse) UsageMetadata() any {
	model := r.model
	provider := r.provider
	if model == "" {
		model = "bedrock"
	}
	if provider == "" {
		provider = "bedrock"
	}

	if structuredUsage := convertAWSUsage(r.usage, model, provider); structuredUsage != nil {
		return structuredUsage
	}
	return r.usage
}

func (r *bedrockChatResponse) Candidates() []gollm.Candidate {
	return []gollm.Candidate{
		&bedrockCandidate{
			content:   r.content,
			toolCalls: r.toolCalls,
		},
	}
}

type bedrockCandidate struct {
	content   string
	toolCalls []gollm.FunctionCall
}

var _ gollm.Candidate = (*bedrockCandidate)(nil)

func (c *bedrockCandidate) Parts() []gollm.Part {
	var parts []gollm.Part

	if c.content != "" {
		parts = append(parts, &bedrockPart{content: c.content})
	}

	if len(c.toolCalls) > 0 {
		parts = append(parts, &bedrockPart{toolCalls: c.toolCalls})
	}

	return parts
}

func (c *bedrockCandidate) String() string {
	return fmt.Sprintf("BedrockCandidate(Content: %d chars, ToolCalls: %d)",
		len(c.content), len(c.toolCalls))
}

type bedrockPart struct {
	content   string
	toolCalls []gollm.FunctionCall
}

var _ gollm.Part = (*bedrockPart)(nil)

func (p *bedrockPart) AsText() (string, bool) {
	return p.content, p.content != ""
}

func (p *bedrockPart) AsFunctionCalls() ([]gollm.FunctionCall, bool) {
	return p.toolCalls, len(p.toolCalls) > 0
}

type simpleBedrockCompletionResponse struct {
	content  string
	usage    any
	model    string
	provider string
}

var _ gollm.CompletionResponse = (*simpleBedrockCompletionResponse)(nil)

func (r *simpleBedrockCompletionResponse) Response() string {
	return r.content
}

func (r *simpleBedrockCompletionResponse) UsageMetadata() any {
	model := r.model
	provider := r.provider
	if model == "" {
		model = "bedrock"
	}
	if provider == "" {
		provider = "bedrock"
	}

	if structuredUsage := convertAWSUsage(r.usage, model, provider); structuredUsage != nil {
		return structuredUsage
	}
	return r.usage
}

func extractTextFromResponse(response gollm.ChatResponse) string {
	if response == nil {
		return ""
	}

	candidates := response.Candidates()
	if len(candidates) == 0 {
		return ""
	}

	parts := candidates[0].Parts()
	for _, part := range parts {
		if text, ok := part.AsText(); ok {
			return text
		}
	}

	return ""
}
