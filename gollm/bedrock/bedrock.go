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

package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/nirmata/kubectl-ai/gollm"
	"k8s.io/klog/v2"
)

func init() {
	if err := gollm.RegisterProvider("bedrock", newBedrockClientFactory); err != nil {
		klog.Fatalf("Failed to register bedrock provider: %v", err)
	}
	klog.V(1).Info("Successfully registered Bedrock provider with Claude and Nova support")
}

func newBedrockClientFactory(ctx context.Context, opts gollm.ClientOptions) (gollm.Client, error) {
	return NewBedrockClient(ctx, opts)
}

type BedrockClient struct {
	runtimeClient *bedrockruntime.Client
	bedrockClient *bedrock.Client
	options       *BedrockOptions
	clientOpts    gollm.ClientOptions
}

var _ gollm.Client = &BedrockClient{}

func NewBedrockClient(ctx context.Context, opts gollm.ClientOptions) (*BedrockClient, error) {
	options := mergeWithClientOptions(DefaultOptions, opts)
	client, err := NewBedrockClientWithOptions(ctx, options)
	if err != nil {
		return nil, err
	}

	client.clientOpts = opts
	return client, nil
}

func mergeWithClientOptions(defaults *BedrockOptions, opts gollm.ClientOptions) *BedrockOptions {
	merged := *defaults

	if opts.InferenceConfig != nil {
		config := opts.InferenceConfig
		if config.Model != "" {
			merged.Model = config.Model
		}
		if config.Region != "" {
			merged.Region = config.Region
		}
		if config.Temperature != 0 {
			merged.Temperature = config.Temperature
		}
		if config.MaxTokens != 0 {
			merged.MaxTokens = config.MaxTokens
		}
		if config.TopP != 0 {
			merged.TopP = config.TopP
		}
		if config.MaxRetries != 0 {
			merged.MaxRetries = config.MaxRetries
		}
	}

	return &merged
}

func convertAWSUsage(awsUsage any, model, provider string) *gollm.Usage {
	if awsUsage == nil {
		return nil
	}

	if usage, ok := awsUsage.(*types.TokenUsage); ok {
		return &gollm.Usage{
			InputTokens:  int(aws.ToInt32(usage.InputTokens)),
			OutputTokens: int(aws.ToInt32(usage.OutputTokens)),
			TotalTokens:  int(aws.ToInt32(usage.TotalTokens)),
			Model:        model,
			Provider:     provider,
			Timestamp:    time.Now(),
		}
	}

	return nil
}

func NewBedrockClientWithOptions(ctx context.Context, options *BedrockOptions) (*BedrockClient, error) {
	klog.V(1).Infof("Initializing Bedrock client - Region: %s, Model: %s", options.Region, options.Model)
	configOptions := []func(*config.LoadOptions) error{
		config.WithRegion(options.Region),
	}

	if options.CredentialsProvider != nil {
		configOptions = append(configOptions, config.WithCredentialsProvider(options.CredentialsProvider))
	}
	if options.MaxRetries > 0 {
		configOptions = append(configOptions, config.WithRetryMaxAttempts(options.MaxRetries))
	}

	// Create a timeout context for AWS config loading to prevent indefinite hangs
	// This is crucial because config.LoadDefaultConfig can hang during credential resolution
	configTimeout := 30 * time.Second
	if options.Timeout > 0 {
		configTimeout = options.Timeout
	}

	configCtx, cancel := context.WithTimeout(ctx, configTimeout)
	defer cancel()

	klog.V(2).Infof("Loading AWS configuration with timeout: %v", configTimeout)

	cfg, err := config.LoadDefaultConfig(configCtx, configOptions...)
	if err != nil {
		// Check if the error was due to context timeout
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s: AWS configuration loading timed out after %v - this usually indicates credential or network issues. Please check your AWS credentials and network connectivity: %w", ErrMsgConfigLoad, configTimeout, err)
		}
		return nil, fmt.Errorf("%s: %w", ErrMsgConfigLoad, err)
	}

	klog.V(2).Info("AWS configuration loaded successfully")

	return &BedrockClient{
		runtimeClient: bedrockruntime.NewFromConfig(cfg),
		bedrockClient: bedrock.NewFromConfig(cfg),
		options:       options,
	}, nil
}

