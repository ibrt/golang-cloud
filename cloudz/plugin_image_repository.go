package cloudz

import (
	"fmt"

	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	goecr "github.com/awslabs/goformation/v6/cloudformation/ecr"
	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/boolz"
	"github.com/ibrt/golang-bites/jsonz"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-validation/vz"
)

// Image repository constants.
const (
	ImageRepositoryPluginDisplayName = "ImageRepository"
	ImageRepositoryPluginName        = "image-repository"
	ImageRepositoryRefRepository     = CloudRef("r")
	ImageRepositoryAttARN            = CloudAtt("Arn")
	ImageRepositoryAttRepositoryURI  = CloudAtt("RepositoryUri")
)

var (
	_ ImageRepository = &imageRepositoryImpl{}
	_ Plugin          = &imageRepositoryImpl{}
)

// ImageRepositoryConfigFunc returns the image repository config for a given Stage.
type ImageRepositoryConfigFunc func(Stage, *ImageRepositoryDependencies) *ImageRepositoryConfig

// ImageRepositoryEventHookFunc describes a image repository event hook.
type ImageRepositoryEventHookFunc func(ImageRepository, Event, string)

// ImageRepositoryConfig describes the image repository config.
type ImageRepositoryConfig struct {
	Stage     Stage  `validate:"required"`
	Name      string `validate:"required,resource-name"`
	Cloud     *ImageRepositoryConfigCloud
	EventHook ImageRepositoryEventHookFunc
}

// MustValidate validates the image repository config.
func (c *ImageRepositoryConfig) MustValidate(stageTarget StageTarget) {
	vz.MustValidateStruct(c)
	errorz.Assertf(stageTarget == Local || c.Cloud != nil, "missing ImageRepositoryConfig.Cloud")
}

// ImageRepositoryConfigCloud describes part of the image repository config.
type ImageRepositoryConfigCloud struct {
	IsScanOnPushEnabled  bool
	AreTagsMutable       bool
	TaggedImagesPolicy   *ImageRepositoryConfigTaggedImagesPolicy
	UntaggedImagesPolicy *ImageRepositoryConfigUntaggedImagesPolicy
}

// ImageRepositoryConfigTaggedImagesPolicy describes part of the image repository config.
type ImageRepositoryConfigTaggedImagesPolicy struct {
	TagPrefixes  []string `validate:"required"`
	MaximumCount int      `validate:"required"`
}

// ImageRepositoryConfigUntaggedImagesPolicy describes part of the image repository config.
type ImageRepositoryConfigUntaggedImagesPolicy struct {
	DeleteAfterDays int `validate:"required"`
}

// ImageRepositoryDependencies describes the image repository dependencies.
type ImageRepositoryDependencies struct {
	OtherDependencies OtherDependencies
}

// MustValidate validates the image repository dependencies.
func (d *ImageRepositoryDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// ImageRepositoryLocalMetadata describes the image repository local metadata.
type ImageRepositoryLocalMetadata struct {
	ImageName string
}

// ImageRepositoryCloudMetadata describes the image repository cloud metadata.
type ImageRepositoryCloudMetadata struct {
	Exports   CloudExports
	ImageName string
}

// ImageRepository describes an image repository.
type ImageRepository interface {
	Plugin
	GetConfig() *ImageRepositoryConfig
	GetLocalMetadata() *ImageRepositoryLocalMetadata
	GetCloudMetadata(require bool) *ImageRepositoryCloudMetadata
}

type imageRepositoryImpl struct {
	cfgFunc       ImageRepositoryConfigFunc
	deps          *ImageRepositoryDependencies
	cfg           *ImageRepositoryConfig
	localMetadata *ImageRepositoryLocalMetadata
	cloudMetadata *ImageRepositoryCloudMetadata
}

// NewImageRepository initializes a new ImageRepository.
func NewImageRepository(cfgFunc ImageRepositoryConfigFunc, deps *ImageRepositoryDependencies) ImageRepository {
	deps.MustValidate()

	return &imageRepositoryImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*imageRepositoryImpl) GetDisplayName() string {
	return ImageRepositoryPluginDisplayName
}

// GetName implements the Plugin interface.
func (p *imageRepositoryImpl) GetName() string {
	return ImageRepositoryPluginName
}

// GetInstanceName implements the Plugin interface.
func (p *imageRepositoryImpl) GetInstanceName() *string {
	return stringz.Ptr(p.cfg.Name)
}

// GetDependenciesMap implements the Plugin interface.
func (p *imageRepositoryImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{}
	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}
	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *imageRepositoryImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *imageRepositoryImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(ImageRepositoryPluginName))
	return p.cfg.Stage
}

// GetConfig implements the ImageRepository interface.
func (p *imageRepositoryImpl) GetConfig() *ImageRepositoryConfig {
	return p.cfg
}

