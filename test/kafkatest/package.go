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

package kafkatest

import (
	"github.com/cisco-open/go-lanai/pkg/log"
	"github.com/cisco-open/go-lanai/test"
	"github.com/cisco-open/go-lanai/test/apptest"
	"go.uber.org/fx"
)

var logger = log.New("KafkaTest")
var messageLogger = msgLogger{
	logger: logger,
	level: log.LevelInfo,
}

// WithMockedBinder returns a test.Options that provides mocked kafka.Binder and a MessageRecorder.
// Tests can wire the MessageRecorder and verify invocation of kafka.Producer
// Note: The main purpose of this test configuration is to fulfill dependency injection and validate kafka.Producer is
//		 invoked as expected. It doesn't validate/invoke any message options such as ValueEncoder or Key, nor does it
//		 respect any binding configuration
func WithMockedBinder() test.Options {
	testOpts := []test.Options{
		apptest.WithFxOptions(
			fx.Provide(provideMockedBinder),
		),
	}
	return test.WithOptions(testOpts...)
}



