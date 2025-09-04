package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

func main() {
	fmt.Printf("Using endpoint: %s\n", os.Getenv("NIRMATA_ENDPOINT"))
	
	// Create Nirmata client
	client, err := gollm.NewClient(context.Background(), "nirmata", gollm.WithSkipVerifySSL())
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	fmt.Println("Testing non-streaming chat...")
	
	// Test basic chat (no system prompt first to match your curl)
	chat := client.StartChat("", "us.anthropic.claude-sonnet-4-20250514-v1:0")
	resp, err := chat.Send(context.Background(), "Hello, how are you today?")
	if err != nil {
		log.Fatalf("Chat send failed: %v", err)
	}

	candidates := resp.Candidates()
	if len(candidates) == 0 {
		log.Fatal("No candidates in response")
	}

	fmt.Printf("Response: %s\n", candidates[0].String())
	fmt.Printf("Usage metadata: %v\n", resp.UsageMetadata())

	fmt.Println("\nTesting streaming chat...")
	
	// Test streaming
	iterator, err := chat.SendStreaming(context.Background(), "Tell me a short joke")
	if err != nil {
		log.Fatalf("Streaming failed: %v", err)
	}

	fmt.Print("Streaming response: ")
	for streamResp, streamErr := range iterator {
		if streamErr != nil {
			log.Fatalf("Stream error: %v", streamErr)
		}
		
		candidates := streamResp.Candidates()
		if len(candidates) > 0 {
			fmt.Print(candidates[0].String())
		}
	}
	fmt.Println()
	
	fmt.Println("End-to-end test completed successfully!")
}