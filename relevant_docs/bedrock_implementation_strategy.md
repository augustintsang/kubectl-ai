# AWS Bedrock Implementation Strategy for kubectl-ai - HYBRID APPROACH

## Executive Summary

After analyzing **go-llm-apps dependencies** and **existing provider patterns**, this document presents a **hybrid approach** that supports BOTH self-contained Bedrock configuration AND backward compatibility with existing go-llm-apps usage. This eliminates global interface pollution while maintaining all existing functionality.

## Critical Discovery: Bedrock is the Outlier

### **🔍 Provider Pattern Analysis**
All other providers follow **identical self-contained patterns**:

```go
// OpenAI Pattern - Self-contained
var openAIAPIKey = os.Getenv("OPENAI_API_KEY")
var openAIModel = os.Getenv("OPENAI_MODEL")

func NewOpenAIClient(ctx context.Context, opts ClientOptions) (*OpenAIClient, error) {
    // Uses environment variables + opts.SkipVerifySSL only
    httpClient := createCustomHTTPClient(opts.SkipVerifySSL)
}

// Grok Pattern - Self-contained  
func NewGrokClient(ctx context.Context, opts ClientOptions) (*GrokClient, error) {
    apiKey := os.Getenv("GROK_API_KEY")
    endpoint := os.Getenv("GROK_ENDPOINT")
    httpClient := createCustomHTTPClient(opts.SkipVerifySSL)
}

// Ollama Pattern - Self-contained
func NewOllamaClient(ctx context.Context, opts ClientOptions) (*OllamaClient, error) {
    httpClient := createCustomHTTPClient(opts.SkipVerifySSL)
    client := api.NewClient(envconfig.Host(), httpClient)
}
```

### **🚫 Bedrock Problem: Only Provider Using Global Interfaces**
```go
// ❌ ONLY Bedrock does this - creates global coupling
func mergeWithClientOptions(defaults *BedrockOptions, opts gollm.ClientOptions) *BedrockOptions {
    if opts.InferenceConfig != nil {          // ❌ Global interface dependency
        // Complex merging logic
    }
}

if cs.client.clientOpts.UsageCallback != nil {  // ❌ Global interface dependency
    cs.client.clientOpts.UsageCallback("bedrock", cs.model, *structuredUsage)
}
```

## Problem Analysis: go-llm-apps Dependencies

### **🔗 Current go-llm-apps Usage Pattern**
```go
// How go-llm-apps calls kubectl-ai gollm
llmClient, err := gollm.NewClient(clientCtx, providerName,
    gollm.WithInferenceConfig(inferenceCfg),     // ❌ ONLY needed for Bedrock
    gollm.WithUsageCallback(usageCallback),     // ❌ ONLY needed for Bedrock  
    gollm.WithDebug(h.debug),
)

inferenceConfig := &gollm.InferenceConfig{
    Model:       cfg.Benchmark.ModelArn,
    Temperature: 0.1,
    MaxTokens:   2048,
    MaxRetries:  3,
}
```

### **💡 Key Insight: Other Providers Ignore These Options**
- **OpenAI, Grok, Ollama, LlamaCpp**: Don't use `InferenceConfig` or `UsageCallback`
- **Only Bedrock**: Depends on these global interfaces
- **Solution**: Make Bedrock follow the same self-contained pattern while maintaining backward compatibility

## Hybrid Implementation Strategy

### **🎯 Design Goals**
1. **✅ Zero Global Interface Changes**: No modifications to global `gollm` interfaces
2. **✅ Backward Compatibility**: Existing go-llm-apps code continues working  
3. **✅ Self-Contained Primary**: New code uses simple environment + URL config
4. **✅ Pattern Consistency**: Follows same patterns as OpenAI, Grok, Ollama

### **🔧 Configuration Priority (Hybrid Approach)**
```go
// 1. FIRST: Environment variables (like other providers)
BEDROCK_REGION=us-west-2
BEDROCK_MODEL=claude-3-sonnet
BEDROCK_TEMPERATURE=0.7
BEDROCK_MAX_TOKENS=2048
BEDROCK_USAGE_LOGGING=true

// 2. SECOND: URL parameters (like other providers)  
bedrock://us-west-2/claude-3-sonnet?temperature=0.7&max_tokens=2048

// 3. FALLBACK: Global interfaces (for go-llm-apps compatibility)
gollm.WithInferenceConfig(config)  // Only used if env/URL not set
gollm.WithUsageCallback(callback)  // Used alongside Bedrock logging
```

