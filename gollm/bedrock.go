// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gollm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"k8s.io/klog/v2"
)

// Register the Bedrock provider factory on package initialization
func init() {
	if err := RegisterProvider("bedrock", newBedrockClientFactory); err != nil {
		klog.Fatalf("Failed to register bedrock provider: %v", err)
	}
}

// newBedrockClientFactory creates a new Bedrock client with the given options
func newBedrockClientFactory(ctx context.Context, opts ClientOptions) (Client, error) {
	return NewBedrockClient(ctx, opts)
}

// BedrockClient implements the gollm.Client interface for AWS Bedrock models
type BedrockClient struct {
	client *bedrockruntime.Client
}

// Ensure BedrockClient implements the Client interface
var _ Client = &BedrockClient{}

// NewBedrockClient creates a new client for interacting with AWS Bedrock models
func NewBedrockClient(ctx context.Context, opts ClientOptions) (*BedrockClient, error) {
	// Load AWS config with timeout protection
	configCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(configCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Default to us-east-1 for Bedrock if no region is set
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	return &BedrockClient{
		client: bedrockruntime.NewFromConfig(cfg),
	}, nil
}

// Close cleans up any resources used by the client
func (c *BedrockClient) Close() error {
	return nil
}

// StartChat starts a new chat session with the specified system prompt and model
func (c *BedrockClient) StartChat(systemPrompt, model string) Chat {
	selectedModel := getBedrockModel(model)

	// Enhance system prompt for tool-use shim compatibility
	// Detect if tool-use shim is enabled by looking for JSON formatting instructions
	enhancedPrompt := systemPrompt
	if strings.Contains(systemPrompt, "```json") && strings.Contains(systemPrompt, "\"action\"") {
		// Tool-use shim is enabled - add stronger JSON formatting instructions for all Bedrock models
		enhancedPrompt += "\n\nCRITICAL JSON FORMATTING REQUIREMENTS:\n"
		enhancedPrompt += "1. You MUST ALWAYS wrap your JSON responses in ```json code blocks exactly as shown in the examples above.\n"
		enhancedPrompt += "2. NEVER respond with raw JSON without the markdown ```json formatting.\n"
		enhancedPrompt += "3. Ensure your JSON is syntactically correct with proper commas between fields.\n"
		enhancedPrompt += "4. This is critical for proper parsing. Example format:\n"
		enhancedPrompt += "```json\n{\"thought\": \"your reasoning\", \"action\": {\"name\": \"tool_name\", \"command\": \"command\"}}\n```\n"
		enhancedPrompt += "Note the comma after the \"thought\" field! Malformed JSON will cause failures."
	}

	return &bedrockChat{
		client:       c,
		systemPrompt: enhancedPrompt,
		model:        selectedModel,
		messages:     []types.Message{},
	}
}

// GenerateCompletion generates a single completion for the given request
func (c *BedrockClient) GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error) {
	chat := c.StartChat("", req.Model)
	chatResponse, err := chat.Send(ctx, req.Prompt)
	if err != nil {
		return nil, err
	}

	// Wrap ChatResponse in a CompletionResponse
	return &bedrockCompletionResponse{
		chatResponse: chatResponse,
	}, nil
}

// SetResponseSchema sets the response schema for the client (not supported by Bedrock)
func (c *BedrockClient) SetResponseSchema(schema *Schema) error {
	return fmt.Errorf("response schema not supported by Bedrock")
}

// ListModels returns the list of supported Bedrock models
func (c *BedrockClient) ListModels(ctx context.Context) ([]string, error) {
	return []string{
		"us.anthropic.claude-sonnet-4-20250514-v1:0",   // Claude Sonnet 4 (default)
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0", // Claude 3.7 Sonnet
	}, nil
}

// bedrockChat implements the Chat interface for Bedrock conversations
type bedrockChat struct {
	client       *BedrockClient
	systemPrompt string
	model        string
	messages     []types.Message
	toolConfig   *types.ToolConfiguration
	functionDefs []*FunctionDefinition
}

