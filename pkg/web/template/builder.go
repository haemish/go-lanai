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

package template

import (
	"errors"
	"fmt"
	"github.com/cisco-open/go-lanai/pkg/web"
	"github.com/cisco-open/go-lanai/pkg/web/internal/mvc"
	"net/http"
	"reflect"
)

var supportedResponseTypes = []reflect.Type {
	reflect.TypeOf(ModelView{}),
	reflect.TypeOf(&ModelView{}),
}

// ModelViewHandlerFunc is a function with following signature
// 	- two input parameters with 1st as context.Context and 2nd as <request>
// 	- two output parameters with 1st as <response> and 2nd as error
// where
// <request>:   a struct or a pointer to a struct whose fields are properly tagged
// <response>:  a pointer to a ModelView.
// e.g.: func(context.Context, request *AnyStructWithTag) (response *ModelView, error) {...}
type ModelViewHandlerFunc interface{}

// MappingBuilder builds web.TemplateMapping using web.GinBindingRequestDecoder, TemplateEncodeResponseFunc and TemplateErrorEncoder
// MappingBuilder.Path, MappingBuilder.Method and MappingBuilder.HandlerFunc are required to successfully build a mapping.
// See ModelViewHandlerFunc for supported strongly typed function signatures.
// Example:
// <code>
// template.Post("/path/to/page").HandlerFunc(func...).Build()
// </code>
type MappingBuilder struct {
	name        string
	group       string
	path        string
	method      string
	condition   web.RequestMatcher
	handlerFunc ModelViewHandlerFunc
}

func New(names ...string) *MappingBuilder {
	var name string
	if len(names) > 0 {
		name = names[0]
	}
	return &MappingBuilder{
		name:   name,
		method: web.MethodAny,
	}
}

// Convenient Constructors

func Any(path string) *MappingBuilder {
	return New().Path(path).Method(web.MethodAny)
}

func Get(path string) *MappingBuilder {
	return New().Get(path)
}

func Post(path string) *MappingBuilder {
	return New().Post(path)
}

/*****************************
	Public
******************************/

func (b *MappingBuilder) Name(name string) *MappingBuilder {
	b.name = name
	return b
}

func (b *MappingBuilder) Group(group string) *MappingBuilder {
	b.group = group
	return b
}

func (b *MappingBuilder) Path(path string) *MappingBuilder {
	b.path = path
	return b
}

func (b *MappingBuilder) Method(method string) *MappingBuilder {
	b.method = method
	return b
}

func (b *MappingBuilder) Condition(condition web.RequestMatcher) *MappingBuilder {
	b.condition = condition
	return b
}

func (b *MappingBuilder) HandlerFunc(endpointFunc ModelViewHandlerFunc) *MappingBuilder {
	b.handlerFunc = endpointFunc
	return b
}

// Convenient setters

func (b *MappingBuilder) Get(path string) *MappingBuilder {
	return b.Path(path).Method(http.MethodGet)
}

func (b *MappingBuilder) Post(path string) *MappingBuilder {
	return b.Path(path).Method(http.MethodPost)
}

func (b *MappingBuilder) Build() web.TemplateMapping {
	if err := b.validate(); err != nil {
		panic(err)
	}
	return b.buildMapping()
}

/*****************************
	Private
******************************/
func (b *MappingBuilder) validate() (err error) {
	if b.path == "" && (b.group == "" || b.group == "/") {
		err = errors.New("empty path")
	}

	if b.handlerFunc == nil {
		err = errors.New("handler func is required for template mapping")
	}
	return
}

func (b *MappingBuilder) buildMapping() web.MvcMapping {
	if b.method == "" {
		b.method = web.MethodAny
	}

	if b.name == "" {
		b.name = fmt.Sprintf("%s %s", b.method, b.path)
	}

	metadata := mvc.NewFuncMetadata(b.handlerFunc, validateHandlerFunc)
	decReq := mvc.GinBindingRequestDecoder(metadata)
	encResp := TemplateEncodeResponseFunc

	return web.NewMvcMapping(b.name, b.group, b.path, b.method, b.condition,
		metadata.HandlerFunc(), decReq, encResp, TemplateErrorEncoder)
}

// this is an additional validator, to make sure the response value is supported type
func validateHandlerFunc(f *reflect.Value) error {
	if !f.IsValid() || f.IsZero() {
		return errors.New("missing ModelViewHandlerFunc")
	}
	t := f.Type()
	// check response type
	foundMV := false
	OUTER:
	for i := t.NumOut() - 1; i >= 0; i-- {
		for _, supported := range supportedResponseTypes {
			if t.Out(i).ConvertibleTo(supported) {
				foundMV = true
				break OUTER
			}
		}
	}

	switch {
	case !foundMV:
		return errors.New("ModelViewHandlerFunc need return ModelView or *ModelView")
		//more checks if needed
	}

	return nil
}
