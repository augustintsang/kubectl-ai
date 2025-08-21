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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/GoogleCloudPlatform/kubectl-ai/pkg/api"
	"k8s.io/klog/v2"
)

// Register the Nirmata provider factory on package initialization
func init() {
	if err := RegisterProvider("nirmata", newNirmataClientFactory); err != nil {
		klog.Fatalf("Failed to register nirmata provider: %v", err)
	}
}

// newNirmataClientFactory creates a new Nirmata client with the given options
func newNirmataClientFactory(ctx context.Context, opts ClientOptions) (Client, error) {
	return NewNirmataClient(ctx, opts)
}

// NirmataClient implements the gollm.Client interface for Nirmata models via HTTP
type NirmataClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	apiKey     string
}

// Ensure NirmataClient implements the Client interface
var _ Client = &NirmataClient{}

// NewNirmataClient creates a new client for interacting with Nirmata models
func NewNirmataClient(ctx context.Context, opts ClientOptions) (*NirmataClient, error) {
	// Validate API key
	apiKey := os.Getenv("NIRMATA_API_KEY")
	if apiKey == "" {
		return nil, errors.New("NIRMATA_API_KEY environment variable required")
	}

	// Determine base URL (endpoint takes precedence)
	baseURLStr := os.Getenv("NIRMATA_ENDPOINT")
	if baseURLStr == "" {
		baseURLStr = os.Getenv("NIRMATA_BASE_URL")
	}
	if baseURLStr == "" {
		return nil, errors.New("NIRMATA_ENDPOINT or NIRMATA_BASE_URL environment variable required")
	}

	baseURL, err := url.Parse(baseURLStr)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}

	// Create HTTP client with SSL configuration
	httpClient := createCustomHTTPClient(opts.SkipVerifySSL)

	return &NirmataClient{
		baseURL:    baseURL,
		httpClient: httpClient,
		apiKey:     apiKey,
	}, nil
}

// Close cleans up any resources used by the client
func (c *NirmataClient) Close() error {
	return nil
}

// StartChat starts a new chat session with the specified system prompt and model
func (c *NirmataClient) StartChat(systemPrompt, model string) Chat {
	selectedModel := getNirmataModel(model)

	chat := &nirmataChat{
		client:       c,
		systemPrompt: systemPrompt,
		model:        selectedModel,
		history:      []nirmataMessage{},
	}

	// Add system prompt to history immediately if provided (unlike Bedrock, Nirmata needs it in message array)
	if systemPrompt != "" {
		chat.history = append(chat.history, nirmataMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	return chat
}

// GenerateCompletion generates a single completion for the given request
func (c *NirmataClient) GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error) {
	chat := c.StartChat("", req.Model)
	chatResponse, err := chat.Send(ctx, req.Prompt)
	if err != nil {
		return nil, err
	}

	// Wrap ChatResponse in a CompletionResponse
	return &nirmataCompletionResponse{
		chatResponse: chatResponse,
	}, nil
}

// SetResponseSchema sets the response schema for the client (not supported by Nirmata)
func (c *NirmataClient) SetResponseSchema(schema *Schema) error {
	return fmt.Errorf("response schema not supported by Nirmata")
}

// ListModels returns the list of supported Nirmata models
func (c *NirmataClient) ListModels(ctx context.Context) ([]string, error) {
	return []string{
		"claude-sonnet-4",
		"us.anthropic.claude-sonnet-4-20250514-v1:0",
	}, nil
}

// nirmataChat implements the Chat interface for Nirmata conversations
type nirmataChat struct {
	client       *NirmataClient
	systemPrompt string
	model        string
	history      []nirmataMessage
	functionDefs []*FunctionDefinition
}

// Simple message format for HTTP API
type nirmataMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type nirmataChatRequest struct {
	Messages []nirmataMessage `json:"messages"`
}

type nirmataChatResponse struct {
	Message  string `json:"message"`
	Metadata any    `json:"metadata,omitempty"`
}

func (c *nirmataChat) Initialize(history []*api.Message) error {
	c.history = make([]nirmataMessage, 0, len(history))

	// Add system prompt if exists
	if c.systemPrompt != "" {
		c.history = append(c.history, nirmataMessage{
			Role:    "system",
			Content: c.systemPrompt,
		})
	}

	for _, msg := range history {
		// Convert api.Message to nirmataMessage
		role := "user"
		switch msg.Source {
		case api.MessageSourceUser:
			role = "user"
		case api.MessageSourceModel, api.MessageSourceAgent:
			role = "assistant"
		default:
			// Skip unknown message sources
			continue
		}

		// Convert payload to string content
		var content string
		if msg.Type == api.MessageTypeText && msg.Payload != nil {
			if textPayload, ok := msg.Payload.(string); ok {
				content = textPayload
			} else {
				// Try to convert other types to string
				content = fmt.Sprintf("%v", msg.Payload)
			}
		} else {
			// Skip non-text messages for now
			continue
		}

		if content == "" {
			continue
		}

		nirmataMsg := nirmataMessage{
			Role:    role,
			Content: content,
		}

		c.history = append(c.history, nirmataMsg)
	}

	return nil
}

// Send sends a message to the chat and returns the response
func (c *nirmataChat) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
	if len(contents) == 0 {
		return nil, errors.New("no content provided")
	}

	// Convert contents to user message
	userMessage := c.convertContentsToMessage(contents)

	// Build complete message history (client manages state)
	messages := append(c.history, userMessage)

	// Create request
	req := nirmataChatRequest{Messages: messages}

	// Execute request with model parameter
	endpoint := fmt.Sprintf("chat?model=%s", c.model)
	var resp nirmataChatResponse
	if err := c.client.doRequest(ctx, endpoint, req, &resp); err != nil {
		return nil, err
	}

	// Update conversation history
	c.history = append(c.history, userMessage)
	c.history = append(c.history, nirmataMessage{
		Role:    "assistant",
		Content: resp.Message,
	})

	// Extract response content
	response := &nirmataResponse{
		message:  resp.Message,
		metadata: resp.Metadata,
		model:    c.model,
	}

	return response, nil
}

