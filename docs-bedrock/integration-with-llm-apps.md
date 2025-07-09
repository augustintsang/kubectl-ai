I'll create a comprehensive guide for implementing usage metrics and inference configurations in your kubectl-ai/gollm repository. This guide will help Cursor understand the requirements and implementation approach.

I've created a comprehensive implementation guide that you can share with Cursor in your kubectl-ai repository. This guide provides:

# Implementation Guide: Usage Metrics & Inference Configuration for kubectl-ai/gollm

## Context & Integration Requirements

This guide provides implementation instructions for adding usage metrics and inference configuration support to the `kubectl-ai/gollm` package, specifically for integration with the `go-llm-apps` repository.

### Integration Points in go-llm-apps

The `go-llm-apps` repository expects the following integration points:

1. **Handler Level** (`cmd/webserver/handlers.go`): Creates `gollm.Client` with configuration
2. **Agent Level** (`pkg/agent/conversation.go`): Processes streaming responses and extracts usage
3. **Application Level** (`pkg/apps/`): Aggregates usage data across conversations

### Required Interface Enhancements

## 1. Client Options Pattern

Implement a flexible options pattern for `gollm.NewClient()`:

```go
// ClientOption defines configuration options for the gollm client
type ClientOption func(*ClientConfig) error

// ClientConfig holds configuration for the gollm client
type ClientConfig struct {
    InferenceConfig *InferenceConfig
    UsageCallback   UsageCallback
    Debug          bool
    SkipSSLVerify  bool
    // Add other configuration options as needed
}

// WithInferenceConfig sets the inference configuration
func WithInferenceConfig(config *InferenceConfig) ClientOption {
    return func(c *ClientConfig) error {
        c.InferenceConfig = config
        return nil
    }
}

// WithUsageCallback sets a callback for usage metrics
func WithUsageCallback(callback UsageCallback) ClientOption {
    return func(c *ClientConfig) error {
        c.UsageCallback = callback
        return nil
    }
}

// WithDebug enables debug mode
func WithDebug(debug bool) ClientOption {
    return func(c *ClientConfig) error {
        c.Debug = debug
        return nil
    }
}

// WithSkipVerifySSL skips SSL verification
func WithSkipVerifySSL() ClientOption {
    return func(c *ClientConfig) error {
        c.SkipSSLVerify = true
        return nil
    }
}
```

## 2. Usage Metrics Types

Define comprehensive usage tracking structures:

```go
// Usage represents token usage and cost information
type Usage struct {
    InputTokens  int     `json:"inputTokens,omitempty"`
    OutputTokens int     `json:"outputTokens,omitempty"`
    TotalTokens  int     `json:"totalTokens,omitempty"`
    InputCost    float64 `json:"inputCost,omitempty"`
    OutputCost   float64 `json:"outputCost,omitempty"`
    TotalCost    float64 `json:"totalCost,omitempty"`
    Model        string  `json:"model,omitempty"`
    Provider     string  `json:"provider,omitempty"`
    Timestamp    time.Time `json:"timestamp,omitempty"`
}

// UsageCallback is called when usage data is available
type UsageCallback func(providerName string, model string, usage Usage)

// UsageMetadata interface for extracting usage from responses
type UsageMetadata interface {
    GetUsage() *Usage
    GetModel() string
    GetProvider() string
}
```

## 3. Inference Configuration

Define configuration structure compatible with go-llm-apps:

```go
// InferenceConfig matches the structure expected by go-llm-apps
type InferenceConfig struct {
    Model       string  `json:"model" yaml:"model"`
    Region      string  `json:"region" yaml:"region"`
    Temperature float32 `json:"temperature" yaml:"temperature"`
    MaxTokens   int32   `json:"maxTokens" yaml:"maxTokens"`
    TopP        float32 `json:"topP" yaml:"topP"`
    TopK        int32   `json:"topK" yaml:"topK"`
    MaxRetries  int     `json:"maxRetries" yaml:"maxRetries"`
}

// DefaultInferenceConfig provides sensible defaults
func DefaultInferenceConfig() *InferenceConfig {
    return &InferenceConfig{
        Temperature: 0.1,
        MaxTokens:   64000,
        TopP:        0.1,
        TopK:        1,
        MaxRetries:  10,
    }
}
```

## 4. Enhanced Client Interface

Update the main client creation function:

