# gollm Integration Architecture Guide for go-llm-apps

## Overview

This guide provides architectural guidance for integrating the `kubectl-ai/gollm` package into your go-llm-apps repository. This integration enables seamless usage of the gollm bedrock provider with comprehensive usage metrics tracking and inference configuration management.

## Integration Architecture

### 1. Application Layer Structure

The integration follows a layered architecture pattern commonly found in enterprise Go applications:

```
go-llm-apps/
├── cmd/
│   └── webserver/
│       ├── main.go              # Application entry point
│       └── handlers.go          # HTTP handlers with gollm client creation
├── pkg/
│   ├── agent/
│   │   ├── conversation.go      # Agent conversation logic with streaming
│   │   ├── interfaces.go        # Agent interfaces
│   │   └── usage_tracker.go     # Usage metrics aggregation
│   ├── apps/
│   │   ├── kubectl_agent.go     # kubectl-specific agent implementation
│   │   └── usage_aggregator.go  # Application-level usage tracking
│   ├── config/
│   │   ├── inference.go         # Inference configuration management
│   │   └── llm.go              # LLM provider configuration
│   └── middleware/
│       └── usage_middleware.go  # Usage tracking middleware
└── internal/
    ├── metrics/
    │   └── collector.go         # Metrics collection and reporting
    └── storage/
        └── usage_store.go       # Usage data persistence
```

### 2. Integration Points

#### A. Handler Level Integration (`cmd/webserver/handlers.go`)

The webserver handlers create and configure gollm clients with inference configuration and usage callbacks:

```go
package main

import (
    "context"
    "encoding/json"
    "net/http"
    
    "github.com/your-org/go-llm-apps/pkg/config"
    "github.com/your-org/go-llm-apps/pkg/metrics"
    "github.com/your-kubectl-ai/gollm"
)

type WebServer struct {
    gollmClient   gollm.Client
    usageTracker  *metrics.UsageTracker
    config        *config.InferenceConfig
}

func NewWebServer() (*WebServer, error) {
    // Load inference configuration
    inferenceConfig := config.LoadInferenceConfig()
    
    // Create usage tracker
    usageTracker := metrics.NewUsageTracker()
    
    // Create gollm client with configuration
    client, err := gollm.NewClient(
        context.Background(),
        "bedrock",
        gollm.WithInferenceConfig(&gollm.InferenceConfig{
            Model:       inferenceConfig.Model,
            Region:      inferenceConfig.Region,
            Temperature: inferenceConfig.Temperature,
            MaxTokens:   inferenceConfig.MaxTokens,
            TopP:        inferenceConfig.TopP,
            TopK:        inferenceConfig.TopK,
            MaxRetries:  inferenceConfig.MaxRetries,
        }),
        gollm.WithUsageCallback(usageTracker.OnUsage),
        gollm.WithDebug(inferenceConfig.Debug),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create gollm client: %w", err)
    }
    
    return &WebServer{
        gollmClient:  client,
        usageTracker: usageTracker,
        config:       inferenceConfig,
    }, nil
}

func (ws *WebServer) handleKubectlQuery(w http.ResponseWriter, r *http.Request) {
    // Create agent with gollm client
    agent := agent.NewKubectlAgent(ws.gollmClient, ws.config)
    
    // Process request with usage tracking
    response, err := agent.ProcessQuery(r.Context(), request)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // Return response with usage metadata
    json.NewEncoder(w).Encode(response)
}
```

#### B. Agent Level Integration (`pkg/agent/conversation.go`)

The agent layer processes streaming responses and extracts usage data:

```go
package agent

import (
    "context"
    "fmt"
    
    "github.com/your-kubectl-ai/gollm"
    "github.com/your-org/go-llm-apps/pkg/config"
)

type KubectlAgent struct {
    client gollm.Client
    config *config.InferenceConfig
    usage  *UsageAggregator
}

func NewKubectlAgent(client gollm.Client, config *config.InferenceConfig) *KubectlAgent {
    return &KubectlAgent{
        client: client,
        config: config,
        usage:  NewUsageAggregator(),
    }
}

func (a *KubectlAgent) ProcessQuery(ctx context.Context, query string) (*QueryResponse, error) {
    // Start chat session
    chat := a.client.StartChat(
        "You are an expert Kubernetes administrator assistant.",
        a.config.Model,
    )
    
    // Send streaming request
    iterator, err := chat.SendStreaming(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to send query: %w", err)
    }
    
    var fullResponse string
    var totalUsage *gollm.Usage
    
    // Process streaming response
    for response, err := range iterator {
        if err != nil {
            return nil, fmt.Errorf("streaming error: %w", err)
        }
        
        if response == nil {
            break // End of stream
        }
        
        // Accumulate response content
        candidates := response.Candidates()
        if len(candidates) > 0 {
            fullResponse += candidates[0].Content
        }
        
        // Extract usage metadata
        if usageMetadata := response.UsageMetadata(); usageMetadata != nil {
            totalUsage = usageMetadata.GetUsage()
        }
    }
    
    // Aggregate usage data
    if totalUsage != nil {
        a.usage.AddUsage(*totalUsage)
    }
    
    return &QueryResponse{
        Content: fullResponse,
        Usage:   totalUsage,
        Model:   a.config.Model,
    }, nil
}
```

