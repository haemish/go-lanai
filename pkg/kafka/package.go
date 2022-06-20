package kafka

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/actuator/health"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/bootstrap"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/log"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/tlsconfig"
	"go.uber.org/fx"
)

var logger = log.New("Kafka")

var Module = &bootstrap.Module{
	Precedence: bootstrap.KafkaPrecedence,
	Options: []fx.Option{
		fx.Provide(BindKafkaProperties, ProvideKafkaBinder),
		fx.Invoke(initialize),
	},
}

const (
	FxGroup = "kafka"
)

// Use Allow service to include this module in main()
func Use() {
	bootstrap.Register(tlsconfig.Module)
	bootstrap.Register(Module)
}

type binderDI struct {
	fx.In
	AppContext           *bootstrap.ApplicationContext
	Properties           KafkaProperties
	ProducerInterceptors []ProducerMessageInterceptor  `group:"kafka"`
	ConsumerInterceptors []ConsumerDispatchInterceptor `group:"kafka"`
	HandlerInterceptors  []ConsumerHandlerInterceptor  `group:"kafka"`
	TlsProviderFactory   *tlsconfig.ProviderFactory
}

func ProvideKafkaBinder(di binderDI) Binder {
	return NewBinder(di.AppContext, func(opt *BinderOption) {
		*opt = BinderOption{
			ApplicationConfig:    di.AppContext.Config(),
			Properties:           di.Properties,
			ProducerInterceptors: append(opt.ProducerInterceptors, di.ProducerInterceptors...),
			ConsumerInterceptors: append(opt.ConsumerInterceptors, di.ConsumerInterceptors...),
			HandlerInterceptors:  append(opt.HandlerInterceptors, di.HandlerInterceptors...),
			TlsProviderFactory:   di.TlsProviderFactory,
		}
	})
}

type initDI struct {
	fx.In
	AppCtx          *bootstrap.ApplicationContext
	Lifecycle       fx.Lifecycle
	Properties      KafkaProperties
	Binder          Binder
	HealthRegistrar health.Registrar `optional:"true"`
}

func initialize(di initDI) {
	// register lifecycle functions
	di.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return di.Binder.(BinderLifecycle).Start(di.AppCtx)
		},
		OnStop:  di.Binder.(BinderLifecycle).Shutdown,
	})

	// register health endpoints if applicable
	if di.HealthRegistrar == nil {
		return
	}

	di.HealthRegistrar.MustRegister(&HealthIndicator{
		binder: di.Binder.(SaramaBinder),
	})
}