// SendStreaming sends a message and returns a streaming response
func (c *nirmataChat) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	if len(contents) == 0 {
		return nil, errors.New("no content provided")
	}

	// Convert contents to user message
	userMessage := c.convertContentsToMessage(contents)

	// Build complete message history
	messages := append(c.history, userMessage)

	// Create request
	req := nirmataChatRequest{Messages: messages}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	// Build URL with model parameter
	endpoint := fmt.Sprintf("chat?model=%s", c.model)
	u := c.client.baseURL.JoinPath(endpoint)

	// Create streaming request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "NIRMATA-JWT "+c.client.apiKey)

	// Execute request
	httpResp, err := c.client.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		body, _ := io.ReadAll(httpResp.Body)
		return nil, &APIError{
			StatusCode: httpResp.StatusCode,
			Message:    fmt.Sprintf("HTTP %d", httpResp.StatusCode),
			Err:        fmt.Errorf("%s", body),
		}
	}

	// Update history with user message
	c.history = append(c.history, userMessage)

	// Return streaming iterator
	return func(yield func(ChatResponse, error) bool) {
		defer httpResp.Body.Close()

		var fullContent strings.Builder
		scanner := bufio.NewScanner(httpResp.Body)

		// Process streaming chunks
		for scanner.Scan() {
			chunk := scanner.Text()
			fullContent.WriteString(chunk)

			response := &nirmataStreamResponse{
				content: chunk,
				model:   c.model,
				done:    false,
			}

			if !yield(response, nil) {
				return
			}
		}

		// Update chat history with complete response
		if fullContent.Len() > 0 {
			c.history = append(c.history, nirmataMessage{
				Role:    "assistant",
				Content: fullContent.String(),
			})
		}

		// Check for scanner errors
		if err := scanner.Err(); err != nil {
			yield(nil, fmt.Errorf("stream error: %w", err))
		}
	}, nil
}

// convertContentsToMessage converts gollm contents to simple message
func (c *nirmataChat) convertContentsToMessage(contents []any) nirmataMessage {
	var contentStr strings.Builder

	for i, content := range contents {
		if i > 0 {
			contentStr.WriteString(" ")
		}

		switch v := content.(type) {
		case string:
			contentStr.WriteString(v)
		case *api.Message:
			if v.Type == api.MessageTypeText && v.Payload != nil {
				if textPayload, ok := v.Payload.(string); ok {
					contentStr.WriteString(textPayload)
				} else {
					contentStr.WriteString(fmt.Sprintf("%v", v.Payload))
				}
			}
		default:
			contentStr.WriteString(fmt.Sprintf("%v", v))
		}
	}

	return nirmataMessage{
		Role:    "user",
		Content: contentStr.String(),
	}
}

