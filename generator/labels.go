package generator

import (
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/lucaslorentz/caddy-docker-proxy/v2/caddyfile"
)

type targetsProvider func() ([]string, error)

func labelsToCaddyfile(labels map[string]string, templateData interface{}, defaultImport string, getTargets targetsProvider) (*caddyfile.Container, error) {
	funcMap := template.FuncMap{
		"upstreams": func(options ...interface{}) (string, error) {
			targets, err := getTargets()
			sort.Strings(targets)
			transformed := []string{}
			for _, target := range targets {
				for _, param := range options {
					if protocol, isProtocol := param.(string); isProtocol {
						target = protocol + "://" + target
					} else if port, isPort := param.(int); isPort {
						target = target + ":" + strconv.Itoa(port)
					}
				}
				transformed = append(transformed, target)
			}
			sort.Strings(transformed)
			return strings.Join(transformed, " "), err
		},
		"http": func() string {
			return "http"
		},
		"https": func() string {
			return "https"
		},
		"h2c": func() string {
			return "h2c"
		},
	}

	container, err := caddyfile.FromLabels(labels, templateData, funcMap)
	if err != nil || container == nil {
		return container, err
	}

	if strings.TrimSpace(defaultImport) != "" {
		applyDefaultImport(container, strings.TrimSpace(defaultImport))
	}

	return container, nil
}

func applyDefaultImport(container *caddyfile.Container, defaultImport string) {
	for _, block := range container.Children {
		if block.IsGlobalBlock() || block.IsSnippet() || block.IsMatcher() {
			continue
		}
		if hasDirective(block, "import") || hasDirective(block, "tls") {
			continue
		}
		importBlock := caddyfile.CreateBlock()
		importBlock.AddKeys("import", defaultImport)
		block.AddBlock(importBlock)
	}
}

func hasDirective(block *caddyfile.Block, name string) bool {
	for _, child := range block.Container.Children {
		if child.GetFirstKey() == name {
			return true
		}
	}
	return false
}