func (c *BedrockClient) Close() error {
	klog.V(2).Info("Bedrock client closed")
	return nil
}

func (c *BedrockClient) StartChat(systemPrompt, model string) gollm.Chat {
	selectedModel := model
	if selectedModel == "" {
		selectedModel = c.options.Model
	}

	if !isModelSupported(selectedModel) {
		klog.Errorf("Unsupported model requested: %s, falling back to default: %s", selectedModel, c.options.Model)
		return &bedrockChatSession{
			client:       c,
			systemPrompt: systemPrompt,
			model:        c.options.Model,
			history:      make([]types.Message, 0),
			functionDefs: make([]*gollm.FunctionDefinition, 0),
		}
	}

	klog.V(1).Infof("Starting chat session with model: %s", selectedModel)

	return &bedrockChatSession{
		client:       c,
		systemPrompt: systemPrompt,
		model:        selectedModel,
		history:      make([]types.Message, 0),
		functionDefs: make([]*gollm.FunctionDefinition, 0),
	}
}

func (c *BedrockClient) GenerateCompletion(ctx context.Context, req *gollm.CompletionRequest) (gollm.CompletionResponse, error) {
	klog.V(1).Infof("GenerateCompletion called with model: %s", req.Model)

	model := req.Model
	if model == "" {
		model = c.options.Model
	}

	if !isModelSupported(model) {
		return nil, fmt.Errorf("%s: %s", ErrMsgUnsupportedModel, model)
	}

	chat := c.StartChat("", model)

	response, err := chat.Send(ctx, req.Prompt)
	if err != nil {
		return nil, fmt.Errorf("completion failed: %w", err)
	}

	return &simpleBedrockCompletionResponse{
		content:  extractTextFromResponse(response),
		usage:    response.UsageMetadata(),
		model:    model,
		provider: "bedrock",
	}, nil
}

func (c *BedrockClient) SetResponseSchema(schema *gollm.Schema) error {
	klog.V(1).Info("Response schema set for Bedrock client")
	return nil
}

func (c *BedrockClient) ListModels(ctx context.Context) ([]string, error) {
	return getSupportedModels(), nil
}

type bedrockChatSession struct {
	client       *BedrockClient
	systemPrompt string
	model        string
	history      []types.Message
	functionDefs []*gollm.FunctionDefinition
}

var _ gollm.Chat = (*bedrockChatSession)(nil)

func (cs *bedrockChatSession) SetFunctionDefinitions(defs []*gollm.FunctionDefinition) error {
	cs.functionDefs = defs
	klog.V(1).Infof("SetFunctionDefinitions called with %d definitions", len(defs))
	return nil
}

