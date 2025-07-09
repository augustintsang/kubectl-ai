# Architecture Guide: go-llm-apps Integration with kubectl-ai/gollm

## Overview

This guide provides the architecture and implementation approach for structuring the `go-llm-apps` repository to integrate seamlessly with the `kubectl-ai/gollm` package, enabling comprehensive usage metrics tracking and inference configuration management.

## Repository Structure

```
go-llm-apps/
├── cmd/
│   └── webserver/
│       ├── main.go                 # Main application entry point
│       └── handlers.go             # HTTP handlers with gollm integration
├── pkg/
│   ├── agent/
│   │   ├── conversation.go         # Conversation management with usage tracking
│   │   ├── types.go               # Agent-specific types and interfaces
│   │   └── metrics.go             # Usage metrics collection
│   ├── apps/
│   │   ├── manager.go             # Application-wide usage aggregation
│   │   ├── storage.go             # Usage data persistence
│   │   └── reporting.go           # Metrics reporting and analytics
│   ├── config/
│   │   ├── inference.go           # Inference configuration management
│   │   ├── providers.go           # LLM provider configurations
│   │   └── loader.go              # Configuration loading utilities
│   └── middleware/
│       ├── metrics.go             # HTTP middleware for request tracking
│       ├── logging.go             # Request/response logging
│       └── cors.go                # CORS handling
├── internal/
│   ├── models/
│   │   ├── usage.go               # Usage data models
│   │   ├── conversation.go        # Conversation models
│   │   └── metrics.go             # Metrics aggregation models
│   └── storage/
│       ├── interfaces.go          # Storage interfaces
│       ├── memory.go              # In-memory storage implementation
│       └── file.go                # File-based storage implementation
├── configs/
│   ├── inference.yaml             # Default inference configurations
│   └── providers.yaml             # LLM provider settings
├── .env.example                   # Environment configuration template
└── README.md                      # Repository documentation
```

## 1. Application Layer Structure

### Main Application Integration

The main application creates gollm clients with proper configuration and usage tracking:

```go
// Main webserver setup with usage tracking
func main() {
    ctx := context.Background()
    
    // Load inference configurations
    cfg, err := config.LoadConfig()
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }
    
    // Initialize usage manager for metrics collection
    usageManager := apps.NewUsageManager(cfg.Storage)
    
    // Initialize agent manager with gollm integration
    agentManager := agent.NewManager(cfg.Inference, usageManager)
    
    // Setup HTTP server with middleware
    router := setupRoutes(agentManager, usageManager)
    
    // Start server with graceful shutdown
    startServer(router, cfg.Server.Port)
}
```

### HTTP Handlers with gollm Integration

Handlers create gollm clients with configuration and process kubectl queries:

```go
// ChatHandler processes kubectl queries using gollm
func (h *Handlers) ChatHandler(c *gin.Context) {
    var req ChatRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // Create conversation with gollm client
    conversation, err := h.agentManager.CreateConversation(
        c.Request.Context(), 
        req.ConversationID, 
        req.Provider)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    // Process kubectl query with usage tracking
    response, err := conversation.ProcessQuery(c.Request.Context(), req.Query)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, response)
}
```

## 2. Agent Layer with gollm Integration

### Conversation Management

The agent layer creates and manages gollm clients with proper configuration:

```go
// Conversation wraps gollm client with usage tracking
type Conversation struct {
    ID              string
    Provider        string
    client          gollm.Client
    chat            gollm.ChatSession
    usageCallback   func(models.Usage)
    inferenceConfig *config.InferenceConfig
}

// NewConversation creates a gollm client with configuration
func NewConversation(ctx context.Context, id, provider string, cfg *config.InferenceConfig, usageCallback func(models.Usage)) (*Conversation, error) {
    // Create usage callback for gollm
    gollmUsageCallback := func(providerName, model string, usage gollm.Usage) {
        // Convert gollm.Usage to internal models.Usage
        internalUsage := models.Usage{
            ConversationID: id,
            Provider:       providerName,
            Model:          model,
            InputTokens:    usage.InputTokens,
            OutputTokens:   usage.OutputTokens,
            TotalTokens:    usage.TotalTokens,
            InputCost:      usage.InputCost,
            OutputCost:     usage.OutputCost,
            TotalCost:      usage.TotalCost,
            Timestamp:      usage.Timestamp,
        }
        
        usageCallback(internalUsage)
    }
    
    // Create gollm client with configuration
    client, err := gollm.NewClient(ctx, provider,
        gollm.WithInferenceConfig(&gollm.InferenceConfig{
            Model:       cfg.Model,
            Region:      cfg.Region,
            Temperature: cfg.Temperature,
            MaxTokens:   cfg.MaxTokens,
            TopP:        cfg.TopP,
            TopK:        cfg.TopK,
            MaxRetries:  cfg.MaxRetries,
        }),
        gollm.WithUsageCallback(gollmUsageCallback),
        gollm.WithDebug(cfg.Debug),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create gollm client: %w", err)
    }
    
    // Start chat session
    chat := client.StartChat("You are a helpful kubectl assistant.", cfg.Model)
    
    return &Conversation{
        ID:              id,
        Provider:        provider,
        client:          client,
        chat:            chat,
        usageCallback:   usageCallback,
        inferenceConfig: cfg,
    }, nil
}
```

