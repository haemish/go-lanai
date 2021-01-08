package security

import (
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/bootstrap"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/utils/order"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/web"
)

const (
	MinSecurityPrecedence = bootstrap.FrameworkModulePrecedence + 2000
	MaxSecurityPrecedence = bootstrap.FrameworkModulePrecedence + 2999
)

const (
	ContextKeySecurity = web.ContextKeySecurity
)

const (
	WSSharedKeyCompositeAuthSuccessHandler = "CompositeAuthSuccessHandler"
	WSSharedKeyCompositeAuthErrorHandler = "CompositeAuthErrorHandler"
	WSSharedKeyCompositeAccessDeniedHandler = "CompositeAccessDeniedHandler"
	WSSharedKeySessionStore = "SessionStore"
	WSSharedKeyRequestPreProcessors = "RequestPreProcessors"
)

// Middleware Orders
const (
	_ = iota
	MWOrderSessionHandling = HighestMiddlewareOrder + iota * 20
	MWOrderAuthPersistence
	MWOrderErrorHandling
	MWOrderCsrfHandling
	MWOrderBasicAuth
	MWOrderFormLogout
	MWOrderFormAuth
	// ... TODO more MW goes here
	MWOrderAccessControl = LowestMiddlewareOrder - 200
)

// Feature Orders, if feature is not listed here, it's unordered. Unordered features are applied at last
const (
	_ = iota
	FeatureOrderAuthenticator = iota * 100
	FeatureOrderBasicAuth
	FeatureOrderMFA
	FeatureOrderFormLogin
	FeatureOrderLogout
	FeatureOrderCsrf
	FeatureOrderAccess
	FeatureOrderSession
	FeatureOrderRequestCache
	// ... TODO more Feature goes here
	FeatureOrderErrorHandling = order.Lowest - 200
)

// AuthenticationSuccessHandler Orders, if not listed here, it's unordered. Unordered handlers are applied at last
const (
	_ = iota
	HandlerOrderChangeSession = iota * 100
	HandlerOrderConcurrentSession

)

// CSRF headers and parameter names - shared by CSRF feature and session feature's request cache
const CsrfParamName = "_csrf"
const CsrfHeaderName = "X-CSRF-TOKEN"