## Implementation Details - HYBRID APPROACH

### **1. Self-Contained Configuration Parser** - `config.go`
```go
// New function - follows OpenAI/Grok pattern exactly
func parseBedrockConfig(u *url.URL) *BedrockOptions {
    // Start with defaults
    config := &BedrockOptions{
        Region:      getEnvOrDefault("BEDROCK_REGION", "us-west-2"),
        Model:       getEnvOrDefault("BEDROCK_MODEL", "us.anthropic.claude-sonnet-4-20250514-v1:0"),
        Temperature: getEnvFloatOrDefault("BEDROCK_TEMPERATURE", 0.1),
        MaxTokens:   getEnvIntOrDefault("BEDROCK_MAX_TOKENS", 64000),
        TopP:        getEnvFloatOrDefault("BEDROCK_TOP_P", 0.9),
        MaxRetries:  getEnvIntOrDefault("BEDROCK_MAX_RETRIES", 10),
        Timeout:     30 * time.Second,
    }
    
    // URL overrides (like llamacpp pattern)
    if u != nil {
        // Parse region from host: bedrock://us-west-2/
        if u.Host != "" {
            config.Region = u.Host
        }
        
        // Parse model from path: bedrock://us-west-2/claude-3-sonnet
        if u.Path != "" {
            model := strings.TrimPrefix(u.Path, "/")
            if model != "" {
                config.Model = model
            }
        }
        
        // Parse query parameters: ?temperature=0.7&max_tokens=2048
        if temp := u.Query().Get("temperature"); temp != "" {
            if f, err := strconv.ParseFloat(temp, 32); err == nil {
                config.Temperature = float32(f)
            }
        }
        if maxTokens := u.Query().Get("max_tokens"); maxTokens != "" {
            if i, err := strconv.Atoi(maxTokens); err == nil {
                config.MaxTokens = int32(i)
            }
        }
        if topP := u.Query().Get("top_p"); topP != "" {
            if f, err := strconv.ParseFloat(topP, 32); err == nil {
                config.TopP = float32(f)
            }
        }
    }
    
    return config
}

// Helper functions (like other providers)
func getEnvOrDefault(key, defaultVal string) string {
    if val := os.Getenv(key); val != "" {
        return val
    }
    return defaultVal
}
```

### **2. Hybrid Client Factory** - `bedrock.go`
```go
func newBedrockClientFactory(ctx context.Context, opts gollm.ClientOptions) (gollm.Client, error) {
    // STEP 1: Parse self-contained config (like other providers)
    options := parseBedrockConfig(opts.URL)
    
    // STEP 2: Backward compatibility - merge global interfaces IF present
    if needsBackwardCompatibility(opts) {
        options = mergeGlobalOptions(options, opts)
    }
    
    return NewBedrockClientWithOptions(ctx, options)
}

// Simple check - no interface modification needed
func needsBackwardCompatibility(opts gollm.ClientOptions) bool {
    // Use reflection or interface detection to check if global options exist
    // This doesn't require modifying global interfaces
    return hasInferenceConfig(opts) || hasUsageCallback(opts)
}

// Simplified merger (only used for backward compatibility)
func mergeGlobalOptions(bedrockConfig *BedrockOptions, opts gollm.ClientOptions) *BedrockOptions {
    // Only override if bedrock-specific config wasn't set
    if config := extractInferenceConfig(opts); config != nil {
        if bedrockConfig.Model == DefaultOptions.Model && config.Model != "" {
            bedrockConfig.Model = config.Model
        }
        if bedrockConfig.Temperature == DefaultOptions.Temperature && config.Temperature != 0 {
            bedrockConfig.Temperature = config.Temperature
        }
        // ... etc
    }
    
    return bedrockConfig
}
```

