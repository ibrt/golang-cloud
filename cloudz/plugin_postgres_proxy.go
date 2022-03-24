package cloudz

import (
	"fmt"
	"net/url"

	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	goiam "github.com/awslabs/goformation/v6/cloudformation/iam"
	gologs "github.com/awslabs/goformation/v6/cloudformation/logs"
	gords "github.com/awslabs/goformation/v6/cloudformation/rds"
	gosm "github.com/awslabs/goformation/v6/cloudformation/secretsmanager"
	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/boolz"
	"github.com/ibrt/golang-bites/jsonz"
	"github.com/ibrt/golang-bites/numeric/intz"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-validation/vz"
)

// Postgres proxy constants.
const (
	PostgresProxyPluginDisplayName     = "PostgresProxy"
	PostgresProxyPluginName            = "postgres-proxy"
	PostgresProxyRefSecret             = CloudRef("s")
	PostgresProxyRefRole               = CloudRef("r")
	PostgresProxyRefLogGroup           = CloudRef("lg")
	PostgresProxyRefDBProxy            = CloudRef("p")
	PostgresProxyRefDBProxyTargetGroup = CloudRef("tg")
	PostgresProxyAttARN                = CloudAtt("Arn")
	PostgresProxyAttRoleID             = CloudAtt("RoleId")
	PostgresProxyAttDBProxyARN         = CloudAtt("DBProxyArn")
	PostgresProxyAttEndpoint           = CloudAtt("Endpoint")
	PostgresProxyAttTargetGroupARN     = CloudAtt("TargetGroupArn")
)

var (
	_ PostgresProxy = &postgresProxyImpl{}
	_ Plugin        = &postgresProxyImpl{}
)

// PostgresProxyConfigFunc returns the postgres proxy config for a given Stage.
type PostgresProxyConfigFunc func(Stage, *PostgresProxyDependencies) *PostgresProxyConfig

// PostgresProxyConfig describes the postgres proxy config.
type PostgresProxyConfig struct {
	Stage Stage `validate:"required"`
}

// MustValidate validates the postgres proxy config.
func (c *PostgresProxyConfig) MustValidate(_ StageTarget) {
	vz.MustValidateStruct(c)
}

// PostgresProxyDependencies describes the postgres proxy dependencies.
type PostgresProxyDependencies struct {
	Network           Network  `validate:"required"`
	Postgres          Postgres `validate:"required"`
	OtherDependencies []Plugin
}

// MustValidate validates the postgres proxy dependencies.
func (d *PostgresProxyDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// PostgresProxyLocalMetadata describes the postgres proxy local metadata.
type PostgresProxyLocalMetadata struct {
	ExternalURL *url.URL
	InternalURL *url.URL
}

// PostgresProxyCloudMetadata describes the postgres proxy cloud metadata.
type PostgresProxyCloudMetadata struct {
	Exports CloudExports
	URL     string
}

// PostgresProxy describes a postgres proxy.
type PostgresProxy interface {
	Plugin
	GetConfig() *PostgresProxyConfig
	GetCloudMetadata() *PostgresProxyCloudMetadata
}

type postgresProxyImpl struct {
	cfgFunc       PostgresProxyConfigFunc
	deps          *PostgresProxyDependencies
	cfg           *PostgresProxyConfig
	localMetadata *PostgresProxyLocalMetadata
	cloudMetadata *PostgresProxyCloudMetadata
}

// NewPostgresProxy initializes a new PostgresProxy.
func NewPostgresProxy(cfgFunc PostgresProxyConfigFunc, deps *PostgresProxyDependencies) PostgresProxy {
	deps.MustValidate()

	return &postgresProxyImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*postgresProxyImpl) GetDisplayName() string {
	return PostgresProxyPluginDisplayName
}

// GetName implements the Plugin interface.
func (p *postgresProxyImpl) GetName() string {
	return PostgresProxyPluginName
}

// GetInstanceName implements the Plugin interface.
func (p *postgresProxyImpl) GetInstanceName() *string {
	return nil
}

// GetDependenciesMap implements the Plugin interface.
func (p *postgresProxyImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{
		p.deps.Network:  {},
		p.deps.Postgres: {},
	}

	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}

	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *postgresProxyImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *postgresProxyImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(PostgresProxyPluginName))
	return p.cfg.Stage
}

// GetConfig implements the PostgresProxy interface.
func (p *postgresProxyImpl) GetConfig() *PostgresProxyConfig {
	return p.cfg
}

// GetCloudMetadata implements the PostgresProxy interface.
func (p *postgresProxyImpl) GetCloudMetadata() *PostgresProxyCloudMetadata {
	errorz.Assertf(p.cloudMetadata != nil, "cloud not deployed", errorz.Prefix(PostgresProxyPluginName))
	return p.cloudMetadata
}

// IsDeployed implements the Plugin interface.
func (p *postgresProxyImpl) IsDeployed() bool {
	return p.cloudMetadata != nil
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *postgresProxyImpl) UpdateLocalTemplate(_ *dctypes.Config, _ string) {
	p.localMetadata = &PostgresProxyLocalMetadata{
		ExternalURL: p.deps.Postgres.GetLocalMetadata().ExternalURL,
		InternalURL: p.deps.Postgres.GetLocalMetadata().InternalURL,
	}
}

