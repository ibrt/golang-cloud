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

// Mail constants.
const (
	MailPluginDisplayName = "Mail"
	MailPluginName        = "mail"

	mailHogVersion = "1.0.1"
)

var (
	_ Mail   = &mailImpl{}
	_ Plugin = &mailImpl{}
)

// MailConfigFunc returns the mail config for a given Stage.
type MailConfigFunc func(Stage, *MailDependencies) *MailConfig

// MailConfig describes the mail config.
type MailConfig struct {
	Stage Stage `validate:"required"`
	Local *MailConfigLocal
}

// MustValidate validates the mail config.
func (c *MailConfig) MustValidate(stageTarget StageTarget) {
	vz.MustValidateStruct(c)
	errorz.Assertf(stageTarget == Cloud || c.Local != nil, "missing MailConfig.Local")
}

// MailConfigLocal describes part of the mail config.
type MailConfigLocal struct {
	ExternalPort     uint16 `validate:"required"`
	SMTPExternalPort uint16 `validate:"required"`
}

// MailDependencies describes the mail dependencies.
type MailDependencies struct {
	OtherDependencies OtherDependencies
}

// MustValidate validates the mail dependencies.
func (d *MailDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// MailLocalMetadata describes the mail local metadata.
type MailLocalMetadata struct {
	ContainerName string
	ExternalURL   *url.URL
	InternalURL   *url.URL
	ExternalSMTP  *MailLocalMetadataSMTP
	InternalSMTP  *MailLocalMetadataSMTP
}

// MailLocalMetadataSMTP describes part of the mail local metadata.
type MailLocalMetadataSMTP struct {
	Username string
	Password string
	Host     string
	Port     uint16
}

// ToURL converts the MailLocalMetadataSMTP to a "smtp://" URL.
func (s *MailLocalMetadataSMTP) ToURL() *url.URL {
	return urlz.MustParse(fmt.Sprintf("smtp://%v:%v@%v:%v", s.Username, s.Password, s.Host, s.Port))
}

// Mail describes a mail.
type Mail interface {
	Plugin
	GetConfig() *MailConfig
	GetDependencies() *MailDependencies
	GetLocalMetadata() *MailLocalMetadata
}

type mailImpl struct {
	cfgFunc       MailConfigFunc
	deps          *MailDependencies
	cfg           *MailConfig
	localMetadata *MailLocalMetadata
}

// NewMail initializes a new Mail.
func NewMail(cfgFunc MailConfigFunc, deps *MailDependencies) Mail {
	deps.MustValidate()

	return &mailImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*mailImpl) GetDisplayName() string {
	return MailPluginDisplayName
}

// GetName implements the Plugin interface.
func (p *mailImpl) GetName() string {
	return MailPluginName
}

// GetInstanceName implements the Plugin interface.
func (p *mailImpl) GetInstanceName() *string {
	return nil
}

// GetDependenciesMap implements the Plugin interface.
func (p *mailImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{}
	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}
	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *mailImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *mailImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(MailPluginName))
	return p.cfg.Stage
}

// GetConfig implements the Mail interface.
func (p *mailImpl) GetConfig() *MailConfig {
	return p.cfg
}

// GetDependencies implements the Mail interface.
func (p *mailImpl) GetDependencies() *MailDependencies {
	return p.deps
}

// GetLocalMetadata implements the Mail interface.
func (p *mailImpl) GetLocalMetadata() *MailLocalMetadata {
	errorz.Assertf(p.localMetadata != nil, "local not deployed", errorz.Prefix(MailPluginName))
	return p.localMetadata
}

// IsDeployed implements the Plugin interface.
func (p *mailImpl) IsDeployed() bool {
	return false
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *mailImpl) UpdateLocalTemplate(tpl *dctypes.Config, _ string) {
	containerName := LocalGetContainerName(p)

	p.localMetadata = &MailLocalMetadata{
		ContainerName: containerName,
		ExternalURL:   urlz.MustParse(fmt.Sprintf("http://localhost:%v/api/v2", p.cfg.Local.ExternalPort)),
		InternalURL:   urlz.MustParse(fmt.Sprintf("http://%v:%v/api/v2", containerName, p.cfg.Local.ExternalPort)),
		ExternalSMTP: &MailLocalMetadataSMTP{
			Username: "",
			Password: "mailhog",
			Host:     "localhost",
			Port:     p.cfg.Local.SMTPExternalPort,
		},
		InternalSMTP: &MailLocalMetadataSMTP{
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
func (p *mailImpl) GetCloudTemplate(_ string) *gocf.Template {
	// nothing to do here
	return nil
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *mailImpl) UpdateCloudMetadata(_ *awscft.Stack) {
	// nothing to do here
}

// BeforeDeployHook implements the Plugin interface.
func (p *mailImpl) BeforeDeployHook(_ string) {

}

// AfterDeployHook implements the Plugin interface.
func (*mailImpl) AfterDeployHook(_ string) {
	// nothing to do here
}
