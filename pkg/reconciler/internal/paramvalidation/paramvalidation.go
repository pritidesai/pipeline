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

package paramvalidation

import (
	"fmt"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"github.com/tektoncd/pipeline/pkg/substitution"
	"k8s.io/apimachinery/pkg/util/sets"
)

// ExtractParamArrayLengths extract and return the lengths of all array params
// Example of returned value: {"a-array-params": 2,"b-array-params": 2 }
func ExtractParamArrayLengths(defaults []v1beta1.ParamSpec, params []v1beta1.Param) map[string]int {
	// Collect all array params
	arrayParamsLengths := make(map[string]int)

	// Collect array params lengths from defaults
	for _, p := range defaults {
		if p.Default != nil {
			if p.Default.Type == v1beta1.ParamTypeArray {
				arrayParamsLengths[p.Name] = len(p.Default.ArrayVal)
			}
		}
	}

	// Collect array params lengths from params
	for _, p := range params {
		if p.Value.Type == v1beta1.ParamTypeArray {
			arrayParamsLengths[p.Name] = len(p.Value.ArrayVal)
		}
	}
	return arrayParamsLengths
}

// ValidateOutofBoundArrayParams validates if the array indexing params are out of bound
// example of arrayIndexingParams: ["$(params.a-array-param[1])", "$(params.b-array-param[2])"]
// example of arrayParamsLengths: {"a-array-params": 2,"b-array-params": 2 }
func ValidateOutofBoundArrayParams(arrayIndexingParams []string, arrayParamsLengths map[string]int) error {
	outofBoundParams := sets.String{}
	for _, val := range arrayIndexingParams {
		indexString := substitution.ExtractIndexString(val)
		idx, _ := substitution.ExtractIndex(indexString)
		// this will extract the param name from reference
		// e.g. $(params.a-array-param[1]) -> a-array-param
		paramName, _, _ := substitution.ExtractVariablesFromString(substitution.TrimArrayIndex(val), "params")

		if paramLength, ok := arrayParamsLengths[paramName[0]]; ok {
			if idx >= paramLength {
				outofBoundParams.Insert(val)
			}
		}
	}
	if outofBoundParams.Len() > 0 {
		return fmt.Errorf("non-existent param references:%v", outofBoundParams.List())
	}
	return nil
}

// ExtractArrayIndexingParamRefs takes a string of the form `foo-$(params.array-param[1])-bar` and extracts the portions of the string that reference an element in an array param.
// For example, for the string “foo-$(params.array-param[1])-bar-$(params.other-array-param[2])-$(params.string-param)`,
// it would return ["$(params.array-param[1])", "$(params.other-array-param[2])"].
func ExtractArrayIndexingParamRefs(paramReference string) []string {
	l := []string{}
	list := substitution.ExtractParamsExpressions(paramReference)
	for _, val := range list {
		indexString := substitution.ExtractIndexString(val)
		if indexString != "" {
			l = append(l, val)
		}
	}
	return l
}
