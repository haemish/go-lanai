package generator

import (
	"github.com/getkin/kin-openapi/openapi3"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"text/template"
)

// ApiVersionGenerator generate 1 file per API version, based on OpenAPI specs
type ApiVersionGenerator struct {
	data             map[string]interface{}
	template         *template.Template
	nameRegex        *regexp.Regexp
	defaultRegenRule RegenMode
	rules            RegenRules
	templateFS       fs.FS
	outputFS         fs.FS
}

const versionGeneratorName = "version"

func newApiVersionGenerator(opts ...func(option *Option)) *ApiVersionGenerator {
	o := &Option{}
	for _, fn := range opts {
		fn(o)
	}

	return &ApiVersionGenerator{
		data:             o.Data,
		template:         o.Template,
		templateFS:       o.TemplateFS,
		outputFS:         o.OutputFS,
		nameRegex:        regexp.MustCompile("^(version.)(.+)(.tmpl)"),
		defaultRegenRule: o.DefaultRegenMode,
		rules:            o.RegenRules,
	}
}

func (m *ApiVersionGenerator) determineFilename(template string) string {
	var result string
	matches := m.nameRegex.FindStringSubmatch(path.Base(template))
	if len(matches) < 2 {
		result = ""
	}

	result = matches[2]
	return result
}

func (m *ApiVersionGenerator) Generate(tmplPath string, dirEntry fs.DirEntry) error {
	if dirEntry.IsDir() || !m.nameRegex.MatchString(path.Base(tmplPath)) {
		// Skip over it
		return nil
	}

	// get all versions
	iterateOver := make(map[string][]string)
	for pathName, _ := range m.data[CKOpenAPIData].(*openapi3.T).Paths {
		version := apiVersion(pathName)
		iterateOver[version] = append(iterateOver[version], pathName)
	}

	var toGenerate []GenerationContext
	for version, versionData := range iterateOver {
		data := copyOf(m.data)
		sort.Strings(versionData)
		data["VersionData"] = versionData
		data["Version"] = version

		targetDir, err := ConvertSrcRootToTargetDir(path.Dir(tmplPath), data, m.templateFS)
		if err != nil {
			return err
		}

		outputFile := path.Join(targetDir, m.determineFilename(tmplPath))
		regenRule, err := getApplicableRegenRules(outputFile, m.rules, m.defaultRegenRule)
		if err != nil {
			return err
		}
		toGenerate = append(toGenerate, *NewGenerationContext(
			tmplPath,
			outputFile,
			regenRule,
			data,
		))
	}

	for _, gc := range toGenerate {
		logger.Infof("version generator generating %v\n", gc.filename)
		err := GenerateFileFromTemplate(gc, m.template)
		if err != nil {
			return err
		}
	}

	return nil
}