### **3. Simplified Client Structure** - `bedrock.go`
```go
type BedrockClient struct {
    runtimeClient *bedrockruntime.Client
    bedrockClient *bedrock.Client
    options       *BedrockOptions
    usageCallback func(string, string, TokenUsage) // Simplified, optional
}

func NewBedrockClient(ctx context.Context, opts gollm.ClientOptions) (*BedrockClient, error) {
    return newBedrockClientFactory(ctx, opts)
}

// Remove complex global coupling - store only what's needed
func NewBedrockClientWithOptions(ctx context.Context, options *BedrockOptions) (*BedrockClient, error) {
    // Same AWS configuration logic (already good)
    // ...
    
    client := &BedrockClient{
        runtimeClient: bedrockruntime.NewFromConfig(cfg),
        bedrockClient: bedrock.NewFromConfig(cfg),
        options:       options,
        // Remove: clientOpts storage (eliminates coupling)
    }
    
    return client, nil
}
```

### **4. Hybrid Usage Tracking** - `bedrock.go`
```go
// Bedrock-specific usage logging (primary)
func (cs *bedrockChatSession) logUsage(usage *types.TokenUsage) {
    if os.Getenv("BEDROCK_USAGE_LOGGING") == "true" {
        klog.Infof("Bedrock Usage - Model: %s, Input: %d, Output: %d, Total: %d",
            cs.model,
            aws.ToInt32(usage.InputTokens),
            aws.ToInt32(usage.OutputTokens),
            aws.ToInt32(usage.TotalTokens))
    }
}

// Optional global callback support (backward compatibility)
func (cs *bedrockChatSession) handleUsage(usage *types.TokenUsage) {
    // Always do bedrock-specific logging first
    cs.logUsage(usage)
    
    // Call global callback if available (for go-llm-apps compatibility)
    if cs.client.usageCallback != nil {
        simpleUsage := TokenUsage{
            InputTokens:  int(aws.ToInt32(usage.InputTokens)),
            OutputTokens: int(aws.ToInt32(usage.OutputTokens)),
            TotalTokens:  int(aws.ToInt32(usage.TotalTokens)),
        }
        cs.client.usageCallback("bedrock", cs.model, simpleUsage)
    }
}
```

## Usage Patterns - HYBRID SUPPORT

### **✅ Primary Pattern: Self-Contained (New Code)**
```go
// Environment-first configuration (like OpenAI/Grok)
export BEDROCK_REGION=us-west-2
export BEDROCK_MODEL=claude-3-sonnet
export BEDROCK_TEMPERATURE=0.7
export BEDROCK_MAX_TOKENS=2048
export BEDROCK_USAGE_LOGGING=true

// Simple client creation
client, err := gollm.NewClient(ctx, "bedrock://")

// Or URL-based configuration
client, err := gollm.NewClient(ctx, "bedrock://us-east-1/claude-3-sonnet?temperature=0.8")
```

### **✅ Backward Compatibility: Global Interfaces (Existing go-llm-apps)**
```go
// Existing go-llm-apps code continues to work unchanged
inferenceConfig := &gollm.InferenceConfig{
    Model:       "claude-3-sonnet",
    Temperature: 0.7,
    MaxTokens:   2048,
}

llmClient, err := gollm.NewClient(ctx, "bedrock://us-west-2/",
    gollm.WithInferenceConfig(inferenceConfig),
    gollm.WithUsageCallback(usageCallback),
)
```

### **✅ Hybrid Pattern: Best of Both Worlds**
```go
// Environment provides defaults
export BEDROCK_REGION=us-west-2
export BEDROCK_USAGE_LOGGING=true

// Global options override specific values
inferenceConfig := &gollm.InferenceConfig{
    Model:       "claude-3-5-sonnet",  // Overrides default
    Temperature: 0.8,                 // Overrides default
}

client, err := gollm.NewClient(ctx, "bedrock://",
    gollm.WithInferenceConfig(inferenceConfig),
)
```

## Changes Required in go-llm-apps

### **🎯 Option 1: Zero Changes (Recommended)**
- **Current code**: Continues working exactly as-is
- **Benefit**: No migration needed
- **Usage**: Global interfaces work as fallback

