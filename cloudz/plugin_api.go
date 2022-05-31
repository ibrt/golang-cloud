package cloudz

import (
	"crypto/sha1"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	goapigwv2 "github.com/awslabs/goformation/v6/cloudformation/apigatewayv2"
	golambda "github.com/awslabs/goformation/v6/cloudformation/lambda"
	goroute53 "github.com/awslabs/goformation/v6/cloudformation/route53"
	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/boolz"
	"github.com/ibrt/golang-bites/filez"
	"github.com/ibrt/golang-bites/jsonz"
	"github.com/ibrt/golang-bites/numeric/intz"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-bites/templatez"
	"github.com/ibrt/golang-bites/urlz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-lambda/lambdaz/testlambdaz"
	"github.com/ibrt/golang-validation/vz"

	"github.com/ibrt/golang-cloud/cloudz/internal/assets"
)

// API constants.
const (
	APIPluginDisplayName       = "API"
	APIPluginName              = "api"
	APIRefAPI                  = CloudRef("api")
	APIRefPermission           = CloudRef("perm")
	APIRefDomainName           = CloudRef("dn")
	APIRefRecordSet            = CloudRef("rs")
	APIRefStage                = CloudRef("stg")
	APIRefAPIMapping           = CloudRef("dnmap")
	APIRefIntegration          = CloudRef("intg")
	APIAttAPIEndpoint          = CloudAtt("ApiEndpoint")
	APIAttRegionalDomainName   = CloudAtt("RegionalDomainName")
	APIAttRegionalHostedZoneID = CloudAtt("RegionalHostedZoneId")
)

var (
	_ API    = &apiImpl{}
	_ Plugin = &apiImpl{}
)

// APIConfigFunc returns the api config for a given Stage.
type APIConfigFunc func(Stage, *APIDependencies) *APIConfig

// APIEventHookFunc describes an api event hook.
type APIEventHookFunc func(API, Event, string)

// APIConfig describes the api config.
type APIConfig struct {
	Stage     Stage    `validate:"required"`
	Name      string   `validate:"required,resource-name"`
	RouteKeys []string `validate:"required"`
	Local     *APIConfigLocal
	Cloud     *APIConfigCloud
	EventHook APIEventHookFunc
}

// MustValidate validates the api config.
func (c *APIConfig) MustValidate(stageTarget StageTarget) {
	vz.MustValidateStruct(c)
	errorz.Assertf(stageTarget == Cloud || c.Local != nil, "missing APIConfig.Local")
	errorz.Assertf(stageTarget == Local || c.Cloud != nil, "missing APIConfig.Cloud")
}

// APIConfigLocal describes part of the api config.
type APIConfigLocal struct {
	ExternalPort uint16 `validate:"required"`
}

// APIConfigCloud describes part of the api config.
type APIConfigCloud struct {
	DomainName string `validate:"required,fqdn"`
	CORSDomain string `validate:"required,fqdn"`
}

// APIDependencies describes the api dependencies.
type APIDependencies struct {
	Certificate       Certificate `validate:"required"`
	Function          Function    `validate:"required"`
	OtherDependencies OtherDependencies
}

// MustValidate validates the function dependencies.
func (d *APIDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// APILocalMetadata describes the api local metadata.
type APILocalMetadata struct {
	ExternalURL *url.URL
	InternalURL *url.URL
}

// APICloudMetadata describes the api cloud metadata.
type APICloudMetadata struct {
	Exports CloudExports
	URL     *url.URL
}

// API describes an api.
type API interface {
	Plugin
	GetConfig() *APIConfig
	GetLocalMetadata() *APILocalMetadata
	GetCloudMetadata(require bool) *APICloudMetadata
}

type apiImpl struct {
	cfgFunc       APIConfigFunc
	deps          *APIDependencies
	cfg           *APIConfig
	localMetadata *APILocalMetadata
	cloudMetadata *APICloudMetadata
}

// NewAPI initializes a new API.
func NewAPI(cfgFunc APIConfigFunc, deps *APIDependencies) API {
	deps.MustValidate()

	return &apiImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*apiImpl) GetDisplayName() string {
	return APIPluginDisplayName
}

// GetName implements the Plugin interface.
func (p *apiImpl) GetName() string {
	return APIPluginName
}

// GetInstanceName implements the Plugin interface.
func (p *apiImpl) GetInstanceName() *string {
	return stringz.Ptr(p.cfg.Name)
}

// GetDependenciesMap implements the Plugin interface.
func (p *apiImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{
		p.deps.Certificate: {},
		p.deps.Function:    {},
	}

	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}

	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *apiImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *apiImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(APIPluginName))
	return p.cfg.Stage
}