func (cs *bedrockChatSession) Send(ctx context.Context, contents ...any) (gollm.ChatResponse, error) {
	if !isModelSupported(cs.model) {
		return nil, fmt.Errorf("%s: %s. Supported models: %v",
			ErrMsgUnsupportedModel, cs.model, getSupportedModels())
	}

	userMessage, err := cs.processContents(contents...)
	if err != nil {
		return nil, err
	}

	if userMessage != "" {
		cs.addTextMessage(types.ConversationRoleUser, userMessage)
	}
	input := cs.buildConverseInput()

	// Log tool configuration details
	if input.ToolConfig != nil && len(input.ToolConfig.Tools) > 0 {
		klog.V(1).Infof("Sending Converse request with %d tools enabled", len(input.ToolConfig.Tools))
		for i, tool := range input.ToolConfig.Tools {
			if toolSpec, ok := tool.(*types.ToolMemberToolSpec); ok {
				toolName := "unknown"
				if toolSpec.Value.Name != nil {
					toolName = *toolSpec.Value.Name
				}
				klog.V(2).Infof("Tool %d: %s", i, toolName)
			}
		}
	} else {
		klog.V(2).Info("Sending Converse request with no tools")
	}

	klog.V(2).Infof("Sending Converse request for model: %s with %d messages in history", cs.model, len(cs.history))

	output, err := cs.client.runtimeClient.Converse(ctx, input)
	if err != nil {
		cs.removeLastMessage()
		return nil, fmt.Errorf("converse API failed: %w", err)
	}

	response := cs.parseConverseOutput(&output.Output)
	response.usage = output.Usage

	if cs.client.clientOpts.UsageCallback != nil {
		if structuredUsage := convertAWSUsage(output.Usage, cs.model, "bedrock"); structuredUsage != nil {
			cs.client.clientOpts.UsageCallback("bedrock", cs.model, *structuredUsage)
		}
	}

	cs.addAssistantResponse(response)

	return response, nil
}

func (cs *bedrockChatSession) SendStreaming(ctx context.Context, contents ...any) (gollm.ChatResponseIterator, error) {
	if !isModelSupported(cs.model) {
		return nil, fmt.Errorf("%s: %s. Supported models: %v",
			ErrMsgUnsupportedModel, cs.model, getSupportedModels())
	}

	userMessage, err := cs.processContents(contents...)
	if err != nil {
		return nil, err
	}

	cs.addTextMessage(types.ConversationRoleUser, userMessage)

	input := cs.buildConverseStreamInput()
	klog.V(2).Infof("Starting ConverseStream for model: %s", cs.model)
	output, err := cs.client.runtimeClient.ConverseStream(ctx, input)
	if err != nil {
		cs.removeLastMessage()
		return nil, fmt.Errorf("ConverseStream failed: %w", err)
	}
	return cs.createStreamingIterator(output), nil
}

func (cs *bedrockChatSession) processContents(contents ...any) (string, error) {
	var messages []string
	var toolResults []types.ContentBlock

	for _, content := range contents {
		switch c := content.(type) {
		case string:
			messages = append(messages, c)
		case gollm.FunctionCallResult:
			toolResult := &types.ContentBlockMemberToolResult{
				Value: types.ToolResultBlock{
					ToolUseId: aws.String(c.ID),
					Content: []types.ToolResultContentBlock{
						&types.ToolResultContentBlockMemberText{
							Value: cs.formatToolResult(c),
						},
					},
				},
			}
			toolResults = append(toolResults, toolResult)
		default:
			return "", fmt.Errorf("unsupported content type: %T", content)
		}
	}

	if len(messages) > 0 && len(toolResults) > 0 {
		return "", fmt.Errorf("cannot mix text messages and tool results in the same call")
	}

	if len(toolResults) > 0 {
		cs.addToolResults(toolResults)
		return "", nil
	}

	if len(messages) == 0 {
		return "", errors.New("no valid messages provided")
	}

	return strings.Join(messages, "\n"), nil
}

func (cs *bedrockChatSession) formatToolResult(result gollm.FunctionCallResult) string {
	if result.Result == nil {
		return fmt.Sprintf("Tool %s completed successfully", result.Name)
	}

	resultJSON, err := json.Marshal(result.Result)
	if err != nil {
		return fmt.Sprintf("Tool %s completed with result: %v", result.Name, result.Result)
	}

	return string(resultJSON)
}

func (cs *bedrockChatSession) addMessage(role types.ConversationRole, contentBlocks ...types.ContentBlock) {
	if len(contentBlocks) == 0 {
		klog.V(3).Infof("Skipping empty message for role: %s", role)
		return
	}

	message := types.Message{
		Role:    role,
		Content: contentBlocks,
	}

	cs.history = append(cs.history, message)
}

