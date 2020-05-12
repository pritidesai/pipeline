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

package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/davecgh/go-spew/spew"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"knative.dev/pkg/apis"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	resourcev1alpha1 "github.com/tektoncd/pipeline/pkg/apis/resource/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/contexts"
	"github.com/tektoncd/pipeline/pkg/list"
	"github.com/tektoncd/pipeline/pkg/names"
	"github.com/tektoncd/pipeline/pkg/reconciler/pipeline/dag"
	"github.com/tektoncd/pipeline/pkg/reconciler/taskrun/resources"
)

const (
	// ReasonRunning indicates that the reason for the inprogress status is that the TaskRun
	// is just starting to be reconciled
	ReasonRunning = "Running"

	// ReasonFailed indicates that the reason for the failure status is that one of the TaskRuns failed
	ReasonFailed = "Failed"

	// ReasonCancelled indicates that the reason for the cancelled status is that one of the TaskRuns cancelled
	ReasonCancelled = "Cancelled"

	// ReasonSucceeded indicates that the reason for the finished status is that all of the TaskRuns
	// completed successfully
	ReasonSucceeded = "Succeeded"

	// ReasonCompleted indicates that the reason for the finished status is that all of the TaskRuns
	// completed successfully but with some conditions checking failed
	ReasonCompleted = "Completed"

	// ReasonTimedOut indicates that the PipelineRun has taken longer than its configured
	// timeout
	ReasonTimedOut = "PipelineRunTimeout"

	// ReasonConditionCheckFailed indicates that the reason for the failure status is that the
	// condition check associated to the pipeline task evaluated to false
	ReasonConditionCheckFailed = "ConditionCheckFailed"
)

// TaskNotFoundError indicates that the resolution failed because a referenced Task couldn't be retrieved
type TaskNotFoundError struct {
	Name string
	Msg  string
}

func (e *TaskNotFoundError) Error() string {
	return fmt.Sprintf("Couldn't retrieve Task %q: %s", e.Name, e.Msg)
}

type ConditionNotFoundError struct {
	Name string
	Msg  string
}

func (e *ConditionNotFoundError) Error() string {
	return fmt.Sprintf("Couldn't retrieve Condition %q: %s", e.Name, e.Msg)
}

// ResolvedPipelineRunTask contains a Task and its associated TaskRun, if it
// exists. TaskRun can be nil to represent there being no TaskRun.
type ResolvedPipelineRunTask struct {
	TaskRunName           string
	TaskRun               *v1beta1.TaskRun
	PipelineTask          *v1beta1.PipelineTask
	ResolvedTaskResources *resources.ResolvedTaskResources
	// ConditionChecks ~~TaskRuns but for evaling conditions
	ResolvedConditionChecks TaskConditionCheckState // Could also be a TaskRun or maybe just a Pod?
}

// PipelineRunState is a slice of ResolvedPipelineRunTasks the represents the current execution
// state of the PipelineRun.
type PipelineRunState []*ResolvedPipelineRunTask

func (t ResolvedPipelineRunTask) IsDone() (isDone bool) {
	if t.TaskRun == nil || t.PipelineTask == nil {
		return
	}

	status := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
	retriesDone := len(t.TaskRun.Status.RetriesStatus)
	retries := t.PipelineTask.Retries
	isDone = status.IsTrue() || status.IsFalse() && retriesDone >= retries
	return
}

// IsSuccessful returns true only if the taskrun itself has completed successfully
func (t ResolvedPipelineRunTask) IsSuccessful() bool {
	if t.TaskRun == nil {
		return false
	}
	c := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
	if c == nil {
		return false
	}

	return c.Status == corev1.ConditionTrue
}

// IsFailure returns true only if the taskrun itself has failed
func (t ResolvedPipelineRunTask) IsFailure() bool {
	if t.TaskRun == nil {
		return false
	}
	c := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
	retriesDone := len(t.TaskRun.Status.RetriesStatus)
	retries := t.PipelineTask.Retries
	return c.IsFalse() && retriesDone >= retries
}

// IsCancelled returns true only if the taskrun itself has cancelled
func (t ResolvedPipelineRunTask) IsCancelled() bool {
	if t.TaskRun == nil {
		return false
	}

	c := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
	if c == nil {
		return false
	}

	return c.IsFalse() && c.Reason == v1beta1.TaskRunSpecStatusCancelled
}

