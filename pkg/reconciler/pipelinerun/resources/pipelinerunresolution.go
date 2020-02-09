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
	"fmt"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"knative.dev/pkg/apis"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
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

	// ReasonTimedOut indicates that the PipelineRun has taken longer than its configured
	// timeout
	ReasonTimedOut = "PipelineRunTimeout"

	// ReasonConditionCheckFailed indicates that the reason for the failure status is that the
	// condition check associated to the pipeline task evaluated to false
	ReasonConditionCheckFailed = "ConditionCheckFailed"

	ReasonRunOnStateConflicted = "RunOnStateConflicted"
)

// ResolvedPipelineRunTask contains a Task and its associated TaskRun, if it
// exists. TaskRun can be nil to represent there being no TaskRun.
type ResolvedPipelineRunTask struct {
	TaskRunName           string
	TaskRun               *v1alpha1.TaskRun
	PipelineTask          *v1alpha1.PipelineTask
	ResolvedTaskResources *resources.ResolvedTaskResources
	// ConditionChecks ~~TaskRuns but for evaling conditions
	ResolvedConditionChecks TaskConditionCheckState // Could also be a TaskRun or maybe just a Pod?
	// IsRunOnFailure is driven by the runOn section of the dependent tasks, runOn:  state: ["failure"]
	// it is set to true if any of the tasks within the pipeline depends on the current task's failure state
	// by default, IsRunOnFailure is set to false
	IsRunOnFailure bool
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

	// the TaskRun has failed but the Pipeline should continue running
	// as at least one of the Tasks in Pipeline rely on failure of this Task
	if t.IsRunOnFailure && c.IsFalse() {
		return false
	}

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

	return c.IsFalse() && c.Reason == v1alpha1.TaskRunSpecStatusCancelled
}

