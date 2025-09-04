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

package gollm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
)

func TestNirmataClientCreation(t *testing.T) {
	tests := []struct {
		name          string
		apiKey        string
		endpoint      string
		baseURL       string
		expectedError bool
		errorContains string
	}{
		{
			name:          "missing API key (now optional)",
			apiKey:        "",
			endpoint:      "https://api.nirmata.com:8443",
			expectedError: false,
		},
		{
			name:          "missing endpoint",
			apiKey:        "test-jwt-token",
			endpoint:      "",
			baseURL:       "",
			expectedError: true,
			errorContains: "NIRMATA_ENDPOINT or NIRMATA_BASE_URL environment variable required",
		},
		{
			name:          "valid configuration with endpoint",
			apiKey:        "test-jwt-token",
			endpoint:      "https://api.nirmata.com:8443",
			expectedError: false,
		},
		{
			name:          "valid configuration with baseURL",
			apiKey:        "test-jwt-token",
			baseURL:       "https://localhost:8443",
			expectedError: false,
		},
		{
			name:          "endpoint takes precedence over baseURL",
			apiKey:        "test-jwt-token",
			endpoint:      "https://api.nirmata.com:8443",
			baseURL:       "https://localhost:8443",
			expectedError: false,
		},
		{
			name:          "invalid URL",
			apiKey:        "test-jwt-token",
			endpoint:      "http://[::1:bad:url",
			expectedError: true,
			errorContains: "parsing base URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			os.Setenv("NIRMATA_API_KEY", tt.apiKey)
			os.Setenv("NIRMATA_ENDPOINT", tt.endpoint)
			os.Setenv("NIRMATA_BASE_URL", tt.baseURL)
			defer func() {
				os.Unsetenv("NIRMATA_API_KEY")
				os.Unsetenv("NIRMATA_ENDPOINT")
				os.Unsetenv("NIRMATA_BASE_URL")
			}()

			ctx := context.Background()
			client, err := NewNirmataClient(ctx, ClientOptions{})

			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if client == nil {
				t.Error("expected non-nil client")
			}

			// Verify client properties
			if client.apiKey != tt.apiKey {
				t.Errorf("expected apiKey %q, got %q", tt.apiKey, client.apiKey)
			}

			expectedURL := tt.endpoint
			if expectedURL == "" {
				expectedURL = tt.baseURL
			}
			if client.baseURL.String() != expectedURL {
				t.Errorf("expected baseURL %q, got %q", expectedURL, client.baseURL.String())
			}
		})
	}
}

func TestNirmataModelSelection(t *testing.T) {
	tests := []struct {
		name          string
		envModel      string
		providedModel string
		expectedModel string
	}{
		{
			name:          "default model when none provided",
			envModel:      "",
			providedModel: "",
			expectedModel: "us.anthropic.claude-sonnet-4-20250514-v1:0",
		},
		{
			name:          "environment variable model",
			envModel:      "custom-model",
			providedModel: "",
			expectedModel: "custom-model",
		},
		{
			name:          "provided model takes precedence",
			envModel:      "env-model",
			providedModel: "provided-model",
			expectedModel: "provided-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envModel != "" {
				os.Setenv("NIRMATA_MODEL", tt.envModel)
				defer os.Unsetenv("NIRMATA_MODEL")
			}

			actualModel := getNirmataModel(tt.providedModel)
			if actualModel != tt.expectedModel {
				t.Errorf("expected model %q, got %q", tt.expectedModel, actualModel)
			}
		})
	}
}

