# Usage Metrics and Inference Configuration Implementation Analysis

## Executive Summary

This document provides a deep analysis of implementing comprehensive usage metrics tracking and advanced inference configuration in kubectl-ai. Based on examination of the current codebase, we identify key implementation patterns, gaps, and recommendations for building a production-ready metrics and configuration system.

## Current State Analysis

### Existing Usage Tracking Architecture

#### Interface Design
The current `gollm` package defines usage tracking through the `UsageMetadata()` interface:

```go
// From gollm/interfaces.go
type CompletionResponse interface {
    Response() string
    UsageMetadata() any  // Generic interface for usage data
}

type ChatResponse interface {
    UsageMetadata() any  // Same generic approach
    Candidates() []Candidate
}
```

**Key Observations:**
- **Generic Interface**: Uses `any` type, allowing provider-specific implementations
- **No Standardization**: Each provider returns different data structures
- **Basic Implementation**: Currently focused on raw provider response data

#### Provider Implementations

**1. OpenAI Provider** (`gollm/openai.go`)
```go
func (r *openAIChatResponse) UsageMetadata() any {
    if r.openaiCompletion != nil && r.openaiCompletion.Usage.TotalTokens > 0 {
        return r.openaiCompletion.Usage  // Returns OpenAI-specific usage struct
    }
    return nil
}
```
- Returns structured token usage (`InputTokens`, `OutputTokens`, `TotalTokens`)
- Includes completion and prompt tokens
- No cost calculation

**2. Bedrock Provider** (`gollm/bedrock/bedrock.go`)
```go
func (cs *bedrockChatSession) Send(ctx context.Context, contents ...any) (gollm.ChatResponse, error) {
    // ... API call logic ...
    response.usage = output.Usage  // AWS Bedrock usage data
    return response, nil
}
```
- Extracts AWS native usage metadata
- Includes input/output tokens from Bedrock API
- Currently stores raw AWS response structure

**3. Gemini Provider** (`gollm/gemini.go`)  
```go
func (r *GeminiChatResponse) UsageMetadata() any {
    return r.geminiResponse.UsageMetadata  // Google-specific usage
}
```
- Returns Google AI usage metadata
- Provider-native structure

**4. Limited Providers** (Grok, LlamaCpp, etc.)
```go
func (r *simpleGrokCompletionResponse) UsageMetadata() any {
    return nil  // No usage tracking implemented
}
```

### Current Configuration Architecture

#### Provider-Specific Configuration

**Bedrock Configuration** (`gollm/bedrock/config.go`)
```go
type BedrockOptions struct {
    Region              string
    CredentialsProvider aws.CredentialsProvider
    Model               string
    MaxTokens           int32
    Temperature         float32
    TopP                float32
    Timeout             time.Duration
    MaxRetries          int
}
```

**Global Configuration** (`gollm/factory.go`)
```go
type ClientOptions struct {
    URL           *url.URL
    SkipVerifySSL bool
    // Limited global options
}
```

## Gap Analysis

### Usage Metrics Gaps

#### 1. **No Standardized Metrics Structure**
- Each provider returns different data formats
- No common fields across providers
- Difficult to aggregate or compare usage

#### 2. **Missing Cost Calculation**
- No built-in cost calculation per provider
- No pricing model integration
- No cost tracking across conversations

#### 3. **Limited Streaming Metrics**
- Streaming responses often lack complete usage data
- Token counting during streaming is incomplete
- No real-time usage tracking

#### 4. **No Persistence or Aggregation**
- Usage data only available per request
- No session-level or aggregate tracking
- No metrics export capability

#### 5. **Missing Business Metrics**
- No user/session attribution
- No feature usage tracking (function calls, streaming, etc.)
- No error rate tracking

### Configuration Gaps

#### 1. **Limited Global Configuration**
- Provider-specific options scattered
- No unified configuration schema
- Limited environment variable support

#### 2. **No Runtime Configuration Updates**
- Static configuration only
- No dynamic parameter adjustment
- No A/B testing support

#### 3. **Missing Inference Optimization**
- No automatic parameter tuning
- No model selection optimization
- No cost/performance balancing

## Implementation Recommendations

### Phase 1: Standardized Usage Metrics

#### A. Define Universal Usage Metrics Interface

