# Nirmata gollm Provider Implementation Guide (Revised)

## Overview

This document provides a comprehensive guide for implementing a "nirmata" provider in the kubectl-ai/gollm library. The Nirmata provider acts as a thin HTTP client wrapper that communicates with the `/chat` API endpoint in go-llm-apps.

## Key Design Principles

1. **Thin HTTP Wrapper**: The provider is a minimal translation layer between gollm's interface and the `/chat` endpoint
2. **Simple Message Format**: Uses `{role, content}` JSON format, not AWS Bedrock types
3. **Stateless Design**: Matches the `/chat` endpoint's stateless architecture
4. **Pattern Consistency**: Follows established gollm patterns (especially llamacpp for HTTP)

## Architecture Flow

```
Client Application
    ↓ (uses gollm interface)
gollm "nirmata" provider (thin HTTP wrapper)
    ↓ (HTTP POST with simple JSON)
go-llm-apps /chat endpoint (stateless processor)
    ↓ (converts to single prompt string)
gollm "bedrock" provider
    ↓ (AWS SDK with complex types)
AWS Bedrock Converse API
```

## Implementation File

Create `nirmata.go` in the gollm repository:

```go
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

// Environment variables (package-level, following gollm pattern)
var (
    nirmataAPIKey   string
    nirmataEndpoint string
    nirmataBaseURL  string
    nirmataModel    string
)

func init() {
    // Load environment variables
    nirmataAPIKey = os.Getenv("NIRMATA_API_KEY")
    nirmataEndpoint = os.Getenv("NIRMATA_ENDPOINT")
    nirmataBaseURL = os.Getenv("NIRMATA_BASE_URL")
    nirmataModel = os.Getenv("NIRMATA_MODEL")
    
    // Set defaults
    if nirmataModel == "" {
        nirmataModel = "claude-sonnet-4"
    }
    
    // Register provider
    if err := RegisterProvider("nirmata", newNirmataClientFactory); err != nil {
        klog.Fatalf("Failed to register nirmata provider: %v", err)
    }
}

func newNirmataClientFactory(ctx context.Context, opts ClientOptions) (Client, error) {
    return NewNirmataClient(ctx, opts)
}

// NirmataClient - thin HTTP client
type NirmataClient struct {
    baseURL    *url.URL
    httpClient *http.Client
    apiKey     string
}

// Simple message format matching /chat endpoint expectations
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

// nirmataChat manages conversation state client-side
type nirmataChat struct {
    client       *NirmataClient
    model        string
    systemPrompt string
    history      []nirmataMessage // Client-managed conversation history
    functionDefs []*FunctionDefinition
}

func NewNirmataClient(ctx context.Context, opts ClientOptions) (*NirmataClient, error) {
    // Validate API key
    apiKey := nirmataAPIKey
    if apiKey == "" {
        return nil, errors.New("NIRMATA_API_KEY environment variable required")
    }
    
    // Determine base URL (endpoint takes precedence)
    baseURLStr := nirmataEndpoint
    if baseURLStr == "" {
        baseURLStr = nirmataBaseURL
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
    
    // Set headers (NIRMATA-JWT format is verified in architecture doc)
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
    
    // For streaming responses, return the response object directly
    if resp == nil {
        return nil
    }
    
    // Parse JSON response
    return json.NewDecoder(httpResp.Body).Decode(resp)
}

// StartChat creates a new chat session
func (c *NirmataClient) StartChat(systemPrompt, model string) Chat {
    if model == "" {
        model = nirmataModel
    }
    
    chat := &nirmataChat{
        client:       c,
        model:        model,
        systemPrompt: systemPrompt,
        history:      []nirmataMessage{},
        functionDefs: nil,
    }
    
    // If system prompt provided, add it as first message
    // Note: /chat endpoint will include this in the flattened prompt
    if systemPrompt != "" {
        chat.history = append(chat.history, nirmataMessage{
            Role:    "system",
            Content: systemPrompt,
        })
    }
    
    return chat
}

// Initialize implements the Chat interface requirement
func (c *nirmataChat) Initialize(messages []*api.Message) error {
    // Convert api.Message history to our simple format
    c.history = []nirmataMessage{}
    
    // Re-add system prompt if exists
    if c.systemPrompt != "" {
        c.history = append(c.history, nirmataMessage{
            Role:    "system",
            Content: c.systemPrompt,
        })
    }
    
    // Convert provided messages
    for _, msg := range messages {
        if msg.Text != "" {
            role := "user"
            if msg.Role != "" {
                role = msg.Role
            }
            c.history = append(c.history, nirmataMessage{
                Role:    role,
                Content: msg.Text,
            })
        }
    }
    
    return nil
}

// Send implements synchronous message sending
func (c *nirmataChat) Send(ctx context.Context, contents ...any) (ChatResponse, error) {
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
    
    // Return response implementing ChatResponse interface
    return &nirmataChatResponseWrapper{
        text:     resp.Message,
        metadata: resp.Metadata,
    }, nil
}

// SendStreaming implements streaming message sending
func (c *nirmataChat) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
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
    return &nirmataStreamIterator{
        response: httpResp,
        scanner:  bufio.NewScanner(httpResp.Body),
        chat:     c,
        buffer:   &strings.Builder{},
    }, nil
}

// SetFunctionDefinitions for future function calling support
func (c *nirmataChat) SetFunctionDefinitions(functionDefinitions []*FunctionDefinition) error {
    c.functionDefs = functionDefinitions
    // Note: Function calling would require /chat endpoint support
    return nil
}

// IsRetryableError determines if an error should be retried
func (c *nirmataChat) IsRetryableError(err error) bool {
    return DefaultIsRetryableError(err)
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
            contentStr.WriteString(v.Text)
        default:
            contentStr.WriteString(fmt.Sprintf("%v", v))
        }
    }
    
    return nirmataMessage{
        Role:    "user",
        Content: contentStr.String(),
    }
}

// Response wrapper implementing ChatResponse interface
type nirmataChatResponseWrapper struct {
    text     string
    metadata any
}

func (r *nirmataChatResponseWrapper) UsageMetadata() any {
    return r.metadata
}

func (r *nirmataChatResponseWrapper) Candidates() []Candidate {
    return []Candidate{&nirmataCandidate{text: r.text}}
}

// Candidate implementation
type nirmataCandidate struct {
    text string
}

func (c *nirmataCandidate) String() string {
    return c.text
}

func (c *nirmataCandidate) Parts() []Part {
    return []Part{&nirmataTextPart{text: c.text}}
}

// Part implementation
type nirmataTextPart struct {
    text string
}

func (p *nirmataTextPart) AsText() (string, bool) {
    return p.text, true
}

func (p *nirmataTextPart) AsFunctionCalls() ([]FunctionCall, bool) {
    return nil, false
}

// Streaming iterator implementation
type nirmataStreamIterator struct {
    response *http.Response
    scanner  *bufio.Scanner
    chat     *nirmataChat
    buffer   *strings.Builder
    current  ChatResponse
    err      error
}

func (i *nirmataStreamIterator) Next() bool {
    if i.scanner.Scan() {
        chunk := i.scanner.Text()
        i.buffer.WriteString(chunk)
        i.current = &nirmataChatResponseWrapper{
            text: chunk,
        }
        return true
    }
    
    if err := i.scanner.Err(); err != nil {
        i.err = err
        return false
    }
    
    // Update chat history with complete response
    if i.buffer.Len() > 0 {
        i.chat.history = append(i.chat.history, nirmataMessage{
            Role:    "assistant",
            Content: i.buffer.String(),
        })
    }
    
    i.response.Body.Close()
    return false
}

func (i *nirmataStreamIterator) Value() ChatResponse {
    return i.current
}

func (i *nirmataStreamIterator) Error() error {
    return i.err
}

// Additional Client interface methods

func (c *NirmataClient) GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error) {
    // Convert completion to chat format
    chat := c.StartChat("", req.Model)
    response, err := chat.Send(ctx, req.Prompt)
    if err != nil {
        return nil, err
    }
    
    // Extract text from first candidate
    candidates := response.Candidates()
    if len(candidates) == 0 {
        return nil, fmt.Errorf("no candidates in response")
    }
    
    text := candidates[0].String()
    
    return &nirmataCompletionResponse{
        text:     text,
        metadata: response.UsageMetadata(),
    }, nil
}

type nirmataCompletionResponse struct {
    text     string
    metadata any
}

func (r *nirmataCompletionResponse) Response() string {
    return r.text
}

func (r *nirmataCompletionResponse) UsageMetadata() any {
    return r.metadata
}

func (c *NirmataClient) SetResponseSchema(schema *Schema) error {
    // Schema not supported by /chat endpoint
    return errors.New("response schema not supported by Nirmata provider")
}

func (c *NirmataClient) ListModels(ctx context.Context) ([]string, error) {
    // Return models supported by /chat endpoint
    return []string{
        "claude-sonnet-4",
        "us.anthropic.claude-sonnet-4-20250514-v1:0",
    }, nil
}

func (c *NirmataClient) Close() error {
    // No resources to clean up
    return nil
}
```

