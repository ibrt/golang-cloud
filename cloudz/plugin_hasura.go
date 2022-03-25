package cloudz

import (
	"crypto/rsa"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	goecs "github.com/awslabs/goformation/v6/cloudformation/ecs"
	elbv2 "github.com/awslabs/goformation/v6/cloudformation/elasticloadbalancingv2"
	goiam "github.com/awslabs/goformation/v6/cloudformation/iam"
	gologs "github.com/awslabs/goformation/v6/cloudformation/logs"
	goroute53 "github.com/awslabs/goformation/v6/cloudformation/route53"
	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/boolz"
	"github.com/ibrt/golang-bites/filez"
	"github.com/ibrt/golang-bites/jsonz"
	"github.com/ibrt/golang-bites/numeric/intz"
	"github.com/ibrt/golang-bites/rsaz"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-bites/templatez"
	"github.com/ibrt/golang-bites/urlz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-shell/shellz"
	"github.com/ibrt/golang-validation/vz"

	"github.com/ibrt/golang-cloud/cloudz/internal/assets"
)

// Hasura constants.
const (
	HasuraPluginDisplayName      = "Hasura"
	HasuraPluginName             = "hasura"
	HasuraRefLogGroup            = CloudRef("lg")
	HasuraRefRoleExecution       = CloudRef("r-ex")
	HasuraRefRoleTask            = CloudRef("r-tk")
	HasuraRefTaskDefinition      = CloudRef("td")
	HasuraRefTargetGroup         = CloudRef("tg")
	HasuraRefListenerRule        = CloudRef("lr")
	HasuraRefCluster             = CloudRef("cl")
	HasuraRefService             = CloudRef("svc")
	HasuraRefRecordSet           = CloudRef("rs")
	HasuraAttARN                 = CloudAtt("Arn")
	HasuraAttIsDefault           = CloudAtt("IsDefault")
	HasuraAttLoadBalancerARNs    = CloudAtt("LoadBalancerArns")
	HasuraAttName                = CloudAtt("Name")
	HasuraAttRoleID              = CloudAtt("RoleId")
	HasuraAttRuleARN             = CloudAtt("RuleArn")
	HasuraAttTargetGroupFullName = CloudAtt("TargetGroupFullName")
	HasuraAttTargetGroupName     = CloudAtt("TargetGroupName")

	hasuraVersion          = "2.2.2"
	hasuraLocalAdminSecret = "secret"
	hasuraCloudPort        = 7329 // Note: it doesn't really matter as long as it's unique-ish.
)

var (
	_ Hasura = &hasuraImpl{}
	_ Plugin = &hasuraImpl{}

	hasuraConfigDirParts = []string{
		"config",
	}
)

// HasuraConfigFunc returns the hasura config for a given Stage.
type HasuraConfigFunc func(Stage, *HasuraDependencies) *HasuraConfig

// HasuraEventHookFunc describes a hasura event hook.
type HasuraEventHookFunc func(Hasura, Event, string)

// HasuraConfig describes the hasura config.
type HasuraConfig struct {
	Stage            Stage `validate:"required"`
	UnauthorizedRole *string
	JWT              *HasuraConfigJWT `validate:"required"`
	Environment      map[string]string
	Local            *HasuraConfigLocal
	Cloud            *HasuraConfigCloud
	EventHook        HasuraEventHookFunc
}

// MustValidate validates the hasura config.
func (c *HasuraConfig) MustValidate(stageTarget StageTarget) {
	vz.MustValidateStruct(c)
	errorz.Assertf(stageTarget == Cloud || c.Local != nil, "missing HasuraConfig.Local")
	errorz.Assertf(stageTarget == Local || c.Cloud != nil, "missing HasuraConfig.Cloud")
}

// HasuraConfigJWT describes part of the hasura config.
type HasuraConfigJWT struct {
	PublicKey *rsa.PublicKey `validate:"required"`
	Issuer    string         `validate:"required"`
	Audience  string         `validate:"required"`
}

