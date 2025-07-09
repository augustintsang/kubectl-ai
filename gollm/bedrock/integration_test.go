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
	"errors"
	"testing"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBedrockProviderRegistration verifies that the Bedrock provider is properly registered
func TestBedrockProviderRegistration(t *testing.T) {
	ctx := context.Background()

	// Test provider is registered and can be created
	client, err := gollm.NewClient(ctx, "bedrock")

	// We expect this to work (create client) but may fail due to missing AWS credentials
	// The important thing is that the provider is recognized and the factory is called
	if err != nil {
		// If error contains credential-related messages, that's expected in test environment
		errStr := err.Error()
		assert.Contains(t, errStr, "AWS")
	} else {
		// If no error, verify it's a Bedrock client
		assert.NotNil(t, client)
		defer client.Close()
	}
}

// TestBedrockClientOptionsIntegration tests the full ClientOptions integration
func TestBedrockClientOptionsIntegration(t *testing.T) {
	ctx := context.Background()

	// Test various ClientOptions configurations
	testCases := []struct {
		name        string
		options     []gollm.Option
		description string
	}{
		{
			name: "inference_config_only",
			options: []gollm.Option{
				gollm.WithInferenceConfig(&gollm.InferenceConfig{
					Model:       "us.anthropic.claude-sonnet-4-20250514-v1:0",
					Region:      "us-east-1",
					Temperature: 0.8,
					MaxTokens:   2000,
				}),
			},
			description: "Client with inference configuration",
		},
		{
			name: "usage_callback_only",
			options: []gollm.Option{
				gollm.WithUsageCallback(func(provider, model string, usage gollm.Usage) {
					// Callback implementation
				}),
			},
			description: "Client with usage callback",
		},
		{
			name: "full_configuration",
			options: []gollm.Option{
				gollm.WithInferenceConfig(&gollm.InferenceConfig{
					Model:       "us.amazon.nova-pro-v1:0",
					Region:      "us-west-2",
					Temperature: 0.7,
					MaxTokens:   4000,
					TopP:        0.9,
					MaxRetries:  3,
				}),
				gollm.WithUsageCallback(func(provider, model string, usage gollm.Usage) {
					// Full callback
				}),
				gollm.WithDebug(true),
			},
			description: "Client with all options configured",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := gollm.NewClient(ctx, "bedrock", tc.options...)

			// Again, we expect the provider to be recognized
			if err != nil {
				// Credential errors are expected in test environment
				assert.Contains(t, err.Error(), "AWS")
			} else {
				assert.NotNil(t, client)
				defer client.Close()
			}
		})
	}
}

// TestStreamingImplementation tests that streaming follows the correct patterns
func TestStreamingImplementation(t *testing.T) {
	// Test the streaming interface compliance without calling AWS
	client := &BedrockClient{
		options:    DefaultOptions,
		clientOpts: gollm.ClientOptions{},
	}

	chat := client.StartChat("You are a helpful assistant", "us.anthropic.claude-sonnet-4-20250514-v1:0")

	// Verify that the chat session has the correct type for streaming
	session, ok := chat.(*bedrockChatSession)
	require.True(t, ok)

	// Verify the streaming method exists and has correct signature
	assert.NotNil(t, session)
	assert.Equal(t, "us.anthropic.claude-sonnet-4-20250514-v1:0", session.model)

	// Test that the streaming interface is implemented correctly by checking the method exists
	// We can't actually call it without AWS credentials, but we can verify the interface

	// Test with unsupported model to verify error handling
	unsupportedSession := &bedrockChatSession{
		client: client,
		model:  "unsupported-model",
	}

	ctx := context.Background()
	_, err := unsupportedSession.SendStreaming(ctx, "Hello")

	// Should fail due to unsupported model before even trying AWS
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported model")
}

