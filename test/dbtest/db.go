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

package dbtest

import (
	"context"
	"flag"
	"fmt"
	"github.com/cisco-open/go-lanai/test"
	"github.com/cisco-open/go-lanai/test/apptest"
	"github.com/cockroachdb/copyist"
	"go.uber.org/fx"
	"io"
	"sync"
	"testing"
)

const (
	flagCopyistRecordMode = "record"
)

type copyistCK struct{}

var (
	ctxKeyCopyistCloser = copyistCK{}
	regOnce             = sync.Once{}
)

const (
	dsKeyHost     = "host"
	dsKeyPort     = "port"
	dsKeyDB       = "dbname"
	dsKeySslMode  = "sslmode"
	dsKeyUsername = "user"
	dsKeyPassword = "password"
)

func withDB(mode mode, dbName string, opts []DBOptions) []test.Options {
	setCopyistModeFlag(mode)

	// prepare options
	opt := DBOption{
		Host:     "127.0.0.1",
		Port:     26257,
		DBName:   dbName,
		Username: "root",
	}
	for _, fn := range opts {
		fn(&opt)
	}

	return []test.Options{
		test.Setup(initializePostgresMock()),
		test.Setup(openCopyistConn(&opt)),
		test.Teardown(closeCopyistConn()),
		apptest.WithFxOptions(
			fx.Provide(testGormDialectorProvider(&opt)),
		),
		apptest.WithProperties(
			fmt.Sprintf("data.db.host: %s", opt.Host),
			fmt.Sprintf("data.db.port: %d", opt.Port),
			fmt.Sprintf("data.db.database: %s", opt.DBName),
			fmt.Sprintf("data.db.username: %s", opt.Username),
			fmt.Sprintf("data.db.password: %s", opt.Password),
		),
	}
}

func initializePostgresMock() test.SetupFunc {
	return func(ctx context.Context, t *testing.T) (context.Context, error) {
		regOnce.Do(func() {
			copyist.Register("postgres")
		})
		return ctx, nil
	}
}

func setCopyistModeFlag(mode mode) {
	switch mode {
	case modePlayback:
		mustSetFlag(flagCopyistRecordMode, "false")
	case modeRecord:
		mustSetFlag(flagCopyistRecordMode, "true")
	default:
	}
}

func openCopyistConn(opt *DBOption) test.SetupFunc {
	return func(ctx context.Context, t *testing.T) (context.Context, error) {
		if opt.DBName == "" {
			return nil, fmt.Errorf("DBName is required for recording mode")
		}

		closer := copyist.Open(t)
		if closer == nil {
			return ctx, nil
		}
		return context.WithValue(ctx, ctxKeyCopyistCloser, closer), nil
	}
}

func closeCopyistConn() test.TeardownFunc {
	return func(ctx context.Context, t *testing.T) error {
		switch v := ctx.Value(ctxKeyCopyistCloser).(type) {
		case io.Closer:
			return v.Close()
		}
		return nil
	}
}

func mustSetFlag(name, value string) {
	e := flag.Set(name, value)
	if e != nil {
		panic(e)
	}
}