// doRequest follows llamacpp's clean HTTP pattern
func (c *NirmataClient) doRequest(ctx context.Context, endpoint string, req any, resp any) error {
	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	// Build URL
	u := c.baseURL.JoinPath(endpoint)

	// Create request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "NIRMATA-JWT "+c.apiKey)

	// Execute
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer httpResp.Body.Close()

	// Handle errors
	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return &APIError{
			StatusCode: httpResp.StatusCode,
			Message:    fmt.Sprintf("HTTP %d", httpResp.StatusCode),
			Err:        fmt.Errorf("%s", body),
		}
	}

	// Parse JSON response
	return json.NewDecoder(httpResp.Body).Decode(resp)
}

// SetFunctionDefinitions configures the available functions for tool use
func (c *nirmataChat) SetFunctionDefinitions(functions []*FunctionDefinition) error {
	c.functionDefs = functions
	// Function calling would require /chat endpoint support
	return nil
}

// IsRetryableError determines if an error is retryable
func (c *nirmataChat) IsRetryableError(err error) bool {
	return DefaultIsRetryableError(err)
}

// nirmataResponse implements ChatResponse for regular (non-streaming) responses
type nirmataResponse struct {
	message  string
	metadata any
	model    string
}

// UsageMetadata returns the usage metadata from the response
func (r *nirmataResponse) UsageMetadata() any {
	return r.metadata
}

// Candidates returns the candidate responses
func (r *nirmataResponse) Candidates() []Candidate {
	candidate := &nirmataCandidate{
		text:  r.message,
		model: r.model,
	}
	return []Candidate{candidate}
}

// nirmataStreamResponse implements ChatResponse for streaming responses
type nirmataStreamResponse struct {
	content string
	model   string
	done    bool
}

// UsageMetadata returns the usage metadata from the streaming response
func (r *nirmataStreamResponse) UsageMetadata() any {
	return nil // No usage metadata in streaming chunks
}

// Candidates returns the candidate responses for streaming
func (r *nirmataStreamResponse) Candidates() []Candidate {
	if r.content == "" {
		return []Candidate{}
	}

	candidate := &nirmataStreamCandidate{
		content: r.content,
		model:   r.model,
	}
	return []Candidate{candidate}
}

// nirmataCandidate implements Candidate for regular responses
type nirmataCandidate struct {
	text  string
	model string
}

// String returns a string representation of the candidate
func (c *nirmataCandidate) String() string {
	return c.text
}

// Parts returns the parts of the candidate
func (c *nirmataCandidate) Parts() []Part {
	return []Part{&nirmataTextPart{text: c.text}}
}

// nirmataStreamCandidate implements Candidate for streaming responses
type nirmataStreamCandidate struct {
	content string
	model   string
}

// String returns a string representation of the streaming candidate
func (c *nirmataStreamCandidate) String() string {
	return c.content
}

// Parts returns the parts of the streaming candidate
func (c *nirmataStreamCandidate) Parts() []Part {
	return []Part{&nirmataTextPart{text: c.content}}
}

// nirmataTextPart implements Part for text content
type nirmataTextPart struct {
	text string
}

// AsText returns the text content
func (p *nirmataTextPart) AsText() (string, bool) {
	return p.text, true
}

// AsFunctionCalls returns nil since this is a text part
func (p *nirmataTextPart) AsFunctionCalls() ([]FunctionCall, bool) {
	return nil, false
}

// Helper functions

// getNirmataModel returns the model to use, checking in order:
// 1. Explicitly provided model
// 2. Environment variable NIRMATA_MODEL
// 3. Default model (Claude Sonnet 4)
func getNirmataModel(model string) string {
	if model != "" {
		klog.V(2).Infof("Using explicitly provided model: %s", model)
		return model
	}

	if envModel := os.Getenv("NIRMATA_MODEL"); envModel != "" {
		klog.V(1).Infof("Using model from environment variable: %s", envModel)
		return envModel
	}

	defaultModel := "claude-sonnet-4"
	klog.V(1).Infof("Using default model: %s", defaultModel)
	return defaultModel
}

// nirmataCompletionResponse wraps a ChatResponse to implement CompletionResponse
type nirmataCompletionResponse struct {
	chatResponse ChatResponse
}

var _ CompletionResponse = (*nirmataCompletionResponse)(nil)

func (r *nirmataCompletionResponse) Response() string {
	if r.chatResponse == nil {
		return ""
	}
	candidates := r.chatResponse.Candidates()
	if len(candidates) == 0 {
		return ""
	}
	parts := candidates[0].Parts()
	for _, part := range parts {
		if text, ok := part.AsText(); ok {
			return text
		}
	}
	return ""
}

func (r *nirmataCompletionResponse) UsageMetadata() any {
	if r.chatResponse == nil {
		return nil
	}
	return r.chatResponse.UsageMetadata()
}