func (cs *bedrockChat) Initialize(history []*api.Message) error {
	cs.messages = make([]types.Message, 0, len(history))

	for _, msg := range history {
		// Convert api.Message to types.Message
		var role types.ConversationRole
		switch msg.Source {
		case api.MessageSourceUser:
			role = types.ConversationRoleUser
		case api.MessageSourceModel, api.MessageSourceAgent:
			role = types.ConversationRoleAssistant
		default:
			// Skip unknown message sources
			continue
		}

		// Convert payload to string content
		var content string
		if msg.Type == api.MessageTypeText && msg.Payload != nil {
			if textPayload, ok := msg.Payload.(string); ok {
				content = textPayload
			} else {
				// Try to convert other types to string
				content = fmt.Sprintf("%v", msg.Payload)
			}
		} else {
			// Skip non-text messages for now
			continue
		}

		if content == "" {
			continue
		}

		bedrockMsg := types.Message{
			Role: role,
			Content: []types.ContentBlock{
				&types.ContentBlockMemberText{Value: content},
			},
		}

		cs.messages = append(cs.messages, bedrockMsg)
	}

	return nil
}

// Send sends a message to the chat and returns the response
func (c *bedrockChat) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	if len(contents) == 0 {
		return nil, errors.New("no content provided")
	}

	// Process and append messages to history
	if err := c.addContentsToHistory(contents); err != nil {
		return nil, err
	}

	// Prepare the request
	input := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(c.model),
		Messages: c.messages,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens: aws.Int32(4096),
		},
	}

	// Add system prompt if provided
	if c.systemPrompt != "" {
		input.System = []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: c.systemPrompt},
		}
	}

	// PHASE 2: Request Configuration Validation
	if c.toolConfig != nil {
		klog.Infof("[BEDROCK-FUNCTION-DEBUG] Configuring %d function definitions for model: %s",
			len(c.functionDefs), c.model)

		// Log tool names being configured
		var toolNames []string
		for _, tool := range c.toolConfig.Tools {
			if toolSpec, ok := tool.(*types.ToolMemberToolSpec); ok {
				if toolSpec.Value.Name != nil {
					toolNames = append(toolNames, *toolSpec.Value.Name)
				}
			}
		}
		klog.Infof("[BEDROCK-FUNCTION-DEBUG] Tool names: %v", toolNames)

		input.ToolConfig = c.toolConfig
		klog.Infof("[BEDROCK-FUNCTION-DEBUG] ✅ ToolConfig set with ToolChoice: %T", c.toolConfig.ToolChoice)

		// Log first tool structure for validation
		if len(c.toolConfig.Tools) > 0 {
			klog.V(2).Infof("[BEDROCK-FUNCTION-DEBUG] Sample tool structure: %+v", c.toolConfig.Tools[0])
		}
	} else if len(c.functionDefs) > 0 {
		klog.Errorf("[BEDROCK-FUNCTION-DEBUG] ❌ Function definitions exist but toolConfig is nil")
	} else {
		klog.Infof("[BEDROCK-FUNCTION-DEBUG] No function definitions available")
	}

	// PHASE 4: Model and API Response Validation
	klog.Infof("[BEDROCK-FUNCTION-DEBUG] Sending request to model: %s", c.model)
	klog.V(2).Infof("[BEDROCK-FUNCTION-DEBUG] Request has ToolConfig: %t", input.ToolConfig != nil)
	if input.ToolConfig != nil {
		klog.V(2).Infof("[BEDROCK-FUNCTION-DEBUG] ToolConfig has %d tools", len(input.ToolConfig.Tools))
	}

	// Call the Bedrock Converse API
	output, err := c.client.client.Converse(ctx, input)
	if err != nil {
		klog.Errorf("[BEDROCK-FUNCTION-DEBUG] ❌ Converse API error: %v", err)
		return nil, fmt.Errorf("bedrock converse error: %w", err)
	}

	// After successful response
	klog.Infof("[BEDROCK-FUNCTION-DEBUG] ✅ Received response from Bedrock")
	klog.V(2).Infof("[BEDROCK-FUNCTION-DEBUG] Raw output type: %T", output.Output)

	// Log stop reason if available
	if output.StopReason != "" {
		klog.Infof("[BEDROCK-FUNCTION-DEBUG] Stop reason: %s", string(output.StopReason))
	}

	// Extract response content and update conversation history
	response := &bedrockResponse{
		output: output,
		model:  c.model,
	}

	// Update conversation history with assistant's response
	if output.Output != nil {
		if msg, ok := output.Output.(*types.ConverseOutputMemberMessage); ok {
			c.messages = append(c.messages, msg.Value)
		}
	}

	return response, nil
}