```go
// pkg/metrics/types.go
type UsageMetrics struct {
    // Basic token metrics (required)
    InputTokens   int64     `json:"input_tokens"`
    OutputTokens  int64     `json:"output_tokens"`
    TotalTokens   int64     `json:"total_tokens"`
    
    // Cost metrics (optional)
    InputCost     *float64  `json:"input_cost,omitempty"`
    OutputCost    *float64  `json:"output_cost,omitempty"`
    TotalCost     *float64  `json:"total_cost,omitempty"`
    
    // Request metadata
    Provider      string    `json:"provider"`
    Model         string    `json:"model"`
    RequestID     string    `json:"request_id,omitempty"`
    Timestamp     time.Time `json:"timestamp"`
    Duration      int64     `json:"duration_ms"`
    
    // Feature usage
    FunctionCalls int       `json:"function_calls"`
    IsStreaming   bool      `json:"is_streaming"`
    
    // Quality metrics
    FinishReason  string    `json:"finish_reason,omitempty"`
    Truncated     bool      `json:"truncated"`
    
    // Provider-specific data
    Raw           any       `json:"raw,omitempty"`
}

type MetricsTracker interface {
    TrackUsage(ctx context.Context, metrics *UsageMetrics) error
    GetSessionMetrics(sessionID string) (*SessionMetrics, error)
    GetAggregateMetrics(timeRange TimeRange) (*AggregateMetrics, error)
}
```

#### B. Extend gollm Interfaces

```go
// gollm/interfaces.go - Enhanced interfaces
type ChatResponse interface {
    UsageMetadata() any                    // Keep for backward compatibility
    StandardUsageMetrics() *UsageMetrics   // NEW: Standardized metrics
    Candidates() []Candidate
}

type CompletionResponse interface {
    Response() string
    UsageMetadata() any                    // Keep for backward compatibility  
    StandardUsageMetrics() *UsageMetrics   // NEW: Standardized metrics
}

type ClientOptions struct {
    URL            *url.URL
    SkipVerifySSL  bool
    MetricsTracker MetricsTracker         // NEW: Optional metrics tracking
    PricingConfig  *PricingConfiguration  // NEW: Cost calculation
}
```

#### C. Implement Provider Adapters

```go
// gollm/metrics/adapters.go
func ExtractBedrockMetrics(rawUsage any, model string) *UsageMetrics {
    if usage, ok := rawUsage.(*bedrockruntime.ConverseOutput); ok {
        return &UsageMetrics{
            InputTokens:  int64(*usage.Usage.InputTokens),
            OutputTokens: int64(*usage.Usage.OutputTokens),
            TotalTokens:  int64(*usage.Usage.TotalTokens),
            Provider:     "bedrock",
            Model:        model,
            Timestamp:    time.Now(),
            Raw:          rawUsage,
        }
    }
    return nil
}

func ExtractOpenAIMetrics(rawUsage any, model string) *UsageMetrics {
    if usage, ok := rawUsage.(*openai.Usage); ok {
        return &UsageMetrics{
            InputTokens:  int64(usage.PromptTokens),
            OutputTokens: int64(usage.CompletionTokens),
            TotalTokens:  int64(usage.TotalTokens),
            Provider:     "openai",
            Model:        model,
            Timestamp:    time.Now(),
            Raw:          rawUsage,
        }
    }
    return nil
}
```

### Phase 2: Cost Calculation System

#### A. Pricing Configuration

```go
// pkg/metrics/pricing.go
type PricingConfiguration struct {
    Providers map[string]*ProviderPricing `json:"providers"`
    Currency  string                      `json:"currency"` // USD, EUR, etc.
    UpdatedAt time.Time                   `json:"updated_at"`
}

type ProviderPricing struct {
    Models map[string]*ModelPricing `json:"models"`
}

type ModelPricing struct {
    InputCostPer1KTokens  float64 `json:"input_cost_per_1k_tokens"`
    OutputCostPer1KTokens float64 `json:"output_cost_per_1k_tokens"`
    // Additional pricing models
    RequestCost          *float64 `json:"request_cost,omitempty"`
    ImageCostPerMP       *float64 `json:"image_cost_per_mp,omitempty"`
}

func CalculateCost(metrics *UsageMetrics, pricing *ModelPricing) {
    if pricing == nil {
        return
    }
    
    inputCost := float64(metrics.InputTokens) / 1000.0 * pricing.InputCostPer1KTokens
    outputCost := float64(metrics.OutputTokens) / 1000.0 * pricing.OutputCostPer1KTokens
    
    metrics.InputCost = &inputCost
    metrics.OutputCost = &outputCost
    totalCost := inputCost + outputCost
    metrics.TotalCost = &totalCost
}
```

