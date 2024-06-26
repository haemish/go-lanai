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
	"fmt"
	"github.com/cisco-open/go-lanai/pkg/utils"
	"github.com/gin-gonic/gin"
	"net/http"
	"path"
)

type contextPathCtxKey struct {}

// Interfaces, functions, HandlerFunc wrappers and gin middlewares that make sure *gin.Context available in endpoints and
// context properly propagated in Request

// SimpleGinMapping is a SimpleMapping that supported by gin.HandlerFunc
// See mapping.MappingBuilder
type SimpleGinMapping interface {
	SimpleMapping
	GinHandlerFunc() gin.HandlerFunc
}

// MiddlewareGinMapping is a MiddlewareMapping that supported by gin.HandlerFunc
// See middleware.MappingBuilder
type MiddlewareGinMapping interface {
	MiddlewareMapping
	GinHandlerFunc() gin.HandlerFunc
}

/**************************
	Public
 **************************/

// GinContext returns *gin.Context which either contained in the context or is the given context itself
func GinContext(ctx context.Context) *gin.Context {
	if ginCtx, ok := ctx.(*gin.Context); ok {
		return ginCtx
	}

	if ginCtx, ok := ctx.Value(gin.ContextKey).(*gin.Context); ok {
		return ginCtx
	}
	return nil
}

// MustGinContext returns *gin.Context like GinContext but panic if not found
func MustGinContext(ctx context.Context) *gin.Context {
	if gc := GinContext(ctx); gc != nil {
		return gc
	}
	panic(fmt.Sprintf("gin.Context is not found in given context %v", ctx))
}

// HttpRequest returns *http.Request associated with given context
func HttpRequest(ctx context.Context) *http.Request {
	if gc := GinContext(ctx); gc != nil {
		return gc.Request
	}
	return nil
}

// MustHttpRequest returns *http.Request associated with given context, panic if not found
func MustHttpRequest(ctx context.Context) *http.Request {
	return MustGinContext(ctx).Request
}

// ContextPath returns the "server.context-path" from properties with leading "/".
// This function returns empty string if context-path is root or not set
func ContextPath(ctx context.Context) string {
	ctxPath, _ := ctx.Value(contextPathCtxKey{}).(string)
	return ctxPath
}

// SetKV set a kv pair to given context if:
// - The context is a utils.MutableContext
// - The context has utils.MutableContext as parent/ancestors
// - The context contains *gin.Context
// The value then can be obtained via context.Context.Value(key)
//
// This function uses utils.FindMutableContext and GinContext() to find KV storage. Then store KV pair using following rule:
// - If utils.FindMutableContext returns non-nil, utils.MutableContext interface is used
// - If utils.FindMutableContext returns nil but *gin.Context is found:
// 		+ If the key is string, KV pair is set as-is
// 		+ Otherwise, uses fmt.Sprintf(`%v`, key) as key and set KV pair
// - If none of conditions met, this function does nothing
func SetKV(ctx context.Context, key interface{}, value interface{}) {
	if mc := utils.FindMutableContext(ctx); mc != nil {
		mc.Set(key, value)
	}
	if gc := GinContext(ctx); gc != nil {
		switch k := key.(type) {
		case string:
			gc.Set(k, value)
		default:
			gc.Set(fmt.Sprintf("%v", key), value)
		}
	}
}

/**************************
	Customizers
 **************************/

// PriorityGinContextCustomizer implements Customizer and order.PriorityOrdered
type PriorityGinContextCustomizer struct {
	properties *ServerProperties
}

func NewPriorityGinContextCustomizer(properties *ServerProperties) *PriorityGinContextCustomizer {
	return &PriorityGinContextCustomizer{
		properties: properties,
	}
}

func (c PriorityGinContextCustomizer) PriorityOrder() int {
	// medium precedence, makes this customizer before any non-priority-ordered customizers
	return 0
}

//nolint:contextcheck // context is not relevant here - should pass the context parameter
func (c PriorityGinContextCustomizer) Customize(_ context.Context, r *Registrar) error {
	if e := r.AddGlobalMiddlewares(GinContextMerger()); e != nil {
		return e
	}
	if e := r.AddGlobalMiddlewares(PropertiesAware(c.properties)); e != nil {
		return e
	}
	return r.AddEngineOptions(func(engine *Engine) {
		engine.ContextWithFallback = true
	})
}

/**************************
	Handler Func
 **************************/

// PropertiesAware is a Gin middleware mandatory for all mappings.
// It save necessary properties into request's context. e.g. context-path
// The saved properties can be used in many components/utilities.
func PropertiesAware(props *ServerProperties) gin.HandlerFunc {
	return func(gc *gin.Context) {
		if mc := utils.FindMutableContext(gc); mc != nil {
			ctxPath := path.Clean("/" + props.ContextPath)
			if ctxPath != "/" && ctxPath != "." {
				mc.Set(contextPathCtxKey{}, ctxPath)
			}
		}
	}
}