// SendStreaming sends a message and returns a streaming response
func (c *bedrockChat) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	if len(contents) == 0 {
		return nil, errors.New("no content provided")
	}

	// Process and append messages to history
	if err := c.addContentsToHistory(contents); err != nil {
		return nil, err
	}

	// Prepare the streaming request
	input := &bedrockruntime.ConverseStreamInput{
		ModelId:  aws.String(c.model),
		Messages: c.messages,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens: aws.Int32(4096),
		},
	}

	// Add system prompt if provided
	if c.systemPrompt != "" {
		input.System = []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: c.systemPrompt},
		}
	}

	// Add tool configuration if functions are defined (streaming)
	if c.toolConfig != nil {
		klog.V(2).Infof("[BEDROCK-FUNCTION-DEBUG] Adding tool config to streaming request")
		input.ToolConfig = c.toolConfig
	}

	// Start the streaming request
	output, err := c.client.client.ConverseStream(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("bedrock stream error: %w", err)
	}

	// Return streaming iterator
	return func(yield func(ChatResponse, error) bool) {
		defer func() {
			if stream := output.GetStream(); stream != nil {
				stream.Close()
			}
		}()

		var assistantMessage types.Message
		assistantMessage.Role = types.ConversationRoleAssistant
		var fullContent strings.Builder

		// Tool state tracking for streaming
		type partialTool struct {
			id    string
			name  string
			input strings.Builder
		}
		partialTools := make(map[int32]*partialTool)
		var completedTools []types.ToolUseBlock

		// Process streaming events
		stream := output.GetStream()
		for event := range stream.Events() {
			// Debug: log all event types
			klog.V(3).Infof("[BEDROCK-FUNCTION-DEBUG] Streaming event type: %T", event)
			
			switch v := event.(type) {
			case *types.ConverseStreamOutputMemberContentBlockDelta:
				// Handle text deltas
				if textDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
					fullContent.WriteString(textDelta.Value)

					response := &bedrockStreamResponse{
						content: textDelta.Value,
						model:   c.model,
						done:    false,
					}

					if !yield(response, nil) {
						return
					}
				}

				// Handle tool input deltas
				if toolDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberToolUse); ok {
					idx := aws.ToInt32(v.Value.ContentBlockIndex)
					if partial, exists := partialTools[idx]; exists {
						deltaInput := aws.ToString(toolDelta.Value.Input)
						partial.input.WriteString(deltaInput)
						klog.Infof("[BEDROCK-FUNCTION-DEBUG] Tool input delta for %s (idx %d): '%s'", partial.name, idx, deltaInput)
					} else {
						klog.Errorf("[BEDROCK-FUNCTION-DEBUG] Received tool delta for unknown index %d", idx)
					}
				} else {
					// Log what type of delta we actually got
					klog.V(2).Infof("[BEDROCK-FUNCTION-DEBUG] ContentBlockDelta type: %T", v.Value.Delta)
				}

			case *types.ConverseStreamOutputMemberContentBlockStart:
				// Handle content block start (for tool calls)
				if v.Value.Start != nil {
					klog.V(3).Infof("Content block started at index: %v", aws.ToInt32(v.Value.ContentBlockIndex))
					// BEDROCK DEBUG: Log what type of content block is starting
					klog.Infof("[BEDROCK-FUNCTION-DEBUG] Content block start - Type: %T", v.Value.Start)
					if toolStart, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
						klog.Infof("[BEDROCK-FUNCTION-DEBUG] ✅ STREAMING TOOL USE STARTED - ID: %s, Name: %s",
							aws.ToString(toolStart.Value.ToolUseId), aws.ToString(toolStart.Value.Name))
						
						// Store partial tool for input accumulation
						idx := aws.ToInt32(v.Value.ContentBlockIndex)
						partialTools[idx] = &partialTool{
							id:   aws.ToString(toolStart.Value.ToolUseId),
							name: aws.ToString(toolStart.Value.Name),
						}
					}
				}

			case *types.ConverseStreamOutputMemberContentBlockStop:
				// Handle content block stop (tool completion)
				idx := aws.ToInt32(v.Value.ContentBlockIndex)
				if partial, exists := partialTools[idx]; exists {
					klog.Infof("[BEDROCK-FUNCTION-DEBUG] ✅ TOOL COMPLETED - ID: %s, Name: %s", partial.id, partial.name)
					
					// Create complete ToolUseBlock
					inputJSON := partial.input.String()
					klog.Infof("[BEDROCK-FUNCTION-DEBUG] Tool input JSON for %s: '%s'", partial.name, inputJSON)
					
					// TODO: Fix Input field creation - currently broken
					toolUse := types.ToolUseBlock{
						ToolUseId: aws.String(partial.id),
						Name:      aws.String(partial.name),
						// Input: ??? - This is the problem
					}
					completedTools = append(completedTools, toolUse)
					
					// Yield tool immediately (like text deltas)
					response := &bedrockStreamResponse{
						content:  "",
						model:    c.model,
						done:     false,
						toolUses: []types.ToolUseBlock{toolUse},
					}
					if !yield(response, nil) {
						return
					}
					
					delete(partialTools, idx)
				}

			case *types.ConverseStreamOutputMemberMetadata:
				// Handle final usage metadata
				if v.Value.Usage != nil {
					finalResponse := &bedrockStreamResponse{
						content: "",
						usage:   v.Value.Usage,
						model:   c.model,
						done:    true,
					}
					yield(finalResponse, nil)
				}
			}
		}

		// Update conversation history with the full response
		if fullContent.Len() > 0 {
			assistantMessage.Content = append(assistantMessage.Content,
				&types.ContentBlockMemberText{Value: fullContent.String()})
		}
		
		// Include completed tools in conversation history
		for _, tool := range completedTools {
			assistantMessage.Content = append(assistantMessage.Content,
				&types.ContentBlockMemberToolUse{Value: tool})
		}
		
		// Only add to history if there's content or tools
		if len(assistantMessage.Content) > 0 {
			c.messages = append(c.messages, assistantMessage)
		}

		// Check for stream errors
		if err := stream.Err(); err != nil {
			yield(nil, fmt.Errorf("stream error: %w", err))
		}
	}, nil
}

