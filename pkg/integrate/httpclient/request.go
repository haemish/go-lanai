package httpclient

import (
	"bytes"
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/web"
	"encoding/base64"
	"encoding/json"
	"fmt"
	httptransport "github.com/go-kit/kit/transport/http"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type RequestOptions func(r *Request)

// Request is wraps all information about the request
type Request struct {
	Path       string
	Method     string
	Params     map[string]string
	Headers    http.Header
	Body       interface{}
	EncodeFunc httptransport.EncodeRequestFunc
}

func NewRequest(path, method string, opts ...RequestOptions) *Request {
	r := Request{
		Path:       path,
		Method:     method,
		Params:     map[string]string{},
		Headers:    http.Header{},
		EncodeFunc: EncodeJSONRequest,
	}
	for _, f := range opts {
		f(&r)
	}
	return &r
}

func EncodeJSONRequest(c context.Context, r *http.Request, request interface{}) error {
	if request == nil {
		r.Body = nil
		r.GetBody = nil
		r.ContentLength = 0
		return nil
	}
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	if headerer, ok := request.(web.Headerer); ok {
		for k := range headerer.Headers() {
			r.Header.Set(k, headerer.Headers().Get(k))
		}
	}
	var b bytes.Buffer
	r.Body = io.NopCloser(&b)
	err := json.NewEncoder(&b).Encode(request)
	if err != nil {
		return err
	}

	buf := b.Bytes()
	r.GetBody = func() (io.ReadCloser, error) {
		r := bytes.NewReader(buf)
		return io.NopCloser(r), nil
	}
	r.ContentLength = int64(b.Len())
	return nil
}

func effectiveEncodeFunc(ctx context.Context, req *http.Request, val interface{}) error {
	var r *Request
	switch v := val.(type) {
	case *Request:
		r = v
	case Request:
		r = &v
	default:
		return NewRequestSerializationError(fmt.Errorf("request encoder expects *Request but got %T", val))
	}

	// set headers
	for k := range r.Headers {
		req.Header.Set(k, r.Headers.Get(k))
	}

	// set params
	applyParams(req, r.Params)

	return r.EncodeFunc(ctx, req, r.Body)
}

func WithoutHeader(key string) RequestOptions {
	switch {
	case key == "":
		return noop()
	default:
		return func(r *Request) {
			r.Headers.Del(key)
		}
	}
}

func WithHeader(key, value string) RequestOptions {
	switch {
	case key == "" || value == "":
		return noop()
	default:
		return func(r *Request) {
			r.Headers.Add(key, value)
		}
	}
}

func WithParam(key, value string) RequestOptions {
	switch {
	case key == "":
		return noop()
	case value == "":
		return func(r *Request) {
			delete(r.Params, key)
		}
	default:
		return func(r *Request) {
			r.Params[key] = value
		}
	}
}

func WithBody(body interface{}) RequestOptions {
	return func(r *Request) {
		r.Body = body
	}
}

func WithRequestEncodeFunc(enc httptransport.EncodeRequestFunc) RequestOptions {
	return func(r *Request) {
		r.EncodeFunc = enc
	}
}

func WithBasicAuth(username, password string) RequestOptions {
	raw := username + ":" + password
	b64 := base64.StdEncoding.EncodeToString([]byte(raw))
	auth := "Basic " + b64
	return WithHeader(HeaderAuthorization, auth)
}

func WithUrlEncodedBody(body url.Values) RequestOptions {
	return func(r *Request) {
		r.Headers.Set(HeaderContentType, MediaTypeFormUrlEncoded)
		r.Body = body
		r.EncodeFunc = urlEncodedBodyEncoder
	}
}

func urlEncodedBodyEncoder(_ context.Context, r *http.Request, v interface{}) error {
	values, ok := v.(url.Values)
	if !ok {
		return NewRequestSerializationError(fmt.Errorf("www-form-urlencoded body expects url.Values but got %T", v))
	}
	reader := strings.NewReader(values.Encode())
	r.Body = io.NopCloser(reader)
	return nil
}

func applyParams(req *http.Request, params map[string]string) {
	if len(params) == 0 {
		return
	}

	queries := make([]string, len(params))
	i := 0
	for k, v := range params {
		queries[i] = k + "=" + url.QueryEscape(v)
		i++
	}
	req.URL.RawQuery = strings.Join(queries, "&")
}

func noop() func(r *Request) {
	return func(_ *Request) {
		// noop
	}
}
