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

// AWS Edge Cases Tests - These tests focus on error conditions and edge cases with real AWS
// Run with: go test -tags=aws_integration -v ./... -run TestLargeRequestsAndLimits
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLargeRequestsAndLimits tests handling of large requests and AWS limits
func TestLargeRequestsAndLimits(t *testing.T) {
	ctx := context.Background()

	client, err := gollm.NewClient(ctx, "bedrock",
		gollm.WithInferenceConfig(&gollm.InferenceConfig{
			Model:       ClaudeModel,
			Temperature: 0.3,
			MaxTokens:   4000, // Large but reasonable
		}),
	)
	require.NoError(t, err, "Should create client for large request test")
	defer client.Close()

	chat := client.StartChat("You are a helpful assistant.", ClaudeModel)

	t.Run("large_prompt_test", func(t *testing.T) {
		// Create a large but reasonable prompt
		largePrompt := "Please analyze the following Kubernetes YAML configuration and provide detailed feedback:\n\n"
		for i := 0; i < 10; i++ {
			largePrompt += fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: example-app-%c
  namespace: production
spec:
  replicas: 3
  selector:
    matchLabels:
      app: example-app-%c
  template:
    metadata:
      labels:
        app: example-app-%c
    spec:
      containers:
      - name: app
        image: nginx:1.21
        ports:
        - containerPort: 80
        resources:
          requests:
            memory: "64Mi"
            cpu: "250m"
          limits:
            memory: "128Mi"
            cpu: "500m"
---
`, 'a'+i, 'a'+i, 'a'+i)
		}
		largePrompt += "\nPlease provide analysis and recommendations for these configurations."

		response, err := chat.Send(ctx, largePrompt)
		require.NoError(t, err, "Large prompt should be handled successfully")
		require.NotNil(t, response, "Should get response for large prompt")

		candidates := response.Candidates()
		require.Greater(t, len(candidates), 0, "Should have response candidates")
		content := ""
		parts := candidates[0].Parts()
		for _, part := range parts {
			if text, ok := part.AsText(); ok {
				content += text
			}
		}
		assert.NotEmpty(t, content, "Should get meaningful response content")

		// Verify usage tracking works with large requests
		usageMetadata := response.UsageMetadata()
		require.NotNil(t, usageMetadata, "Should have usage metadata for large request")
		usage := usageMetadata.(*gollm.Usage)
		assert.Greater(t, usage.InputTokens, 1000, "Large prompt should consume significant input tokens")

		t.Logf("✅ Large prompt test completed - Input tokens: %d, Output tokens: %d",
			usage.InputTokens, usage.OutputTokens)
	})

	t.Run("maximum_token_limit_test", func(t *testing.T) {
		// Test with maximum token limit
		maxTokenClient, err := gollm.NewClient(ctx, "bedrock",
			gollm.WithInferenceConfig(&gollm.InferenceConfig{
				Model:       ClaudeModel,
				Temperature: 0.3,
				MaxTokens:   8000, // Close to model maximum
			}),
		)
		require.NoError(t, err, "Should create client with max tokens")
		defer maxTokenClient.Close()

		maxChat := maxTokenClient.StartChat("You are a helpful assistant.", ClaudeModel)

		prompt := "Write a comprehensive guide to Kubernetes troubleshooting that covers all major issues, debugging techniques, and best practices. Be extremely detailed and thorough."

		response, err := maxChat.Send(ctx, prompt)
		// This might fail if we hit model limits, which is expected behavior
		if err != nil {
			t.Logf("Expected behavior: Large token request failed: %v", err)
			assert.Contains(t, err.Error(), "token", "Error should relate to token limits")
		} else {
			require.NotNil(t, response, "If successful, should have response")
			usageMetadata := response.UsageMetadata()
			if usageMetadata != nil {
				usage := usageMetadata.(*gollm.Usage)
				t.Logf("Max token test completed - Total tokens: %d", usage.TotalTokens)
			}
		}
	})
}

// TestConcurrentRequests tests handling multiple concurrent requests
func TestConcurrentRequests(t *testing.T) {
	ctx := context.Background()

	// Track usage across all concurrent requests
	var allUsage []gollm.Usage
	var usageMutex sync.Mutex

	client, err := gollm.NewClient(ctx, "bedrock",
		gollm.WithInferenceConfig(&gollm.InferenceConfig{
			Model:       NovaModel,
			Temperature: 0.1,
			MaxTokens:   500,
		}),
		gollm.WithUsageCallback(func(provider, model string, usage gollm.Usage) {
			usageMutex.Lock()
			defer usageMutex.Unlock()
			allUsage = append(allUsage, usage)
		}),
	)
	require.NoError(t, err, "Should create client for concurrent test")
	defer client.Close()

	t.Run("concurrent_chat_sessions", func(t *testing.T) {
		numGoroutines := 5
		requestsPerGoroutine := 3

		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines*requestsPerGoroutine)
		results := make(chan string, numGoroutines*requestsPerGoroutine)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				// Each goroutine creates its own chat session
				chat := client.StartChat("You are a helpful assistant.", NovaModel)

				for j := 0; j < requestsPerGoroutine; j++ {
					prompt := fmt.Sprintf("Worker %d, request %d: What is %d + %d?", workerID, j, workerID, j)

					response, err := chat.Send(ctx, prompt)
					if err != nil {
						errors <- err
						return
					}

					candidates := response.Candidates()
					if len(candidates) > 0 {
						content := ""
						parts := candidates[0].Parts()
						for _, part := range parts {
							if text, ok := part.AsText(); ok {
								content += text
							}
						}
						results <- fmt.Sprintf("Worker %d: %s", workerID, content)
					}
				}
			}(i)
		}

		// Wait for all goroutines to complete
		wg.Wait()
		close(errors)
		close(results)

		// Check for any errors
		errorCount := 0
		for err := range errors {
			errorCount++
			t.Logf("Concurrent request error: %v", err)
		}

		// Collect results
		resultCount := 0
		for result := range results {
			resultCount++
			t.Logf("Concurrent result: %s", result)
		}

		// Verify most requests succeeded
		expectedRequests := numGoroutines * requestsPerGoroutine
		assert.Less(t, errorCount, expectedRequests/2, "Most concurrent requests should succeed")
		assert.Greater(t, resultCount, expectedRequests/2, "Should get results from most requests")

		// Verify usage tracking worked across concurrent requests
		usageMutex.Lock()
		usageCount := len(allUsage)
		usageMutex.Unlock()

		assert.Greater(t, usageCount, 0, "Should capture usage from concurrent requests")
		t.Logf("✅ Concurrent test completed - %d errors, %d results, %d usage records",
			errorCount, resultCount, usageCount)
	})
}

// TestStreamingEdgeCases tests streaming edge cases and error conditions
func TestStreamingEdgeCases(t *testing.T) {
	ctx := context.Background()

	client, err := gollm.NewClient(ctx, "bedrock",
		gollm.WithInferenceConfig(&gollm.InferenceConfig{
			Model:       ClaudeModel,
			Temperature: 0.3,
			MaxTokens:   1000,
		}),
	)
	require.NoError(t, err, "Should create client for streaming edge cases")
	defer client.Close()

	chat := client.StartChat("You are a helpful assistant.", ClaudeModel)

	t.Run("cancelled_streaming_context", func(t *testing.T) {
		// Create a context that will be cancelled during streaming
		streamCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		prompt := "Write a very long detailed explanation about Kubernetes networking, including CNI plugins, service discovery, ingress controllers, network policies, and troubleshooting. Be extremely comprehensive and detailed."

		responseStream, err := chat.SendStreaming(streamCtx, prompt)
		require.NoError(t, err, "Should start streaming")

		chunkCount := 0
		var lastError error

		for response, streamErr := range responseStream {
			if streamErr != nil {
				lastError = streamErr
				break
			}

			if response == nil {
				break
			}

			chunkCount++
			// Cancel after receiving some chunks
			if chunkCount >= 3 {
				cancel()
			}
		}

		// Should have received some chunks before cancellation
		assert.Greater(t, chunkCount, 0, "Should receive some chunks before cancellation")

		// Should eventually get a cancellation error or complete normally
		if lastError != nil {
			t.Logf("Expected cancellation behavior: %v", lastError)
		} else {
			t.Logf("Stream completed normally despite cancellation")
		}

		t.Logf("✅ Cancelled streaming test completed - %d chunks before cancellation", chunkCount)
	})

	t.Run("rapid_streaming_requests", func(t *testing.T) {
		// Test multiple rapid streaming requests
		numRequests := 3
		var wg sync.WaitGroup

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(requestID int) {
				defer wg.Done()

				prompt := fmt.Sprintf("Request %d: Count from 1 to 10 and explain each number.", requestID)
				responseStream, err := chat.SendStreaming(ctx, prompt)
				if err != nil {
					t.Logf("Rapid streaming request %d failed: %v", requestID, err)
					return
				}

				chunkCount := 0
				for response, streamErr := range responseStream {
					if streamErr != nil {
						t.Logf("Rapid streaming request %d error: %v", requestID, streamErr)
						break
					}

					if response == nil {
						break
					}

					chunkCount++
				}

				t.Logf("Rapid streaming request %d completed with %d chunks", requestID, chunkCount)
			}(i)
		}

		wg.Wait()
		t.Log("✅ Rapid streaming requests test completed")
	})
}

// TestUsageCallbackEdgeCases tests edge cases in usage callback functionality
func TestUsageCallbackEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("panic_in_usage_callback", func(t *testing.T) {
		// Test that panics in usage callback don't crash the client
		panickingCallback := func(provider, model string, usage gollm.Usage) {
			if usage.TotalTokens > 0 {
				panic("Test panic in usage callback")
			}
		}

		client, err := gollm.NewClient(ctx, "bedrock",
			gollm.WithInferenceConfig(&gollm.InferenceConfig{
				Model:       NovaModel,
				Temperature: 0.1,
				MaxTokens:   100,
			}),
			gollm.WithUsageCallback(panickingCallback),
		)
		require.NoError(t, err, "Should create client with panicking callback")
		defer client.Close()

		chat := client.StartChat("You are a helpful assistant.", NovaModel)

		// This should work despite the panicking callback
		response, err := chat.Send(ctx, "Say hello.")

		// The request itself should succeed
		require.NoError(t, err, "Request should succeed despite callback panic")
		require.NotNil(t, response, "Should get response despite callback panic")

		candidates := response.Candidates()
		require.Greater(t, len(candidates), 0, "Should have response content")
		content := ""
		parts := candidates[0].Parts()
		for _, part := range parts {
			if text, ok := part.AsText(); ok {
				content += text
			}
		}
		assert.NotEmpty(t, content, "Should get response content")

		t.Log("✅ Panicking callback test completed - client remained functional")
	})

	t.Run("slow_usage_callback", func(t *testing.T) {
		// Test that slow callbacks don't significantly impact performance
		slowCallback := func(provider, model string, usage gollm.Usage) {
			// Simulate slow processing
			time.Sleep(100 * time.Millisecond)
		}

		client, err := gollm.NewClient(ctx, "bedrock",
			gollm.WithInferenceConfig(&gollm.InferenceConfig{
				Model:       NovaModel,
				Temperature: 0.1,
				MaxTokens:   100,
			}),
			gollm.WithUsageCallback(slowCallback),
		)
		require.NoError(t, err, "Should create client with slow callback")
		defer client.Close()

		chat := client.StartChat("You are a helpful assistant.", NovaModel)

		startTime := time.Now()
		response, err := chat.Send(ctx, "Say hello.")
		duration := time.Since(startTime)

		require.NoError(t, err, "Request should succeed with slow callback")
		require.NotNil(t, response, "Should get response")

		// The request shouldn't be significantly slowed by the callback
		// (callback should run asynchronously or be handled gracefully)
		assert.Less(t, duration, 10*time.Second, "Request should complete in reasonable time")

		t.Logf("✅ Slow callback test completed in %v", duration)
	})

	t.Run("concurrent_usage_callbacks", func(t *testing.T) {
		// Test usage callback thread safety
		var callbackCount int64
		var mutex sync.Mutex

		threadSafeCallback := func(provider, model string, usage gollm.Usage) {
			mutex.Lock()
			defer mutex.Unlock()
			callbackCount++
		}

		client, err := gollm.NewClient(ctx, "bedrock",
			gollm.WithInferenceConfig(&gollm.InferenceConfig{
				Model:       NovaModel,
				Temperature: 0.1,
				MaxTokens:   100,
			}),
			gollm.WithUsageCallback(threadSafeCallback),
		)
		require.NoError(t, err, "Should create client with thread-safe callback")
		defer client.Close()

		// Make multiple concurrent requests
		numRequests := 5
		var wg sync.WaitGroup

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(requestID int) {
				defer wg.Done()

				chat := client.StartChat("You are a helpful assistant.", NovaModel)
				prompt := fmt.Sprintf("Request %d: Say hello.", requestID)

				_, err := chat.Send(ctx, prompt)
				if err != nil {
					t.Logf("Concurrent request %d failed: %v", requestID, err)
				}
			}(i)
		}

		wg.Wait()

		mutex.Lock()
		finalCallbackCount := callbackCount
		mutex.Unlock()

		assert.Greater(t, finalCallbackCount, int64(0), "Should have received usage callbacks")
		t.Logf("✅ Concurrent callback test completed - %d callbacks", finalCallbackCount)
	})
}

// TestErrorRecoveryAndResilience tests error recovery and resilience
func TestErrorRecoveryAndResilience(t *testing.T) {
	ctx := context.Background()

	client, err := gollm.NewClient(ctx, "bedrock",
		gollm.WithInferenceConfig(&gollm.InferenceConfig{
			Model:       ClaudeModel,
			Temperature: 0.3,
			MaxTokens:   1000,
		}),
	)
	require.NoError(t, err, "Should create client for resilience test")
	defer client.Close()

	t.Run("invalid_function_call_recovery", func(t *testing.T) {
		chat := client.StartChat("You are a helpful assistant with access to tools.", ClaudeModel)

		// Define a function
		functions := []*gollm.FunctionDefinition{
			{
				Name:        "test_function",
				Description: "A test function",
				Parameters: &gollm.Schema{
					Type: "object",
					Properties: map[string]*gollm.Schema{
						"param": {Type: "string"},
					},
					Required: []string{"param"},
				},
			},
		}

		err := chat.SetFunctionDefinitions(functions)
		require.NoError(t, err, "Should set function definitions")

		// Make a request that might trigger function calling
		response, err := chat.Send(ctx, "Use the test_function with parameter 'hello'.")
		require.NoError(t, err, "Initial request should succeed")

		candidates := response.Candidates()
		require.Greater(t, len(candidates), 0, "Should have candidates")

		// If function calls were made, test sending invalid results
		var functionCalls []gollm.FunctionCall
		parts := candidates[0].Parts()
		for _, part := range parts {
			if calls, ok := part.AsFunctionCalls(); ok {
				functionCalls = append(functionCalls, calls...)
			}
		}

		if len(functionCalls) > 0 {
			// Send invalid function call results
			invalidResults := []gollm.FunctionCallResult{
				{
					Name:   "nonexistent_function",
					Result: map[string]any{"error": "function not found"},
				},
			}

			// This should handle the error gracefully
			resultContents := make([]any, len(invalidResults))
			for i, result := range invalidResults {
				resultContents[i] = result
			}
			followupResponse, err := chat.Send(ctx, resultContents...)
			if err != nil {
				// Error is acceptable for invalid function results
				t.Logf("Expected error for invalid function results: %v", err)
			} else {
				// If no error, should still get a response
				require.NotNil(t, followupResponse, "Should handle invalid results gracefully")
				t.Log("Invalid function results handled gracefully")
			}
		}

		// The chat session should still be functional after error
		recoveryResponse, err := chat.Send(ctx, "Just say 'recovered' and nothing else.")
		require.NoError(t, err, "Chat should recover after function call error")
		require.NotNil(t, recoveryResponse, "Should get recovery response")

		t.Log("✅ Function call error recovery test completed")
	})

	t.Run("session_recovery_after_timeout", func(t *testing.T) {
		chat := client.StartChat("You are a helpful assistant.", ClaudeModel)

		// Make a normal request first
		response1, err := chat.Send(ctx, "Say 'first request'.")
		require.NoError(t, err, "First request should succeed")
		require.NotNil(t, response1, "Should get first response")

		// Make a request with a very short timeout
		shortCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
		defer cancel()

		_, err = chat.Send(shortCtx, "This request should timeout.")
		assert.Error(t, err, "Short timeout request should fail")
		assert.Contains(t, err.Error(), "context", "Error should relate to context")

		// The chat session should still work after timeout
		response2, err := chat.Send(ctx, "Say 'recovered after timeout'.")
		require.NoError(t, err, "Chat should recover after timeout")
		require.NotNil(t, response2, "Should get recovery response")

		candidates := response2.Candidates()
		require.Greater(t, len(candidates), 0, "Should have recovery candidates")
		content := ""
		parts := candidates[0].Parts()
		for _, part := range parts {
			if text, ok := part.AsText(); ok {
				content += text
			}
		}
		assert.NotEmpty(t, content, "Should get recovery content")

		t.Log("✅ Session recovery after timeout test completed")
	})
}

// TestRealWorldUsagePatterns tests patterns that are likely to be used in production
func TestRealWorldUsagePatterns(t *testing.T) {
	ctx := context.Background()

	t.Run("kubectl_ai_simulation", func(t *testing.T) {
		// Simulate exact kubectl-ai usage pattern
		var totalUsage []gollm.Usage
		var usageMutex sync.Mutex

		client, err := gollm.NewClient(ctx, "bedrock",
			gollm.WithInferenceConfig(&gollm.InferenceConfig{
				Model:       ClaudeModel,
				Temperature: 0.1,
				MaxTokens:   2000,
			}),
			gollm.WithUsageCallback(func(provider, model string, usage gollm.Usage) {
				usageMutex.Lock()
				defer usageMutex.Unlock()
				totalUsage = append(totalUsage, usage)
			}),
		)
		require.NoError(t, err, "Should create kubectl-ai style client")
		defer client.Close()

		// kubectl-ai typically starts with this kind of system prompt
		systemPrompt := `You are kubectl-ai, an AI assistant for Kubernetes. You help users with kubectl commands and Kubernetes troubleshooting.

Guidelines:
- Provide accurate kubectl commands
- Explain what each command does
- Consider security implications
- Suggest best practices
- Always validate resource names and namespaces`

		chat := client.StartChat(systemPrompt, ClaudeModel)

		// Simulate typical kubectl-ai interactions
		kubectlQueries := []string{
			"I have a pod that's not starting. The pod name is 'webapp-123' in namespace 'production'. Help me debug this.",
			"Show me how to check resource usage for all pods in the 'kube-system' namespace.",
			"My deployment is not rolling out properly. The deployment name is 'api-server' in 'default' namespace. What should I check?",
			"How do I scale my deployment 'frontend' to 5 replicas in the 'web' namespace?",
		}

		for i, query := range kubectlQueries {
			t.Run(fmt.Sprintf("kubectl_query_%d", i+1), func(t *testing.T) {
				// Use streaming as kubectl-ai would
				responseStream, err := chat.SendStreaming(ctx, query)
				require.NoError(t, err, "kubectl-ai streaming should work")

				var fullResponse strings.Builder
				chunkCount := 0

				for response, streamErr := range responseStream {
					if streamErr != nil {
						require.NoError(t, streamErr, "kubectl-ai stream should not error")
						break
					}

					if response == nil {
						break
					}

					chunkCount++
					candidates := response.Candidates()
					if len(candidates) > 0 {
						parts := candidates[0].Parts()
						for _, part := range parts {
							if text, ok := part.AsText(); ok {
								fullResponse.WriteString(text)
							}
						}
					}
				}

				responseText := fullResponse.String()
				assert.NotEmpty(t, responseText, "Should get kubectl-ai response")
				assert.Greater(t, chunkCount, 0, "Should receive streaming chunks")

				// Verify response quality for kubectl-ai
				assert.True(t,
					strings.Contains(responseText, "kubectl") || strings.Contains(responseText, "kubernetes"),
					"Response should contain kubectl/kubernetes content")

				t.Logf("kubectl-ai query %d: %d chunks, %d chars",
					i+1, chunkCount, len(responseText))
			})
		}

		// Verify total usage tracking
		usageMutex.Lock()
		totalQueries := len(totalUsage)
		totalTokens := 0
		for _, usage := range totalUsage {
			totalTokens += usage.TotalTokens
		}
		usageMutex.Unlock()

		assert.Equal(t, len(kubectlQueries), totalQueries, "Should track usage for each query")
		assert.Greater(t, totalTokens, 0, "Should accumulate token usage")

		t.Logf("✅ kubectl-ai simulation completed - %d queries, %d total tokens",
			totalQueries, totalTokens)
	})

	t.Run("multi_session_conversation", func(t *testing.T) {
		// Test multiple separate chat sessions (common in web applications)
		client, err := gollm.NewClient(ctx, "bedrock",
			gollm.WithInferenceConfig(&gollm.InferenceConfig{
				Model:       NovaModel,
				Temperature: 0.3,
				MaxTokens:   500,
			}),
		)
		require.NoError(t, err, "Should create client for multi-session test")
		defer client.Close()

		// Create multiple independent chat sessions
		sessions := []gollm.Chat{
			client.StartChat("You are a Kubernetes expert.", NovaModel),
			client.StartChat("You are a helpful coding assistant.", NovaModel),
			client.StartChat("You are a DevOps specialist.", NovaModel),
		}

		// Test each session independently
		for i, session := range sessions {
			t.Run(fmt.Sprintf("session_%d", i+1), func(t *testing.T) {
				prompt := fmt.Sprintf("Session %d: Hello, introduce yourself briefly.", i+1)
				response, err := session.Send(ctx, prompt)
				require.NoError(t, err, "Session %d should work", i+1)
				require.NotNil(t, response, "Session %d should get response", i+1)

				candidates := response.Candidates()
				require.Greater(t, len(candidates), 0, "Session %d should have candidates", i+1)
				content := ""
				parts := candidates[0].Parts()
				for _, part := range parts {
					if text, ok := part.AsText(); ok {
						content += text
					}
				}
				assert.NotEmpty(t, content, "Session %d should have content", i+1)

				t.Logf("Session %d response: %q", i+1, content[:min(100, len(content))])
			})
		}

		t.Log("✅ Multi-session conversation test completed")
	})
}
