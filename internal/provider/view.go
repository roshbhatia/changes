package provider

import (
	"bytes"
	_ "embed"
	"sort"
	"strings"
	"text/template"

	providerlib "github.com/roshbhatia/go-utils/provider"
)

//go:embed templates/list.txt
var listTemplateSource string

//go:embed templates/validation.txt
var validationTemplateSource string

type listView struct {
	Name        string
	Description string
	Actions     string
	Command     string
	Path        string
}

type validationView struct {
	Mark   string
	Name   string
	Checks []checkView
}

type checkView struct {
	Mark    string
	Kind    string
	Target  string
	Message string
}

// RenderList renders provider discovery results for terminal output.
func RenderList(providers []LoadedManifest) (string, error) {
	views := make([]listView, 0, len(providers))
	for _, loaded := range providers {
		actions := make([]string, 0, len(loaded.Manifest.Actions))
		for action := range loaded.Manifest.Actions {
			actions = append(actions, action)
		}
		sort.Strings(actions)
		views = append(views, listView{
			Name:        loaded.Manifest.Name,
			Description: loaded.Manifest.Description,
			Actions:     strings.Join(actions, ", "),
			Command:     strings.Join(loaded.Manifest.Command, " "),
			Path:        loaded.Path,
		})
	}
	return render("provider list", listTemplateSource, views)
}

// RenderValidations renders deterministic validation checks.
func RenderValidations(validations []Validation) (string, error) {
	views := make([]validationView, 0, len(validations))
	for _, validation := range validations {
		view := validationView{Mark: "+", Name: validation.Manifest.Name}
		if !validation.OK() {
			view.Mark = "x"
		}
		for _, check := range validation.Checks {
			mark := "+"
			if check.Status != providerlib.CheckOK {
				mark = "x"
			}
			view.Checks = append(view.Checks, checkView{
				Mark: mark, Kind: check.Kind, Target: check.Target, Message: check.Message,
			})
		}
		views = append(views, view)
	}
	return render("provider validation", validationTemplateSource, views)
}

func render(name, source string, data any) (string, error) {
	parsed, err := template.New(name).Option("missingkey=error").Parse(source)
	if err != nil {
		return "", err
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(output.String()), nil
}
