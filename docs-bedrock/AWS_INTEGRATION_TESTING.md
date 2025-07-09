# AWS Integration Testing Guide

This guide explains how to run comprehensive AWS integration tests for the Bedrock provider that make actual API calls to verify real-world functionality.

## Prerequisites

### 1. AWS SSO Setup

These tests require actual AWS credentials with Bedrock access. The recommended approach is AWS SSO:

```bash
# Configure AWS SSO
aws configure sso

# Login to AWS SSO (required before running tests)
aws sso login

# Verify your credentials
aws sts get-caller-identity
```

### 2. Bedrock Model Access

Ensure your AWS account has access to the required Bedrock models:

- **Anthropic Claude models**: `us.anthropic.claude-3-7-sonnet-20250219-v1:0`, `us.anthropic.claude-sonnet-4-20250514-v1:0`
- **Amazon Nova models**: `us.amazon.nova-lite-v1:0`, `us.amazon.nova-pro-v1:0`

Request model access in the AWS Bedrock console if needed.

### 3. Required Permissions

Your AWS role/user needs these permissions:
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "bedrock:ListFoundationModels",
                "bedrock:InvokeModel",
                "bedrock:InvokeModelWithResponseStream"
            ],
            "Resource": "*"
        }
    ]
}
```

## Running the Tests

### 1. Basic Integration Tests

Run all AWS integration tests:

```bash
cd gollm/bedrock
go test -tags=aws_integration -v ./...
```

### 2. Specific Test Categories

**Test AWS credentials and SSO:**
```bash
go test -tags=aws_integration -v ./... -run TestAWSCredentials
```

**Test streaming functionality:**
```bash
go test -tags=aws_integration -v ./... -run TestRealStreamingFunctionality
```

**Test usage tracking:**
```bash
go test -tags=aws_integration -v ./... -run TestRealUsageTracking
```

**Test k8s-bench patterns:**
```bash
go test -tags=aws_integration -v ./... -run TestK8sBenchCommandLinePattern
```

**Test LLM-Apps integration patterns:**
```bash
go test -tags=aws_integration -v ./... -run TestLLMAppsIntegrationPattern
```

**Test multiple AWS profiles:**
```bash
go test -tags=aws_integration -v ./... -run TestAWSSSOCredentials
```

**Test model availability:**
```bash
go test -tags=aws_integration -v ./... -run TestBedrockModelAvailability
```

**Test streaming performance:**
```bash
go test -tags=aws_integration -v ./... -run TestStreamingPerformance
```

### 3. Environment Configuration

**Set specific region:**
```bash
AWS_REGION=us-west-2 go test -tags=aws_integration -v ./...
```

**Use specific profile:**
```bash
AWS_PROFILE=my-dev-profile go test -tags=aws_integration -v ./...
```

**Test with debug output:**
```bash
go test -tags=aws_integration -v ./... -run TestLLMAppsIntegrationPattern
```

## Test Coverage

### Core AWS Integration (`aws_integration_test.go`)

1. **TestAWSCredentials**
   - ✅ Verifies AWS config loading
   - ✅ Tests region configuration
   - ✅ Makes actual Bedrock API call to verify credentials
   - **Purpose**: Ensures AWS credentials work before running other tests

2. **TestRealBedrockClientCreation**
   - ✅ Tests client creation with various configurations
   - ✅ Tests inference config application
   - ✅ Tests usage callback setup
   - ✅ Tests debug mode
   - **Purpose**: Verifies `gollm.NewClient()` works with real AWS credentials

3. **TestRealStreamingFunctionality**
   - ✅ Tests actual streaming responses from AWS Bedrock
   - ✅ Verifies usage callback is called during streaming
   - ✅ Tests response chunking and content extraction
   - ✅ Verifies usage metadata in streaming responses
   - **Purpose**: Essential for llm-apps integration which uses streaming

4. **TestRealUsageTracking**
   - ✅ Tests comprehensive usage tracking across multiple requests
   - ✅ Verifies usage structure and data accuracy
   - ✅ Tests callback functionality with real data
   - ✅ Validates token counting and aggregation
   - **Purpose**: Verifies usage metrics for cost tracking in llm-apps

5. **TestToolCallingWithRealAPI**
   - ✅ Tests tool calling functionality with real models
   - ✅ Tests function definition setup
   - ✅ Tests function call result processing
   - ✅ Verifies usage tracking for tool calls
   - **Purpose**: Tests advanced features needed for kubectl-ai

6. **TestLLMAppsIntegrationPattern**
   - ✅ Simulates exact llm-apps usage patterns
   - ✅ Tests handler-level client creation
   - ✅ Tests agent-level streaming response processing
   - ✅ Tests application-level usage aggregation
   - ✅ Verifies Kubernetes-style prompts work
   - **Purpose**: Critical test for actual llm-apps integration

7. **TestMultipleModelsWithRealCalls**
   - ✅ Tests multiple Bedrock models
   - ✅ Verifies model-specific behavior
   - ✅ Tests usage tracking per model
   - **Purpose**: Ensures compatibility with different model families

### AWS SSO & Profile Testing (`aws_sso_integration_test.go`)

1. **TestAWSSSOCredentials**
   - ✅ Verifies SSO session is active
   - ✅ Tests multiple AWS profiles
   - ✅ Tests multiple regions
   - ✅ Validates Bedrock access per profile/region
   - **Purpose**: Ensures SSO authentication works correctly

2. **TestK8sBenchCommandLinePattern**
   - ✅ Tests exact k8s-bench command patterns
   - ✅ Simulates `./k8s-bench run --llm-provider bedrock --models "..."`
   - ✅ Tests Kubernetes troubleshooting prompts
   - ✅ Verifies response quality and kubectl command generation
   - **Purpose**: Validates integration with k8s-bench evaluation framework

3. **TestBedrockModelAvailability**
   - ✅ Lists all available models in current account/region
   - ✅ Tests model family access (Claude, Nova, etc.)
   - ✅ Identifies which test models are accessible
   - **Purpose**: Helps debug model access issues

4. **TestStreamingPerformance**
   - ✅ Measures streaming latency and performance
   - ✅ Tracks time to first chunk
   - ✅ Measures tokens per second
   - ✅ Analyzes inter-chunk timing
   - **Purpose**: Ensures performance meets production requirements

## Integration with llm-apps

These tests specifically verify patterns used in `integration-with-llm-apps.md`:

### Handler Level Integration
```go
// Tests verify this pattern works:
client, err := gollm.NewClient(ctx, "bedrock",
    gollm.WithInferenceConfig(config),
    gollm.WithUsageCallback(callback),
)
```

### Agent Level Integration
```go
// Tests verify streaming response processing:
responseStream, err := chat.SendStreaming(ctx, prompt)
for response, err := range responseStream {
    // Extract content and usage as llm-apps would
}
```

### Application Level Integration
```go
// Tests verify usage aggregation:
usageCallback := func(provider, model string, usage gollm.Usage) {
    // Aggregate usage as llm-apps would
}
```

## Troubleshooting

### Common Issues

1. **AWS credentials not configured**
   ```
   Error: AWS credentials must be configured for integration tests
   Solution: Run `aws sso login` or configure AWS credentials
   ```

2. **Model access denied**
   ```
   Error: Model not accessible
   Solution: Request access to Bedrock models in AWS console
   ```

3. **Region not supported**
   ```
   Error: Region 'xyz' Bedrock not accessible
   Solution: Use a region where Bedrock is available (us-east-1, us-west-2, etc.)
   ```

4. **SSO session expired**
   ```
   Error: Should be able to get caller identity - ensure 'aws sso login' has been run
   Solution: Run `aws sso login` to refresh session
   ```

### Debug Mode

Run tests with detailed output:
```bash
go test -tags=aws_integration -v ./... -run TestLLMAppsIntegrationPattern 2>&1 | tee test-output.log
```

### Check Model Availability

Before running other tests, check what models are available:
```bash
go test -tags=aws_integration -v ./... -run TestBedrockModelAvailability
```

## Expected Test Output

Successful test run should show:

```
=== RUN   TestAWSCredentials
✅ AWS credentials verified successfully
--- PASS: TestAWSCredentials

