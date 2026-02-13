package caddyfile

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/caddyserver/caddy/v2/caddyconfig"
)

// Process caddyfile and removes wrong server blocks
func Process(caddyfileContent []byte) ([]byte, []byte) {
	if len(caddyfileContent) == 0 {
		return caddyfileContent, nil
	}

	logsBuffer := bytes.Buffer{}
	adapter := caddyconfig.GetAdapter("caddyfile")

	container, err := Unmarshal(caddyfileContent)
	if err != nil {
		logsBuffer.WriteString(fmt.Sprintf("[ERROR]  Invalid caddyfile: %s\n%s\n", err.Error(), caddyfileContent))
		return nil, logsBuffer.Bytes()
	}

	newContainer := CreateContainer()

	container.sort()
	for _, block := range container.Children {
		blockSnapshot := block.Marshal()
		newContainer.AddBlock(block)

		for {
			_, _, err := adapter.Adapt(newContainer.Marshal(), nil)
			if err == nil {
				break
			}

			ambiguousName := ambiguousSiteName(err)
			if ambiguousName == "" || !removeBlockKey(block, ambiguousName) || len(block.Keys) == 0 {
				newContainer.Remove(block)
				logsBuffer.WriteString(fmt.Sprintf("[ERROR]  Removing invalid block: %s\n%s\n", err.Error(), blockSnapshot))
				if ambiguousName != "" {
					logsBuffer.WriteString(fmt.Sprintf("[ERROR]  Hint: duplicate site address %q detected. This often happens when a Docker label defines a site already present in the base Caddyfile. Remove one or set CADDY_DOCKER_MERGE_SITES=true to merge.\n", ambiguousName))
				}
				break
			}
		}
	}

	return newContainer.Marshal(), logsBuffer.Bytes()
}

const ambiguousSiteDefinitionPrefix = "ambiguous site definition:"

func ambiguousSiteName(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	index := strings.Index(message, ambiguousSiteDefinitionPrefix)
	if index == -1 {
		return ""
	}
	name := strings.TrimSpace(message[index+len(ambiguousSiteDefinitionPrefix):])
	if name == "" {
		return ""
	}
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func removeBlockKey(block *Block, key string) bool {
	if block == nil || key == "" {
		return false
	}
	removed := false
	keys := block.Keys[:0]
	for _, existing := range block.Keys {
		if existing == key && !removed {
			removed = true
			continue
		}
		keys = append(keys, existing)
	}
	block.Keys = keys
	return removed
}