## Key Implementation Decisions

### 1. **Simple Message Types**
- Uses `{role, content}` format that matches `/chat` endpoint expectations
- NOT using AWS Bedrock types since the endpoint flattens to string anyway
- Maintains proper abstraction between HTTP API and provider implementation

### 2. **HTTP Pattern from llamacpp**
- Clean `doRequest` method for all HTTP operations
- Consistent error handling with APIError
- Proper context propagation

### 3. **Client-Side State Management**
- Provider maintains conversation history (since `/chat` is stateless)
- Implements `Initialize()` method as required by gollm interface
- Updates history after each successful response

### 4. **Streaming Support**
- Handles chunked HTTP responses from `/chat` endpoint
- Accumulates complete response for history updates
- Returns chunks as they arrive for real-time display

### 5. **Authentication**
- Uses verified `NIRMATA-JWT` header format
- JWT token from environment variable
- No token refresh (handled externally)

## Environment Configuration

```bash
# Required
export NIRMATA_API_KEY="your_jwt_token"
export NIRMATA_ENDPOINT="https://api.nirmata.com:8443"  # OR
export NIRMATA_BASE_URL="https://localhost:8443"

# Optional
export NIRMATA_MODEL="claude-sonnet-4"
export LLM_SKIP_VERIFY_SSL=true  # For development
```