#### C. Application Level Integration (`pkg/apps/usage_aggregator.go`)

Application-level usage aggregation across multiple conversations:

```go
package apps

import (
    "sync"
    "time"
    
    "github.com/your-kubectl-ai/gollm"
)

type UsageAggregator struct {
    mu           sync.RWMutex
    totalUsage   map[string]*AggregatedUsage
    conversions  map[string]*ConversationUsage
}

type AggregatedUsage struct {
    TotalInputTokens   int     `json:"totalInputTokens"`
    TotalOutputTokens  int     `json:"totalOutputTokens"`
    TotalCost         float64 `json:"totalCost"`
    RequestCount      int     `json:"requestCount"`
    AverageCost       float64 `json:"averageCost"`
    LastUpdate        time.Time `json:"lastUpdate"`
}

type ConversationUsage struct {
    ConversationID string                 `json:"conversationId"`
    Model          string                 `json:"model"`
    Provider       string                 `json:"provider"`
    UsageHistory   []gollm.Usage         `json:"usageHistory"`
    StartTime      time.Time             `json:"startTime"`
    LastActivity   time.Time             `json:"lastActivity"`
}

func NewUsageAggregator() *UsageAggregator {
    return &UsageAggregator{
        totalUsage:   make(map[string]*AggregatedUsage),
        conversions:  make(map[string]*ConversationUsage),
    }
}

func (ua *UsageAggregator) OnUsage(provider, model string, usage gollm.Usage) {
    ua.mu.Lock()
    defer ua.mu.Unlock()
    
    key := fmt.Sprintf("%s:%s", provider, model)
    
    // Update total usage
    if aggUsage, exists := ua.totalUsage[key]; exists {
        aggUsage.TotalInputTokens += usage.InputTokens
        aggUsage.TotalOutputTokens += usage.OutputTokens
        aggUsage.TotalCost += usage.TotalCost
        aggUsage.RequestCount++
        aggUsage.AverageCost = aggUsage.TotalCost / float64(aggUsage.RequestCount)
        aggUsage.LastUpdate = time.Now()
    } else {
        ua.totalUsage[key] = &AggregatedUsage{
            TotalInputTokens:  usage.InputTokens,
            TotalOutputTokens: usage.OutputTokens,
            TotalCost:        usage.TotalCost,
            RequestCount:     1,
            AverageCost:      usage.TotalCost,
            LastUpdate:       time.Now(),
        }
    }
}

func (ua *UsageAggregator) GetTotalUsage() map[string]*AggregatedUsage {
    ua.mu.RLock()
    defer ua.mu.RUnlock()
    
    result := make(map[string]*AggregatedUsage)
    for k, v := range ua.totalUsage {
        result[k] = v
    }
    return result
}
```

### 3. Configuration Management

#### A. Inference Configuration (`pkg/config/inference.go`)

