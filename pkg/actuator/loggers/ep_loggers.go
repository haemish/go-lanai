package loggers

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/actuator"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/log"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/web"
	"fmt"
	"net/http"
	"strings"
)

const (
	ID              = "loggers"
	EnableByDefault = true
)

var (
	allLevels = []log.LoggingLevel{
		log.LevelOff, log.LevelDebug, log.LevelInfo, log.LevelWarn, log.LevelError,
	}
)

type ReadInput struct {
	Name string `uri:"name"`
}

type WriteInput struct {
	Prefix string `uri:"name" binding:"required"`
	ConfiguredLevel *log.LoggingLevel `json:"configuredLevel"`
}

type ReadOutput struct {
	Levels  []log.LoggingLevel     `json:"levels"`
	Loggers map[string]LoggerLevel `json:"loggers"`
}

type LoggerLevel struct {
	EffectiveLevel  *log.LoggingLevel  `json:"effectiveLevel,omitempty"`
	ConfiguredLevel *log.LoggingLevel `json:"configuredLevel,omitempty"`
}

// LoggersEndpoint implements actuator.Endpoint, actuator.WebEndpoint
//goland:noinspection GoNameStartsWithPackageName
type LoggersEndpoint struct {
	actuator.WebEndpointBase
	pathSuffix map[actuator.Operation]string
}

func newEndpoint(di regDI) *LoggersEndpoint {
	ep := LoggersEndpoint{}
	ep.pathSuffix = map[actuator.Operation]string{
		actuator.NewReadOperation(ep.ReadAll):    "",
		actuator.NewReadOperation(ep.ReadAll):    "/",
		actuator.NewReadOperation(ep.ReadByName): "/:name",
		actuator.NewWriteOperation(ep.Write):     "/:name",
	}
	ops := make([]actuator.Operation, 0, len(ep.pathSuffix))
	for k := range ep.pathSuffix {
		ops = append(ops, k)
	}
	ep.WebEndpointBase = actuator.MakeWebEndpointBase(func(opt *actuator.EndpointOption) {
		opt.Id = ID
		opt.Ops = ops
		opt.Properties = &di.MgtProperties.Endpoints
		opt.EnabledByDefault = EnableByDefault
	})
	return &ep
}

// Mappings implements WebEndpoint
func (ep *LoggersEndpoint) Mappings(op actuator.Operation, group string) ([]web.Mapping, error) {
	builder, e := ep.RestMappingBuilder(op, group, ep.MappingPath, ep.MappingName)
	if e != nil {
		return nil, e
	}
	if op.Mode() == actuator.OperationWrite {
		builder.EncodeResponseFunc(ep.WriteEncodeResponse)
	}
	return []web.Mapping{builder.Build()}, nil
}

func (ep *LoggersEndpoint) MappingPath(op actuator.Operation, props *actuator.WebEndpointsProperties) string {
	path := ep.WebEndpointBase.MappingPath(op, props)
	suffix, _ := ep.pathSuffix[op]
	return path + suffix
}

// ReadAll returns all loggers
func (ep *LoggersEndpoint) ReadAll(_ context.Context, _ *struct{}) (interface{}, error) {
	cfgs := log.Levels("")
	out := ReadOutput{
		Levels: allLevels,
		Loggers: map[string]LoggerLevel{},
	}
	for _, v := range cfgs {
		out.Loggers[v.Name] = LoggerLevel{
			EffectiveLevel:  v.EffectiveLevel,
			ConfiguredLevel: v.ConfiguredLevel,
		}
	}
	return out, nil
}

// ReadByName find one logger by name
func (ep *LoggersEndpoint) ReadByName(_ context.Context, in *ReadInput) (interface{}, error) {
	cfgs := log.Levels(in.Name)
	for k, v := range cfgs {
		if k == strings.ToLower(in.Name) || v.Name == in.Name {
			return &LoggerLevel{
				EffectiveLevel:  v.EffectiveLevel,
				ConfiguredLevel: v.ConfiguredLevel,
			}, nil
		}
	}
	return nil, web.NewHttpError(http.StatusNotFound, fmt.Errorf("logger with name %s not found", in.Name))
}

// Write update logger levels
func (ep *LoggersEndpoint) Write(_ context.Context, in *WriteInput) (interface{}, error) {
	log.SetLevel(in.Prefix, in.ConfiguredLevel)
	return nil, nil
}

func (ep *LoggersEndpoint) WriteEncodeResponse(_ context.Context, rw http.ResponseWriter, _ interface{}) error {
	rw.WriteHeader(http.StatusNoContent)
	return nil
}