### Phase 3: Advanced Configuration System

#### A. Unified Configuration Schema

```go
// pkg/config/types.go
type InferenceConfig struct {
    // Global defaults
    DefaultProvider string                    `yaml:"default_provider"`
    DefaultModel    string                    `yaml:"default_model"`
    
    // Provider configurations
    Providers map[string]*ProviderConfig     `yaml:"providers"`
    
    // Performance optimization
    Optimization *OptimizationConfig         `yaml:"optimization"`
    
    // Metrics and monitoring
    Metrics *MetricsConfig                   `yaml:"metrics"`
    
    // Runtime behavior
    Runtime *RuntimeConfig                   `yaml:"runtime"`
}

type ProviderConfig struct {
    // Connection settings
    Endpoint    string            `yaml:"endpoint,omitempty"`
    Region      string            `yaml:"region,omitempty"`
    Timeout     time.Duration     `yaml:"timeout"`
    MaxRetries  int               `yaml:"max_retries"`
    
    // Authentication
    APIKey      string            `yaml:"api_key,omitempty"`
    Credentials map[string]string `yaml:"credentials,omitempty"`
    
    // Model settings
    Models map[string]*ModelConfig `yaml:"models"`
    
    // Rate limiting
    RateLimit *RateLimitConfig    `yaml:"rate_limit,omitempty"`
}

type ModelConfig struct {
    // Inference parameters
    MaxTokens        int32   `yaml:"max_tokens"`
    Temperature      float32 `yaml:"temperature"`
    TopP             float32 `yaml:"top_p"`
    TopK             int32   `yaml:"top_k,omitempty"`
    FrequencyPenalty float32 `yaml:"frequency_penalty,omitempty"`
    PresencePenalty  float32 `yaml:"presence_penalty,omitempty"`
    
    // Streaming settings
    StreamingEnabled bool `yaml:"streaming_enabled"`
    StreamChunkSize  int  `yaml:"stream_chunk_size,omitempty"`
    
    // Function calling
    FunctionCalling *FunctionCallingConfig `yaml:"function_calling,omitempty"`
    
    // Cost constraints
    MaxCostPerRequest *float64 `yaml:"max_cost_per_request,omitempty"`
    
    // Quality settings
    ResponseSchema   string   `yaml:"response_schema,omitempty"`
    OutputFormat     string   `yaml:"output_format,omitempty"`
}

type OptimizationConfig struct {
    // Model selection
    AutoModelSelection   bool                      `yaml:"auto_model_selection"`
    ModelSelectionRules  []*ModelSelectionRule    `yaml:"model_selection_rules"`
    
    // Parameter optimization
    AutoParameterTuning  bool                     `yaml:"auto_parameter_tuning"`
    TuningStrategy       string                   `yaml:"tuning_strategy"` // cost, speed, quality
    
    // Caching
    ResponseCaching      bool                     `yaml:"response_caching"`
    CacheTTL            time.Duration            `yaml:"cache_ttl"`
}

type ModelSelectionRule struct {
    Condition   string  `yaml:"condition"`   // "cost < 0.01", "tokens > 1000"
    Model       string  `yaml:"model"`
    Provider    string  `yaml:"provider"`
    Priority    int     `yaml:"priority"`
}
```

### Phase 4: Intelligent Inference Management

#### A. Model Selection Optimization

