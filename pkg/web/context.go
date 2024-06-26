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

package web

import (
	"context"
	"github.com/cisco-open/go-lanai/pkg/utils/matcher"
	"net/http"
	"regexp"
)

// Validation reference: https://godoc.org/github.com/go-playground/validator#hdr-Baked_In_Validators_and_Tags

var (
	pathParamPattern, _ = regexp.Compile(`\/:[^\/]*`)
)

/*********************************
	Customization
 *********************************/

// Customizer is invoked by Registrar at the beginning of initialization,
// customizers can register anything except for additional customizers
// If a customizer retains the given context in anyway, it should also implement PostInitCustomizer to release it
type Customizer interface {
	Customize(ctx context.Context, r *Registrar) error
}

// PostInitCustomizer is invoked by Registrar after initialization, register anything in PostInitCustomizer.PostInit
// would cause error or takes no effect
type PostInitCustomizer interface {
	Customizer
	PostInit(ctx context.Context, r *Registrar) error
}

type EngineOptions func(*Engine)

/*********************************
	Request
 *********************************/

// DecodeRequestFunc extracts a payload from a http.Request. It's designed to be used by MvcMapping.
// Example of common implementation includes JSON decoder or form data extractor
type DecodeRequestFunc func(ctx context.Context, httpReq *http.Request) (req interface{}, err error)

// RequestRewriter handles request rewrite. e.g. rewrite http.Request.URL.Path
type RequestRewriter interface {
	// HandleRewrite take the rewritten request and put it through the entire handling cycle.
	// The http.Request.Context() is carried over
	// Note: if no error is returned, caller should stop processing the original request and discard the original request
	HandleRewrite(rewritten *http.Request) error
}

/*********************************
	Response
 *********************************/

// EncodeResponseFunc encodes a user response object into http.ResponseWriter. It's designed to be used by MvcMapping.
// Example of common implementation includes JSON encoder or template based HTML generator.
type EncodeResponseFunc func(ctx context.Context, rw http.ResponseWriter, resp interface{}) error

// StatusCoder is an additional interface that a user response object or error could implement.
// EncodeResponseFunc and EncodeErrorFunc should typically check for this interface and manipulate response status code accordingly
type StatusCoder interface {
	StatusCode() int
}

// Headerer is an additional interface that a user response object or error could implement.
// EncodeResponseFunc and EncodeErrorFunc should typically check for this interface and manipulate response headers accordingly
type Headerer interface {
	Headers() http.Header
}

// BodyContainer is an additional interface that a user response object or error could implement.
// This interface is majorly used internally for mapping
type BodyContainer interface {
	Body() interface{}
}

/*********************************
	Error Translator
 *********************************/

// EncodeErrorFunc is responsible for encoding an error to the ResponseWriter. It's designed to be used by MvcMapping.
// Example of common implementation includes JSON encoder or template based HTML generator.
type EncodeErrorFunc func(ctx context.Context, err error, w http.ResponseWriter)

// ErrorTranslator can be registered via web.Registrar
// it will contribute our MvcMapping's error handling process.
// Note: it won't contribute Middleware's error handling
//
// Implementing Notes:
// 	1. if it doesn't handle the error, return same error
//  2. if custom StatusCode is required, make the returned error implement StatusCoder
//  3. if custom Header is required, make the returned error implement Headerer
//  4. we have HttpError to help with custom Headerer and StatusCoder implementation
type ErrorTranslator interface {
	Translate(ctx context.Context, err error) error
}

// ErrorTranslateFunc is similar to ErrorTranslator in function format. Mostly used for selective error translation
// registration (ErrorHandlerMapping). Same implementing rules applies
type ErrorTranslateFunc func(ctx context.Context, err error) error
func (fn ErrorTranslateFunc) Translate(ctx context.Context, err error) error {
	return fn(ctx, err)
}

/*********************************
	Mappings
 *********************************/

// Controller is usually implemented by user-domain types to provide a group of HTTP handling logics.
// Each Controller provides a list of Mapping that defines how HTTP requests should be handled.
// See Mapping
type Controller interface {
	Mappings() []Mapping
}

// Mapping is generic interface for all kind of HTTP mappings.
// User-domain do not typically to implement this interface. Instead, predefined implementation and their builders
// should be used.
// See StaticMapping, RoutedMapping, MvcMapping, SimpleMapping, etc.
type Mapping interface {
	Name() string
}

// StaticMapping defines static assets handling. e.g. javascripts, css, images, etc.
// See assets.New()
type StaticMapping interface {
	Mapping
	Path() string
	StaticRoot() string
	Aliases() map[string]string
	AddAlias(path, filePath string) StaticMapping
}

