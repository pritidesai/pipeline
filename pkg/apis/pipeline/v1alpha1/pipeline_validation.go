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

package v1alpha1

import (
	"context"
	"fmt"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	"strings"

	"github.com/tektoncd/pipeline/pkg/apis/validate"
	"github.com/tektoncd/pipeline/pkg/list"
	"github.com/tektoncd/pipeline/pkg/reconciler/pipeline/dag"
	"github.com/tektoncd/pipeline/pkg/substitution"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/validation"
	"knative.dev/pkg/apis"
)

var _ apis.Validatable = (*Pipeline)(nil)

// Validate checks that the Pipeline structure is valid but does not validate
// that any references resources exist, that is done at run time.
func (p *Pipeline) Validate(ctx context.Context) *apis.FieldError {
	if err := validate.ObjectMetadata(p.GetObjectMeta()); err != nil {
		return err.ViaField("metadata")
	}
	return p.Spec.Validate(ctx)
}

func validateDeclaredResources(ps *PipelineSpec) error {
	encountered := map[string]struct{}{}
	for _, r := range ps.Resources {
		if _, ok := encountered[r.Name]; ok {
			return fmt.Errorf("resource with name %q appears more than once", r.Name)
		}
		encountered[r.Name] = struct{}{}
	}
	required := []string{}
	for _, t := range ps.Tasks {
		if t.Resources != nil {
			for _, input := range t.Resources.Inputs {
				required = append(required, input.Resource)
			}
			for _, output := range t.Resources.Outputs {
				required = append(required, output.Resource)
			}
		}

		for _, condition := range t.Conditions {
			for _, cr := range condition.Resources {
				required = append(required, cr.Resource)
			}
		}
	}
	for _, t := range ps.Finally {
		if t.Resources != nil {
			for _, input := range t.Resources.Inputs {
				required = append(required, input.Resource)
			}
			for _, output := range t.Resources.Outputs {
				required = append(required, output.Resource)
			}
		}
	}

	provided := make([]string, 0, len(ps.Resources))
	for _, resource := range ps.Resources {
		provided = append(provided, resource.Name)
	}
	missing := list.DiffLeft(required, provided)
	if len(missing) > 0 {
		return fmt.Errorf("pipeline declared resources didn't match usage in Tasks: Didn't provide required values: %s", missing)
	}

	return nil
}

func isOutput(outputs []PipelineTaskOutputResource, resource string) bool {
	for _, output := range outputs {
		if output.Resource == resource {
			return true
		}
	}
	return false
}

// validateFrom ensures that the `from` values make sense: that they rely on values from Tasks
// that ran previously, and that the PipelineResource is actually an output of the Task it should come from.
func validateFrom(tasks []PipelineTask) error {
	taskOutputs := map[string][]PipelineTaskOutputResource{}
	for _, task := range tasks {
		var to []PipelineTaskOutputResource
		if task.Resources != nil {
			to = make([]PipelineTaskOutputResource, len(task.Resources.Outputs))
			copy(to, task.Resources.Outputs)
		}
		taskOutputs[task.Name] = to
	}
	for _, t := range tasks {
		inputResources := []PipelineTaskInputResource{}
		if t.Resources != nil {
			inputResources = append(inputResources, t.Resources.Inputs...)
		}

		for _, c := range t.Conditions {
			inputResources = append(inputResources, c.Resources...)
		}

		for _, rd := range inputResources {
			for _, pt := range rd.From {
				outputs, found := taskOutputs[pt]
				if !found {
					return fmt.Errorf("expected resource %s to be from task %s, but task %s doesn't exist", rd.Resource, pt, pt)
				}
				if !isOutput(outputs, rd.Resource) {
					return fmt.Errorf("the resource %s from %s must be an output but is an input", rd.Resource, pt)
				}
			}
		}
	}
	return nil
}

// validateGraph ensures the Pipeline's dependency Graph (DAG) make sense: that there is no dependency
// cycle or that they rely on values from Tasks that ran previously, and that the PipelineResource
// is actually an output of the Task it should come from.
func validateGraph(tasks []PipelineTask) error {
	if _, err := dag.Build(PipelineTaskList(tasks)); err != nil {
		return err
	}
	return nil
}

