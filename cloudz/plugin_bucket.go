package cloudz

import (
	"fmt"
	"net/url"

	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	gos3 "github.com/awslabs/goformation/v6/cloudformation/s3"
	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/boolz"
	"github.com/ibrt/golang-bites/numeric/intz"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-bites/urlz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-validation/vz"
)

// Bucket constants.
const (
	BucketPluginDisplayName      = "Bucket"
	BucketPluginName             = "bucket"
	BucketRefBucket              = CloudRef("b")
	BucketRefBucketPolicyPublic  = CloudRef("bp-pub")
	BucketAttARN                 = CloudAtt("Arn")
	BucketAttDomainName          = CloudAtt("DomainName")
	BucketAttDualStackDomainName = CloudAtt("DualStackDomainName")
	BucketAttRegionalDomainName  = CloudAtt("RegionalDomainName")

	minioVersion     = "2022.4.16"
	minioPort        = 9000
	minioConsolePort = 9001
)

var (
	_ Bucket = &bucketImpl{}
	_ Plugin = &bucketImpl{}
)

// BucketConfigFunc returns the bucket config for a given Stage.
type BucketConfigFunc func(Stage, *BucketDependencies) *BucketConfig

// BucketEventHookFunc describes a bucket event hook.
type BucketEventHookFunc func(Bucket, Event, string)

// BucketConfig describes the bucket config.
type BucketConfig struct {
	Stage                 Stage  `validate:"required"`
	Name                  string `validate:"required,resource-name"`
	IsPublicAccessEnabled bool
	Local                 *BucketConfigLocal
	Cloud                 *BucketConfigCloud
	EventHook             BucketEventHookFunc
}

// MustValidate validates the bucket config.
func (c *BucketConfig) MustValidate(stageTarget StageTarget) {
	vz.MustValidateStruct(c)
	errorz.Assertf(stageTarget == Cloud || c.Local != nil, "missing BucketConfig.Local")
	errorz.Assertf(stageTarget == Local || c.Cloud != nil, "missing BucketConfig.Cloud")
}

// BucketConfigLocal describes part of the bucket config.
type BucketConfigLocal struct {
	ExternalPort        uint16 `validate:"required"`
	ConsoleExternalPort uint16 `validate:"required"`
}

// BucketConfigCloud describes part of the bucket config.
type BucketConfigCloud struct {
	IsVersioningEnabled                   bool
	DeleteObjectsAfterDays                *uint16
	DeletePreviousObjectVersionsAfterDays *uint16
}

// BucketDependencies describes the bucket dependencies.
type BucketDependencies struct {
	OtherDependencies OtherDependencies
}

// MustValidate validates the bucket dependencies.
func (d *BucketDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// BucketLocalMetadata describes the bucket local metadata.
type BucketLocalMetadata struct {
	ContainerName      string
	AccessKey          string
	SecretKey          string
	BucketName         string
	ExternalURL        *url.URL
	InternalURL        *url.URL
	ConsoleExternalURL *url.URL
}

// BucketCloudMetadata describes the bucket cloud metadata.
type BucketCloudMetadata struct {
	Exports    CloudExports
	BucketName string
	BucketURL  *url.URL
}

// GetName returns the bucket name.
func (m *BucketCloudMetadata) GetName() string {
	return m.Exports.GetRef(BucketRefBucket)
}

// Bucket describes a bucket.
type Bucket interface {
	Plugin
	GetConfig() *BucketConfig
	GetLocalMetadata() *BucketLocalMetadata
	GetCloudMetadata(require bool) *BucketCloudMetadata
}

type bucketImpl struct {
	cfgFunc       BucketConfigFunc
	deps          *BucketDependencies
	cfg           *BucketConfig
	localMetadata *BucketLocalMetadata
	cloudMetadata *BucketCloudMetadata
}

// NewBucket initializes a new Bucket.
func NewBucket(cfgFunc BucketConfigFunc, deps *BucketDependencies) Bucket {
	deps.MustValidate()

	return &bucketImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*bucketImpl) GetDisplayName() string {
	return BucketPluginDisplayName
}

// GetName implements the Plugin interface.
func (p *bucketImpl) GetName() string {
	return BucketPluginName
}

// GetInstanceName implements the Plugin interface.
func (p *bucketImpl) GetInstanceName() *string {
	return stringz.Ptr(p.cfg.Name)
}

// GetDependenciesMap implements the Plugin interface.
func (p *bucketImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{}
	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}
	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *bucketImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *bucketImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(BucketPluginDisplayName))
	return p.cfg.Stage
}