// TestToolCallingInterface tests the tool calling functionality interface
func TestToolCallingInterface(t *testing.T) {
	client := &BedrockClient{
		options:    DefaultOptions,
		clientOpts: gollm.ClientOptions{},
	}

	chat := client.StartChat("You are a helpful assistant", "test-model")

	// Define test function definitions
	functions := []*gollm.FunctionDefinition{
		{
			Name:        "get_weather",
			Description: "Get the current weather for a location",
			Parameters: &gollm.Schema{
				Type: "object",
				Properties: map[string]*gollm.Schema{
					"location": {
						Type:        "string",
						Description: "The city and state, e.g. San Francisco, CA",
					},
					"unit": {
						Type:        "string",
						Description: "Temperature unit (celsius or fahrenheit)",
					},
				},
				Required: []string{"location"},
			},
		},
	}

	// Test SetFunctionDefinitions
	err := chat.SetFunctionDefinitions(functions)
	assert.NoError(t, err)

	// Verify internal state is set correctly
	session, ok := chat.(*bedrockChatSession)
	require.True(t, ok)
	assert.Len(t, session.functionDefs, 1)
	assert.Equal(t, "get_weather", session.functionDefs[0].Name)
}

// TestUsageMetadataStructure tests that usage metadata returns correct structure
func TestUsageMetadataStructure(t *testing.T) {
	// Test with actual AWS types
	awsUsage := &types.TokenUsage{
		InputTokens:  aws.Int32(100),
		OutputTokens: aws.Int32(50),
		TotalTokens:  aws.Int32(150),
	}

	// Test bedrockChatResponse usage metadata
	response := &bedrockChatResponse{
		content:   "Test response",
		usage:     awsUsage,
		toolCalls: []gollm.FunctionCall{},
	}

	metadata := response.UsageMetadata()
	assert.NotNil(t, metadata)

	// Should return structured usage
	usage, ok := metadata.(*gollm.Usage)
	require.True(t, ok)
	assert.Equal(t, 100, usage.InputTokens)
	assert.Equal(t, 50, usage.OutputTokens)
	assert.Equal(t, 150, usage.TotalTokens)
}

// TestFunctionCallResultProcessing tests processing of function call results
func TestFunctionCallResultProcessing(t *testing.T) {
	session := &bedrockChatSession{
		client: &BedrockClient{
			options:    DefaultOptions,
			clientOpts: gollm.ClientOptions{},
		},
		model:        "test-model",
		systemPrompt: "Test system prompt",
		history:      []types.Message{},
		functionDefs: []*gollm.FunctionDefinition{},
	}

	// Test processing function call results
	result := gollm.FunctionCallResult{
		ID:   "call_123",
		Name: "get_weather",
		Result: map[string]interface{}{
			"temperature": 72,
			"condition":   "sunny",
			"humidity":    65,
		},
	}

	// Test processContents with function call result
	message, err := session.processContents(result)
	assert.NoError(t, err)
	assert.Empty(t, message) // Function call results don't return text message

	// Verify history was updated with tool result
	assert.Len(t, session.history, 1)
	assert.Equal(t, types.ConversationRoleUser, session.history[0].Role)
}

// TestModelSupport tests the model support validation
func TestModelSupport(t *testing.T) {
	testCases := []struct {
		model    string
		expected bool
	}{
		// Supported models
		{"us.anthropic.claude-sonnet-4-20250514-v1:0", true},
		{"us.anthropic.claude-3-7-sonnet-20250219-v1:0", true},
		{"us.amazon.nova-pro-v1:0", true},
		{"us.amazon.nova-lite-v1:0", true},
		{"us.amazon.nova-micro-v1:0", true},

		// Case insensitive support
		{"US.ANTHROPIC.CLAUDE-SONNET-4-20250514-V1:0", true},
		{"us.Amazon.Nova-Pro-V1:0", true},

		// ARN support
		{"arn:aws:bedrock:us-west-2:123456789012:inference-profile/us.anthropic.claude-sonnet-4-20250514-v1:0", true},
		{"arn:aws:bedrock:us-east-1:123456789012:foundation-model/us.amazon.nova-pro-v1:0", true},

		// Unsupported models
		{"gpt-4", false},
		{"unsupported-model", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.model, func(t *testing.T) {
			result := isModelSupported(tc.model)
			assert.Equal(t, tc.expected, result, "Model: %s", tc.model)
		})
	}
}

