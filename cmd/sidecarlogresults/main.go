/*
Copyright 2019 The Tekton Authors

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
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/tektoncd/pipeline/internal/sidecarlogresults"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	"github.com/tektoncd/pipeline/pkg/pod"
)

// SignalHandler defines a function that handles OS signals
type SignalHandler func(chan os.Signal)

// DefaultSignalHandler is the default implementation of signal handling
var DefaultSignalHandler SignalHandler = func(sigs chan os.Signal) {
	sig := <-sigs
	log.Printf("Received signal %s, exiting...", sig)
}

// Environment variable name for the Kubernetes sidecar feature flag
const (
	// This environment variable would be set by the controller based on the feature flag in the config map
	kubernetesNativeSidecarEnvVar = "ENABLE_KUBERNETES_SIDECAR"

	// Default value for the feature flag if not set in the environment
	defaultEnableKubernetesSidecar = false
)

func main() {
	var resultsDir string
	var resultNames string
	var stepResultsStr string
	var stepNames string

	flag.StringVar(&resultsDir, "results-dir", pipeline.DefaultResultPath, "Path to the results directory. Default is /tekton/results")
	flag.StringVar(&resultNames, "result-names", "", "comma separated result names to expect from the steps running in the pod. eg. foo,bar,baz")
	flag.StringVar(&stepResultsStr, "step-results", "", "json containing a map of step Name as key and list of result Names. eg. {\"stepName\":[\"foo\",\"bar\",\"baz\"]}")
	flag.StringVar(&stepNames, "step-names", "", "comma separated step names. eg. foo,bar,baz")
	flag.Parse()

	var expectedResults []string
	// strings.Split returns [""] instead of [] for empty string, we don't want pass [""] to other methods.
	if len(resultNames) > 0 {
		expectedResults = strings.Split(resultNames, ",")
	}
	expectedStepResults := map[string][]string{}
	if err := json.Unmarshal([]byte(stepResultsStr), &expectedStepResults); err != nil {
		log.Fatal(err)
	}
	err := sidecarlogresults.LookForResults(os.Stdout, pod.RunDir, resultsDir, expectedResults, pipeline.StepsDir, expectedStepResults)
	if err != nil {
		log.Fatal(err)
	}

	var names []string
	if len(stepNames) > 0 {
		names = strings.Split(stepNames, ",")
	}
	err = sidecarlogresults.LookForArtifacts(os.Stdout, names, pod.RunDir)
	if err != nil {
		log.Fatal(err)
	}

	// Debug: Log all environment variables to help diagnose the issue
	log.Println("DEBUG: Environment variables:")
	for _, env := range os.Environ() {
		log.Println("DEBUG: " + env)
	}

	// Check if Kubernetes native sidecar feature is enabled from the config map
	// This value would be set by the controller based on the feature flag in the config map
	enabled := isKubernetesNativeSidecarEnabled()
	log.Printf("DEBUG: Kubernetes native sidecar enabled: %v", enabled)
	log.Printf("DEBUG: ENABLE_KUBERNETES_SIDECAR env value: %q", os.Getenv(kubernetesNativeSidecarEnvVar))

	if enabled {
		// If native sidecar is enabled, wait for termination signal
		log.Println("Results collection complete. Kubernetes native sidecar enabled, waiting for termination signal...")
		waitForSignal(DefaultSignalHandler)
	} else {
		// If native sidecar is not enabled, just exit normally
		log.Println("Results collection complete. Kubernetes native sidecar not enabled, exiting normally.")
	}
}

// isKubernetesNativeSidecarEnabled checks if the Kubernetes native sidecar feature is enabled
// by reading the environment variable that would be set by the controller based on the config map
func isKubernetesNativeSidecarEnabled() bool {
	value := os.Getenv(kubernetesNativeSidecarEnvVar)
	if value == "" {
		log.Printf("DEBUG: %s environment variable not set, using default: %v",
			kubernetesNativeSidecarEnvVar, defaultEnableKubernetesSidecar)
		return defaultEnableKubernetesSidecar
	}

	// Check for case-insensitive "true" or "false" values
	valueLower := strings.ToLower(value)
	if valueLower == "true" {
		return true
	} else if valueLower == "false" {
		return false
	}

	// Try parsing as a boolean
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		log.Printf("Warning: Invalid value for %s: %s. Defaulting to %v.",
			kubernetesNativeSidecarEnvVar, value, defaultEnableKubernetesSidecar)
		return defaultEnableKubernetesSidecar
	}

	return enabled
}

// waitForSignal blocks until the process receives a termination signal
// It uses the provided handler to process the signal
func waitForSignal(handler SignalHandler) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	handler(sigs)
}
