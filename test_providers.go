package main

import (
	"context"
	"fmt"
	"log"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	// Import bedrock package to register the provider
	_ "github.com/GoogleCloudPlatform/kubectl-ai/gollm/bedrock"
)

func main() {
	fmt.Println("Testing Bedrock Provider Registration...")

	ctx := context.Background()

	// Try to create a bedrock client to verify it's registered
	fmt.Println("Attempting to create bedrock client...")

	client, err := gollm.NewClient(ctx, "bedrock",
		gollm.WithInferenceConfig(&gollm.InferenceConfig{
			Model:  "anthropic.claude-3-sonnet-20240229-v1:0",
			Region: "us-west-2",
		}),
		gollm.WithDebug(true),
	)

	if err != nil {
		// We expect some error (likely AWS credential related), but NOT "provider not registered"
		if fmt.Sprintf("%v", err) == `provider "bedrock" not registered. Available providers: [grok llamacpp ollama openai openai-compatible azopenai gemini vertexai]` {
			log.Fatalf("❌ FAILED: Bedrock provider is NOT registered: %v", err)
		} else {
			fmt.Printf("✅ SUCCESS: Bedrock provider is registered! Got expected credential/config error: %v\n", err)
		}
	} else {
		fmt.Println("✅ SUCCESS: Bedrock provider is registered and client created successfully!")
		if client != nil {
			fmt.Println("✅ Client is not nil")
		}
	}

	// Also test listing models if client was created
	if client != nil {
		models, err := client.ListModels(ctx)
		if err != nil {
			fmt.Printf("⚠️  Could not list models: %v\n", err)
		} else {
			fmt.Printf("✅ Available models: %v\n", models)
		}

		// Close the client
		client.Close()
	}

	fmt.Println("✅ Provider registration test completed successfully!")
}