// TestInferenceConfigMerging tests the inference config merging logic thoroughly
func TestInferenceConfigMerging(t *testing.T) {
	testCases := []struct {
		name            string
		defaults        *BedrockOptions
		inferenceConfig *gollm.InferenceConfig
		expectedResult  *BedrockOptions
	}{
		{
			name: "complete_override",
			defaults: &BedrockOptions{
				Region:      "us-west-2",
				Model:       "default-model",
				MaxTokens:   1000,
				Temperature: 0.5,
				TopP:        0.9,
				MaxRetries:  3,
			},
			inferenceConfig: &gollm.InferenceConfig{
				Model:       "custom-model",
				Region:      "us-east-1",
				Temperature: 0.8,
				MaxTokens:   2000,
				TopP:        0.95,
				MaxRetries:  5,
			},
			expectedResult: &BedrockOptions{
				Region:      "us-east-1",
				Model:       "custom-model",
				MaxTokens:   2000,
				Temperature: 0.8,
				TopP:        0.95,
				MaxRetries:  5,
			},
		},
		{
			name: "partial_override",
			defaults: &BedrockOptions{
				Region:      "us-west-2",
				Model:       "default-model",
				MaxTokens:   1000,
				Temperature: 0.5,
				TopP:        0.9,
				MaxRetries:  3,
			},
			inferenceConfig: &gollm.InferenceConfig{
				Temperature: 0.7,
				MaxTokens:   4000,
			},
			expectedResult: &BedrockOptions{
				Region:      "us-west-2",     // From defaults
				Model:       "default-model", // From defaults
				MaxTokens:   4000,            // From config
				Temperature: 0.7,             // From config
				TopP:        0.9,             // From defaults
				MaxRetries:  3,               // From defaults
			},
		},
		{
			name: "zero_values_ignored",
			defaults: &BedrockOptions{
				Region:      "us-west-2",
				Model:       "default-model",
				MaxTokens:   1000,
				Temperature: 0.5,
				TopP:        0.9,
				MaxRetries:  3,
			},
			inferenceConfig: &gollm.InferenceConfig{
				Model:       "new-model", // Should override
				Temperature: 0,           // Should be ignored
				MaxTokens:   0,           // Should be ignored
				TopP:        0.8,         // Should override
			},
			expectedResult: &BedrockOptions{
				Region:      "us-west-2", // From defaults
				Model:       "new-model", // From config
				MaxTokens:   1000,        // From defaults (config was 0)
				Temperature: 0.5,         // From defaults (config was 0)
				TopP:        0.8,         // From config
				MaxRetries:  3,           // From defaults
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientOpts := gollm.ClientOptions{
				InferenceConfig: tc.inferenceConfig,
			}

			result := mergeWithClientOptions(tc.defaults, clientOpts)

			assert.Equal(t, tc.expectedResult.Region, result.Region)
			assert.Equal(t, tc.expectedResult.Model, result.Model)
			assert.Equal(t, tc.expectedResult.MaxTokens, result.MaxTokens)
			assert.Equal(t, tc.expectedResult.Temperature, result.Temperature)
			assert.Equal(t, tc.expectedResult.TopP, result.TopP)
			assert.Equal(t, tc.expectedResult.MaxRetries, result.MaxRetries)
		})
	}
}