// GetConfig implements the API interface.
func (p *apiImpl) GetConfig() *APIConfig {
	return p.cfg
}

// GetLocalMetadata implements the API interface.
func (p *apiImpl) GetLocalMetadata() *APILocalMetadata {
	errorz.Assertf(p.localMetadata != nil, "local not deployed", errorz.Prefix(APIPluginName))
	return p.localMetadata
}

// GetCloudMetadata implements the API interface.
func (p *apiImpl) GetCloudMetadata(require bool) *APICloudMetadata {
	errorz.Assertf(!require || p.cloudMetadata != nil, "cloud not deployed", errorz.Prefix(APIPluginName))
	return p.cloudMetadata
}

// IsDeployed implements the Plugin interface.
func (p *apiImpl) IsDeployed() bool {
	return p.cloudMetadata != nil
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *apiImpl) UpdateLocalTemplate(tpl *dctypes.Config, buildDirPath string) {
	containerName := LocalGetContainerName(p)

	p.localMetadata = &APILocalMetadata{
		ExternalURL: urlz.MustParse(fmt.Sprintf("http://localhost:%v", p.cfg.Local.ExternalPort)),
		InternalURL: urlz.MustParse(fmt.Sprintf("http://%v:%v", containerName, p.cfg.Local.ExternalPort)),
	}

	tpl.Services = append(tpl.Services, dctypes.ServiceConfig{
		Name: containerName,
		Build: dctypes.BuildConfig{
			Context: buildDirPath,
		},
		ContainerName: containerName,
		Image:         containerName,
		Networks:      p.GetConfig().Stage.AsLocalStage().GetServiceNetworkConfig(),
		Ports: []dctypes.ServicePortConfig{
			{
				Target:    uint32(p.cfg.Local.ExternalPort),
				Published: uint32(p.cfg.Local.ExternalPort),
			},
		},
		Restart: "unless-stopped",
	})
}

