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

// AWS SSO Integration Tests - These tests require AWS SSO login and test different profiles/regions
// Run with: go test -tags=aws_integration -v ./... -run TestAWSSSOIntegration
//go:build aws_integration

package bedrock

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAWSSSOCredentials tests AWS SSO authentication specifically
func TestAWSSSOCredentials(t *testing.T) {
	ctx := context.Background()

	// Test 1: Check if SSO session is active
	t.Run("sso_session_active", func(t *testing.T) {
		awsConfig, err := config.LoadDefaultConfig(ctx)
		require.NoError(t, err, "Should load AWS config")

		// Use STS to verify the current identity
		stsClient := sts.NewFromConfig(awsConfig)
		identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		require.NoError(t, err, "Should be able to get caller identity - ensure 'aws sso login' has been run")

		// Log identity information
		t.Logf("✅ AWS SSO Authentication verified:")
		t.Logf("   Account ID: %s", *identity.Account)
		t.Logf("   User ARN: %s", *identity.Arn)
		t.Logf("   User ID: %s", *identity.UserId)

		// Verify this is an SSO session (SSO ARNs typically contain "assumed-role")
		if strings.Contains(*identity.Arn, "assumed-role") {
			t.Logf("   ✅ SSO session detected (assumed role)")
		} else {
			t.Logf("   ⚠️  Non-SSO credentials detected")
		}
	})

	// Test 2: Test different AWS profiles (if configured)
	t.Run("test_multiple_profiles", func(t *testing.T) {
		// Common SSO profile names to test
		profilesToTest := []string{
			"default",
			"dev",
			"prod",
			"test",
			"sso",
		}

		originalProfile := os.Getenv("AWS_PROFILE")
		defer func() {
			if originalProfile == "" {
				os.Unsetenv("AWS_PROFILE")
			} else {
				os.Setenv("AWS_PROFILE", originalProfile)
			}
		}()

		workingProfiles := []string{}

		for _, profile := range profilesToTest {
			t.Run(fmt.Sprintf("profile_%s", profile), func(t *testing.T) {
				os.Setenv("AWS_PROFILE", profile)

				// Try to load config with this profile
				awsConfig, err := config.LoadDefaultConfig(ctx)
				if err != nil {
					t.Logf("Profile '%s' not configured or accessible: %v", profile, err)
					return
				}

				// Try to get caller identity
				stsClient := sts.NewFromConfig(awsConfig)
				identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
				if err != nil {
					t.Logf("Profile '%s' authentication failed: %v", profile, err)
					return
				}

				workingProfiles = append(workingProfiles, profile)
				t.Logf("✅ Profile '%s' working - Account: %s, Region: %s",
					profile, *identity.Account, awsConfig.Region)

				// Test Bedrock access with this profile
				bedrockClient := bedrock.NewFromConfig(awsConfig)
				_, err = bedrockClient.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
				if err != nil {
					t.Logf("Profile '%s' has no Bedrock access: %v", profile, err)
				} else {
					t.Logf("✅ Profile '%s' has Bedrock access", profile)
				}
			})
		}

		t.Logf("Working profiles found: %v", workingProfiles)
		// Note: This test checks for alternative profiles but doesn't fail if none are found
		// since many users only have the current SSO profile configured
		if len(workingProfiles) == 0 {
			t.Log("⚠️ No alternative AWS profiles found - this is expected for SSO-only setups")
		} else {
			t.Logf("✅ Found %d working alternative profiles", len(workingProfiles))
		}
	})

	// Test 3: Test different regions
	t.Run("test_multiple_regions", func(t *testing.T) {
		// Common regions where Bedrock is available
		regionsToTest := []string{
			"us-east-1",
			"us-west-2",
			"eu-west-1",
			"ap-southeast-1",
		}

		originalRegion := os.Getenv("AWS_REGION")
		defer func() {
			if originalRegion == "" {
				os.Unsetenv("AWS_REGION")
			} else {
				os.Setenv("AWS_REGION", originalRegion)
			}
		}()

		workingRegions := []string{}

		for _, region := range regionsToTest {
			t.Run(fmt.Sprintf("region_%s", region), func(t *testing.T) {
				os.Setenv("AWS_REGION", region)

				awsConfig, err := config.LoadDefaultConfig(ctx)
				require.NoError(t, err, "Should load config for region %s", region)

				// Test Bedrock availability in this region
				bedrockClient := bedrock.NewFromConfig(awsConfig)
				models, err := bedrockClient.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
				if err != nil {
					t.Logf("Region '%s' Bedrock not accessible: %v", region, err)
					return
				}

				modelCount := len(models.ModelSummaries)
				workingRegions = append(workingRegions, region)
				t.Logf("✅ Region '%s' has %d Bedrock models available", region, modelCount)

				// Test creating a client in this region
				client, err := gollm.NewClient(ctx, "bedrock",
					gollm.WithInferenceConfig(&gollm.InferenceConfig{
						Region: region,
						Model:  ClaudeModel,
					}),
				)
				if err != nil {
					t.Logf("Failed to create gollm client in region '%s': %v", region, err)
				} else {
					client.Close()
					t.Logf("✅ gollm client created successfully in region '%s'", region)
				}
			})
		}

		t.Logf("Working regions found: %v", workingRegions)
		assert.Greater(t, len(workingRegions), 0, "At least one region should have Bedrock access")
	})
}