// HasuraConfigLocal describes part of the hasura config.
type HasuraConfigLocal struct {
	ExternalPort           uint16 `validate:"required"`
	ConsoleExternalPort    uint16 `validate:"required"`
	ConsoleAPIExternalPort uint16 `validate:"required"`
	EnableAllowList        bool
}

// HasuraConfigCloud describes part of the hasura config.
type HasuraConfigCloud struct {
	DomainName      string `validate:"required"`
	Replicas        int    `validate:"required"`
	CPU             int    `validate:"required"`
	Memory          int    `validate:"required"`
	AdminSecret     string `validate:"required,min=16"`
	CORSDomain      *string
	EnableAllowList bool
}

// HasuraDependencies describes the hasura dependencies.
type HasuraDependencies struct {
	Certificate       Certificate     `validate:"required"`
	ImageRepository   ImageRepository `validate:"required"`
	LoadBalancer      LoadBalancer    `validate:"required"`
	Network           Network         `validate:"required"`
	Postgres          Postgres        `validate:"required"`
	OtherDependencies OtherDependencies
}

// MustValidate validates the hasura dependencies.
func (d *HasuraDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// HasuraLocalMetadata describes the hasura local metadata.
type HasuraLocalMetadata struct {
	ContainerName        string
	ConsoleContainerName string
	AdminSecret          string
	ExternalURL          *url.URL
	InternalURL          *url.URL
}

// HasuraCloudMetadata describes the hasura cloud metadata.
type HasuraCloudMetadata struct {
	Exports CloudExports
	URL     *url.URL
}

// Hasura describes a hasura.
type Hasura interface {
	Plugin
	GetConfig() *HasuraConfig
	GetDependencies() *HasuraDependencies
	GetLocalMetadata() *HasuraLocalMetadata
	GetCloudMetadata() *HasuraCloudMetadata
	ApplyLocalMetadata()
}

type hasuraImpl struct {
	cfgFunc       HasuraConfigFunc
	deps          *HasuraDependencies
	cfg           *HasuraConfig
	localMetadata *HasuraLocalMetadata
	cloudMetadata *HasuraCloudMetadata
}

// NewHasura initializes a new Hasura.
func NewHasura(cfgFunc HasuraConfigFunc, deps *HasuraDependencies) Hasura {
	deps.MustValidate()

	return &hasuraImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*hasuraImpl) GetDisplayName() string {
	return HasuraPluginDisplayName
}

// GetName implements the Plugin interface.
func (p *hasuraImpl) GetName() string {
	return HasuraPluginName
}

// GetInstanceName implements the Plugin interface.
func (p *hasuraImpl) GetInstanceName() *string {
	return nil
}

// GetDependenciesMap implements the Plugin interface.
func (p *hasuraImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{
		p.deps.Certificate:     {},
		p.deps.ImageRepository: {},
		p.deps.LoadBalancer:    {},
		p.deps.Network:         {},
		p.deps.Postgres:        {},
	}

	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}

	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *hasuraImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *hasuraImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(HasuraPluginName))
	return p.cfg.Stage
}

// GetConfig implements the Hasura interface.
func (p *hasuraImpl) GetConfig() *HasuraConfig {
	return p.cfg
}

// GetDependencies implements the Hasura interface.
func (p *hasuraImpl) GetDependencies() *HasuraDependencies {
	return p.deps
}

// GetLocalMetadata implements the Hasura interface.
func (p *hasuraImpl) GetLocalMetadata() *HasuraLocalMetadata {
	errorz.Assertf(p.localMetadata != nil, "local not deployed", errorz.Prefix(HasuraPluginName))
	return p.localMetadata
}

// GetCloudMetadata implements the Hasura interface.
func (p *hasuraImpl) GetCloudMetadata() *HasuraCloudMetadata {
	errorz.Assertf(p.cloudMetadata != nil, "cloud not deployed", errorz.Prefix(HasuraPluginName))
	return p.cloudMetadata
}

