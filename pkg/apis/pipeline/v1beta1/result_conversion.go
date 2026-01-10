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

package v1beta1

import (
	"context"

	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

func (r TaskResult) convertTo(ctx context.Context, sink *v1.TaskResult) {
	sink.Name = r.Name
	sink.Type = v1.ResultsType(r.Type)
	sink.Description = r.Description
	if r.Properties != nil {
		properties := make(map[string]v1.PropertySpec)
		for k, v := range r.Properties {
			properties[k] = v1.PropertySpec{Type: v1.ParamType(v.Type)}
		}
		sink.Properties = properties
	}
	if r.Value != nil {
		sink.Value = &v1.ParamValue{}
		r.Value.convertTo(ctx, sink.Value)
	if r.Default != nil {
		sink.Default = &v1.ParamValue{
			Type: v1.ParamType(r.Default.Type),
		}
		switch r.Default.Type {
		case ParamTypeString:
			sink.Default.StringVal = r.Default.StringVal
		case ParamTypeArray:
			sink.Default.ArrayVal = r.Default.ArrayVal
		default: // In case of ParamTypeObject
			defaults := make(map[string]string)
			for k, v := range r.Default.ObjectVal {
				defaults[k] = v
			}
			sink.Default.ObjectVal = defaults
		}
	}
	}
}

func (r *TaskResult) convertFrom(ctx context.Context, source v1.TaskResult) {
	r.Name = source.Name
	r.Type = ResultsType(source.Type)
	r.Description = source.Description
	if source.Properties != nil {
		properties := make(map[string]PropertySpec)
		for k, v := range source.Properties {
			properties[k] = PropertySpec{Type: ParamType(v.Type)}
		}
		r.Properties = properties
	}
	if source.Value != nil {
		r.Value = &ParamValue{}
	if source.Default != nil {
		r.Default = &ParamValue{
			Type: ParamType(source.Default.Type),
		}
		switch source.Default.Type {
		case v1.ParamTypeString:
			r.Default.StringVal = source.Default.StringVal
		case v1.ParamTypeArray:
			r.Default.ArrayVal = source.Default.ArrayVal
		default:
			defaults := make(map[string]string)
			for k, v := range source.Default.ObjectVal {
				defaults[k] = v
			}
			r.Default.ObjectVal = defaults
		}
	}
		r.Value.convertFrom(ctx, *source.Value)
	}
}
