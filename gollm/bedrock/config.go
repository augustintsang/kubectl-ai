// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bedrock

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubectl-ai/gollm"
	"github.com/aws/aws-sdk-go-v2/aws"
)

const (
	Name = "bedrock"

	ErrMsgConfigLoad       = "failed to load AWS configuration"
	ErrMsgModelInvoke      = "failed to invoke Bedrock model"
	ErrMsgResponseParse    = "failed to parse Bedrock response"
	ErrMsgRequestBuild     = "failed to build request"
	ErrMsgStreamingFailed  = "Bedrock streaming failed"
	ErrMsgUnsupportedModel = "unsupported model - only Claude and Nova models are supported"
)

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

var DefaultOptions = &BedrockOptions{
	Region:      "us-west-2",
	Model:       "us.anthropic.claude-sonnet-4-20250514-v1:0",
	MaxTokens:   64000,
	Temperature: 0.1,
	TopP:        0.9,
	Timeout:     30 * time.Second,
	MaxRetries:  10,
}

// supportedModelsByRegion defines the available models for each AWS region
// This allows for region-specific model availability and easier maintenance
var supportedModelsByRegion = map[string][]string{
	"us-east-1": {
		"us.anthropic.claude-sonnet-4-20250514-v1:0",
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
		"us.amazon.nova-pro-v1:0",
		"us.amazon.nova-lite-v1:0",
		"us.amazon.nova-micro-v1:0",
		"anthropic.claude-v2:1",
		"anthropic.claude-instant-v1",
		"amazon.nova-pro-v1:0",
		"mistral.mistral-large-2402-v1:0",
	},
	"us-west-2": {
		"us.anthropic.claude-sonnet-4-20250514-v1:0",
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
		"us.amazon.nova-pro-v1:0",
		"us.amazon.nova-lite-v1:0",
		"us.amazon.nova-micro-v1:0",
		"anthropic.claude-v2:1",
		"amazon.nova-pro-v1:0",
		"stability.sd3-large-v1:0",
	},
	"eu-west-1": {
		"anthropic.claude-v2:1",
		"anthropic.claude-instant-v1",
		"amazon.nova-pro-v1:0",
		"amazon.nova-lite-v1:0",
		"amazon.nova-micro-v1:0",
	},
	"eu-central-1": {
		"anthropic.claude-v2:1",
		"anthropic.claude-instant-v1",
		"amazon.nova-pro-v1:0",
		"amazon.nova-lite-v1:0",
		"amazon.nova-micro-v1:0",
	},
	"ap-southeast-1": {
		"anthropic.claude-v2:1",
		"anthropic.claude-instant-v1",
		"amazon.nova-pro-v1:0",
		"amazon.nova-lite-v1:0",
		"amazon.nova-micro-v1:0",
	},
	"ap-northeast-1": {
		"anthropic.claude-v2:1",
		"anthropic.claude-instant-v1",
		"amazon.nova-pro-v1:0",
		"amazon.nova-lite-v1:0",
		"amazon.nova-micro-v1:0",
	},
}

// isModelSupported checks if the given model is supported in the specified region
func isModelSupported(model string) bool {
	return isModelSupportedInRegion(model, "")
}

// isModelSupportedInRegion checks if the given model is supported in the specified region
func isModelSupportedInRegion(model, region string) bool {
	if model == "" {
		return false
	}

	modelLower := strings.ToLower(model)

	// If region is specified, check region-specific models first
	if region != "" {
		if models, exists := supportedModelsByRegion[region]; exists {
			for _, supported := range models {
				if modelLower == strings.ToLower(supported) {
					return true
				}
			}
		}
	}

	// Fallback: check all regions if no region specified or model not found in specified region
	for _, models := range supportedModelsByRegion {
		for _, supported := range models {
			if modelLower == strings.ToLower(supported) {
				return true
			}
		}
	}

	// Handle special cases (ARNs and inference profiles)
	if strings.Contains(modelLower, "arn:aws:bedrock") {
		if strings.Contains(modelLower, "inference-profile") {
			if strings.Contains(modelLower, "anthropic") || strings.Contains(modelLower, "claude") {
				return true
			}

			if strings.Contains(modelLower, "amazon") || strings.Contains(modelLower, "nova") {
				return true
			}

			return true
		}

		if strings.Contains(modelLower, "foundation-model") {
			parts := strings.Split(model, "/")
			if len(parts) > 0 {
				extractedModel := parts[len(parts)-1]
				return isModelSupportedInRegion(extractedModel, region)
			}
		}
	}

	return false
}

// getSupportedModels returns all supported models across all regions
func getSupportedModels() []string {
	return getSupportedModelsForRegion("")
}

// getSupportedModelsForRegion returns supported models for the specified region
// If region is empty, returns all models across all regions
func getSupportedModelsForRegion(region string) []string {
	if region != "" {
		if models, exists := supportedModelsByRegion[region]; exists {
			// Return a copy to avoid external modification
			result := make([]string, len(models))
			copy(result, models)
			return result
		}
		return []string{} // Return empty slice if region not found
	}

	// Return all models across all regions (with deduplication)
	modelSet := make(map[string]bool)
	for _, models := range supportedModelsByRegion {
		for _, model := range models {
			modelSet[model] = true
		}
	}

	var result []string
	for model := range modelSet {
		result = append(result, model)
	}

	return result
}

// BedrockUsageCallback is a bedrock-specific callback for usage tracking
// This is separate from the global gollm interfaces to keep scope limited to bedrock
type BedrockUsageCallback func(provider, model string, usage gollm.Usage)