func (state PipelineRunState) toMap() map[string]*ResolvedPipelineRunTask {
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

// GetNextTasks will return the next ResolvedPipelineRunTasks to execute, which are the ones in the
// list of candidateTasks which aren't yet indicated in state to be running.
func (state PipelineRunState) GetNextTasks(candidateTasks map[string]struct{}) []*ResolvedPipelineRunTask {
	// initialize a list of tasks to return tasks next in execution queue
	tasks := []*ResolvedPipelineRunTask{}
	// iterate over PipelineRunState to get TaskRun and PipelineTask for each candidate tasks
	for _, t := range state {
		// for each candidate task, check if its TaskRun is initialized or not
		if _, ok := candidateTasks[t.PipelineTask.Name]; ok {
			// a candidate task with empty/nil TaskRun says that the task has not started executing yet
			if t.TaskRun == nil {
				// this task has not initialized at all or never attempted execution yet
				// but before adding it to the queue, check whether is it eligible for execution
				// check if it has any dependency specified using runAfter
				if len(t.PipelineTask.RunAfter) == 0 {
					// this task has no dependency on any other task in the Pipeline
					// so append it to the queue for execution
					tasks = append(tasks, t)
				} else {
					// check if any of the tasks under runAfter is marked as conflicted
					if state.isRunAfterConflicted(t.PipelineTask.RunAfter) {
						// one of the tasks specified in runAfter was marked as Conflicted therefore
						// mark this task as conflicted using "ReasonRunOnStateConflicted" as well
						//t.TaskStatus.SetCondition(&apis.Condition{
						//	Type:    apis.ConditionSucceeded,
						//	Status:  corev1.ConditionFalse,
						//	Reason:  ReasonRunOnStateConflicted,
						//	Message: fmt.Sprintf("RunOn states conflicted for Task %s in PipelineRun %s", t.TaskRunName, t.PipelineTask.Name),
						//})
						t.TaskRun = &v1alpha1.TaskRun{
							TypeMeta:   v1.TypeMeta{},
							ObjectMeta: v1.ObjectMeta{},
							Spec:       v1alpha1.TaskRunSpec{},
							Status:     v1alpha1.TaskRunStatus{},
						}
						t.TaskRun.Status.SetCondition(&apis.Condition{
							Type:    apis.ConditionSucceeded,
							Status:  corev1.ConditionFalse,
							Reason:  ReasonRunOnStateConflicted,
							Message: fmt.Sprintf("RunOn states conflicted for Task %s in PipelineRun %s", t.TaskRunName, t.PipelineTask.Name),
						})
					} else {
						// after discovering this task has a dependency on some task in the Pipeline
						// check if there is any condition specified using runOn
						if len(t.PipelineTask.RunOn) == 0 {
							// when runOn is not specified, by default, enforce dependent tasks state to be succeeded
							// add it to the queue if all parents are successful otherwise no task should be added to the queue
							if state.isAllParentsSuccessful(t.PipelineTask.RunAfter) {
								tasks = append(tasks, t)
							}
						} else {
							// here, a task has runAfter and runOn both specified
							// verify for all tasks in runOn, given states match with parent tasks
							// for task3 having two tasks in runOn:
							// task: task1 with states: ["success", "failure"]
							// task: task2 with states: ["skip"]
							// verify that task1 is either successful OR failed AND task2 is skipped
							// TODO: validate runOn has tasks which are also present in runAfter
							if state.validateRunOn(t.PipelineTask.RunOn) {
								tasks = append(tasks, t)
							} else {
								// states specified in runOn doesn't match with the state of the parent TaskRun/s
								// this task will be marked as conflicted using "ReasonRunOnStateConflicted"
								//t.TaskStatus.SetCondition(&apis.Condition{
								//	Type:    apis.ConditionSucceeded,
								//	Status:  corev1.ConditionFalse,
								//	Reason:  ReasonRunOnStateConflicted,
								//	Message: fmt.Sprintf("RunOn states conflicted for Task %s in PipelineRun %s", t.TaskRunName, t.PipelineTask.Name),
								//})
								t.TaskRun = &v1alpha1.TaskRun{
									TypeMeta:   v1.TypeMeta{},
									ObjectMeta: v1.ObjectMeta{},
									Spec:       v1alpha1.TaskRunSpec{},
									Status:     v1alpha1.TaskRunStatus{},
								}
								t.TaskRun.Status.SetCondition(&apis.Condition{
									Type:    apis.ConditionSucceeded,
									Status:  corev1.ConditionFalse,
									Reason:  ReasonRunOnStateConflicted,
									Message: fmt.Sprintf("RunOn states conflicted for Task %s in PipelineRun %s", t.TaskRunName, t.PipelineTask.Name),
								})
							}
						}
					}
				}
			} else {
				// TaskRun is initialized which means this task has been on the queue of execution
				status := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
				// task status is set to False after a task is declared as a failure (after exhausting number of retries)
				// once a task has failed, rest of the tasks in the pipeline are not being pushed
				// into the queue by default to maintain backward compatibility
				if status != nil && status.IsFalse() {
					// make sure status of TaskRun is not set to Cancelled  OR Reason is set to neither TaskRunCancelled nor ConditionCheckFailed
					if !(t.TaskRun.IsCancelled() || status.Reason == v1alpha1.TaskRunSpecStatusCancelled || status.Reason == ReasonConditionCheckFailed || status.Reason == ReasonRunOnStateConflicted) {
						// task has been declared failure but not exhausted number of retries so add it to the queue
						if len(t.TaskRun.Status.RetriesStatus) < t.PipelineTask.Retries {
							tasks = append(tasks, t)
						}
					}
				}
			}
		}
	}
	return tasks
}

func (state PipelineRunState) verifyTaskStateWithParent(taskName string, taskState v1alpha2.PipelineTaskState) bool {
	for _, t := range state {
		if taskName == t.PipelineTask.Name {
			c := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
			if (taskState == v1alpha2.PipelineTaskStateSuccess && c.IsTrue()) ||
				(taskState == v1alpha2.PipelineTaskStateFailure && c.IsFalse()) ||
				(taskState == v1alpha2.PipelineTaskStateSkip && c.Reason == ReasonConditionCheckFailed) ||
				taskState == v1alpha2.PipelineTaskStateAny {
				return true
			}
		}
	}
	return false
}

func (state PipelineRunState) isAllParentsSuccessful(taskNames []string) bool {
	allParentsSuccessful := true
	for _, taskName := range taskNames {
		if !state.verifyTaskStateWithParent(taskName, v1alpha2.PipelineTaskStateSuccess) {
			allParentsSuccessful = false
			break
		}
	}
	return allParentsSuccessful
}

// validate a task at least has one of the specified states
// e.g., task1 -> ["success", "failure"],
// return true if task1 is either successful or failed
// return false if task1 is neither successful nor failed, it could be skipped
func (state PipelineRunState) isParentInRunOnValid(taskName string, taskStates []v1alpha2.PipelineTaskState) bool {
	validStates := false
	for _, taskState := range taskStates {
		if state.verifyTaskStateWithParent(taskName, taskState) {
			validStates = true
			break
		}
	}
	return validStates
}

// validate all the tasks specified under runOn
// all tasks under runOn should match with at least one of the states specified
// e.g. task1 -> ["success", "failure"], task2 ->  ["success"]
// return true if (task1 is either successful OR failed) AND (task2 is successful)
func (state PipelineRunState) validateRunOn(runOn []v1alpha2.PipelineTaskRunOn) bool {
	validRunOnTasks := true
	for _, runOnTasks := range runOn {
		if !state.isParentInRunOnValid(runOnTasks.Task, runOnTasks.States) {
			validRunOnTasks = false
			break
		}
	}
	return validRunOnTasks
}

func (state PipelineRunState) isRunAfterConflicted(runAfter []string) bool {
	for _, t := range state {
		for _, task := range runAfter {
			if t.PipelineTask.Name == task {
				c := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
				if c.IsFalse() && c.Reason == ReasonRunOnStateConflicted {
					return true
				}
			}
		}
	}
	return false
}

func isStateInRunOn(states []v1alpha2.PipelineTaskState, stage v1alpha2.PipelineTaskState) bool {
	for _, s := range states {
		if stage == s {
			return true
		}
	}
	return false
}

func isTaskInRunAfter(runAfter []string, task string) bool {
	for _, runAfterTask := range runAfter {
		if runAfterTask == task {
			return true
		}
	}
	return false
}

// VisitedPipelineTaskNames returns a list of the names of all of the PipelineTasks in state
// which have either successfully completed or failed or skipped.
func (state PipelineRunState) VisitedPipelineTaskNames() []string {
	done := []string{}
	for _, t := range state {
		if t.TaskRun != nil {
			c := t.TaskRun.Status.GetCondition(apis.ConditionSucceeded)
			if c.IsTrue() || (c.IsFalse() && c.Reason == ReasonFailed) {
				done = append(done, t.PipelineTask.Name)
			}
		}
	}
	return done
}

// GetTaskRun is a function that will retrieve the TaskRun name.
type GetTaskRun func(name string) (*v1alpha1.TaskRun, error)

// GetResourcesFromBindings will retrieve all Resources bound in PipelineRun pr and return a map
// from the declared name of the PipelineResource (which is how the PipelineResource will
// be referred to in the PipelineRun) to the PipelineResource, obtained via getResource.
func GetResourcesFromBindings(pr *v1alpha1.PipelineRun, getResource resources.GetResource) (map[string]*v1alpha1.PipelineResource, error) {
	rs := map[string]*v1alpha1.PipelineResource{}
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
func ValidateResourceBindings(p *v1alpha1.PipelineSpec, pr *v1alpha1.PipelineRun) error {
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
func ValidateWorkspaceBindings(p *v1alpha1.PipelineSpec, pr *v1alpha1.PipelineRun) error {
	pipelineRunWorkspaces := make(map[string]v1alpha1.WorkspaceBinding)
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

// ResolvePipelineRun retrieves all Tasks instances which are reference by tasks, getting
// instances from getTask. If it is unable to retrieve an instance of a referenced Task, it
// will return an error, otherwise it returns a list of all of the Tasks retrieved.
// It will retrieve the Resources needed for the TaskRun using the mapping of providedResources.
func ResolvePipelineRun(
	pipelineRun v1alpha1.PipelineRun,
	getTask resources.GetTask,
	getTaskRun resources.GetTaskRun,
	getClusterTask resources.GetClusterTask,
	getCondition GetCondition,
	tasks []v1alpha1.PipelineTask,
	providedResources map[string]*v1alpha1.PipelineResource,
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
			t        v1alpha1.TaskInterface
			err      error
			spec     v1alpha1.TaskSpec
			taskName string
			kind     v1alpha1.TaskKind
		)

		if pt.TaskRef != nil {
			if pt.TaskRef.Kind == v1alpha1.ClusterTaskKind {
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

		// iterate over each task and find out if runOn is specified for that task
		// set each PipelineTask as non-blocking if its failure is acceptable
		for _, t := range tasks {
			for _, runOn := range t.RunOn {
				// may be delete this check from here if already covered in validation
				if runOn.Task == pt.Name {
					if isTaskInRunAfter(t.RunAfter, runOn.Task) {
						if isStateInRunOn(runOn.States, v1alpha2.PipelineTaskStateFailure) ||
							isStateInRunOn(runOn.States, v1alpha2.PipelineTaskStateAny) {
							rprt.IsRunOnFailure = true
						}
					}
				} else {
					// throw an error because task is in runOn but not listed in runAfter
				}
			}
		}
		// Add this task to the state of the PipelineRun
		state = append(state, &rprt)
	}
	return state, nil
}

// getConditionCheckName should return a unique name for a `ConditionCheck` if one has not already been defined, and the existing one otherwise.
func getConditionCheckName(taskRunStatus map[string]*v1alpha1.PipelineRunTaskRunStatus, trName, conditionName string) string {
	trStatus, ok := taskRunStatus[trName]
	if ok && trStatus.ConditionChecks != nil {
		for k, v := range trStatus.ConditionChecks {
			// TODO(1022): Should  we allow multiple conditions of the same type?
			if conditionName == v.ConditionName {
				return k
			}
		}
	}
	return names.SimpleNameGenerator.RestrictLengthWithRandomSuffix(fmt.Sprintf("%s-%s", trName, conditionName))
}

// getTaskRunName should return a unique name for a `TaskRun` if one has not already been defined, and the existing one otherwise.
func getTaskRunName(taskRunsStatus map[string]*v1alpha1.PipelineRunTaskRunStatus, ptName, prName string) string {
	for k, v := range taskRunsStatus {
		if v.PipelineTaskName == ptName {
			return k
		}
	}

	return names.SimpleNameGenerator.RestrictLengthWithRandomSuffix(fmt.Sprintf("%s-%s", prName, ptName))
}

// GetPipelineConditionStatus will return the Condition that the PipelineRun prName should be
// updated with, based on the status of the TaskRuns in state.
func GetPipelineConditionStatus(pr *v1alpha1.PipelineRun, state PipelineRunState, logger *zap.SugaredLogger, dag *dag.Graph) *apis.Condition {
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
			Message: fmt.Sprintf("PipelineRun %q failed to finish within %q", pr.Name, pr.Spec.Timeout.String()),
		}
	}

	// TODO remove this clause since we do not want to fail on single failed task
	// A single failed task mean we fail the pipeline
	for _, rprt := range state {
		if rprt.IsCancelled() {
			logger.Infof("TaskRun %s is cancelled, so PipelineRun %s is cancelled", rprt.TaskRunName, pr.Name)
			return &apis.Condition{
				Type:    apis.ConditionSucceeded,
				Status:  corev1.ConditionFalse,
				Reason:  ReasonCancelled,
				Message: fmt.Sprintf("TaskRun %s has cancelled", rprt.TaskRun.Name),
			}
		}

		if rprt.IsFailure() { //IsDone ensures we have crossed the retry limit
			logger.Infof("TaskRun %s has failed, so PipelineRun %s has failed, retries done: %b", rprt.TaskRunName, pr.Name, len(rprt.TaskRun.Status.RetriesStatus))
			return &apis.Condition{
				Type:    apis.ConditionSucceeded,
				Status:  corev1.ConditionFalse,
				Reason:  ReasonFailed,
				Message: fmt.Sprintf("TaskRun %s has failed", rprt.TaskRun.Name),
			}
		}
	}

	allTasks := []string{}
	successOrSkipTasks := []string{}

	// Check to see if all tasks are success or skipped or allowed failure with runOn
	for _, rprt := range state {
		allTasks = append(allTasks, rprt.PipelineTask.Name)
		if rprt.IsSuccessful() ||
			isSkipped(rprt, state.toMap(), dag) ||
			(rprt.IsRunOnFailure && rprt.TaskRun != nil && rprt.TaskRun.Status.Conditions != nil && rprt.TaskRun.Status.GetCondition(apis.ConditionSucceeded).IsFalse()) ||
			(rprt.TaskRun != nil && rprt.TaskRun.Status.Conditions != nil && rprt.TaskRun.Status.GetCondition(apis.ConditionSucceeded).Reason == ReasonRunOnStateConflicted) {
			successOrSkipTasks = append(successOrSkipTasks, rprt.PipelineTask.Name)
		}
	}

	if reflect.DeepEqual(allTasks, successOrSkipTasks) {
		logger.Infof("All TaskRuns have finished for PipelineRun %s so it has finished", pr.Name)
		return &apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionTrue,
			Reason:  ReasonSucceeded,
			Message: "All Tasks have completed executing",
		}
	}

	// Hasn't timed out; no taskrun failed yet; and not all tasks have finished....
	// Must keep running then....

	return &apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionUnknown,
		Reason:  ReasonRunning,
		Message: "Not all Tasks in the Pipeline have finished executing",
	}
}