// ApplyLocalMetadata applies migrations & metadata from the config dir to the local Hasura.
func (p *hasuraImpl) ApplyLocalMetadata() {
	p.runCmd("migrate", "--disable-interactive", "apply", "--all-databases")
	p.runCmd("metadata", "apply")
}

// IsDeployed implements the Plugin interface.
func (p *hasuraImpl) IsDeployed() bool {
	return p.cloudMetadata != nil
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *hasuraImpl) UpdateLocalTemplate(tpl *dctypes.Config, buildDirPath string) {
	containerName := LocalGetContainerName(p)
	consoleContainerName := LocalGetContainerName(p, "console")

	p.localMetadata = &HasuraLocalMetadata{
		ContainerName:        containerName,
		ConsoleContainerName: consoleContainerName,
		AdminSecret:          hasuraLocalAdminSecret,
		ExternalURL:          urlz.MustParse(fmt.Sprintf("http://localhost:%v/v1/graphql", p.cfg.Local.ExternalPort)),
		InternalURL:          urlz.MustParse(fmt.Sprintf("http://%v:%v/v1/graphql", containerName, p.cfg.Local.ExternalPort)),
	}

	tpl.Services = append(tpl.Services, dctypes.ServiceConfig{
		Name:          containerName,
		ContainerName: containerName,
		DependsOn: []string{
			p.deps.Postgres.GetLocalMetadata().ContainerName,
		},
		Environment: func() map[string]*string {
			e := map[string]*string{
				"HASURA_GRAPHQL_ADMIN_SECRET":      stringz.Ptr(hasuraLocalAdminSecret),
				"HASURA_GRAPHQL_DATABASE_URL":      stringz.Ptr(p.deps.Postgres.GetLocalMetadata().InternalURL.String()),
				"HASURA_GRAPHQL_DEV_MODE":          stringz.Ptr("true"),
				"HASURA_GRAPHQL_ENABLED_LOG_TYPES": stringz.Ptr("startup, http-log, webhook-log, websocket-log, query-log"),
				"HASURA_GRAPHQL_ENABLE_ALLOWLIST":  stringz.Ptr(fmt.Sprintf("%v", p.cfg.Local.EnableAllowList)),
				"HASURA_GRAPHQL_ENABLE_CONSOLE":    stringz.Ptr("false"),
				"HASURA_GRAPHQL_ENABLE_TELEMETRY":  stringz.Ptr("false"),
				"HASURA_GRAPHQL_LOG_LEVEL":         stringz.Ptr("debug"),
				"HASURA_GRAPHQL_SERVER_PORT":       stringz.Ptr(fmt.Sprintf("%v", p.cfg.Local.ExternalPort)),
				"HASURA_GRAPHQL_JWT_SECRET": stringz.Ptr(jsonz.MustMarshalString(map[string]interface{}{
					"type":     "RS256",
					"key":      string(rsaz.RSAPublicKeyToPEM(p.cfg.JWT.PublicKey)),
					"issuer":   p.cfg.JWT.Issuer,
					"audience": p.cfg.JWT.Audience,
				})),
			}

			if p.cfg.UnauthorizedRole != nil && *p.cfg.UnauthorizedRole != "" {
				e["HASURA_GRAPHQL_UNAUTHORIZED_ROLE"] = p.cfg.UnauthorizedRole
			}

			for k, v := range p.cfg.Environment {
				e[k] = stringz.Ptr(v)
			}

			return e
		}(),
		Image:    "hasura/graphql-engine:v" + hasuraVersion,
		Networks: p.cfg.Stage.AsLocalStage().GetServiceNetworkConfig(),
		Ports: []dctypes.ServicePortConfig{
			{
				Target:    uint32(p.cfg.Local.ExternalPort),
				Published: uint32(p.cfg.Local.ExternalPort),
			},
		},
		Restart: "unless-stopped",
	})

	tpl.Services = append(tpl.Services, dctypes.ServiceConfig{
		Name: consoleContainerName,
		Build: dctypes.BuildConfig{
			Context: buildDirPath,
		},
		ContainerName: consoleContainerName,
		DependsOn: []string{
			containerName,
		},
		Image:    consoleContainerName,
		Networks: p.cfg.Stage.AsLocalStage().GetServiceNetworkConfig(),
		Ports: []dctypes.ServicePortConfig{
			{
				Target:    uint32(p.cfg.Local.ConsoleExternalPort),
				Published: uint32(p.cfg.Local.ConsoleExternalPort),
			},
			{
				Target:    uint32(p.cfg.Local.ConsoleAPIExternalPort),
				Published: uint32(p.cfg.Local.ConsoleAPIExternalPort),
			},
		},
		Restart: "unless-stopped",
		Volumes: []dctypes.ServiceVolumeConfig{
			{
				Type:   "bind",
				Source: filez.MustAbs(p.cfg.Stage.GetConfig().App.GetConfigDirPath(p, hasuraConfigDirParts...)),
				Target: "/hasura",
			},
		},
	})
}