// TestK8sBenchCommandLinePattern tests the exact k8s-bench command line pattern
func TestK8sBenchCommandLinePattern(t *testing.T) {
	ctx := context.Background()

	// This test simulates the exact command line pattern:
	// ./k8s-bench run --agent-bin ./kubectl-ai --llm-provider bedrock --models "us.anthropic.claude-sonnet-4-20250514-v1:0,us.amazon.nova-pro-v1:0"

	modelList := "us.anthropic.claude-sonnet-4-20250514-v1:0,us.amazon.nova-pro-v1:0"
	models := strings.Split(modelList, ",")

	t.Run("k8s_bench_exact_pattern", func(t *testing.T) {
		for _, model := range models {
			t.Run(fmt.Sprintf("model_%s", strings.ReplaceAll(model, ":", "_")), func(t *testing.T) {
				// Step 1: Create client exactly as k8s-bench would
				client, err := gollm.NewClient(ctx, "bedrock")
				require.NoError(t, err, "k8s-bench style client creation should work for %s", model)
				defer client.Close()

				// Step 2: Start chat session exactly as k8s-bench would
				systemPrompt := "You are a Kubernetes expert assistant. Provide helpful, accurate information about Kubernetes operations and troubleshooting."
				chat := client.StartChat(systemPrompt, model)
				require.NotNil(t, chat, "Should start chat for model %s", model)

				// Step 3: Send a typical k8s-bench task prompt
				taskPrompt := `I have a pod that's in CrashLoopBackOff state. The pod name is "webapp-deployment-abc123-xyz789" in the "production" namespace. Please provide kubectl commands to:
1. Check the pod status and events
2. Examine the pod logs
3. Describe the pod to see configuration issues
4. Check if there are resource constraints

Provide the exact kubectl commands I should run.`

				// Test streaming response (k8s-bench uses streaming)
				responseStream, err := chat.SendStreaming(ctx, taskPrompt)
				require.NoError(t, err, "k8s-bench streaming should work for %s", model)

				var fullResponse strings.Builder
				responseChunks := 0
				totalTokens := 0

				// Collect streaming response as k8s-bench would
				for response, streamErr := range responseStream {
					if streamErr != nil {
						require.NoError(t, streamErr, "Stream should not error for %s", model)
						break
					}

					if response == nil {
						break // End of stream
					}

					responseChunks++

					// Extract content
					candidates := response.Candidates()
					if len(candidates) > 0 {
						parts := candidates[0].Parts()
						for _, part := range parts {
							if text, ok := part.AsText(); ok {
								fullResponse.WriteString(text)
							}
						}
					}

					// Extract usage (k8s-bench tracks this)
					if response.UsageMetadata() != nil {
						if usage := response.UsageMetadata().(*gollm.Usage); usage != nil {
							totalTokens = usage.TotalTokens
						}
					}
				}

				// Verify response quality (k8s-bench would evaluate this)
				responseText := fullResponse.String()
				assert.NotEmpty(t, responseText, "Should get response for %s", model)
				assert.Greater(t, responseChunks, 0, "Should receive streaming chunks for %s", model)
				assert.Greater(t, totalTokens, 0, "Should track token usage for %s", model)

				// Verify response contains kubectl commands (typical k8s-bench evaluation)
				assert.Contains(t, responseText, "kubectl", "Response should contain kubectl commands for %s", model)
				assert.True(t,
					strings.Contains(responseText, "logs") || strings.Contains(responseText, "describe") || strings.Contains(responseText, "get"),
					"Response should contain relevant kubectl operations for %s", model)

				t.Logf("✅ k8s-bench pattern test for %s:", model)
				t.Logf("   - Response chunks: %d", responseChunks)
				t.Logf("   - Total tokens: %d", totalTokens)
				t.Logf("   - Response length: %d chars", len(responseText))
				t.Logf("   - Contains kubectl: %t", strings.Contains(responseText, "kubectl"))
			})
		}
	})
}