// GinContextMerger is a Gin middleware that merge Request.Context() with gin.Context,
// allowing values in gin.Context also available via Request.Context().Value().
// This middleware is mandatory for all mappings.
// Note:	as of Gin 1.8.0, if we set gin.Engine.ContextWithFallback to true. This makes gin.Context fully integrated
// 			with its underling Request.Context(). The side effect of this is gin.Context.Value() is also calling
// 			Request.Context().Value(), which cause stack overflow on non-existing keys.
//
//			To break this loop, we use different version of utils.ContextValuer to extract values from gin.Context(),
//			without using gin.Context.Value() function.
func GinContextMerger() gin.HandlerFunc {
	return func(gc *gin.Context) {
		ctx := utils.MakeMutableContext(gc.Request.Context(), ginContextValuer(gc))
		// Note, this is optional since Gin 1.8.0. We are doing this simply for performance
		ctx.Set(gin.ContextKey, gc)
		gc.Request = gc.Request.WithContext(ctx)
	}
}

// NewHttpGinHandlerFunc integrate http.HandlerFunc with GIN handler
func NewHttpGinHandlerFunc(handlerFunc http.HandlerFunc) gin.HandlerFunc {
	if handlerFunc == nil {
		panic(fmt.Errorf("cannot wrap a nil hanlder"))
	}

	handler := func(c *gin.Context) {
		c = preProcessGinContext(c)
		handlerFunc(c.Writer, c.Request)
	}
	return handler
}


func preProcessGinContext(gc *gin.Context) *gin.Context {
	// because of GinContextMerger is mandatory middleware for all mappings, we are sure gc.Request.Context() contains gin.Context.
	// So we only need to make sure it's also mutable
	rc := gc.Request.Context()
	ctx := utils.MakeMutableContext(rc)
	if ctx != rc {
		gc.Request = gc.Request.WithContext(ctx)
	}
	// note, we could also make a copy of gin context in case we want to use it out of request scope
	// but currently, we don't have such requirement
	return gc
}

/**************************
	helpers
 **************************/

func ginContextValuer(gc *gin.Context) func(key interface{}) interface{} {
	return func(key interface{}) interface{} {
		switch strKey, _ := key.(string); strKey {
		case gin.ContextKey:
			return gc
		default:
			v, _ := gc.Get(strKey)
			return v
		}
	}
}

/*********************************
	SimpleGinMapping
 *********************************/

// simpleGinMapping implements SimpleGinMapping
type simpleGinMapping struct {
	simpleMapping
	handlerFunc gin.HandlerFunc
}

// NewSimpleGinMapping create a SimpleGinMapping.
// It's recommended to use mapping.MappingBuilder instead of this function:
// e.g.
// <code>
// mapping.Post("/path/to/api").HandlerFunc(func...).Build()
// </code>
func NewSimpleGinMapping(name, group, path, method string, condition RequestMatcher, handlerFunc gin.HandlerFunc) SimpleGinMapping {
	return &simpleGinMapping{
		simpleMapping: *NewSimpleMapping(name, group, path, method, condition, nil).(*simpleMapping),
		handlerFunc:   handlerFunc,
	}
}

func (m simpleGinMapping) GinHandlerFunc() gin.HandlerFunc {
	if m.handlerFunc != nil {
		return m.handlerFunc
	}

	if m.simpleMapping.handlerFunc != nil {
		return NewHttpGinHandlerFunc(m.simpleMapping.handlerFunc)
	}
	return nil
}

/*********************************
	MiddlewareGinMapping
 *********************************/

// middlewareGinMapping implements MiddlewareGinMapping
type middlewareGinMapping struct {
	middlewareMapping
	handlerFunc gin.HandlerFunc
}

// NewMiddlewareGinMapping create a MiddlewareGinMapping with gin.HandlerFunc
// It's recommended to use middleware.MappingBuilder instead of this function:
// e.g.
// <code>
// middleware.NewBuilder("my-auth").Order(-10).Use(func...).Build()
// </code>
func NewMiddlewareGinMapping(name string, order int, matcher RouteMatcher, cond RequestMatcher, handlerFunc gin.HandlerFunc) MiddlewareGinMapping {
	return &middlewareGinMapping{
		middlewareMapping: *NewMiddlewareMapping(name, order, matcher, cond, nil).(*middlewareMapping),
		handlerFunc:       handlerFunc,
	}
}

func (m middlewareGinMapping) GinHandlerFunc() gin.HandlerFunc {
	if m.handlerFunc != nil {
		return m.handlerFunc
	}

	if m.middlewareMapping.handlerFunc != nil {
		return NewHttpGinHandlerFunc(m.middlewareMapping.handlerFunc)
	}
	return nil
}