// GetConfig implements the Bucket interface.
func (p *bucketImpl) GetConfig() *BucketConfig {
	return p.cfg
}

// GetLocalMetadata implements the Bucket interface.
func (p *bucketImpl) GetLocalMetadata() *BucketLocalMetadata {
	errorz.Assertf(p.localMetadata != nil, "local not deployed", errorz.Prefix(BucketPluginDisplayName))
	return p.localMetadata
}

// GetCloudMetadata implements the Bucket interface.
func (p *bucketImpl) GetCloudMetadata(require bool) *BucketCloudMetadata {
	errorz.Assertf(!require || p.cloudMetadata != nil, "cloud not deployed", errorz.Prefix(BucketPluginDisplayName))
	return p.cloudMetadata
}

// IsDeployed implements the Plugin interface.
func (p *bucketImpl) IsDeployed() bool {
	return p.cloudMetadata != nil
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *bucketImpl) UpdateLocalTemplate(tpl *dctypes.Config, _ string) {
	containerName := fmt.Sprintf("%v-%v", p.cfg.Stage.GetConfig().App.GetConfig().Name, BucketPluginName)
	bucketName := fmt.Sprintf("%v-%v", p.cfg.Stage.GetConfig().App.GetConfig().Name, p.cfg.Name)

	bucketSuffix := ""
	if p.cfg.IsPublicAccessEnabled {
		bucketSuffix = ":download"
	}

	p.localMetadata = &BucketLocalMetadata{
		ContainerName:      containerName,
		AccessKey:          LocalAWSAccessKeyID,
		SecretKey:          LocalAWSSecretAccessKey,
		BucketName:         bucketName,
		ExternalURL:        urlz.MustParse(fmt.Sprintf("http://localhost:%v/%v", p.cfg.Local.ExternalPort, bucketName)),
		InternalURL:        urlz.MustParse(fmt.Sprintf("http://%v:%v/%v", containerName, minioPort, bucketName)),
		ConsoleExternalURL: urlz.MustParse(fmt.Sprintf("http://localhost:%v", p.cfg.Local.ConsoleExternalPort)),
	}

	for _, svc := range tpl.Services {
		if svc.Name == containerName {
			defaultBuckets := *svc.Environment["MINIO_DEFAULT_BUCKETS"]
			svc.Environment["MINIO_DEFAULT_BUCKETS"] = stringz.Ptr(defaultBuckets + "," + bucketName + bucketSuffix)
			return
		}
	}

	tpl.Services = append(tpl.Services, dctypes.ServiceConfig{
		Name:          containerName,
		ContainerName: containerName,
		Environment: map[string]*string{
			"MINIO_ROOT_USER":       stringz.Ptr(LocalAWSAccessKeyID),
			"MINIO_ROOT_PASSWORD":   stringz.Ptr(LocalAWSSecretAccessKey),
			"MINIO_ACCESS_KEY":      stringz.Ptr(LocalAWSAccessKeyID),
			"MINIO_SECRET_KEY":      stringz.Ptr(LocalAWSSecretAccessKey),
			"BITNAMI_DEBUG":         stringz.Ptr("true"),
			"MINIO_DEFAULT_BUCKETS": stringz.Ptr(bucketName + bucketSuffix),
		},
		Image:    "bitnami/minio:" + minioVersion,
		Networks: p.cfg.Stage.AsLocalStage().GetServiceNetworkConfig(),
		Ports: []dctypes.ServicePortConfig{
			{
				Target:    minioPort,
				Published: uint32(p.cfg.Local.ExternalPort),
			},
			{
				Target:    minioConsolePort,
				Published: uint32(p.cfg.Local.ConsoleExternalPort),
			},
		},
		Restart: "unless-stopped",
	})
}

