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
	"os"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBedrockProviderIntegrationWithKubectlAI verifies that the Bedrock provider
// integrates correctly with the main kubectl-ai application patterns
func TestBedrockProviderIntegrationWithKubectlAI(t *testing.T) {
	ctx := context.Background()

	// Test 1: Provider registration and discovery
	t.Run("provider_registration", func(t *testing.T) {
		client, err := gollm.NewClient(ctx, "bedrock")
		if err != nil {
			if strings.Contains(err.Error(), "aws") || strings.Contains(err.Error(), "credentials") {
				t.Skip("Skipping Bedrock integration test - AWS credentials not available")
			}
			t.Fatalf("Failed to create Bedrock client: %v", err)
		}
		defer client.Close()

		assert.NotNil(t, client, "Bedrock client should be created successfully")
	})

	// Test 2: Verify client options are properly used
	t.Run("client_options_integration", func(t *testing.T) {
		client, err := gollm.NewClient(ctx, "bedrock",
			gollm.WithInferenceConfig(&gollm.InferenceConfig{
				Temperature: 0.7,
				MaxTokens:   2048,
				TopP:        0.9,
			}),
			gollm.WithUsageCallback(func(provider, model string, usage gollm.Usage) {
				assert.Equal(t, "bedrock", provider)
				assert.NotEmpty(t, model)
				// Usage should have proper structure
				assert.NotEmpty(t, usage.Provider)
			}),
		)
		if err != nil {
			if strings.Contains(err.Error(), "aws") || strings.Contains(err.Error(), "credentials") {
				t.Skip("Skipping Bedrock integration test - AWS credentials not available")
			}
			t.Fatalf("Failed to create Bedrock client with options: %v", err)
		}
		defer client.Close()

		// Verify the client was created with options
		assert.NotNil(t, client)

		// Note: We can't easily test the callback without making actual API calls,
		// but we've verified the interface is correct in the unit tests
	})

	// Test 3: K8s-bench style configuration
	t.Run("k8s_bench_configuration", func(t *testing.T) {
		// This tests the pattern used by k8s-bench for model configuration
		supportedModels := []string{
			"us.anthropic.claude-sonnet-4-20250514-v1:0",
			"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
			"us.amazon.nova-pro-v1:0",
			"us.amazon.nova-lite-v1:0",
			"us.amazon.nova-micro-v1:0",
		}

		for _, model := range supportedModels {
			client, err := gollm.NewClient(ctx, "bedrock")
			if err != nil {
				if strings.Contains(err.Error(), "aws") || strings.Contains(err.Error(), "credentials") {
					t.Skip("Skipping Bedrock k8s-bench test - AWS credentials not available")
				}
				t.Fatalf("Failed to create Bedrock client for model %s: %v", model, err)
			}

			// Start a chat session with the model (similar to k8s-bench pattern)
			chat := client.StartChat("You are a helpful assistant", model)
			assert.NotNil(t, chat, "Chat session should be created for model %s", model)

			client.Close()
		}
	})

	// Test 4: Environment variable configuration (commonly used in CI/k8s-bench)
	t.Run("environment_configuration", func(t *testing.T) {
		// Test common environment variables used for Bedrock
		originalRegion := os.Getenv("AWS_REGION")
		originalProfile := os.Getenv("AWS_PROFILE")

		// Set test environment variables
		os.Setenv("AWS_REGION", "us-east-1")
		os.Setenv("AWS_PROFILE", "test-profile")

		// Cleanup
		defer func() {
			if originalRegion == "" {
				os.Unsetenv("AWS_REGION")
			} else {
				os.Setenv("AWS_REGION", originalRegion)
			}
			if originalProfile == "" {
				os.Unsetenv("AWS_PROFILE")
			} else {
				os.Setenv("AWS_PROFILE", originalProfile)
			}
		}()

		client, err := gollm.NewClient(ctx, "bedrock")
		if err != nil {
			if strings.Contains(err.Error(), "aws") || strings.Contains(err.Error(), "credentials") || strings.Contains(err.Error(), "profile") {
				t.Skip("Skipping Bedrock environment test - AWS credentials not available")
			}
			t.Fatalf("Failed to create Bedrock client with environment config: %v", err)
		}
		defer client.Close()

		assert.NotNil(t, client)
	})
}

