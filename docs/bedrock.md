# AWS Bedrock Integration Guide

kubectl-ai now supports AWS Bedrock, allowing you to use Amazon's foundational models for Kubernetes operations. This guide covers setup, configuration, and usage of AWS Bedrock with kubectl-ai.

## Overview

AWS Bedrock is Amazon's managed service for foundational models from leading AI companies. kubectl-ai integrates with Bedrock to provide AI-powered Kubernetes management using models like:

- **Anthropic Claude**: Advanced reasoning and code generation
- **Amazon Nova**: High-performance and cost-effective models
- **Mistral**: Multilingual and specialized models

## Prerequisites

1. **AWS Account**: You need an AWS account with appropriate permissions
2. **AWS CLI**: Install and configure AWS CLI (optional but recommended)
3. **Model Access**: Request access to the models you want to use in AWS Bedrock console
4. **kubectl-ai**: Version 0.0.12 or later

## Setup

### 1. AWS Credentials

Configure your AWS credentials using one of these methods:

#### Option A: AWS CLI (Recommended)
```bash
aws configure
```

#### Option B: Environment Variables
```bash
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_DEFAULT_REGION=us-west-2
```

#### Option C: AWS SSO
```bash
aws configure sso
```

#### Option D: IAM Role (for EC2/ECS/Lambda)
AWS SDK will automatically use the IAM role attached to your instance.

### 2. Required AWS Permissions

Ensure your AWS credentials have the following permissions:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "bedrock:InvokeModel",
                "bedrock:InvokeModelWithResponseStream",
                "bedrock:GetFoundationModel",
                "bedrock:ListFoundationModels"
            ],
            "Resource": "*"
        }
    ]
}
```

### 3. Request Model Access

1. Go to the [AWS Bedrock Console](https://console.aws.amazon.com/bedrock)
2. Navigate to "Model access" in the left sidebar
3. Request access to the models you want to use
4. Wait for approval (this can take a few minutes to hours)

## Configuration

### Basic Configuration

Create or edit `~/.config/kubectl-ai/config.yaml`:

```yaml
llm-provider: bedrock
model: us.anthropic.claude-sonnet-4-20250514-v1:0
```

### Environment Variables for Advanced Configuration

For advanced model parameters, use environment variables:

```bash
export BEDROCK_TEMPERATURE=0.7
export BEDROCK_MAX_TOKENS=4000
export BEDROCK_TOP_P=0.9
export BEDROCK_MAX_RETRIES=3
export BEDROCK_TIMEOUT=30s
```

## Usage

### Basic Usage

```bash
# Use default Bedrock model
kubectl-ai --llm-provider bedrock

# Use specific model
kubectl-ai --llm-provider bedrock --model us.anthropic.claude-sonnet-4-20250514-v1:0

# Use with specific region
kubectl-ai --llm-provider bedrock --model anthropic.claude-v2:1 --region us-east-1
```

### Interactive Mode

```bash
kubectl-ai --llm-provider bedrock
> list all pods in kube-system namespace
> create a deployment for nginx with 3 replicas
> help me troubleshoot why my pod is pending
```

### One-shot Commands

```bash
# Quick queries
kubectl-ai --llm-provider bedrock --quiet "show me failing pods"

# Pipe input
echo "create a configmap from my .env file" | kubectl-ai --llm-provider bedrock
```

## Available Models

For the most up-to-date list of available models by region, see the [AWS Bedrock Models Documentation](https://docs.aws.amazon.com/bedrock/latest/userguide/models-supported.html).

### Model Recommendations

- **For complex reasoning**: `us.anthropic.claude-sonnet-4-20250514-v1:0`
- **For balanced performance**: `us.anthropic.claude-3-7-sonnet-20250219-v1:0`
- **For cost efficiency**: `us.amazon.nova-lite-v1:0`
- **For high throughput**: `us.amazon.nova-micro-v1:0`

## Troubleshooting

### Common Issues

#### 1. "Access Denied" Error
```bash
Error: failed to invoke Bedrock model: access denied
```

**Solution**: Ensure you have requested access to the model in the Bedrock console and have proper IAM permissions.

#### 2. "Model Not Found" Error
```bash
Error: unsupported model - only Claude and Nova models are supported
```

**Solution**: Use a supported model from the AWS documentation, or check if the model is available in your region.

#### 3. "Region Not Available" Error
```bash
Error: failed to load AWS configuration
```

**Solution**: Ensure the model is available in your specified region, or try a different region.

#### 4. Timeout Issues
```bash
Error: context deadline exceeded
```

**Solution**: Increase timeout value using environment variable:
```bash
export BEDROCK_TIMEOUT=60s
```

### Debug Mode

Enable debug mode for detailed logging:

```bash
export BEDROCK_DEBUG=true
kubectl-ai --llm-provider bedrock
```

### Verify Configuration

Check your current configuration:

```bash
kubectl-ai --llm-provider bedrock model
```

## Advanced Bedrock Features

### Custom Inference Profiles

Use AWS Bedrock Inference Profiles:

```bash
kubectl-ai --llm-provider bedrock --model "arn:aws:bedrock:us-west-2:123456789012:inference-profile/my-profile"
```

### Token Usage Tracking

Bedrock responses include built-in token usage information. You can access this programmatically:

```go
response, err := chat.Send(ctx, "Hello!")
if usage, ok := response.UsageMetadata().(*gollm.Usage); ok {
    fmt.Printf("Tokens used: %d\n", usage.TotalTokens)
}
```

## Examples

### Creating Resources

```bash
kubectl-ai --llm-provider bedrock "create a deployment for redis with persistent storage"
```

### Debugging Issues

```bash
kubectl-ai --llm-provider bedrock "why is my pod stuck in pending state?"
```

### Resource Management

```bash
kubectl-ai --llm-provider bedrock "scale down all non-essential deployments"
```

### Security Analysis

```bash
kubectl-ai --llm-provider bedrock "audit my cluster for security issues"
```

## Support

For issues specific to the Bedrock integration:

1. Check the [kubectl-ai GitHub Issues](https://github.com/GoogleCloudPlatform/kubectl-ai/issues)
2. Review [AWS Bedrock Documentation](https://docs.aws.amazon.com/bedrock/)
3. Verify your AWS credentials and permissions
4. Enable debug mode for detailed error messages 