// GetCloudTemplate implements the Plugin interface.
func (p *hasuraImpl) GetCloudTemplate(_ string) *gocf.Template {
	tpl := gocf.NewTemplate()

	tpl.Resources[HasuraRefLogGroup.Ref()] = &gologs.LogGroup{
		LogGroupName:    stringz.Ptr(HasuraRefLogGroup.Name(p)),
		RetentionInDays: intz.Ptr(90),
	}
	CloudAddExpRef(tpl, p, HasuraRefLogGroup)
	CloudAddExpGetAtt(tpl, p, HasuraRefLogGroup, HasuraAttARN)

	tpl.Resources[HasuraRefRoleExecution.Ref()] = &goiam.Role{
		AssumeRolePolicyDocument: NewAssumeRolePolicyDocument("ecs-tasks.amazonaws.com"),
		ManagedPolicyArns: &[]string{
			"arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy",
		},
		RoleName: stringz.Ptr(HasuraRefRoleExecution.Name(p)),
		Tags:     CloudGetDefaultTags(HasuraRefRoleExecution.Name(p)),
	}
	CloudAddExpRef(tpl, p, HasuraRefRoleExecution)
	CloudAddExpGetAtt(tpl, p, HasuraRefRoleExecution, HasuraAttARN)
	CloudAddExpGetAtt(tpl, p, HasuraRefRoleExecution, HasuraAttRoleID)

	tpl.Resources[HasuraRefRoleTask.Ref()] = &goiam.Role{
		AssumeRolePolicyDocument: NewAssumeRolePolicyDocument("ecs-tasks.amazonaws.com"),
		RoleName:                 stringz.Ptr(HasuraRefRoleTask.Name(p)),
		Tags:                     CloudGetDefaultTags(HasuraRefRoleTask.Name(p)),
	}
	CloudAddExpRef(tpl, p, HasuraRefRoleTask)
	CloudAddExpGetAtt(tpl, p, HasuraRefRoleTask, HasuraAttARN)
	CloudAddExpGetAtt(tpl, p, HasuraRefRoleTask, HasuraAttRoleID)

	tpl.Resources[HasuraRefTaskDefinition.Ref()] = &goecs.TaskDefinition{
		ContainerDefinitions: &[]goecs.TaskDefinition_ContainerDefinition{
			{
				Environment: CloudGetTaskDefinitionKeyValuePairs(
					func() map[string]string {
						e := map[string]string{
							"HASURA_GRAPHQL_ADMIN_SECRET":              p.cfg.Cloud.AdminSecret,
							"HASURA_GRAPHQL_DATABASE_URL":              p.deps.Postgres.GetCloudMetadata().URL.String(),
							"HASURA_GRAPHQL_DEV_MODE":                  "false",
							"HASURA_GRAPHQL_ENABLED_APIS":              "graphql",
							"HASURA_GRAPHQL_ENABLED_LOG_TYPES":         "startup,http-log,webhook-log,websocket-log,query-log",
							"HASURA_GRAPHQL_ENABLE_ALLOWLIST":          fmt.Sprintf("%v", p.cfg.Cloud.EnableAllowList),
							"HASURA_GRAPHQL_ENABLE_CONSOLE":            "false",
							"HASURA_GRAPHQL_ENABLE_MAINTENANCE_MODE":   "true",
							"HASURA_GRAPHQL_ENABLE_TELEMETRY":          "false",
							"HASURA_GRAPHQL_GRACEFUL_SHUTDOWN_TIMEOUT": "29",
							"HASURA_GRAPHQL_SERVER_PORT":               fmt.Sprintf("%v", hasuraCloudPort),
							"HASURA_GRAPHQL_JWT_SECRET": jsonz.MustMarshalString(map[string]interface{}{
								"type":     "RS256",
								"key":      string(rsaz.RSAPublicKeyToPEM(p.cfg.JWT.PublicKey)),
								"issuer":   p.cfg.JWT.Issuer,
								"audience": p.cfg.JWT.Audience,
							}),
							"HASURA_GRAPHQL_LOG_LEVEL": func() string {
								if p.cfg.Stage.GetMode().IsProduction() {
									return "warning"
								}
								return "debug"
							}(),
						}

						if p.cfg.UnauthorizedRole != nil && *p.cfg.UnauthorizedRole != "" {
							e["HASURA_GRAPHQL_UNAUTHORIZED_ROLE"] = *p.cfg.UnauthorizedRole
						}

						if p.cfg.Cloud.CORSDomain != nil && *p.cfg.Cloud.CORSDomain != "" {
							e["HASURA_GRAPHQL_CORS_DOMAIN"] = *p.cfg.Cloud.CORSDomain
						}

						for k, v := range p.cfg.Environment {
							e[k] = v
						}

						return e
					}()),
				Image: stringz.Ptr(fmt.Sprintf("%v:%v",
					p.deps.ImageRepository.GetCloudMetadata().ImageName,
					p.cfg.Stage.AsCloudStage().GetCloudConfig().Version)),
				LogConfiguration: &goecs.TaskDefinition_LogConfiguration{
					LogDriver: "awslogs",
					Options: &map[string]string{
						"awslogs-region":        gocf.Ref("AWS::Region"),
						"awslogs-group":         gocf.Ref(HasuraRefLogGroup.Ref()),
						"awslogs-stream-prefix": HasuraRefTaskDefinition.Name(p),
					},
				},
				MountPoints: &[]goecs.TaskDefinition_MountPoint{
					{
						ContainerPath: stringz.Ptr("/tmp"),
						SourceVolume:  stringz.Ptr("tmp"),
					},
					{
						ContainerPath: stringz.Ptr("/root/.hasura"),
						SourceVolume:  stringz.Ptr("hasura"),
					},
				},
				Name: stringz.Ptr(HasuraRefTaskDefinition.Name(p)),
				PortMappings: &[]goecs.TaskDefinition_PortMapping{
					{
						ContainerPort: intz.Ptr(hasuraCloudPort),
						HostPort:      intz.Ptr(hasuraCloudPort),
						Protocol:      stringz.Ptr("tcp"),
					},
				},
				ReadonlyRootFilesystem: boolz.Ptr(true),
				StopTimeout:            intz.Ptr(30),
			},
		},
		Cpu:              stringz.Ptr(fmt.Sprintf("%v", p.cfg.Cloud.CPU)),
		ExecutionRoleArn: stringz.Ptr(gocf.Ref(HasuraRefRoleExecution.Ref())),
		Family:           stringz.Ptr(HasuraRefTaskDefinition.Name(p)),
		Memory:           stringz.Ptr(fmt.Sprintf("%v", p.cfg.Cloud.CPU)),
		NetworkMode:      stringz.Ptr("awsvpc"),
		RequiresCompatibilities: &[]string{
			"FARGATE",
		},
		TaskRoleArn: stringz.Ptr(gocf.Ref(HasuraRefRoleTask.Ref())),
		Volumes: &[]goecs.TaskDefinition_Volume{
			{
				Name: stringz.Ptr("tmp"),
			},
			{
				Name: stringz.Ptr("hasura"),
			},
		},
		Tags: CloudGetDefaultTags(HasuraRefTaskDefinition.Name(p)),
	}
	CloudAddExpRef(tpl, p, HasuraRefTaskDefinition)

	tpl.Resources[HasuraRefTargetGroup.Ref()] = &elbv2.TargetGroup{
		HealthCheckPath:            stringz.Ptr("/healthz"),
		HealthCheckIntervalSeconds: intz.Ptr(15),
		HealthyThresholdCount:      intz.Ptr(2),
		UnhealthyThresholdCount:    intz.Ptr(8),
		Port:                       intz.Ptr(hasuraCloudPort),
		Protocol:                   stringz.Ptr("HTTP"),
		ProtocolVersion:            stringz.Ptr("HTTP1"), // TODO(ibrt): Try HTTP2?
		TargetGroupAttributes: &[]elbv2.TargetGroup_TargetGroupAttribute{
			{
				Key:   stringz.Ptr("deregistration_delay.timeout_seconds"),
				Value: stringz.Ptr("30"),
			},
		},
		TargetType: stringz.Ptr("ip"),
		VpcId:      stringz.Ptr(p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefVPC)),
		Tags:       CloudGetDefaultTags(HasuraRefTargetGroup.Name(p)),
	}
	CloudAddExpRef(tpl, p, HasuraRefTargetGroup)
	CloudAddExpGetAtt(tpl, p, HasuraRefTargetGroup, HasuraAttLoadBalancerARNs)
	CloudAddExpGetAtt(tpl, p, HasuraRefTargetGroup, HasuraAttTargetGroupFullName)
	CloudAddExpGetAtt(tpl, p, HasuraRefTargetGroup, HasuraAttTargetGroupName)

	tpl.Resources[HasuraRefListenerRule.Ref()] = &elbv2.ListenerRule{
		Actions: []elbv2.ListenerRule_Action{
			{
				TargetGroupArn: stringz.Ptr(gocf.Ref(HasuraRefTargetGroup)),
				Type:           "forward",
			},
		},
		Conditions: []elbv2.ListenerRule_RuleCondition{
			{
				Field: stringz.Ptr("host-header"),
				HostHeaderConfig: &elbv2.ListenerRule_HostHeaderConfig{
					Values: &[]string{
						p.cfg.Cloud.DomainName,
					},
				},
			},
		},
		ListenerArn: p.deps.LoadBalancer.GetCloudMetadata().Exports.GetAtt(LoadBalancerRefListenerHTTPS, LoadBalancerAttListenerArn),
		Priority:    100,
	}
	CloudAddExpRef(tpl, p, HasuraRefListenerRule)
	CloudAddExpGetAtt(tpl, p, HasuraRefListenerRule, HasuraAttIsDefault)
	CloudAddExpGetAtt(tpl, p, HasuraRefListenerRule, HasuraAttRuleARN)

	tpl.Resources[HasuraRefCluster.Ref()] = &goecs.Cluster{
		ClusterName: stringz.Ptr(HasuraRefCluster.Name(p)),
		ClusterSettings: &[]goecs.Cluster_ClusterSettings{
			{
				Name: stringz.Ptr("containerInsights"),
				Value: func() *string {
					if p.cfg.Stage.GetMode().IsProduction() {
						return stringz.Ptr("enabled")
					}
					return stringz.Ptr("disabled")
				}(),
			},
		},
		Tags: CloudGetDefaultTags(HasuraRefCluster.Name(p)),
	}
	CloudAddExpRef(tpl, p, HasuraRefCluster)
	CloudAddExpGetAtt(tpl, p, HasuraRefCluster, HasuraAttARN)

	tpl.Resources[HasuraRefService.Ref()] = &goecs.Service{
		AWSCloudFormationDependsOn: []string{
			HasuraRefTargetGroup.Ref(),
		},
		Cluster: stringz.Ptr(gocf.Ref(HasuraRefCluster.Ref())),
		DeploymentController: &goecs.Service_DeploymentController{
			Type: stringz.Ptr("ECS"),
		},
		DeploymentConfiguration: &goecs.Service_DeploymentConfiguration{
			DeploymentCircuitBreaker: &goecs.Service_DeploymentCircuitBreaker{
				Enable:   true,
				Rollback: true,
			},
		},
		DesiredCount: func() *int {
			if p.cfg.Stage.GetMode().IsProduction() {
				return intz.Ptr(p.cfg.Cloud.Replicas)
			}
			return intz.Ptr(1)
		}(),
		EnableECSManagedTags: boolz.Ptr(true),
		LaunchType:           stringz.Ptr("FARGATE"),
		LoadBalancers: &[]goecs.Service_LoadBalancer{
			{
				ContainerName:  stringz.Ptr(HasuraRefTaskDefinition.Name(p)),
				ContainerPort:  intz.Ptr(hasuraCloudPort),
				TargetGroupArn: stringz.Ptr(gocf.Ref(HasuraRefTargetGroup.Ref())),
			},
		},
		NetworkConfiguration: &goecs.Service_NetworkConfiguration{
			AwsvpcConfiguration: &goecs.Service_AwsVpcConfiguration{
				AssignPublicIp: stringz.Ptr("DISABLED"),
				SecurityGroups: &[]string{
					p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefSecurityGroup),
				},
				Subnets: &[]string{
					p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefSubnetPrivateA),
					p.deps.Network.GetCloudMetadata().Exports.GetRef(NetworkRefSubnetPrivateB),
				},
			},
		},
		PropagateTags:      stringz.Ptr("TASK_DEFINITION"),
		SchedulingStrategy: stringz.Ptr("REPLICA"),
		TaskDefinition:     stringz.Ptr(gocf.Ref(HasuraRefTaskDefinition.Ref())),
		Tags:               CloudGetDefaultTags(HasuraRefService.Name(p)),
	}
	CloudAddExpRef(tpl, p, HasuraRefService)
	CloudAddExpGetAtt(tpl, p, HasuraRefService, HasuraAttName)

	tpl.Resources[HasuraRefRecordSet.Ref()] = &goroute53.RecordSet{
		AliasTarget: &goroute53.RecordSet_AliasTarget{
			DNSName:      p.deps.LoadBalancer.GetCloudMetadata().Exports.GetAtt(LoadBalancerRefLoadBalancer, LoadBalancerAttDNSName),
			HostedZoneId: p.deps.LoadBalancer.GetCloudMetadata().Exports.GetAtt(LoadBalancerRefLoadBalancer, LoadBalancerAttCanonicalHostedZoneID),
		},
		HostedZoneId: stringz.Ptr(p.deps.Certificate.GetConfig().Cloud.HostedZoneID),
		Name:         p.cfg.Cloud.DomainName,
		Type:         "A",
	}
	CloudAddExpRef(tpl, p, HasuraRefRecordSet)

	return tpl
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *hasuraImpl) UpdateCloudMetadata(stack *awscft.Stack) {
	p.cloudMetadata = &HasuraCloudMetadata{
		Exports: NewCloudExports(stack),
		URL:     urlz.MustParse(fmt.Sprintf("https://%v/v1/graphql", p.cfg.Cloud.DomainName)),
	}
}

