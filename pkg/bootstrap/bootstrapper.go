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

package bootstrap

import (
	"context"
	"github.com/cisco-open/go-lanai/pkg/utils"
	"go.uber.org/fx"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

/**************************
	Bootstrapper
 **************************/

var (
	once                 sync.Once
	bootstrapperInstance *Bootstrapper
)

type ContextOption func(ctx context.Context) context.Context

/**************************
	Singleton Pattern
 **************************/

// GlobalBootstrapper returns globally configured Bootstrapper.
// This bootstrapper is the one that being used by Execute, any package-level function works against this instance
func GlobalBootstrapper() *Bootstrapper {
	return bootstrapper()
}

func bootstrapper() *Bootstrapper {
	once.Do(func() {
		bootstrapperInstance = NewBootstrapper()
	})
	return bootstrapperInstance
}

func Register(m *Module) {
	bootstrapper().Register(m)
}

func AddOptions(options ...fx.Option) {
	bootstrapper().AddOptions(options...)
}

func AddInitialAppContextOptions(options ...ContextOption) {
	bootstrapper().AddInitialAppContextOptions(options...)
}

func AddStartContextOptions(options ...ContextOption) {
	bootstrapper().AddStartContextOptions(options...)
}

func AddStopContextOptions(options ...ContextOption) {
	bootstrapper().AddStopContextOptions(options...)
}

/**************************
	Bootstrapper
 **************************/

// Bootstrapper stores application configurations for bootstrapping
type Bootstrapper struct {
	modules      utils.GenericSet[*Module]
	adhocModule  *Module
	initCtxOpts  []ContextOption
	startCtxOpts []ContextOption
	stopCtxOpts  []ContextOption
}

// NewBootstrapper create a new Bootstrapper.
// Note: "bootstrap" package uses Singleton patterns for application bootstrap. Calling this function directly is not recommended
//
//	This function is exported for test packages to use
func NewBootstrapper() *Bootstrapper {
	return &Bootstrapper{
		modules:     utils.NewGenericSet[*Module](),
		adhocModule: newAnonymousModule(),
	}
}

func (b *Bootstrapper) Register(m *Module) {
	b.modules.Add(m)
}

func (b *Bootstrapper) AddOptions(options ...fx.Option) {
	b.adhocModule.Options = append(b.adhocModule.Options, options...)
}

func (b *Bootstrapper) AddInitialAppContextOptions(options ...ContextOption) {
	b.initCtxOpts = append(b.initCtxOpts, options...)
}

func (b *Bootstrapper) AddStartContextOptions(options ...ContextOption) {
	b.startCtxOpts = append(b.startCtxOpts, options...)
}

func (b *Bootstrapper) AddStopContextOptions(options ...ContextOption) {
	b.stopCtxOpts = append(b.stopCtxOpts, options...)
}

// EnableCliRunnerMode implements CliRunnerEnabler
func (b *Bootstrapper) EnableCliRunnerMode(runnerProviders ...interface{}) {
	enableCliRunnerMode(b, runnerProviders)
}

func (b *Bootstrapper) NewApp(cliCtx *CliExecContext, priorityOptions []fx.Option, regularOptions []fx.Option) *App {
	// create App
	app := &App{
		ctx:          NewApplicationContext(b.initCtxOpts...),
		startCtxOpts: b.startCtxOpts,
		stopCtxOpts:  b.stopCtxOpts,
	}

	// Decide default module
	initModule := InitModule(cliCtx, app)
	miscModules := MiscModules()

	// Decide ad-hoc fx options
	mainModule := newApplicationMainModule()
	for _, o := range priorityOptions {
		mainModule.PriorityOptions = append(mainModule.PriorityOptions, o)
	}

	for _, o := range regularOptions {
		mainModule.Options = append(mainModule.Options, o)
	}

	// Expand and resolve modules
	resolvedModules := b.modules.Copy()
	resolvedModules.Add(initModule, mainModule, b.adhocModule)
	resolvedModules.Add(miscModules...)
	for changed := true; changed;  {
		before := len(resolvedModules)
		for module := range resolvedModules {
			resolvedModules.Add(module.Modules...)
		}
		changed = before != len(resolvedModules)
	}
	modules := resolvedModules.Values()
	sort.SliceStable(modules, func(i, j int) bool { return modules[i].Precedence < modules[j].Precedence })

	// add priority options first
	var options []fx.Option
	for _, m := range modules {
		options = append(options, m.PriorityOptions...)
	}

	// add other options later
	for _, m := range modules {
		options = append(options, m.Options...)
	}

	// create fx.App, which will kick off all fx options
	app.App = fx.New(options...)
	return app
}

/**************************
	Application
 **************************/

type App struct {
	*fx.App
	ctx          *ApplicationContext
	startCtxOpts []ContextOption
	stopCtxOpts  []ContextOption
}

// EagerGetApplicationContext returns the global ApplicationContext before it becomes available for dependency injection
// Important: packages should typically get ApplicationContext via fx's dependency injection,
//
//	which internal application config are guaranteed.
//	Only packages involved in priority bootstrap (appconfig, consul, vault, etc)
//	should use this function for logging purpose
func (app *App) EagerGetApplicationContext() *ApplicationContext {
	return app.ctx
}

func (app *App) Run() {
	// to be revised:
	//  1. (Solved)	Support Timeout in bootstrap.Context
	//  2. (Solved) Restore logging
	var cancel context.CancelFunc
	done := app.Done()
	startCtx := app.ctx.Context
	for _, opt := range app.startCtxOpts {
		startCtx = opt(startCtx)
	}

	// This is so that we know that the context in the life cycle hook is the child of bootstrap context
	startCtx, cancel = context.WithTimeout(startCtx, app.StartTimeout())
	defer cancel()

	// log error and exit
	if err := app.Start(startCtx); err != nil {
		logger.WithContext(startCtx).Errorf("Failed to start up: %v", err)
		exit(1)
	}

	// this line blocks until application shutting down
	printSignal(<-done)

	// shutdown sequence
	stopCtx := context.WithValue(app.ctx.Context, ctxKeyStopTime, time.Now().UTC())
	for _, opt := range app.stopCtxOpts {
		stopCtx = opt(stopCtx)
	}

	stopCtx, cancel = context.WithTimeout(stopCtx, app.StopTimeout())
	defer cancel()

	if err := app.Stop(stopCtx); err != nil {
		logger.WithContext(stopCtx).Errorf("Shutdown with Error: %v", err)
		exit(1)
	}
}

func printSignal(signal os.Signal) {
	logger.Infof(strings.ToUpper(signal.String()))
}

func exit(code int) {
	os.Exit(code)
}