```go
package config

import (
    "os"
    "strconv"
)

type InferenceConfig struct {
    Model       string  `json:"model" yaml:"model"`
    Region      string  `json:"region" yaml:"region"`
    Temperature float32 `json:"temperature" yaml:"temperature"`
    MaxTokens   int32   `json:"maxTokens" yaml:"maxTokens"`
    TopP        float32 `json:"topP" yaml:"topP"`
    TopK        int32   `json:"topK" yaml:"topK"`
    MaxRetries  int     `json:"maxRetries" yaml:"maxRetries"`
    Debug       bool    `json:"debug" yaml:"debug"`
}

func LoadInferenceConfig() *InferenceConfig {
    config := &InferenceConfig{
        Model:       getEnvOrDefault("LLM_MODEL", "anthropic.claude-3-sonnet-20240229-v1:0"),
        Region:      getEnvOrDefault("AWS_REGION", "us-east-1"),
        Temperature: getEnvFloat32OrDefault("LLM_TEMPERATURE", 0.1),
        MaxTokens:   getEnvInt32OrDefault("LLM_MAX_TOKENS", 64000),
        TopP:        getEnvFloat32OrDefault("LLM_TOP_P", 0.1),
        TopK:        getEnvInt32OrDefault("LLM_TOP_K", 1),
        MaxRetries:  getEnvIntOrDefault("LLM_MAX_RETRIES", 10),
        Debug:       getEnvBoolOrDefault("LLM_DEBUG", false),
    }
    
    return config
}

func getEnvOrDefault(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func getEnvFloat32OrDefault(key string, defaultValue float32) float32 {
    if value := os.Getenv(key); value != "" {
        if parsed, err := strconv.ParseFloat(value, 32); err == nil {
            return float32(parsed)
        }
    }
    return defaultValue
}

func getEnvInt32OrDefault(key string, defaultValue int32) int32 {
    if value := os.Getenv(key); value != "" {
        if parsed, err := strconv.ParseInt(value, 10, 32); err == nil {
            return int32(parsed)
        }
    }
    return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
    if value := os.Getenv(key); value != "" {
        if parsed, err := strconv.Atoi(value); err == nil {
            return parsed
        }
    }
    return defaultValue
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
    if value := os.Getenv(key); value != "" {
        if parsed, err := strconv.ParseBool(value); err == nil {
            return parsed
        }
    }
    return defaultValue
}
```

#### B. LLM Provider Configuration (`pkg/config/llm.go`)

```go
package config

type LLMConfig struct {
    Provider        string           `json:"provider" yaml:"provider"`
    InferenceConfig *InferenceConfig `json:"inferenceConfig" yaml:"inferenceConfig"`
    UsageTracking   UsageConfig      `json:"usageTracking" yaml:"usageTracking"`
}

type UsageConfig struct {
    Enabled          bool   `json:"enabled" yaml:"enabled"`
    StorageBackend   string `json:"storageBackend" yaml:"storageBackend"`
    MetricsEndpoint  string `json:"metricsEndpoint" yaml:"metricsEndpoint"`
    ReportingInterval string `json:"reportingInterval" yaml:"reportingInterval"`
}

func LoadLLMConfig() *LLMConfig {
    return &LLMConfig{
        Provider:        getEnvOrDefault("LLM_PROVIDER", "bedrock"),
        InferenceConfig: LoadInferenceConfig(),
        UsageTracking: UsageConfig{
            Enabled:          getEnvBoolOrDefault("USAGE_TRACKING_ENABLED", true),
            StorageBackend:   getEnvOrDefault("USAGE_STORAGE_BACKEND", "memory"),
            MetricsEndpoint:  getEnvOrDefault("METRICS_ENDPOINT", "/metrics"),
            ReportingInterval: getEnvOrDefault("REPORTING_INTERVAL", "5m"),
        },
    }
}
```

### 4. Metrics and Monitoring

#### A. Usage Tracker (`pkg/metrics/usage_tracker.go`)

```go
package metrics

import (
    "encoding/json"
    "fmt"
    "sync"
    "time"
    
    "github.com/your-kubectl-ai/gollm"
)

type UsageTracker struct {
    mu       sync.RWMutex
    metrics  map[string]*ModelMetrics
    storage  UsageStorage
}

type ModelMetrics struct {
    Provider         string    `json:"provider"`
    Model           string    `json:"model"`
    RequestCount    int64     `json:"requestCount"`
    TotalInputTokens int64     `json:"totalInputTokens"`
    TotalOutputTokens int64    `json:"totalOutputTokens"`
    TotalCost       float64   `json:"totalCost"`
    AverageLatency  float64   `json:"averageLatency"`
    LastUsed        time.Time `json:"lastUsed"`
}

type UsageStorage interface {
    Store(key string, metrics *ModelMetrics) error
    Load(key string) (*ModelMetrics, error)
    List() (map[string]*ModelMetrics, error)
}

func NewUsageTracker() *UsageTracker {
    return &UsageTracker{
        metrics: make(map[string]*ModelMetrics),
        storage: NewMemoryStorage(), // or NewDatabaseStorage()
    }
}

func (ut *UsageTracker) OnUsage(provider, model string, usage gollm.Usage) {
    ut.mu.Lock()
    defer ut.mu.Unlock()
    
    key := fmt.Sprintf("%s:%s", provider, model)
    
    if metrics, exists := ut.metrics[key]; exists {
        metrics.RequestCount++
        metrics.TotalInputTokens += int64(usage.InputTokens)
        metrics.TotalOutputTokens += int64(usage.OutputTokens)
        metrics.TotalCost += usage.TotalCost
        metrics.LastUsed = usage.Timestamp
    } else {
        ut.metrics[key] = &ModelMetrics{
            Provider:          provider,
            Model:            model,
            RequestCount:     1,
            TotalInputTokens: int64(usage.InputTokens),
            TotalOutputTokens: int64(usage.OutputTokens),
            TotalCost:        usage.TotalCost,
            LastUsed:         usage.Timestamp,
        }
    }
    
    // Persist to storage
    if ut.storage != nil {
        ut.storage.Store(key, ut.metrics[key])
    }
}

func (ut *UsageTracker) GetMetrics() map[string]*ModelMetrics {
    ut.mu.RLock()
    defer ut.mu.RUnlock()
    
    result := make(map[string]*ModelMetrics)
    for k, v := range ut.metrics {
        result[k] = v
    }
    return result
}

func (ut *UsageTracker) GetMetricsJSON() ([]byte, error) {
    metrics := ut.GetMetrics()
    return json.Marshal(metrics)
}
```

