package cloudz

import (
	"fmt"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ibrt/golang-validation/vz"

	"github.com/ibrt/golang-cloud/opz"
)

// AppConfig describes the app config.
type AppConfig struct {
	DisplayName   string      `validate:"required"`
	Name          string      `validate:"required,resource-name"`
	RootDirPath   string      `validate:"required,dir"`
	ConfigDirPath string      `validate:"required,dir"`
	BuildDirPath  string      `validate:"required,parent-dir"`
	AWSConfig     *aws.Config `validate:"required"`
	Plugins       []Plugin    `validate:"required"`
}

// GetRootDirPath returns the root dir path.
func (c *AppConfig) GetRootDirPath(additionalParts ...string) string {
	return filepath.Join(append([]string{c.RootDirPath}, additionalParts...)...)
}

// GetBuildDirPath returns the build dir path.
func (c *AppConfig) GetBuildDirPath(additionalParts ...string) string {
	return filepath.Join(append([]string{c.BuildDirPath}, additionalParts...)...)
}

// GetConfigDirPath returns the config dir path.
func (c *AppConfig) GetConfigDirPath(additionalParts ...string) string {
	return filepath.Join(append([]string{c.ConfigDirPath}, additionalParts...)...)
}

// GetBuildDirPathForPlugin returns a build dir path for the given plugin.
func (c *AppConfig) GetBuildDirPathForPlugin(p Plugin, additionalParts ...string) string {
	if instanceName := p.GetInstanceName(); instanceName != nil && *instanceName != "" {
		return c.GetBuildDirPath(append([]string{
			p.GetStage().GetName(),
			fmt.Sprintf("%v-%v", p.GetName(), *instanceName),
		}, additionalParts...)...)
	}

	return c.GetBuildDirPath(append([]string{
		p.GetStage().GetName(),
		p.GetName(),
	}, additionalParts...)...)
}

// GetConfigDirPathForPlugin returns a config dir path for the given plugin.
func (c *AppConfig) GetConfigDirPathForPlugin(p Plugin, additionalParts ...string) string {
	if instanceName := p.GetInstanceName(); instanceName != nil && *instanceName != "" {
		return c.GetConfigDirPath(append([]string{
			fmt.Sprintf("%v-%v", p.GetName(), *instanceName),
		}, additionalParts...)...)
	}

	return c.GetConfigDirPath(append([]string{
		p.GetName(),
	}, additionalParts...)...)
}

// MustValidate validates the app config.
func (c *AppConfig) MustValidate() {
	vz.MustValidateStruct(c)
}

// App describes an App.
type App interface {
	GetConfig() *AppConfig
	GetOperations() opz.Operations
	GetSortedPlugins() [][]Plugin
}

type appImpl struct {
	cfg           *AppConfig
	ops           opz.Operations
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
		ops:           opz.NewOperations(cfg.BuildDirPath, cfg.AWSConfig),
		sortedPlugins: sortedPlugins,
	}
}

// GetConfig implements the App interface.
func (a *appImpl) GetConfig() *AppConfig {
	return a.cfg
}

// GetOperations implements the App interface.
func (a *appImpl) GetOperations() opz.Operations {
	return a.ops
}

// GetSortedPlugins implements the App interface.
func (a *appImpl) GetSortedPlugins() [][]Plugin {
	return a.sortedPlugins
}
