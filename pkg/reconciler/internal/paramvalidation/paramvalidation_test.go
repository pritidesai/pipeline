/*
Copyright 2023 The Tekton Authors

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

package paramvalidation_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/reconciler/internal/paramvalidation"
)

func TestExtractParamArrayLengths(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pp       []v1beta1.ParamSpec
		prp      []v1beta1.Param
		expected map[string]int
	}{{
		name: "get lengths from default array params",
		pp: []v1beta1.ParamSpec{
			{
				Name:    "an-array-param",
				Type:    v1beta1.ParamTypeArray,
				Default: v1beta1.NewStructuredValues("1", "2"),
			},
		},
		expected: map[string]int{"an-array-param": 2},
	}, {
		name: "get lengths from array params",
		pp: []v1beta1.ParamSpec{
			{
				Name: "an-array-param",
				Type: v1beta1.ParamTypeArray,
			},
		},
		prp: []v1beta1.Param{
			{
				Name:  "an-array-param",
				Value: *v1beta1.NewStructuredValues("1", "2")},
		},
		expected: map[string]int{"an-array-param": 2},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			m := paramvalidation.ExtractParamArrayLengths(tc.pp, tc.prp)
			if d := cmp.Diff(tc.expected, m); d != "" {
				t.Errorf("args: diff(-want,+got):\n%s", d)
			}
		})
	}
}

func TestValidateOutofBoundArrayParams(t *testing.T) {
	for _, tc := range []struct {
		name                string
		arrayIndexingParams []string
		arrayParamsLengths  map[string]int
	}{{
		name:                "normal param name",
		arrayIndexingParams: []string{"$(params.an-array-param[1])"},
		arrayParamsLengths:  map[string]int{"an-array-param": 2},
	}, {
		name:                "double quotes param name",
		arrayIndexingParams: []string{`$(params["an-array-param[1]"])`},
		arrayParamsLengths:  map[string]int{"an-array-param": 2},
	}, {
		name:                "single quote param name",
		arrayIndexingParams: []string{"$(params['an-array-param[1]'])"},
		arrayParamsLengths:  map[string]int{"an-array-param": 2},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if err := paramvalidation.ValidateOutofBoundArrayParams(tc.arrayIndexingParams, tc.arrayParamsLengths); err != nil {
				t.Errorf("got unexpected error:%v", err)
			}
		})
	}
}

func TestValidateOutofBoundArrayParams_Error(t *testing.T) {
	for _, tc := range []struct {
		name                string
		arrayIndexingParams []string
		arrayParamsLengths  map[string]int
	}{{
		name:                "normal param name",
		arrayIndexingParams: []string{"$(params.an-array-param[3])"},
		arrayParamsLengths:  map[string]int{"an-array-param": 2},
	}, {
		name:                "double quotes param name",
		arrayIndexingParams: []string{`$(params["an-array-param[3]"])`},
		arrayParamsLengths:  map[string]int{"an-array-param": 2},
	}, {
		name:                "single quote param name",
		arrayIndexingParams: []string{"$(params['an-array-param[3]'])"},
		arrayParamsLengths:  map[string]int{"an-array-param": 2},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if err := paramvalidation.ValidateOutofBoundArrayParams(tc.arrayIndexingParams, tc.arrayParamsLengths); err == nil {
				t.Errorf("Expected error but got nil")
			}
		})
	}
}

func TestExtractArrayIndexingParamRefs(t *testing.T) {
	for _, tc := range []struct {
		name           string
		paramReference string
		expected       []string
	}{{
		name:           "get single ref from strings",
		paramReference: "$(params.an-array-param[0])",
		expected:       []string{"$(params.an-array-param[0])"},
	}, {
		name:           "get multiple refs from string",
		paramReference: "$(params.an-array-param[0]) $(params.an-array-param[1])",
		expected:       []string{"$(params.an-array-param[0])", "$(params.an-array-param[1])"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			p := paramvalidation.ExtractArrayIndexingParamRefs(tc.paramReference)
			if d := cmp.Diff(tc.expected, p); d != "" {
				t.Errorf("args: diff(-want,+got):\n%s", d)
			}
		})
	}
}
