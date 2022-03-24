package cloudz

import (
	"fmt"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ibrt/golang-validation/vz"
)

// AppConfig describes the app config.
type AppConfig struct {
	DisplayName   string      `validate:"required"`
	Name          string      `validate:"required,resource-name"`
	ConfigDirPath string      `validate:"required,dir"`
	BuildDirPath  string      `validate:"required,parent-dir"`
	AWSConfig     *aws.Config `validate:"required"`
	Plugins       []Plugin    `validate:"required"`
}

// MustValidate validates the app config.
func (c *AppConfig) MustValidate() {
	vz.MustValidateStruct(c)
}

// App describes an App.
type App interface {
	GetConfig() *AppConfig
	GetBuildDirPath(p Plugin, additionalParts ...string) string
	GetConfigDirPath(p Plugin, additionalParts ...string) string
	GetOperations() Operations
	GetSortedPlugins() [][]Plugin
}

type appImpl struct {
	cfg           *AppConfig
	ops           Operations
	sortedPlugins [][]Plugin
}

// NewApp initializes a new App.
func NewApp(cfg *AppConfig) App {
	cfg.MustValidate()

	sortedPlugins := make([][]Plugin, 0)
	pluginsMap := make(map[Plugin]map[Plugin]struct{})

	for _, plugin := range cfg.Plugins {
		pluginsMap[plugin] = plugin.GetDependenciesMap()
	}

	for len(pluginsMap) > 0 {
		sortedPlugins = append(sortedPlugins, make([]Plugin, 0))

		for plugin, dependedOnInstance := range pluginsMap {
			if len(dependedOnInstance) == 0 {
				sortedPlugins[len(sortedPlugins)-1] = append(sortedPlugins[len(sortedPlugins)-1], plugin)
			}
		}

		for _, plugin := range sortedPlugins[len(sortedPlugins)-1] {
			delete(pluginsMap, plugin)

			for _, dependedOnInstance := range pluginsMap {
				delete(dependedOnInstance, plugin)
			}
		}
	}

	return &appImpl{
		cfg:           cfg,
		ops:           NewOperations(cfg.AWSConfig),
		sortedPlugins: sortedPlugins,
	}
}

// GetConfig implements the App interface.
func (a *appImpl) GetConfig() *AppConfig {
	return a.cfg
}

// GetBuildDirPath returns a build dir path for the given plugin.
func (a *appImpl) GetBuildDirPath(p Plugin, additionalParts ...string) string {
	parts := []string{
		a.GetConfig().BuildDirPath,
		p.GetStage().GetName(),
	}

	if instanceName := p.GetInstanceName(); instanceName != nil && *instanceName != "" {
		parts = append(parts, fmt.Sprintf("%v-%v", p.GetName(), *instanceName))
	} else {
		parts = append(parts, p.GetName())
	}

	return filepath.Join(append(parts, additionalParts...)...)
}

// GetConfigDirPath returns a config dir path for the given plugin.
func (a *appImpl) GetConfigDirPath(p Plugin, additionalParts ...string) string {
	parts := []string{
		a.GetConfig().ConfigDirPath,
	}

	if instanceName := p.GetInstanceName(); instanceName != nil && *instanceName != "" {
		parts = append(parts, fmt.Sprintf("%v-%v", p.GetName(), *instanceName))
	} else {
		parts = append(parts, p.GetName())
	}

	return filepath.Join(append(parts, additionalParts...)...)
}

// GetOperations implements the App interface.
func (a *appImpl) GetOperations() Operations {
	return a.ops
}

// GetSortedPlugins implements the App interface.
func (a *appImpl) GetSortedPlugins() [][]Plugin {
	return a.sortedPlugins
}