// TestBedrockModelAvailability tests which Bedrock models are actually available in the current account/region
func TestBedrockModelAvailability(t *testing.T) {
	ctx := context.Background()

	awsConfig, err := config.LoadDefaultConfig(ctx)
	require.NoError(t, err, "Should load AWS config")

	bedrockClient := bedrock.NewFromConfig(awsConfig)

	// Get all available models
	t.Run("list_available_models", func(t *testing.T) {
		models, err := bedrockClient.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
		require.NoError(t, err, "Should list foundation models")

		t.Logf("Available Bedrock models in region %s:", awsConfig.Region)
		t.Logf("Total models: %d", len(models.ModelSummaries))

		anthropicModels := []string{}
		amazonModels := []string{}
		otherModels := []string{}

		for _, model := range models.ModelSummaries {
			modelId := *model.ModelId
			if strings.Contains(modelId, "anthropic") {
				anthropicModels = append(anthropicModels, modelId)
			} else if strings.Contains(modelId, "amazon") {
				amazonModels = append(amazonModels, modelId)
			} else {
				otherModels = append(otherModels, modelId)
			}
		}

		t.Logf("Anthropic models (%d): %v", len(anthropicModels), anthropicModels)
		t.Logf("Amazon models (%d): %v", len(amazonModels), amazonModels)
		t.Logf("Other models (%d): %v", len(otherModels), otherModels)

		// Test if our test models are available
		testModels := []string{ClaudeModel, NovaModel}
		for _, testModel := range testModels {
			available := false
			for _, model := range models.ModelSummaries {
				if *model.ModelId == testModel {
					available = true
					break
				}
			}
			t.Logf("Test model %s available: %t", testModel, available)
			if !available {
				t.Logf("⚠️  Test model %s not available in current account/region", testModel)
			}
		}
	})

	// Test access to specific model families
	t.Run("test_model_families", func(t *testing.T) {
		modelFamilies := map[string][]string{
			"claude": {
				"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
				"us.anthropic.claude-sonnet-4-20250514-v1:0",
				"anthropic.claude-3-sonnet-20240229-v1:0",
			},
			"nova": {
				"us.amazon.nova-lite-v1:0",
				"us.amazon.nova-pro-v1:0",
				"us.amazon.nova-micro-v1:0",
			},
		}

		for family, models := range modelFamilies {
			t.Run(fmt.Sprintf("family_%s", family), func(t *testing.T) {
				workingModels := []string{}

				for _, model := range models {
					// Test if we can create a client with this model
					client, err := gollm.NewClient(ctx, "bedrock",
						gollm.WithInferenceConfig(&gollm.InferenceConfig{
							Model:       model,
							Temperature: 0.1,
							MaxTokens:   100,
						}),
					)
					if err != nil {
						t.Logf("Model %s not accessible: %v", model, err)
						continue
					}

					// Try a simple request
					chat := client.StartChat("Test", model)
					response, err := chat.Send(ctx, "Say 'test' and nothing else.")
					client.Close()

					if err != nil {
						t.Logf("Model %s request failed: %v", model, err)
					} else {
						workingModels = append(workingModels, model)
						if response.UsageMetadata() != nil {
							usage := response.UsageMetadata().(*gollm.Usage)
							t.Logf("✅ Model %s working - %d tokens", model, usage.TotalTokens)
						} else {
							t.Logf("✅ Model %s working - no usage data", model)
						}
					}
				}

				t.Logf("Working models in %s family: %v", family, workingModels)
			})
		}
	})
}

