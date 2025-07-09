// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gollm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageStruct(t *testing.T) {
	tests := []struct {
		name     string
		usage    Usage
		expected Usage
	}{
		{
			name: "complete usage with all fields",
			usage: Usage{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
				InputCost:    0.001,
				OutputCost:   0.0075,
				TotalCost:    0.0085,
				Model:        "claude-3-sonnet",
				Provider:     "bedrock",
				Timestamp:    time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			expected: Usage{
				InputTokens:  100,
				OutputTokens: 50,
				TotalTokens:  150,
				InputCost:    0.001,
				OutputCost:   0.0075,
				TotalCost:    0.0085,
				Model:        "claude-3-sonnet",
				Provider:     "bedrock",
				Timestamp:    time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "minimal usage with required fields only",
			usage: Usage{
				InputTokens:  25,
				OutputTokens: 10,
				Provider:     "openai",
			},
			expected: Usage{
				InputTokens:  25,
				OutputTokens: 10,
				TotalTokens:  0, // Should be calculated separately if needed
				Provider:     "openai",
			},
		},
		{
			name:     "zero usage (default struct)",
			usage:    Usage{},
			expected: Usage{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.usage)

			// Test JSON marshaling/unmarshaling
			data, err := tt.usage.MarshalJSON()
			require.NoError(t, err)

			var unmarshaled Usage
			err = unmarshaled.UnmarshalJSON(data)
			require.NoError(t, err)

			// Compare without timestamp precision issues
			assert.Equal(t, tt.usage.InputTokens, unmarshaled.InputTokens)
			assert.Equal(t, tt.usage.OutputTokens, unmarshaled.OutputTokens)
			assert.Equal(t, tt.usage.Provider, unmarshaled.Provider)
		})
	}
}

func TestInferenceConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   InferenceConfig
		expected InferenceConfig
	}{
		{
			name: "complete inference config",
			config: InferenceConfig{
				Model:       "claude-3-sonnet",
				Region:      "us-west-2",
				Temperature: 0.7,
				MaxTokens:   1000,
				TopP:        0.9,
				TopK:        40,
				MaxRetries:  3,
			},
			expected: InferenceConfig{
				Model:       "claude-3-sonnet",
				Region:      "us-west-2",
				Temperature: 0.7,
				MaxTokens:   1000,
				TopP:        0.9,
				TopK:        40,
				MaxRetries:  3,
			},
		},
		{
			name: "minimal config",
			config: InferenceConfig{
				Model: "gpt-4",
			},
			expected: InferenceConfig{
				Model: "gpt-4",
			},
		},
		{
			name:     "empty config",
			config:   InferenceConfig{},
			expected: InferenceConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config)

			// Test YAML marshaling for config files
			data, err := tt.config.MarshalYAML()
			require.NoError(t, err)
			require.NotNil(t, data)
		})
	}
}

func TestUsageCallback(t *testing.T) {
	var capturedProvider, capturedModel string
	var capturedUsage Usage
	var callCount int

	// Test callback function
	callback := func(providerName string, model string, usage Usage) {
		capturedProvider = providerName
		capturedModel = model
		capturedUsage = usage
		callCount++
	}

	// Test callback invocation
	testUsage := Usage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalCost:    0.005,
		Provider:     "bedrock",
		Model:        "claude-3-sonnet",
	}

	callback("bedrock", "claude-3-sonnet", testUsage)

	assert.Equal(t, 1, callCount)
	assert.Equal(t, "bedrock", capturedProvider)
	assert.Equal(t, "claude-3-sonnet", capturedModel)
	assert.Equal(t, testUsage, capturedUsage)
}

func TestUsageExtractor(t *testing.T) {
	// Mock usage extractor implementation
	extractor := &mockUsageExtractor{}

	// Test with nil raw usage
	usage := extractor.ExtractUsage(nil, "test-model", "test-provider")
	assert.Nil(t, usage)

	// Test with mock data
	mockRawUsage := map[string]interface{}{
		"input_tokens":  100,
		"output_tokens": 50,
	}

	usage = extractor.ExtractUsage(mockRawUsage, "test-model", "test-provider")
	require.NotNil(t, usage)
	assert.Equal(t, 100, usage.InputTokens)
	assert.Equal(t, 50, usage.OutputTokens)
	assert.Equal(t, "test-model", usage.Model)
	assert.Equal(t, "test-provider", usage.Provider)
}