func (cs *bedrockChatSession) addTextMessage(role types.ConversationRole, content string) {
	if content == "" {
		return
	}
	textBlock := &types.ContentBlockMemberText{Value: content}
	cs.addMessage(role, textBlock)
}

func (cs *bedrockChatSession) addToolResults(toolResults []types.ContentBlock) {
	if len(toolResults) > 0 {
		cs.addMessage(types.ConversationRoleUser, toolResults...)
	}
}

func (cs *bedrockChatSession) addAssistantResponse(response *bedrockChatResponse) {
	var contentBlocks []types.ContentBlock

	if response.content != "" {
		contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
			Value: response.content,
		})
	}
	for _, toolCall := range response.toolCalls {
		toolUseBlock := cs.createToolUseBlock(toolCall)
		contentBlocks = append(contentBlocks, toolUseBlock)
	}

	if len(contentBlocks) > 0 {
		cs.addMessage(types.ConversationRoleAssistant, contentBlocks...)
	}
}

func (cs *bedrockChatSession) createToolUseBlock(toolCall gollm.FunctionCall) *types.ContentBlockMemberToolUse {
	toolUseBlock := &types.ContentBlockMemberToolUse{
		Value: types.ToolUseBlock{
			ToolUseId: aws.String(toolCall.ID),
			Name:      aws.String(toolCall.Name),
		},
	}

	var inputDoc document.Interface
	if len(toolCall.Arguments) > 0 {
		inputDoc = document.NewLazyDocument(toolCall.Arguments)
	} else {
		inputDoc = document.NewLazyDocument(map[string]any{})
	}
	toolUseBlock.Value.Input = inputDoc

	return toolUseBlock
}

func (cs *bedrockChatSession) removeLastMessage() {
	if len(cs.history) > 0 {
		cs.history = cs.history[:len(cs.history)-1]
		klog.V(3).Info("Removed last message from history")
	}
}

func (cs *bedrockChatSession) buildConverseInput() *bedrockruntime.ConverseInput {
	input := &bedrockruntime.ConverseInput{
		ModelId:  aws.String(cs.model),
		Messages: cs.history,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens:   aws.Int32(cs.client.options.MaxTokens),
			Temperature: aws.Float32(cs.client.options.Temperature),
			TopP:        aws.Float32(cs.client.options.TopP),
		},
	}

	if cs.systemPrompt != "" {
		input.System = []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: cs.systemPrompt},
		}
		klog.V(2).Info("Added system prompt to Bedrock input")
	}

	if len(cs.functionDefs) > 0 {
		klog.V(1).Infof("Setting up tool configuration with %d function definitions", len(cs.functionDefs))
		tools := cs.buildTools()
		if len(tools) > 0 {
			input.ToolConfig = &types.ToolConfiguration{
				Tools: tools,
				ToolChoice: &types.ToolChoiceMemberAny{Value: types.AnyToolChoice{}},
			}
			klog.V(1).Infof("Tool configuration set with %d tools and ToolChoice=Any", len(tools))
		} else {
			klog.V(1).Info("No tools built despite having function definitions")
		}
	} else {
		klog.V(2).Info("No function definitions provided, skipping tool configuration")
	}

	return input
}

func (cs *bedrockChatSession) buildConverseStreamInput() *bedrockruntime.ConverseStreamInput {
	input := &bedrockruntime.ConverseStreamInput{
		ModelId:  aws.String(cs.model),
		Messages: cs.history,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens:   aws.Int32(cs.client.options.MaxTokens),
			Temperature: aws.Float32(cs.client.options.Temperature),
			TopP:        aws.Float32(cs.client.options.TopP),
		},
	}

	if cs.systemPrompt != "" {
		input.System = []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: cs.systemPrompt},
		}
	}

	if len(cs.functionDefs) > 0 {
		tools := cs.buildTools()
		if len(tools) > 0 {
			input.ToolConfig = &types.ToolConfiguration{
				Tools: tools,
				ToolChoice: &types.ToolChoiceMemberAny{Value: types.AnyToolChoice{}},
			}
			klog.V(1).Infof("Tool configuration set with %d tools and ToolChoice=Any", len(tools))
		}
	}

	return input
}

