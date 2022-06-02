package cloudz

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	goiam "github.com/awslabs/goformation/v6/cloudformation/iam"
	gologs "github.com/awslabs/goformation/v6/cloudformation/logs"
	gords "github.com/awslabs/goformation/v6/cloudformation/rds"
	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/boolz"
	"github.com/ibrt/golang-bites/filez"
	"github.com/ibrt/golang-bites/numeric/intz"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-bites/templatez"
	"github.com/ibrt/golang-bites/urlz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-validation/vz"

	"github.com/ibrt/golang-cloud/cloudz/internal/assets"
)

// Postgres constants.
const (
	PostgresPluginDisplayName   = "Postgres"
	PostgresPluginName          = "postgres"
	PostgresRefDBParameterGroup = CloudRef("pg")
	PostgresRefDBSubnetGroup    = CloudRef("sg")
	PostgresRefLogGroup         = CloudRef("lg")
	PostgresRefRoleMonitoring   = CloudRef("r-mon")
	PostgresRefDBInstance       = CloudRef("i")
	PostgresAttARN              = CloudAtt("Arn")
	PostgresAttRoleID           = CloudAtt("RoleId")
	PostgresAttEndpointAddress  = CloudAtt("Endpoint.Address")
	PostgresAttEndpointPort     = CloudAtt("Endpoint.Port")

	postgresVersion      = "12.10"
	postgresPort         = 5432
	postgresAdminVersion = "6.8"
	postgresAdminPort    = 80
)

var (
	_ Postgres = &postgresImpl{}
	_ Plugin   = &postgresImpl{}
)

// PostgresConfigFunc returns the postgres config for a given Stage.
type PostgresConfigFunc func(Stage, *PostgresDependencies) *PostgresConfig

// PostgresEventHookFunc describes a postgres event hook.
type PostgresEventHookFunc func(Postgres, Event, string)

// PostgresConfig describes the postgres config.
type PostgresConfig struct {
	Stage     Stage `validate:"required"`
	Local     *PostgresConfigLocal
	Cloud     *PostgresConfigCloud
	EventHook PostgresEventHookFunc
}

// MustValidate validates the postgres config.
func (c *PostgresConfig) MustValidate(stageTarget StageTarget) {
	vz.MustValidateStruct(c)
	errorz.Assertf(stageTarget == Local || c.Cloud != nil, "missing PostgresConfig.Cloud")
	errorz.Assertf(stageTarget == Cloud || c.Local != nil, "missing PostgresConfig.Local")
}

// PostgresConfigCloud describes part of the postgres config.
type PostgresConfigCloud struct {
	Password            string `validate:"required,min=16"`
	AllocatedStorageGBs int    `validate:"required,min=5"`
	InstanceClass       string `validate:"required"`
}

// PostgresConfigLocal describes part of the postgres config.
type PostgresConfigLocal struct {
	ExternalPort      uint16 `validate:"required"`
	AdminExternalPort uint16 `validate:"required"`
}

// PostgresDependencies describes the postgres dependencies.
type PostgresDependencies struct {
	Network           Network `validate:"required"`
	OtherDependencies OtherDependencies
}

// MustValidate validates the postgres dependencies.
func (d *PostgresDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// PostgresLocalMetadata describes the postgres local metadata.
type PostgresLocalMetadata struct {
	ContainerName           string
	ExternalURL             *url.URL
	InternalURL             *url.URL
	AdminConsoleExternalURL *url.URL
}

// PostgresCloudMetadata describes the postgres cloud metadata.
type PostgresCloudMetadata struct {
	Exports CloudExports
	URL     *url.URL
}

// Postgres describes a postgres.
type Postgres interface {
	Plugin
	GetConfig() *PostgresConfig
	GetDependencies() *PostgresDependencies
	GetLocalMetadata() *PostgresLocalMetadata
	GetCloudMetadata(require bool) *PostgresCloudMetadata
}

type postgresImpl struct {
	cfgFunc       PostgresConfigFunc
	deps          *PostgresDependencies
	cfg           *PostgresConfig
	localMetadata *PostgresLocalMetadata
	cloudMetadata *PostgresCloudMetadata
}

// NewPostgres initializes a new Postgres.
func NewPostgres(cfgFunc PostgresConfigFunc, deps *PostgresDependencies) Postgres {
	deps.MustValidate()

	return &postgresImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*postgresImpl) GetDisplayName() string {
	return PostgresPluginDisplayName
}

// GetName implements the Plugin interface.
func (p *postgresImpl) GetName() string {
	return PostgresPluginName
}

// GetInstanceName implements the Plugin interface.
func (p *postgresImpl) GetInstanceName() *string {
	return nil
}

// GetDependenciesMap implements the Plugin interface.
func (p *postgresImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{
		p.deps.Network: {},
	}

	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}

	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *postgresImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *postgresImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(PostgresPluginName))
	return p.cfg.Stage
}

