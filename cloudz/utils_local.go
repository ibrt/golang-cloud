package cloudz

import (
	"strings"
)

// Pseudo-secret values used for local services.
const (
	LocalAWSAccessKeyID = "aws-access-key-id"
	LocalAWSSecretKey   = "aws-secret-key"
	LocalPassword       = "password"
	LocalSecret         = "secret"
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
