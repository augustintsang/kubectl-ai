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

import "time"

// InferenceConfig holds bedrock-specific inference configuration
type InferenceConfig struct {
	Model         string        `json:"model,omitempty"`
	Region        string        `json:"region,omitempty"`
	Temperature   float32       `json:"temperature,omitempty"`
	MaxTokens     int32         `json:"maxTokens,omitempty"`
	TopP          float32       `json:"topP,omitempty"`
	MaxRetries    int           `json:"maxRetries,omitempty"`
	Timeout       time.Duration `json:"timeout,omitempty"`
	StopSequences []string      `json:"stopSequences,omitempty"`
}

// Usage represents token usage information for bedrock
type Usage struct {
	InputTokens  int       `json:"inputTokens"`
	OutputTokens int       `json:"outputTokens"`
	TotalTokens  int       `json:"totalTokens"`
	Model        string    `json:"model"`
	Provider     string    `json:"provider"`
	Timestamp    time.Time `json:"timestamp"`
}

// UsageCallback is the function signature for usage tracking callbacks
type UsageCallback func(provider, model string, usage Usage)