// BedrockClientOptions provides bedrock-specific configuration
// This replaces the need for global ClientOptions changes
type BedrockClientOptions struct {
	// Existing SSL option (preserved for compatibility)
	SkipVerifySSL bool

	// Bedrock-specific features that don't affect other providers
	UsageCallback BedrockUsageCallback
	Debug         bool

	// Advanced inference parameters (loaded from env vars if not specified)
	Temperature float32
	MaxTokens   int32
	TopP        float32
	TopK        int32
	MaxRetries  int
	Region      string
	Model       string
	Timeout     time.Duration
}

// LoadBedrockConfigFromEnv loads advanced configuration from environment variables
// This addresses droot's preference for env vars over explicit config
func LoadBedrockConfigFromEnv() BedrockClientOptions {
	opts := BedrockClientOptions{
		// Set sensible defaults
		Temperature: 0.1,
		MaxTokens:   64000,
		TopP:        0.9,
		TopK:        40,
		MaxRetries:  3,
		Region:      "us-west-2",
		Model:       "us.anthropic.claude-sonnet-4-20250514-v1:0",
		Timeout:     30 * time.Second,
	}

	// Load from environment variables
	if temp := os.Getenv("BEDROCK_TEMPERATURE"); temp != "" {
		if val, err := strconv.ParseFloat(temp, 32); err == nil {
			opts.Temperature = float32(val)
		}
	}

	if maxTokens := os.Getenv("BEDROCK_MAX_TOKENS"); maxTokens != "" {
		if val, err := strconv.Atoi(maxTokens); err == nil {
			opts.MaxTokens = int32(val)
		}
	}

	if topP := os.Getenv("BEDROCK_TOP_P"); topP != "" {
		if val, err := strconv.ParseFloat(topP, 32); err == nil {
			opts.TopP = float32(val)
		}
	}

	if topK := os.Getenv("BEDROCK_TOP_K"); topK != "" {
		if val, err := strconv.Atoi(topK); err == nil {
			opts.TopK = int32(val)
		}
	}

	if retries := os.Getenv("BEDROCK_MAX_RETRIES"); retries != "" {
		if val, err := strconv.Atoi(retries); err == nil {
			opts.MaxRetries = val
		}
	}

	if region := os.Getenv("BEDROCK_REGION"); region != "" {
		opts.Region = region
	}

	if model := os.Getenv("BEDROCK_MODEL"); model != "" {
		opts.Model = model
	}

	if timeout := os.Getenv("BEDROCK_TIMEOUT"); timeout != "" {
		if val, err := time.ParseDuration(timeout); err == nil {
			opts.Timeout = val
		}
	}

	if os.Getenv("BEDROCK_DEBUG") == "true" {
		opts.Debug = true
	}

	return opts
}

// ToBedrockOptions converts BedrockClientOptions to the internal BedrockOptions
// This bridges the new bedrock-specific config with the existing implementation
func (opts BedrockClientOptions) ToBedrockOptions() *BedrockOptions {
	return &BedrockOptions{
		Model:               opts.Model,
		Region:              opts.Region,
		Temperature:         opts.Temperature,
		MaxTokens:           opts.MaxTokens,
		TopP:                opts.TopP,
		MaxRetries:          opts.MaxRetries,
		Timeout:             opts.Timeout,
		CredentialsProvider: nil, // Keep existing behavior
	}
}

// MergeWithDefaults combines user options with environment and defaults
// This replaces the global mergeWithClientOptions function
func (opts BedrockClientOptions) MergeWithDefaults() BedrockClientOptions {
	// Start with environment variables as base
	merged := LoadBedrockConfigFromEnv()

	// Override with user-specified values (zero values are ignored)
	if opts.Temperature != 0 {
		merged.Temperature = opts.Temperature
	}
	if opts.MaxTokens != 0 {
		merged.MaxTokens = opts.MaxTokens
	}
	if opts.TopP != 0 {
		merged.TopP = opts.TopP
	}
	if opts.TopK != 0 {
		merged.TopK = opts.TopK
	}
	if opts.MaxRetries != 0 {
		merged.MaxRetries = opts.MaxRetries
	}
	if opts.Region != "" {
		merged.Region = opts.Region
	}
	if opts.Model != "" {
		merged.Model = opts.Model
	}
	if opts.Timeout != 0 {
		merged.Timeout = opts.Timeout
	}

	// Always use user-specified callback and debug settings
	merged.UsageCallback = opts.UsageCallback
	merged.Debug = opts.Debug
	merged.SkipVerifySSL = opts.SkipVerifySSL

	return merged
}

// NewBedrockClientWithConfig creates a new bedrock client with bedrock-specific options
// This is the new recommended way to create bedrock clients with enhanced features
func NewBedrockClientWithConfig(ctx context.Context, opts BedrockClientOptions) (*BedrockClient, error) {
	// Merge user options with defaults and environment
	config := opts.MergeWithDefaults()

	// Convert to internal BedrockOptions format
	bedrockOpts := config.ToBedrockOptions()

	// Create the client using existing infrastructure
	client, err := NewBedrockClientWithOptions(ctx, bedrockOpts)
	if err != nil {
		return nil, err
	}

	// Store bedrock-specific configuration
	client.bedrockConfig = config

	return client, nil
}

// BedrockClientWrapper wraps BedrockClient to implement gollm.Client interface
// This allows bedrock-specific clients to work with existing gollm interfaces
type BedrockClientWrapper struct {
	*BedrockClient
}

// WrapAsGollmClient wraps a bedrock client to implement gollm.Client
// This preserves compatibility with existing code that expects gollm.Client
func WrapAsGollmClient(client *BedrockClient) gollm.Client {
	return &BedrockClientWrapper{BedrockClient: client}
}

// Ensure wrapper implements gollm.Client
var _ gollm.Client = (*BedrockClientWrapper)(nil)
