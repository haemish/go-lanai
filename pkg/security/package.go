package security

import (
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/bootstrap"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/web"
	"go.uber.org/fx"
)

var Module = &bootstrap.Module{
	Name: "security",
	Precedence: MaxSecurityPrecedence,
	Options: []fx.Option{
		fx.Provide(provideSecurityInitialization),
		fx.Invoke(initialize),
	},
}

func init() {
	bootstrap.Register(Module)
}

// Maker func, does nothing. Allow service to include this module in main()
func Use() {

}

/**************************
	Provider
***************************/
type dependencies struct {
	fx.In
	GlobalAuthenticator Authenticator `optional:"true"`
	// may be generic security properties
}

type global struct {
	fx.Out
	Initializer Initializer
	Registrar Registrar
}

// We let configurer.initializer can be autowired as both Initializer and Registrar
func provideSecurityInitialization(di dependencies) global {
	initializer := newSecurity(di.GlobalAuthenticator)
	return global{
		Initializer: initializer,
		Registrar: initializer,
	}
}

/**************************
	Initialize
***************************/
type initDI struct {
	fx.In
	Registerer  *web.Registrar
	Initializer Initializer
}

func initialize(di initDI) {
	if err := di.Initializer.Initialize(di.Registerer); err != nil {
		panic(err)
	}
}