```go
// NewClient creates a new gollm client with the specified provider and options
func NewClient(ctx context.Context, providerName string, opts ...ClientOption) (Client, error) {
    // Initialize default configuration
    config := &ClientConfig{
        InferenceConfig: DefaultInferenceConfig(),
        Debug:          false,
        SkipSSLVerify:  false,
    }
    
    // Apply provided options
    for _, opt := range opts {
        if err := opt(config); err != nil {
            return nil, fmt.Errorf("failed to apply client option: %w", err)
        }
    }
    
    // Get the provider factory
    factory, exists := providers[providerName]
    if !exists {
        return nil, fmt.Errorf("unknown provider: %s", providerName)
    }
    
    // Create client with configuration
    client, err := factory(ctx, *config)
    if err != nil {
        return nil, fmt.Errorf("failed to create client for provider %s: %w", providerName, err)
    }
    
    return client, nil
}
```

## 5. Enhanced Response Interfaces

Update response interfaces to include usage metadata:

```go
// ChatResponse must include usage metadata
type ChatResponse interface {
    Candidates() []Candidate
    UsageMetadata() UsageMetadata  // Add this method
}

// CompletionResponse must include usage metadata  
type CompletionResponse interface {
    Text() string
    UsageMetadata() UsageMetadata  // Add this method
}
```

## 6. Bedrock Provider Implementation

### Client Implementation

```go
// BedrockClient implementation with usage tracking
type BedrockClient struct {
    client        *bedrockruntime.Client
    config        ClientConfig
    usageCallback UsageCallback
}

func NewBedrockClient(ctx context.Context, config ClientConfig) (*BedrockClient, error) {
    // Initialize AWS Bedrock client
    awsConfig, err := awsconfig.LoadDefaultConfig(ctx, 
        awsconfig.WithRegion(config.InferenceConfig.Region))
    if err != nil {
        return nil, fmt.Errorf("failed to load AWS config: %w", err)
    }
    
    client := bedrockruntime.NewFromConfig(awsConfig)
    
    return &BedrockClient{
        client:        client,
        config:        config,
        usageCallback: config.UsageCallback,
    }, nil
}
```

### Usage Extraction Implementation

```go
// BedrockChatResponse implementation with usage tracking
type BedrockChatResponse struct {
    content     string
    usage       *Usage
    model       string
    provider    string
    candidates  []Candidate
}

func (r *BedrockChatResponse) UsageMetadata() UsageMetadata {
    return &BedrockUsageMetadata{
        usage:    r.usage,
        model:    r.model,
        provider: r.provider,
    }
}

// BedrockUsageMetadata implements UsageMetadata interface
type BedrockUsageMetadata struct {
    usage    *Usage
    model    string
    provider string
}

func (u *BedrockUsageMetadata) GetUsage() *Usage {
    return u.usage
}

func (u *BedrockUsageMetadata) GetModel() string {
    return u.model
}

func (u *BedrockUsageMetadata) GetProvider() string {
    return u.provider
}
```

### Streaming Response with Usage

```go
// Enhanced streaming implementation with usage tracking
func (cs *bedrockChatSession) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
    // ... existing implementation ...
    
    return func(yield func(ChatResponse, error) bool) {
        var aggregatedUsage Usage
        
        for event := range stream.Events() {
            switch e := event.(type) {
            case *types.ConverseStreamOutputMemberContentBlockDelta:
                // Handle content streaming
                response := &BedrockChatResponse{
                    content:  deltaText,
                    usage:    &aggregatedUsage, // Updated incrementally
                    model:    cs.model,
                    provider: "bedrock",
                }
                
                if !yield(response, nil) {
                    return
                }
                
            case *types.ConverseStreamOutputMemberMetadata:
                // Extract usage from metadata
                if e.Value.Usage != nil {
                    usage := extractUsageFromBedrock(e.Value.Usage)
                    aggregatedUsage = *usage
                    
                    // Call usage callback if provided
                    if cs.usageCallback != nil {
                        cs.usageCallback("bedrock", cs.model, *usage)
                    }
                }
            }
        }
        
        // Final response with complete usage
        yield(nil, nil) // End of stream
    }, nil
}

// Helper function to extract usage from Bedrock response
func extractUsageFromBedrock(bedrockUsage *types.TokenUsage) *Usage {
    usage := &Usage{
        Timestamp: time.Now(),
        Provider:  "bedrock",
    }
    
    if bedrockUsage.InputTokens != nil {
        usage.InputTokens = int(*bedrockUsage.InputTokens)
    }
    
    if bedrockUsage.OutputTokens != nil {
        usage.OutputTokens = int(*bedrockUsage.OutputTokens)
    }
    
    usage.TotalTokens = usage.InputTokens + usage.OutputTokens
    
    // Calculate costs based on model pricing
    // You'll need to implement cost calculation based on your model pricing
    usage.InputCost = calculateInputCost(usage.InputTokens, model)
    usage.OutputCost = calculateOutputCost(usage.OutputTokens, model)
    usage.TotalCost = usage.InputCost + usage.OutputCost
    
    return usage
}
```

