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

// AWS Integration Tests - These tests require real AWS credentials and make actual API calls
// Run with: go test -tags=aws_integration -v ./...
//go:build aws_integration

package bedrock

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// Test models that should be available in most AWS accounts
	ClaudeModel = "us.anthropic.claude-3-7-sonnet-20250219-v1:0"
	NovaModel   = "us.amazon.nova-micro-v1:0" // Changed from nova-lite to nova-micro (accessible)

	// Test prompts
	SimplePrompt    = "Hello! Please respond with exactly 'Hello world' and nothing else."
	StreamingPrompt = "Count from 1 to 5, putting each number on a new line. Be concise."
	ToolPrompt      = "What's the weather like in San Francisco? Use the get_weather function."
)

// TestAWSCredentials verifies that AWS credentials are properly configured
func TestAWSCredentials(t *testing.T) {
	ctx := context.Background()

	// Test 1: Verify AWS config can be loaded
	awsConfig, err := config.LoadDefaultConfig(ctx)
	require.NoError(t, err, "AWS credentials must be configured for integration tests")
	require.NotNil(t, awsConfig, "AWS config should be loaded")

	// Test 2: Verify region is set
	region := awsConfig.Region
	require.NotEmpty(t, region, "AWS region must be configured")
	t.Logf("Using AWS region: %s", region)

	// Test 3: Verify we can create Bedrock clients
	bedrockClient := bedrock.NewFromConfig(awsConfig)
	bedrockRuntimeClient := bedrockruntime.NewFromConfig(awsConfig)
	require.NotNil(t, bedrockClient, "Should be able to create Bedrock client")
	require.NotNil(t, bedrockRuntimeClient, "Should be able to create Bedrock runtime client")

	// Test 4: Test credentials by making a simple API call
	// We'll use ListFoundationModels to verify credentials work
	input := &bedrock.ListFoundationModelsInput{}
	_, err = bedrockClient.ListFoundationModels(ctx, input)
	if err != nil {
		// If this fails, it's likely a credentials issue
		require.NoError(t, err, "AWS credentials test failed - ensure SSO is active and permissions are correct")
	}

	t.Log("✅ AWS credentials verified successfully")
}

// TestRealBedrockClientCreation tests creating real Bedrock clients with various configurations
func TestRealBedrockClientCreation(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name    string
		options []gollm.Option
		model   string
	}{
		{
			name:  "default_configuration",
			model: ClaudeModel,
		},
		{
			name: "with_inference_config",
			options: []gollm.Option{
				gollm.WithInferenceConfig(&gollm.InferenceConfig{
					Model:       NovaModel,
					Temperature: 0.7,
					MaxTokens:   1000,
					TopP:        0.9,
				}),
			},
			model: NovaModel,
		},
		{
			name: "with_usage_callback",
			options: []gollm.Option{
				gollm.WithUsageCallback(func(provider, model string, usage gollm.Usage) {
					// This will be tested by the actual usage tests below
					t.Logf("Usage callback called: %s/%s - %d tokens", provider, model, usage.TotalTokens)
				}),
			},
			model: ClaudeModel,
		},
		{
			name: "full_configuration",
			options: []gollm.Option{
				gollm.WithInferenceConfig(&gollm.InferenceConfig{
					Model:       ClaudeModel,
					Temperature: 0.5,
					MaxTokens:   2000,
					TopP:        0.8,
					MaxRetries:  3,
				}),
				gollm.WithUsageCallback(func(provider, model string, usage gollm.Usage) {
					t.Logf("Full config usage: %+v", usage)
				}),
				gollm.WithDebug(true),
			},
			model: ClaudeModel,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := gollm.NewClient(ctx, "bedrock", tc.options...)
			require.NoError(t, err, "Should create Bedrock client successfully")
			require.NotNil(t, client, "Client should not be nil")

			// Verify we can start a chat session
			chat := client.StartChat("You are a helpful assistant.", tc.model)
			require.NotNil(t, chat, "Should be able to start chat session")

			client.Close()
			t.Logf("✅ Successfully created and closed client for: %s", tc.name)
		})
	}
}

