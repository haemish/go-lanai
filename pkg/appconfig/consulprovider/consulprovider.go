package consulprovider

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/appconfig"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/consul"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/log"
	"fmt"
)

var logger = log.New("Config.Consul")

const (
	ConsulConfigPrefix = "cloud.consul.config"
	ConfigKeyAppName   = "application.name"
)

type ConsulConfigProperties struct {
	Enabled        bool   `json:"enabled"`
	Prefix         string `json:"prefix"`
	DefaultContext string `json:"default-context"`
	ProfileSeparator string `json:"profile-separator"`
}

type ConfigProvider struct {
	appconfig.ProviderMeta
	contextPath  string
	connection   *consul.Connection
}

func (configProvider *ConfigProvider) Name() string {
	return fmt.Sprintf("consul:%s", configProvider.contextPath)
}

func (configProvider *ConfigProvider) Load(ctx context.Context) (loadError error) {
	defer func(){
		if loadError != nil {
			configProvider.Loaded = false
		} else {
			configProvider.Loaded = true
		}
	}()

	configProvider.Settings = make(map[string]interface{})

	// load keys from default context
	var defaultSettings map[string]interface{}

	defaultSettings, loadError = configProvider.connection.ListKeyValuePairs(
		ctx,
		configProvider.contextPath)
	if loadError != nil {
		return loadError
	}

	unFlattenedSettings, loadError := appconfig.UnFlatten(defaultSettings)
	if loadError != nil {
		return loadError
	}

	configProvider.Settings = unFlattenedSettings

	logger.WithContext(ctx).Infof("Retrieved %d configs from consul: %s", len(defaultSettings), configProvider.contextPath)
	return nil
}

func NewConsulProvider(precedence int, contextPath string, conn *consul.Connection) *ConfigProvider {
	return &ConfigProvider{
			ProviderMeta: appconfig.ProviderMeta{Precedence: precedence},
			contextPath:  contextPath, //fmt.Sprintf("%s/%s", f.sourceConfig.Prefix, f.contextPath)
			connection:   conn,
		}
}
