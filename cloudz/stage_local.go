package cloudz

import (
	"bytes"
	"fmt"
	"os"

	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-shell/shellz"
	"github.com/ibrt/golang-validation/vz"
	"gopkg.in/yaml.v3"
)

// LocalStageConfig describes the local Stage config.
type LocalStageConfig struct {
	*StageConfig `validate:"required"`
}

// MustValidate validates the local stage config.
func (c *LocalStageConfig) MustValidate() {
	vz.MustValidateStruct(c)
}

// LocalStage describes a local Stage.
type LocalStage interface {
	Stage
	GetLocalConfig() *LocalStageConfig
	GetServiceNetworkConfig() map[string]*dctypes.ServiceNetworkConfig
	Create()
	Destroy()
}

type localStageImpl struct {
	cfg           *LocalStageConfig
	localTemplate *dctypes.Config
}

// NewLocalStage initializes a new LocalStage.
func NewLocalStage(cfg *LocalStageConfig) LocalStage {
	cfg.MustValidate()

	stage := &localStageImpl{
		cfg: cfg,
		localTemplate: &dctypes.Config{
			Version:  "3.8",
			Services: dctypes.Services{},
			Networks: map[string]dctypes.NetworkConfig{
				cfg.App.GetConfig().Name: {
					Name: cfg.App.GetConfig().Name,
				},
			},
			Volumes: map[string]dctypes.VolumeConfig{},
		},
	}

	for _, pluginGroup := range cfg.App.GetSortedPlugins() {
		for _, plugin := range pluginGroup {
			plugin.Configure(stage)
			buildDirPath := cfg.App.GetConfig().GetBuildDirPathForPlugin(plugin)
			plugin.UpdateLocalTemplate(stage.localTemplate, buildDirPath)
		}
	}

	return stage
}

// GetName implements the Stage interface.
func (s *localStageImpl) GetName() string {
	return Local.String()
}

// GetTarget implements the Stage interface.
func (s *localStageImpl) GetTarget() StageTarget {
	return Local
}

// GetMode implements the Stage interface.
func (s *localStageImpl) GetMode() StageMode {
	return Staging
}

// GetConfig implements the Stage interface.
func (s *localStageImpl) GetConfig() *StageConfig {
	return s.cfg.StageConfig
}

// AsLocalStage implements the Stage interface.
func (s *localStageImpl) AsLocalStage() LocalStage {
	return s
}

// AsCloudStage implements the Stage interface.
func (s *localStageImpl) AsCloudStage() CloudStage {
	panic(errorz.Errorf("local Stage: does not implement cloud Stage"))
}

// GetLocalConfig implements the LocalStage interface.
func (s *localStageImpl) GetLocalConfig() *LocalStageConfig {
	return s.cfg
}

// GetServiceNetworkConfig implements the LocalStage interface.
func (s *localStageImpl) GetServiceNetworkConfig() map[string]*dctypes.ServiceNetworkConfig {
	return map[string]*dctypes.ServiceNetworkConfig{
		s.cfg.App.GetConfig().Name: {},
	}
}

// Create implements the LocalStage interface.
func (s *localStageImpl) Create() {
	s.Destroy()

	for _, pluginGroup := range s.cfg.App.GetSortedPlugins() {
		for _, plugin := range pluginGroup {
			plugin.EventHook(LocalBeforeCreateEvent, s.cfg.App.GetConfig().GetBuildDirPathForPlugin(plugin))
		}
	}

	s.runCmd("up", "--build", "-d", "--remove-orphans")

	for _, pluginGroup := range s.cfg.App.GetSortedPlugins() {
		for _, plugin := range pluginGroup {
			plugin.EventHook(LocalAfterCreateEvent, s.cfg.App.GetConfig().GetBuildDirPathForPlugin(plugin))
		}
	}
}

// Destroy implements the LocalStage interface.
func (s *localStageImpl) Destroy() {
	for _, svc := range s.localTemplate.Services {
		if svc.Build.Context != "" {
			// Note: workaround for docker-compose requiring build directories to always exist, even on "down".
			errorz.MaybeMustWrap(os.MkdirAll(svc.Build.Context, 0777))
		}
	}

	s.runCmd("down", "-v", "--rmi", "local", "--remove-orphans")
}

func (s *localStageImpl) runCmd(params ...interface{}) {
	rawTpl, err := yaml.Marshal(s.localTemplate)
	errorz.MaybeMustWrap(err)

	fmt.Println(string(rawTpl))

	shellz.NewCommand("docker-compose").
		AddParams("-p", s.cfg.App.GetConfig().Name).
		AddParams("-f", "-").
		AddParams(params...).
		SetStdin(bytes.NewReader(rawTpl)).
		MustRun()
}
