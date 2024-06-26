// Copyright 2023 Cisco Systems, Inc. and its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package mvc

import (
	"context"
	"errors"
	"fmt"
	"github.com/cisco-open/go-lanai/pkg/web"
	"net/http"
	"reflect"
)

/*****************************
	Func Metadata
******************************/
const (
	templateInvalidMvcHandlerFunc = "invalid MVC handler function signature: %v, but got <%v>"
	errorMsgExpectFunc       = "expecting a function"
	errorMsgInputParams      = "function should have one or two input parameters, where the first is context.Context and the second is a struct or pointer to struct"
	errorMsgOutputParams     = "function should have at least two output parameters, where the the last is error"
	errorMsgInvalidSignature = "unable to find request or response type"
)

// mapping related
type errorInvalidMvcHandlerFunc struct {
	reason error
	target *reflect.Value
}

func (e *errorInvalidMvcHandlerFunc) Error() string {
	return fmt.Sprintf(templateInvalidMvcHandlerFunc, e.reason.Error(), e.target.Type())
}

var (
	specialTypeContext        = reflect.TypeOf((*context.Context)(nil)).Elem()
	specialTypeHttpRequestPtr = reflect.TypeOf(&http.Request{})
	specialTypeInt            = reflect.TypeOf(0)
	specialTypeHttpHeader     = reflect.TypeOf((*http.Header)(nil)).Elem()
	specialTypeError          = reflect.TypeOf((*error)(nil)).Elem()
)

// HandlerFuncValidator validate HandlerFunc signature
type HandlerFuncValidator func(f *reflect.Value) error

// HandlerFunc is a function with supported signature to handle MVC request and returns MVC response or error
// See rest.MappingBuilder and template.MappingBuilder for supported function signatures
type HandlerFunc interface{}

type param struct {
	i int
	t reflect.Type
}

func (p param) isValid() bool {
	return p.i >= 0 && p.t != nil
}

// out parameters
type mvcOut struct {
	count    int
	sc       param
	header   param
	response param
	error    param
}

// in parameters
type mvcIn struct {
	count   int
	context param
	request param
}

type Metadata struct {
	function *reflect.Value
	request  reflect.Type
	response reflect.Type
	in       mvcIn
	out      mvcOut
}

// NewFuncMetadata uses reflect to analyze the given handler function and create a Metadata.
// this function panic if given function have incorrect signature
// Caller can provide an optional validator to further validate function signature on top of default validation
func NewFuncMetadata(endpointFunc HandlerFunc, validator HandlerFuncValidator) *Metadata {
	f := reflect.ValueOf(endpointFunc)
	err := validateFunc(&f, validator)
	if err != nil {
		//fatal error
		panic(err)
	}

	t := f.Type()
	unknown := param{-1, nil}
	meta := Metadata{
		function: &f,
		in: mvcIn{
			context: unknown, request: unknown,
		},
		out: mvcOut{
			sc: unknown, header: unknown,
			response: unknown, error: unknown,
		},
	}

	// parse input params
	for i := t.NumIn() - 1; i >= 0; i-- {
		switch it := t.In(i); {
		case it.ConvertibleTo(specialTypeContext):
			meta.in.context = param{i, it}
		case !meta.in.request.isValid() && isSupportedRequestType(it):
			meta.in.request = param{i, it}
			meta.request = it
		default:
			panic(&errorInvalidMvcHandlerFunc{
				reason: errors.New(fmt.Sprintf("unknown input parameters at index %v", i)),
				target: &f,
			})
		}
		meta.in.count++
	}

	// parse output params
	for i := t.NumOut() - 1; i >= 0; i-- {
		switch ot := t.Out(i); {
		case ot.ConvertibleTo(specialTypeInt):
			meta.out.sc = param{i, ot}
		case ot.ConvertibleTo(specialTypeHttpHeader):
			meta.out.header = param{i, ot}
		case ot.ConvertibleTo(specialTypeError):
			meta.out.error = param{i, ot}
		case !meta.out.response.isValid() && isSupportedResponseType(ot):
			// we allow interface and map as response
			meta.out.response = param{i, ot}
			meta.response = ot
		default:
			panic(&errorInvalidMvcHandlerFunc{
				reason: errors.New(fmt.Sprintf("unknown return parameters at index %v", i)),
				target: &f,
			})
		}
		meta.out.count++
	}

	if meta.response == nil || meta.in.count < 1 || meta.out.count < 2 || meta.in.count > 1 && meta.request == nil {
		panic(&errorInvalidMvcHandlerFunc{
			reason: errors.New(errorMsgInvalidSignature),
			target: &f,
		})
	}

	return &meta
}

func (m Metadata) HandlerFunc() web.MvcHandlerFunc {
	return func(c context.Context, request interface{}) (response interface{}, err error) {
		// prepare input params
		in := make([]reflect.Value, m.in.count)
		in[m.in.context.i] = reflect.ValueOf(c)
		if m.in.request.isValid() {
			in[m.in.request.i] = reflect.ValueOf(request)
		}

		out := m.function.Call(in)

		// post process output
		err, _ = out[m.out.error.i].Interface().(error)
		response = out[m.out.response.i].Interface()
		if !m.out.sc.isValid() && !m.out.header.isValid() {
			return response, err
		}

		// if necessary, wrap the response
		wrapper := &web.Response{B: response}
		if m.out.sc.isValid() {
			wrapper.SC = int(out[m.out.sc.i].Int())
		}

		if m.out.header.isValid() {
			wrapper.H, _ = out[m.out.header.i].Interface().(http.Header)
		}

		return wrapper, err
	}
}

func validateFunc(f *reflect.Value, validator HandlerFuncValidator) (err error) {
	// For now, we check function signature at runtime.
	// I wish there is a way to check it at compile-time that I didn't know of
	t := f.Type()
	switch {
	case f.Kind() != reflect.Func:
		return &errorInvalidMvcHandlerFunc{
			reason: errors.New(errorMsgExpectFunc),
			target: f,
		}
	// In params validation
	case t.NumIn() < 1 || t.NumIn() > 2:
		fallthrough
	case !t.In(0).ConvertibleTo(specialTypeContext):
		fallthrough
	case t.NumIn() == 2 && !isSupportedRequestType(t.In(t.NumIn()-1)):
		return &errorInvalidMvcHandlerFunc{
			reason: errors.New(errorMsgInputParams),
			target: f,
		}

	// Out params validation
	case t.NumOut() < 2:
		fallthrough
	case !t.Out(t.NumOut() - 1).ConvertibleTo(specialTypeError):
		return &errorInvalidMvcHandlerFunc{
			reason: errors.New(errorMsgOutputParams),
			target: f,
		}
	}

	if validator != nil {
		return validator(f)
	}
	return nil
}

func isStructOrPtrToStruct(t reflect.Type) (ret bool) {
	ret = t.Kind() == reflect.Struct
	ret = ret || t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct
	return
}

// isHttpRequestPtr returns true if given type is *http.Request
func isHttpRequestPtr(t reflect.Type) bool {
	return t == specialTypeHttpRequestPtr
}

func isSupportedRequestType(t reflect.Type) bool {
	return isStructOrPtrToStruct(t)
}

func isSupportedResponseType(t reflect.Type) bool {
	if isStructOrPtrToStruct(t) {
		return true
	}
	switch t.Kind() {
	case reflect.Interface:
		fallthrough
	case reflect.Map:
		fallthrough
	case reflect.String:
		return true
	case reflect.Slice:
		fallthrough
	case reflect.Array:
		return t.Elem().Kind() == reflect.Uint8
	default:
		return false
	}
}
