# Bedrock Function Calling Debug Investigation

## Implementation Complete ✅

This debug implementation has been successfully added to the Bedrock provider in `gollm/bedrock.go` to help investigate why native function calling fails with AWS Bedrock Claude models.

## Debug Phases Implemented

### Phase 1: Response Content Block Analysis
- **Location**: `bedrockCandidate.Parts()` method around line 490
- **Purpose**: Logs what content block types Bedrock actually returns
- **Key Debugging**: Will show if `ContentBlockMemberToolUse` blocks are ever received

### Phase 2: Request Configuration Validation  
- **Location**: `bedrockChat.Send()` method around line 225
- **Purpose**: Verifies tool configuration is correctly sent to Bedrock
- **Key Debugging**: Logs tool names, ToolChoice type, and request structure

### Phase 3: Tool Schema Conversion Analysis
- **Location**: `bedrockChat.SetFunctionDefinitions()` method around line 402
- **Purpose**: Validates tool schema conversion for complex parameters
- **Key Debugging**: Shows original vs converted schemas, document creation

### Phase 4: Model and API Response Validation
- **Location**: `bedrockChat.Send()` method around line 253
- **Purpose**: Logs complete API request/response cycle
- **Key Debugging**: Model name, stop reason, raw output types

## How to Use

### 1. Run the Test Program
```bash
# Ensure AWS credentials are configured
aws sso login --profile your-profile

# Run the test with verbose logging
./test_bedrock_debug -v=3 2>&1 | grep "BEDROCK-FUNCTION-DEBUG"
```

### 2. Run kubectl-ai with Debug Logging
```bash
# Enable verbose logging
kubectl-ai --llm-provider=bedrock --model=us.anthropic.claude-sonnet-4-20250514-v1:0 -v=3 "list pods and tell me which ones are failing"
```

### 3. Analyze the Debug Output

Look for these key log patterns:

**✅ Expected Success Patterns:**
```
[BEDROCK-FUNCTION-DEBUG] Converting 2 function definitions to Bedrock tools
[BEDROCK-FUNCTION-DEBUG] Processing tool: kubectl
[BEDROCK-FUNCTION-DEBUG] ✅ Successfully created ToolConfiguration with 2 tools
[BEDROCK-FUNCTION-DEBUG] Configuring 2 function definitions for model: us.anthropic.claude-sonnet-4-20250514-v1:0
[BEDROCK-FUNCTION-DEBUG] Tool names: [kubectl bash]
[BEDROCK-FUNCTION-DEBUG] ✅ ToolConfig set with ToolChoice: *types.ToolChoiceMemberAny
[BEDROCK-FUNCTION-DEBUG] ✅ Received response from Bedrock
[BEDROCK-FUNCTION-DEBUG] Processing 1 content blocks from Bedrock response
[BEDROCK-FUNCTION-DEBUG] ✅ TOOL USE DETECTED - ID: tooluse_123, Name: kubectl
[BEDROCK-FUNCTION-DEBUG] Total tool calls extracted: 1
```

**❌ Problem Indicators:**
```
[BEDROCK-FUNCTION-DEBUG] Content block 0 type: *types.ContentBlockMemberText
[BEDROCK-FUNCTION-DEBUG] Text content: I'll help you list the pods...
[BEDROCK-FUNCTION-DEBUG] Total tool calls extracted: 0
```

## Expected Findings

Based on the investigation hypothesis, you'll likely see:

1. **Phase 1**: Only `ContentBlockMemberText` blocks returned, never `ContentBlockMemberToolUse`
2. **Phase 2**: Tool configuration appears correct in request  
3. **Phase 3**: Schema conversion working properly
4. **Phase 4**: Bedrock accepts request but returns text-only responses

## Test Cases to Execute

### Simple Function Test
```bash
./test_bedrock_debug -v=3
```
This tests with a parameter-less function to isolate schema complexity.

### Model Comparison
Test different Claude models:
```bash
# Claude 3.7 Sonnet
kubectl-ai --llm-provider=bedrock --model=us.anthropic.claude-3-7-sonnet-20250219-v1:0 -v=3 "what time is it"

# Claude Sonnet 4 
kubectl-ai --llm-provider=bedrock --model=us.anthropic.claude-sonnet-4-20250514-v1:0 -v=3 "what time is it"
```

### Complex Schema Test
```bash
kubectl-ai --llm-provider=bedrock -v=3 "create a pod named test-pod with nginx image"
```

## Critical Questions This Will Answer

1. **Is Bedrock returning ToolUse content blocks at all?**
   - Look for `✅ TOOL USE DETECTED` vs only text content

2. **Does this behavior differ between Claude model versions?**  
   - Compare debug output between different model IDs

3. **Are tool schemas being converted to correct AWS format?**
   - Check `Converted schema for X:` log entries

4. **Is this a Bedrock service limitation or gollm bug?**
   - If request shows correct tool config but response has no tool use blocks

## Success Criteria

This investigation succeeds when we can definitively determine:
- Whether Bedrock returns ToolUse blocks in responses
- Whether the issue is model-specific (Claude Sonnet 4) 
- Whether tool configuration format matches AWS requirements
- Whether this requires a Bedrock service bug report or gollm fix

## Next Steps After Investigation

Based on findings:

1. **If no ToolUse blocks returned**: File AWS Bedrock support case
2. **If schema conversion issues**: Fix gollm schema mapping  
3. **If model-specific**: Document model limitations and recommend alternatives
4. **If request format issues**: Update tool configuration to match AWS specs

## Log Level Configuration

- `-v=1`: Basic function calling status
- `-v=2`: Detailed schema and tool information  
- `-v=3`: Full debug output including request/response details

Use `-v=3` for complete debugging information.