### **🎯 Option 2: Gradual Migration (Optional)**
```go
// Step 1: Add environment variables (no code changes)
export BEDROCK_REGION=us-west-2
export BEDROCK_MODEL=claude-3-sonnet
export BEDROCK_USAGE_LOGGING=true

// Step 2: Simplify code gradually (optional)
// Before:
llmClient, err := gollm.NewClient(ctx, "bedrock://",
    gollm.WithInferenceConfig(inferenceCfg),
    gollm.WithUsageCallback(usageCallback),
)

// After:
llmClient, err := gollm.NewClient(ctx, "bedrock://")
```

### **🎯 Option 3: Bedrock-Specific Direct Usage (Future)**
```go
// Future option: Direct bedrock package usage for advanced features
import "github.com/GoogleCloudPlatform/kubectl-ai/gollm/bedrock"

client, err := bedrock.NewBedrockClientWithOptions(ctx, &bedrock.BedrockOptions{
    Region:      "us-west-2",
    Model:       "claude-3-sonnet",
    Temperature: 0.7,
})
```

## Benefits of Hybrid Approach

### **✅ Eliminates Global Coupling**
- **No global interface modifications** required
- **Self-contained bedrock package** - can be moved to separate module
- **Pattern consistency** with OpenAI, Grok, Ollama, LlamaCpp
- **Zero impact** on other providers

### **✅ Maintains Full Compatibility**
- **Existing go-llm-apps code** works unchanged
- **All current functionality** preserved
- **Gradual migration path** available
- **No breaking changes** required

### **✅ Simplifies New Development**
- **Environment-first** configuration like other providers
- **URL-based** parameters for easy testing
- **Bedrock-specific** usage logging and debugging
- **No global interface knowledge** needed

### **✅ Reduces Implementation Complexity**
- **Remove `mergeWithClientOptions`** complex logic (backward compatibility only)
- **Remove `clientOpts` storage** (eliminates coupling)
- **Remove `convertAWSUsage`** global struct dependency
- **Simpler usage tracking** with optional callback support

## What Gets Simplified

### **❌ Removed Complexities**
```go
// Remove these complex global dependencies
func mergeWithClientOptions(defaults *BedrockOptions, opts gollm.ClientOptions) *BedrockOptions
func convertAWSUsage(awsUsage any, model, provider string) *gollm.Usage
clientOpts gollm.ClientOptions // Remove global options storage
```

### **✅ Added Simplifications**
- **-150 lines**: Remove complex global interface merging
- **+50 lines**: Add simple environment variable parsing
- **Self-contained**: All bedrock logic in bedrock package
- **Standard patterns**: Identical to OpenAI, Grok, Ollama patterns

### **🔄 What Stays the Same**
- ✅ **All 4 files**: Same package structure
- ✅ **All AWS logic**: Credential handling, timeouts, regions
- ✅ **All features**: Streaming, function calling, error handling
- ✅ **All interfaces**: Perfect `gollm.Client` and `gollm.Chat` implementation
- ✅ **All tests**: 80% of tests unchanged

## Implementation Timeline - HYBRID APPROACH

### **Week 1: Self-Contained Implementation**
1. **Day 1-2**: Add `parseBedrockConfig()` with env vars + URL parsing
2. **Day 3-4**: Implement hybrid factory with backward compatibility detection
3. **Day 5**: Add simplified usage tracking with optional callback

### **Week 2: Integration & Testing**
1. **Day 1-2**: Update tests for hybrid approach
2. **Day 3**: Test with existing go-llm-apps (zero changes)
3. **Day 4**: Test new self-contained patterns
4. **Day 5**: Documentation and polish

**Total**: 2 weeks for complete hybrid implementation

## Conclusion - OPTIMAL HYBRID SOLUTION

This **hybrid approach** delivers the **optimal solution** by:

- ✅ **Zero Global Interface Changes**: No modifications to global `gollm` interfaces
- ✅ **Full Backward Compatibility**: Existing go-llm-apps code works unchanged
- ✅ **Self-Contained Primary**: New code uses simple environment + URL config
- ✅ **Pattern Consistency**: Follows identical patterns as all other providers
- ✅ **Simplified Maintenance**: All bedrock logic self-contained
- ✅ **Gradual Migration**: Optional migration path for simplified usage

The implementation becomes **significantly simpler and self-contained** while maintaining **perfect compatibility** with existing usage patterns. This satisfies all requirements while following established patterns from other providers.

**Status**: Ready for implementation - optimal hybrid solution identified. 