// SetFunctionDefinitions configures the available functions for tool use
func (c *bedrockChat) SetFunctionDefinitions(functions []*FunctionDefinition) error {
	c.functionDefs = functions

	if len(functions) == 0 {
		c.toolConfig = nil
		return nil
	}

	// PHASE 3: Tool Schema Conversion Analysis
	klog.Infof("[BEDROCK-FUNCTION-DEBUG] Converting %d function definitions to Bedrock tools", len(functions))

	var tools []types.Tool
	for i, fn := range functions {
		if fn == nil {
			klog.Warningf("[BEDROCK-FUNCTION-DEBUG] Null function definition encountered at index %d", i)
			continue
		}

		klog.Infof("[BEDROCK-FUNCTION-DEBUG] Processing tool: %s", fn.Name)
		klog.V(2).Infof("[BEDROCK-FUNCTION-DEBUG] Tool description: %s", fn.Description)

		// Convert gollm function definition to AWS tool specification
		inputSchema := make(map[string]interface{})
		if fn.Parameters != nil {
			klog.V(2).Infof("[BEDROCK-FUNCTION-DEBUG] Original parameters for %s: %+v",
				fn.Name, fn.Parameters)

			// Convert Schema to map[string]interface{}
			jsonData, err := json.Marshal(fn.Parameters)
			if err != nil {
				klog.Errorf("[BEDROCK-FUNCTION-DEBUG] ❌ Failed to marshal parameters for %s: %v", fn.Name, err)
				return fmt.Errorf("failed to marshal function parameters: %w", err)
			}
			if err := json.Unmarshal(jsonData, &inputSchema); err != nil {
				klog.Errorf("[BEDROCK-FUNCTION-DEBUG] ❌ Failed to unmarshal parameters for %s: %v", fn.Name, err)
				return fmt.Errorf("failed to unmarshal function parameters: %w", err)
			}

			klog.V(2).Infof("[BEDROCK-FUNCTION-DEBUG] Converted schema for %s: %+v",
				fn.Name, inputSchema)
		} else {
			klog.Infof("[BEDROCK-FUNCTION-DEBUG] Tool %s has no parameters", fn.Name)
		}

		toolSpec := types.ToolSpecification{
			Name:        aws.String(fn.Name),
			Description: aws.String(fn.Description),
			InputSchema: &types.ToolInputSchemaMemberJson{
				Value: document.NewLazyDocument(inputSchema),
			},
		}

		klog.V(2).Infof("[BEDROCK-FUNCTION-DEBUG] Schema document created for %s", fn.Name)
		tools = append(tools, &types.ToolMemberToolSpec{Value: toolSpec})
	}

	if len(tools) == 0 {
		klog.Errorf("[BEDROCK-FUNCTION-DEBUG] ❌ No valid tools created from %d function definitions", len(functions))
		c.toolConfig = nil
		return nil
	}

	c.toolConfig = &types.ToolConfiguration{
		Tools: tools,
		ToolChoice: &types.ToolChoiceMemberAny{
			Value: types.AnyToolChoice{},
		},
	}

	klog.Infof("[BEDROCK-FUNCTION-DEBUG] ✅ Successfully created ToolConfiguration with %d tools", len(tools))
	return nil
}