```go
// pkg/inference/optimizer.go
type InferenceOptimizer struct {
    config     *config.InferenceConfig
    metrics    metrics.MetricsTracker
    pricing    *metrics.PricingConfiguration
    predictor  *PerformancePredictor
}

type OptimizationRequest struct {
    Query         string
    Context       map[string]any
    Constraints   *Constraints
    Preferences   *Preferences
}

type Constraints struct {
    MaxCost        *float64
    MaxLatency     *time.Duration
    RequiredModel  string
    RequiredRegion string
}

type Preferences struct {
    OptimizeFor    string  // "cost", "speed", "quality"
    QualityWeight  float64
    CostWeight     float64
    SpeedWeight    float64
}

func (o *InferenceOptimizer) SelectOptimalModel(req *OptimizationRequest) (*ModelSelection, error) {
    // Analyze query complexity
    complexity := o.analyzeQueryComplexity(req.Query)
    
    // Get available models
    availableModels := o.getAvailableModels(req.Constraints)
    
    // Score each model
    var candidates []*ScoredModel
    for _, model := range availableModels {
        score := o.scoreModel(model, complexity, req.Preferences)
        candidates = append(candidates, &ScoredModel{
            Model: model,
            Score: score,
        })
    }
    
    // Select best model
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].Score > candidates[j].Score
    })
    
    if len(candidates) == 0 {
        return nil, fmt.Errorf("no suitable models found")
    }
    
    return &ModelSelection{
        Provider: candidates[0].Model.Provider,
        Model:    candidates[0].Model.Name,
        Score:    candidates[0].Score,
        Reasoning: o.generateReasoning(candidates[0]),
    }, nil
}
```

## Implementation Strategy

### Immediate Actions (Weeks 1-2)

1. **Define Standard Metrics Interface**
   - Create `pkg/metrics/types.go` with `UsageMetrics` struct
   - Add `StandardUsageMetrics()` method to gollm interfaces
   - Implement backward compatibility

2. **Basic Provider Adapters**
   - Create adapters for existing providers (OpenAI, Bedrock, Gemini)
   - Ensure consistent token counting
   - Add timestamp and duration tracking

3. **Simple Cost Calculation**
   - Define basic pricing configuration format
   - Implement cost calculation for major providers
   - Add cost fields to metrics

### Short-term Goals (Weeks 3-4)

4. **Enhanced Configuration System**
   - Design unified configuration schema
   - Implement configuration validation
   - Add environment variable integration

5. **Metrics Persistence**
   - Create in-memory metrics tracker
   - Add session-level aggregation
   - Implement basic metrics export

### Medium-term Goals (Weeks 5-8)

6. **Advanced Features**
   - Dynamic configuration updates
   - Real-time pricing updates
   - Streaming metrics improvements

7. **Performance Optimization**
   - Basic model selection rules
   - Cost constraint enforcement
   - Performance monitoring

### Long-term Vision (Weeks 9-12)

8. **Intelligent Systems**
   - Machine learning-based model selection
   - Predictive performance modeling
   - Automated parameter optimization

9. **Enterprise Features**
   - Multi-tenant metrics
   - Advanced cost management
   - Compliance and audit trails

## Risk Assessment

### Technical Risks

1. **Breaking Changes**: Introducing new interfaces may break existing code
   - **Mitigation**: Maintain backward compatibility, gradual migration

2. **Performance Impact**: Metrics tracking overhead
   - **Mitigation**: Asynchronous tracking, configurable detail levels

3. **Pricing Accuracy**: External pricing data may be outdated
   - **Mitigation**: Automatic updates, fallback mechanisms

### Operational Risks

1. **Configuration Complexity**: Too many options may overwhelm users
   - **Mitigation**: Sensible defaults, configuration templates

2. **Cost Overruns**: Automatic optimization may increase costs
   - **Mitigation**: Hard cost limits, user approval workflows

## Success Metrics

### Technical Success
- **Metric Coverage**: 95% of requests have complete usage metrics
- **Cost Accuracy**: <5% error in cost calculations
- **Performance**: <10ms overhead for metrics tracking

### User Success  
- **Adoption**: 80% of users enable metrics tracking
- **Cost Savings**: 20% average cost reduction through optimization
- **Developer Experience**: Reduced configuration complexity

## Conclusion

Implementing comprehensive usage metrics and intelligent inference configuration in kubectl-ai requires a phased approach balancing immediate value delivery with long-term extensibility. The current architecture provides a solid foundation, but significant enhancements are needed for production-grade usage tracking and optimization.

The proposed implementation prioritizes standardization, backward compatibility, and incremental deployment while building toward intelligent, cost-aware inference management that can significantly improve both user experience and operational efficiency.