func (cs *bedrockChatSession) buildTools() []types.Tool {
	if len(cs.functionDefs) == 0 {
		klog.V(2).Info("No function definitions provided, returning empty tools")
		return []types.Tool{}
	}

	tools := make([]types.Tool, 0, len(cs.functionDefs))
	klog.V(1).Infof("Building %d tools for Bedrock", len(cs.functionDefs))

	for i, funcDef := range cs.functionDefs {
		if funcDef == nil {
			klog.V(2).Infof("Skipping nil function definition at index %d", i)
			continue
		}

		klog.V(2).Infof("Building tool %q with description: %q", funcDef.Name, funcDef.Description)

		toolSpec := &types.ToolSpecification{
			Name:        aws.String(funcDef.Name),
			Description: aws.String(funcDef.Description),
		}

		if funcDef.Parameters != nil {
			schemaMap := convertSchemaToMap(funcDef.Parameters)
			if schemaMap != nil {
				klog.V(3).Infof("Tool %q schema: %+v", funcDef.Name, schemaMap)
				schemaDoc := document.NewLazyDocument(schemaMap)
				toolSpec.InputSchema = &types.ToolInputSchemaMemberJson{
					Value: schemaDoc,
				}
			} else {
				klog.V(2).Infof("BEDROCK_DEBUG: Tool %q has nil schema after conversion", funcDef.Name)
			}
		} else {
			klog.V(2).Infof("BEDROCK_DEBUG: Tool %q has no parameters", funcDef.Name)
		}

		tool := &types.ToolMemberToolSpec{
			Value: *toolSpec,
		}

		tools = append(tools, tool)
		klog.V(2).Infof("Successfully built tool %q", funcDef.Name)
	}

	klog.V(1).Infof("Built %d tools for Bedrock successfully", len(tools))
	return tools
}

func convertSchemaToMap(schema *gollm.Schema) map[string]any {
	if schema == nil {
		return nil
	}

	schemaMap := make(map[string]any)

	if schema.Type != "" {
		schemaMap["type"] = string(schema.Type)
	}
	if schema.Description != "" {
		schemaMap["description"] = schema.Description
	}

	// Handle properties for object types
	if len(schema.Properties) > 0 {
		properties := make(map[string]any)
		for propName, prop := range schema.Properties {
			properties[propName] = convertSchemaToMap(prop)
		}
		schemaMap["properties"] = properties
	}

	// Handle required fields (not just for objects)
	if len(schema.Required) > 0 {
		schemaMap["required"] = schema.Required
	}

	if schema.Type == "array" && schema.Items != nil {
		schemaMap["items"] = convertSchemaToMap(schema.Items)
	}

	return schemaMap
}