func TestNirmataMessageConversion(t *testing.T) {
	chat := &nirmataChat{}

	tests := []struct {
		name            string
		contents        []any
		expectedRole    string
		expectedContent string
	}{
		{
			name:            "single string",
			contents:        []any{"Hello, world!"},
			expectedRole:    "user",
			expectedContent: "Hello, world!",
		},
		{
			name:            "multiple strings",
			contents:        []any{"Hello", "world"},
			expectedRole:    "user",
			expectedContent: "Hello world",
		},
		{
			name: "api.Message",
			contents: []any{&api.Message{
				Type:    api.MessageTypeText,
				Payload: "Message from API",
			}},
			expectedRole:    "user",
			expectedContent: "Message from API",
		},
		{
			name:            "mixed types",
			contents:        []any{"Hello", 123, "world"},
			expectedRole:    "user",
			expectedContent: "Hello 123 world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := chat.convertContentsToMessage(tt.contents)

			if msg.Role != tt.expectedRole {
				t.Errorf("expected role %q, got %q", tt.expectedRole, msg.Role)
			}

			if msg.Content != tt.expectedContent {
				t.Errorf("expected content %q, got %q", tt.expectedContent, msg.Content)
			}
		})
	}
}

func TestNirmataChatInitialize(t *testing.T) {
	chat := &nirmataChat{
		systemPrompt: "You are a helpful assistant",
	}

	history := []*api.Message{
		{
			Source:  api.MessageSourceUser,
			Type:    api.MessageTypeText,
			Payload: "Hello",
		},
		{
			Source:  api.MessageSourceModel,
			Type:    api.MessageTypeText,
			Payload: "Hi there!",
		},
		{
			Source:  api.MessageSourceUser,
			Type:    api.MessageTypeText,
			Payload: "How are you?",
		},
	}

	err := chat.Initialize(history)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should have system prompt + 3 messages = 4 total
	if len(chat.history) != 4 {
		t.Errorf("expected 4 messages in history, got %d", len(chat.history))
	}

	// First message should be system prompt
	if chat.history[0].Role != "system" || chat.history[0].Content != "You are a helpful assistant" {
		t.Error("expected first message to be system prompt")
	}

	// Check message conversion
	if chat.history[1].Role != "user" || chat.history[1].Content != "Hello" {
		t.Error("expected second message to be user message")
	}

	if chat.history[2].Role != "assistant" || chat.history[2].Content != "Hi there!" {
		t.Error("expected third message to be assistant message")
	}
}

func TestNirmataHTTPIntegration(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and headers
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "NIRMATA-JWT ") {
			t.Errorf("expected NIRMATA-JWT auth header, got %s", authHeader)
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("expected application/json content type, got %s", contentType)
		}

		// Verify request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("error reading request body: %v", err)
		}

		var req nirmataChatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("error unmarshaling request: %v", err)
		}

		// Should have system prompt + user message
		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages in request, got %d", len(req.Messages))
		}

		if req.Messages[0].Role != "system" {
			t.Errorf("expected first message to be system, got %s", req.Messages[0].Role)
		}

		if req.Messages[1].Role != "user" {
			t.Errorf("expected second message to be user, got %s", req.Messages[1].Role)
		}

		// Send response
		response := nirmataChatResponse{
			Message: "Hello! I'm doing well, thank you for asking.",
			Metadata: map[string]any{
				"tokens_used": 15,
				"model":       "claude-sonnet-4",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Set up environment
	os.Setenv("NIRMATA_API_KEY", "test-jwt-token")
	os.Setenv("NIRMATA_ENDPOINT", server.URL)
	defer func() {
		os.Unsetenv("NIRMATA_API_KEY")
		os.Unsetenv("NIRMATA_ENDPOINT")
	}()

	// Create client and chat
	ctx := context.Background()
	client, err := NewNirmataClient(ctx, ClientOptions{})
	if err != nil {
		t.Fatalf("error creating client: %v", err)
	}

	chat := client.StartChat("You are a helpful assistant", "claude-sonnet-4")
	if chat == nil {
		t.Fatal("expected non-nil chat")
	}

	// Send message
	response, err := chat.Send(ctx, "Hello, how are you?")
	if err != nil {
		t.Fatalf("error sending message: %v", err)
	}

	// Verify response
	if response == nil {
		t.Fatal("expected non-nil response")
	}

	candidates := response.Candidates()
	if len(candidates) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(candidates))
	}

	if candidates[0].String() != "Hello! I'm doing well, thank you for asking." {
		t.Errorf("unexpected response content: %s", candidates[0].String())
	}

	// Verify metadata
	metadata := response.UsageMetadata()
	if metadata == nil {
		t.Error("expected non-nil metadata")
	}
}