// GetConfig implements the Postgres interface.
func (p *postgresImpl) GetConfig() *PostgresConfig {
	return p.cfg
}

// GetDependencies implements the Postgres interface.
func (p *postgresImpl) GetDependencies() *PostgresDependencies {
	return p.deps
}

// GetLocalMetadata implements the Postgres interface.
func (p *postgresImpl) GetLocalMetadata() *PostgresLocalMetadata {
	errorz.Assertf(p.localMetadata != nil, "local not deployed", errorz.Prefix(PostgresPluginName))
	return p.localMetadata
}

// GetCloudMetadata implements the Postgres interface.
func (p *postgresImpl) GetCloudMetadata(require bool) *PostgresCloudMetadata {
	errorz.Assertf(!require || p.cloudMetadata != nil, "cloud not deployed", errorz.Prefix(PostgresPluginName))
	return p.cloudMetadata
}

// IsDeployed implements the Plugin interface.
func (p *postgresImpl) IsDeployed() bool {
	return p.cloudMetadata != nil
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *postgresImpl) UpdateLocalTemplate(tpl *dctypes.Config, buildDirPath string) {
	containerName := LocalGetContainerName(p)
	adminContainerName := LocalGetContainerName(p, "admin")

	p.localMetadata = &PostgresLocalMetadata{
		ContainerName:           containerName,
		ExternalURL:             urlz.MustParse(fmt.Sprintf("postgres://postgres:%v@localhost:%v/postgres?sslmode=disable", LocalPassword, p.cfg.Local.ExternalPort)),
		InternalURL:             urlz.MustParse(fmt.Sprintf("postgres://postgres:%v@%v:%v/postgres?sslmode=disable", LocalPassword, containerName, postgresPort)),
		AdminConsoleExternalURL: urlz.MustParse(fmt.Sprintf("http://localhost:%v", p.cfg.Local.AdminExternalPort)),
	}

	tpl.Services = append(tpl.Services, dctypes.ServiceConfig{
		Name: containerName,
		Build: dctypes.BuildConfig{
			Context: buildDirPath,
		},
		ContainerName: containerName,
		Environment: map[string]*string{
			"POSTGRES_PASSWORD": stringz.Ptr(LocalPassword),
		},
		Image:    containerName,
		Networks: p.cfg.Stage.AsLocalStage().GetServiceNetworkConfig(),
		Ports: []dctypes.ServicePortConfig{
			{
				Target:    postgresPort,
				Published: uint32(p.cfg.Local.ExternalPort),
			},
		},
		Restart: "unless-stopped",
	})

	tpl.Services = append(tpl.Services, dctypes.ServiceConfig{
		Name:          adminContainerName,
		ContainerName: adminContainerName,
		DependsOn: []string{
			containerName,
		},
		Environment: map[string]*string{
			"PGADMIN_DEFAULT_EMAIL":                   stringz.Ptr("pgadmin4@pgadmin.org"),
			"PGADMIN_DEFAULT_PASSWORD":                stringz.Ptr(LocalPassword),
			"PGADMIN_CONFIG_SERVER_MODE":              stringz.Ptr("False"),
			"PGADMIN_CONFIG_MASTER_PASSWORD_REQUIRED": stringz.Ptr("False"),
		},
		Image:    "dpage/pgadmin4:" + postgresAdminVersion,
		Networks: p.cfg.Stage.AsLocalStage().GetServiceNetworkConfig(),
		Ports: []dctypes.ServicePortConfig{
			{
				Target:    postgresAdminPort,
				Published: uint32(p.cfg.Local.AdminExternalPort),
			},
		},
		Restart: "unless-stopped",
		Volumes: []dctypes.ServiceVolumeConfig{
			{
				Type:     "bind",
				Source:   filez.MustAbs(filepath.Join(buildDirPath, "servers.json")),
				Target:   "/pgadmin4/servers.json",
				ReadOnly: true,
			},
			{
				Type:     "bind",
				Source:   filez.MustAbs(filepath.Join(buildDirPath, "pgpass")),
				Target:   "/pgadmin4/pgpass",
				ReadOnly: true,
			},
		},
	})
}