// IsRetryableError determines if an error is retryable
func (c *bedrockChat) IsRetryableError(err error) bool {
	return DefaultIsRetryableError(err)
}

// addContentsToHistory processes and appends contents to conversation history
func (c *bedrockChat) addContentsToHistory(contents []any) error {
	for _, content := range contents {
		switch v := content.(type) {
		case string:
			c.messages = append(c.messages, types.Message{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{Value: v},
				},
			})
			
		case FunctionCallResult:
			// Convert FunctionCallResult to AWS Bedrock ToolResultBlock format
			toolResultBlock := &types.ContentBlockMemberToolResult{
				Value: types.ToolResultBlock{
					ToolUseId: aws.String(v.ID),
					Content: []types.ToolResultContentBlock{
						&types.ToolResultContentBlockMemberJson{
							Value: document.NewLazyDocument(v.Result),
						},
					},
				},
			}
			
			c.messages = append(c.messages, types.Message{
				Role:    types.ConversationRoleUser,
				Content: []types.ContentBlock{toolResultBlock},
			})
			
		default:
			// Fallback to string conversion for backward compatibility
			c.messages = append(c.messages, types.Message{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{Value: fmt.Sprintf("%v", content)},
				},
			})
		}
	}
	
	return nil
}

// bedrockResponse implements ChatResponse for regular (non-streaming) responses
type bedrockResponse struct {
	output *bedrockruntime.ConverseOutput
	model  string
}

// UsageMetadata returns the usage metadata from the response
func (r *bedrockResponse) UsageMetadata() any {
	if r.output != nil && r.output.Usage != nil {
		return r.output.Usage
	}
	return nil
}