// ToMap returns a map that maps pipeline task name to the resolved pipeline run task
func (state PipelineRunState) ToMap() map[string]*ResolvedPipelineRunTask {
	m := make(map[string]*ResolvedPipelineRunTask)
	for _, rprt := range state {
		m[rprt.PipelineTask.Name] = rprt
	}
	return m
}

func (state PipelineRunState) IsDone() (isDone bool) {
	isDone = true
	for _, t := range state {
		if t.TaskRun == nil || t.PipelineTask == nil {
			return false
		}
		isDone = isDone && t.IsDone()
		if !isDone {
			return
		}
	}
	return
}

// IsBeforeFirstTaskRun returns true if the PipelineRun has not yet started its first TaskRun
func (state PipelineRunState) IsBeforeFirstTaskRun() bool {
	for _, t := range state {
		if t.TaskRun != nil {
			return false
		}
	}
	return true
}

// GetNextTasks will return the next ResolvedPipelineRunTasks to execute, which are the ones in the
// list of candidateTasks which aren't yet indicated in state to be running.
func (state PipelineRunState) GetNextTasks(candidateTasks map[string]struct{}, runState *v1beta1.PipelineTaskRunState) []*ResolvedPipelineRunTask {
	tasks := []*ResolvedPipelineRunTask{}
	for _, t := range state {
		if _, ok := candidateTasks[t.PipelineTask.Name]; ok && t.TaskRun == nil {
			tasks = append(tasks, t)
		}
		if _, ok := candidateTasks[t.PipelineTask.Name]; ok && t.TaskRun != nil {
			status := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
			if status != nil && status.IsFalse() {
				if !(t.TaskRun.IsCancelled() || status.Reason == v1beta1.TaskRunSpecStatusCancelled || status.Reason == ReasonConditionCheckFailed) {
					if len(t.TaskRun.Status.RetriesStatus) < t.PipelineTask.Retries {
						tasks = append(tasks, t)
					}
				}
			}
		}
	}
	// list of tasks in execution queue is empty, check either pipeline has finished executing all pipelineTasks
	// or any one of the pipelineTask has failed
	spew.Dump("At this point runState looks like")
	spew.Dump(runState)
	spew.Dump("At this point runState looks like")
	if len(tasks) == 0 {
		if runState.Failed > 0 || state.isDAGTasksFinished(runState) {
			// return list of tasks with all final tasks
			for _, t := range state {
				if _, ok := runState.FinalTasks[t.PipelineTask.Name]; ok && t.TaskRun == nil {
					tasks = append(tasks, t)
				}
			}
		}
	}
	return tasks
}

// SuccessfulPipelineTaskNames returns a list of the names of all of the PipelineTasks in state
// which have successfully completed.
func (state PipelineRunState) SuccessfulPipelineTaskNames() []string {
	done := []string{}
	for _, t := range state {
		if t.IsSuccessful() {
			done = append(done, t.PipelineTask.Name)
		}
	}
	return done
}

// GetTaskRun is a function that will retrieve the TaskRun name.
type GetTaskRun func(name string) (*v1beta1.TaskRun, error)

// GetResourcesFromBindings will retrieve all Resources bound in PipelineRun pr and return a map
// from the declared name of the PipelineResource (which is how the PipelineResource will
// be referred to in the PipelineRun) to the PipelineResource, obtained via getResource.
func GetResourcesFromBindings(pr *v1beta1.PipelineRun, getResource resources.GetResource) (map[string]*resourcev1alpha1.PipelineResource, error) {
	rs := map[string]*resourcev1alpha1.PipelineResource{}
	for _, resource := range pr.Spec.Resources {
		r, err := resources.GetResourceFromBinding(&resource, getResource)
		if err != nil {
			return rs, fmt.Errorf("error following resource reference for %s: %w", resource.Name, err)
		}
		rs[resource.Name] = r
	}
	return rs, nil
}