// GetLocalMetadata implements the ImageRepository interface.
func (p *imageRepositoryImpl) GetLocalMetadata() *ImageRepositoryLocalMetadata {
	errorz.Assertf(p.localMetadata != nil, "local not deployed", errorz.Prefix(ImageRepositoryPluginName))
	return p.localMetadata
}

// GetCloudMetadata implements the ImageRepository interface.
func (p *imageRepositoryImpl) GetCloudMetadata(require bool) *ImageRepositoryCloudMetadata {
	errorz.Assertf(!require || p.cloudMetadata != nil, "cloud not deployed", errorz.Prefix(ImageRepositoryPluginName))
	return p.cloudMetadata
}

// IsDeployed implements the Plugin interface.
func (p *imageRepositoryImpl) IsDeployed() bool {
	return p.cloudMetadata != nil
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *imageRepositoryImpl) UpdateLocalTemplate(_ *dctypes.Config, _ string) {
	p.localMetadata = &ImageRepositoryLocalMetadata{
		ImageName: fmt.Sprintf("%v-%v", p.cfg.Stage.GetConfig().App.GetConfig().Name, p.cfg.Name),
	}
}

// GetCloudTemplate implements the Plugin interface.
func (p *imageRepositoryImpl) GetCloudTemplate(_ string) *gocf.Template {
	tpl := gocf.NewTemplate()

	tpl.Resources[ImageRepositoryRefRepository.Ref()] = &goecr.Repository{
		ImageScanningConfiguration: &goecr.Repository_ImageScanningConfiguration{
			ScanOnPush: func() *bool {
				if p.cfg.Cloud.IsScanOnPushEnabled {
					return boolz.Ptr(true)
				}
				return boolz.Ptr(false)
			}(),
		},
		ImageTagMutability: func() *string {
			if p.cfg.Cloud.AreTagsMutable {
				return stringz.Ptr("MUTABLE")
			}
			return stringz.Ptr("IMMUTABLE")
		}(),
		LifecyclePolicy: func() *goecr.Repository_LifecyclePolicy {
			if p.cfg.Cloud.TaggedImagesPolicy == nil && p.cfg.Cloud.UntaggedImagesPolicy == nil {
				return nil
			}

			rules := make([]map[string]interface{}, 0)

			if p.cfg.Cloud.TaggedImagesPolicy != nil {
				rules = append(rules, map[string]interface{}{
					"rulePriority": 1,
					"description": fmt.Sprintf(
						"Keep at most %v images tagged with prefixes: %v.",
						p.cfg.Cloud.TaggedImagesPolicy.MaximumCount,
						p.cfg.Cloud.TaggedImagesPolicy.TagPrefixes),
					"selection": map[string]interface{}{
						"countNumber":   p.cfg.Cloud.TaggedImagesPolicy.MaximumCount,
						"countType":     "imageCountMoreThan",
						"tagPrefixList": p.cfg.Cloud.TaggedImagesPolicy.TagPrefixes,
						"tagStatus":     "tagged",
					},
					"action": map[string]interface{}{
						"type": "expire",
					},
				})
			}

			if p.cfg.Cloud.UntaggedImagesPolicy != nil {
				rules = append(rules, map[string]interface{}{
					"rulePriority": 2,
					"description": fmt.Sprintf(
						"Expire untagged images after %v days.",
						p.cfg.Cloud.UntaggedImagesPolicy.DeleteAfterDays),
					"selection": map[string]interface{}{
						"countNumber": p.cfg.Cloud.UntaggedImagesPolicy.DeleteAfterDays,
						"countType":   "sinceImagePushed",
						"countUnit":   "days",
						"tagStatus":   "untagged",
					},
					"action": map[string]interface{}{
						"type": "expire",
					},
				})
			}

			return &goecr.Repository_LifecyclePolicy{
				LifecyclePolicyText: stringz.Ptr(jsonz.MustMarshalIndentDefaultString(map[string]interface{}{
					"rules": rules,
				})),
			}
		}(),
		RepositoryName: stringz.Ptr(ImageRepositoryRefRepository.Name(p)),
		Tags:           CloudGetDefaultTags(ImageRepositoryRefRepository.Name(p)),
	}
	CloudAddExpRef(tpl, p, ImageRepositoryRefRepository)
	CloudAddExpGetAtt(tpl, p, ImageRepositoryRefRepository, ImageRepositoryAttARN)
	CloudAddExpGetAtt(tpl, p, ImageRepositoryRefRepository, ImageRepositoryAttRepositoryURI)

	return tpl
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *imageRepositoryImpl) UpdateCloudMetadata(stack *awscft.Stack) {
	exports := NewCloudExports(stack)

	p.cloudMetadata = &ImageRepositoryCloudMetadata{
		Exports:   exports,
		ImageName: exports.GetAtt(ImageRepositoryRefRepository, ImageRepositoryAttRepositoryURI),
	}
}

// EventHook implements the Plugin interface.
func (p *imageRepositoryImpl) EventHook(event Event, buildDirPath string) {
	if p.cfg.EventHook != nil {
		p.cfg.EventHook(p, event, buildDirPath)
	}
}