## Usage Examples

### Basic Chat
```go
client, err := gollm.NewClient(ctx, "nirmata")
if err != nil {
    log.Fatal(err)
}
defer client.Close()

chat := client.StartChat("You are a helpful assistant", "claude-sonnet-4")
response, err := chat.Send(ctx, "What is the capital of France?")
if err != nil {
    log.Fatal(err)
}

candidates := response.Candidates()
if len(candidates) > 0 {
    fmt.Println(candidates[0].String())
}
```

### Streaming Response
```go
chat := client.StartChat("", "claude-sonnet-4")
iter, err := chat.SendStreaming(ctx, "Tell me a story")
if err != nil {
    log.Fatal(err)
}

for iter.Next() {
    candidates := iter.Value().Candidates()
    if len(candidates) > 0 {
        fmt.Print(candidates[0].String())
    }
}

if err := iter.Error(); err != nil {
    log.Printf("Streaming error: %v", err)
}
```

### With Conversation History
```go
chat := client.StartChat("You are a math tutor", "claude-sonnet-4")

// First message
resp1, _ := chat.Send(ctx, "What is 2+2?")
candidates1 := resp1.Candidates()
if len(candidates1) > 0 {
    fmt.Println(candidates1[0].String())
}

// Follow-up (history maintained by provider)
resp2, _ := chat.Send(ctx, "Why?")
candidates2 := resp2.Candidates()
if len(candidates2) > 0 {
    fmt.Println(candidates2[0].String())
}
```

## Testing Checklist

### Unit Tests
- [ ] Environment variable loading
- [ ] URL construction with model parameter
- [ ] JWT header format
- [ ] Message conversion logic
- [ ] History management

### Integration Tests
- [ ] Connect to running `/chat` endpoint
- [ ] Verify streaming works
- [ ] Test conversation continuity
- [ ] Error handling (401, 429, 500)
- [ ] Model parameter passing

### Edge Cases
- [ ] Empty messages
- [ ] Long conversations (history truncation)
- [ ] Network interruptions during streaming
- [ ] Invalid JWT tokens
- [ ] Rate limiting

## Important Notes

1. **This is a thin wrapper** - The provider doesn't do complex processing, just translates between gollm interface and `/chat` HTTP API

2. **Stateless endpoint** - The `/chat` endpoint doesn't maintain state, so the provider manages conversation history client-side

3. **Simple format** - Uses simple `{role, content}` JSON, not AWS Bedrock types, since the endpoint flattens everything to a string prompt

4. **No function calling** - Currently out of scope, but structure allows future addition

5. **Authentication** - JWT tokens must be obtained through Nirmata's standard auth flow

## Success Criteria

✅ Provider registers successfully with `gollm.NewClient(ctx, "nirmata")`  
✅ Sends simple message format to `/chat` endpoint  
✅ Handles streaming and non-streaming responses  
✅ Maintains conversation history client-side  
✅ Proper error handling and retry logic  
✅ Environment-based configuration  
✅ Model parameter support  

This implementation provides a clean, minimal translation layer between gollm's interface and the `/chat` endpoint, avoiding unnecessary complexity while maintaining full functionality.