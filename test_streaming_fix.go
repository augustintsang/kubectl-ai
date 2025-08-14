package main

import (
	"context"
	"fmt"
	"log"
	
	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
)

func main() {
	client, err := gollm.NewBedrockClient(context.Background(), gollm.ClientOptions{})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	chat := client.StartChat("You are a helpful assistant.", "us.anthropic.claude-sonnet-4-20250514-v1:0")

	// Set up kubectl function
	functions := []*gollm.FunctionDefinition{{
		Name:        "kubectl",
		Description: "Execute kubectl commands",
		Parameters: &gollm.Schema{
			Type: gollm.TypeObject,
			Properties: map[string]*gollm.Schema{
				"command": {Type: gollm.TypeString, Description: "kubectl command to execute"},
			},
			Required: []string{"command"},
		},
	}}

	if err := chat.SetFunctionDefinitions(functions); err != nil {
		log.Fatalf("Failed to set functions: %v", err)
	}

	fmt.Println("Testing FIXED streaming tool call behavior...")
	fmt.Println("==========================================")

	iterator, err := chat.SendStreaming(context.Background(), "List all pods in the default namespace")
	if err != nil {
		log.Fatalf("Failed to send streaming: %v", err)
	}

	chunkCount := 0
	toolCallsFound := 0
	for response, err := range iterator {
		if err != nil {
			log.Printf("Streaming error: %v", err)
			break
		}

		chunkCount++
		fmt.Printf("\n--- Chunk %d ---\n", chunkCount)
		
		candidates := response.Candidates()
		fmt.Printf("Candidates: %d\n", len(candidates))
		
		for i, candidate := range candidates {
			fmt.Printf("Candidate %d:\n", i)
			fmt.Printf("  String: %s\n", candidate.String())
			
			parts := candidate.Parts()
			fmt.Printf("  Parts: %d\n", len(parts))
			
			for j, part := range parts {
				fmt.Printf("    Part %d:\n", j)
				if text, ok := part.AsText(); ok && text != "" {
					fmt.Printf("      Text: %s\n", text)
				}
				if calls, ok := part.AsFunctionCalls(); ok {
					fmt.Printf("      FunctionCalls: %d\n", len(calls))
					toolCallsFound += len(calls)
					for _, call := range calls {
						fmt.Printf("        Call: %s(%v)\n", call.Name, call.Arguments)
					}
				}
			}
		}
	}

	fmt.Printf("\n=== RESULTS ===\n")
	fmt.Printf("Total chunks received: %d\n", chunkCount)
	fmt.Printf("Total tool calls found: %d\n", toolCallsFound)
	
	if toolCallsFound > 0 {
		fmt.Printf("✅ SUCCESS: Streaming tool calls are now working!\n")
	} else {
		fmt.Printf("❌ ISSUE: No tool calls detected in streaming\n")
	}
}