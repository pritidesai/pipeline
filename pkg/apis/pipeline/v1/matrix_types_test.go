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

package v1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tektoncd/pipeline/test/diff"
)

func TestMatrix_FanOut_ToParams(t *testing.T) {
	tests := []struct {
		name           string
		matrix         Matrix
		want           Combinations
		expectedParams []Params
	}{{
		name: "matrix with no params",
		matrix: Matrix{
			Params: Params{},
		},
		want:           nil,
		expectedParams: nil,
	}, {
		name: "single array in matrix",
		matrix: Matrix{
			Params: Params{{
				Name:  "platform",
				Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"linux", "mac", "windows"}},
			}},
		},
		want: Combinations{{
			"platform": "linux",
		}, {
			"platform": "mac",
		}, {
			"platform": "windows",
		}},
		expectedParams: []Params{{
			{
				Name:  "platform",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux"},
			},
		}, {
			{
				Name:  "platform",
				Value: ParamValue{Type: ParamTypeString, StringVal: "mac"},
			},
		}, {
			{
				Name:  "platform",
				Value: ParamValue{Type: ParamTypeString, StringVal: "windows"},
			},
		}},
	}, {
		name: "multiple arrays in matrix",
		matrix: Matrix{
			Params: Params{{
				Name: "GOARCH", Value: ParamValue{ArrayVal: []string{"linux/amd64", "linux/ppc64le", "linux/s390x"}},
			}, {
				Name: "version", Value: ParamValue{ArrayVal: []string{"go1.17", "go1.18.1"}}},
			},
			Include: []MatrixInclude{{}},
		},
		want: Combinations{{
			"GOARCH": "linux/amd64", "version": "go1.17",
		}, {
			"GOARCH": "linux/ppc64le", "version": "go1.17",
		}, {
			"GOARCH": "linux/s390x", "version": "go1.17",
		}, {
			"GOARCH": "linux/amd64", "version": "go1.18.1",
		}, {
			"GOARCH": "linux/ppc64le", "version": "go1.18.1",
		}, {
			"GOARCH": "linux/s390x", "version": "go1.18.1",
		}},
		expectedParams: []Params{{
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}},
	}, {
		name: "Fan out explicit combinations, no matrix params",
		matrix: Matrix{
			Include: []MatrixInclude{{
				Name: "build-1",
				Params: []Param{{
					Name: "IMAGE", Value: ParamValue{Type: ParamTypeString, StringVal: "image-1"},
				}, {
					Name: "DOCKERFILE", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/Dockerfile1"}}},
			}, {
				Name: "build-2",
				Params: []Param{{
					Name: "IMAGE", Value: ParamValue{Type: ParamTypeString, StringVal: "image-2"},
				}, {
					Name: "DOCKERFILE", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/Dockerfile2"}}},
			}, {
				Name: "build-3",
				Params: []Param{{
					Name: "IMAGE", Value: ParamValue{Type: ParamTypeString, StringVal: "image-3"},
				}, {
					Name: "DOCKERFILE", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/Dockerfile3"}}},
			}},
		},
		want: Combinations{{
			"IMAGE": "image-1", "DOCKERFILE": "path/to/Dockerfile1",
		}, {
			"IMAGE": "image-2", "DOCKERFILE": "path/to/Dockerfile2",
		}, {
			"IMAGE": "image-3", "DOCKERFILE": "path/to/Dockerfile3",
		}},
		expectedParams: []Params{{
			{
				Name:  "DOCKERFILE",
				Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/Dockerfile1"},
			}, {
				Name:  "IMAGE",
				Value: ParamValue{Type: ParamTypeString, StringVal: "image-1"},
			},
		}, {
			{
				Name:  "DOCKERFILE",
				Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/Dockerfile2"},
			}, {
				Name:  "IMAGE",
				Value: ParamValue{Type: ParamTypeString, StringVal: "image-2"},
			},
		}, {
			{
				Name:  "DOCKERFILE",
				Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/Dockerfile3"},
			}, {
				Name:  "IMAGE",
				Value: ParamValue{Type: ParamTypeString, StringVal: "image-3"},
			},
		}},
	}, {
		name: "matrix include unknown param name, append to all combinations",
		matrix: Matrix{
			Params: Params{{
				Name: "GOARCH", Value: ParamValue{ArrayVal: []string{"linux/amd64", "linux/ppc64le", "linux/s390x"}},
			}, {
				Name: "version", Value: ParamValue{ArrayVal: []string{"go1.17", "go1.18.1"}}},
			},
			Include: []MatrixInclude{{
				Name: "common-package",
				Params: Params{{
					Name: "package", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"}}},
			}},
		},
		want: Combinations{{
			"GOARCH": "linux/amd64", "version": "go1.17", "package": "path/to/common/package/",
		}, {
			"GOARCH": "linux/ppc64le", "version": "go1.17", "package": "path/to/common/package/",
		}, {
			"GOARCH": "linux/s390x", "version": "go1.17", "package": "path/to/common/package/",
		}, {
			"GOARCH": "linux/amd64", "version": "go1.18.1", "package": "path/to/common/package/",
		}, {
			"GOARCH": "linux/ppc64le", "version": "go1.18.1", "package": "path/to/common/package/",
		}, {
			"GOARCH": "linux/s390x", "version": "go1.18.1", "package": "path/to/common/package/",
		}},
		expectedParams: []Params{{
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
			}, {
				Name:  "package",
				Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
			}, {
				Name:  "package",
				Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
			}, {
				Name:  "package",
				Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
			}, {
				Name:  "package",
				Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
			}, {
				Name:  "package",
				Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
			}, {
				Name:  "package",
				Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}},
	}, {
		name: "matrix include param value does not exist, generate a new combination",
		matrix: Matrix{
			Params: Params{{
				Name: "GOARCH", Value: ParamValue{ArrayVal: []string{"linux/amd64", "linux/ppc64le", "linux/s390x"}},
			}, {
				Name: "version", Value: ParamValue{ArrayVal: []string{"go1.17", "go1.18.1"}}},
			},
			Include: []MatrixInclude{{
				Name: "non-existent-arch",
				Params: Params{{
					Name: "GOARCH", Value: ParamValue{Type: ParamTypeString, StringVal: "I-do-not-exist"}}},
			}},
		},
		want: Combinations{{
			"GOARCH": "linux/amd64", "version": "go1.17",
		}, {
			"GOARCH": "linux/ppc64le", "version": "go1.17",
		}, {
			"GOARCH": "linux/s390x", "version": "go1.17",
		}, {
			"GOARCH": "linux/amd64", "version": "go1.18.1",
		}, {
			"GOARCH": "linux/ppc64le", "version": "go1.18.1",
		}, {
			"GOARCH": "linux/s390x", "version": "go1.18.1",
		}, {
			"GOARCH": "I-do-not-exist",
		}},
		expectedParams: []Params{{
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "I-do-not-exist"},
			},
		}},
	}, {
		name: "Matrix include filters single parameter and appends missing vlaues",
		matrix: Matrix{
			Params: Params{{
				Name: "GOARCH", Value: ParamValue{ArrayVal: []string{"linux/amd64", "linux/ppc64le", "linux/s390x"}},
			}, {
				Name: "version", Value: ParamValue{ArrayVal: []string{"go1.17", "go1.18.1"}}},
			},
			Include: []MatrixInclude{{
				Name: "s390x-no-race",
				Params: []Param{{
					Name: "GOARCH", Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
				}, {
					Name: "flags", Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"}}},
			}},
		},
		want: Combinations{{
			"GOARCH": "linux/amd64", "version": "go1.17",
		}, {
			"GOARCH": "linux/ppc64le", "version": "go1.17",
		}, {
			"GOARCH": "linux/s390x", "version": "go1.17", "flags": "-cover -v",
		}, {
			"GOARCH": "linux/amd64", "version": "go1.18.1",
		}, {
			"GOARCH": "linux/ppc64le", "version": "go1.18.1",
		}, {
			"GOARCH": "linux/s390x", "version": "go1.18.1", "flags": "-cover -v",
		}},
		expectedParams: []Params{{
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {

			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
			}, {
				Name:  "flags",
				Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}, {
			{
				Name:  "GOARCH",
				Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
			}, {
				Name:  "flags",
				Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"},
			}, {
				Name:  "version",
				Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
			},
		}},
	},
		{
			name: "Matrix include filters multiple parameters and append new parameters",
			matrix: Matrix{
				Params: Params{{
					Name: "GOARCH", Value: ParamValue{ArrayVal: []string{"linux/amd64", "linux/ppc64le", "linux/s390x"}},
				}, {
					Name: "version", Value: ParamValue{ArrayVal: []string{"go1.17", "go1.18.1"}}},
				},
				Include: []MatrixInclude{
					{
						Name: "390x-no-race",
						Params: Params{{
							Name: "GOARCH", Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"}}, {
							Name: "flags", Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"}}, {
							Name: "version", Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"}}},
					},
					{
						Name: "amd64-no-race",
						Params: Params{{
							Name: "GOARCH", Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"}}, {
							Name: "flags", Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"}}, {
							Name: "version", Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"}}},
					},
				},
			},
			want: Combinations{{
				"GOARCH": "linux/amd64", "version": "go1.17", "flags": "-cover -v",
			}, {
				"GOARCH": "linux/ppc64le", "version": "go1.17",
			}, {
				"GOARCH": "linux/s390x", "version": "go1.17",
			}, {
				"GOARCH": "linux/amd64", "version": "go1.18.1",
			}, {
				"GOARCH": "linux/ppc64le", "version": "go1.18.1",
			}, {
				"GOARCH": "linux/s390x", "version": "go1.18.1", "flags": "-cover -v",
			}},
			expectedParams: []Params{{
				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
				}, {
					Name:  "flags",
					Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"},
				}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
				},
			}, {
				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
				}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
				},
			}, {

				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
				}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
				},
			}, {
				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
				}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
				},
			}, {
				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
				}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
				},
			}, {
				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
				}, {
					Name:  "flags",
					Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"},
				}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
				},
			}},
		}, {
			name: "Matrix params and include params handles filter, appending, and generating new combinations at once",
			matrix: Matrix{
				Params: Params{{
					Name: "GOARCH", Value: ParamValue{ArrayVal: []string{"linux/amd64", "linux/ppc64le", "linux/s390x"}},
				}, {
					Name: "version", Value: ParamValue{ArrayVal: []string{"go1.17", "go1.18.1"}}},
				},
				Include: []MatrixInclude{{
					Name: "common-package",
					Params: []Param{{
						Name: "package", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"}}},
				}, {
					Name: "s390x-no-race",
					Params: []Param{{
						Name: "GOARCH", Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
					}, {
						Name: "flags", Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"}}},
				}, {
					Name: "go117-context",
					Params: []Param{{
						Name: "version", Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
					}, {
						Name: "context", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/go117/context"}}},
				}, {
					Name: "non-existent-arch",
					Params: []Param{{
						Name: "GOARCH", Value: ParamValue{Type: ParamTypeString, StringVal: "I-do-not-exist"}},
					},
				}},
			},
			want: Combinations{{
				"GOARCH": "linux/amd64", "context": "path/to/go117/context", "package": "path/to/common/package/", "version": "go1.17",
			}, {
				"GOARCH": "linux/ppc64le", "context": "path/to/go117/context", "package": "path/to/common/package/", "version": "go1.17",
			}, {
				"GOARCH": "linux/s390x", "context": "path/to/go117/context", "flags": "-cover -v", "package": "path/to/common/package/", "version": "go1.17",
			}, {
				"GOARCH": "linux/amd64", "package": "path/to/common/package/", "version": "go1.18.1",
			}, {
				"GOARCH": "linux/ppc64le", "package": "path/to/common/package/", "version": "go1.18.1",
			}, {
				"GOARCH": "linux/s390x", "package": "path/to/common/package/", "flags": "-cover -v", "version": "go1.18.1",
			}, {
				"GOARCH": "I-do-not-exist",
			}},
			expectedParams: []Params{{
				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
				}, {
					Name:  "context",
					Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/go117/context"},
				}, {
					Name:  "package",
					Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"},
				}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
				},
			}, {
				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
				}, {
					Name:  "context",
					Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/go117/context"},
				}, {
					Name:  "package",
					Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"},
				}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
				},
			}, {
				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
				}, {
					Name:  "context",
					Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/go117/context"},
				}, {
					Name:  "flags",
					Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"},
				}, {
					Name:  "package",
					Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"},
				}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
				},
			}, {
				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/amd64"},
				}, {
					Name:  "package",
					Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"}}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
				},
			}, {
				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/ppc64le"},
				}, {
					Name:  "package",
					Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"},
				}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
				},
			}, {

				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
				}, {
					Name:  "flags",
					Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"},
				}, {
					Name:  "package",
					Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"},
				}, {
					Name:  "version",
					Value: ParamValue{Type: ParamTypeString, StringVal: "go1.18.1"},
				},
			}, {
				{
					Name:  "GOARCH",
					Value: ParamValue{Type: ParamTypeString, StringVal: "I-do-not-exist"},
				},
			}},
		}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.matrix.FanOut()
			if d := cmp.Diff(tt.want, tt.matrix.FanOut()); d != "" {
				t.Errorf("Combinations of Parameters did not match the expected Params: %s", diff.PrintWantGot(d))
			}
			c := tt.matrix.FanOut().ToParams()
			for i := range c {
				if d := cmp.Diff(tt.expectedParams[i], c[i]); d != "" {
					t.Errorf("The formatted Combinations of Parameters did not match the expected Params: %s", diff.PrintWantGot(d))
				}
			}
		})
	}
}

