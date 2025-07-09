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
	"fmt"
	"log"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

// DemoBedrockUsageTracking demonstrates how to use the enhanced Bedrock provider
// with usage tracking and inference configuration.
func DemoBedrockUsageTracking() {
	// This example shows the functionality without requiring AWS credentials
	// In practice, you would set up AWS credentials via environment variables or IAM roles

	// Set up usage tracking
	var usageStats []gollm.Usage

	// Define usage callback to collect metrics
	usageCallback := func(provider, model string, usage gollm.Usage) {
		usageStats = append(usageStats, usage)

		log.Printf("Usage captured - Provider: %s, Model: %s", provider, model)
		log.Printf("Tokens: Input=%d, Output=%d, Total=%d",
			usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
		log.Printf("Cost: Input=$%.4f, Output=$%.4f, Total=$%.4f",
			usage.InputCost, usage.OutputCost, usage.TotalCost)
	}

	// Configure inference parameters
	config := &gollm.InferenceConfig{
		Model:       "us.anthropic.claude-sonnet-4-20250514-v1:0",
		Region:      "us-west-2",
		Temperature: 0.7,
		MaxTokens:   4000,
		TopP:        0.9,
		MaxRetries:  3,
	}

	// Note: This would normally create a real client, but we're showing the interface
	// ctx := context.Background()
	// client, err := gollm.NewClient(ctx, "bedrock",
	// 	gollm.WithInferenceConfig(config),
	// 	gollm.WithUsageCallback(usageCallback),
	// 	gollm.WithDebug(true),
	// )
	// if err != nil {
	// 	log.Fatalf("Failed to create client: %v", err)
	// }
	// defer client.Close()

	// For demonstration, we'll show what the client options would look like
	clientOpts := gollm.ClientOptions{
		InferenceConfig: config,
		UsageCallback:   usageCallback,
		Debug:           true,
	}

	// Show how options are merged
	merged := mergeWithClientOptions(DefaultOptions, clientOpts)

	fmt.Printf("Original Default Options:\n")
	fmt.Printf("  Model: %s\n", DefaultOptions.Model)
	fmt.Printf("  Region: %s\n", DefaultOptions.Region)
	fmt.Printf("  Temperature: %.2f\n", DefaultOptions.Temperature)
	fmt.Printf("  MaxTokens: %d\n", DefaultOptions.MaxTokens)
	fmt.Printf("  TopP: %.2f\n", DefaultOptions.TopP)

	fmt.Printf("\nMerged Options (with InferenceConfig):\n")
	fmt.Printf("  Model: %s\n", merged.Model)
	fmt.Printf("  Region: %s\n", merged.Region)
	fmt.Printf("  Temperature: %.2f\n", merged.Temperature)
	fmt.Printf("  MaxTokens: %d\n", merged.MaxTokens)
	fmt.Printf("  TopP: %.2f\n", merged.TopP)

	// Demonstrate usage conversion
	fmt.Printf("\nUsage Tracking Demonstration:\n")

	// Simulate AWS usage data (this would come from actual API calls)
	// mockUsage := &types.TokenUsage{
	// 	InputTokens:  aws.Int32(150),
	// 	OutputTokens: aws.Int32(75),
	// 	TotalTokens:  aws.Int32(225),
	// }

	// Show how usage would be converted and tracked
	// structuredUsage := convertAWSUsage(mockUsage, config.Model, "bedrock")
	// if structuredUsage != nil {
	// 	usageCallback("bedrock", config.Model, *structuredUsage)
	// }

	fmt.Printf("Usage callback would be called with structured gollm.Usage data\n")
	fmt.Printf("Collected usage stats: %d records\n", len(usageStats))

	// Example of what would happen in a real scenario:
	// chat := client.StartChat("You are a helpful assistant", "")
	// response, err := chat.Send(ctx, "Hello, how are you?")
	// if err != nil {
	// 	log.Fatalf("Chat failed: %v", err)
	// }
	//
	// // Get structured usage from response
	// if usage, ok := response.UsageMetadata().(*gollm.Usage); ok {
	// 	fmt.Printf("Response usage: %d tokens, model: %s\n", usage.TotalTokens, usage.Model)
	// }
	//
	// // Usage callback would have been automatically called
	// fmt.Printf("Total usage records captured: %d\n", len(usageStats))

	fmt.Printf("\n✅ Enhanced Bedrock provider supports:\n")
	fmt.Printf("   - Provider-agnostic inference configuration\n")
	fmt.Printf("   - Structured usage tracking with callbacks\n")
	fmt.Printf("   - Per-chat usage metrics\n")
	fmt.Printf("   - Backward compatibility with existing code\n")
}

// DemoBedrockInferenceConfig shows different inference configuration scenarios
func DemoBedrockInferenceConfig() {
	fmt.Printf("Bedrock Inference Configuration Examples:\n\n")

	// Example 1: Custom model and region
	config1 := &gollm.InferenceConfig{
		Model:  "us.amazon.nova-pro-v1:0",
		Region: "us-east-1",
	}
	merged1 := mergeWithClientOptions(DefaultOptions, gollm.ClientOptions{InferenceConfig: config1})
	fmt.Printf("1. Custom model and region:\n")
	fmt.Printf("   Model: %s (was: %s)\n", merged1.Model, DefaultOptions.Model)
	fmt.Printf("   Region: %s (was: %s)\n", merged1.Region, DefaultOptions.Region)
	fmt.Printf("   Temperature: %.2f (unchanged)\n", merged1.Temperature)

	// Example 2: Creative writing parameters
	config2 := &gollm.InferenceConfig{
		Temperature: 0.9,
		TopP:        0.95,
		MaxTokens:   8000,
	}
	merged2 := mergeWithClientOptions(DefaultOptions, gollm.ClientOptions{InferenceConfig: config2})
	fmt.Printf("\n2. Creative writing parameters:\n")
	fmt.Printf("   Temperature: %.2f (was: %.2f)\n", merged2.Temperature, DefaultOptions.Temperature)
	fmt.Printf("   TopP: %.2f (was: %.2f)\n", merged2.TopP, DefaultOptions.TopP)
	fmt.Printf("   MaxTokens: %d (was: %d)\n", merged2.MaxTokens, DefaultOptions.MaxTokens)

	// Example 3: Conservative/analytical parameters
	config3 := &gollm.InferenceConfig{
		Temperature: 0.1,
		TopP:        0.5,
		MaxTokens:   2000,
		MaxRetries:  1,
	}
	merged3 := mergeWithClientOptions(DefaultOptions, gollm.ClientOptions{InferenceConfig: config3})
	fmt.Printf("\n3. Conservative/analytical parameters:\n")
	fmt.Printf("   Temperature: %.2f (was: %.2f)\n", merged3.Temperature, DefaultOptions.Temperature)
	fmt.Printf("   TopP: %.2f (was: %.2f)\n", merged3.TopP, DefaultOptions.TopP)
	fmt.Printf("   MaxTokens: %d (was: %d)\n", merged3.MaxTokens, DefaultOptions.MaxTokens)
	fmt.Printf("   MaxRetries: %d (was: %d)\n", merged3.MaxRetries, DefaultOptions.MaxRetries)

	// Example 4: Partial override (only temperature)
	config4 := &gollm.InferenceConfig{
		Temperature: 0.5,
	}
	merged4 := mergeWithClientOptions(DefaultOptions, gollm.ClientOptions{InferenceConfig: config4})
	fmt.Printf("\n4. Partial override (temperature only):\n")
	fmt.Printf("   Temperature: %.2f (was: %.2f)\n", merged4.Temperature, DefaultOptions.Temperature)
	fmt.Printf("   Other parameters remain at defaults\n")

	fmt.Printf("\n✅ Inference configuration provides flexible parameter control\n")
}

// DemoUsageAggregation demonstrates aggregating usage across multiple calls
func DemoUsageAggregation() {
	fmt.Printf("Usage Aggregation Example:\n\n")

	// Usage aggregator
	type UsageAggregator struct {
		totalInputTokens  int
		totalOutputTokens int
		totalCost         float64
		callCount         int
		modelUsage        map[string]gollm.Usage
	}

	aggregator := &UsageAggregator{
		modelUsage: make(map[string]gollm.Usage),
	}

	// Usage callback that aggregates metrics
	usageCallback := func(provider, model string, usage gollm.Usage) {
		aggregator.totalInputTokens += usage.InputTokens
		aggregator.totalOutputTokens += usage.OutputTokens
		aggregator.totalCost += usage.TotalCost
		aggregator.callCount++

		// Track per-model usage
		if existing, exists := aggregator.modelUsage[model]; exists {
			existing.InputTokens += usage.InputTokens
			existing.OutputTokens += usage.OutputTokens
			existing.TotalCost += usage.TotalCost
			aggregator.modelUsage[model] = existing
		} else {
			aggregator.modelUsage[model] = usage
		}

		fmt.Printf("Call %d: %s used %d tokens (Input: %d, Output: %d)\n",
			aggregator.callCount, model, usage.TotalTokens, usage.InputTokens, usage.OutputTokens)
	}

	// Simulate multiple API calls with different models
	scenarios := []struct {
		model        string
		inputTokens  int
		outputTokens int
	}{
		{"us.anthropic.claude-sonnet-4-20250514-v1:0", 100, 50},
		{"us.amazon.nova-pro-v1:0", 150, 75},
		{"us.anthropic.claude-sonnet-4-20250514-v1:0", 80, 40},
		{"us.amazon.nova-lite-v1:0", 200, 100},
	}

	for _, scenario := range scenarios {
		usage := gollm.Usage{
			InputTokens:  scenario.inputTokens,
			OutputTokens: scenario.outputTokens,
			TotalTokens:  scenario.inputTokens + scenario.outputTokens,
			Model:        scenario.model,
			Provider:     "bedrock",
			// In real scenario, costs would be calculated
			TotalCost: float64(scenario.inputTokens+scenario.outputTokens) * 0.0001, // Mock cost
		}

		usageCallback("bedrock", scenario.model, usage)
	}

	fmt.Printf("\n📊 Aggregated Usage Summary:\n")
	fmt.Printf("Total Calls: %d\n", aggregator.callCount)
	fmt.Printf("Total Input Tokens: %d\n", aggregator.totalInputTokens)
	fmt.Printf("Total Output Tokens: %d\n", aggregator.totalOutputTokens)
	fmt.Printf("Total Tokens: %d\n", aggregator.totalInputTokens+aggregator.totalOutputTokens)
	fmt.Printf("Total Cost: $%.4f\n", aggregator.totalCost)

	fmt.Printf("\n📈 Per-Model Usage:\n")
	for model, usage := range aggregator.modelUsage {
		shortModel := model
		if len(model) > 30 {
			shortModel = model[:30] + "..."
		}
		fmt.Printf("  %s: %d tokens, $%.4f\n", shortModel, usage.TotalTokens, usage.TotalCost)
	}

	fmt.Printf("\n✅ Usage tracking enables cost monitoring and optimization\n")
}