// Candidates returns the candidate responses
func (r *bedrockResponse) Candidates() []Candidate {
	if r.output == nil || r.output.Output == nil {
		return []Candidate{}
	}

	if msg, ok := r.output.Output.(*types.ConverseOutputMemberMessage); ok {
		candidate := &bedrockCandidate{
			message: &msg.Value,
			model:   r.model,
		}
		return []Candidate{candidate}
	}

	return []Candidate{}
}

// bedrockStreamResponse implements ChatResponse for streaming responses
type bedrockStreamResponse struct {
	content  string
	usage    *types.TokenUsage
	model    string
	done     bool
	toolUses []types.ToolUseBlock
}

// UsageMetadata returns the usage metadata from the streaming response
func (r *bedrockStreamResponse) UsageMetadata() any {
	return r.usage
}

// Candidates returns the candidate responses for streaming
func (r *bedrockStreamResponse) Candidates() []Candidate {
	if r.content == "" && r.usage == nil && len(r.toolUses) == 0 {
		return []Candidate{}
	}

	candidate := &bedrockStreamCandidate{
		content:  r.content,
		model:    r.model,
		toolUses: r.toolUses,
	}
	return []Candidate{candidate}
}

// bedrockCandidate implements Candidate for regular responses
type bedrockCandidate struct {
	message *types.Message
	model   string
}

// String returns a string representation of the candidate
func (c *bedrockCandidate) String() string {
	if c.message == nil {
		return ""
	}

	var content strings.Builder
	for _, block := range c.message.Content {
		if textBlock, ok := block.(*types.ContentBlockMemberText); ok {
			content.WriteString(textBlock.Value)
		}
	}
	return content.String()
}

// Parts returns the parts of the candidate
func (c *bedrockCandidate) Parts() []Part {
	if c.message == nil {
		return []Part{}
	}

	// PHASE 1: Response Content Block Analysis
	klog.Infof("[BEDROCK-FUNCTION-DEBUG] Processing %d content blocks from Bedrock response", len(c.message.Content))

	var parts []Part
	for i, block := range c.message.Content {
		// CRITICAL: Log actual content block types received from Bedrock
		klog.Infof("[BEDROCK-FUNCTION-DEBUG] Content block %d type: %T", i, block)

		switch v := block.(type) {
		case *types.ContentBlockMemberText:
			klog.Infof("[BEDROCK-FUNCTION-DEBUG] Text content: %.200s...", v.Value)
			parts = append(parts, &bedrockTextPart{text: v.Value})

		case *types.ContentBlockMemberToolUse:
			klog.Infof("[BEDROCK-FUNCTION-DEBUG] ✅ TOOL USE DETECTED - ID: %s, Name: %s",
				aws.ToString(v.Value.ToolUseId), aws.ToString(v.Value.Name))

			// Log input parameters if present
			if v.Value.Input != nil {
				klog.Infof("[BEDROCK-FUNCTION-DEBUG] Tool input type: %T", v.Value.Input)
				klog.Infof("[BEDROCK-FUNCTION-DEBUG] Tool input value: %+v", v.Value.Input)
				
				// Try to unmarshal to see what's inside
				var testMap map[string]any
				if err := v.Value.Input.UnmarshalSmithyDocument(&testMap); err != nil {
					klog.Errorf("[BEDROCK-FUNCTION-DEBUG] Can't unmarshal as map: %v", err)
					var testInterface interface{}
					if err2 := v.Value.Input.UnmarshalSmithyDocument(&testInterface); err2 != nil {
						klog.Errorf("[BEDROCK-FUNCTION-DEBUG] Can't unmarshal as interface: %v", err2)
					} else {
						klog.Infof("[BEDROCK-FUNCTION-DEBUG] Unmarshalled as interface{} type %T: %v", testInterface, testInterface)
					}
				} else {
					klog.Infof("[BEDROCK-FUNCTION-DEBUG] Successfully unmarshalled as map: %v", testMap)
				}
			}

			parts = append(parts, &bedrockToolPart{toolUse: &v.Value})

		default:
			klog.Errorf("[BEDROCK-FUNCTION-DEBUG] ❌ UNKNOWN CONTENT TYPE: %T, Value: %+v", block, block)
		}
	}

	// Log final tool calls count
	var toolCallCount int
	for _, part := range parts {
		if _, isToolPart := part.(*bedrockToolPart); isToolPart {
			toolCallCount++
		}
	}
	klog.Infof("[BEDROCK-FUNCTION-DEBUG] Total tool calls extracted: %d", toolCallCount)

	return parts
}