// isSkipped returns true if a Task in a TaskRun will not be run either because
// its Condition Checks failed or because one of the parent tasks's conditions failed
// Note that this means isSkipped returns false if a conditionCheck is in progress
func isSkipped(rprt *ResolvedPipelineRunTask, stateMap map[string]*ResolvedPipelineRunTask, d *dag.Graph) bool {
	// Taskrun not skipped if it already exists
	if rprt.TaskRun != nil {
		return false
	}

	// Check if conditionChecks have failed, if so task is skipped
	if len(rprt.ResolvedConditionChecks) > 0 {
		// isSkipped is only true iof
		if rprt.ResolvedConditionChecks.IsDone() && !rprt.ResolvedConditionChecks.IsSuccess() {
			return true
		}
	}

	// Recursively look at parent tasks to see if they have been skipped,
	// if any of the parents have been skipped, skip as well
	node := d.Nodes[rprt.PipelineTask.Name]
	for _, p := range node.Prev {
		skip := isSkipped(stateMap[p.Task.HashKey()], stateMap, d)
		if skip {
			return true
		}
	}

	return false
}

func resolveConditionChecks(pt *v1alpha1.PipelineTask, taskRunStatus map[string]*v1alpha1.PipelineRunTaskRunStatus, taskRunName string, getTaskRun resources.GetTaskRun, getCondition GetCondition, providedResources map[string]*v1alpha1.PipelineResource) ([]*ResolvedConditionCheck, error) {
	rccs := []*ResolvedConditionCheck{}
	for i := range pt.Conditions {
		ptc := pt.Conditions[i]
		cName := ptc.ConditionRef
		c, err := getCondition(cName)
		if err != nil {
			return nil, &ConditionNotFoundError{
				Name: cName,
				Msg:  err.Error(),
			}
		}
		conditionCheckName := getConditionCheckName(taskRunStatus, taskRunName, cName)
		cctr, err := getTaskRun(conditionCheckName)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, fmt.Errorf("error retrieving ConditionCheck %s for taskRun name %s : %w", conditionCheckName, taskRunName, err)
			}
		}
		conditionResources := map[string]*v1alpha1.PipelineResource{}
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
			Condition:             c,
			ConditionCheckName:    conditionCheckName,
			ConditionCheck:        v1alpha1.NewConditionCheck(cctr),
			PipelineTaskCondition: &ptc,
			ResolvedResources:     conditionResources,
		}

		rccs = append(rccs, &rcc)
	}
	return rccs, nil
}

