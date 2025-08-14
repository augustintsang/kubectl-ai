package main

import (
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
)

func main() {
	jsonStr := `{"command": "get pods -n default"}`
	
	fmt.Println("=== Testing different LazyDocument creation methods ===\n")
	
	// Method 1: json.RawMessage
	fmt.Println("1. json.RawMessage:")
	doc1 := document.NewLazyDocument(json.RawMessage(jsonStr))
	fmt.Printf("   Type: %T\n", doc1)
	fmt.Printf("   Value: %+v\n", doc1)
	var map1 map[string]any
	err1 := doc1.UnmarshalSmithyDocument(&map1)
	fmt.Printf("   Unmarshal result: %v, map=%v\n\n", err1, map1)
	
	// Method 2: Parsed map[string]interface{}
	fmt.Println("2. Parsed map[string]interface{}:")
	var parsed1 map[string]interface{}
	json.Unmarshal([]byte(jsonStr), &parsed1)
	doc2 := document.NewLazyDocument(parsed1)
	fmt.Printf("   Type: %T\n", doc2)
	fmt.Printf("   Value: %+v\n", doc2)
	var map2 map[string]any
	err2 := doc2.UnmarshalSmithyDocument(&map2)
	fmt.Printf("   Unmarshal result: %v, map=%v\n\n", err2, map2)
	
	// Method 3: Parsed map[string]any
	fmt.Println("3. Parsed map[string]any:")
	var parsed2 map[string]any
	json.Unmarshal([]byte(jsonStr), &parsed2)
	doc3 := document.NewLazyDocument(parsed2)
	fmt.Printf("   Type: %T\n", doc3)
	fmt.Printf("   Value: %+v\n", doc3)
	var map3 map[string]any
	err3 := doc3.UnmarshalSmithyDocument(&map3)
	fmt.Printf("   Unmarshal result: %v, map=%v\n\n", err3, map3)
	
	// Method 4: interface{} (let json.Unmarshal decide the type)
	fmt.Println("4. interface{} (json decides type):")
	var parsed3 interface{}
	json.Unmarshal([]byte(jsonStr), &parsed3)
	fmt.Printf("   Parsed type: %T = %v\n", parsed3, parsed3)
	doc4 := document.NewLazyDocument(parsed3)
	fmt.Printf("   Doc type: %T\n", doc4)
	fmt.Printf("   Doc value: %+v\n", doc4)
	var map4 map[string]any
	err4 := doc4.UnmarshalSmithyDocument(&map4)
	fmt.Printf("   Unmarshal result: %v, map=%v\n", err4, map4)
}