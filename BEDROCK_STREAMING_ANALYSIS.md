# Bedrock Streaming Tool Call Detection - Deep Dive Analysis

## Executive Summary

The Bedrock streaming implementation successfully detects tool calls and logs them, but **fails to make them accessible** through the standard gollm `Parts()` → `AsFunctionCalls()` interface. This creates an inconsistency between streaming and non-streaming tool call handling.

## Problem Statement

**Observed Behavior:**
- ✅ **Non-streaming**: Tool calls are properly extracted and accessible via `AsFunctionCalls()`
- ❌ **Streaming**: Tool calls are detected and logged but return `0 parts` and `0 tool calls`
- ✅ **Both methods**: FunctionCallResult handling works correctly (separate from this issue)

## Empirical Evidence

### Test Results
```
Non-Streaming (Working):
  - Candidates: 1
  - Parts: 1  
  - FunctionCalls found: 1
  - Tool accessible via AsFunctionCalls(): ✅

Streaming (Broken):
  - Candidates: 1
  - Parts: 0  ← KEY ISSUE
  - FunctionCalls found: 0
  - Tool accessible via AsFunctionCalls(): ❌
  - But logs show: "STREAMING TOOL USE STARTED" ✅
```

## Root Cause Analysis

### 1. Non-Streaming Implementation (Working)

**File:** `gollm/bedrock.go` lines 567-611

```go
// bedrockCandidate.Parts() - NON-STREAMING
func (c *bedrockCandidate) Parts() []Part {
    var parts []Part
    for i, block := range c.message.Content {
        switch v := block.(type) {
        case *types.ContentBlockMemberText:
            parts = append(parts, &bedrockTextPart{text: v.Value})
            
        case *types.ContentBlockMemberToolUse:  // ← CRITICAL: Tool conversion
            parts = append(parts, &bedrockToolPart{toolUse: &v.Value})
            
        default:
            klog.Errorf("Unknown content type: %T", block)
        }
    }
    return parts
}
```

**Key Success Factors:**
- ✅ Processes complete `types.Message.Content` blocks
- ✅ Converts `ContentBlockMemberToolUse` → `bedrockToolPart`
- ✅ `bedrockToolPart.AsFunctionCalls()` returns proper `FunctionCall` objects

### 2. Streaming Implementation (Broken)

**File:** `gollm/bedrock.go` lines 625-630

```go
// bedrockStreamCandidate.Parts() - STREAMING  
func (c *bedrockStreamCandidate) Parts() []Part {
    if c.content == "" {
        return []Part{}  // ← RETURNS EMPTY - NO TOOL PARTS!
    }
    return []Part{&bedrockTextPart{text: c.content}}
}
```

**Critical Flaws:**
- ❌ **Only handles text content** - ignores tool calls entirely
- ❌ **No tool call storage** - `bedrockStreamCandidate` has no tool call field
- ❌ **Missing conversion logic** - never creates `bedrockToolPart` objects

### 3. Streaming Event Processing Analysis

**File:** `gollm/bedrock.go` lines 357-367

```go
case *types.ConverseStreamOutputMemberContentBlockStart:
    if v.Value.Start != nil {
        if toolStart, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
            // ✅ DETECTION: Logs tool call correctly
            klog.Infof("STREAMING TOOL USE STARTED - ID: %s, Name: %s",
                aws.ToString(toolStart.Value.ToolUseId), 
                aws.ToString(toolStart.Value.Name))
            
            // ❌ CRITICAL MISSING: Never yields tool call as response!
            // ❌ CRITICAL MISSING: Never stores tool call data!
        }
    }
    // ❌ No tool call response yielded here
```

**The Core Problem:**
1. **Tool Detection**: ✅ Works perfectly - logs show tools are detected
2. **Tool Storage**: ❌ Missing - no mechanism to store tool calls during streaming
3. **Tool Yielding**: ❌ Missing - tool calls never yielded as stream responses
4. **Tool Conversion**: ❌ Missing - no conversion to `bedrockToolPart` objects

## Architectural Comparison

### OpenAI Streaming (Reference Implementation)

**File:** `gollm/openai.go` lines 346-360

```go
// OpenAI accumulates tool calls during streaming
var currentToolCalls []openai.ChatCompletionMessageToolCall

// Handle tool call completion
if tool, ok := acc.JustFinishedToolCall(); ok {
    newToolCall := openai.ChatCompletionMessageToolCall{
        ID: tool.ID,
        Function: openai.ChatCompletionMessageToolCallFunction{
            Name:      tool.Name,
            Arguments: tool.Arguments,
        },
    }
    currentToolCalls = append(currentToolCalls, newToolCall)  // ← ACCUMULATE
    toolCallsForThisChunk = []openai.ChatCompletionMessageToolCall{newToolCall}
}

streamResponse := &openAIChatStreamResponse{
    toolCalls: toolCallsForThisChunk,  // ← YIELD IN RESPONSE
}

// Only yield if there's content or tool calls to report
if streamResponse.content != "" || len(streamResponse.toolCalls) > 0 {
    yield(streamResponse, nil)  // ← YIELD TOOL CALLS
}
```