// ResolvePipelineTaskResources matches PipelineResources referenced by pt inputs and outputs with the
// providedResources and returns an instance of ResolvedTaskResources.
func ResolvePipelineTaskResources(pt v1alpha1.PipelineTask, ts *v1alpha1.TaskSpec, taskName string, kind v1alpha1.TaskKind, providedResources map[string]*v1alpha1.PipelineResource) (*resources.ResolvedTaskResources, error) {
	rtr := resources.ResolvedTaskResources{
		TaskName: taskName,
		TaskSpec: ts,
		Kind:     kind,
		Inputs:   map[string]*v1alpha1.PipelineResource{},
		Outputs:  map[string]*v1alpha1.PipelineResource{},
	}
	if pt.Resources != nil {
		for _, taskInput := range pt.Resources.Inputs {
			if resource, ok := providedResources[taskInput.Resource]; ok {
				rtr.Inputs[taskInput.Name] = resource
			} else {
				if ts.Inputs == nil {
					return nil, fmt.Errorf("pipelineTask tried to use input resource %s not present in declared resources", taskInput.Resource)
				}
				for _, r := range ts.Inputs.Resources {
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
				if ts.Outputs == nil {
					return nil, fmt.Errorf("pipelineTask tried to use output resource %s not present in declared resources", taskOutput.Resource)
				}
				for _, r := range ts.Outputs.Resources {
					if r.Name == taskOutput.Name && !r.Optional {
						return nil, fmt.Errorf("pipelineTask tried to use output resource %s not present in declared resources", taskOutput.Resource)
					}
				}
			}
		}
	}
	return &rtr, nil
}
