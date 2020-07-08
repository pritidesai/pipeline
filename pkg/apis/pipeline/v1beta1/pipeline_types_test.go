package v1beta1_test

import (
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

func TestPipeline_Copy(t *testing.T) {
	p := v1beta1.Pipeline{
		ObjectMeta: v1.ObjectMeta{Name: "sample-pipeline", Namespace: "foo"},
		Spec: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{
				{Name: "dag-task-1", TaskRef: &v1beta1.TaskRef{Name: "sample-task"}},
			},
			Finally: []v1beta1.PipelineTask{
				{Name: "final-task-1", TaskRef: &v1beta1.TaskRef{Name: "sample-task"}},
			},
		},
	}

	r := p.Copy()

	t.Log("Tasks:", r.PipelineSpec().Tasks)
	t.Log("Length of Tasks: ", len(r.PipelineSpec().Tasks))
	t.Log("Capacity of Tasks: ", cap(r.PipelineSpec().Tasks))

	t.Log("Finally: ", r.PipelineSpec().Finally)
	t.Log("Length of Finally: ", len(r.PipelineSpec().Finally))
	t.Log("Capacity of Finally: ", cap(r.PipelineSpec().Finally))

	a := append(r.PipelineSpec().Tasks, r.PipelineSpec().Finally...)
	t.Log()
	t.Log("Append Tasks and Finally: ", a)
	t.Logf("Address: %p", a)
	t.Log("Length: ", len(a))
	t.Log("Capacity: ", cap(a))
}

func TestPipeline_Copy_Muliple_Tasks(t *testing.T) {
	p := v1beta1.Pipeline{
		ObjectMeta: v1.ObjectMeta{Name: "sample-pipeline", Namespace: "foo"},
		Spec: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{
				{Name: "dag-task-1", TaskRef: &v1beta1.TaskRef{Name: "sample-task"}},
				{Name: "dag-task-2", TaskRef: &v1beta1.TaskRef{Name: "sample-task"}},
				{Name: "dag-task-3", TaskRef: &v1beta1.TaskRef{Name: "sample-task"}},
				{Name: "dag-task-4", TaskRef: &v1beta1.TaskRef{Name: "sample-task"}},
				{Name: "dag-task-5", TaskRef: &v1beta1.TaskRef{Name: "sample-task"}},
			},
			Finally: []v1beta1.PipelineTask{
				{Name: "final-task-1", TaskRef: &v1beta1.TaskRef{Name: "sample-task"}},
			},
		},
	}

	r := p.Copy()

	t.Log("Tasks:", r.PipelineSpec().Tasks)
	t.Log("Length of Tasks: ", len(r.PipelineSpec().Tasks))
	t.Log("Capacity of Tasks: ", cap(r.PipelineSpec().Tasks))

	t.Log("Finally: ", r.PipelineSpec().Finally)
	t.Log("Length of Finally: ", len(r.PipelineSpec().Finally))
	t.Log("Capacity of Finally: ", cap(r.PipelineSpec().Finally))

	a := append(r.PipelineSpec().Tasks, r.PipelineSpec().Finally...)
	t.Log()
	t.Log("Append Tasks and Finally: ", a)
	t.Logf("Address: %p", a)
	t.Log("Length: ", len(a))
	t.Log("Capacity: ", cap(a))
}