func TestClientOptionsFunctionalPattern(t *testing.T) {
	// Test that new functional options work with existing ClientOptions

	config := &InferenceConfig{
		Model:       "claude-3-sonnet",
		Temperature: 0.7,
		MaxTokens:   1000,
	}

	var capturedUsage *Usage
	callback := func(provider, model string, usage Usage) {
		capturedUsage = &usage
	}

	extractor := &mockUsageExtractor{}

	// Build options using functional pattern
	opts := ClientOptions{}

	// Apply new options (these functions will be implemented)
	WithInferenceConfig(config)(&opts)
	WithUsageCallback(callback)(&opts)
	WithUsageExtractor(extractor)(&opts)
	WithDebug(true)(&opts)

	// Verify options were set correctly
	assert.Equal(t, config, opts.InferenceConfig)
	assert.NotNil(t, opts.UsageCallback)
	assert.Equal(t, extractor, opts.UsageExtractor)
	assert.True(t, opts.Debug)

	// Test callback works
	testUsage := Usage{InputTokens: 10, OutputTokens: 5}
	opts.UsageCallback("test", "model", testUsage)
	require.NotNil(t, capturedUsage)
	assert.Equal(t, testUsage, *capturedUsage)
}

func TestBackwardsCompatibility(t *testing.T) {
	// Test that existing ClientOptions usage still works
	opts := ClientOptions{
		SkipVerifySSL: true,
	}

	// Existing pattern should work
	assert.True(t, opts.SkipVerifySSL)

	// New fields should be nil/zero values
	assert.Nil(t, opts.InferenceConfig)
	assert.Nil(t, opts.UsageCallback)
	assert.Nil(t, opts.UsageExtractor)
	assert.False(t, opts.Debug)
}

func TestUsageAggregation(t *testing.T) {
	// Test usage aggregation scenario
	var totalCost float64
	var totalTokens int
	var callCount int

	aggregator := func(provider, model string, usage Usage) {
		totalCost += usage.TotalCost
		totalTokens += usage.TotalTokens
		callCount++
	}

	// Simulate multiple calls
	usages := []Usage{
		{InputTokens: 100, OutputTokens: 50, TotalTokens: 150, TotalCost: 0.001},
		{InputTokens: 200, OutputTokens: 100, TotalTokens: 300, TotalCost: 0.002},
		{InputTokens: 50, OutputTokens: 25, TotalTokens: 75, TotalCost: 0.0005},
	}

	for _, usage := range usages {
		aggregator("bedrock", "claude-3-sonnet", usage)
	}

	assert.Equal(t, 3, callCount)
	assert.Equal(t, 0.0035, totalCost)
	assert.Equal(t, 525, totalTokens) // 150 + 300 + 75
}

// Mock implementation for testing - definition moved to integration_test.go to avoid duplication

func TestUsageValidation(t *testing.T) {
	tests := []struct {
		name    string
		usage   Usage
		isValid bool
	}{
		{
			name: "valid usage with all required fields",
			usage: Usage{
				InputTokens:  100,
				OutputTokens: 50,
				Provider:     "bedrock",
			},
			isValid: true,
		},
		{
			name: "valid usage with zero tokens",
			usage: Usage{
				InputTokens:  0,
				OutputTokens: 0,
				Provider:     "bedrock",
			},
			isValid: true,
		},
		{
			name: "invalid usage missing provider",
			usage: Usage{
				InputTokens:  100,
				OutputTokens: 50,
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.usage.IsValid()
			assert.Equal(t, tt.isValid, isValid)
		})
	}
}

func TestInferenceConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  InferenceConfig
		isValid bool
	}{
		{
			name: "valid config with reasonable values",
			config: InferenceConfig{
				Temperature: 0.7,
				MaxTokens:   1000,
				TopP:        0.9,
			},
			isValid: true,
		},
		{
			name: "invalid temperature too high",
			config: InferenceConfig{
				Temperature: 2.5, // > 2.0
			},
			isValid: false,
		},
		{
			name: "invalid negative max tokens",
			config: InferenceConfig{
				MaxTokens: -100,
			},
			isValid: false,
		},
		{
			name: "edge case: zero values",
			config: InferenceConfig{
				Temperature: 0.0,
				MaxTokens:   0,
			},
			isValid: true, // Zero values should be valid (means use defaults)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.config.IsValid()
			assert.Equal(t, tt.isValid, isValid)
		})
	}
}