func (cs *bedrockChatSession) parseConverseOutput(output *types.ConverseOutput) *bedrockChatResponse {
	response := &bedrockChatResponse{
		usage:     nil,
		toolCalls: []gollm.FunctionCall{},
		model:     cs.model,
		provider:  "bedrock",
	}

	if messageOutput, ok := (*output).(*types.ConverseOutputMemberMessage); ok {
		message := messageOutput.Value
		klog.V(2).Infof("Parsing Bedrock response with %d content blocks", len(message.Content))

		if len(message.Content) > 0 {
			var contentParts []string
			for i, content := range message.Content {
				switch c := content.(type) {
				case *types.ContentBlockMemberText:
					klog.V(3).Infof("Content block %d: text content (length: %d)", i, len(c.Value))
					contentParts = append(contentParts, c.Value)
				case *types.ContentBlockMemberToolUse:
					klog.V(2).Infof("Content block %d: tool use detected", i)
					toolCall := gollm.FunctionCall{}

					if c.Value.ToolUseId != nil {
						toolCall.ID = *c.Value.ToolUseId
						klog.V(3).Infof("Tool call ID: %s", toolCall.ID)
					}
					if c.Value.Name != nil {
						toolCall.Name = *c.Value.Name
						klog.V(2).Infof("Tool call name: %s", toolCall.Name)
					}

					if c.Value.Input != nil {
						var inputValue any
						if err := c.Value.Input.UnmarshalSmithyDocument(&inputValue); err != nil {
							klog.Errorf("Failed to unmarshal tool call arguments for %s: %v", toolCall.Name, err)
							toolCall.Arguments = map[string]any{}
						} else {
							if argMap, ok := inputValue.(map[string]any); ok {
								toolCall.Arguments = argMap
								klog.V(3).Infof("Tool call %s arguments: %+v", toolCall.Name, argMap)
							} else {
								klog.Errorf("Tool call %s arguments are not map[string]any, got %T: %v", toolCall.Name, inputValue, inputValue)
								toolCall.Arguments = map[string]any{}
							}
						}
					} else {
						klog.V(3).Infof("Tool call %s has no input arguments", toolCall.Name)
						toolCall.Arguments = map[string]any{}
					}

					response.toolCalls = append(response.toolCalls, toolCall)
					klog.V(2).Infof("Added tool call: %s (ID: %s)", toolCall.Name, toolCall.ID)
				default:
					klog.V(2).Infof("Content block %d: unknown type %T", i, c)
				}
			}
			response.content = strings.Join(contentParts, "\n")
			klog.V(1).Infof("Parsed response: %d tool calls, content length: %d", len(response.toolCalls), len(response.content))
		} else {
			klog.V(2).Info("Bedrock response has no content blocks")
		}
	} else {
		klog.Errorf("Unexpected ConverseOutput type: %T", *output)
		response.content = "Error: Unable to parse response"
	}

	return response
}