// RoutedMapping defines dynamic HTTP handling with specific HTTP Route (path and method) and optionally a RequestMatcher as condition.
// RoutedMapping includes SimpleMapping, MvcMapping, etc.
type RoutedMapping interface {
	Mapping
	Group() string
	Path() string
	Method() string
	Condition() RequestMatcher
}

// SimpleMapping endpoints that are directly implemented as HandlerFunc.
// See mapping.MappingBuilder
type SimpleMapping interface {
	RoutedMapping
	HandlerFunc() http.HandlerFunc
}

// MvcHandlerFunc is the generic function to be used for MvcMapping.
// See MvcMapping, rest.EndpointFunc, template.ModelViewHandlerFunc
type MvcHandlerFunc func(c context.Context, request interface{}) (response interface{}, err error)

// MvcMapping defines HTTP handling that follows MVC pattern:
// 1. The http.Request is decoded in to a request model object using MvcMapping.DecodeRequestFunc().
// 2. The request model object is processed by MvcMapping.HandlerFunc() and a response model object is returned.
// 3. The response model object is rendered into http.ResponseWriter using MvcMapping.EncodeResponseFunc().
// 4. If any steps yield error, the error is rendered into http.ResponseWriter using MvcMapping.EncodeErrorFunc()
//
// Note:
// Functions here are all weakly typed signature. User-domain developers typically should use mapping builders
// (rest.MappingBuilder, template.MappingBuilder, etc) to create concrete MvcMapping instances.
// See EndpointMapping or TemplateMapping
type MvcMapping interface {
	RoutedMapping
	DecodeRequestFunc() DecodeRequestFunc
	EncodeResponseFunc() EncodeResponseFunc
	EncodeErrorFunc() EncodeErrorFunc
	HandlerFunc() MvcHandlerFunc
}

// EndpointMapping defines REST API mapping.
// REST API is usually implemented by Controller and accept/produce JSON objects
// See rest.MappingBuilder
type EndpointMapping MvcMapping

// TemplateMapping defines templated MVC mapping. e.g. html templates
// Templated MVC is usually implemented by Controller and produce a template and model for dynamic html generation.
// See template.MappingBuilder
type TemplateMapping MvcMapping

// MiddlewareMapping defines middlewares that applies to all or selected set (via Matcher and Condition) of requests.
// Middlewares are often used for task like security, pre/post processing request or response, metrics measurements, etc.
// See middleware.MappingBuilder
type MiddlewareMapping interface {
	Mapping
	Matcher() RouteMatcher
	Order() int
	Condition() RequestMatcher
	HandlerFunc() http.HandlerFunc
}

// ErrorTranslateMapping defines how errors should be handled before it's rendered into http.ResponseWriter.
// See weberror.MappingBuilder
type ErrorTranslateMapping interface {
	Mapping
	Matcher() RouteMatcher
	Order() int
	Condition() RequestMatcher
	TranslateFunc() ErrorTranslateFunc
}

/*********************************
	Routing Matchers
 *********************************/

// Route contains information needed for registering handler func in gin.Engine
type Route struct {
	Method string
	Path   string
	Group  string
}

// RouteMatcher is a typed ChainableMatcher that accept *Route or Route
type RouteMatcher interface {
	matcher.ChainableMatcher
}

// RequestMatcher is a typed ChainableMatcher that accept *http.Request or http.Request
type RequestMatcher interface {
	matcher.ChainableMatcher
}

// NormalizedPath removes path parameter name from path.
// path "/path/with/:param" is effectively same as "path/with/:other_param_name"
func NormalizedPath(path string) string {
	return pathParamPattern.ReplaceAllString(path, "/:var")
}

/*********************************
	SimpleMapping
 *********************************/

// simpleMapping implements SimpleMapping
type simpleMapping struct {
	name        string
	group       string
	path        string
	method      string
	condition   RequestMatcher
	handlerFunc http.HandlerFunc
}

// NewSimpleMapping create a SimpleMapping.
// It's recommended to use mapping.MappingBuilder instead of this function:
// e.g.
// <code>
// mapping.Post("/path/to/api").HandlerFunc(func...).Build()
// </code>
func NewSimpleMapping(name, group, path, method string, condition RequestMatcher, handlerFunc http.HandlerFunc) SimpleMapping {
	return &simpleMapping{
		name:        name,
		group:       group,
		path:        path,
		method:      method,
		condition:   condition,
		handlerFunc: handlerFunc,
	}
}

func (g simpleMapping) HandlerFunc() http.HandlerFunc {
	return g.handlerFunc
}

func (g simpleMapping) Condition() RequestMatcher {
	return g.condition
}

func (g simpleMapping) Method() string {
	return g.method
}

func (g simpleMapping) Group() string {
	return g.group
}

func (g simpleMapping) Path() string {
	return g.path
}

func (g simpleMapping) Name() string {
	return g.name
}

