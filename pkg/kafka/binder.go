package kafka

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/bootstrap"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/utils/loop"
	"fmt"
	"github.com/Shopify/sarama"
	"go.uber.org/fx"
	"math"
	"strings"
	"sync"
	"time"
)

type SaramaKafkaBinder struct {
	appConfig            bootstrap.ApplicationConfig
	properties           *KafkaProperties
	brokers              []string
	initOnce             sync.Once
	startOnce            sync.Once
	defaults             bindingConfig
	producerInterceptors []ProducerMessageInterceptor
	consumerInterceptors []ConsumerDispatchInterceptor
	handlerInterceptors  []ConsumerHandlerInterceptor
	monitor              *loop.Loop

	globalClient      sarama.Client
	adminClient       sarama.ClusterAdmin
	provisioner       *saramaTopicProvisioner
	producers         map[string]BindingLifecycle
	subscribers       map[string]BindingLifecycle
	consumerGroups    map[string]BindingLifecycle
	monitorCancelFunc context.CancelFunc
}

type factoryDI struct {
	fx.In
	AppContext           *bootstrap.ApplicationContext
	Properties           KafkaProperties
	ProducerInterceptors []ProducerMessageInterceptor  `group:"kafka"`
	ConsumerInterceptors []ConsumerDispatchInterceptor `group:"kafka"`
	HandlerInterceptors  []ConsumerHandlerInterceptor  `group:"kafka"`
}

func NewKafkaBinder(di factoryDI) Binder {
	s := &SaramaKafkaBinder{
		appConfig:  di.AppContext.Config(),
		properties: &di.Properties,
		brokers:    di.Properties.Brokers,
		producerInterceptors: append(di.ProducerInterceptors,
			mimeTypeProducerInterceptor{},
		),
		consumerInterceptors: di.ConsumerInterceptors,
		handlerInterceptors:  di.HandlerInterceptors,
		monitor:              loop.NewLoop(),
		producers:            make(map[string]BindingLifecycle),
		subscribers:          make(map[string]BindingLifecycle),
		consumerGroups:       make(map[string]BindingLifecycle),
	}

	if e := s.Initialize(context.Background()); e != nil {
		panic(e)
	}
	return s
}

func (b *SaramaKafkaBinder) prepareDefaults(ctx context.Context, saramaDefaults *sarama.Config) {
	b.defaults = bindingConfig{
		name:       "default",
		properties: BindingProperties{},
		sarama:     *saramaDefaults,
		msgLogger:  newSaramaMessageLogger(),
		producer: producerConfig{
			keyEncoder:   binaryEncoder{},
			interceptors: b.producerInterceptors,
			provisioning: topicConfig{
				autoCreateTopic:      true,
				autoAddPartitions:    true,
				allowLowerPartitions: true,
				partitionCount:       1,
				replicationFactor:    1,
			},
		},
		consumer: consumerConfig{
			dispatchInterceptors: b.consumerInterceptors,
			handlerInterceptors:  b.handlerInterceptors,
			msgLogger:            newSaramaMessageLogger(),
		},
	}

	// try load default properties
	if e := b.appConfig.Bind(&b.defaults.properties, ConfigKafkaDefaultBindingPrefix); e != nil {
		logger.WithContext(ctx).Infof("default kafka binding properties [%s.*] is not configured")
	}
}

func (b *SaramaKafkaBinder) Produce(topic string, options ...ProducerOptions) (Producer, error) {
	if _, ok := b.producers[topic]; ok {
		logger.Warnf("producer for topic %s already exist. please use the existing instance", topic)
		return nil, NewKafkaError(ErrorCodeProducerExists, "producer for topic %s already exist", topic)
	}

	// apply defaults and options
	cfg := b.defaults // make a copy
	cfg.name = strings.ToLower(topic)
	for _, optionFunc := range options {
		optionFunc(&cfg)
	}

	// load and apply properties
	props := b.loadProperties(cfg.name)
	WithProducerProperties(&props.Producer)(&cfg)

	if e := b.provisioner.provisionTopic(topic, &cfg); e != nil {
		return nil, e
	}

	p, err := newSaramaProducer(topic, b.brokers, &cfg)

	if err != nil {
		return nil, err
	} else {
		b.producers[topic] = p
		return p, nil
	}
}

