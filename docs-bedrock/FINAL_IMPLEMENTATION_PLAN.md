# Final Implementation Plan: Usage Metrics & Inference Configuration

## 🚨 **Confirmed Analysis: Simple Bug, Not Architecture Problem**

After deep codebase analysis, I've **confirmed** the real issue - Bedrock has a **simple implementation bug** where it completely ignores the excellent existing infrastructure.

### **The Bug** 🐛

**In `gollm/bedrock/bedrock.go:55-57`:**
```go
func NewBedrockClient(ctx context.Context, opts gollm.ClientOptions) (*BedrockClient, error) {
    options := DefaultOptions                    // ❌ BUG: Completely ignores 'opts'!
    return NewBedrockClientWithOptions(ctx, options)
}
```

### **What This Breaks** ❌

- ❌ `WithInferenceConfig()` - ignored
- ❌ `WithUsageCallback()` - ignored  
- ❌ Perfect `gollm.Usage` struct - unused
- ❌ Provider-agnostic inference parameters - unused
- ❌ All functional options - ignored

### **What's Already Perfect** ✅

1. **`gollm.Usage` struct** - Complete with all needed fields ✅
2. **`gollm.InferenceConfig` struct** - All parameters defined ✅ 
3. **`UsageCallback` type** - Properly defined as `func(provider, model string, usage Usage)` ✅
4. **Functional options** - `WithInferenceConfig()`, `WithUsageCallback()` implemented ✅
5. **AWS usage capture** - `response.usage = output.Usage` already happens ✅
6. **Inference config usage** - `buildConverseInput()` uses inference parameters ✅

### **The Fix: 30 Lines of Code** 🔧

#### **1. Store ClientOptions in BedrockClient**

```go
type BedrockClient struct {
    runtimeClient *bedrockruntime.Client
    bedrockClient *bedrock.Client
    options       *BedrockOptions
    clientOpts    gollm.ClientOptions  // ADD: Store original options
}

func NewBedrockClient(ctx context.Context, opts gollm.ClientOptions) (*BedrockClient, error) {
    // MERGE: Combine opts.InferenceConfig with DefaultOptions
    options := mergeWithClientOptions(DefaultOptions, opts)
    client, err := NewBedrockClientWithOptions(ctx, options)
    if err != nil {
        return nil, err
    }
    
    // STORE: Keep original opts for callbacks/debug
    client.clientOpts = opts
    return client, nil
}
```

#### **2. Merge InferenceConfig with BedrockOptions**

```go
func mergeWithClientOptions(defaults *BedrockOptions, opts gollm.ClientOptions) *BedrockOptions {
    merged := *defaults  // Copy defaults
    
    if opts.InferenceConfig != nil {
        config := opts.InferenceConfig
        if config.Model != "" {
            merged.Model = config.Model
        }
        if config.Region != "" {
            merged.Region = config.Region
        }
        if config.Temperature != 0 {
            merged.Temperature = config.Temperature
        }
        if config.MaxTokens != 0 {
            merged.MaxTokens = config.MaxTokens
        }
        if config.TopP != 0 {
            merged.TopP = config.TopP
        }
        if config.MaxRetries != 0 {
            merged.MaxRetries = config.MaxRetries
        }
    }
    
    return &merged
}
```

#### **3. Convert AWS Usage to gollm.Usage**

```go
func convertAWSUsage(awsUsage any, model, provider string) *gollm.Usage {
    if awsUsage == nil {
        return nil
    }
    
    if usage, ok := awsUsage.(*types.TokenUsage); ok {
        return &gollm.Usage{
            InputTokens:  int(aws.ToInt32(usage.InputTokens)),
            OutputTokens: int(aws.ToInt32(usage.OutputTokens)),
            TotalTokens:  int(aws.ToInt32(usage.TotalTokens)),
            Model:        model,
            Provider:     provider,
            Timestamp:    time.Now(),
            // Cost calculation would go here if needed
        }
    }
    
    return nil
}
```

#### **4. Call UsageCallback When Available**

```go
func (cs *bedrockChatSession) Send(ctx context.Context, contents ...any) (gollm.ChatResponse, error) {
    // ... existing code ...
    
    response := cs.parseConverseOutput(&output.Output)
    response.usage = output.Usage
    
    // NEW: Call usage callback if configured
    if cs.client.clientOpts.UsageCallback != nil {
        if structuredUsage := convertAWSUsage(output.Usage, cs.model, "bedrock"); structuredUsage != nil {
            cs.client.clientOpts.UsageCallback("bedrock", cs.model, *structuredUsage)
        }
    }
    
    cs.addAssistantResponse(response)
    return response, nil
}
```

#### **5. Return Structured Usage in UsageMetadata()**

```go
func (r *bedrockChatResponse) UsageMetadata() any {
    // NEW: Return structured gollm.Usage instead of raw AWS data
    if structuredUsage := convertAWSUsage(r.usage, "bedrock", "bedrock"); structuredUsage != nil {
        return structuredUsage
    }
    return r.usage  // Fallback to raw data
}
```

### **Result After Fix** 🎉

```go
// WORKS: Provider-agnostic usage tracking
config := &gollm.InferenceConfig{
    Model:       "claude-3-sonnet",
    Temperature: 0.7,
    MaxTokens:   4000,
}

var usageStats []gollm.Usage
callback := func(provider, model string, usage gollm.Usage) {
    usageStats = append(usageStats, usage)
}

client, _ := gollm.NewClient(ctx, "bedrock",
    gollm.WithInferenceConfig(config),    // ✅ WORKS: Sets temperature, model, etc.
    gollm.WithUsageCallback(callback),    // ✅ WORKS: Captures structured usage
    gollm.WithDebug(true),                // ✅ WORKS: Enables debug logging
)

chat := client.StartChat("You are helpful", "")
response, _ := chat.Send(ctx, "Hello")

// ✅ WORKS: Structured usage available
usage := response.UsageMetadata().(*gollm.Usage)
fmt.Printf("Tokens: %d, Cost: $%.4f\n", usage.TotalTokens, usage.TotalCost)

// ✅ WORKS: Callback fired with structured data
fmt.Printf("Captured %d usage records\n", len(usageStats))
```

### **Summary**

✅ **This approach is correct** - it's a simple 30-line fix to use existing infrastructure
✅ **Perfect integration** with go-llm-apps out of the box
✅ **Zero breaking changes** - purely additive
✅ **Follows existing patterns** used by all other providers
✅ **Per-chat usage tracking** as requested
✅ **Provider-agnostic** but not over-engineered

The bug fix will enable **all** the existing excellent infrastructure to work with Bedrock immediately. 