// ValidateResourceBindings validate that the PipelineResources declared in Pipeline p are bound in PipelineRun.
func ValidateResourceBindings(p *v1beta1.PipelineSpec, pr *v1beta1.PipelineRun) error {
	required := make([]string, 0, len(p.Resources))
	optional := make([]string, 0, len(p.Resources))
	for _, resource := range p.Resources {
		if resource.Optional {
			// create a list of optional resources
			optional = append(optional, resource.Name)
		} else {
			// create a list of required resources
			required = append(required, resource.Name)
		}
	}
	provided := make([]string, 0, len(pr.Spec.Resources))
	for _, resource := range pr.Spec.Resources {
		provided = append(provided, resource.Name)
	}
	// verify that the list of required resources exists in the provided resources
	missing := list.DiffLeft(required, provided)
	if len(missing) > 0 {
		return fmt.Errorf("Pipeline's declared required resources are missing from the PipelineRun: %s", missing)
	}
	// verify that the list of provided resources does not have any extra resources (outside of required and optional resources combined)
	extra := list.DiffLeft(provided, append(required, optional...))
	if len(extra) > 0 {
		return fmt.Errorf("PipelineRun's declared resources didn't match usage in Pipeline: %s", extra)
	}
	return nil
}

// ValidateWorkspaceBindings validates that the Workspaces expected by a Pipeline are provided by a PipelineRun.
func ValidateWorkspaceBindings(p *v1beta1.PipelineSpec, pr *v1beta1.PipelineRun) error {
	pipelineRunWorkspaces := make(map[string]v1beta1.WorkspaceBinding)
	for _, binding := range pr.Spec.Workspaces {
		pipelineRunWorkspaces[binding.Name] = binding
	}

	for _, ws := range p.Workspaces {
		if _, ok := pipelineRunWorkspaces[ws.Name]; !ok {
			return fmt.Errorf("pipeline expects workspace with name %q be provided by pipelinerun", ws.Name)
		}
	}
	return nil
}

// ValidateServiceaccountMapping validates that the ServiceAccountNames defined by a PipelineRun are not correct.
func ValidateServiceaccountMapping(p *v1beta1.PipelineSpec, pr *v1beta1.PipelineRun) error {
	pipelineTasks := make(map[string]string)
	for _, task := range p.Tasks {
		pipelineTasks[task.Name] = task.Name
	}

	for _, name := range pr.Spec.ServiceAccountNames {
		if _, ok := pipelineTasks[name.TaskName]; !ok {
			return fmt.Errorf("PipelineRun's ServiceAccountNames defined wrong taskName: %q, not existed in Pipeline", name.TaskName)
		}
	}
	return nil
}

// ResolvePipelineRun retrieves all Tasks instances which are reference by tasks, getting
// instances from getTask. If it is unable to retrieve an instance of a referenced Task, it
// will return an error, otherwise it returns a list of all of the Tasks retrieved.
// It will retrieve the Resources needed for the TaskRun using the mapping of providedResources.
func ResolvePipelineRun(
	ctx context.Context,
	pipelineRun v1beta1.PipelineRun,
	getTask resources.GetTask,
	getTaskRun resources.GetTaskRun,
	getClusterTask resources.GetClusterTask,
	getCondition GetCondition,
	tasks []v1beta1.PipelineTask,
	providedResources map[string]*resourcev1alpha1.PipelineResource,
) (PipelineRunState, error) {

	state := []*ResolvedPipelineRunTask{}
	for i := range tasks {
		pt := tasks[i]

		rprt := ResolvedPipelineRunTask{
			PipelineTask: &pt,
			TaskRunName:  getTaskRunName(pipelineRun.Status.TaskRuns, pt.Name, pipelineRun.Name),
		}

		// Find the Task that this PipelineTask is using
		var (
			t        v1beta1.TaskInterface
			err      error
			spec     v1beta1.TaskSpec
			taskName string
			kind     v1beta1.TaskKind
		)

		if pt.TaskRef != nil {
			if pt.TaskRef.Kind == v1beta1.ClusterTaskKind {
				t, err = getClusterTask(pt.TaskRef.Name)
			} else {
				t, err = getTask(pt.TaskRef.Name)
			}
			if err != nil {
				return nil, &TaskNotFoundError{
					Name: pt.TaskRef.Name,
					Msg:  err.Error(),
				}
			}
			spec = t.TaskSpec()
			taskName = t.TaskMetadata().Name
			kind = pt.TaskRef.Kind
		} else {
			spec = *pt.TaskSpec
		}
		spec.SetDefaults(contexts.WithUpgradeViaDefaulting(ctx))
		rtr, err := ResolvePipelineTaskResources(pt, &spec, taskName, kind, providedResources)
		if err != nil {
			return nil, fmt.Errorf("couldn't match referenced resources with declared resources: %w", err)
		}

		rprt.ResolvedTaskResources = rtr

		taskRun, err := getTaskRun(rprt.TaskRunName)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("error retrieving TaskRun %s: %w", rprt.TaskRunName, err)
			}
		}
		if taskRun != nil {
			rprt.TaskRun = taskRun
		}

		// Get all conditions that this pipelineTask will be using, if any
		if len(pt.Conditions) > 0 {
			rcc, err := resolveConditionChecks(&pt, pipelineRun.Status.TaskRuns, rprt.TaskRunName, getTaskRun, getCondition, providedResources)
			if err != nil {
				return nil, err
			}
			rprt.ResolvedConditionChecks = rcc
		}

		// Add this task to the state of the PipelineRun
		state = append(state, &rprt)
	}
	return state, nil
}