### Query Processing with Usage Extraction

Process kubectl queries and extract usage metadata from gollm responses:

```go
// ProcessQuery sends query to gollm and extracts usage
func (c *Conversation) ProcessQuery(ctx context.Context, query string) (*models.Response, error) {
    // Send query to gollm
    response, err := c.chat.Send(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to send query: %w", err)
    }
    
    // Extract response content
    content := ""
    for _, candidate := range response.Candidates() {
        content += candidate.Content()
    }
    
    // Extract usage metadata from gollm response
    var usage *models.Usage
    if usageMeta := response.UsageMetadata(); usageMeta != nil {
        gollmUsage := usageMeta.GetUsage()
        if gollmUsage != nil {
            usage = &models.Usage{
                ConversationID: c.ID,
                Provider:       usageMeta.GetProvider(),
                Model:          usageMeta.GetModel(),
                InputTokens:    gollmUsage.InputTokens,
                OutputTokens:   gollmUsage.OutputTokens,
                TotalTokens:    gollmUsage.TotalTokens,
                InputCost:      gollmUsage.InputCost,
                OutputCost:     gollmUsage.OutputCost,
                TotalCost:      gollmUsage.TotalCost,
                Timestamp:      gollmUsage.Timestamp,
            }
        }
    }
    
    return &models.Response{
        ConversationID: c.ID,
        Content:        content,
        Usage:          usage,
        Timestamp:      time.Now(),
    }, nil
}
```

## 3. Application Layer Usage Management

### Usage Aggregation and Storage

The application layer aggregates usage data across conversations:

```go
// UsageManager handles usage data aggregation
type UsageManager struct {
    storage      storage.UsageStorage
    metrics      *models.ModelMetrics
    conversations map[string]*models.ConversationUsage
}

// RecordUsage stores and aggregates usage data
func (um *UsageManager) RecordUsage(usage models.Usage) error {
    // Store individual usage record
    if err := um.storage.StoreUsage(usage); err != nil {
        return fmt.Errorf("failed to store usage: %w", err)
    }
    
    // Update conversation-level usage
    if convUsage, exists := um.conversations[usage.ConversationID]; exists {
        convUsage.AddUsage(usage)
    } else {
        um.conversations[usage.ConversationID] = models.NewConversationUsage(usage.ConversationID, usage)
    }
    
    // Update application-level metrics
    um.metrics.UpdateMetrics(usage)
    
    return nil
}

// GetAggregatedUsage returns application-wide usage statistics
func (um *UsageManager) GetAggregatedUsage() (*models.AggregatedUsage, error) {
    return &models.AggregatedUsage{
        TotalConversations: len(um.conversations),
        TotalTokens:        um.metrics.TotalTokens,
        TotalCost:          um.metrics.TotalCost,
        ProviderBreakdown:  um.metrics.ProviderBreakdown,
        ModelBreakdown:     um.metrics.ModelBreakdown,
        LastUpdated:        time.Now(),
    }, nil
}
```

## 4. Configuration Management

### Environment-based Inference Configuration

Load inference configurations from environment variables:

```go
// InferenceConfig matches gollm.InferenceConfig
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

// LoadInferenceConfigs loads provider configurations
func LoadInferenceConfigs() (map[string]*InferenceConfig, error) {
    configs := make(map[string]*InferenceConfig)
    
    // Bedrock configuration from environment
    configs["bedrock"] = &InferenceConfig{
        Model:       getEnvOrDefault("BEDROCK_MODEL", "anthropic.claude-3-sonnet-20240229-v1:0"),
        Region:      getEnvOrDefault("BEDROCK_REGION", "us-east-1"),
        Temperature: getEnvFloat32OrDefault("BEDROCK_TEMPERATURE", 0.1),
        MaxTokens:   getEnvInt32OrDefault("BEDROCK_MAX_TOKENS", 64000),
        TopP:        getEnvFloat32OrDefault("BEDROCK_TOP_P", 0.1),
        TopK:        getEnvInt32OrDefault("BEDROCK_TOP_K", 1),
        MaxRetries:  getEnvIntOrDefault("BEDROCK_MAX_RETRIES", 10),
        Debug:       getEnvBoolOrDefault("BEDROCK_DEBUG", false),
    }
    
    return configs, nil
}
```

