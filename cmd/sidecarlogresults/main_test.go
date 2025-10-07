/*
Copyright 2025 The Tekton Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// TestWaitForSignal tests that the waitForSignal function correctly
// passes signals to the handler
func TestWaitForSignal(t *testing.T) {
	// Create a channel to know when the test is done
	done := make(chan bool, 1)
	
	// Create a channel to receive the signal in our test handler
	receivedSignal := make(chan os.Signal, 1)
	
	// Create a test signal handler
	testHandler := func(sigs chan os.Signal) {
		// Forward the signal to our test channel
		sig := <-sigs
		receivedSignal <- sig
		done <- true
	}
	
	// Start waitForSignal in a goroutine with our test handler
	go func() {
		waitForSignal(testHandler)
	}()
	
	// Give the goroutine time to set up
	time.Sleep(100 * time.Millisecond)
	
	// Send a test signal
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find process: %v", err)
	}
	
	// Send SIGINT to the process
	err = process.Signal(syscall.SIGINT)
	if err != nil {
		t.Fatalf("Failed to send signal: %v", err)
	}
	
	// Wait for the signal handler to complete with a timeout
	select {
	case <-done:
		// Check which signal was received
		select {
		case sig := <-receivedSignal:
			if sig != syscall.SIGINT {
				t.Errorf("Expected SIGINT but got %v", sig)
			}
		default:
			t.Error("Signal was not forwarded to the test channel")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for signal to be processed")
	}
}

// TestDefaultSignalHandler tests that the default signal handler works correctly
func TestDefaultSignalHandler(t *testing.T) {
	// Create a signal channel
	sigChan := make(chan os.Signal, 1)
	
	// Send a test signal
	sigChan <- syscall.SIGTERM
	
	// The default handler should process the signal without blocking
	// We can't easily test the log output, but we can verify it doesn't hang
	DefaultSignalHandler(sigChan)
	
	// If we got here without hanging, the test passes
}

// TestIsKubernetesNativeSidecarEnabled tests the feature flag detection function
func TestIsKubernetesNativeSidecarEnabled(t *testing.T) {
	// Save original env var value to restore later
	originalValue := os.Getenv(kubernetesNativeSidecarEnvVar)
	defer os.Setenv(kubernetesNativeSidecarEnvVar, originalValue)
	
	testCases := []struct {
		name     string
		envValue string
		expected bool
	}{
		{
			name:     "Feature flag not set",
			envValue: "",
			expected: defaultEnableKubernetesSidecar,
		},
		{
			name:     "Feature flag enabled with 'true'",
			envValue: "true",
			expected: true,
		},
		{
			name:     "Feature flag enabled with 'TRUE' (case insensitive)",
			envValue: "TRUE",
			expected: true,
		},
		{
			name:     "Feature flag disabled with 'false'",
			envValue: "false",
			expected: false,
		},
		{
			name:     "Feature flag disabled with 'FALSE' (case insensitive)",
			envValue: "FALSE",
			expected: false,
		},
		{
			name:     "Feature flag with invalid value",
			envValue: "not-a-bool",
			expected: defaultEnableKubernetesSidecar,
		},
		{
			name:     "Feature flag with 1 (truthy)",
			envValue: "1",
			expected: true,
		},
		{
			name:     "Feature flag with 0 (falsy)",
			envValue: "0",
			expected: false,
		},
		{
			name:     "Feature flag with t (truthy)",
			envValue: "t",
			expected: true,
		},
		{
			name:     "Feature flag with f (falsy)",
			envValue: "f",
			expected: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set the environment variable for this test case
			os.Setenv(kubernetesNativeSidecarEnvVar, tc.envValue)
			
			// Check if the function returns the expected result
			result := isKubernetesNativeSidecarEnabled()
			if result != tc.expected {
				t.Errorf("Expected isKubernetesNativeSidecarEnabled() to return %v for value %q, but got %v",
					tc.expected, tc.envValue, result)
			}
		})
	}
}

// TestConfigMapFeatureFlagIntegration tests that the feature flag is correctly read from the environment
// which would be set by the controller based on the config map
func TestConfigMapFeatureFlagIntegration(t *testing.T) {
	// Save original env var value to restore later
	originalValue := os.Getenv(kubernetesNativeSidecarEnvVar)
	defer os.Setenv(kubernetesNativeSidecarEnvVar, originalValue)
	
	// Test when feature flag is enabled in the config map
	os.Setenv(kubernetesNativeSidecarEnvVar, "true")
	if !isKubernetesNativeSidecarEnabled() {
		t.Error("Expected feature flag to be enabled when set to 'true' in environment")
	}
	
	// Test when feature flag is disabled in the config map
	os.Setenv(kubernetesNativeSidecarEnvVar, "false")
	if isKubernetesNativeSidecarEnabled() {
		t.Error("Expected feature flag to be disabled when set to 'false' in environment")
	}
	
	// Test with mixed case (should be case insensitive)
	os.Setenv(kubernetesNativeSidecarEnvVar, "True")
	if !isKubernetesNativeSidecarEnabled() {
		t.Error("Expected feature flag to be enabled when set to 'True' in environment (case insensitive)")
	}
}

// TestSignalHandlerIntegration tests the integration between waitForSignal and DefaultSignalHandler
func TestSignalHandlerIntegration(t *testing.T) {
	// This test is more complex and might be flaky in CI environments
	// So we'll skip it by default
	t.Skip("Skipping integration test that sends real signals")
	
	// Create a channel to know when the signal handler completes
	done := make(chan bool, 1)
	
	// Create a test handler that signals completion
	testHandler := func(sigs chan os.Signal) {
		<-sigs
		done <- true
	}
	
	// Start waitForSignal in a goroutine
	go func() {
		waitForSignal(testHandler)
	}()
	
	// Give the goroutine time to set up
	time.Sleep(100 * time.Millisecond)
	
	// Send a signal to ourselves
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find process: %v", err)
	}
	
	err = process.Signal(syscall.SIGINT)
	if err != nil {
		t.Fatalf("Failed to send signal: %v", err)
	}
	
	// Wait for the signal handler to complete with a timeout
	select {
	case <-done:
		// Test passed
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for signal to be processed")
	}
}

// Made with Bob