// EventHook implements the Plugin interface.
func (p *hasuraImpl) EventHook(event Event, buildDirPath string) {
	switch event {
	case LocalBeforeCreateEvent:
		p.localBeforeCreateEventHook(buildDirPath)
	case LocalAfterCreateEvent:
		p.localAfterCreateEventHook()
	case CloudBeforeDeployEvent:
		p.cloudBeforeDeployEventHook(buildDirPath)
	}

	if p.cfg.EventHook != nil {
		p.cfg.EventHook(p, event, buildDirPath)
	}
}

func (p *hasuraImpl) localBeforeCreateEventHook(buildDirPath string) {
	filez.MustPrepareDir(buildDirPath, 0777)
	cfgDirPath := p.cfg.Stage.GetConfig().App.GetConfigDirPath(p, hasuraConfigDirParts...)

	if !filez.MustCheckExists(cfgDirPath) {
		filez.MustCopyEmbedFSSimple(
			assets.HasuraConsoleDefaultConfigAssetFS,
			assets.HasuraConsoleDefaultConfigPathPrefix,
			cfgDirPath)
	}

	filez.MustWriteFile(
		filepath.Join(buildDirPath, "docker-entrypoint.sh"), 0777, 0666,
		templatez.MustParseAndExecuteText(
			assets.HasuraConsoleDockerEntrypointSHTemplateAsset,
			assets.HasuraConsoleDockerEntrypointSHTemplateData{
				Host:           LocalGetContainerName(p),
				Port:           p.cfg.Local.ExternalPort,
				ConsolePort:    p.cfg.Local.ConsoleExternalPort,
				ConsoleAPIPort: p.cfg.Local.ConsoleAPIExternalPort,
			}))

	filez.MustWriteFile(
		filepath.Join(buildDirPath, "Dockerfile"), 0777, 0666,
		templatez.MustParseAndExecuteText(
			assets.HasuraConsoleDockerfileTemplateAsset,
			assets.HasuraConsoleDockerfileTemplateData{
				Version:     hasuraVersion,
				Port:        p.cfg.Local.ExternalPort,
				AdminSecret: hasuraLocalAdminSecret,
			}))
}

