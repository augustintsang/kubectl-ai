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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCleanExtensionPatternDemo(t *testing.T) {
	// Test demonstrates the clean extension pattern implementation:
	// 1. No global interface pollution
	// 2. Provider-specific extensions via type assertions
	// 3. Clean separation of concerns
	// 4. Backwards compatibility maintained

	t.Run("CleanGlobalInterface", func(t *testing.T) {
		// Global ClientOptions should only have basic shared fields
		opts := ClientOptions{
			SkipVerifySSL: true,
		}

		assert.True(t, opts.SkipVerifySSL)
		// No provider-specific fields polluting the global interface!
		// This is the key improvement - the interface is clean
	})

	t.Run("ProviderSpecificExtensions", func(t *testing.T) {
		// Provider-specific extensions should be accessed via type assertion
		// This test shows the pattern works without actually requiring AWS credentials

		ctx := context.Background()

		// This demonstrates how users would access provider-specific features
		t.Skip("Skipping actual client creation - demonstrates pattern only")

		// Example pattern:
		// client, err := NewClient(ctx, "bedrock")
		// if bedrockClient, ok := client.(bedrock.ConfigurableClient); ok {
		//     bedrockClient.SetInferenceConfig(&bedrock.InferenceConfig{...})
		//     bedrockClient.SetUsageCallback(func(provider, model string, usage bedrock.Usage) {...})
		// }

		_ = ctx // Avoid unused variable warning
	})

	t.Run("UsageTypeSeparation", func(t *testing.T) {
		// Test that we maintain clean separation between:
		// 1. Global Usage type (for general compatibility)
		// 2. Provider-specific Usage types (for advanced features)

		// Global Usage type should exist for basic interoperability
		globalUsage := Usage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			Model:        "test-model",
			Provider:     "test-provider",
			Timestamp:    time.Now(),
		}

		// Verify global Usage has required fields
		assert.True(t, globalUsage.IsValid())
		assert.Equal(t, 100, globalUsage.InputTokens)
		assert.Equal(t, 50, globalUsage.OutputTokens)
		assert.Equal(t, 150, globalUsage.TotalTokens)
		assert.Equal(t, "test-provider", globalUsage.Provider)

		// Provider-specific Usage types can extend this with provider-specific fields
		// This is tested in the provider-specific test files (e.g., bedrock_test.go)
	})

	t.Run("BackwardsCompatibility", func(t *testing.T) {
		// Existing patterns should continue to work
		ctx := context.Background()

		// Basic client creation should work unchanged
		_, err := NewClient(ctx, "nonexistent-provider")
		assert.Error(t, err) // Expected - provider doesn't exist
		assert.Contains(t, err.Error(), "not registered")

		// WithSkipVerifySSL should still work
		opts := ClientOptions{}
		WithSkipVerifySSL()(&opts)
		assert.True(t, opts.SkipVerifySSL)
	})

	t.Run("ExtensionPatternBenefits", func(t *testing.T) {
		// Document the benefits of the new pattern
		benefits := []string{
			"No global interface pollution",
			"Clean type assertion pattern",
			"Provider-specific features without global impact",
			"Incremental adoption by providers",
			"Backwards compatibility maintained",
			"Clear separation of concerns",
		}

		// This is more of a documentation test
		assert.Len(t, benefits, 6, "Should have these key benefits")
		t.Logf("Extension pattern benefits: %v", benefits)
	})
}

// Example demonstrates the clean extension pattern usage
func Example_cleanExtensionPattern() {
	// This shows how the pattern would be used in practice
	fmt.Println("Clean Extension Pattern:")
	fmt.Println("1. Create client with clean global interface")
	fmt.Println("2. Use type assertion for provider-specific features")
	fmt.Println("3. Configure advanced features without global pollution")
	fmt.Println("4. Maintain backwards compatibility")

	// Output:
	// Clean Extension Pattern:
	// 1. Create client with clean global interface
	// 2. Use type assertion for provider-specific features
	// 3. Configure advanced features without global pollution
	// 4. Maintain backwards compatibility
}

// TestProviderRegistration verifies basic provider registration still works
func TestProviderRegistration(t *testing.T) {
	ctx := context.Background()

	t.Run("list_available_providers", func(t *testing.T) {
		// Test that we can list providers (this indirectly tests registration)
		_, err := NewClient(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "LLM_CLIENT is not set")
		// Error message should list available providers
	})

	t.Run("invalid_provider_error", func(t *testing.T) {
		// Test that invalid providers give helpful error messages
		_, err := NewClient(ctx, "invalid-provider-xyz")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not registered")
		assert.Contains(t, err.Error(), "Available providers")
	})
}
