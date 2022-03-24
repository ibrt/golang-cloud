package cloudz

import (
	"strings"
)

// LocalGetContainerName generates a container name for the given plugin.
func LocalGetContainerName(p Plugin, additionalParts ...string) string {
	parts := []string{
		p.GetStage().GetConfig().App.GetConfig().Name,
		p.GetName(),
	}

	if instanceName := p.GetInstanceName(); instanceName != nil {
		parts = append(parts, *instanceName)
	}

	return strings.Join(append(parts, additionalParts...), "-")
}
