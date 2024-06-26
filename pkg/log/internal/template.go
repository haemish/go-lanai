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

package internal

import (
	"bytes"
	"github.com/cisco-open/go-lanai/pkg/utils"
	"go.uber.org/zap/buffer"
	"io"
	"strings"
	"text/template"
)

const (
	LogKeyMessage    = "msg"
	LogKeyName       = "logger"
	LogKeyTimestamp  = "time"
	LogKeyCaller     = "caller"
	LogKeyLevel      = "level"
	LogKeyContext    = "ctx"
	LogKeyStacktrace = "stacktrace"
)

const (
	logTemplate = "lanai-log-template"
)

type Fields map[string]interface{}

type TextFormatter interface {
	Format(kvs Fields, w io.Writer) error
}

type TemplatedFormatter struct {
	text        string
	tmpl        *template.Template
	fixedFields utils.StringSet
	isTerm      bool
}

func NewTemplatedFormatter(tmpl string, fixedFields utils.StringSet, isTerm bool) *TemplatedFormatter {
	formatter := &TemplatedFormatter{
		text:        tmpl,
		fixedFields: fixedFields,
		isTerm:      isTerm,
	}
	formatter.init()
	return formatter
}

func (f *TemplatedFormatter) init() {
	if !strings.HasSuffix(f.text, "\n") {
		f.text = f.text + "\n"
	}

	funcMap := TmplFuncMapNonTerm
	colorFuncMap := TmplColorFuncMapNonTerm
	if f.isTerm {
		colorFuncMap = TmplFuncMap
		funcMap = TmplColorFuncMap
	}

	t, e := template.New(logTemplate).
		Option("missingkey=zero").
		Funcs(funcMap).
		Funcs(colorFuncMap).
		Funcs(template.FuncMap{
			"kv": MakeKVFunc(f.fixedFields),
		}).
		Parse(f.text)
	if e != nil {
		panic(e)
	}
	f.tmpl = t
}

func (f *TemplatedFormatter) Format(kvs Fields, w io.Writer) error {
	switch w.(type) {
	case *buffer.Buffer:
		return f.tmpl.Execute(w, kvs)
	default:
		// from documents of template.Template.Execute:
		// 		A template may be executed safely in parallel, although if parallel
		// 		executions share a Writer the output may be interleaved.
		// to prevent this from happening, we use an in-memory buffer. Hopefully this is faster than mutex locking
		var buf bytes.Buffer
		if e := f.tmpl.Execute(&buf, kvs); e != nil {
			return e
		}
		if _, e := w.Write(buf.Bytes()); e != nil {
			return e
		}
		return nil
	}
}
