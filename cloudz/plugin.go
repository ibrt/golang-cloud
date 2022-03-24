package cloudz

import (
	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	dctypes "github.com/docker/cli/cli/compose/types"
)

// Event describes an event.
type Event string

// Known events.
const (
	LocalBeforeCreateEvent Event = "localBeforeCreate"
	LocalAfterCreateEvent  Event = "localAfterCreate"
	CloudBeforeDeployEvent Event = "cloudBeforeDeploy"
	CloudAfterDeployEvent  Event = "cloudAfterDeploy"
)

// Plugin describes a plugin, i.e. a set of behaviors, tools, and components.
type Plugin interface {
	GetDisplayName() string
	GetName() string
	GetInstanceName() *string // nil for singleton Plugins
	GetDependenciesMap() map[Plugin]struct{}
	Configure(stage Stage)
	GetStage() Stage
	IsDeployed() bool
	UpdateLocalTemplate(tpl *dctypes.Config, buildDirPath string)
	GetCloudTemplate(buildDirPath string) *gocf.Template
	UpdateCloudMetadata(stack *awscft.Stack)
	EventHook(event Event, buildDirPath string)
}

// OtherDependencies describes a set of unstructured dependencies.
type OtherDependencies []Plugin

// Find looks a dependency given plugin name and instance name. Returns nil if not found.
func (d OtherDependencies) Find(name string, instanceName *string) Plugin {
	for _, p := range d {
		if p.GetName() == name {
			if i := p.GetInstanceName(); (i == nil && instanceName == nil) || (i != nil && instanceName != nil && *i == *instanceName) {
				return p
			}
		}
	}
	return nil
}