// getConditionCheckName should return a unique name for a `ConditionCheck` if one has not already been defined, and the existing one otherwise.
func getConditionCheckName(taskRunStatus map[string]*v1beta1.PipelineRunTaskRunStatus, trName, conditionRegisterName string) string {
	trStatus, ok := taskRunStatus[trName]
	if ok && trStatus.ConditionChecks != nil {
		for k, v := range trStatus.ConditionChecks {
			// TODO(1022): Should  we allow multiple conditions of the same type?
			if conditionRegisterName == v.ConditionName {
				return k
			}
		}
	}
	return names.SimpleNameGenerator.RestrictLengthWithRandomSuffix(fmt.Sprintf("%s-%s", trName, conditionRegisterName))
}

// getTaskRunName should return a unique name for a `TaskRun` if one has not already been defined, and the existing one otherwise.
func getTaskRunName(taskRunsStatus map[string]*v1beta1.PipelineRunTaskRunStatus, ptName, prName string) string {
	for k, v := range taskRunsStatus {
		if v.PipelineTaskName == ptName {
			return k
		}
	}

	return names.SimpleNameGenerator.RestrictLengthWithRandomSuffix(fmt.Sprintf("%s-%s", prName, ptName))
}

// GetPipelineConditionStatus will return the Condition that the PipelineRun prName should be
// updated with, based on the status of the TaskRuns in state.
func GetPipelineConditionStatus(pr *v1beta1.PipelineRun, state PipelineRunState, logger *zap.SugaredLogger, dag *dag.Graph) *apis.Condition {
	// We have 4 different states here:
	// 1. Timed out -> Failed
	// 2. Any one TaskRun has failed - >Failed. This should change with #1020 and #1023
	// 3. All tasks are done or are skipped (i.e. condition check failed).-> Success
	// 4. A Task or Condition is running right now  or there are things left to run -> Running
	if pr.IsTimedOut() {
		return &apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionFalse,
			Reason:  ReasonTimedOut,
			Message: fmt.Sprintf("PipelineRun %q failed to finish within %q", pr.Name, pr.Spec.Timeout.Duration.String()),
		}
	}

	for _, rprt := range state {
		if pr.Status.RunState.GetTaskState(rprt.PipelineTask.Name) == v1beta1.PipelineTaskCancelled {
			logger.Infof("TaskRun %v is cancelled, so PipelineRun %s is cancelled", rprt.TaskRunName, pr.Name)
			return &apis.Condition{
				Type:    apis.ConditionSucceeded,
				Status:  corev1.ConditionFalse,
				Reason:  ReasonCancelled,
				Message: fmt.Sprintf("TaskRun %v has cancelled", rprt.PipelineTask.Name),
			}

		}
		// A single failed task mean we fail the pipeline without any final tasks
		// pipeline execution continues and final tasks are executed if they are specified
		if len(pr.Status.RunState.FinalTasks) == 0 {
			if _, ok := pr.Status.RunState.DAGTasks[rprt.PipelineTask.Name]; ok {
				if pr.Status.RunState.GetTaskState(rprt.PipelineTask.Name) == v1beta1.PipelineTaskFailed {
					logger.Infof("TaskRun %v has failed, so PipelineRun %s has failed, retries done: %b", rprt.TaskRunName, pr.Name, len(rprt.TaskRun.Status.RetriesStatus))
					return &apis.Condition{
						Type:    apis.ConditionSucceeded,
						Status:  corev1.ConditionFalse,
						Reason:  ReasonFailed,
						Message: fmt.Sprintf("TaskRun %v has failed", rprt.TaskRun.Name),
					}
				}
			}
		}
	}

	// Check to see if all tasks are success or skipped
	if state.isDAGTasksFinished(pr.Status.RunState) && state.isFinalTasksFinished(pr.Status.RunState) {
		logger.Infof("All TaskRuns have finished for PipelineRun %s so it has finished", pr.Name)
		reason := ReasonSucceeded
		if pr.Status.RunState.Skipped != 0 {
			reason = ReasonCompleted
		}
		return &apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionTrue,
			Reason:  reason,
			Message: fmt.Sprintf("Tasks Completed: %d, Skipped: %d", pr.Status.RunState.Succeeded, pr.Status.RunState.Skipped),
		}
	}

	// Hasn't timed out; no taskrun failed yet; and not all tasks have finished....
	// Must keep running then....
	return &apis.Condition{
		Type:   apis.ConditionSucceeded,
		Status: corev1.ConditionUnknown,
		Reason: ReasonRunning,
		Message: fmt.Sprintf("Tasks Completed: %d, Incomplete: %d, Skipped: %d", pr.Status.RunState.Succeeded,
			pr.Status.RunState.NotStarted, pr.Status.RunState.Skipped),
	}
}

