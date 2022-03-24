package cloudz

import (
	"fmt"
	"net/url"

	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-bites/urlz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-validation/vz"
)

// Mailer constants.
const (
	MailerPluginDisplayName = "Mailer"
	MailerPluginName        = "mailer"

	mailHogVersion = "1.0.1"
)

var (
	_ Mailer = &mailerImpl{}
	_ Plugin = &mailerImpl{}
)

// MailerConfigFunc returns the mailer config for a given Stage.
type MailerConfigFunc func(Stage, *MailerDependencies) *MailerConfig

// MailerConfig describes the mailer config.
type MailerConfig struct {
	Stage Stage `validate:"required"`
	Local *MailerConfigLocal
}

// MustValidate validates the mailer config.
func (c *MailerConfig) MustValidate(stageTarget StageTarget) {
	vz.MustValidateStruct(c)
	errorz.Assertf(stageTarget == Cloud || c.Local != nil, "missing MailerConfig.Local")
}

// MailerConfigLocal describes part of the mailer config.
type MailerConfigLocal struct {
	ExternalPort     uint16 `validate:"required"`
	SMTPExternalPort uint16 `validate:"required"`
}

// MailerDependencies describes the mailer dependencies.
type MailerDependencies struct {
	OtherDependencies OtherDependencies
}

// MustValidate validates the mailer dependencies.
func (d *MailerDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// MailerLocalMetadata describes the mailer local metadata.
type MailerLocalMetadata struct {
	ContainerName string
	ExternalURL   *url.URL
	InternalURL   *url.URL
	ExternalSMTP  *MailerLocalMetadataSMTP
	InternalSMTP  *MailerLocalMetadataSMTP
}

// MailerLocalMetadataSMTP describes part of the mailer local metadata.
type MailerLocalMetadataSMTP struct {
	Username string
	Password string
	Host     string
	Port     uint16
}

// Mailer describes a mailer.
type Mailer interface {
	Plugin
	GetConfig() *MailerConfig
	GetDependencies() *MailerDependencies
	GetLocalMetadata() *MailerLocalMetadata
}

type mailerImpl struct {
	cfgFunc       MailerConfigFunc
	deps          *MailerDependencies
	cfg           *MailerConfig
	localMetadata *MailerLocalMetadata
}

// NewMailer initializes a new Mailer.
func NewMailer(cfgFunc MailerConfigFunc, deps *MailerDependencies) Mailer {
	deps.MustValidate()

	return &mailerImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*mailerImpl) GetDisplayName() string {
	return MailerPluginDisplayName
}

// GetName implements the Plugin interface.
func (p *mailerImpl) GetName() string {
	return MailerPluginName
}

// GetInstanceName implements the Plugin interface.
func (p *mailerImpl) GetInstanceName() *string {
	return nil
}

// GetDependenciesMap implements the Plugin interface.
func (p *mailerImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{}
	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}
	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *mailerImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *mailerImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(MailerPluginName))
	return p.cfg.Stage
}

// GetConfig implements the Mailer interface.
func (p *mailerImpl) GetConfig() *MailerConfig {
	return p.cfg
}

// GetDependencies implements the Mailer interface.
func (p *mailerImpl) GetDependencies() *MailerDependencies {
	return p.deps
}

// GetLocalMetadata implements the Mailer interface.
func (p *mailerImpl) GetLocalMetadata() *MailerLocalMetadata {
	errorz.Assertf(p.localMetadata != nil, "local not deployed", errorz.Prefix(MailerPluginName))
	return p.localMetadata
}

// IsDeployed implements the Plugin interface.
func (p *mailerImpl) IsDeployed() bool {
	return false
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *mailerImpl) UpdateLocalTemplate(tpl *dctypes.Config, _ string) {
	containerName := LocalGetContainerName(p)

	p.localMetadata = &MailerLocalMetadata{
		ContainerName: containerName,
		ExternalURL:   urlz.MustParse(fmt.Sprintf("http://localhost:%v/api/v2", p.cfg.Local.ExternalPort)),
		InternalURL:   urlz.MustParse(fmt.Sprintf("http://%v:%v/api/v2", containerName, p.cfg.Local.ExternalPort)),
		ExternalSMTP: &MailerLocalMetadataSMTP{
			Username: "",
			Password: "mailhog",
			Host:     "localhost",
			Port:     p.cfg.Local.SMTPExternalPort,
		},
		InternalSMTP: &MailerLocalMetadataSMTP{
			Username: "",
			Password: "mailhog",
			Host:     containerName,
			Port:     p.cfg.Local.SMTPExternalPort,
		},
	}

	tpl.Services = append(tpl.Services, dctypes.ServiceConfig{
		Name:          containerName,
		ContainerName: containerName,
		Environment: map[string]*string{
			"MH_HOSTNAME":       stringz.Ptr("localhost"),
			"MH_UI_BIND_ADDR":   stringz.Ptr(fmt.Sprintf("0.0.0.0:%v", p.cfg.Local.ExternalPort)),
			"MH_API_BIND_ADDR":  stringz.Ptr(fmt.Sprintf("0.0.0.0:%v", p.cfg.Local.ExternalPort)),
			"MH_SMTP_BIND_ADDR": stringz.Ptr(fmt.Sprintf("0.0.0.0:%v", p.cfg.Local.SMTPExternalPort)),
			"MH_STORAGE":        stringz.Ptr("memory"),
		},
		Image:    "mailhog/mailhog:v" + mailHogVersion,
		Networks: p.cfg.Stage.AsLocalStage().GetServiceNetworkConfig(),
		Ports: []dctypes.ServicePortConfig{
			{
				Target:    uint32(p.cfg.Local.ExternalPort),
				Published: uint32(p.cfg.Local.ExternalPort),
			},
			{
				Target:    uint32(p.cfg.Local.SMTPExternalPort),
				Published: uint32(p.cfg.Local.SMTPExternalPort),
			},
		},
		Restart: "unless-stopped",
	})
}

// GetCloudTemplate implements the Plugin interface.
func (p *mailerImpl) GetCloudTemplate(_ string) *gocf.Template {
	// nothing to do here
	return nil
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *mailerImpl) UpdateCloudMetadata(_ *awscft.Stack) {
	// nothing to do here
}

// BeforeDeployHook implements the Plugin interface.
func (p *mailerImpl) BeforeDeployHook(_ string) {

}

// AfterDeployHook implements the Plugin interface.
func (*mailerImpl) AfterDeployHook(_ string) {
	// nothing to do here
}