=== RUN   TestRealStreamingFunctionality
Streaming usage callback: bedrock/us.anthropic.claude-3-7-sonnet-20250219-v1:0 - Input: 45, Output: 123, Total: 168
✅ Streaming test completed - 15 chunks, 168 tokens
--- PASS: TestRealStreamingFunctionality

=== RUN   TestLLMAppsIntegrationPattern
LLM-Apps usage capture: bedrock/us.anthropic.claude-3-7-sonnet-20250219-v1:0 - 234 tokens
✅ LLM-Apps integration test completed:
   - Queries processed: 3
   - Total tokens used: 456
   - Average tokens per query: 152.0
   - Provider: us.anthropic.claude-3-7-sonnet-20250219-v1:0
--- PASS: TestLLMAppsIntegrationPattern
```

## Performance Benchmarks

The tests also provide performance metrics:

```
✅ Streaming performance metrics:
   - Time to first chunk: 1.234s
   - Total stream time: 8.567s
   - Streaming duration: 7.333s
   - Response chunks: 42
   - Total tokens: 456
   - Avg inter-chunk time: 174ms
   - Tokens per second: 62.18
```

## Integration with CI/CD

For automated testing in CI/CD:

```yaml
# GitHub Actions example
- name: Run AWS Integration Tests
  env:
    AWS_REGION: us-east-1
    AWS_PROFILE: ci-profile
  run: |
    aws sso login --profile ci-profile
    go test -tags=aws_integration -v ./gollm/bedrock/...
```

## Security Notes

- Tests use read-only Bedrock operations
- No sensitive data is logged (credentials are never printed)
- Usage tracking is ephemeral (no persistent storage)
- Tests clean up AWS resources automatically
- All API calls are standard Bedrock inference calls (no management operations)

These tests provide comprehensive validation that the Bedrock provider works correctly with real AWS credentials and can be used reliably in production applications like kubectl-ai and k8s-bench. 