// GetCloudTemplate implements the Plugin interface.
func (p *bucketImpl) GetCloudTemplate(_ string) *gocf.Template {
	tpl := gocf.NewTemplate()

	tpl.Resources[BucketRefBucket.Ref()] = &gos3.Bucket{
		BucketName: stringz.Ptr(BucketRefBucket.Name(p)),
		LifecycleConfiguration: &gos3.Bucket_LifecycleConfiguration{
			Rules: []gos3.Bucket_Rule{
				{
					AbortIncompleteMultipartUpload: &gos3.Bucket_AbortIncompleteMultipartUpload{
						DaysAfterInitiation: 30,
					},
					ExpirationInDays: func() *int {
						if p.cfg.Cloud.DeleteObjectsAfterDays != nil {
							return intz.Ptr(int(*p.cfg.Cloud.DeleteObjectsAfterDays))
						}
						return nil
					}(),
					NoncurrentVersionExpirationInDays: func() *int {
						if p.cfg.Cloud.DeletePreviousObjectVersionsAfterDays != nil {
							return intz.Ptr(int(*p.cfg.Cloud.DeletePreviousObjectVersionsAfterDays))
						}
						return nil
					}(),
					Status: "Enabled",
				},
			},
		},
		PublicAccessBlockConfiguration: func() *gos3.Bucket_PublicAccessBlockConfiguration {
			block := boolz.Ptr(true)
			if p.cfg.IsPublicAccessEnabled {
				block = boolz.Ptr(false)
			}

			return &gos3.Bucket_PublicAccessBlockConfiguration{
				BlockPublicAcls:       block,
				BlockPublicPolicy:     block,
				IgnorePublicAcls:      block,
				RestrictPublicBuckets: block,
			}
		}(),
		VersioningConfiguration: func() *gos3.Bucket_VersioningConfiguration {
			if p.cfg.Cloud.IsVersioningEnabled {
				return &gos3.Bucket_VersioningConfiguration{
					Status: "Enabled",
				}
			}
			return nil
		}(),
		Tags: CloudGetDefaultTags(BucketRefBucket.Name(p)),
	}
	CloudAddExpRef(tpl, p, BucketRefBucket)
	CloudAddExpGetAtt(tpl, p, BucketRefBucket, BucketAttARN)
	CloudAddExpGetAtt(tpl, p, BucketRefBucket, BucketAttDomainName)
	CloudAddExpGetAtt(tpl, p, BucketRefBucket, BucketAttDualStackDomainName)
	CloudAddExpGetAtt(tpl, p, BucketRefBucket, BucketAttRegionalDomainName)

	if p.cfg.IsPublicAccessEnabled {
		tpl.Resources[BucketRefBucketPolicyPublic.Ref()] = &gos3.BucketPolicy{
			Bucket: gocf.Ref(BucketRefBucket.Ref()),
			PolicyDocument: NewPolicyDocument(
				NewPolicyStatement().
					SetWildcardPrincipal().
					AddActions("s3:GetObject").
					AddResources(gocf.Join("", []string{
						"arn:aws:s3:::",
						gocf.Ref(BucketRefBucket.Ref()),
						"/*",
					}))),
		}
	}

	return tpl
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *bucketImpl) UpdateCloudMetadata(stack *awscft.Stack) {
	p.cloudMetadata = &BucketCloudMetadata{
		Exports:    NewCloudExports(stack),
		BucketName: BucketRefBucket.Name(p),
		BucketURL:  urlz.MustParse(fmt.Sprintf("https://s3.%v.amazonaws.com/%v", p.cfg.Stage.GetConfig().App.GetConfig().AWSConfig.Region, BucketRefBucket.Name(p))),
	}
}

// EventHook implements the Plugin interface.
func (p *bucketImpl) EventHook(event Event, buildDirPath string) {
	if p.cfg.EventHook != nil {
		p.cfg.EventHook(p, event, buildDirPath)
	}
}
