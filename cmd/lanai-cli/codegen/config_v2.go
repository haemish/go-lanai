package codegen

import "cto-github.cisco.com/NFV-BU/go-lanai/cmd/lanai-cli/codegen/generator"

type ConfigV2 struct {
	Project    ProjectV2      `json:"project"`
	Templates  TemplatesV2    `json:"templates"`
	Components ComponentsV2   `json:"components"`
	Regen      RegenerationV2 `json:"regen"`
}

func (c ConfigV2) ToOptions() []generator.Options {
	return []generator.Options{
		c.Project.ToOption(),
		c.Components.ToOption(),
		c.Regen.ToOption(),
	}
}

type ProjectV2 struct {
	// Name service name
	Name string `json:"name"`
	// Module golang module
	Module string `json:"module"`
	// Port
	Port int `json:"port"`
	// ContextPath golang module
	ContextPath string `json:"context-path"`
	// Description golang module
	Description string `json:"description"`
}

func (p *ProjectV2) ToOption() generator.Options {
	return generator.WithProject(generator.Project{
		Name:        p.Name,
		Module:      p.Module,
		Description: p.Description,
		Port:        p.Port,
		ContextPath: p.ContextPath,
	})
}

type TemplatesV2 struct {
	Path string `json:"path"`
}

type ComponentsV2 struct {
	Contract ContractV2 `json:"contract"`
}

func (c *ComponentsV2) ToOption() generator.Options {
	return generator.WithComponents(generator.Components{
		Contract: generator.Contract{
			Path:   c.Contract.Path,
			Naming: generator.ContractNaming{
				RegExps: c.Contract.Naming.RegExps,
			},
		},
	})
}

type ContractV2 struct {
	Path   string           `json:"path"`
	Naming ContractNamingV2 `json:"naming"`
}

type ContractNamingV2 struct {
	RegExps map[string]string `json:"regular-expressions"`
}

type RegenMode generator.RegenMode

type RegenRule struct {
	// Pattern wildcard pattern of output file path
	Pattern string `json:"pattern"`
	// Mode regeneration mode on matched output files in case of changes. (ignore, overwrite, reference, etc.)
	Mode RegenMode `json:"mode"`
}

type RegenRules []RegenRule

type RegenerationV2 struct {
	Default RegenMode  `json:"default"`
	Rules   RegenRules `json:"rules"`
}

func (r RegenerationV2) ToOption() func(*generator.Option) {
	rules := make(generator.RegenRules, len(r.Rules))
	for i := range r.Rules {
		rules[i] = generator.RegenRule{
			Pattern: r.Rules[i].Pattern,
			Mode:    generator.RegenMode(r.Rules[i].Mode),
		}
	}
	return generator.WithRegenRules(rules, generator.RegenMode(r.Default))
}