// GetCloudTemplate implements the Plugin interface.
func (p *postgresProxyImpl) GetCloudTemplate(_ string) *gocf.Template {
	tpl := gocf.NewTemplate()

	tpl.Resources[PostgresProxyRefSecret.Ref()] = &gosm.Secret{
		Name: stringz.Ptr(PostgresProxyRefSecret.Name(p)),
		SecretString: stringz.Ptr(jsonz.MustMarshalIndentDefaultString(map[string]interface{}{
			"username": p.cfg.Stage.GetName(),
			"password": p.deps.Postgres.GetConfig().Cloud.Password,
		})),
		Tags: CloudGetDefaultTags(PostgresProxyRefSecret.Name(p)),
	}
	CloudAddExpRef(tpl, p, PostgresProxyRefSecret)

	tpl.Resources[PostgresProxyRefRole.Ref()] = &goiam.Role{
		AssumeRolePolicyDocument: NewAssumeRolePolicyDocument("rds.amazonaws.com"),
		Policies: &[]goiam.Role_Policy{
			{
				PolicyName: "Policy",
				PolicyDocument: NewPolicyDocument(
					NewPolicyStatement().
						AddActions("secretsmanager:GetSecretValue").
						AddResources(gocf.Ref(PostgresProxyRefSecret.Ref())),
					NewPolicyStatement().
						AddActions("kms:Decrypt").
						AddResources(gocf.Sub("arn:aws:kms:${AWS::Region}:${AWS::AccountId}:key/aws/secretsmanager"))),
			},
		},
		RoleName: stringz.Ptr(PostgresProxyRefRole.Name(p)),
		Tags:     CloudGetDefaultTags(PostgresProxyRefRole.Name(p)),
	}
	CloudAddExpRef(tpl, p, PostgresProxyRefSecret)
	CloudAddExpGetAtt(tpl, p, PostgresProxyRefSecret, PostgresProxyAttARN)
	CloudAddExpGetAtt(tpl, p, PostgresProxyRefSecret, PostgresProxyAttRoleID)

	tpl.Resources[PostgresProxyRefLogGroup.Ref()] = &gologs.LogGroup{
		LogGroupName:    stringz.Ptr(PostgresProxyRefLogGroup.Name(p)),
		RetentionInDays: intz.Ptr(90),
	}
	CloudAddExpRef(tpl, p, PostgresProxyRefLogGroup)
	CloudAddExpGetAtt(tpl, p, PostgresProxyRefLogGroup, PostgresProxyAttARN)

	tpl.Resources[PostgresProxyRefDBProxy.Ref()] = &gords.DBProxy{
		AWSCloudFormationDependsOn: []string{
			PostgresProxyRefLogGroup.Ref(),
		},
		Auth: []gords.DBProxy_AuthFormat{
			{
				AuthScheme: stringz.Ptr("SECRETS"),
				IAMAuth:    stringz.Ptr("DISABLED"),
				SecretArn:  stringz.Ptr(gocf.Ref(PostgresProxyRefSecret.Ref())),
			},
		},
		DBProxyName:  PostgresProxyRefSecret.Name(p),
		DebugLogging: boolz.Ptr(p.cfg.Stage.GetMode().IsStaging()),
		EngineFamily: "POSTGRESQL",
		RequireTLS:   boolz.Ptr(true),
		RoleArn:      gocf.GetAtt(PostgresProxyRefRole.Ref(), "Arn"),
		VpcSecurityGroupIds: &[]string{
			p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefSecurityGroup),
		},
		VpcSubnetIds: []string{
			p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefSubnetPublicA),
			p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefSubnetPublicB),
		},
		Tags: &[]gords.DBProxy_TagFormat{
			{
				Key:   stringz.Ptr("Name"),
				Value: stringz.Ptr(PostgresProxyRefDBProxy.Name(p)),
			},
		},
	}
	CloudAddExpRef(tpl, p, PostgresProxyRefDBProxy)
	CloudAddExpGetAtt(tpl, p, PostgresProxyRefDBProxy, PostgresProxyAttDBProxyARN)
	CloudAddExpGetAtt(tpl, p, PostgresProxyRefDBProxy, PostgresProxyAttEndpoint)

	tpl.Resources[PostgresProxyRefDBProxyTargetGroup.Ref()] = &gords.DBProxyTargetGroup{
		ConnectionPoolConfigurationInfo: &gords.DBProxyTargetGroup_ConnectionPoolConfigurationInfoFormat{
			ConnectionBorrowTimeout: intz.Ptr(10),
		},
		DBInstanceIdentifiers: &[]string{
			p.deps.Postgres.GetCloudMetadata().Exports.GetRef(PostgresRefDBInstance),
		},
		DBProxyName:     gocf.Ref(PostgresProxyRefDBProxy.Ref()),
		TargetGroupName: "default",
	}
	CloudAddExpRef(tpl, p, PostgresProxyRefDBProxyTargetGroup)
	CloudAddExpGetAtt(tpl, p, PostgresProxyRefDBProxyTargetGroup, PostgresProxyAttTargetGroupARN)

	return tpl
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *postgresProxyImpl) UpdateCloudMetadata(stack *awscft.Stack) {
	exports := NewCloudExports(stack)

	p.cloudMetadata = &PostgresProxyCloudMetadata{
		Exports: exports,
		URL: fmt.Sprintf("postgres://%v:%v@%v/%v",
			p.cfg.Stage.GetName(),
			p.deps.Postgres.GetConfig().Cloud.Password,
			exports.GetAtt(PostgresProxyRefDBProxy, PostgresProxyAttEndpoint),
			p.cfg.Stage.GetName()),
	}
}

// BeforeDeployHook implements the Plugin interface.
func (*postgresProxyImpl) BeforeDeployHook(_ string) {
	// nothing to do here
}

// AfterDeployHook implements the Plugin interface.
func (*postgresProxyImpl) AfterDeployHook(_ string) {
	// nothing to do here
}