### 5. Middleware Integration

#### A. Usage Middleware (`pkg/middleware/usage_middleware.go`)

```go
package middleware

import (
    "context"
    "net/http"
    "time"
    
    "github.com/your-org/go-llm-apps/pkg/metrics"
)

type UsageMiddleware struct {
    tracker *metrics.UsageTracker
}

func NewUsageMiddleware(tracker *metrics.UsageTracker) *UsageMiddleware {
    return &UsageMiddleware{
        tracker: tracker,
    }
}

func (um *UsageMiddleware) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        
        // Add usage tracker to context
        ctx := context.WithValue(r.Context(), "usage_tracker", um.tracker)
        r = r.WithContext(ctx)
        
        // Process request
        next.ServeHTTP(w, r)
        
        // Log request duration
        duration := time.Since(start)
        // You can add additional request-level metrics here
        _ = duration
    })
}

func (um *UsageMiddleware) MetricsHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        metrics, err := um.tracker.GetMetricsJSON()
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        
        w.Header().Set("Content-Type", "application/json")
        w.Write(metrics)
    }
}
```

### 6. Environment Configuration

Create a `.env` or configuration file for environment variables:

```bash
# LLM Configuration
LLM_PROVIDER=bedrock
LLM_MODEL=anthropic.claude-3-sonnet-20240229-v1:0
AWS_REGION=us-east-1

# Inference Configuration
LLM_TEMPERATURE=0.1
LLM_MAX_TOKENS=64000
LLM_TOP_P=0.1
LLM_TOP_K=1
LLM_MAX_RETRIES=10
LLM_DEBUG=false

# Usage Tracking
USAGE_TRACKING_ENABLED=true
USAGE_STORAGE_BACKEND=memory
METRICS_ENDPOINT=/metrics
REPORTING_INTERVAL=5m

# Server Configuration
SERVER_PORT=8080
SERVER_HOST=0.0.0.0
```

### 7. Main Application Integration

#### A. Application Entry Point (`cmd/webserver/main.go`)

```go
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/gorilla/mux"
    "github.com/your-org/go-llm-apps/pkg/config"
    "github.com/your-org/go-llm-apps/pkg/middleware"
    "github.com/your-org/go-llm-apps/pkg/metrics"
)

func main() {
    // Load configuration
    cfg := config.LoadLLMConfig()
    
    // Create usage tracker
    tracker := metrics.NewUsageTracker()
    
    // Create webserver
    server, err := NewWebServer()
    if err != nil {
        log.Fatalf("Failed to create webserver: %v", err)
    }
    
    // Setup middleware
    usageMiddleware := middleware.NewUsageMiddleware(tracker)
    
    // Setup routes
    router := mux.NewRouter()
    router.Use(usageMiddleware.Middleware)
    
    // API routes
    api := router.PathPrefix("/api/v1").Subrouter()
    api.HandleFunc("/kubectl/query", server.handleKubectlQuery).Methods("POST")
    api.HandleFunc("/usage/metrics", usageMiddleware.MetricsHandler()).Methods("GET")
    
    // Health check
    router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    }).Methods("GET")
    
    // Start server
    serverAddr := fmt.Sprintf("%s:%s", 
        config.getEnvOrDefault("SERVER_HOST", "0.0.0.0"),
        config.getEnvOrDefault("SERVER_PORT", "8080"))
    
    srv := &http.Server{
        Addr:    serverAddr,
        Handler: router,
    }
    
    // Graceful shutdown
    go func() {
        sigChan := make(chan os.Signal, 1)
        signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
        <-sigChan
        
        log.Println("Shutting down server...")
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        
        if err := srv.Shutdown(ctx); err != nil {
            log.Printf("Server shutdown error: %v", err)
        }
    }()
    
    log.Printf("Server starting on %s", serverAddr)
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("Server failed to start: %v", err)
    }
}
```