func TestNirmataStreamingIntegration(t *testing.T) {
	// Create a mock HTTP server for streaming
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send streaming response
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)

		chunks := []string{"Hello", " there", "!", " How", " are", " you?"}
		for _, chunk := range chunks {
			fmt.Fprint(w, chunk+"\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(10 * time.Millisecond) // Small delay to simulate streaming
		}
	}))
	defer server.Close()

	// Set up environment
	os.Setenv("NIRMATA_API_KEY", "test-jwt-token")
	os.Setenv("NIRMATA_ENDPOINT", server.URL)
	defer func() {
		os.Unsetenv("NIRMATA_API_KEY")
		os.Unsetenv("NIRMATA_ENDPOINT")
	}()

	// Create client and chat
	ctx := context.Background()
	client, err := NewNirmataClient(ctx, ClientOptions{})
	if err != nil {
		t.Fatalf("error creating client: %v", err)
	}

	chat := client.StartChat("", "claude-sonnet-4")
	iterator, err := chat.SendStreaming(ctx, "Tell me a story")
	if err != nil {
		t.Fatalf("error starting streaming: %v", err)
	}

	// Collect streaming chunks
	var chunks []string
	for response, err := range iterator {
		if err != nil {
			t.Errorf("streaming error: %v", err)
			break
		}

		candidates := response.Candidates()
		if len(candidates) > 0 {
			chunk := candidates[0].String()
			if chunk != "" {
				chunks = append(chunks, chunk)
			}
		}
	}

	// Verify we received chunks
	if len(chunks) == 0 {
		t.Error("expected to receive streaming chunks")
	}

	expectedChunks := []string{"Hello", " there", "!", " How", " are", " you?"}
	if len(chunks) != len(expectedChunks) {
		t.Errorf("expected %d chunks, got %d", len(expectedChunks), len(chunks))
	}

	for i, chunk := range chunks {
		if i < len(expectedChunks) && chunk != expectedChunks[i] {
			t.Errorf("expected chunk %d to be %q, got %q", i, expectedChunks[i], chunk)
		}
	}
}

func TestNirmataErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedError  bool
		errorContains  string
	}{
		{
			name:          "401 Unauthorized",
			statusCode:    401,
			responseBody:  "Invalid JWT token",
			expectedError: true,
			errorContains: "HTTP 401",
		},
		{
			name:          "429 Rate Limited",
			statusCode:    429,
			responseBody:  "Rate limit exceeded",
			expectedError: true,
			errorContains: "HTTP 429",
		},
		{
			name:          "500 Internal Server Error",
			statusCode:    500,
			responseBody:  "Internal server error",
			expectedError: true,
			errorContains: "HTTP 500",
		},
		{
			name:          "200 Success",
			statusCode:    200,
			responseBody:  `{"message": "Success", "metadata": {}}`,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				fmt.Fprint(w, tt.responseBody)
			}))
			defer server.Close()

			// Set up environment
			os.Setenv("NIRMATA_API_KEY", "test-jwt-token")
			os.Setenv("NIRMATA_ENDPOINT", server.URL)
			defer func() {
				os.Unsetenv("NIRMATA_API_KEY")
				os.Unsetenv("NIRMATA_ENDPOINT")
			}()

			// Create client and send request
			ctx := context.Background()
			client, err := NewNirmataClient(ctx, ClientOptions{})
			if err != nil {
				t.Fatalf("error creating client: %v", err)
			}

			chat := client.StartChat("", "claude-sonnet-4")
			_, err = chat.Send(ctx, "Test message")

			if tt.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestNirmataCompletionResponse(t *testing.T) {
	// Create a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := nirmataChatResponse{
			Message: "This is a completion response",
			Metadata: map[string]any{
				"tokens_used": 10,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Set up environment
	os.Setenv("NIRMATA_API_KEY", "test-jwt-token")
	os.Setenv("NIRMATA_ENDPOINT", server.URL)
	defer func() {
		os.Unsetenv("NIRMATA_API_KEY")
		os.Unsetenv("NIRMATA_ENDPOINT")
	}()

	// Test completion interface
	ctx := context.Background()
	client, err := NewNirmataClient(ctx, ClientOptions{})
	if err != nil {
		t.Fatalf("error creating client: %v", err)
	}

	req := &CompletionRequest{
		Prompt: "Complete this sentence:",
		Model:  "claude-sonnet-4",
	}

	response, err := client.GenerateCompletion(ctx, req)
	if err != nil {
		t.Fatalf("error generating completion: %v", err)
	}

	if response.Response() != "This is a completion response" {
		t.Errorf("expected specific response text, got %q", response.Response())
	}

	metadata := response.UsageMetadata()
	if metadata == nil {
		t.Error("expected non-nil metadata")
	}
}

func TestNirmataProviderRegistration(t *testing.T) {
	// Test that we can create a client using the provider registry
	os.Setenv("NIRMATA_API_KEY", "test-jwt-token")
	os.Setenv("NIRMATA_ENDPOINT", "https://localhost:8443")
	defer func() {
		os.Unsetenv("NIRMATA_API_KEY")
		os.Unsetenv("NIRMATA_ENDPOINT")
	}()

	ctx := context.Background()
	client, err := NewClient(ctx, "nirmata")
	if err != nil {
		t.Fatalf("error creating client via registry: %v", err)
	}

	if client == nil {
		t.Error("expected non-nil client")
	}

	// Verify it's a NirmataClient
	if _, ok := client.(*NirmataClient); !ok {
		t.Errorf("expected NirmataClient, got %T", client)
	}
}

func TestNirmataListModels(t *testing.T) {
	// Set up environment
	os.Setenv("NIRMATA_API_KEY", "test-jwt-token")
	os.Setenv("NIRMATA_ENDPOINT", "https://localhost:8443")
	defer func() {
		os.Unsetenv("NIRMATA_API_KEY")
		os.Unsetenv("NIRMATA_ENDPOINT")
	}()

	ctx := context.Background()
	client, err := NewNirmataClient(ctx, ClientOptions{})
	if err != nil {
		t.Fatalf("error creating client: %v", err)
	}

	models, err := client.ListModels(ctx)
	if err != nil {
		t.Errorf("error listing models: %v", err)
	}

	expectedModels := []string{
		"us.anthropic.claude-sonnet-4-20250514-v1:0",
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
	}

	if len(models) != len(expectedModels) {
		t.Errorf("expected %d models, got %d", len(expectedModels), len(models))
	}

	for i, model := range models {
		if i < len(expectedModels) && model != expectedModels[i] {
			t.Errorf("expected model %d to be %q, got %q", i, expectedModels[i], model)
		}
	}
}

func TestNirmataSetResponseSchema(t *testing.T) {
	// Set up environment
	os.Setenv("NIRMATA_API_KEY", "test-jwt-token")
	os.Setenv("NIRMATA_ENDPOINT", "https://localhost:8443")
	defer func() {
		os.Unsetenv("NIRMATA_API_KEY")
		os.Unsetenv("NIRMATA_ENDPOINT")
	}()

	ctx := context.Background()
	client, err := NewNirmataClient(ctx, ClientOptions{})
	if err != nil {
		t.Fatalf("error creating client: %v", err)
	}

	// Should return error as schema is not supported
	err = client.SetResponseSchema(&Schema{Type: TypeString})
	if err == nil {
		t.Error("expected error for unsupported schema")
	}

	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected error to mention not supported, got %q", err.Error())
	}
}