func (b *SaramaKafkaBinder) Subscribe(topic string, options ...ConsumerOptions) (Subscriber, error) {
	if _, ok := b.subscribers[topic]; ok {
		logger.Warnf("subscriber for topic %s already exist. please use the existing instance", topic)
		return nil, NewKafkaError(ErrorCodeConsumerExists, "producer for topic %s already exist", topic)
	}

	// apply defaults and options
	cfg := b.defaults // make a copy
	cfg.name = strings.ToLower(topic)
	for _, optionFunc := range options {
		optionFunc(&cfg)
	}

	// load and apply properties
	props := b.loadProperties(cfg.name)
	WithConsumerProperties(&props.Consumer)(&cfg)

	sub, err := newSaramaSubscriber(topic, b.brokers, &cfg, b.provisioner)
	if err != nil {
		return nil, err
	}

	b.subscribers[topic] = sub
	return sub, nil
}

func (b *SaramaKafkaBinder) Consume(topic string, group string, options ...ConsumerOptions) (GroupConsumer, error) {
	if _, ok := b.consumerGroups[topic]; ok {
		logger.Warnf("consumer group for topic %s already exist. please use the existing instance", topic)
		return nil, NewKafkaError(ErrorCodeConsumerExists, "producer for topic %s already exist", topic)
	}

	// apply defaults and options
	cfg := b.defaults // make a copy
	cfg.name = strings.ToLower(topic)
	for _, optionFunc := range options {
		optionFunc(&cfg)
	}

	// load and apply properties
	props := b.loadProperties(cfg.name)
	WithConsumerProperties(&props.Consumer)(&cfg)

	cg, err := newSaramaGroupConsumer(topic, group, b.brokers, &cfg, b.provisioner)
	if err != nil {
		return nil, err
	}

	b.consumerGroups[topic] = cg
	return cg, nil
}

func (b *SaramaKafkaBinder) ListTopics() (topics []string) {
	topics = make([]string, 0, len(b.producers)+len(b.subscribers)+len(b.consumerGroups))
	for t := range b.producers {
		topics = append(topics, t)
	}
	for t := range b.subscribers {
		topics = append(topics, t)
	}
	for t := range b.consumerGroups {
		topics = append(topics, t)
	}
	return topics
}

func (b *SaramaKafkaBinder) Client() sarama.Client {
	return b.globalClient
}

// Initialize implements BinderLifecycle, prepare for use, negotiate default configs, etc.
func (b *SaramaKafkaBinder) Initialize(ctx context.Context) (err error) {
	b.initOnce.Do(func() {
		cfg := defaultSaramaConfig(b.properties)

		// prepare defaults
		b.prepareDefaults(ctx, cfg)

		// create a global client
		b.globalClient, err = sarama.NewClient(b.brokers, cfg)
		if err != nil {
			err = NewKafkaError(ErrorCodeBrokerNotReachable, fmt.Sprintf("unable to connect to Kafka brokers %v: %v", b.brokers, err), err)
			return
		}

		b.adminClient, err = sarama.NewClusterAdmin(b.brokers, cfg)
		if err != nil {
			err = NewKafkaError(ErrorCodeBrokerNotReachable, fmt.Sprintf("unable to connect to Kafka brokers %v: %v", b.brokers, err), err)
			return
		}

		b.provisioner = &saramaTopicProvisioner{
			globalClient: b.globalClient,
			adminClient:  b.adminClient,
		}
	})

	return
}

// Start implements BinderLifecycle, start all bindings if not started yet (Producer, Subscriber, GroupConsumer, etc).
func (b *SaramaKafkaBinder) Start(_ context.Context) (err error) {
	b.startOnce.Do(func() {
		var loopCtx context.Context
		loopCtx, b.monitorCancelFunc = b.monitor.Run(context.Background())
		b.monitor.Repeat(b.tryStartTaskFunc(loopCtx), func(opt *loop.TaskOption) {
			opt.RepeatIntervalFunc = b.tryStartRepeatIntervalFunc()
		})
	})
	return nil
}

// Shutdown implements BinderLifecycle, close resources
func (b *SaramaKafkaBinder) Shutdown(ctx context.Context) error {
	logger.WithContext(ctx).Infof("Kafka shutting down")

	logger.WithContext(ctx).Debugf("stopping binding watchdog...")
	if b.monitorCancelFunc != nil {
		b.monitorCancelFunc()
	}

	logger.WithContext(ctx).Debugf("closing producers...")
	for _, p := range b.producers {
		if e := p.Close(); e != nil {
			// since application is shutting down, we just log the error
			logger.WithContext(ctx).Errorf("error while closing kafka producer: %v", e)
		}
	}

	logger.WithContext(ctx).Debugf("closing subscribers...")
	for _, p := range b.subscribers {
		if e := p.Close(); e != nil {
			// since application is shutting down, we just log the error
			logger.WithContext(ctx).Errorf("error while closing kafka subscriber: %v", e)
		}
	}

	logger.WithContext(ctx).Debugf("closing group consumers...")
	for _, p := range b.consumerGroups {
		if e := p.Close(); e != nil {
			// since application is shutting down, we just log the error
			logger.WithContext(ctx).Errorf("error while closing kafka consumer: %v", e)
		}
	}

	logger.WithContext(ctx).Debugf("closing connections...")
	if e := b.adminClient.Close(); e != nil {
		logger.WithContext(ctx).Errorf("error while closing kafka admin client: %v", e)
	}

	if e := b.globalClient.Close(); e != nil {
		logger.WithContext(ctx).Errorf("error while closing kafka global client: %v", e)
	}

	logger.WithContext(ctx).Infof("Kafka connections closed")
	return nil
}