## 5. Data Models and Metrics

### Usage Data Models

Define models for usage tracking compatible with gollm:

```go
// Usage represents token usage and cost information
type Usage struct {
    ID             string    `json:"id"`
    ConversationID string    `json:"conversation_id"`
    Provider       string    `json:"provider"`
    Model          string    `json:"model"`
    InputTokens    int       `json:"input_tokens"`
    OutputTokens   int       `json:"output_tokens"`
    TotalTokens    int       `json:"total_tokens"`
    InputCost      float64   `json:"input_cost"`
    OutputCost     float64   `json:"output_cost"`
    TotalCost      float64   `json:"total_cost"`
    Timestamp      time.Time `json:"timestamp"`
}

// ModelMetrics aggregates usage across providers and models
type ModelMetrics struct {
    TotalTokens       int                   `json:"total_tokens"`
    TotalCost         float64               `json:"total_cost"`
    ProviderBreakdown map[string]UsageStats `json:"provider_breakdown"`
    ModelBreakdown    map[string]UsageStats `json:"model_breakdown"`
}

// UpdateMetrics updates aggregated metrics with new usage
func (mm *ModelMetrics) UpdateMetrics(usage Usage) {
    mm.TotalTokens += usage.TotalTokens
    mm.TotalCost += usage.TotalCost
    
    // Update provider-specific metrics
    providerStats := mm.ProviderBreakdown[usage.Provider]
    providerStats.TotalTokens += usage.TotalTokens
    providerStats.TotalCost += usage.TotalCost
    providerStats.RequestCount++
    mm.ProviderBreakdown[usage.Provider] = providerStats
    
    // Update model-specific metrics
    modelStats := mm.ModelBreakdown[usage.Model]
    modelStats.TotalTokens += usage.TotalTokens
    modelStats.TotalCost += usage.TotalCost
    modelStats.RequestCount++
    mm.ModelBreakdown[usage.Model] = modelStats
}
```

## 6. Storage Interfaces

### Usage Storage Interface

Define storage interfaces for usage data persistence:

```go
// UsageStorage interface for persisting usage data
type UsageStorage interface {
    StoreUsage(usage models.Usage) error
    GetUsage(id string) (*models.Usage, error)
    GetUsageByConversation(conversationID string) ([]models.Usage, error)
    GetUsageByTimeRange(startTime, endTime time.Time) ([]models.Usage, error)
    GetRecentUsage(limit int) ([]models.Usage, error)
    DeleteUsage(id string) error
}

// JSON file-based storage implementation
type FileStorage struct {
    filePath string
    usage    []models.Usage
    mutex    sync.RWMutex
}

// StoreUsage persists usage data to JSON file
func (fs *FileStorage) StoreUsage(usage models.Usage) error {
    fs.mutex.Lock()
    defer fs.mutex.Unlock()
    
    fs.usage = append(fs.usage, usage)
    
    // Write to file
    data, err := json.MarshalIndent(fs.usage, "", "  ")
    if err != nil {
        return err
    }
    
    return os.WriteFile(fs.filePath, data, 0644)
}
```

## 7. Environment Configuration

Sample `.env` file for configuration:

```env
# Server Configuration
SERVER_PORT=:8080
SERVER_HOST=localhost

# Bedrock Configuration  
BEDROCK_MODEL=anthropic.claude-3-sonnet-20240229-v1:0
BEDROCK_REGION=us-east-1
BEDROCK_TEMPERATURE=0.1
BEDROCK_MAX_TOKENS=64000
BEDROCK_TOP_P=0.1
BEDROCK_TOP_K=1
BEDROCK_MAX_RETRIES=10
BEDROCK_DEBUG=false

# OpenAI Configuration
OPENAI_MODEL=gpt-4
OPENAI_API_KEY=your-openai-api-key
OPENAI_TEMPERATURE=0.1
OPENAI_MAX_TOKENS=4000
OPENAI_TOP_P=0.1
OPENAI_MAX_RETRIES=3
OPENAI_DEBUG=false

# Storage Configuration
STORAGE_TYPE=memory
STORAGE_FILE_PATH=./data/usage.json

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Metrics
METRICS_ENABLED=true
METRICS_RETENTION_DAYS=30
```