// TestRealStreamingFunctionality tests actual streaming responses from AWS Bedrock
func TestRealStreamingFunctionality(t *testing.T) {
	ctx := context.Background()

	// Create client with usage tracking for streaming
	var usageRecords []gollm.Usage
	var usageMutex sync.Mutex

	client, err := gollm.NewClient(ctx, "bedrock",
		gollm.WithInferenceConfig(&gollm.InferenceConfig{
			Model:       ClaudeModel,
			Temperature: 0.3,
			MaxTokens:   500,
		}),
		gollm.WithUsageCallback(func(provider, model string, usage gollm.Usage) {
			usageMutex.Lock()
			defer usageMutex.Unlock()
			usageRecords = append(usageRecords, usage)
			t.Logf("Streaming usage callback: %s/%s - Input: %d, Output: %d, Total: %d",
				provider, model, usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
		}),
		gollm.WithDebug(true),
	)
	require.NoError(t, err, "Should create client for streaming test")
	defer client.Close()

	chat := client.StartChat("You are a helpful assistant. Be concise.", ClaudeModel)
	require.NotNil(t, chat, "Should start chat session")

	// Test streaming response
	t.Run("streaming_response", func(t *testing.T) {
		responseCount := 0
		var fullResponse strings.Builder
		var lastUsage gollm.Usage

		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		responseStream, err := chat.SendStreaming(ctx, StreamingPrompt)
		require.NoError(t, err, "Should start streaming successfully")

		// Collect streaming responses
		for response, streamErr := range responseStream {
			if streamErr != nil {
				require.NoError(t, streamErr, "Stream should not have errors")
				break
			}

			if response == nil {
				break // End of stream
			}

			responseCount++

			// Verify response structure
			require.NotNil(t, response, "Response should not be nil")

			candidates := response.Candidates()
			if len(candidates) > 0 {
				parts := candidates[0].Parts()
				for _, part := range parts {
					if text, ok := part.AsText(); ok && text != "" {
						fullResponse.WriteString(text)
						t.Logf("Stream chunk %d: %q", responseCount, text)
					}
				}
			}

			// Check if usage metadata is available
			if response.UsageMetadata() != nil {
				if usage := response.UsageMetadata().(*gollm.Usage); usage != nil {
					lastUsage = *usage
				}
			}
		}

		// Verify we received streaming responses
		assert.Greater(t, responseCount, 0, "Should receive at least one streaming response")

		// Verify we got some content
		fullText := fullResponse.String()
		assert.NotEmpty(t, fullText, "Should receive some text content")
		t.Logf("Full streaming response: %q", fullText)

		// Verify usage data was captured
		assert.Greater(t, lastUsage.TotalTokens, 0, "Should have token usage data")
		assert.Equal(t, "bedrock", lastUsage.Provider, "Provider should be bedrock")
		assert.Equal(t, ClaudeModel, lastUsage.Model, "Model should match")

		// Verify usage callback was called
		usageMutex.Lock()
		assert.Greater(t, len(usageRecords), 0, "Usage callback should have been called")
		usageMutex.Unlock()

		t.Logf("✅ Streaming test completed - %d chunks, %d tokens", responseCount, lastUsage.TotalTokens)
	})
}

// TestRealUsageTracking tests comprehensive usage tracking with real API calls
func TestRealUsageTracking(t *testing.T) {
	ctx := context.Background()

	// Track all usage across multiple requests
	var allUsage []gollm.Usage
	var usageMutex sync.Mutex

	client, err := gollm.NewClient(ctx, "bedrock",
		gollm.WithInferenceConfig(&gollm.InferenceConfig{
			Model:       ClaudeModel,
			Temperature: 0.1,
			MaxTokens:   200,
		}),
		gollm.WithUsageCallback(func(provider, model string, usage gollm.Usage) {
			usageMutex.Lock()
			defer usageMutex.Unlock()
			allUsage = append(allUsage, usage)

			// Verify usage structure
			assert.Equal(t, "bedrock", provider)
			assert.Equal(t, ClaudeModel, model)
			assert.Greater(t, usage.TotalTokens, 0, "Should have token count")
			assert.Greater(t, usage.InputTokens, 0, "Should have input tokens")
			assert.GreaterOrEqual(t, usage.OutputTokens, 0, "Should have output tokens >= 0")
			assert.Equal(t, usage.InputTokens+usage.OutputTokens, usage.TotalTokens, "Total should equal sum")
			assert.NotZero(t, usage.Timestamp, "Should have timestamp")
		}),
	)
	require.NoError(t, err, "Should create client for usage tracking test")
	defer client.Close()

	chat := client.StartChat("You are a helpful assistant.", ClaudeModel)

	// Test multiple requests to accumulate usage
	prompts := []string{
		"Say hello.",
		"Count to 3.",
		"What is 2+2?",
	}

	for i, prompt := range prompts {
		t.Run(fmt.Sprintf("request_%d", i+1), func(t *testing.T) {
			response, err := chat.Send(ctx, prompt)
			require.NoError(t, err, "Request should succeed")
			require.NotNil(t, response, "Response should not be nil")

			// Verify response has usage metadata
			usageMetadata := response.UsageMetadata()
			require.NotNil(t, usageMetadata, "Response should have usage metadata")

			usage := usageMetadata.(*gollm.Usage)
			assert.Greater(t, usage.TotalTokens, 0, "Should have token usage")
			assert.Equal(t, "bedrock", usage.Provider, "Provider should be bedrock")

			// Verify we got actual content
			candidates := response.Candidates()
			require.Greater(t, len(candidates), 0, "Should have at least one candidate")
			content := ""
			parts := candidates[0].Parts()
			for _, part := range parts {
				if text, ok := part.AsText(); ok {
					content += text
				}
			}
			assert.NotEmpty(t, content, "Should have response content")

			t.Logf("Request %d: %q -> %q (tokens: %d)", i+1, prompt, content, usage.TotalTokens)
		})
	}

	// Verify total usage tracking
	usageMutex.Lock()
	totalRequests := len(allUsage)
	totalTokens := 0
	for _, usage := range allUsage {
		totalTokens += usage.TotalTokens
	}
	usageMutex.Unlock()

	assert.Equal(t, len(prompts), totalRequests, "Should have usage record for each request")
	assert.Greater(t, totalTokens, 0, "Should have accumulated token usage")

	t.Logf("✅ Usage tracking test completed - %d requests, %d total tokens", totalRequests, totalTokens)
}

// TestToolCallingWithRealAPI tests tool calling functionality with real AWS API calls
func TestToolCallingWithRealAPI(t *testing.T) {
	ctx := context.Background()

	client, err := gollm.NewClient(ctx, "bedrock",
		gollm.WithInferenceConfig(&gollm.InferenceConfig{
			Model:       ClaudeModel, // Claude supports tool calling
			Temperature: 0.1,
			MaxTokens:   1000,
		}),
	)
	require.NoError(t, err, "Should create client for tool calling test")
	defer client.Close()

	chat := client.StartChat("You are a helpful assistant with access to tools.", ClaudeModel)

	// Define a test function
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

	err = chat.SetFunctionDefinitions(functions)
	require.NoError(t, err, "Should set function definitions")

	// Test tool calling
	response, err := chat.Send(ctx, ToolPrompt)
	require.NoError(t, err, "Tool calling request should succeed")
	require.NotNil(t, response, "Response should not be nil")

	// Verify we got candidates
	candidates := response.Candidates()
	require.Greater(t, len(candidates), 0, "Should have at least one candidate")

	// Check if the model attempted to use tools
	// Note: The exact behavior depends on the model - it might use tools or just respond with text
	content := ""
	var functionCalls []gollm.FunctionCall
	parts := candidates[0].Parts()
	for _, part := range parts {
		if text, ok := part.AsText(); ok {
			content += text
		}
		if calls, ok := part.AsFunctionCalls(); ok {
			functionCalls = append(functionCalls, calls...)
		}
	}

	if len(functionCalls) > 0 {
		t.Logf("✅ Model used tool calling: %d function calls", len(functionCalls))
		for i, call := range functionCalls {
			t.Logf("Function call %d: %s with args %s", i+1, call.Name, call.Arguments)
		}

		// Test sending function results back
		results := make([]gollm.FunctionCallResult, len(functionCalls))
		for i, call := range functionCalls {
			results[i] = gollm.FunctionCallResult{
				ID:     call.ID,
				Name:   call.Name,
				Result: map[string]any{"temperature": "72°F", "condition": "sunny", "location": "San Francisco, CA"},
			}
		}

		// Send results using the regular Send method
		resultContents := make([]any, len(results))
		for i, result := range results {
			resultContents[i] = result
		}
		followupResponse, err := chat.Send(ctx, resultContents...)
		require.NoError(t, err, "Should handle function call results")
		require.NotNil(t, followupResponse, "Follow-up response should not be nil")

		followupCandidates := followupResponse.Candidates()
		if len(followupCandidates) > 0 {
			followupContent := ""
			followupParts := followupCandidates[0].Parts()
			for _, part := range followupParts {
				if text, ok := part.AsText(); ok {
					followupContent += text
				}
			}
			t.Logf("Follow-up response: %q", followupContent)
		}
	} else {
		t.Logf("ℹ️  Model responded with text instead of tool calling: %q", content)
		// This is still valid behavior - some models prefer to respond directly
	}

	// Verify usage tracking for tool calling
	usageMetadata := response.UsageMetadata()
	if usageMetadata != nil {
		usage := usageMetadata.(*gollm.Usage)
		assert.Greater(t, usage.TotalTokens, 0, "Tool calling should consume tokens")
		t.Logf("Tool calling usage: %d tokens", usage.TotalTokens)
	}

	t.Log("✅ Tool calling test completed")
}

// TestLLMAppsIntegrationPattern tests the exact patterns used by llm-apps
func TestLLMAppsIntegrationPattern(t *testing.T) {
	ctx := context.Background()

	// This test simulates the exact pattern described in integration-with-llm-apps.md

	// Step 1: Handler Level - Create client with configuration
	var conversationUsage []gollm.Usage
	var usageMutex sync.Mutex

	usageCallback := func(providerName string, model string, usage gollm.Usage) {
		usageMutex.Lock()
		defer usageMutex.Unlock()
		conversationUsage = append(conversationUsage, usage)

		// Simulate usage aggregation that llm-apps would do
		t.Logf("LLM-Apps usage capture: %s/%s - %d tokens", providerName, model, usage.TotalTokens)
	}

	inferenceConfig := &gollm.InferenceConfig{
		Model:       ClaudeModel,
		Temperature: 0.7,
		MaxTokens:   1500,
		TopP:        0.9,
	}

	client, err := gollm.NewClient(ctx, "bedrock",
		gollm.WithInferenceConfig(inferenceConfig),
		gollm.WithUsageCallback(usageCallback),
		gollm.WithDebug(true),
	)
	require.NoError(t, err, "LLM-Apps style client creation should succeed")
	defer client.Close()

	// Step 2: Agent Level - Process streaming responses and extract usage
	systemPrompt := "You are a helpful Kubernetes assistant. Provide concise, practical answers."
	chat := client.StartChat(systemPrompt, ClaudeModel)

	// Simulate a conversation like kubectl-ai would have
	kubernetesQueries := []string{
		"How do I list all pods in a namespace?",
		"What's the difference between a Deployment and a ReplicaSet?",
		"How do I check pod logs?",
	}

	totalConversationTokens := 0

	for i, query := range kubernetesQueries {
		t.Run(fmt.Sprintf("kubernetes_query_%d", i+1), func(t *testing.T) {
			// Test streaming response (as llm-apps would use)
			responseStream, err := chat.SendStreaming(ctx, query)
			require.NoError(t, err, "Streaming request should succeed")

			var fullResponse strings.Builder
			var finalUsage *gollm.Usage
			responseChunks := 0

			for response, streamErr := range responseStream {
				if streamErr != nil {
					require.NoError(t, streamErr, "Stream should not error")
					break
				}

				if response == nil {
					break // End of stream
				}

				responseChunks++

				// Extract content (as agent would do)
				candidates := response.Candidates()
				if len(candidates) > 0 {
					parts := candidates[0].Parts()
					for _, part := range parts {
						if text, ok := part.AsText(); ok {
							fullResponse.WriteString(text)
						}
					}
				}

				// Extract usage metadata (as agent would do)
				if response.UsageMetadata() != nil {
					if usage := response.UsageMetadata().(*gollm.Usage); usage != nil {
						finalUsage = usage
					}
				}
			}

			// Verify we got a proper response
			responseText := fullResponse.String()
			assert.NotEmpty(t, responseText, "Should get response content")
			assert.Greater(t, responseChunks, 0, "Should receive streaming chunks")

			// Verify usage data is complete
			require.NotNil(t, finalUsage, "Should have usage metadata")
			assert.Greater(t, finalUsage.TotalTokens, 0, "Should have token count")
			assert.Equal(t, "bedrock", finalUsage.Provider, "Provider should be bedrock")
			assert.Equal(t, ClaudeModel, finalUsage.Model, "Model should match")

			totalConversationTokens += finalUsage.TotalTokens

			t.Logf("Query %d result: %q (tokens: %d, chunks: %d)",
				i+1, responseText[:min(100, len(responseText))], finalUsage.TotalTokens, responseChunks)
		})
	}

	// Step 3: Application Level - Aggregate usage across conversation
	usageMutex.Lock()
	callbackTokens := 0
	for _, usage := range conversationUsage {
		callbackTokens += usage.TotalTokens
	}
	usageMutex.Unlock()

	// Verify usage tracking consistency
	assert.Equal(t, len(kubernetesQueries), len(conversationUsage), "Should have usage for each query")
	assert.Equal(t, totalConversationTokens, callbackTokens, "Callback and response usage should match")
	assert.Greater(t, totalConversationTokens, 0, "Should have accumulated tokens")

	// Simulate final usage report (as llm-apps would generate)
	t.Logf("✅ LLM-Apps integration test completed:")
	t.Logf("   - Queries processed: %d", len(kubernetesQueries))
	t.Logf("   - Total tokens used: %d", totalConversationTokens)
	t.Logf("   - Average tokens per query: %.1f", float64(totalConversationTokens)/float64(len(kubernetesQueries)))
	t.Logf("   - Provider: %s", inferenceConfig.Model)
}

// TestMultipleModelsWithRealCalls tests multiple models with actual API calls
func TestMultipleModelsWithRealCalls(t *testing.T) {
	ctx := context.Background()

	models := []string{
		ClaudeModel,
		NovaModel,
	}

	for _, model := range models {
		t.Run(fmt.Sprintf("model_%s", strings.ReplaceAll(model, ".", "_")), func(t *testing.T) {
			client, err := gollm.NewClient(ctx, "bedrock",
				gollm.WithInferenceConfig(&gollm.InferenceConfig{
					Model:       model,
					Temperature: 0.3,
					MaxTokens:   300,
				}),
			)
			require.NoError(t, err, "Should create client for model %s", model)
			defer client.Close()

			chat := client.StartChat("You are a helpful assistant.", model)

			// Test simple request
			response, err := chat.Send(ctx, "Please say 'Hello from "+model+"' and nothing else.")
			require.NoError(t, err, "Request should succeed for model %s", model)
			require.NotNil(t, response, "Response should not be nil")

			candidates := response.Candidates()
			require.Greater(t, len(candidates), 0, "Should have candidates")

			content := ""
			parts := candidates[0].Parts()
			for _, part := range parts {
				if text, ok := part.AsText(); ok {
					content += text
				}
			}
			assert.NotEmpty(t, content, "Should have response content")
			t.Logf("Model %s response: %q", model, content)

			// Verify usage metadata
			usageMetadata := response.UsageMetadata()
			require.NotNil(t, usageMetadata, "Should have usage metadata")
			usage := usageMetadata.(*gollm.Usage)
			assert.Equal(t, model, usage.Model, "Usage model should match")
			assert.Greater(t, usage.TotalTokens, 0, "Should have token usage")
		})
	}
}

// Helper function for min (Go 1.21+ has this built-in)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