// validateParamResults ensure that task result variables are properly configured
func validateParamResults(params []v1beta1.Param) error {
	for _, param := range params {
		expressions, ok := v1beta1.GetVarSubstitutionExpressionsForParam(param)
		if ok {
			if v1beta1.LooksLikeContainsResultRefs(expressions) {
				if _, err := v1beta1.NewResultRefs(expressions); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// validatePipelineResults ensure that task result variables are properly configured
func validatePipelineResults(results []PipelineResult) error {
	for _, result := range results {
		expressions, ok := v1beta1.GetVarSubstitutionExpressionsForPipelineResult(result)
		if ok {
			if v1beta1.LooksLikeContainsResultRefs(expressions) {
				if _, err := v1beta1.NewResultRefs(expressions); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Validate checks that taskNames in the Pipeline are valid and that the graph
// of Tasks expressed in the Pipeline makes sense.
func (ps *PipelineSpec) Validate(ctx context.Context) *apis.FieldError {
	if equality.Semantic.DeepEqual(ps, &PipelineSpec{}) {
		return apis.ErrGeneric("expected at least one, got none", "spec.description", "spec.params", "spec.resources", "spec.tasks", "spec.workspaces")
	}

	// PipelineTask must have a valid unique label, at least one of taskRef or taskSpec should be specified
	if err := validatePipelineTasks(ctx, ps.Tasks, ps.Finally); err != nil {
		return err
	}

	// All declared resources should be used, and the Pipeline shouldn't try to use any resources
	// that aren't declared
	if err := validateDeclaredResources(ps); err != nil {
		return apis.ErrInvalidValue(err.Error(), "spec.resources")
	}

	// The from values should make sense
	if err := validateFrom(ps.Tasks); err != nil {
		return apis.ErrInvalidValue(err.Error(), "spec.tasks.resources.inputs.from")
	}

	// Validate the pipeline task graph
	if err := validateGraph(ps.Tasks); err != nil {
		return apis.ErrInvalidValue(err.Error(), "spec.tasks")
	}

	pipelineTaskResults := getPipelineTaskParameters(ps.Tasks, ps.Finally)
	if err := validateParamResults(pipelineTaskResults); err != nil {
		return apis.ErrInvalidValue(err.Error(), "spec.tasks.params.value")
	}

	// The parameter variables should be valid
	pipelineTaskParameters := getPipelineTaskParameters(ps.Tasks, ps.Finally)
	if err := validatePipelineParameterVariables(pipelineTaskParameters, ps.Params); err != nil {
		return err
	}

	// Validate the pipeline's workspaces.
	if err := validatePipelineWorkspaces(ps.Workspaces, ps.Tasks, ps.Finally); err != nil {
		return err
	}

	return nil
}

func validatePipelineTasks(ctx context.Context, tasks []PipelineTask, finalTasks []FinalPipelineTask) *apis.FieldError {
	// Names cannot be duplicated
	taskNames := map[string]struct{}{}
	var err *apis.FieldError
	for i, t := range tasks {
		if err = validatePipelineTaskName(ctx, "spec.tasks", i, t.Name, t.TaskRef, t.TaskSpec, taskNames); err != nil {
			return err
		}
	}
	//finalTaskNames := map[string]struct{}{}
	for i, t := range finalTasks {
		if err = validatePipelineTaskName(ctx, "spec.finally", i, t.Name, t.TaskRef, t.TaskSpec, taskNames); err != nil {
			return err
		}
	}
	return nil
}

func validatePipelineTaskName(ctx context.Context, prefix string, i int, name string, taskRef *TaskRef, taskSpec *TaskSpec, taskNames map[string]struct{}) *apis.FieldError {
	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		return &apis.FieldError{
			Message: fmt.Sprintf("invalid value %q", name),
			Paths:   []string{fmt.Sprintf(prefix+"[%d].name", i)},
			Details: "Pipeline Task name must be a valid DNS Label." +
				"For more info refer to https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names",
		}
	}
	// can't have both taskRef and taskSpec at the same time
	if (taskRef != nil && taskRef.Name != "") && taskSpec != nil {
		return apis.ErrMultipleOneOf(fmt.Sprintf(prefix+"[%d].taskRef", i), fmt.Sprintf(prefix+"[%d].taskSpec", i))
	}
	// Check that one of TaskRef and TaskSpec is present
	if (taskRef == nil || (taskRef != nil && taskRef.Name == "")) && taskSpec == nil {
		return apis.ErrMissingOneOf(fmt.Sprintf(prefix+"[%d].taskRef", i), fmt.Sprintf(prefix+"[%d].taskSpec", i))
	}
	// Validate TaskSpec if it's present
	if taskSpec != nil {
		if err := taskSpec.Validate(ctx); err != nil {
			return err
		}
	}
	if taskRef != nil && taskRef.Name != "" {
		// Task names are appended to the container name, which must exist and
		// must be a valid k8s name
		if errSlice := validation.IsQualifiedName(name); len(errSlice) != 0 {
			return apis.ErrInvalidValue(strings.Join(errSlice, ","), fmt.Sprintf(prefix+"[%d].name", i))
		}
		// TaskRef name must be a valid k8s name
		if errSlice := validation.IsQualifiedName(taskRef.Name); len(errSlice) != 0 {
			return apis.ErrInvalidValue(strings.Join(errSlice, ","), fmt.Sprintf(prefix+"[%d].taskRef.name", i))
		}
		if _, ok := taskNames[name]; ok {
			return apis.ErrMultipleOneOf(fmt.Sprintf(prefix+"[%d].name", i))
		}
		taskNames[name] = struct{}{}
	}
	return nil
}

func validatePipelineWorkspaces(wss []WorkspacePipelineDeclaration, pts []PipelineTask, finalTasks []FinalPipelineTask) *apis.FieldError {
	// Workspace names must be non-empty and unique.
	wsTable := make(map[string]struct{})
	for i, ws := range wss {
		if ws.Name == "" {
			return apis.ErrInvalidValue(fmt.Sprintf("workspace %d has empty name", i), "spec.workspaces")
		}
		if _, ok := wsTable[ws.Name]; ok {
			return apis.ErrInvalidValue(fmt.Sprintf("workspace with name %q appears more than once", ws.Name), "spec.workspaces")
		}
		wsTable[ws.Name] = struct{}{}
	}

	// Any workspaces used in PipelineTasks should have their name declared in the Pipeline's
	// Workspaces list.
	for ptIdx, pt := range pts {
		for wsIdx, ws := range pt.Workspaces {
			if _, ok := wsTable[ws.Workspace]; !ok {
				return apis.ErrInvalidValue(
					fmt.Sprintf("pipeline task %q expects workspace with name %q but none exists in pipeline spec", pt.Name, ws.Workspace),
					fmt.Sprintf("spec.tasks[%d].workspaces[%d]", ptIdx, wsIdx),
				)
			}
		}
	}
	for tIdx, t := range finalTasks {
		for wsIdx, ws := range t.Workspaces {
			if _, ok := wsTable[ws.Workspace]; !ok {
				return apis.ErrInvalidValue(
					fmt.Sprintf("pipeline task %q expects workspace with name %q but none exists in pipeline spec", t.Name, ws.Workspace),
					fmt.Sprintf("spec.finally[%d].workspaces[%d]", tIdx, wsIdx),
				)
			}
		}
	}
	return nil
}

func getPipelineTaskParameters(tasks []PipelineTask, finalTasks []FinalPipelineTask) []v1beta1.Param {
	var parameters []v1beta1.Param
	for _, t := range tasks {
		parameters = append(parameters, t.Params...)
	}
	for _, t := range finalTasks {
		parameters = append(parameters, t.Params...)
	}
	return parameters
}

func validatePipelineParameterVariables(pipelineTaskParameters []v1beta1.Param, params []ParamSpec) *apis.FieldError {
	parameterNames := map[string]struct{}{}
	arrayParameterNames := map[string]struct{}{}

	for _, p := range params {
		// Verify that p is a valid type.
		validType := false
		for _, allowedType := range AllParamTypes {
			if p.Type == allowedType {
				validType = true
			}
		}
		if !validType {
			return apis.ErrInvalidValue(string(p.Type), fmt.Sprintf("spec.params.%s.type", p.Name))
		}

		// If a default value is provided, ensure its type matches param's declared type.
		if (p.Default != nil) && (p.Default.Type != p.Type) {
			return &apis.FieldError{
				Message: fmt.Sprintf(
					"\"%v\" type does not match default value's type: \"%v\"", p.Type, p.Default.Type),
				Paths: []string{
					fmt.Sprintf("spec.params.%s.type", p.Name),
					fmt.Sprintf("spec.params.%s.default.type", p.Name),
				},
			}
		}

		// Add parameter name to parameterNames, and to arrayParameterNames if type is array.
		parameterNames[p.Name] = struct{}{}
		if p.Type == ParamTypeArray {
			arrayParameterNames[p.Name] = struct{}{}
		}
	}
	return validatePipelineVariables(pipelineTaskParameters, "params", parameterNames, arrayParameterNames)
}

func validatePipelineVariables(parameters []v1beta1.Param, prefix string, paramNames map[string]struct{}, arrayParamNames map[string]struct{}) *apis.FieldError {
	for _, param := range parameters {
		if param.Value.Type == ParamTypeString {
			if err := validatePipelineVariable(fmt.Sprintf("param[%s]", param.Name), param.Value.StringVal, prefix, paramNames); err != nil {
				return err
			}
			if err := validatePipelineNoArrayReferenced(fmt.Sprintf("param[%s]", param.Name), param.Value.StringVal, prefix, arrayParamNames); err != nil {
				return err
			}
		} else {
			for _, arrayElement := range param.Value.ArrayVal {
				if err := validatePipelineVariable(fmt.Sprintf("param[%s]", param.Name), arrayElement, prefix, paramNames); err != nil {
					return err
				}
				if err := validatePipelineArraysIsolated(fmt.Sprintf("param[%s]", param.Name), arrayElement, prefix, arrayParamNames); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validatePipelineVariable(name, value, prefix string, vars map[string]struct{}) *apis.FieldError {
	return substitution.ValidateVariable(name, value, prefix, "task parameter", "pipelinespec.params", vars)
}

func validatePipelineNoArrayReferenced(name, value, prefix string, vars map[string]struct{}) *apis.FieldError {
	return substitution.ValidateVariableProhibited(name, value, prefix, "task parameter", "pipelinespec.params", vars)
}

func validatePipelineArraysIsolated(name, value, prefix string, vars map[string]struct{}) *apis.FieldError {
	return substitution.ValidateVariableIsolated(name, value, prefix, "task parameter", "pipelinespec.params", vars)
}
