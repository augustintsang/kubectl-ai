# kubectl-ai Dependency Analysis: Safe Modification Impact Assessment

## Executive Summary

This document provides a comprehensive analysis of how the **Nirmata go-llm-apps** repository currently depends on the **kubectl-ai** repository. The goal is to understand all integration points, API usage patterns, and critical dependencies to ensure that modifications to kubectl-ai do not break existing functionality in go-llm-apps. This analysis focuses on actual usage rather than potential enhancements.

## Table of Contents

1. [Current kubectl-ai Dependencies](#current-kubectl-ai-dependencies)
2. [Critical Integration Points](#critical-integration-points)
3. [API Usage Patterns and Interfaces](#api-usage-patterns-and-interfaces)
4. [Version Dependencies and Fork Status](#version-dependencies-and-fork-status)
5. [Safe Modification Guidelines](#safe-modification-guidelines)
6. [Breaking Change Impact Assessment](#breaking-change-impact-assessment)

---

## Current kubectl-ai Dependencies

### 1. Direct Import Dependencies

#### **gollm Client Library** - CRITICAL DEPENDENCY
- **Import Path**: `github.com/GoogleCloudPlatform/kubectl-ai/gollm`
- **Usage Scope**: Used in ALL applications and core functionality
- **Key APIs Used**:
  ```go
  // Core client creation and configuration
  gollm.NewClient(ctx, provider, ...options)
  gollm.WithInferenceConfig(config)
  gollm.WithUsageCallback(callback)
  gollm.WithDebug(debug)
  
  // Chat and conversation management
  client.StartChat(systemPrompt, model)
  gollm.NewRetryChat(chat, retryConfig)
  chat.SendStreaming(ctx, messages...)
  chat.SetFunctionDefinitions(definitions)
  
  // Configuration structures
  gollm.InferenceConfig{Model, Temperature, MaxTokens, etc.}
  gollm.RetryConfig{MaxAttempts, Backoff, etc.}
  ```
- **Files Dependent**: 
  - `pkg/apps/run.go` (line 31)
  - `cmd/webserver/handlers.go` (lines 19, 78-84)
  - `benchmarks/main.go` (lines 470-485)
  - `samples/remediate-app/main.go` (lines 42-45)
  - `samples/react-agent/main.go` (lines 63-77)
- **Breaking Change Risk**: **CRITICAL** - Any changes to gollm interfaces will break core functionality

#### **UI and Document System** - HIGH DEPENDENCY
- **Import Path**: `github.com/GoogleCloudPlatform/kubectl-ai/pkg/ui`
- **Usage Scope**: Agent interactions, terminal interfaces, streaming responses
- **Key APIs Used**:
  ```go
  // Document and UI management
  ui.NewDocument()
  ui.NewTerminalUI(doc, recorder, streaming)
  ui.NewAgentTextBlock()
  ui.NewErrorBlock()
  
  // Block management and streaming
  agentTextBlock.SetStreaming(true)
  agentTextBlock.SetText(content)
  doc.AddBlock(block)
  ```
- **Files Dependent**:
  - `pkg/agent/agent.go` (lines 7, 25)
  - `pkg/agent/conversation.go` (lines 9, 88-92, 116-120)
  - `pkg/apps/run.go` (lines 3, 63)
  - `samples/react-agent/main.go` (lines 73-78)
- **Breaking Change Risk**: **HIGH** - Changes to UI interfaces affect user experience

#### **Tool System and MCP** - HIGH DEPENDENCY
- **Import Paths**: 
  - `github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools`
  - `github.com/GoogleCloudPlatform/kubectl-ai/pkg/mcp`
- **Usage Scope**: Tool registration, MCP server integration, function calling
- **Key APIs Used**:
  ```go
  // Tool management
  tools.RegisterTool(tool)
  tools.Lookup(toolName)
  tools.Default()
  tools.ConvertToolToGollm(mcpTool)
  tools.NewMCPTool(serverName, toolName, description, schema, manager)
  tools.NewCustomTool(config)
  
  // MCP integration
  mcp.NewManager(config)
  mcp.Config{Servers: []mcp.ServerConfig{...}}
  manager.RegisterWithToolSystem(ctx, callback)
  manager.DiscoverAndConnectServers(ctx)
  ```
- **Files Dependent**:
  - `pkg/agent/tools.go` (lines 6-7, 14-47)
  - `pkg/agent/mcp.go` (lines 6, 173-end)
  - `pkg/agent/conversation.go` (lines 10, 233-270)
  - `samples/react-agent/main.go` (lines 98-118)
  - `samples/react-agent/tools.go` (lines 30-40)
- **Breaking Change Risk**: **HIGH** - Tool system is core to agent functionality

#### **Journal and Logging** - MEDIUM DEPENDENCY
- **Import Path**: `github.com/GoogleCloudPlatform/kubectl-ai/pkg/journal`
- **Usage Scope**: Request/response recording, debugging, audit trails
- **Key APIs Used**:
  ```go
  // Journal recording
  journal.LogRecorder{}
  journal.ContextWithRecorder(ctx, recorder)
  recorder.Close()
  ```
- **Files Dependent**:
  - `samples/remediate-app/main.go` (lines 27-35)
  - `samples/react-agent/main.go` (lines 46-52)
- **Breaking Change Risk**: **MEDIUM** - Used in samples, affects debugging capabilities

### 2. Indirect Dependencies and Data Structures

#### **gollm Data Types** - CRITICAL DEPENDENCY
These types are used throughout the codebase:
```go
// Function calling and tool integration
gollm.FunctionDefinition{Name, Description, Parameters}
gollm.FunctionCall{ID, Name, Arguments}
gollm.FunctionCallResult{ID, Name, Result}
gollm.Schema{Type, Properties, Required, etc.}

// Streaming and response handling
gollm.Chat interface with Send/SendStreaming methods
gollm.Client interface with StartChat method
```
**Breaking Change Risk**: **CRITICAL** - These types are embedded in application logic

#### **Provider Bedrock Integration** - HIGH DEPENDENCY
- **Import Path**: `github.com/GoogleCloudPlatform/kubectl-ai/gollm/bedrock` (via blank import)
- **Usage**: Provider registration for AWS Bedrock
- **Location**: `cmd/webserver/handlers.go` (line 20)
- **Breaking Change Risk**: **HIGH** - Default provider for production workloads

---

## Critical Integration Points

### 1. **Application Framework Dependencies**

#### **AppRunner Integration** - CRITICAL
The core application runner pattern depends heavily on kubectl-ai:
```go
// pkg/apps/run.go lines 63-69
doc := ui.NewDocument()
ag := agent.NewAgent(systemPrompt, llmClient, model, doc)
conversation, err := ag.NewConversation(ctx, doc)
```
**Critical APIs**:
- Agent creation and conversation management
- UI document lifecycle
- Streaming conversation interfaces

#### **Conversation Loop** - CRITICAL  
The agentic conversation pattern is fundamental:
```go
// pkg/agent/conversation.go lines 41-61
c.llmChat = gollm.NewRetryChat(
    c.llm.StartChat(c.systemPrompt, c.model),
    gollm.RetryConfig{...}
)
```
**Critical APIs**:
- Chat session management
- Retry configuration
- Function calling integration

### 2. **Tool System Integration Points**

#### **Tool Registration Pattern** - HIGH DEPENDENCY
```go
// pkg/agent/tools.go lines 14-18
func RegisterTools(toolsToRegister ...tools.Tool) {
    for _, tool := range toolsToRegister {
        tools.RegisterTool(tool)
    }
}
```
**Critical APIs**:
- `tools.Tool` interface
- `tools.RegisterTool()` function
- Tool lookup mechanisms

#### **MCP Server Integration** - HIGH DEPENDENCY
```go
// pkg/agent/tools.go lines 21-47
manager := mcp.NewManager(mcpConfig)
manager.RegisterWithToolSystem(ctx, callback)
manager.DiscoverAndConnectServers(ctx)
```
**Critical APIs**:
- MCP manager lifecycle
- Tool registration callbacks
- Server discovery protocols

### 3. **Bedrock Provider Dependencies**

#### **Provider Configuration** - HIGH DEPENDENCY
All applications depend on Bedrock provider functionality:
```go
// cmd/webserver/handlers.go lines 78-84
llmClient, err := gollm.NewClient(clientCtx, providerName,
    gollm.WithInferenceConfig(inferenceCfg),
    gollm.WithUsageCallback(usageCallback.UsageCallback),
    gollm.WithDebug(h.debug),
)
```
**Critical Provider Features**:
- AWS Bedrock authentication
- Inference configuration
- Usage callbacks and metrics
- Multi-region support

---

## API Usage Patterns and Interfaces

### 1. **gollm Client Usage Patterns**

#### **Client Creation Pattern** - Used in 6+ files
```go
// Standard client creation pattern
llmClient, err := gollm.NewClient(ctx, providerName,
    gollm.WithInferenceConfig(inferenceCfg),
    gollm.WithUsageCallback(usageCallback.UsageCallback),
    gollm.WithDebug(h.debug),
)
```
**Critical Options Used**:
- `gollm.WithInferenceConfig()` - ALL applications require this
- `gollm.WithUsageCallback()` - Used for metrics and billing
- `gollm.WithDebug()` - Used for development and troubleshooting
- `gollm.WithSkipVerifySSL()` - Used in samples for testing

#### **Inference Configuration Pattern** - Used everywhere
```go
inferenceConfig := &gollm.InferenceConfig{
    Model:       cfg.Benchmark.ModelArn,
    Temperature: 0.1,
    MaxTokens:   2048,
    MaxRetries:  3,
}
```
**Required Fields**: Model, Temperature, MaxTokens, MaxRetries
**Breaking Change Risk**: Changes to this struct will break all applications

#### **Chat Session Management** - Core conversation pattern
```go
// Agent conversation initialization
c.llmChat = gollm.NewRetryChat(
    c.llm.StartChat(c.systemPrompt, c.model),
    gollm.RetryConfig{
        MaxAttempts:    3,
        InitialBackoff: 10 * time.Second,
        MaxBackoff:     60 * time.Second,
        BackoffFactor:  2,
        Jitter:         true,
    },
)
```

### 2. **Tool System API Patterns**

#### **Tool Interface Requirements**
All custom tools must implement this interface:
```go
type Tool interface {
    Name() string
    Description() string
    FunctionDefinition() *gollm.FunctionDefinition
    IsInteractive(args map[string]any) (bool, error)
    CheckModifiesResource(args map[string]any) string
    Run(ctx context.Context, args map[string]any) (any, error)
}
```

#### **Function Definition Pattern**
```go
// Used in tool registration throughout samples/
definition := tool.FunctionDefinition()
functionDefinitions = append(functionDefinitions, definition)
```

#### **MCP Tool Creation Pattern**
```go
// pkg/agent/tools.go lines 30-35
mcpTool := tools.NewMCPTool(serverName, toolInfo.Name, 
    toolInfo.Description, schema, manager)
tools.RegisterTool(mcpTool)
```

### 3. **UI System API Patterns**

#### **Document and Block Management**
```go
// Standard UI initialization pattern
doc := ui.NewDocument()
agentTextBlock := ui.NewAgentTextBlock()
agentTextBlock.SetStreaming(true)
doc.AddBlock(agentTextBlock)
```

#### **Terminal UI Pattern** - Used in samples
```go
userInterface, err := ui.NewTerminalUI(doc, recorder, true)
defer userInterface.Close()
```

#### **Streaming Response Pattern**
```go
// pkg/agent/conversation.go lines 116-120
agentTextBlock.SetStreaming(true)
doc.AddBlock(agentTextBlock)
stream, err := conversation.llmChat.SendStreaming(ctx, currChatContent...)
```

---

## Version Dependencies and Fork Status

### 1. **Current Fork Configuration**

#### **go.mod Replace Directives**
```go
// Current dependency overrides
replace github.com/mark3labs/mcp-go v0.32.0 => github.com/mark3labs/mcp-go v0.31.0

replace github.com/GoogleCloudPlatform/kubectl-ai => github.com/augustintsang/kubectl-ai v0.0.0-20250703013351-699897349d92

replace github.com/GoogleCloudPlatform/kubectl-ai/gollm => github.com/augustintsang/kubectl-ai/gollm v0.0.0-20250703013351-699897349d92
```

#### **Critical Version Dependencies**
```go
require (
    github.com/GoogleCloudPlatform/kubectl-ai v0.0.15-0.20250625173409-5e2e4b7b3c72
    github.com/GoogleCloudPlatform/kubectl-ai/gollm v0.0.0-00010101000000-000000000000
    // ... other dependencies
)
```

### 2. **Fork Dependency Analysis**

#### **augustintsang/kubectl-ai Fork** - CRITICAL DEPENDENCY
- **Commit Hash**: `v0.0.0-20250703013351-699897349d92`
- **Upstream Version**: Based on older kubectl-ai version
- **Risk Level**: **CRITICAL** - Fork may have custom changes
- **Impact**: Any changes to kubectl-ai that conflict with fork modifications will break builds

#### **MCP Version Downgrade** - HIGH DEPENDENCY
- **Current**: Downgraded from v0.32.0 to v0.31.0
- **Reason**: Compatibility issues with older kubectl-ai fork
- **Risk Level**: **HIGH** - Version conflicts may occur

### 3. **Compatibility Requirements**

#### **Go Version Requirements**
- **Current**: `go 1.24.3`
- **kubectl-ai Requirement**: Must remain compatible
- **Breaking Change Risk**: Go version bumps in kubectl-ai

#### **Third-Party Dependencies**
Key shared dependencies that must remain compatible:
- `k8s.io/klog/v2 v2.130.1`
- `github.com/google/uuid v1.6.0`
- `github.com/pkg/errors v0.9.1`
- `gopkg.in/yaml.v3 v3.0.1`

### 4. **Build and Runtime Dependencies**

#### **Provider Registration** - Runtime Critical
```go
// cmd/webserver/handlers.go line 20
_ "github.com/GoogleCloudPlatform/kubectl-ai/gollm/bedrock"
```
**Critical**: This blank import must remain for Bedrock provider registration

#### **Container Build Dependencies**
- **Registry**: Uses `ghcr.io/nirmata/go-llm-apps`
- **Build Tool**: Uses `ko` for container builds
- **Runtime**: Depends on kubectl-ai libraries being available in container

---

## Safe Modification Guidelines

### 1. **Critical APIs That Must Not Change**

#### **gollm.Client Interface** - DO NOT MODIFY
```go
type Client interface {
    StartChat(systemPrompt, model string) Chat
}
```
**Usage**: Core to all applications - used in 6+ files
**Impact**: Breaking this interface will stop all applications from functioning

#### **gollm.Chat Interface** - DO NOT MODIFY
```go
type Chat interface {
    Send(ctx context.Context, message ...any) (*ChatResponse, error)
    SendStreaming(ctx context.Context, message ...any) (<-chan ChatResponse, error)
    SetFunctionDefinitions(definitions []*FunctionDefinition) error
}
```
**Usage**: Core conversation functionality
**Impact**: Used in agent conversation loops - critical for streaming

#### **Configuration Structs** - PRESERVE FIELDS
```go
// These fields are required and cannot be removed
type InferenceConfig struct {
    Model       string  // REQUIRED - used everywhere
    Temperature float32 // REQUIRED - used everywhere  
    MaxTokens   int32   // REQUIRED - used everywhere
    MaxRetries  int     // REQUIRED - used everywhere
    // Adding new fields is safe, removing is not
}
```

### 2. **Safe Modification Patterns**

#### **Adding New Fields** ✅ SAFE
```go
// Adding to structs is safe if done carefully
type InferenceConfig struct {
    Model       string  // existing required field
    Temperature float32 // existing required field
    NewField    string  `json:"newField,omitempty"` // SAFE: optional field
}
```

#### **Adding New Methods** ✅ SAFE
```go
// Adding methods to interfaces is safe if done as new interfaces
type ExtendedClient interface {
    Client  // embed existing interface
    NewMethod() error  // SAFE: new functionality
}
```

#### **Adding New Options** ✅ SAFE
```go
// Adding new options is safe
func WithNewOption(value string) ClientOption {
    return func(c *client) { c.newField = value }
}
```

### 3. **High-Risk Modification Areas**

#### **Function Signatures** 🔴 HIGH RISK
```go
// DO NOT CHANGE these function signatures
gollm.NewClient(ctx, provider, ...options) (Client, error)
gollm.NewRetryChat(chat, config) Chat
gollm.WithInferenceConfig(config) ClientOption
```

#### **Error Types** 🔴 HIGH RISK
Any changes to error types or error messages that code depends on for flow control

#### **Provider Registration** 🔴 HIGH RISK
```go
// This blank import pattern must continue to work
_ "github.com/GoogleCloudPlatform/kubectl-ai/gollm/bedrock"
```

### 4. **Safe Modification Process**

#### **Step 1: Analyze Usage**
Before modifying kubectl-ai:
1. Search go-llm-apps codebase for the component being changed
2. Identify all usage patterns
3. Document breaking change impact

#### **Step 2: Backward Compatibility**
- Add new functionality alongside existing APIs
- Deprecate old APIs instead of removing immediately
- Provide migration guides for breaking changes

#### **Step 3: Testing Strategy**
- Run go-llm-apps tests after kubectl-ai changes
- Test with current fork configuration
- Verify container builds continue to work

---

## Breaking Change Impact Assessment

### 1. **Critical Breaking Changes** 🔴 

#### **gollm Interface Changes** - CATASTROPHIC IMPACT
**Scenarios**:
- Changing `gollm.NewClient()` signature
- Modifying `Chat.SendStreaming()` return types
- Removing fields from `InferenceConfig`

**Impact Analysis**:
```
Affected Files: 15+ files across entire codebase
Build Impact: Complete build failure
Runtime Impact: All applications unusable
Recovery Time: 2-4 weeks of development
```

**Files That Will Break**:
- `pkg/apps/run.go` - Core application runner
- `cmd/webserver/handlers.go` - Web service layer
- `benchmarks/main.go` - Benchmark framework
- All sample applications

#### **Tool System Changes** - HIGH IMPACT
**Scenarios**:
- Changing `tools.Tool` interface
- Modifying `tools.RegisterTool()` signature
- Breaking MCP integration APIs

**Impact Analysis**:
```
Affected Files: 8+ files in agent and samples
Build Impact: Agent system failure
Runtime Impact: No tool functionality, no MCP
Recovery Time: 1-2 weeks of development
```

#### **UI System Changes** - HIGH IMPACT
**Scenarios**:
- Removing UI component methods
- Changing document lifecycle APIs
- Breaking streaming interfaces

**Impact Analysis**:
```
Affected Files: 4+ files
Build Impact: Agent and sample failures
Runtime Impact: No user interface, no streaming
Recovery Time: 1 week of development
```

### 2. **Medium Risk Breaking Changes** 🟡

#### **Provider System Changes** - MEDIUM IMPACT
**Scenarios**:
- Changing provider registration patterns
- Modifying Bedrock provider interfaces
- Breaking authentication flows

**Impact Analysis**:
```
Affected Files: 3-5 files
Build Impact: Partial failure
Runtime Impact: Provider-specific failures
Recovery Time: 2-5 days of development
```

#### **Configuration Changes** - MEDIUM IMPACT
**Scenarios**:
- Changing configuration struct fields
- Modifying client option patterns
- Breaking retry configurations

**Impact Analysis**:
```
Affected Files: 5+ files
Build Impact: Configuration errors
Runtime Impact: Application misconfiguration
Recovery Time: 1-3 days of development
```

### 3. **Acceptable Changes** ✅

#### **Additive Changes** - LOW IMPACT
- Adding new client options
- Adding new provider support
- Adding new UI components
- Adding new tool types

#### **Internal Implementation Changes** - NO IMPACT
- Refactoring internal code that doesn't change public APIs
- Performance improvements
- Bug fixes that don't change interfaces

### 4. **Fork-Specific Risks** 🔴

#### **augustintsang Fork Conflicts** - CRITICAL RISK
Current fork is based on older kubectl-ai version:
- Fork may have custom modifications
- Upstream changes may conflict with fork changes
- Version compatibility issues

**Mitigation Requirements**:
1. **Before Making Changes**: Compare fork with upstream
2. **Identify Custom Code**: Document fork-specific modifications
3. **Test Compatibility**: Verify changes work with fork version
4. **Coordinate Updates**: Plan fork synchronization strategy

### 5. **Testing Requirements for kubectl-ai Changes**

#### **Mandatory Testing Checklist**
Before releasing kubectl-ai changes:

- [ ] **Build Test**: `go build ./...` in go-llm-apps
- [ ] **Unit Tests**: Run go-llm-apps test suite
- [ ] **Integration Test**: Test with current fork configuration
- [ ] **Sample Applications**: Verify all samples still work
- [ ] **Container Build**: Verify `ko` builds still work
- [ ] **Benchmarks**: Verify benchmark framework functions

#### **Critical Test Cases**
```bash
# Test core functionality
cd go-llm-apps
go test ./pkg/apps/...
go test ./pkg/agent/...

# Test samples
cd samples/react-agent && go build .
cd samples/remediate-app && go build .

# Test container build
make ko-build

# Test with current dependencies
go mod verify && go mod tidy
```

---

## Summary and Recommendations

### **For kubectl-ai Maintainers**

#### **DO**:
- ✅ Add new optional fields to structs
- ✅ Add new methods to new interfaces (composition pattern)
- ✅ Add new client options using functional options pattern
- ✅ Provide migration guides for any breaking changes
- ✅ Test changes against go-llm-apps before release

#### **DON'T**:
- 🔴 Remove or change existing interface methods
- 🔴 Change function signatures of core APIs
- 🔴 Remove fields from configuration structs
- 🔴 Break provider registration patterns
- 🔴 Change import paths without deprecation period

### **Current Dependency Status**
- **Fork Dependency**: Critical risk due to augustintsang fork
- **API Surface**: Large surface area with deep integration
- **Breaking Change Risk**: Very high due to extensive usage
- **Recovery Effort**: Potentially weeks for major breaking changes

### **Recommendation for Safe Evolution**
1. **Gradual Migration**: Help go-llm-apps migrate off fork
2. **Backward Compatibility**: Maintain compatibility for at least 2 versions
3. **Early Communication**: Notify about breaking changes in advance
4. **Testing Partnership**: Include go-llm-apps in kubectl-ai CI/testing

This dependency analysis shows that go-llm-apps is heavily integrated with kubectl-ai and breaking changes require careful coordination to avoid significant downtime and development effort. 