func resolveConditionChecks(pt *v1beta1.PipelineTask, taskRunStatus map[string]*v1beta1.PipelineRunTaskRunStatus, taskRunName string, getTaskRun resources.GetTaskRun, getCondition GetCondition, providedResources map[string]*resourcev1alpha1.PipelineResource) ([]*ResolvedConditionCheck, error) {
	rccs := []*ResolvedConditionCheck{}
	for i := range pt.Conditions {
		ptc := pt.Conditions[i]
		cName := ptc.ConditionRef
		crName := fmt.Sprintf("%s-%s", cName, strconv.Itoa(i))
		c, err := getCondition(cName)
		if err != nil {
			return nil, &ConditionNotFoundError{
				Name: cName,
				Msg:  err.Error(),
			}
		}
		conditionCheckName := getConditionCheckName(taskRunStatus, taskRunName, crName)
		cctr, err := getTaskRun(conditionCheckName)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("error retrieving ConditionCheck %s for taskRun name %s : %w", conditionCheckName, taskRunName, err)
			}
		}
		conditionResources := map[string]*resourcev1alpha1.PipelineResource{}
		for _, declared := range ptc.Resources {
			if r, ok := providedResources[declared.Resource]; ok {
				conditionResources[declared.Name] = r
			} else {
				for _, resource := range c.Spec.Resources {
					if declared.Name == resource.Name && !resource.Optional {
						return nil, fmt.Errorf("resources %s missing for condition %s in pipeline task %s", declared.Resource, cName, pt.Name)
					}
				}
			}
		}

		rcc := ResolvedConditionCheck{
			ConditionRegisterName: crName,
			Condition:             c,
			ConditionCheckName:    conditionCheckName,
			ConditionCheck:        v1beta1.NewConditionCheck(cctr),
			PipelineTaskCondition: &ptc,
			ResolvedResources:     conditionResources,
		}

		rccs = append(rccs, &rcc)
	}
	return rccs, nil
}