// loadProperties load properties for particular topic
func (b *SaramaKafkaBinder) loadProperties(name string) *BindingProperties {
	prefix := ConfigKafkaBindingPrefix + "." + strings.ToLower(name)
	props := b.defaults.properties // make a copy
	if e := b.appConfig.Bind(&props, prefix); e != nil {
		props = b.defaults.properties // make a fresh copy
	}
	return &props
}

// tryStartTaskFunc try to start any registered bindings if it's not started yet
// this task should be run periodically to perform delayed start of any Subscriber or GroupConsumer
func (b *SaramaKafkaBinder) tryStartTaskFunc(loopCtx context.Context) loop.TaskFunc {
	return func(_ context.Context, l *loop.Loop) (ret interface{}, err error) {
		// we cannot use passed-in context, because this context will be cancelled as soon as this function finishes
		allStarted := true
		for _, lc := range b.producers {
			if e := lc.Start(loopCtx); e != nil {
				allStarted = false
			}
		}

		for _, lc := range b.subscribers {
			if e := lc.Start(loopCtx); e != nil {
				allStarted = false
			}
		}

		for _, lc := range b.consumerGroups {
			if e := lc.Start(loopCtx); e != nil {
				allStarted = false
			}
		}

		return allStarted, nil
	}
}

// tryStartRepeatIntervalFunc decide repeat rate of tryStartTaskFunc
// we repeat fast at beginning
// when all bindings are successfully started, we reduce the repeating rate
// S-shaped curve.
// Logistic Function 	https://en.wikipedia.org/wiki/Logistic_function
//						https://en.wikipedia.org/wiki/Generalised_logistic_function
func (b *SaramaKafkaBinder) tryStartRepeatIntervalFunc() loop.RepeatIntervalFunc {

	var fn func(int) time.Duration
	n := -1

	min := float64(b.properties.Binder.InitialHeartbeat)
	max := math.Max(min, float64(b.properties.Binder.WatchdogHeartbeat))
	mid := math.Max(1, b.properties.Binder.HeartbeatCurveMidpoint)
	k := math.Max(0.2, b.properties.Binder.HeartbeatCurveFactor)

	if float64(time.Minute) < max-min && mid >= 5 {
		fn = b.logisticModel(min, max, k, mid, time.Second)
	} else {
		fn = b.linearModel(min, max, mid)
	}

	return func(result interface{}, err error) time.Duration {
		switch allStarted := result.(type) {
		case bool:
			if allStarted {
				return time.Duration(b.properties.Binder.WatchdogHeartbeat)
			} else {
				ret := fn(n)
				n = n + 1
				//logger.Debugf("retry bindings in %dms", ret.Milliseconds())
				return ret
			}
		default:
			return time.Duration(b.properties.Binder.WatchdogHeartbeat)
		}
	}
}

// logisticModel returns delay function f(n) using logistic model
// Logistic Function 	https://en.wikipedia.org/wiki/Logistic_function
//						https://en.wikipedia.org/wiki/Generalised_logistic_function
func (b *SaramaKafkaBinder) logisticModel(min, max, k, n0 float64, y0 time.Duration) func(n int) time.Duration {
	// minK is calculated to make sure f(0) < min + y0 (first value is within y0 seconds of min value)
	minK := math.Log((max-min)/float64(y0)-1) / n0
	k = math.Max(k, minK)
	return func(n int) time.Duration {
		if n < 0 {
			return time.Duration(min)
		}
		return time.Duration((max-min)/(1+math.Exp(-k*(float64(n)-n0))) + min)
	}
}

// logisticModel returns delay function f(n) using linear model
func (b *SaramaKafkaBinder) linearModel(min, max, n0 float64) func(n int) time.Duration {
	return func(n int) time.Duration {
		return time.Duration((max-min)/n0/2*float64(n) + min)
	}
}