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

package tracing

import (
	"context"
	"github.com/cisco-open/go-lanai/pkg/bootstrap"
	"github.com/cisco-open/go-lanai/pkg/log"
	"github.com/cisco-open/go-lanai/pkg/redis"
	"github.com/cisco-open/go-lanai/pkg/scheduler"
	"github.com/cisco-open/go-lanai/pkg/tracing"
	"github.com/cisco-open/go-lanai/pkg/tracing/instrument"
	"github.com/cisco-open/go-lanai/pkg/web"
	"github.com/opentracing/opentracing-go"
	"go.uber.org/fx"
)

var logger = log.New("Tracing")

var Module = &bootstrap.Module{
	Name:       "Tracing",
	Precedence: bootstrap.TracingPrecedence,
	PriorityOptions: []fx.Option{
		fx.Provide(tracing.BindTracingProperties),
		fx.Provide(provideTracer),
		fx.Provide(instrument.CliRunnerTracingProvider()),
		fx.Provide(instrument.KafkaTracingTracingProvider()),
		fx.Invoke(initialize),
	},
}

type TracerClosingHook *fx.Hook

var defaultTracerCloser fx.Hook

type kCtxDefaultTracerCloser struct {}

// Use does nothing. Allow service to include this module in main()
func Use() {
	bootstrap.Register(Module)
	EnableBootstrapTracing(bootstrap.GlobalBootstrapper())
}

// EnableBootstrapTracing enable bootstrap tracing on a given bootstrapper.
// bootstrap.GlobalBootstrapper() should be used for regular application that uses bootstrap.Execute()
func EnableBootstrapTracing(bootstrapper *bootstrap.Bootstrapper) {
	// logger extractor
	log.RegisterContextLogFields(tracing.TracingLogValuers)

	appTracer, closer := tracing.NewDefaultTracer()
	bootstrapper.AddInitialAppContextOptions(instrument.MakeBootstrapTracingOption(appTracer, tracing.OpNameBootstrap))
	bootstrapper.AddStartContextOptions(instrument.MakeStartTracingOption(appTracer, tracing.OpNameStart))
	bootstrapper.AddStopContextOptions(instrument.MakeStopTracingOption(appTracer, tracing.OpNameStop))
	defaultTracerCloser = fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.WithContext(ctx).Infof("closing default Tracer...")
			e := closer.Close()
			if e != nil {
				logger.WithContext(ctx).Errorf("failed to close default Tracer: %v", e)
			}
			logger.WithContext(ctx).Infof("default Tracer closed")
			return e
		},
	}
	bootstrapper.AddInitialAppContextOptions(func(ctx context.Context) context.Context {
		return context.WithValue(ctx, kCtxDefaultTracerCloser{}, defaultTracerCloser)
	})
}

/**************************
	Provide dependencies
***************************/
type tracerOut struct {
	fx.Out
	Tracer opentracing.Tracer
	FxHook TracerClosingHook
}

func provideTracer(ctx *bootstrap.ApplicationContext, props tracing.TracingProperties) (ret tracerOut) {
	ret = tracerOut{
		Tracer: opentracing.NoopTracer{},
	}

	if !props.Enabled {
		return
	}

	tracers := make([]opentracing.Tracer, 0, 2)
	if props.Jaeger.Enabled {
		tracer, closer := tracing.NewJaegerTracer(ctx, &props.Jaeger, &props.Sampler)
		tracers = append(tracers, tracer)
		ret.FxHook = &fx.Hook{
			OnStop: func(ctx context.Context) error {
				logger.WithContext(ctx).Infof("closing Jaeger Tracer...")
				e := closer.Close()
				if e != nil {
					logger.WithContext(ctx).Errorf("failed to close Jaeger Tracer: %v", e)
				}
				logger.WithContext(ctx).Infof("Jaeger Tracer closed")
				return e
			},
		}
	}

	if props.Zipkin.Enabled {
		panic("zipkin is currently unsupported")
	}

	switch len(tracers) {
	case 0:
		return
	case 1:
		ret.Tracer = tracers[0]
		return
	default:
		panic("multiple opentracing.Tracer detected. we currely only support single tracer")
	}
}

/**************************
	Setup
***************************/
type regDI struct {
	fx.In
	AppContext   *bootstrap.ApplicationContext
	Tracer       opentracing.Tracer  `optional:"true"`
	FxHook       TracerClosingHook   `optional:"true"`
	Registrar    *web.Registrar      `optional:"true"`
	RedisFactory redis.ClientFactory `optional:"true"`
	// we could include security configurations, customizations here
}

func initialize(lc fx.Lifecycle, di regDI) {
	if di.Tracer == nil {
		return
	}

	// web instrumentation
	if di.Registrar != nil {
		customizer := instrument.NewTracingWebCustomizer(di.Tracer)
		if e := di.Registrar.Register(customizer); e != nil {
			panic(e)
		}
	}

	// redis instrumentation
	if di.RedisFactory != nil {
		hook := instrument.NewRedisTrackingHook(di.Tracer)
		di.RedisFactory.AddHooks(di.AppContext, hook)
	}

	// scheduler instrumentation
	scheduler.AddDefaultHook(instrument.NewTracingTaskHook(di.Tracer))

	// graceful closer
	if di.FxHook != nil {
		lc.Append(*di.FxHook)
		if defaultCloserFromCtx, ok := di.AppContext.Value(kCtxDefaultTracerCloser{}).(fx.Hook); ok {
			lc.Append(defaultCloserFromCtx)
		} else {
			lc.Append(defaultTracerCloser)
		}
	}
}