## 7. Configuration Application

Ensure inference configuration is properly applied:

```go
// Apply inference configuration to Bedrock requests
func (cs *bedrockChatSession) prepareConverseInput(contents []any) (*bedrockruntime.ConverseInput, error) {
    input := &bedrockruntime.ConverseInput{
        ModelId: aws.String(cs.model),
        Messages: messages,
    }
    
    // Apply inference configuration
    if cs.config.InferenceConfig != nil {
        cfg := cs.config.InferenceConfig
        
        input.InferenceConfig = &types.InferenceConfiguration{
            MaxTokens:   aws.Int32(cfg.MaxTokens),
            Temperature: aws.Float32(cfg.Temperature),
            TopP:        aws.Float32(cfg.TopP),
        }
        
        // Note: TopK may not be available for all models
        if cfg.TopK > 0 {
            input.InferenceConfig.TopK = aws.Int32(cfg.TopK)
        }
    }
    
    return input, nil
}
```

## 8. Testing Requirements

Create comprehensive tests for the new functionality:

```go
func TestUsageMetricsIntegration(t *testing.T) {
    ctx := context.Background()
    
    var capturedUsage *Usage
    usageCallback := func(provider, model string, usage Usage) {
        capturedUsage = &usage
    }
    
    client, err := NewClient(ctx, "bedrock", 
        WithInferenceConfig(&InferenceConfig{
            Model: "anthropic.claude-3-sonnet-20240229-v1:0",
            Temperature: 0.7,
            MaxTokens: 1000,
        }),
        WithUsageCallback(usageCallback))
    
    require.NoError(t, err)
    
    // Test chat completion
    chat := client.StartChat("You are a helpful assistant", "anthropic.claude-3-sonnet-20240229-v1:0")
    response, err := chat.Send(ctx, "Hello!")
    
    require.NoError(t, err)
    require.NotNil(t, response.UsageMetadata())
    require.NotNil(t, capturedUsage)
    require.Greater(t, capturedUsage.TotalTokens, 0)
}
```

## 9. Implementation Checklist

When implementing this in kubectl-ai/gollm:

- [ ] Add ClientOption pattern with inference config and usage callback support
- [ ] Enhance ChatResponse and CompletionResponse interfaces with UsageMetadata()
- [ ] Implement Usage and UsageMetadata types
- [ ] Update NewClient() to accept and apply options
- [ ] Modify bedrock provider to extract and report usage from streaming responses
- [ ] Apply inference configuration to Bedrock API calls
- [ ] Add cost calculation logic for different models
- [ ] Create comprehensive tests for usage tracking and configuration
- [ ] Update documentation and examples
- [ ] Ensure backward compatibility with existing code

## 10. Integration Verification

After implementation, verify integration with go-llm-apps:

1. **Handler Integration**: Confirm `gollm.NewClient()` accepts inference config
2. **Usage Collection**: Verify usage data flows through streaming responses
3. **Configuration Application**: Test that inference parameters affect model behavior
4. **Cost Tracking**: Validate cost calculations are accurate
5. **Error Handling**: Ensure graceful degradation when usage data unavailable

This implementation will provide seamless integration between your kubectl-ai/gollm bedrock provider and the go-llm-apps framework while maintaining clean separation of concerns and comprehensive usage tracking. 

## Key Features of the Guide:

1. **Context Integration** - Explains exactly how your go-llm-apps codebase expects to interact with the gollm package
2. **Client Options Pattern** - Shows how to implement flexible configuration using functional options
3. **Usage Metrics Architecture** - Defines interfaces and types for comprehensive usage tracking
4. **Inference Configuration** - Provides the exact structure that matches your go-llm-apps expectations
5. **Concrete Implementation Examples** - Shows specific code for bedrock provider integration
6. **Testing Strategy** - Includes test examples to verify everything works
7. **Implementation Checklist** - Clear steps for Cursor to follow

## How to Use This Guide:

1. **Copy the guide** to your kubectl-ai repository (in the `/gollm` folder)
2. **Share it with Cursor** when asking for implementation of usage metrics and inference config
3. **Reference specific sections** when you need particular features implemented

The guide is designed to give Cursor all the context it needs to:
- Understand how the integration should work
- Implement the correct interfaces and types
- Ensure compatibility with your go-llm-apps codebase
- Add comprehensive usage tracking with cost calculation
- Apply inference configuration to actual API calls
- Create proper tests for verification

This approach will ensure that your bedrock provider in kubectl-ai/gollm integrates seamlessly with the agent framework and applications in your go-llm-apps repository, with full usage metrics and configuration support.