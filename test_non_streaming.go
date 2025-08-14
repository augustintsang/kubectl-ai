package main

import (
	"context"
	"fmt"
	"log"
	"reflect"
	
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

	fmt.Println("=== NON-STREAMING: Analyzing ToolUseBlock structure ===")

	
	response, err := chat.Send(context.Background(), "List all pods in the default namespace")
	if err != nil {
		log.Fatalf("Failed to send: %v", err)
	}

	// Analyze the response
	candidates := response.Candidates()
	for _, candidate := range candidates {
		parts := candidate.Parts()
		for _, part := range parts {
			if calls, ok := part.AsFunctionCalls(); ok {
				for _, call := range calls {
					fmt.Printf("Tool call found: %s(%v)\n", call.Name, call.Arguments)
					
					// Try to get the underlying part type
					fmt.Printf("Part type: %T\n", part)
					fmt.Printf("Part value: %+v\n", part)
					
					// Use reflection to inspect
					v := reflect.ValueOf(part)
					if v.Kind() == reflect.Ptr {
						v = v.Elem()
					}
					fmt.Printf("Part fields:\n")
					t := v.Type()
					for i := 0; i < v.NumField(); i++ {
						field := t.Field(i)
						value := v.Field(i)
						fmt.Printf("  %s (%s): %v\n", field.Name, field.Type, value.Interface())
						
						// If it's toolUse, inspect deeper
						if field.Name == "toolUse" && !value.IsNil() {
							toolUse := value.Elem()
							fmt.Printf("  ToolUse fields:\n")
							for j := 0; j < toolUse.NumField(); j++ {
								tf := toolUse.Type().Field(j)
								tv := toolUse.Field(j)
								fmt.Printf("    %s (%s): %v\n", tf.Name, tf.Type, tv.Interface())
								
								// Special handling for Input field
								if tf.Name == "Input" && !tv.IsNil() {
									fmt.Printf("    Input type details: %T\n", tv.Interface())
									
									// Try to see what's inside
									if doc, ok := tv.Interface().(interface{ UnmarshalSmithyDocument(v interface{}) error }); ok {
										var raw interface{}
										if err := doc.UnmarshalSmithyDocument(&raw); err == nil {
											fmt.Printf("    Input unmarshalled as interface{}: %T = %v\n", raw, raw)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
}