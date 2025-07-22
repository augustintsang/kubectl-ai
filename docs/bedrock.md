# AWS Bedrock Provider

This document covers the AWS Bedrock provider for kubectl-ai, which enables AI-powered Kubernetes operations using Claude and Nova models via AWS Bedrock.

## Configuration

### Environment Variables

The Bedrock provider uses standard AWS environment variables plus one Bedrock-specific variable:

```bash
# AWS SDK Standard Variables (handled automatically by AWS SDK)
AWS_REGION=us-east-1                    # Primary Bedrock region
AWS_PROFILE=bedrock-profile             # AWS credentials profile
AWS_ACCESS_KEY_ID=your-access-key       # Direct credentials (not recommended)
AWS_SECRET_ACCESS_KEY=your-secret-key   # Direct credentials (not recommended)

# Bedrock-Specific Configuration
BEDROCK_MODEL=us.anthropic.claude-sonnet-4-20250514-v1:0  # Default model
```

### Supported Model IDs (Inference Profiles)

This implementation uses AWS Bedrock inference profiles for cross-region reliability:

**Claude Models**:
- `us.anthropic.claude-sonnet-4-20250514-v1:0` (default, latest Claude Sonnet)
- `us.anthropic.claude-3-7-sonnet-20250219-v1:0`

**Nova Models**:
- `us.amazon.nova-pro-v1:0`
- `us.amazon.nova-lite-v1:0`
- `us.amazon.nova-micro-v1:0`

For the most current model availability, refer to the [AWS Bedrock User Guide](https://docs.aws.amazon.com/bedrock/latest/userguide/model-ids.html).

## Authentication

### Recommended Approach

Use AWS Identity Center or IAM roles rather than access keys:

```bash
# Configure AWS Profile
aws configure sso --profile bedrock-production

# Use the profile
export AWS_PROFILE=bedrock-production
export AWS_REGION=us-east-1
```

### IAM Permissions

Ensure your AWS credentials have the `AmazonBedrockFullAccess` managed policy or equivalent permissions:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "bedrock:InvokeModel",
                "bedrock:InvokeModelWithResponseStream"
            ],
            "Resource": "*"
        }
    ]
}
```

## Usage Examples

### Basic Usage

```bash
export AWS_REGION=us-east-1
export BEDROCK_MODEL=us.anthropic.claude-sonnet-4-20250514-v1:0
kubectl-ai "explain this pod"
```

### Production Configuration

```bash
# Use AWS Profile (recommended)
export AWS_PROFILE=bedrock-production
export AWS_REGION=us-east-1

# Optional: Override default model
export BEDROCK_MODEL=us.amazon.nova-pro-v1:0

# AWS SDK retry configuration
export AWS_MAX_ATTEMPTS=3
export AWS_RETRY_MODE=adaptive

kubectl-ai "help me debug this deployment"
```

### IAM Role (EC2/EKS)

```bash
# No environment variables needed when using IAM roles
# AWS SDK automatically uses instance/pod credentials
kubectl-ai "analyze this deployment"
```

## Configuration Notes

- **Region**: AWS recommends `us-east-1` as the primary Bedrock region
- **Authentication**: AWS SDK handles the credential chain automatically (environment → credentials file → IAM roles)
- **Retry Logic**: AWS SDK provides built-in retry logic and timeout handling
- **Model Access**: Models must be enabled in the AWS Bedrock console before use

## Troubleshooting

### Common Issues

1. **Model not available**: Verify the model is enabled in your AWS region via the Bedrock console
2. **Authentication errors**: Check AWS credentials and IAM permissions
3. **Region issues**: Ensure the specified region supports Bedrock and your desired models
4. **Rate limits**: AWS Bedrock has service quotas; consider using `AWS_RETRY_MODE=adaptive`

### Debug Information

kubectl-ai uses standard klog for logging. Increase verbosity to see detailed provider information:

```bash
kubectl-ai -v=2 "your query"
```