// ResolvePipelineTaskResources matches PipelineResources referenced by pt inputs and outputs with the
// providedResources and returns an instance of ResolvedTaskResources.
func ResolvePipelineTaskResources(pt v1beta1.PipelineTask, ts *v1beta1.TaskSpec, taskName string, kind v1beta1.TaskKind, providedResources map[string]*resourcev1alpha1.PipelineResource) (*resources.ResolvedTaskResources, error) {
	rtr := resources.ResolvedTaskResources{
		TaskName: taskName,
		TaskSpec: ts,
		Kind:     kind,
		Inputs:   map[string]*resourcev1alpha1.PipelineResource{},
		Outputs:  map[string]*resourcev1alpha1.PipelineResource{},
	}
	if pt.Resources != nil {
		for _, taskInput := range pt.Resources.Inputs {
			if resource, ok := providedResources[taskInput.Resource]; ok {
				rtr.Inputs[taskInput.Name] = resource
			} else {
				if ts.Resources == nil || ts.Resources.Inputs == nil {
					return nil, fmt.Errorf("pipelineTask tried to use input resource %s not present in declared resources", taskInput.Resource)
				}
				for _, r := range ts.Resources.Inputs {
					if r.Name == taskInput.Name && !r.Optional {
						return nil, fmt.Errorf("pipelineTask tried to use input resource %s not present in declared resources", taskInput.Resource)
					}
				}
			}
		}
		for _, taskOutput := range pt.Resources.Outputs {
			if resource, ok := providedResources[taskOutput.Resource]; ok {
				rtr.Outputs[taskOutput.Name] = resource
			} else {
				if ts.Resources == nil || ts.Resources.Outputs == nil {
					return nil, fmt.Errorf("pipelineTask tried to use output resource %s not present in declared resources", taskOutput.Resource)
				}
				for _, r := range ts.Resources.Outputs {
					if r.Name == taskOutput.Name && !r.Optional {
						return nil, fmt.Errorf("pipelineTask tried to use output resource %s not present in declared resources", taskOutput.Resource)
					}
				}
			}
		}
	}
	return &rtr, nil
}

// SkippedPipelineTaskNames returns a list of tasks, including task such as
// its Condition Checks failed or because one of the parent tasks's conditions failed
// Note that this means a task would not be included if a conditionCheck is still in progress
func (state PipelineRunState) skippedPipelineTasks(pr *v1beta1.PipelineRun, d *dag.Graph) {
	for _, t := range state {
		// final tasks should not be skipped,
		// continue if its a final task without checking further
		if _, ok := pr.Status.RunState.FinalTasks[t.PipelineTask.Name]; ok {
			continue
		}
		// task is not skipped if it's associated TaskRun already exists
		if t.TaskRun != nil {
			continue
		}
		// Check if conditionChecks have failed, if so task is skipped
		if len(t.ResolvedConditionChecks) > 0 {
			// if conditionChecks are done but was not successful, the task was skipped
			if t.ResolvedConditionChecks.IsDone() && !t.ResolvedConditionChecks.IsSuccess() {
				// do not add if the task already exist in the list
				if s, ok := pr.Status.RunState.DAGTasks[t.PipelineTask.Name]; ok {
					if s == v1beta1.PipelineTaskNotStarted {
						pr.Status.RunState.DAGTasks[t.PipelineTask.Name] = v1beta1.PipelineTaskSkipped
						pr.Status.RunState.Skipped += 1
						pr.Status.RunState.NotStarted -= 1
					}
				}
			}
		}
		// look at parent tasks to see if they have been skipped,
		// if any of the parents have been skipped, skip this task as well
		node := d.Nodes[t.PipelineTask.Name]
		for _, p := range node.Prev {
			// if the parent task is in the list of skipped tasks, add this task
			if s, ok := pr.Status.RunState.DAGTasks[p.Task.HashKey()]; ok {
				if s == v1beta1.PipelineTaskSkipped {
					// do not add if the task already exist in the list
					if s, ok := pr.Status.RunState.DAGTasks[t.PipelineTask.Name]; ok {
						if s == v1beta1.PipelineTaskNotStarted {
							pr.Status.RunState.DAGTasks[t.PipelineTask.Name] = v1beta1.PipelineTaskSkipped
							pr.Status.RunState.Skipped += 1
							pr.Status.RunState.NotStarted -= 1
						}
					}
				}
			}
		}
	}
}

// inList checks if an element already exist in the list or not
func inList(tasks []string, task string) bool {
	for _, t := range tasks {
		if t == task {
			return true
		}
	}
	return false
}

//func (state PipelineRunState) updateDAGTaskState(pr *v1beta1.PipelineRun) {
//	for _, t := range state {
//		if s, ok := pr.Status.RunState.DAGTasks[t.PipelineTask.Name]; ok {
//			if s == v1beta1.PipelineTaskNotStarted {
//				if t.IsDone() {
//					if t.IsSuccessful() {
//						pr.Status.RunState.DAGTasks[t.PipelineTask.Name] = v1beta1.PipelineTaskSucceeded
//						pr.Status.RunState.Succeeded += 1
//						pr.Status.RunState.NotStarted -= 1
//					} else if t.IsFailure() {
//						pr.Status.RunState.DAGTasks[t.PipelineTask.Name] = v1beta1.PipelineTaskFailed
//						pr.Status.RunState.NotStarted -= 1
//					}
//				} else if t.IsCancelled() {
//					pr.Status.RunState.DAGTasks[t.PipelineTask.Name] = v1beta1.PipelineTaskCancelled
//					pr.Status.RunState.NotStarted -= 1
//				}
//			}
//		}
//	}
//}