### 8. Testing Strategy

#### A. Integration Tests

```go
package integration_test

import (
    "context"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/your-kubectl-ai/gollm"
    "github.com/your-org/go-llm-apps/pkg/agent"
    "github.com/your-org/go-llm-apps/pkg/config"
)

func TestGollmIntegration(t *testing.T) {
    // Test configuration
    cfg := &config.InferenceConfig{
        Model:       "anthropic.claude-3-sonnet-20240229-v1:0",
        Region:      "us-east-1",
        Temperature: 0.7,
        MaxTokens:   1000,
    }
    
    // Mock usage callback
    var receivedUsage *gollm.Usage
    usageCallback := func(provider, model string, usage gollm.Usage) {
        receivedUsage = &usage
    }
    
    // Create gollm client
    client, err := gollm.NewClient(
        context.Background(),
        "bedrock",
        gollm.WithInferenceConfig(&gollm.InferenceConfig{
            Model:       cfg.Model,
            Temperature: cfg.Temperature,
            MaxTokens:   cfg.MaxTokens,
        }),
        gollm.WithUsageCallback(usageCallback),
    )
    require.NoError(t, err)
    
    // Create agent
    agent := agent.NewKubectlAgent(client, cfg)
    
    // Test query processing
    response, err := agent.ProcessQuery(context.Background(), "List all pods in default namespace")
    require.NoError(t, err)
    assert.NotEmpty(t, response.Content)
    assert.NotNil(t, receivedUsage)
    assert.Greater(t, receivedUsage.TotalTokens, 0)
}
```

### 9. Documentation and Examples

#### A. Usage Examples

Create examples showing how to use the integrated system:

```go
// Example: Simple kubectl query
func ExampleSimpleQuery() {
    cfg := config.LoadLLMConfig()
    
    client, _ := gollm.NewClient(
        context.Background(),
        cfg.Provider,
        gollm.WithInferenceConfig(cfg.InferenceConfig),
    )
    
    agent := agent.NewKubectlAgent(client, cfg.InferenceConfig)
    response, _ := agent.ProcessQuery(context.Background(), "How do I create a pod?")
    
    fmt.Println(response.Content)
}

// Example: Usage tracking
func ExampleUsageTracking() {
    tracker := metrics.NewUsageTracker()
    
    client, _ := gollm.NewClient(
        context.Background(),
        "bedrock",
        gollm.WithUsageCallback(tracker.OnUsage),
    )
    
    // After using the client...
    metrics := tracker.GetMetrics()
    for key, metric := range metrics {
        fmt.Printf("Model: %s, Requests: %d, Cost: $%.2f\n", 
            key, metric.RequestCount, metric.TotalCost)
    }
}
```

## Implementation Checklist

- [ ] Create configuration management for inference settings
- [ ] Implement usage tracking and aggregation
- [ ] Set up middleware for request/response handling
- [ ] Create agent layer with gollm client integration
- [ ] Implement metrics collection and reporting
- [ ] Add comprehensive error handling
- [ ] Create integration tests
- [ ] Add monitoring and health checks
- [ ] Document API endpoints and usage
- [ ] Set up environment configuration

## Key Benefits

1. **Clean Architecture**: Separation of concerns with clear layers
2. **Configuration Management**: Centralized inference configuration
3. **Usage Tracking**: Comprehensive usage metrics and cost tracking
4. **Scalability**: Support for multiple models and providers
5. **Monitoring**: Built-in metrics and health checks
6. **Testability**: Comprehensive test coverage for integration points
7. **Maintainability**: Clear interfaces and modular design

## Integration Verification

After implementation, verify the integration by:

1. **Configuration Loading**: Ensure inference config is properly loaded and applied
2. **Usage Collection**: Verify usage data flows through streaming responses
3. **Metrics Reporting**: Check that usage metrics are accurately calculated and stored
4. **Error Handling**: Test error scenarios and graceful degradation
5. **Performance**: Validate that integration doesn't impact response times significantly

This architecture provides a robust foundation for integrating the kubectl-ai/gollm package into your go-llm-apps repository with comprehensive usage tracking and configuration management. 