// GetCloudTemplate implements the Plugin interface.
func (p *postgresImpl) GetCloudTemplate(_ string) *gocf.Template {
	tpl := gocf.NewTemplate()

	tpl.Resources[PostgresRefDBParameterGroup.Ref()] = &gords.DBParameterGroup{
		Description: PostgresRefDBParameterGroup.Name(p),
		Family:      "postgres" + strings.Split(postgresVersion, ".")[0],
		Parameters: &map[string]string{
			"application_name": PostgresRefDBParameterGroup.Name(p),
		},
		Tags: CloudGetDefaultTags(PostgresRefDBParameterGroup.Name(p)),
	}
	CloudAddExpRef(tpl, p, PostgresRefDBParameterGroup)

	tpl.Resources[PostgresRefDBSubnetGroup.Ref()] = &gords.DBSubnetGroup{
		DBSubnetGroupDescription: PostgresRefDBSubnetGroup.Name(p),
		DBSubnetGroupName:        stringz.Ptr(PostgresRefDBSubnetGroup.Name(p)),
		SubnetIds: []string{
			p.deps.Network.GetCloudMetadata(true).Exports.GetRef(NetworkRefSubnetPublicA),
			p.deps.Network.GetCloudMetadata(true).Exports.GetRef(NetworkRefSubnetPublicB),
		},
		Tags: CloudGetDefaultTags(PostgresRefDBSubnetGroup.Name(p)),
	}
	CloudAddExpRef(tpl, p, PostgresRefDBSubnetGroup)

	tpl.Resources[PostgresRefLogGroup.Ref()] = &gologs.LogGroup{
		LogGroupName:    stringz.Ptr(PostgresRefLogGroup.Name(p)),
		RetentionInDays: intz.Ptr(90),
	}
	CloudAddExpRef(tpl, p, PostgresRefLogGroup)
	CloudAddExpGetAtt(tpl, p, PostgresRefLogGroup, PostgresAttARN)

	tpl.Resources[PostgresRefRoleMonitoring.Ref()] = &goiam.Role{
		AssumeRolePolicyDocument: NewAssumeRolePolicyDocument("monitoring.rds.amazonaws.com"),
		ManagedPolicyArns: &[]string{
			"arn:aws:iam::aws:policy/service-role/AmazonRDSEnhancedMonitoringRole",
		},
		RoleName: stringz.Ptr(PostgresRefRoleMonitoring.Name(p)),
		Tags:     CloudGetDefaultTags(PostgresRefRoleMonitoring.Name(p)),
	}
	CloudAddExpRef(tpl, p, PostgresRefRoleMonitoring)
	CloudAddExpGetAtt(tpl, p, PostgresRefRoleMonitoring, PostgresAttARN)
	CloudAddExpGetAtt(tpl, p, PostgresRefRoleMonitoring, PostgresAttRoleID)

	rdsDBInstance := &gords.DBInstance{
		AWSCloudFormationDependsOn: []string{
			PostgresRefLogGroup.Ref(),
		},
		AllocatedStorage:        stringz.Ptr(fmt.Sprintf("%v", p.cfg.Cloud.AllocatedStorageGBs)),
		AutoMinorVersionUpgrade: boolz.Ptr(false),
		CopyTagsToSnapshot:      boolz.Ptr(true),
		DBInstanceClass:         p.cfg.Cloud.InstanceClass,
		DBInstanceIdentifier:    stringz.Ptr(PostgresRefDBInstance.Name(p)),
		DBName:                  stringz.Ptr(p.cfg.Stage.GetName()),
		DBParameterGroupName:    stringz.Ptr(gocf.Ref(PostgresRefDBParameterGroup.Ref())),
		DBSubnetGroupName:       stringz.Ptr(gocf.Ref(PostgresRefDBSubnetGroup.Ref())),
		EnableCloudwatchLogsExports: &[]string{
			"postgresql",
			"upgrade",
		},
		Engine:                     stringz.Ptr("postgres"),
		EngineVersion:              stringz.Ptr(postgresVersion),
		MasterUserPassword:         stringz.Ptr(p.cfg.Cloud.Password),
		MasterUsername:             stringz.Ptr(p.cfg.Stage.GetName()),
		PreferredBackupWindow:      stringz.Ptr("07:00-08:00"),
		PreferredMaintenanceWindow: stringz.Ptr("wed:10:00-wed:12:00"),
		PubliclyAccessible:         boolz.Ptr(true),
		StorageEncrypted:           boolz.Ptr(true),
		StorageType:                stringz.Ptr("gp2"),
		VPCSecurityGroups: &[]string{
			p.deps.Network.GetCloudMetadata(true).Exports.GetRef(NetworkRefSecurityGroup),
		},
		Tags: CloudGetDefaultTags(PostgresRefDBInstance.Name(p)),
	}

	if p.cfg.Stage.GetMode().IsProduction() {
		rdsDBInstance.BackupRetentionPeriod = intz.Ptr(30)
		rdsDBInstance.EnablePerformanceInsights = boolz.Ptr(true)
		rdsDBInstance.MonitoringInterval = intz.Ptr(60)
		rdsDBInstance.MonitoringRoleArn = stringz.Ptr(gocf.GetAtt(PostgresRefRoleMonitoring.Ref(), "Arn"))
		rdsDBInstance.MultiAZ = boolz.Ptr(true)
	} else {
		rdsDBInstance.AvailabilityZone = stringz.Ptr(p.cfg.Stage.GetConfig().App.GetConfig().AWSConfig.Region + "a")
		rdsDBInstance.BackupRetentionPeriod = intz.Ptr(1)
	}

	tpl.Resources[PostgresRefDBInstance.Ref()] = rdsDBInstance
	CloudAddExpRef(tpl, p, PostgresRefDBInstance)
	CloudAddExpGetAtt(tpl, p, PostgresRefDBInstance, PostgresAttEndpointAddress)
	CloudAddExpGetAtt(tpl, p, PostgresRefDBInstance, PostgresAttEndpointPort)

	return tpl
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *postgresImpl) UpdateCloudMetadata(stack *awscft.Stack) {
	exports := NewCloudExports(stack)

	p.cloudMetadata = &PostgresCloudMetadata{
		Exports: exports,
		URL: urlz.MustParse(fmt.Sprintf("postgres://%v:%v@%v:%v/%v",
			p.cfg.Stage.GetName(),
			p.cfg.Cloud.Password,
			exports.GetAtt(PostgresRefDBInstance, PostgresAttEndpointAddress),
			exports.GetAtt(PostgresRefDBInstance, PostgresAttEndpointPort),
			p.cfg.Stage.GetName())),
	}
}