func TestMatrix_HasParams(t *testing.T) {
	testCases := []struct {
		name   string
		matrix *Matrix
		want   bool
	}{
		{
			name:   "nil matrix",
			matrix: nil,
			want:   false,
		},
		{
			name:   "empty matrix",
			matrix: &Matrix{},
			want:   false,
		},
		{
			name: "matrixed with params",
			matrix: &Matrix{
				Params: []Param{{Name: "platform", Value: ParamValue{ArrayVal: []string{"linux", "windows"}}}},
			},
			want: true,
		}, {
			name: "matrixed with include",
			matrix: &Matrix{
				Include: []MatrixInclude{{
					Name: "build-1",
					Params: []Param{{
						Name: "IMAGE", Value: ParamValue{Type: ParamTypeString, StringVal: "image-1"},
					}, {
						Name: "DOCKERFILE", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/Dockerfile1"}}},
				}},
			},
			want: false,
		}, {
			name: "matrixed with params and include",
			matrix: &Matrix{
				Params: []Param{{
					Name: "GOARCH", Value: ParamValue{ArrayVal: []string{"linux/amd64", "linux/ppc64le", "linux/s390x"}},
				}},
				Include: []MatrixInclude{{
					Name: "common-package",
					Params: []Param{{
						Name: "package", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"}}},
				}},
			},
			want: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if d := cmp.Diff(tc.want, tc.matrix.hasParams()); d != "" {
				t.Errorf("matrix.hasParams() bool diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestMatrix_HasInclude(t *testing.T) {
	testCases := []struct {
		name   string
		matrix *Matrix
		want   bool
	}{
		{
			name:   "nil matrix",
			matrix: nil,
			want:   false,
		},
		{
			name:   "empty matrix",
			matrix: &Matrix{},
			want:   false,
		},
		{
			name: "matrixed with params",
			matrix: &Matrix{
				Params: []Param{{Name: "platform", Value: ParamValue{ArrayVal: []string{"linux", "windows"}}}},
			},
			want: false,
		}, {
			name: "matrixed with include",
			matrix: &Matrix{
				Include: []MatrixInclude{{
					Name: "build-1",
					Params: []Param{{
						Name: "IMAGE", Value: ParamValue{Type: ParamTypeString, StringVal: "image-1"},
					}, {
						Name: "DOCKERFILE", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/Dockerfile1"}}},
				}},
			},
			want: true,
		}, {
			name: "matrixed with params and include",
			matrix: &Matrix{
				Params: []Param{{
					Name: "GOARCH", Value: ParamValue{ArrayVal: []string{"linux/amd64", "linux/ppc64le", "linux/s390x"}},
				}},
				Include: []MatrixInclude{{
					Name: "common-package",
					Params: []Param{{
						Name: "package", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"}}},
				}},
			},
			want: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if d := cmp.Diff(tc.want, tc.matrix.hasInclude()); d != "" {
				t.Errorf("matrix.hasInclude() bool diff %s", diff.PrintWantGot(d))
			}
		})
	}
}

func TestPipelineTask_CountCombinations(t *testing.T) {
	tests := []struct {
		name   string
		matrix *Matrix
		want   int
	}{{
		name: "combinations count is zero",
		matrix: &Matrix{
			Params: []Param{{}}},
		want: 0,
	}, {
		name: "combinations count is one from one parameter",
		matrix: &Matrix{
			Params: []Param{{
				Name: "foo", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"foo"}},
			}}},

		want: 1,
	}, {
		name: "combinations count is one from two parameters",
		matrix: &Matrix{
			Params: []Param{{
				Name: "foo", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"foo"}},
			}, {
				Name: "bar", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"bar"}},
			}}},
		want: 1,
	}, {
		name: "combinations count is two from one parameter",
		matrix: &Matrix{
			Params: []Param{{
				Name: "foo", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"foo", "bar"}},
			}}},
		want: 2,
	}, {
		name: "combinations count is nine",
		matrix: &Matrix{
			Params: []Param{{
				Name: "foo", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"f", "o", "o"}},
			}, {
				Name: "bar", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"b", "a", "r"}},
			}}},
		want: 9,
	}, {
		name: "combinations count is large",
		matrix: &Matrix{
			Params: []Param{{
				Name: "foo", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"f", "o", "o"}},
			}, {
				Name: "bar", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"b", "a", "r"}},
			}, {
				Name: "quz", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"q", "u", "x"}},
			}, {
				Name: "xyzzy", Value: ParamValue{Type: ParamTypeArray, ArrayVal: []string{"x", "y", "z", "z", "y"}},
			}}},
		want: 135,
	}, {
		name: "explicit combinations in the matrix",
		matrix: &Matrix{
			Include: []MatrixInclude{{
				Name: "build-1",
				Params: []Param{{
					Name: "IMAGE", Value: ParamValue{Type: ParamTypeString, StringVal: "image-1"},
				}, {
					Name: "DOCKERFILE", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/Dockerfile1"},
				}},
			}, {
				Name: "build-2",
				Params: []Param{{
					Name: "IMAGE", Value: ParamValue{Type: ParamTypeString, StringVal: "image-2"},
				}, {
					Name: "DOCKERFILE", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/Dockerfile2"},
				}},
			}, {
				Name: "build-3",
				Params: []Param{{
					Name: "IMAGE", Value: ParamValue{Type: ParamTypeString, StringVal: "image-3"},
				}, {
					Name: "DOCKERFILE", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/Dockerfile3"},
				}},
			}},
		},
		want: 3,
	}, {
		name: "params and include in matrix with overriding combinations params",
		matrix: &Matrix{
			Params: []Param{{
				Name: "GOARCH", Value: ParamValue{ArrayVal: []string{"linux/amd64", "linux/ppc64le", "linux/s390x"}},
			}, {
				Name: "version", Value: ParamValue{ArrayVal: []string{"go1.17", "go1.18.1"}}},
			},
			Include: []MatrixInclude{{
				Name: "common-package",
				Params: []Param{{
					Name: "package", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"}}},
			}, {
				Name: "s390x-no-race",
				Params: []Param{{
					Name: "GOARCH", Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
				}, {
					Name: "flags", Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"}}},
			}, {
				Name: "go117-context",
				Params: []Param{{
					Name: "version", Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
				}, {
					Name: "context", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/go117/context"}}},
			}},
		},
		want: 6,
	}, {
		name: "params and include in matrix with overriding combinations params and one new combination",
		matrix: &Matrix{
			Params: []Param{{
				Name: "GOARCH", Value: ParamValue{ArrayVal: []string{"linux/amd64", "linux/ppc64le", "linux/s390x"}},
			}, {
				Name: "version", Value: ParamValue{ArrayVal: []string{"go1.17", "go1.18.1"}}},
			},
			Include: []MatrixInclude{{
				Name: "common-package",
				Params: []Param{{
					Name: "package", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/common/package/"}}},
			}, {
				Name: "s390x-no-race",
				Params: []Param{{
					Name: "GOARCH", Value: ParamValue{Type: ParamTypeString, StringVal: "linux/s390x"},
				}, {
					Name: "flags", Value: ParamValue{Type: ParamTypeString, StringVal: "-cover -v"}}},
			}, {
				Name: "go117-context",
				Params: []Param{{
					Name: "version", Value: ParamValue{Type: ParamTypeString, StringVal: "go1.17"},
				}, {
					Name: "context", Value: ParamValue{Type: ParamTypeString, StringVal: "path/to/go117/context"}}},
			}, {
				Name: "non-existent-arch",
				Params: []Param{{
					Name: "GOARCH", Value: ParamValue{Type: ParamTypeString, StringVal: "I-do-not-exist"}},
				}},
			}},
		want: 7,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if d := cmp.Diff(tt.want, tt.matrix.CountCombinations()); d != "" {
				t.Errorf("Matrix.CountCombinations() errors diff %s", diff.PrintWantGot(d))
			}
		})
	}
}