//func markSuccess (tasks )

func (state PipelineRunState) UpdatePipelineTaskState(pr *v1beta1.PipelineRun, d *dag.Graph) {
	for _, t := range state {
		spew.Dump("Looking at the task right now")
		spew.Dump(t.PipelineTask.Name)
		if s, ok := pr.Status.RunState.DAGTasks[t.PipelineTask.Name]; ok {
			spew.Dump("task is DAG task")
			// TODO failed task state can change to succeed, fail, skipped, or cancelled
			if s == v1beta1.PipelineTaskNotStarted {
				if t.IsDone() {
					if t.IsSuccessful() {
						pr.Status.RunState.DAGTasks[t.PipelineTask.Name] = v1beta1.PipelineTaskSucceeded
						pr.Status.RunState.Succeeded += 1
						pr.Status.RunState.NotStarted -= 1
					} else if t.IsFailure() {
						pr.Status.RunState.DAGTasks[t.PipelineTask.Name] = v1beta1.PipelineTaskFailed
						pr.Status.RunState.Failed += 1
						pr.Status.RunState.NotStarted -= 1
					}
				} else if t.IsCancelled() {
					pr.Status.RunState.DAGTasks[t.PipelineTask.Name] = v1beta1.PipelineTaskCancelled
					pr.Status.RunState.Cancelled += 1
					pr.Status.RunState.NotStarted -= 1
				}
			}
		} else if s, ok := pr.Status.RunState.FinalTasks[t.PipelineTask.Name]; ok {
			spew.Dump("task is final task")
			if s == v1beta1.PipelineTaskNotStarted {
				if t.IsDone() {
					if t.IsSuccessful() {
						spew.Dump("task is successful")
						pr.Status.RunState.FinalTasks[t.PipelineTask.Name] = v1beta1.PipelineTaskSucceeded
						pr.Status.RunState.Succeeded += 1
						pr.Status.RunState.NotStarted -= 1
					} else if t.IsFailure() {
						spew.Dump("task is failed")
						pr.Status.RunState.FinalTasks[t.PipelineTask.Name] = v1beta1.PipelineTaskFailed
						pr.Status.RunState.Failed += 1
						pr.Status.RunState.NotStarted -= 1
					}
				} else if t.IsCancelled() {
					spew.Dump("task is cancelled")
					pr.Status.RunState.FinalTasks[t.PipelineTask.Name] = v1beta1.PipelineTaskCancelled
					pr.Status.RunState.Cancelled += 1
					pr.Status.RunState.NotStarted -= 1
				}
			}
		}
	}
	state.skippedPipelineTasks(pr, d)
}

func (state PipelineRunState) isDAGTasksFinished(runState *v1beta1.PipelineTaskRunState) bool {
	for _, t := range state {
		if _, ok := runState.FinalTasks[t.PipelineTask.Name]; !ok {
			if !(runState.DAGTasks[t.PipelineTask.Name] == v1beta1.PipelineTaskSucceeded ||
				runState.DAGTasks[t.PipelineTask.Name] == v1beta1.PipelineTaskSkipped) {
				return false
			}
		}
	}
	return true
}

func (state PipelineRunState) isFinalTasksFinished(runState *v1beta1.PipelineTaskRunState) bool {
	for _, t := range state {
		spew.Dump("Looking at final task")
		spew.Dump(t.PipelineTask.Name)
		if _, ok := runState.FinalTasks[t.PipelineTask.Name]; ok {
			spew.Dump("its a final task")
			if !(runState.FinalTasks[t.PipelineTask.Name] == v1beta1.PipelineTaskSucceeded ||
				runState.FinalTasks[t.PipelineTask.Name] == v1beta1.PipelineTaskSkipped ||
				runState.FinalTasks[t.PipelineTask.Name] == v1beta1.PipelineTaskFailed) {
				spew.Dump("its not in succeeded, skipped, or failed so return false")
				return false
			}
		}
	}
	return true
}