// EventHook implements the Plugin interface.
func (p *postgresImpl) EventHook(event Event, buildDirPath string) {
	switch event {
	case LocalBeforeCreateEvent:
		p.localBeforeCreateEvent(buildDirPath)
	}

	if p.cfg.EventHook != nil {
		p.cfg.EventHook(p, event, buildDirPath)
	}
}

func (p *postgresImpl) localBeforeCreateEvent(buildDirPath string) {
	filez.MustPrepareDir(buildDirPath, 0777)

	filez.MustWriteFile(
		filepath.Join(buildDirPath, "Dockerfile"), 0777, 0666,
		templatez.MustParseAndExecuteText(
			assets.PostgresDockerfileTemplateAsset,
			assets.PostgresDockerfileTemplateData{
				Version: postgresVersion,
			}))

	filez.MustWriteFile(
		filepath.Join(buildDirPath, "init.sh"), 0777, 0666,
		assets.PostgresInitSHAsset)

	filez.MustWriteFile(
		filepath.Join(buildDirPath, "servers.json"), 0777, 0666,
		templatez.MustParseAndExecuteText(
			assets.PostgresServersJSONTemplateAsset,
			assets.PostgresServersJSONTemplateData{
				Name:     p.cfg.Stage.GetConfig().App.GetConfig().DisplayName,
				Port:     postgresPort,
				Host:     LocalGetContainerName(p),
				Username: "postgres",
				Database: "postgres",
			}))

	filez.MustWriteFile(
		filepath.Join(buildDirPath, "pgpass"), 0777, 0600,
		templatez.MustParseAndExecuteText(
			assets.PostgresPGPassTemplateAsset,
			assets.PostgresPGPassTemplateData{
				Port:     postgresPort,
				Host:     LocalGetContainerName(p),
				Username: "postgres",
				Password: LocalPassword,
				Database: "postgres",
			}))
}