// bedrockStreamCandidate implements Candidate for streaming responses
type bedrockStreamCandidate struct {
	content  string
	model    string
	toolUses []types.ToolUseBlock
}

// String returns a string representation of the streaming candidate
func (c *bedrockStreamCandidate) String() string {
	return c.content
}

// Parts returns the parts of the streaming candidate
func (c *bedrockStreamCandidate) Parts() []Part {
	var parts []Part
	
	// Handle text content
	if c.content != "" {
		parts = append(parts, &bedrockTextPart{text: c.content})
	}
	
	// Handle tool calls (mirror non-streaming pattern)
	for _, toolUse := range c.toolUses {
		parts = append(parts, &bedrockToolPart{toolUse: &toolUse})
	}
	
	return parts
}

// bedrockTextPart implements Part for text content
type bedrockTextPart struct {
	text string
}

// AsText returns the text content
func (p *bedrockTextPart) AsText() (string, bool) {
	return p.text, true
}

// AsFunctionCalls returns nil since this is a text part
func (p *bedrockTextPart) AsFunctionCalls() ([]FunctionCall, bool) {
	return nil, false
}

// bedrockToolPart implements Part for tool/function calls
type bedrockToolPart struct {
	toolUse *types.ToolUseBlock
}

// AsText returns empty string since this is a tool part
func (p *bedrockToolPart) AsText() (string, bool) {
	return "", false
}

// AsFunctionCalls returns the function calls
func (p *bedrockToolPart) AsFunctionCalls() ([]FunctionCall, bool) {
	if p.toolUse == nil {
		return nil, false
	}

	// Convert AWS tool use to gollm function call
	var args map[string]any
	if p.toolUse.Input != nil {
		if err := p.toolUse.Input.UnmarshalSmithyDocument(&args); err != nil {
			klog.Errorf("Failed to unmarshal tool input: %v", err)
			args = make(map[string]any)
		}
	}

	funcCall := FunctionCall{
		ID:        aws.ToString(p.toolUse.ToolUseId),
		Name:      aws.ToString(p.toolUse.Name),
		Arguments: args,
	}

	return []FunctionCall{funcCall}, true
}

// Helper functions

// getBedrockModel returns the model to use, checking in order:
// 1. Explicitly provided model
// 2. Environment variable BEDROCK_MODEL
// 3. Default model (Claude Sonnet 4)
func getBedrockModel(model string) string {
	if model != "" {
		klog.V(2).Infof("Using explicitly provided model: %s", model)
		return model
	}

	if envModel := os.Getenv("BEDROCK_MODEL"); envModel != "" {
		klog.V(1).Infof("Using model from environment variable: %s", envModel)
		return envModel
	}

	defaultModel := "us.anthropic.claude-sonnet-4-20250514-v1:0"
	klog.V(1).Infof("Using default model: %s", defaultModel)
	return defaultModel
}

// bedrockCompletionResponse wraps a ChatResponse to implement CompletionResponse
type bedrockCompletionResponse struct {
	chatResponse ChatResponse
}

var _ CompletionResponse = (*bedrockCompletionResponse)(nil)

func (r *bedrockCompletionResponse) Response() string {
	if r.chatResponse == nil {
		return ""
	}
	candidates := r.chatResponse.Candidates()
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

func (r *bedrockCompletionResponse) UsageMetadata() any {
	if r.chatResponse == nil {
		return nil
	}
	return r.chatResponse.UsageMetadata()
}