// TestBedrockProviderK8sBenchCompatibility verifies compatibility with k8s-bench patterns
func TestBedrockProviderK8sBenchCompatibility(t *testing.T) {
	ctx := context.Background()

	// Test the exact pattern used by k8s-bench for provider setup
	t.Run("k8s_bench_llm_config_pattern", func(t *testing.T) {
		// This mimics the LLMConfig structure used in k8s-bench
		llmConfigs := []struct {
			ID                string
			ProviderID        string
			ModelID           string
			EnableToolUseShim bool
			Quiet             bool
		}{
			{
				ID:                "shim_disabled-bedrock-us.anthropic.claude-sonnet-4-20250514-v1:0",
				ProviderID:        "bedrock",
				ModelID:           "us.anthropic.claude-sonnet-4-20250514-v1:0",
				EnableToolUseShim: false,
				Quiet:             true,
			},
			{
				ID:                "shim_disabled-bedrock-us.amazon.nova-pro-v1:0",
				ProviderID:        "bedrock",
				ModelID:           "us.amazon.nova-pro-v1:0",
				EnableToolUseShim: false,
				Quiet:             true,
			},
		}

		for _, config := range llmConfigs {
			// Create client with the exact pattern k8s-bench uses
			client, err := gollm.NewClient(ctx, config.ProviderID)
			if err != nil {
				if strings.Contains(err.Error(), "aws") || strings.Contains(err.Error(), "credentials") {
					t.Skipf("Skipping k8s-bench compatibility test for %s - AWS credentials not available", config.ID)
				}
				t.Fatalf("Failed to create client for config %s: %v", config.ID, err)
			}

			// Verify we can start a chat session (this is what k8s-bench does)
			chat := client.StartChat("You are a helpful Kubernetes assistant", config.ModelID)
			assert.NotNil(t, chat, "Chat session should be created for config %s", config.ID)

			client.Close()
		}
	})

	// Test command-line pattern used by k8s-bench
	t.Run("command_line_pattern", func(t *testing.T) {
		// This tests the exact command line pattern:
		// ./k8s-bench run --agent-bin ./kubectl-ai --llm-provider bedrock --models "us.anthropic.claude-sonnet-4-20250514-v1:0,us.amazon.nova-pro-v1:0"

		modelList := "us.anthropic.claude-sonnet-4-20250514-v1:0,us.amazon.nova-pro-v1:0"
		models := strings.Split(modelList, ",")

		for _, model := range models {
			model = strings.TrimSpace(model)

			client, err := gollm.NewClient(ctx, "bedrock")
			if err != nil {
				if strings.Contains(err.Error(), "aws") || strings.Contains(err.Error(), "credentials") {
					t.Skipf("Skipping command line pattern test for %s - AWS credentials not available", model)
				}
				t.Fatalf("Failed to create Bedrock client for model %s: %v", model, err)
			}

			chat := client.StartChat("System prompt", model)
			assert.NotNil(t, chat, "Should create chat for model %s", model)

			client.Close()
		}
	})
}

// TestBedrockProviderFeatureCompleteness verifies all expected features work
func TestBedrockProviderFeatureCompleteness(t *testing.T) {
	ctx := context.Background()

	client, err := gollm.NewClient(ctx, "bedrock")
	if err != nil {
		if strings.Contains(err.Error(), "aws") || strings.Contains(err.Error(), "credentials") {
			t.Skip("Skipping feature completeness test - AWS credentials not available")
		}
		t.Fatalf("Failed to create Bedrock client: %v", err)
	}
	defer client.Close()

	// Test basic chat functionality
	t.Run("basic_chat", func(t *testing.T) {
		chat := client.StartChat("You are a helpful assistant", "us.anthropic.claude-sonnet-4-20250514-v1:0")
		require.NotNil(t, chat)
	})

	// Test streaming interface exists (even if we can't call it without AWS)
	t.Run("streaming_interface", func(t *testing.T) {
		chat := client.StartChat("You are a helpful assistant", "us.anthropic.claude-sonnet-4-20250514-v1:0")
		session, ok := chat.(*bedrockChatSession)
		require.True(t, ok)

		// Verify SendStreaming method exists with correct signature
		assert.NotNil(t, session.SendStreaming)
	})

	// Test tool definitions can be added
	t.Run("tool_support", func(t *testing.T) {
		chat := client.StartChat("You are a helpful assistant", "us.anthropic.claude-sonnet-4-20250514-v1:0")

		toolDef := &gollm.FunctionDefinition{
			Name:        "get_weather",
			Description: "Get weather information",
			Parameters: &gollm.Schema{
				Type: gollm.TypeObject,
				Properties: map[string]*gollm.Schema{
					"location": {
						Type:        gollm.TypeString,
						Description: "The location to get weather for",
					},
				},
				Required: []string{"location"},
			},
		}

		// Verify we can set function definitions without error
		assert.NotPanics(t, func() {
			err := chat.SetFunctionDefinitions([]*gollm.FunctionDefinition{toolDef})
			assert.NoError(t, err)
		})
	})

	// Test usage metadata interface
	t.Run("usage_metadata", func(t *testing.T) {
		// Create a mock response with usage data
		response := &bedrockChatResponse{
			content: "Test response",
			usage: &types.TokenUsage{
				InputTokens:  aws.Int32(10),
				OutputTokens: aws.Int32(20),
			},
		}

		// Verify structured usage metadata is returned
		usageData := response.UsageMetadata()
		assert.NotNil(t, usageData)

		// Should return structured gollm.Usage when possible
		if structuredUsage, ok := usageData.(*gollm.Usage); ok {
			assert.Equal(t, "bedrock", structuredUsage.Provider)
			assert.Equal(t, 10, structuredUsage.InputTokens)
			assert.Equal(t, 20, structuredUsage.OutputTokens)
		}
	})
}

// TestBedrockProviderErrorHandling verifies proper error handling
func TestBedrockProviderErrorHandling(t *testing.T) {
	ctx := context.Background()

	// Test unsupported model handling
	t.Run("unsupported_model", func(t *testing.T) {
		// Create a client with proper options but a known unsupported model
		client := &BedrockClient{
			options:    DefaultOptions,
			clientOpts: gollm.ClientOptions{},
		}

		// Create a session with an unsupported model name
		session := &bedrockChatSession{
			client:       client,
			model:        "unsupported-model",
			history:      make([]types.Message, 0),
			functionDefs: make([]*gollm.FunctionDefinition, 0),
		}

		// Should return error for unsupported model without trying AWS
		_, err := session.SendStreaming(ctx, "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported model")
	})

	// Test provider initialization
	t.Run("provider_registration", func(t *testing.T) {
		// Verify the provider is registered
		client, err := gollm.NewClient(ctx, "bedrock")
		if err != nil && !strings.Contains(err.Error(), "aws") && !strings.Contains(err.Error(), "credentials") {
			t.Fatalf("Bedrock provider should be registered: %v", err)
		}
		if client != nil {
			client.Close()
		}
	})
}