## 8. Integration Testing

### Test gollm Client Creation and Usage Tracking

```go
func TestGollmClientCreation(t *testing.T) {
    ctx := context.Background()
    
    // Setup inference config
    cfg := &config.InferenceConfig{
        Model:       "anthropic.claude-3-sonnet-20240229-v1:0",
        Region:      "us-east-1",
        Temperature: 0.7,
        MaxTokens:   1000,
    }
    
    // Setup usage callback
    var capturedUsage *models.Usage
    usageCallback := func(usage models.Usage) {
        capturedUsage = &usage
    }
    
    // Create conversation
    conv, err := NewConversation(ctx, "test-conv-1", "bedrock", cfg, usageCallback)
    require.NoError(t, err)
    require.NotNil(t, conv)
    
    // Test query processing  
    response, err := conv.ProcessQuery(ctx, "List all pods in default namespace")
    require.NoError(t, err)
    require.NotNil(t, response)
    
    // Verify usage tracking
    assert.NotNil(t, capturedUsage)
    assert.Equal(t, "test-conv-1", capturedUsage.ConversationID)
    assert.Equal(t, "bedrock", capturedUsage.Provider)
    assert.Greater(t, capturedUsage.TotalTokens, 0)
}
```

## 9. Usage Examples

### Simple kubectl Query with Usage Tracking

```go
func main() {
    ctx := context.Background()
    
    // Load configuration
    inferenceConfigs, err := config.LoadInferenceConfigs()
    if err != nil {
        log.Fatalf("Failed to load inference configs: %v", err)
    }
    
    // Setup usage tracking
    usageCallback := func(usage models.Usage) {
        fmt.Printf("Usage: %d tokens, $%.4f cost\n", usage.TotalTokens, usage.TotalCost)
    }
    
    // Create agent manager
    manager := agent.NewManager(inferenceConfigs, usageCallback)
    
    // Create conversation
    conv, err := manager.CreateConversation(ctx, "demo-conversation", "bedrock")
    if err != nil {
        log.Fatalf("Failed to create conversation: %v", err)
    }
    
    // Process kubectl query
    response, err := conv.ProcessQuery(ctx, "Show me all pods that are not running")
    if err != nil {
        log.Fatalf("Failed to process query: %v", err)
    }
    
    fmt.Printf("Response: %s\n", response.Content)
    if response.Usage != nil {
        fmt.Printf("Query used %d tokens, cost $%.4f\n", 
            response.Usage.TotalTokens, response.Usage.TotalCost)
    }
}
```

## 10. Implementation Checklist

- [ ] **Repository Structure**: Create directory structure with proper separation of concerns
- [ ] **Main Application**: Implement webserver with graceful shutdown and middleware
- [ ] **HTTP Handlers**: Create endpoints for chat, usage, and metrics with gollm integration
- [ ] **Agent Layer**: Implement conversation management with gollm client creation
- [ ] **Usage Manager**: Create comprehensive usage tracking and aggregation
- [ ] **Configuration**: Implement environment-based inference configuration loading
- [ ] **Data Models**: Define usage, conversation, and metrics models
- [ ] **Storage Layer**: Implement storage interfaces and concrete implementations  
- [ ] **Middleware**: Add request metrics, logging, and CORS handling
- [ ] **Integration Tests**: Create tests for gollm client creation and usage tracking

## Key Benefits

1. **Clean Architecture**: Separation of concerns with clear boundaries between layers
2. **Configuration Management**: Environment-based configuration with sensible defaults  
3. **Usage Tracking**: Comprehensive metrics collection and aggregation
4. **Scalability**: Modular design supports horizontal scaling
5. **Monitoring**: Built-in metrics and reporting capabilities
6. **Testability**: Comprehensive test coverage for integration points
7. **Maintainability**: Clear interfaces and dependency injection

## Integration Verification

To verify successful integration with kubectl-ai/gollm:

1. **Configuration Loading**: Verify inference configs are loaded and applied to gollm clients
2. **Usage Collection**: Confirm usage data flows through gollm streaming responses
3. **Metrics Reporting**: Test that usage metrics are aggregated and reported correctly
4. **Error Handling**: Ensure graceful handling of gollm client errors
5. **Performance**: Validate that usage tracking doesn't impact response times

This architecture provides a robust foundation for building LLM-powered applications with kubectl-ai/gollm integration, comprehensive usage metrics, and enterprise-grade scalability.
