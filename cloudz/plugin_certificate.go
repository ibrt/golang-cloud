package cloudz

import (
	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	gocm "github.com/awslabs/goformation/v6/cloudformation/certificatemanager"
	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-validation/vz"
)

// Certificate constants.
const (
	CertificatePluginDisplayName = "Certificate"
	CertificatePluginName        = "certificate"
	CertificateRefCertificate    = CloudRef("c")
)

var (
	_ Certificate = &certificateImpl{}
	_ Plugin      = &certificateImpl{}
)

// CertificateConfigFunc returns the certificate for a given Stage.
type CertificateConfigFunc func(Stage, *CertificateDependencies) *CertificateConfig

// CertificateEventHookFunc describes a certificate event hook.
type CertificateEventHookFunc func(Certificate, Event, string)

// CertificateConfig describes the certificate config.
type CertificateConfig struct {
	Stage     Stage  `validate:"required"`
	Name      string `validate:"required,resource-name"`
	Cloud     *CertificateConfigCloud
	EventHook CertificateEventHookFunc
}

// MustValidate validates the certificate config.
func (c *CertificateConfig) MustValidate(stageTarget StageTarget) {
	vz.MustValidateStruct(c)
	errorz.Assertf(stageTarget == Local || c.Cloud != nil, "missing CertificateConfig.Cloud")
}

// CertificateConfigCloud describes part of the certificate config.
type CertificateConfigCloud struct {
	DomainName   string `validate:"required"`
	HostedZoneID string `validate:"required"`
}

// CertificateDependencies describes the certificate dependencies.
type CertificateDependencies struct {
	OtherDependencies OtherDependencies
}

// MustValidate validates the certificate dependencies.
func (d *CertificateDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// CertificateCloudMetadata describes the certificate cloud metadata.
type CertificateCloudMetadata struct {
	Exports CloudExports
	ARN     string
}

// Certificate describes a certificate.
type Certificate interface {
	Plugin
	GetConfig() *CertificateConfig
	GetCloudMetadata(require bool) *CertificateCloudMetadata
}

type certificateImpl struct {
	cfgFunc       CertificateConfigFunc
	deps          *CertificateDependencies
	cfg           *CertificateConfig
	cloudMetadata *CertificateCloudMetadata
}

// NewCertificate initializes a new Certificate.
func NewCertificate(cfgFunc CertificateConfigFunc, deps *CertificateDependencies) Certificate {
	deps.MustValidate()

	return &certificateImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*certificateImpl) GetDisplayName() string {
	return CertificatePluginDisplayName
}

// GetName implements the Plugin interface.
func (p *certificateImpl) GetName() string {
	return CertificatePluginName
}

// GetInstanceName implements the Plugin interface.
func (p *certificateImpl) GetInstanceName() *string {
	return stringz.Ptr(p.cfg.Name)
}

// GetDependenciesMap implements the Plugin interface.
func (p *certificateImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{}
	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}
	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *certificateImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *certificateImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(CertificatePluginName))
	return p.cfg.Stage
}

// GetConfig implements the Certificate interface.
func (p *certificateImpl) GetConfig() *CertificateConfig {
	return p.cfg
}

// GetCloudMetadata implements the Certificate interface.
func (p *certificateImpl) GetCloudMetadata(require bool) *CertificateCloudMetadata {
	errorz.Assertf(!require || p.cloudMetadata != nil, "cloud not deployed", errorz.Prefix(CertificatePluginName))
	return p.cloudMetadata
}

// IsDeployed implements the Plugin interface.
func (p *certificateImpl) IsDeployed() bool {
	return p.cloudMetadata != nil
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *certificateImpl) UpdateLocalTemplate(_ *dctypes.Config, _ string) {
	// nothing to do here
}

// GetCloudTemplate implements the Plugin interface.
func (p *certificateImpl) GetCloudTemplate(_ string) *gocf.Template {
	tpl := gocf.NewTemplate()

	tpl.Resources[CertificateRefCertificate.Ref()] = &gocm.Certificate{
		DomainName: p.cfg.Cloud.DomainName,
		DomainValidationOptions: &[]gocm.Certificate_DomainValidationOption{
			{
				DomainName:   p.cfg.Cloud.DomainName,
				HostedZoneId: stringz.Ptr(p.cfg.Cloud.HostedZoneID),
			},
		},
		ValidationMethod: stringz.Ptr("DNS"),
		Tags:             CloudGetDefaultTags(CertificateRefCertificate.Name(p)),
	}
	CloudAddExpRef(tpl, p, CertificateRefCertificate)

	return tpl
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *certificateImpl) UpdateCloudMetadata(stack *awscft.Stack) {
	exports := NewCloudExports(stack)

	p.cloudMetadata = &CertificateCloudMetadata{
		Exports: exports,
		ARN:     exports.GetRef(CertificateRefCertificate),
	}
}

// EventHook implements the Plugin interface.
func (p *certificateImpl) EventHook(event Event, buildDirPath string) {
	if p.cfg.EventHook != nil {
		p.cfg.EventHook(p, event, buildDirPath)
	}
}
