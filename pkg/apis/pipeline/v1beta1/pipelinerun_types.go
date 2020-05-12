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

package v1beta1

import (
	"fmt"
	"time"

	"github.com/tektoncd/pipeline/pkg/apis/config"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

var (
	groupVersionKind = schema.GroupVersionKind{
		Group:   SchemeGroupVersion.Group,
		Version: SchemeGroupVersion.Version,
		Kind:    pipeline.PipelineRunControllerName,
	}
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PipelineRun represents a single execution of a Pipeline. PipelineRuns are how
// the graph of Tasks declared in a Pipeline are executed; they specify inputs
// to Pipelines such as parameter values and capture operational aspects of the
// Tasks execution such as service account and tolerations. Creating a
// PipelineRun creates TaskRuns for Tasks in the referenced Pipeline.
//
// +k8s:openapi-gen=true
type PipelineRun struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec PipelineRunSpec `json:"spec,omitempty"`
	// +optional
	Status PipelineRunStatus `json:"status,omitempty"`
}

func (pr *PipelineRun) GetName() string {
	return pr.ObjectMeta.GetName()
}

// GetTaskRunRef for pipelinerun
func (pr *PipelineRun) GetTaskRunRef() corev1.ObjectReference {
	return corev1.ObjectReference{
		APIVersion: SchemeGroupVersion.String(),
		Kind:       "TaskRun",
		Namespace:  pr.Namespace,
		Name:       pr.Name,
	}
}

// GetOwnerReference gets the pipeline run as owner reference for any related objects
func (pr *PipelineRun) GetOwnerReference() metav1.OwnerReference {
	return *metav1.NewControllerRef(pr, groupVersionKind)
}

// IsDone returns true if the PipelineRun's status indicates that it is done.
func (pr *PipelineRun) IsDone() bool {
	return !pr.Status.GetCondition(apis.ConditionSucceeded).IsUnknown()
}

// HasStarted function check whether pipelinerun has valid start time set in its status
func (pr *PipelineRun) HasStarted() bool {
	return pr.Status.StartTime != nil && !pr.Status.StartTime.IsZero()
}

// IsCancelled returns true if the PipelineRun's spec status is set to Cancelled state
func (pr *PipelineRun) IsCancelled() bool {
	return pr.Spec.Status == PipelineRunSpecStatusCancelled
}

// GetRunKey return the pipelinerun key for timeout handler map
func (pr *PipelineRun) GetRunKey() string {
	// The address of the pointer is a threadsafe unique identifier for the pipelinerun
	return fmt.Sprintf("%s/%p", pipeline.PipelineRunControllerName, pr)
}

// IsTimedOut returns true if a pipelinerun has exceeded its spec.Timeout based on its status.Timeout
func (pr *PipelineRun) IsTimedOut() bool {
	pipelineTimeout := pr.Spec.Timeout
	startTime := pr.Status.StartTime

	if !startTime.IsZero() && pipelineTimeout != nil {
		timeout := pipelineTimeout.Duration
		if timeout == config.NoTimeoutDuration {
			return false
		}
		runtime := time.Since(startTime.Time)
		if runtime > timeout {
			return true
		}
	}
	return false
}

// GetServiceAccountName returns the service account name for a given
// PipelineTask if configured, otherwise it returns the PipelineRun's serviceAccountName.
func (pr *PipelineRun) GetServiceAccountName(pipelineTaskName string) string {
	serviceAccountName := pr.Spec.ServiceAccountName
	for _, sa := range pr.Spec.ServiceAccountNames {
		if sa.TaskName == pipelineTaskName {
			serviceAccountName = sa.ServiceAccountName
		}
	}
	return serviceAccountName
}

// HasVolumeClaimTemplate returns true if PipelineRun contains volumeClaimTemplates that is
// used for creating PersistentVolumeClaims with an OwnerReference for each run
func (pr *PipelineRun) HasVolumeClaimTemplate() bool {
	for _, ws := range pr.Spec.Workspaces {
		if ws.VolumeClaimTemplate != nil {
			return true
		}
	}
	return false
}

// PipelineRunSpec defines the desired state of PipelineRun
type PipelineRunSpec struct {
	// +optional
	PipelineRef *PipelineRef `json:"pipelineRef,omitempty"`
	// +optional
	PipelineSpec *PipelineSpec `json:"pipelineSpec,omitempty"`
	// Resources is a list of bindings specifying which actual instances of
	// PipelineResources to use for the resources the Pipeline has declared
	// it needs.
	Resources []PipelineResourceBinding `json:"resources,omitempty"`
	// Params is a list of parameter names and values.
	Params []Param `json:"params,omitempty"`
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// +optional
	ServiceAccountNames []PipelineRunSpecServiceAccountName `json:"serviceAccountNames,omitempty"`
	// Used for cancelling a pipelinerun (and maybe more later on)
	// +optional
	Status PipelineRunSpecStatus `json:"status,omitempty"`
	// Time after which the Pipeline times out. Defaults to never.
	// Refer to Go's ParseDuration documentation for expected format: https://golang.org/pkg/time/#ParseDuration
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
	// PodTemplate holds pod specific configuration
	PodTemplate *PodTemplate `json:"podTemplate,omitempty"`
	// Workspaces holds a set of workspace bindings that must match names
	// with those declared in the pipeline.
	// +optional
	Workspaces []WorkspaceBinding `json:"workspaces,omitempty"`
	// TaskRunSpecs holds a set of runtime specs
	// +optional
	TaskRunSpecs []PipelineTaskRunSpec `json:"taskRunSpecs,omitempty"`
}

// PipelineRunSpecStatus defines the pipelinerun spec status the user can provide
type PipelineRunSpecStatus string

const (
	// PipelineRunSpecStatusCancelled indicates that the user wants to cancel the task,
	// if not already cancelled or terminated
	PipelineRunSpecStatusCancelled = "PipelineRunCancelled"
)

// PipelineRef can be used to refer to a specific instance of a Pipeline.
// Copied from CrossVersionObjectReference: https://github.com/kubernetes/kubernetes/blob/169df7434155cbbc22f1532cba8e0a9588e29ad8/pkg/apis/autoscaling/types.go#L64
type PipelineRef struct {
	// Name of the referent; More info: http://kubernetes.io/docs/user-guide/identifiers#names
	Name string `json:"name,omitempty"`
	// API version of the referent
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
}

// PipelineRunStatus defines the observed state of PipelineRun
type PipelineRunStatus struct {
	duckv1beta1.Status `json:",inline"`

	// PipelineRunStatusFields inlines the status fields.
	PipelineRunStatusFields `json:",inline"`
}

var pipelineRunCondSet = apis.NewBatchConditionSet()

// GetCondition returns the Condition matching the given type.
func (pr *PipelineRunStatus) GetCondition(t apis.ConditionType) *apis.Condition {
	return pipelineRunCondSet.Manage(pr).GetCondition(t)
}

// InitializeConditions will set all conditions in pipelineRunCondSet to unknown for the PipelineRun
// and set the started time to the current time
func (pr *PipelineRunStatus) InitializeConditions() {
	if pr.TaskRuns == nil {
		pr.TaskRuns = make(map[string]*PipelineRunTaskRunStatus)
	}
	if pr.StartTime.IsZero() {
		pr.StartTime = &metav1.Time{Time: time.Now()}
	}
	pipelineRunCondSet.Manage(pr).InitializeConditions()
}

// SetCondition sets the condition, unsetting previous conditions with the same
// type as necessary.
func (pr *PipelineRunStatus) SetCondition(newCond *apis.Condition) {
	if newCond != nil {
		pipelineRunCondSet.Manage(pr).SetCondition(*newCond)
	}
}

// MarkResourceNotConvertible adds a Warning-severity condition to the resource noting
// that it cannot be converted to a higher version.
func (pr *PipelineRunStatus) MarkResourceNotConvertible(err *CannotConvertError) {
	pipelineRunCondSet.Manage(pr).SetCondition(apis.Condition{
		Type:     ConditionTypeConvertible,
		Status:   corev1.ConditionFalse,
		Severity: apis.ConditionSeverityWarning,
		Reason:   err.Field,
		Message:  err.Message,
	})
}

// PipelineRunStatusFields holds the fields of PipelineRunStatus' status.
// This is defined separately and inlined so that other types can readily
// consume these fields via duck typing.
type PipelineRunStatusFields struct {
	// StartTime is the time the PipelineRun is actually started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is the time the PipelineRun completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// map of PipelineRunTaskRunStatus with the taskRun name as the key
	// +optional
	TaskRuns map[string]*PipelineRunTaskRunStatus `json:"taskRuns,omitempty"`

	// PipelineResults are the list of results written out by the pipeline task's containers
	// +optional
	PipelineResults []PipelineRunResult `json:"pipelineResults,omitempty"`

	// PipelineRunSpec contains the exact spec used to instantiate the run
	PipelineSpec *PipelineSpec `json:"pipelineSpec,omitempty"`

	// RunState contains a list of pipeline tasks and their execution state
	// +optional
	RunState *PipelineTaskRunState `json:"runState,omitempty"`
}

// PipelineRunResult used to describe the results of a pipeline
type PipelineRunResult struct {
	// Name is the result's name as declared by the Pipeline
	Name string `json:"name"`

	// Value is the result returned from the execution of this PipelineRun
	Value string `json:"value"`
}

// PipelineRunTaskRunStatus contains the name of the PipelineTask for this TaskRun and the TaskRun's Status
type PipelineRunTaskRunStatus struct {
	// PipelineTaskName is the name of the PipelineTask.
	PipelineTaskName string `json:"pipelineTaskName,omitempty"`
	// Status is the TaskRunStatus for the corresponding TaskRun
	// +optional
	Status *TaskRunStatus `json:"status,omitempty"`
	// ConditionChecks maps the name of a condition check to its Status
	// +optional
	ConditionChecks map[string]*PipelineRunConditionCheckStatus `json:"conditionChecks,omitempty"`
}

// PipelineRunConditionCheckStatus returns the condition check status
type PipelineRunConditionCheckStatus struct {
	// ConditionName is the name of the Condition
	ConditionName string `json:"conditionName,omitempty"`
	// Status is the ConditionCheckStatus for the corresponding ConditionCheck
	// +optional
	Status *ConditionCheckStatus `json:"status,omitempty"`
}

// PipelineRunSpecServiceAccountName can be used to configure specific
// ServiceAccountName for a concrete Task
type PipelineRunSpecServiceAccountName struct {
	TaskName           string `json:"taskName,omitempty"`
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PipelineRunList contains a list of PipelineRun
type PipelineRunList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PipelineRun `json:"items,omitempty"`
}

// PipelineTaskRun reports the results of running a step in the Task. Each
// task has the potential to succeed or fail (based on the exit code)
// and produces logs.
type PipelineTaskRun struct {
	Name string `json:"name,omitempty"`
}

// PipelineTaskRunSpec  can be used to configure specific
// specs for a concrete Task
type PipelineTaskRunSpec struct {
	PipelineTaskName       string       `json:"pipelineTaskName,omitempty"`
	TaskServiceAccountName string       `json:"taskServiceAccountName,omitempty"`
	TaskPodTemplate        *PodTemplate `json:"taskPodTemplate,omitempty"`
}

// GetTaskRunSpecs returns the task specific spec for a given
// PipelineTask if configured, otherwise it returns the PipelineRun's default.
func (pr *PipelineRun) GetTaskRunSpecs(pipelineTaskName string) (string, *PodTemplate) {
	serviceAccountName := pr.GetServiceAccountName(pipelineTaskName)
	taskPodTemplate := pr.Spec.PodTemplate
	for _, task := range pr.Spec.TaskRunSpecs {
		if task.PipelineTaskName == pipelineTaskName {
			taskPodTemplate = task.TaskPodTemplate
			serviceAccountName = task.TaskServiceAccountName
		}
	}
	return serviceAccountName, taskPodTemplate
}

const (
	// PipelineTaskNotStarted indicates that the PipelineTask haven't started executing yet
	PipelineTaskNotStarted = "NotStarted"

	// PipelineTaskFailed indicates that the pipeline task has failed
	PipelineTaskFailed = "Failed"

	// PipelineTaskCancelled indicates that the pipeline task has cancelled
	PipelineTaskCancelled = "Cancelled"

	// PipelineTaskSucceeded indicates that the pipeline task has finished executing successfully
	PipelineTaskSucceeded = "Succeeded"

	// PipelineTaskSkipped indicates that the pipeline task has been skipped for one of the reasons:
	// (1) skipped because one or more Conditions failed
	// (2) skipped because parent was skipped
	PipelineTaskSkipped = "Skipped"
)

type PipelineTaskRunState struct {
	// map of PipelineTasks and their execution state, one of:
	// NotStarted, Succeeded, Failed, Cancelled, Skipped
	DAGTasks map[string]string `json:"DAGTasks,omitempty"`
}

// InitializePipelineTaskState will set RunState to zero value
// this initialization is done in the controller for the Pipeline Run
func (pr *PipelineRunStatus) InitializePipelineTaskState() {
	if pr.RunState == nil {
		pr.RunState = &PipelineTaskRunState{
			DAGTasks: make(map[string]string),
		}
	}
}

// SetPipelineTasksToNotStarted sets the RunState with DAGTasks and count of NotStarted tasks
func (pts *PipelineTaskRunState) SetPipelineTasksToNotStarted(pipelineSpec *PipelineSpec) {
	// set all DAG PipelineTasks to Not Started if it doesnt already exist in DAGTasks
	// along with incrementing the not started counter by 1
	for _, t := range pipelineSpec.Tasks {
		if _, ok := pts.DAGTasks[t.Name]; !ok {
			pts.DAGTasks[t.Name] = PipelineTaskNotStarted
		}
	}
}

// GetSucceededTasks returns a list of pipeline tasks, i.e. entries in DAGTasks with value PipelineTaskSucceeded
func (pts *PipelineTaskRunState) GetSucceededTasks() []string {
	tasks := []string{}
	if pts == nil {
		return tasks
	}
	for t, s := range pts.DAGTasks {
		if s == PipelineTaskSucceeded {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

func (pts *PipelineTaskRunState) AddSkippedTask(taskName string) {
	if s, ok := pts.DAGTasks[taskName]; ok {
		if s == PipelineTaskNotStarted {
			pts.DAGTasks[taskName] = PipelineTaskSkipped
		}
	}
}

func (pts *PipelineTaskRunState) AddSucceededTask(taskName string) {
	if s, ok := pts.DAGTasks[taskName]; ok {
		if s == PipelineTaskNotStarted {
			pts.DAGTasks[taskName] = PipelineTaskSucceeded
		}
	}
}

func (pts *PipelineTaskRunState) AddFailedTask(taskName string) {
	if s, ok := pts.DAGTasks[taskName]; ok {
		if s == PipelineTaskNotStarted || s == PipelineTaskFailed {
			pts.DAGTasks[taskName] = PipelineTaskFailed
		}
	}
}

func (pts *PipelineTaskRunState) AddCancelledTask(taskName string) {
	if s, ok := pts.DAGTasks[taskName]; ok {
		if s == PipelineTaskNotStarted {
			pts.DAGTasks[taskName] = PipelineTaskCancelled
		}
	}
}

func (pts *PipelineTaskRunState) CancelledTasksCount() int {
	cancelled := 0
	for _, s := range pts.DAGTasks {
		if s == PipelineTaskCancelled {
			cancelled += 1
		}
	}
	return cancelled
}

func (pts *PipelineTaskRunState) FailedTasksCount() int {
	failed := 0
	for _, s := range pts.DAGTasks {
		if s == PipelineTaskFailed {
			failed += 1
		}
	}
	return failed
}

func (pts *PipelineTaskRunState) SkippedTasksCount() int {
	skipped := 0
	for _, s := range pts.DAGTasks {
		if s == PipelineTaskSkipped {
			skipped += 1
		}
	}
	return skipped
}

func (pts *PipelineTaskRunState) SucceededTasksCount() int {
	succeeded := 0
	for _, s := range pts.DAGTasks {
		if s == PipelineTaskSucceeded {
			succeeded += 1
		}
	}
	return succeeded
}