// TestStreamingPerformance tests streaming performance with real API calls
func TestStreamingPerformance(t *testing.T) {
	ctx := context.Background()

	client, err := gollm.NewClient(ctx, "bedrock",
		gollm.WithInferenceConfig(&gollm.InferenceConfig{
			Model:       ClaudeModel,
			Temperature: 0.3,
			MaxTokens:   1000,
		}),
	)
	require.NoError(t, err, "Should create client for performance test")
	defer client.Close()

	chat := client.StartChat("You are a helpful assistant.", ClaudeModel)

	t.Run("streaming_latency_test", func(t *testing.T) {
		prompt := "Write a detailed explanation of Kubernetes pods, including their lifecycle, networking, and best practices. Be comprehensive."

		startTime := time.Now()
		responseStream, err := chat.SendStreaming(ctx, prompt)
		require.NoError(t, err, "Should start streaming")

		firstChunkTime := time.Time{}
		lastChunkTime := time.Time{}
		chunkTimes := []time.Duration{}
		responseChunks := 0
		totalTokens := 0

		for response, streamErr := range responseStream {
			chunkTime := time.Now()

			if streamErr != nil {
				require.NoError(t, streamErr, "Stream should not error")
				break
			}

			if response == nil {
				break
			}

			responseChunks++

			if firstChunkTime.IsZero() {
				firstChunkTime = chunkTime
			}
			lastChunkTime = chunkTime

			if responseChunks > 1 {
				chunkTimes = append(chunkTimes, chunkTime.Sub(lastChunkTime))
			}

			if response.UsageMetadata() != nil {
				if usage := response.UsageMetadata().(*gollm.Usage); usage != nil {
					totalTokens = usage.TotalTokens
				}
			}
		}

		endTime := time.Now()

		// Calculate performance metrics
		timeToFirstChunk := firstChunkTime.Sub(startTime)
		totalStreamTime := endTime.Sub(startTime)
		streamingTime := lastChunkTime.Sub(firstChunkTime)

		// Calculate average inter-chunk time
		avgInterChunkTime := time.Duration(0)
		if len(chunkTimes) > 0 {
			total := time.Duration(0)
			for _, d := range chunkTimes {
				total += d
			}
			avgInterChunkTime = total / time.Duration(len(chunkTimes))
		}

		// Performance assertions
		assert.Less(t, timeToFirstChunk, 10*time.Second, "Time to first chunk should be reasonable")
		assert.Greater(t, responseChunks, 1, "Should receive multiple chunks for streaming")
		assert.Greater(t, totalTokens, 0, "Should generate tokens")

		// Log performance metrics
		t.Logf("✅ Streaming performance metrics:")
		t.Logf("   - Time to first chunk: %v", timeToFirstChunk)
		t.Logf("   - Total stream time: %v", totalStreamTime)
		t.Logf("   - Streaming duration: %v", streamingTime)
		t.Logf("   - Response chunks: %d", responseChunks)
		t.Logf("   - Total tokens: %d", totalTokens)
		t.Logf("   - Avg inter-chunk time: %v", avgInterChunkTime)
		if totalTokens > 0 && streamingTime > 0 {
			tokensPerSecond := float64(totalTokens) / streamingTime.Seconds()
			t.Logf("   - Tokens per second: %.2f", tokensPerSecond)
		}
	})
}