// TestSchemaConversion tests the schema conversion for tool definitions
func TestSchemaConversion(t *testing.T) {
	// Test complex schema conversion
	schema := &gollm.Schema{
		Type:        "object",
		Description: "A complex test schema",
		Properties: map[string]*gollm.Schema{
			"name": {
				Type:        "string",
				Description: "The name field",
			},
			"age": {
				Type:        "integer",
				Description: "The age field",
			},
			"hobbies": {
				Type:        "array",
				Description: "List of hobbies",
				Items: &gollm.Schema{
					Type: "string",
				},
			},
			"address": {
				Type:        "object",
				Description: "Address information",
				Properties: map[string]*gollm.Schema{
					"street": {
						Type: "string",
					},
					"city": {
						Type: "string",
					},
				},
				Required: []string{"city"},
			},
		},
		Required: []string{"name", "age"},
	}

	result := convertSchemaToMap(schema)

	// Verify top-level structure
	assert.Equal(t, "object", result["type"])
	assert.Equal(t, "A complex test schema", result["description"])
	assert.Contains(t, result, "properties")
	assert.Equal(t, []string{"name", "age"}, result["required"])

	// Verify properties
	properties, ok := result["properties"].(map[string]any)
	require.True(t, ok)
	assert.Len(t, properties, 4)

	// Verify string property
	nameProperty, ok := properties["name"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", nameProperty["type"])
	assert.Equal(t, "The name field", nameProperty["description"])

	// Verify array property
	hobbiesProperty, ok := properties["hobbies"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "array", hobbiesProperty["type"])
	assert.Contains(t, hobbiesProperty, "items")

	// Verify nested object
	addressProperty, ok := properties["address"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", addressProperty["type"])
	assert.Contains(t, addressProperty, "properties")
	assert.Equal(t, []string{"city"}, addressProperty["required"])
}

// TestResponseStructures tests the response structure implementations
func TestResponseStructures(t *testing.T) {
	// Test bedrockChatResponse
	chatResponse := &bedrockChatResponse{
		content: "Hello, how can I help you?",
		usage:   map[string]interface{}{"tokens": 100},
		toolCalls: []gollm.FunctionCall{
			{
				ID:   "call_1",
				Name: "test_function",
				Arguments: map[string]any{
					"param1": "value1",
				},
			},
		},
	}

	// Test Candidates() method
	candidates := chatResponse.Candidates()
	assert.Len(t, candidates, 1)

	candidate := candidates[0]
	parts := candidate.Parts()

	// Should have both text and function call parts
	assert.Len(t, parts, 2)

	// Check text part
	text, hasText := parts[0].AsText()
	assert.True(t, hasText)
	assert.Equal(t, "Hello, how can I help you?", text)

	// Check function call part
	calls, hasCalls := parts[1].AsFunctionCalls()
	assert.True(t, hasCalls)
	assert.Len(t, calls, 1)
	assert.Equal(t, "call_1", calls[0].ID)
	assert.Equal(t, "test_function", calls[0].Name)

	// Test completion response
	completionResponse := &simpleBedrockCompletionResponse{
		content: "This is a completion response",
		usage:   map[string]interface{}{"tokens": 50},
	}

	assert.Equal(t, "This is a completion response", completionResponse.Response())
	assert.NotNil(t, completionResponse.UsageMetadata())
}

// TestErrorHandling tests error handling and retry logic
func TestErrorHandling(t *testing.T) {
	session := &bedrockChatSession{}

	testCases := []struct {
		errorMessage string
		shouldRetry  bool
	}{
		{"throttling exception occurred", true},
		{"ServiceUnavailable error", true},
		{"InternalServerError happened", true},
		{"RequestTimeout error", true},
		{"validation error", false},
		{"authentication failed", false},
		{"normal processing error", false},
	}

	for _, tc := range testCases {
		t.Run(tc.errorMessage, func(t *testing.T) {
			err := errors.New(tc.errorMessage)
			result := session.IsRetryableError(err)
			assert.Equal(t, tc.shouldRetry, result)
		})
	}
}

// BenchmarkUsageConversion benchmarks the usage conversion performance
func BenchmarkUsageConversion(b *testing.B) {
	awsUsage := &types.TokenUsage{
		InputTokens:  aws.Int32(200),
		OutputTokens: aws.Int32(150),
		TotalTokens:  aws.Int32(350),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertAWSUsage(awsUsage, "test-model", "bedrock")
	}
}

// BenchmarkSchemaConversion benchmarks the schema conversion performance
func BenchmarkSchemaConversion(b *testing.B) {
	schema := &gollm.Schema{
		Type: "object",
		Properties: map[string]*gollm.Schema{
			"name":  {Type: "string"},
			"age":   {Type: "integer"},
			"email": {Type: "string"},
		},
		Required: []string{"name", "email"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convertSchemaToMap(schema)
	}
}
