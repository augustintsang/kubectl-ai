package bedrock

import (
	"context"
	"fmt"
	"log"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

// Example: LLM App Migration Guide
// This shows how to migrate from global gollm interfaces to bedrock-specific approaches
// that satisfy droot's requirements without affecting other providers.

// OLD APPROACH (remove - affects global interfaces):
func OldApproachExample() {
	// This is what we need to REMOVE - it affects global gollm interfaces
	/*
		config := &gollm.InferenceConfig{
			Model:       "us.anthropic.claude-sonnet-4-20250514-v1:0",
			Temperature: 0.7,
			MaxTokens:   4000,
			TopP:        0.9,
		}

		var totalUsage []gollm.Usage
		callback := func(provider, model string, usage gollm.Usage) {
			totalUsage = append(totalUsage, usage)
		}

		// DON'T USE - affects global interfaces
		client, err := gollm.NewClient(ctx, "bedrock",
			gollm.WithInferenceConfig(config),
			gollm.WithUsageCallback(callback),
		)
	*/
}

// NEW APPROACH (use - bedrock-specific only):
func NewApproachExample() {
	ctx := context.Background()

	// Step 1: Configure usage tracking (bedrock-specific)
	var totalUsage []gollm.Usage
	usageCallback := func(provider, model string, usage gollm.Usage) {
		totalUsage = append(totalUsage, usage)
		log.Printf("Usage: %s/%s - %d tokens ($%.4f)",
			provider, model, usage.TotalTokens, usage.TotalCost)
	}

	// Step 2: Create bedrock-specific client options
	bedrockOpts := BedrockClientOptions{
		// Basic options
		Model:       "us.anthropic.claude-sonnet-4-20250514-v1:0",
		Region:      "us-west-2",
		Temperature: 0.7,
		MaxTokens:   4000,
		TopP:        0.9,
		MaxRetries:  3,

		// Bedrock-specific features
		UsageCallback: usageCallback,
		Debug:         true,
	}

	// Step 3: Create bedrock client with enhanced config
	bedrockClient, err := NewBedrockClientWithConfig(ctx, bedrockOpts)
	if err != nil {
		log.Fatalf("Failed to create bedrock client: %v", err)
	}
	defer bedrockClient.Close()

	// Step 4: Wrap as gollm.Client if needed for compatibility
	client := WrapAsGollmClient(bedrockClient)

	// Step 5: Use normally - usage tracking happens automatically
	chat := client.StartChat("You are a helpful assistant", "")
	response, err := chat.Send(ctx, "Hello, how are you?")
	if err != nil {
		log.Fatalf("Chat failed: %v", err)
	}

	// Step 6: Extract usage metadata (still works!)
	if usage, ok := response.UsageMetadata().(*gollm.Usage); ok {
		fmt.Printf("This call used %d tokens\n", usage.TotalTokens)
	}

	fmt.Printf("Session total: %d calls tracked\n", len(totalUsage))
}

// ENVIRONMENT VARIABLE APPROACH (droot's preference):
func EnvironmentVariableExample() {
	ctx := context.Background()

	// Set environment variables for advanced config (droot prefers this)
	/*
		export BEDROCK_TEMPERATURE=0.7
		export BEDROCK_MAX_TOKENS=4000
		export BEDROCK_TOP_P=0.9
		export BEDROCK_MODEL=us.anthropic.claude-sonnet-4-20250514-v1:0
		export BEDROCK_REGION=us-west-2
		export BEDROCK_DEBUG=true
	*/

	// Create client that loads config from environment
	bedrockOpts := LoadBedrockConfigFromEnv()

	// Add usage callback programmatically
	bedrockOpts.UsageCallback = func(provider, model string, usage gollm.Usage) {
		log.Printf("Env config usage: %s/%s - %d tokens", provider, model, usage.TotalTokens)
	}

	client, err := NewBedrockClientWithConfig(ctx, bedrockOpts)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Use as normal gollm.Client
	gollmClient := WrapAsGollmClient(client)
	chat := gollmClient.StartChat("You are helpful", "")

	response, err := chat.Send(ctx, "Hello")
	if err != nil {
		log.Fatalf("Chat failed: %v", err)
	}

	fmt.Printf("Response: %s\n", response.Candidates()[0])
}

// KUBECTL-AI INTEGRATION PATTERN:
func KubectlAIIntegrationExample() {
	ctx := context.Background()

	// This is how kubectl-ai can integrate bedrock with enhanced features
	// without affecting other providers

	// Configure for kubectl-ai usage
	bedrockOpts := BedrockClientOptions{
		Model:       "us.anthropic.claude-sonnet-4-20250514-v1:0",
		Temperature: 0.1,  // More deterministic for kubectl commands
		MaxTokens:   2000, // Reasonable limit for CLI responses

		// Track usage for cost monitoring
		UsageCallback: func(provider, model string, usage gollm.Usage) {
			// kubectl-ai could log this to usage analytics
			log.Printf("kubectl-ai usage: %d tokens for model %s", usage.TotalTokens, model)
		},
	}

	// Create bedrock client
	bedrockClient, err := NewBedrockClientWithConfig(ctx, bedrockOpts)
	if err != nil {
		log.Fatalf("Failed to create kubectl-ai bedrock client: %v", err)
	}

	// Wrap for compatibility with existing kubectl-ai client interface
	client := WrapAsGollmClient(bedrockClient)

	// kubectl-ai system prompt
	systemPrompt := `You are kubectl-ai, an AI assistant for Kubernetes operations.
Help users with kubectl commands and Kubernetes troubleshooting.`

	chat := client.StartChat(systemPrompt, "")

	// Typical kubectl-ai interaction
	response, err := chat.Send(ctx, "I have a pod that's not starting. How do I debug this?")
	if err != nil {
		log.Fatalf("kubectl-ai query failed: %v", err)
	}

	// Extract response content
	candidates := response.Candidates()
	if len(candidates) > 0 {
		parts := candidates[0].Parts()
		for _, part := range parts {
			if text, ok := part.AsText(); ok {
				fmt.Printf("kubectl-ai response: %s\n", text)
			}
		}
	}

	// Usage tracking happens automatically via callback
	if usage, ok := response.UsageMetadata().(*gollm.Usage); ok {
		fmt.Printf("kubectl-ai used %d tokens for this query\n", usage.TotalTokens)
	}
}

// BACKWARD COMPATIBILITY:
func BackwardCompatibilityExample() {
	ctx := context.Background()

	// Existing code that expects gollm.Client interface still works

	// Option 1: Simple creation (no enhanced features)
	client, err := gollm.NewClient(ctx, "bedrock")
	if err != nil {
		log.Fatalf("Simple client creation failed: %v", err)
	}
	defer client.Close()

	// Option 2: Enhanced creation with bedrock-specific features
	bedrockOpts := BedrockClientOptions{
		Temperature: 0.8,
		MaxTokens:   3000,
	}

	bedrockClient, err := NewBedrockClientWithConfig(ctx, bedrockOpts)
	if err != nil {
		log.Fatalf("Enhanced client creation failed: %v", err)
	}

	// Wrap to satisfy gollm.Client interface
	enhancedClient := WrapAsGollmClient(bedrockClient)

	// Both clients work the same way from here
	chat1 := client.StartChat("You are helpful", "")
	chat2 := enhancedClient.StartChat("You are helpful", "")

	response1, _ := chat1.Send(ctx, "Hello")
	response2, _ := chat2.Send(ctx, "Hello")

	fmt.Printf("Simple client response: %s\n", response1.Candidates()[0])
	fmt.Printf("Enhanced client response: %s\n", response2.Candidates()[0])

	// Only enhanced client has usage tracking
	if usage, ok := response2.UsageMetadata().(*gollm.Usage); ok {
		fmt.Printf("Enhanced client tracked %d tokens\n", usage.TotalTokens)
	}
}