// GetCloudTemplate implements the Plugin interface.
func (p *apiImpl) GetCloudTemplate(_ string) *gocf.Template {
	tpl := gocf.NewTemplate()

	tpl.Resources[APIRefAPI.Ref()] = &goapigwv2.Api{
		CorsConfiguration: &goapigwv2.Api_Cors{
			AllowCredentials: boolz.Ptr(false),
			AllowHeaders:     &[]string{"*"},
			AllowMethods: &[]string{
				"GET",
				"POST",
			},
			AllowOrigins: &[]string{
				p.cfg.Cloud.CORSDomain,
			},
			MaxAge: intz.Ptr(86400),
		},
		Name:         stringz.Ptr(APIRefAPI.Name(p)),
		ProtocolType: stringz.Ptr("HTTP"),
	}
	CloudAddExpRef(tpl, p, APIRefAPI)
	CloudAddExpGetAtt(tpl, p, APIRefAPI, APIAttAPIEndpoint)

	tpl.Resources[APIRefPermission.Ref()] = &golambda.Permission{
		Action:       "lambda:InvokeFunction",
		FunctionName: p.deps.Function.GetCloudMetadata(true).GetARN(),
		Principal:    "apigateway.amazonaws.com",
		SourceArn: stringz.Ptr(gocf.Join("", []string{
			gocf.Sub("arn:aws:execute-api:${AWS::Region}:${AWS::AccountId}:"),
			gocf.Ref(APIRefAPI.Ref),
			"/*/*",
		})),
	}

	tpl.Resources[APIRefDomainName.Ref()] = &goapigwv2.DomainName{
		DomainName: p.cfg.Cloud.DomainName,
		DomainNameConfigurations: &[]goapigwv2.DomainName_DomainNameConfiguration{
			{
				CertificateArn: stringz.Ptr(p.deps.Certificate.GetCloudMetadata(true).ARN),
				EndpointType:   stringz.Ptr("REGIONAL"),
			},
		},
	}
	CloudAddExpRef(tpl, p, APIRefDomainName)
	CloudAddExpGetAtt(tpl, p, APIRefDomainName, APIAttRegionalDomainName)
	CloudAddExpGetAtt(tpl, p, APIRefDomainName, APIAttRegionalHostedZoneID)

	tpl.Resources[APIRefRecordSet.Ref()] = &goroute53.RecordSet{
		AliasTarget: &goroute53.RecordSet_AliasTarget{
			DNSName:      p.deps.Function.GetCloudMetadata(true).GetARN(),
			HostedZoneId: p.deps.Certificate.GetConfig().Cloud.HostedZoneID,
		},
		// TODO(ibrt): Figure out if we actually needed HostedZoneName instead.
		HostedZoneId:     stringz.Ptr(p.deps.Certificate.GetConfig().Cloud.HostedZoneID),
		MultiValueAnswer: boolz.Ptr(false),
		Name:             p.cfg.Cloud.DomainName,
		Type:             "A",
	}
	CloudAddExpRef(tpl, p, APIRefRecordSet)

	tpl.Resources[APIRefStage.Ref()] = &goapigwv2.Stage{
		ApiId:      gocf.Ref(APIRefAPI.Ref()),
		AutoDeploy: boolz.Ptr(true),
		StageName:  "$default",
	}
	CloudAddExpRef(tpl, p, APIRefStage)

	tpl.Resources[APIRefAPIMapping.Ref()] = &goapigwv2.ApiMapping{
		ApiId:      gocf.Ref(APIRefAPI.Ref()),
		DomainName: p.cfg.Cloud.DomainName,
		Stage:      gocf.Ref(APIRefStage.Ref()),
		AWSCloudFormationDependsOn: []string{
			APIRefDomainName.Ref(),
		},
	}
	CloudAddExpRef(tpl, p, APIRefAPIMapping)

	tpl.Resources[APIRefIntegration.Ref()] = &goapigwv2.Integration{
		ApiId:                gocf.Ref(APIRefAPI.Ref()),
		IntegrationType:      "AWS_PROXY",
		IntegrationUri:       stringz.Ptr(p.deps.Function.GetCloudMetadata(true).GetARN()),
		PayloadFormatVersion: stringz.Ptr("2.0"),
		TimeoutInMillis:      intz.Ptr(29000),
	}
	CloudAddExpRef(tpl, p, APIRefIntegration)

	for _, routeKey := range p.cfg.RouteKeys {
		tpl.Resources[CloudRef(fmt.Sprintf("r-%x", sha1.Sum([]byte(routeKey)))).Ref()] = &goapigwv2.Route{
			ApiId:             gocf.Ref(APIRefAPI.Ref()),
			AuthorizationType: stringz.Ptr("NONE"),
			RouteKey:          routeKey,
			Target: stringz.Ptr(gocf.Join("", []string{
				"integrations/",
				gocf.Ref(APIRefIntegration.Ref()),
			})),
		}
	}

	return tpl
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *apiImpl) UpdateCloudMetadata(stack *awscft.Stack) {
	p.cloudMetadata = &APICloudMetadata{
		Exports: NewCloudExports(stack),
		URL:     urlz.MustParse(fmt.Sprintf("https://%v", p.cfg.Cloud.DomainName)),
	}
}

// EventHook implements the Plugin interface.
func (p *apiImpl) EventHook(event Event, buildDirPath string) {
	switch event {
	case LocalBeforeCreateEvent:
		p.localBeforeCreateEventHook(buildDirPath)
	}

	if p.cfg.EventHook != nil {
		p.cfg.EventHook(p, event, buildDirPath)
	}
}

func (p *apiImpl) localBeforeCreateEventHook(buildDirPath string) {
	filez.MustPrepareDir(buildDirPath, 0777)

	filez.MustWriteFile(
		filepath.Join(buildDirPath, "Dockerfile"), 0777, 0666,
		templatez.MustParseAndExecuteText(
			assets.HTTPAPIDockerfileTemplateAsset,
			assets.HTTPAPIDockerfileTemplateData{
				GoVersion:  strings.TrimPrefix(runtime.Version(), "go"),
				ListenAddr: fmt.Sprintf(":%v", p.cfg.Local.ExternalPort),
			}))

	cfg := &testlambdaz.HTTPSimulatorConfig{
		Routes: func() map[string]*testlambdaz.HTTPSimulatorConfigRoute {
			m := make(map[string]*testlambdaz.HTTPSimulatorConfigRoute)
			for _, routeKey := range p.cfg.RouteKeys {
				m[routeKey] = &testlambdaz.HTTPSimulatorConfigRoute{
					IntegrationName: "function",
				}
			}
			return m
		}(),
		AWSProxyIntegrations: map[string]*testlambdaz.HTTPSimulatorConfigAWSProxyIntegration{
			"function": {
				URL: p.deps.Function.GetLocalMetadata().InternalURL.String(),
			},
		},
	}

	filez.MustWriteFile(
		filepath.Join(buildDirPath, "config.json"), 0777, 0666,
		jsonz.MustMarshalIndentDefault(cfg))
}
