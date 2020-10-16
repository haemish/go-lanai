package commandprovider

import (
	"cto-github.cisco.com/livdu/jupiter/pkg/config"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"sync"
)

const (
	configKeyAppName = "spring.application.name"
)

type ConfigProvider struct {
	config.ProviderMeta
	prefix  string
	appName string
	extras  map[string]string
	flagSet *pflag.FlagSet
	once    sync.Once
}

func (f *ConfigProvider) Load() {
	fmt.Println("Loading command line config")

	//TODO: review the commented out section to see if it's actually needed
	//f.once.Do(func() {
	//	f.extras = args.Extras(func(name string) bool {
	//		return f.flagSet.Lookup(name) != nil
	//	})
	//})
	//
	//if !f.flagSet.Parsed() {
	//	if err := f.flagSet.Parse(os.Args[1:]); err != nil {
	//		return
	//	}
	//}
	f.Valid = false

	f.Settings = make(map[string] interface{})

	f.flagSet.VisitAll(func(flag *pflag.Flag) {
		key := config.NormalizeKey(f.prefix + flag.Name)
		f.Settings[key] = flag.Value.String()
	})

	// Apply extras
	for k, v := range f.extras {
		f.Settings[k] = v
	}

	// Apply application name
	f.Settings[configKeyAppName] = f.appName

	f.Valid = true
}

func extractFlagSet(command *cobra.Command) *pflag.FlagSet {
	flagSet := pflag.NewFlagSet(command.Name(), pflag.ContinueOnError)
	flagSet.ParseErrorsWhitelist.UnknownFlags = true

	command.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		flagSet.AddFlag(flag)
	})

	command.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		flagSet.AddFlag(flag)
	})

	return flagSet
}

func NewCobraProvider(description string, precedence int, command *cobra.Command, prefix string) *ConfigProvider {
	flagSet := extractFlagSet(command)

	return &ConfigProvider{
		ProviderMeta: config.ProviderMeta{Description: description, Precedence: precedence},
		prefix:  prefix,
		flagSet: flagSet,
		appName: command.Root().Name(),
	}
}