**OpenAI Success Pattern:**
- ✅ **Accumulation**: Stores tool calls in `currentToolCalls` array
- ✅ **Yielding**: Includes tool calls in stream responses
- ✅ **Conversion**: `openAIChatStreamResponse` handles tool calls properly
- ✅ **Interface Compliance**: Tool calls accessible via `AsFunctionCalls()`

### Bedrock Streaming (Current - Broken)

```go
// Bedrock detects but never accumulates or yields
case *types.ConverseStreamOutputMemberContentBlockStart:
    if toolStart, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
        // ✅ Detection works
        klog.Infof("STREAMING TOOL USE STARTED...")
        
        // ❌ Missing: No accumulation
        // ❌ Missing: No yielding
        // ❌ Missing: No response creation
    }

// Only text responses are yielded:
response := &bedrockStreamResponse{
    content: textDelta.Value,  // ← Only text content
    // ❌ Missing: No tool calls field
}
yield(response, nil)
```

## Technical Solution Design

### Required Changes

#### 1. Update `bedrockStreamResponse` Structure

```go
type bedrockStreamResponse struct {
    content   string
    usage     *types.TokenUsage
    model     string
    done      bool
    // ✅ ADD: Tool calls field
    toolCalls []types.ToolUseBlock
}
```

#### 2. Update `bedrockStreamCandidate` Structure

```go
type bedrockStreamCandidate struct {
    streamChoice openai.ChatCompletionChunkChoice
    content      string
    // ✅ ADD: Tool calls field  
    toolCalls    []types.ToolUseBlock
}
```

#### 3. Implement Tool Call Accumulation

```go
func (c *bedrockChat) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
    // ✅ ADD: Tool call accumulation
    var currentToolCalls []types.ToolUseBlock
    
    for event := range stream.Events() {
        switch v := event.(type) {
        case *types.ConverseStreamOutputMemberContentBlockStart:
            if toolStart, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
                // ✅ CREATE: Complete tool call object
                toolCall := types.ToolUseBlock{
                    ToolUseId: toolStart.Value.ToolUseId,
                    Name:      toolStart.Value.Name,
                    // Input will be accumulated from deltas
                }
                currentToolCalls = append(currentToolCalls, toolCall)
                
                // ✅ YIELD: Tool call in stream response
                response := &bedrockStreamResponse{
                    content:   "",
                    toolCalls: []types.ToolUseBlock{toolCall},
                    model:     c.model,
                    done:      false,
                }
                yield(response, nil)
            }
        }
    }
}
```

#### 4. Update `bedrockStreamCandidate.Parts()`

```go
func (c *bedrockStreamCandidate) Parts() []Part {
    var parts []Part
    
    // Handle text content
    if c.content != "" {
        parts = append(parts, &bedrockTextPart{text: c.content})
    }
    
    // ✅ ADD: Handle tool calls
    if len(c.toolCalls) > 0 {
        for _, toolCall := range c.toolCalls {
            parts = append(parts, &bedrockToolPart{toolUse: &toolCall})
        }
    }
    
    return parts
}
```

## Impact Analysis

### Current Impact
- ❌ **Streaming tool detection**: Completely non-functional
- ❌ **Developer experience**: Inconsistent behavior between streaming/non-streaming
- ❌ **Use case limitation**: Cannot build streaming tool-enabled applications
- ✅ **Non-streaming**: Unaffected - continues to work perfectly
- ✅ **FunctionCallResult handling**: Unaffected - works for both methods

### Post-Fix Impact
- ✅ **Feature parity**: Streaming matches non-streaming functionality
- ✅ **Consistent interface**: Both methods support `AsFunctionCalls()`
- ✅ **Use case enablement**: Real-time tool-enabled applications possible
- ✅ **Backward compatibility**: No breaking changes

## Implementation Priority

### High Priority (Core Functionality)
1. **Tool call accumulation** - Store detected tool calls
2. **Tool call yielding** - Include in stream responses
3. **Parts() conversion** - Make tool calls accessible via standard interface

### Medium Priority (Enhanced Experience)
1. **Tool input accumulation** - Handle parameter streaming
2. **Error handling** - Proper tool call error propagation
3. **Performance optimization** - Efficient tool call storage

### Low Priority (Polish)
1. **Debug logging enhancement** - Better streaming tool call visibility
2. **Documentation** - Streaming tool call usage examples
3. **Testing** - Comprehensive streaming tool call test suite

## Conclusion

The Bedrock streaming tool call detection issue is a **complete architectural gap** rather than a bug. The implementation detects tool calls perfectly but lacks the infrastructure to:

1. **Store** tool calls during streaming
2. **Yield** tool calls as stream responses  
3. **Convert** tool calls to the gollm Part interface
4. **Present** tool calls via `AsFunctionCalls()`

This is **entirely separate** from the FunctionCallResult handling issue (which is now fixed). The streaming issue requires implementing the missing tool call presentation layer following the successful OpenAI streaming pattern.

**Severity**: High - Breaks streaming tool call functionality entirely
**Complexity**: Medium - Well-defined solution pattern exists
**Risk**: Low - Changes isolated to streaming code path