func (cs *bedrockChatSession) createStreamingIterator(output *bedrockruntime.ConverseStreamOutput) gollm.ChatResponseIterator {
	return func(yield func(gollm.ChatResponse, error) bool) {
		if output == nil || output.GetStream() == nil {
			yield(nil, fmt.Errorf("streaming output or stream is nil"))
			return
		}

		defer output.GetStream().Close()

		var fullContent strings.Builder
		var usage any
		var currentToolCall *gollm.FunctionCall
		var collectedToolCalls []gollm.FunctionCall

		for event := range output.GetStream().Events() {
			switch e := event.(type) {
			case *types.ConverseStreamOutputMemberMessageStart:
				klog.V(3).Info("Stream: Message started")

			case *types.ConverseStreamOutputMemberContentBlockStart:
				if start := e.Value.Start; start != nil {
					if toolStart, ok := start.(*types.ContentBlockStartMemberToolUse); ok {
						klog.V(2).Info("Stream: Tool use block started")
						currentToolCall = &gollm.FunctionCall{}
						
						if toolStart.Value.ToolUseId != nil {
							currentToolCall.ID = *toolStart.Value.ToolUseId
							klog.V(3).Infof("Stream: Tool call ID: %s", currentToolCall.ID)
						}
						if toolStart.Value.Name != nil {
							currentToolCall.Name = *toolStart.Value.Name
							klog.V(2).Infof("Stream: Tool call name: %s", currentToolCall.Name)
						}
						currentToolCall.Arguments = map[string]any{}
					} else {
						klog.V(3).Info("Stream: Content block started")
					}
				}

			case *types.ConverseStreamOutputMemberContentBlockDelta:
				if delta := e.Value.Delta; delta != nil {
					if textDelta, ok := delta.(*types.ContentBlockDeltaMemberText); ok {
						text := textDelta.Value
						fullContent.WriteString(text)

						response := &bedrockChatResponse{
							content:   text,
							usage:     nil,
							toolCalls: []gollm.FunctionCall{},
							model:     cs.model,
							provider:  "bedrock",
						}

						if !yield(response, nil) {
							return
						}
					} else if toolDelta, ok := delta.(*types.ContentBlockDeltaMemberToolUse); ok && currentToolCall != nil {
						klog.V(3).Info("Stream: Tool use delta received")
						if toolDelta.Value.Input != nil {
							// Parse the JSON string input incrementally
							inputStr := *toolDelta.Value.Input
							klog.V(3).Infof("Stream: Tool call %s input delta: %s", currentToolCall.Name, inputStr)
							
							// Try to parse the accumulated input as JSON
							// Note: This is a simplified approach - in a full implementation,
							// you might want to buffer partial JSON strings until complete
							var inputValue any
							if err := json.Unmarshal([]byte(inputStr), &inputValue); err != nil {
								klog.V(3).Infof("Stream: Partial JSON input for %s (will continue buffering): %v", currentToolCall.Name, err)
							} else {
								if argMap, ok := inputValue.(map[string]any); ok {
									currentToolCall.Arguments = argMap
									klog.V(3).Infof("Stream: Tool call %s arguments updated: %+v", currentToolCall.Name, argMap)
								}
							}
						}
					}
				}

			case *types.ConverseStreamOutputMemberContentBlockStop:
				if currentToolCall != nil {
					klog.V(2).Infof("Stream: Tool use block stopped - Added tool call: %s (ID: %s)", currentToolCall.Name, currentToolCall.ID)
					collectedToolCalls = append(collectedToolCalls, *currentToolCall)
					
					// Yield the tool call immediately when it's complete
					response := &bedrockChatResponse{
						content:   "",
						usage:     nil,
						toolCalls: []gollm.FunctionCall{*currentToolCall},
						model:     cs.model,
						provider:  "bedrock",
					}
					
					if !yield(response, nil) {
						return
					}
					
					currentToolCall = nil
				} else {
					klog.V(3).Info("Stream: Content block stopped")
				}

			case *types.ConverseStreamOutputMemberMessageStop:
				klog.V(2).Info("Stream: Message completed")
				if fullContent.Len() > 0 {
					cs.addTextMessage(types.ConversationRoleAssistant, fullContent.String())
				}
				if len(collectedToolCalls) > 0 {
					// Create content blocks for tool calls
					var toolBlocks []types.ContentBlock
					for _, toolCall := range collectedToolCalls {
						toolUseBlock := cs.createToolUseBlock(toolCall)
						toolBlocks = append(toolBlocks, toolUseBlock)
					}
					cs.addMessage(types.ConversationRoleAssistant, toolBlocks...)
				}

			case *types.ConverseStreamOutputMemberMetadata:
				if e.Value.Usage != nil {
					usage = e.Value.Usage

					if cs.client.clientOpts.UsageCallback != nil {
						if structuredUsage := convertAWSUsage(usage, cs.model, "bedrock"); structuredUsage != nil {
							cs.client.clientOpts.UsageCallback("bedrock", cs.model, *structuredUsage)
							klog.V(2).Infof("Usage callback invoked for streaming: %d tokens", structuredUsage.TotalTokens)
						}
					}

					finalResponse := &bedrockChatResponse{
						content:   "",
						usage:     usage,
						toolCalls: []gollm.FunctionCall{},
						model:     cs.model,
						provider:  "bedrock",
					}

					if !yield(finalResponse, nil) {
						return
					}
				}

			default:
				klog.V(3).Infof("Stream: Unknown event type: %T", e)
			}
		}

		if err := output.GetStream().Err(); err != nil {
			yield(nil, fmt.Errorf("stream error: %w", err))
		}
	}
}

func (cs *bedrockChatSession) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	retryableErrors := []string{
		"throttling",
		"serviceunavailable",
		"internalservererror",
		"requesttimeout",
	}

	for _, retryableErr := range retryableErrors {
		if strings.Contains(errStr, retryableErr) {
			return true
		}
	}

	return false
}
