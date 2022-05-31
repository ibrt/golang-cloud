package cloudz

import (
	"path"
	"strings"

	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-validation/vz"
)

// CloudStageConfig describes the Stage cloud config.
type CloudStageConfig struct {
	*StageConfig `validate:"required"`
	Name         string    `validate:"required,resource-name"`
	Version      string    `validate:"required"`
	Mode         StageMode `validate:"required,oneof=production staging"`
}

// MustValidate validates the cloud stage config.
func (c *CloudStageConfig) MustValidate() {
	vz.MustValidateStruct(c)
}

// CloudStage describes a cloud Stage.
type CloudStage interface {
	Stage
	GetCloudConfig() *CloudStageConfig
	GetArtifactsKeyPrefix(p Plugin, additionalParts ...string) string
	IsDeployed() bool
	Deploy()
}

type cloudStageImpl struct {
	cfg *CloudStageConfig
}

// NewCloudStage initializes a new CloudStage.
func NewCloudStage(cfg *CloudStageConfig) CloudStage {
	cfg.MustValidate()

	stage := &cloudStageImpl{
		cfg: cfg,
	}

	for _, pluginGroup := range cfg.App.GetSortedPlugins() {
		for _, plugin := range pluginGroup {
			plugin.Configure(stage)

			if stack := cfg.App.GetOperations().DescribeStack(CloudGetStackName(plugin)); stack != nil {
				plugin.UpdateCloudMetadata(stack)
			}
		}
	}

	return stage
}

// GetName implements the Stage interface.
func (s *cloudStageImpl) GetName() string {
	return s.cfg.Name
}

// GetTarget implements the Stage interface.
func (s *cloudStageImpl) GetTarget() StageTarget {
	return Cloud
}

// GetMode implements the Stage interface.
func (s *cloudStageImpl) GetMode() StageMode {
	return s.cfg.Mode
}

// GetConfig implements the Stage interface.
func (s *cloudStageImpl) GetConfig() *StageConfig {
	return s.cfg.StageConfig
}

// AsLocalStage implements the Stage interface.
func (s *cloudStageImpl) AsLocalStage() LocalStage {
	panic(errorz.Errorf("cloud Stage: does not implement local Stage"))
}

// AsCloudStage implements the Stage interface.
func (s *cloudStageImpl) AsCloudStage() CloudStage {
	return s
}

// GetCloudConfig implements the CloudStage interface.
func (s *cloudStageImpl) GetCloudConfig() *CloudStageConfig {
	return s.cfg
}

// IsDeployed implements the CloudStage interface.
func (s *cloudStageImpl) IsDeployed() bool {
	isDeployed := true

	for _, pluginGroup := range s.cfg.App.GetSortedPlugins() {
		for _, plugin := range pluginGroup {
			isDeployed = isDeployed && plugin.IsDeployed()
		}
	}

	return isDeployed
}

// GetArtifactsKeyPrefix returns an artifacts key prefix for the given plugin.
func (s *cloudStageImpl) GetArtifactsKeyPrefix(p Plugin, additionalParts ...string) string {
	parts := []string{
		s.cfg.Name,
		s.cfg.Version,
		p.GetName(),
	}

	if instanceName := p.GetInstanceName(); instanceName != nil && *instanceName != "" {
		parts = append(parts, *instanceName)
	}

	return path.Join(append([]string{strings.Join(parts, "-")}, additionalParts...)...)
}

// Deploy implements the CloudStage interface.
func (s *cloudStageImpl) Deploy() {
	for _, pluginGroup := range s.cfg.App.GetSortedPlugins() {
		for _, plugin := range pluginGroup {
			buildDirPath := s.cfg.App.GetConfig().GetBuildDirPathForPlugin(plugin)

			tpl := plugin.GetCloudTemplate(buildDirPath)
			if tpl == nil {
				continue
			}

			buf, err := tpl.JSON()
			errorz.MaybeMustWrap(err)

			plugin.EventHook(CloudBeforeDeployEvent, buildDirPath)

			plugin.UpdateCloudMetadata(
				s.cfg.App.GetOperations().UpsertStack(
					CloudGetStackName(plugin),
					string(buf),
					map[string]string{
						"Stage": s.GetName(),
					}))

			plugin.EventHook(CloudAfterDeployEvent, buildDirPath)
		}
	}
}
