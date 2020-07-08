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
	"errors"
	"testing"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetPipelineSpec_Ref(t *testing.T) {
	pipeline := &v1beta1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name: "orchestrate",
		},
		Spec: v1beta1.PipelineSpec{
			Tasks: []v1beta1.PipelineTask{
				{Name: "mytask1", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask2", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask3", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask4", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask5", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask6", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask7", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask8", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask9", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask10", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask11", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask12", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask13", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask14", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask15", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask16", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask17", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask18", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
			},
			Finally: []v1beta1.PipelineTask{
				{Name: "mytask1", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask2", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask3", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask4", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask5", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				{Name: "mytask6", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask7", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask8", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask9", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask10", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask11", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask12", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask13", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask14", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask15", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask16", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask17", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
				//{Name: "mytask18", TaskRef: &v1beta1.TaskRef{Name: "mytask"}},
			},
		},
	}
	pr := &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mypipelinerun",
		},
		Spec: v1beta1.PipelineRunSpec{
			PipelineRef: &v1beta1.PipelineRef{
				Name: "orchestrate",
			},
		},
	}
	gt := func(n string) (v1beta1.PipelineInterface, error) { return pipeline, nil }
	pipelineMeta, pipelineSpec, err := GetPipelineData(context.Background(), pr, gt)

	if err != nil {
		t.Fatalf("Did not expect error getting pipeline spec but got: %s", err)
	}

	if pipelineMeta.Name != "orchestrate" {
		t.Errorf("Expected pipeline name to be `orchestrate` but was %q", pipelineMeta.Name)
	}

	t.Log("pipelineSpec.Tasks Length before append")
	t.Log(len(pipelineSpec.Tasks))
	t.Log("pipelineSpec.Tasks Capacity before append")
	t.Log(cap(pipelineSpec.Tasks))
	t.Log("pipelineSpec.Tasks Address before append")
	t.Logf("%p", pipelineSpec.Tasks)

	t.Log("pipelineSpec.Finally Length before append")
	t.Log(len(pipelineSpec.Finally))
	t.Log("pipelineSpec.Finally Capacity before append")
	t.Log(cap(pipelineSpec.Finally))
	t.Log("pipelineSpec.Finally Address before append")
	t.Logf("%p", pipelineSpec.Finally)

	tasks1 := append(pipelineSpec.Tasks, pipelineSpec.Finally...)

	t.Log("Tasks1 Length")
	t.Log(len(tasks1))
	t.Log("Tasks1 Capacity")
	t.Log(cap(tasks1))
	t.Log("Tasks1 Address")
	t.Logf("%p", tasks1)

	//if len(pipelineSpec.Tasks) != 8 || pipelineSpec.Tasks[0].Name != "mytask1" {
	//	t.Errorf("Pipeline Spec not resolved as expected, expected referenced Pipeline spec but got: %v", pipelineSpec)
	//}
}

func TestGetPipelineSpec_Embedded(t *testing.T) {
	pr := &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mypipelinerun",
		},
		Spec: v1beta1.PipelineRunSpec{
			PipelineSpec: &v1beta1.PipelineSpec{
				Tasks: []v1beta1.PipelineTask{{
					Name: "mytask",
					TaskRef: &v1beta1.TaskRef{
						Name: "mytask",
					},
				}},
			},
		},
	}
	gt := func(n string) (v1beta1.PipelineInterface, error) { return nil, errors.New("shouldn't be called") }
	pipelineMeta, pipelineSpec, err := GetPipelineData(context.Background(), pr, gt)

	if err != nil {
		t.Fatalf("Did not expect error getting pipeline spec but got: %s", err)
	}

	if pipelineMeta.Name != "mypipelinerun" {
		t.Errorf("Expected pipeline name for embedded pipeline to default to name of pipeline run but was %q", pipelineMeta.Name)
	}

	if len(pipelineSpec.Tasks) != 1 || pipelineSpec.Tasks[0].Name != "mytask" {
		t.Errorf("Pipeline Spec not resolved as expected, expected embedded Pipeline spec but got: %v", pipelineSpec)
	}
}

func TestGetPipelineSpec_Invalid(t *testing.T) {
	tr := &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mypipelinerun",
		},
	}
	gt := func(n string) (v1beta1.PipelineInterface, error) { return nil, errors.New("shouldn't be called") }
	_, _, err := GetPipelineData(context.Background(), tr, gt)
	if err == nil {
		t.Fatalf("Expected error resolving spec with no embedded or referenced pipeline spec but didn't get error")
	}
}

func TestGetPipelineSpec_Error(t *testing.T) {
	tr := &v1beta1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mypipelinerun",
		},
		Spec: v1beta1.PipelineRunSpec{
			PipelineRef: &v1beta1.PipelineRef{
				Name: "orchestrate",
			},
		},
	}
	gt := func(n string) (v1beta1.PipelineInterface, error) { return nil, errors.New("something went wrong") }
	_, _, err := GetPipelineData(context.Background(), tr, gt)
	if err == nil {
		t.Fatalf("Expected error when unable to find referenced Pipeline but got none")
	}
}