func (p *hasuraImpl) localAfterCreateEventHook() {
	// TODO(ibrt): Use a waiter instead.
	time.Sleep(30 * time.Second)
	p.ApplyLocalMetadata()
}

func (p *hasuraImpl) cloudBeforeDeployEventHook(buildDirPath string) {
	filez.MustPrepareDir(buildDirPath, 0777)

	imageWithTag := p.deps.ImageRepository.GetCloudMetadata().ImageName + ":" + p.cfg.Stage.AsCloudStage().GetCloudConfig().Version
	cfgDirPath := p.cfg.Stage.GetConfig().App.GetConfigDirPath(p, hasuraConfigDirParts...)

	filez.MustWriteFile(
		filepath.Join(buildDirPath, "Dockerfile"), 0777, 0666,
		templatez.MustParseAndExecuteText(
			assets.HasuraDockerfileTemplateAsset,
			assets.HasuraDockerfileTemplateData{
				Version: hasuraVersion,
			}))

	shellz.NewCommand("cp", "-R", filepath.Join(cfgDirPath, "metadata"), filepath.Join(buildDirPath, "hasura-metadata")).MustRun()
	shellz.NewCommand("cp", "-R", filepath.Join(cfgDirPath, "migrations"), filepath.Join(buildDirPath, "hasura-migrations")).MustRun()
	shellz.NewCommand("docker", "build", "--no-cache", "-t", imageWithTag, ".").SetDir(buildDirPath).MustRun()

	p.cfg.Stage.GetConfig().App.GetOperations().DockerLogin()
	shellz.NewCommand("docker", "push", imageWithTag).MustRun()
}

func (p *hasuraImpl) runCmd(params ...interface{}) {
	shellz.NewCommand("docker").
		AddParams("exec").
		AddParams("-t").
		AddParams(p.GetLocalMetadata().ConsoleContainerName).
		AddParams("hasura-cli", "--skip-update-check").
		AddParams(params...).
		MustRun()
}
