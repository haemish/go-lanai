package bootstrap

import (
	"context"
	"cto-github.cisco.com/livdu/jupiter/pkg/appconfig"
	"fmt"
)

type LifecycleHandler func(context.Context) error

// A Context carries addition data for application.
// delegates all other context calls to the embedded Context.
type ApplicationContext struct {
	context.Context
	config *appconfig.Config
}

func NewContext() *ApplicationContext {
	return &ApplicationContext{
		Context: context.Background(),
	}
}

func (c *ApplicationContext) getConfig() appconfig.ConfigAccessor {
	return c.config
}

/**************************
 context.Context Interface
***************************/
func (_ *ApplicationContext) String() string {
	return "application context"
}

/*************
* unexported methods
**************/
func (c *ApplicationContext) updateConfig(config *appconfig.Config) {
	c.config = config
}

func (c *ApplicationContext) updateParent(parent context.Context) *ApplicationContext {
	c.Context = parent
	return c
}

func (c *ApplicationContext) dumpConfigurations() {
	c.config.Each(func(key string, value interface{}) {
		fmt.Println(key + ": " + fmt.Sprintf("%v", value))
	})
}