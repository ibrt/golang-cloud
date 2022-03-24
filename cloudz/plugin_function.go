package cloudz

import (
	"fmt"
	"net/url"
	"path/filepath"

	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gocf "github.com/awslabs/goformation/v6/cloudformation"
	goiam "github.com/awslabs/goformation/v6/cloudformation/iam"
	golambda "github.com/awslabs/goformation/v6/cloudformation/lambda"
	gologs "github.com/awslabs/goformation/v6/cloudformation/logs"
	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/filez"
	"github.com/ibrt/golang-bites/numeric/intz"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-bites/urlz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-validation/vz"
)

// Function constants.
const (
	FunctionPluginDisplayName = "Function"
	FunctionPluginName        = "function"
	FunctionRefRole           = CloudRef("r")
	FunctionRefLogGroup       = CloudRef("lg")
	FunctionRefFunction       = CloudRef("f")
	FunctionAttARN            = CloudAtt("Arn")
	FunctionAttRoleID         = CloudAtt("RoleId")

	FunctionHandlerFileName = "handler"
	FunctionPackageFileName = "function.zip"

	awsRuntimeInterfaceEmulatorPort = 8080
)

var (
	_ Function = &functionImpl{}
	_ Plugin   = &functionImpl{}
)

// FunctionConfigFunc returns the function config for a given Stage.
type FunctionConfigFunc func(Stage, *FunctionDependencies) *FunctionConfig

// FunctionConfig describes the function config.
type FunctionConfig struct {
	Stage          Stage           `validate:"required"`
	Name           string          `validate:"required"`
	Builder        FunctionBuilder `validate:"required"`
	TimeoutSeconds uint16          `validate:"required"`
	Environment    map[string]string
	Local          *FunctionConfigLocal
	Cloud          *FunctionConfigCloud
}

// MustValidate validates the function config.
func (c *FunctionConfig) MustValidate(stageTarget StageTarget) {
	vz.MustValidateStruct(c)
	errorz.Assertf(stageTarget == Local || c.Cloud != nil, "missing FunctionConfig.Cloud")
	errorz.Assertf(stageTarget == Cloud || c.Local != nil, "missing FunctionConfig.Local")
}

// FunctionConfigLocal describes part of the function config.
type FunctionConfigLocal struct {
	ExternalPort uint16
}

// FunctionConfigCloud describes part of the function config.
type FunctionConfigCloud struct {
	Memory       int `validate:"required"`
	RolePolicies []goiam.Role_Policy
}

// FunctionDependencies describes the function dependencies.
type FunctionDependencies struct {
	ArtifactsBucket   Bucket `validate:"required"`
	Network           Network
	OtherDependencies OtherDependencies
}

// MustValidate validates the function dependencies.
func (d *FunctionDependencies) MustValidate() {
	vz.MustValidateStruct(d)
}

// FunctionLocalMetadata describes the function local metadata.
type FunctionLocalMetadata struct {
	ExternalURL *url.URL
	InternalURL *url.URL
}

// FunctionCloudMetadata describes the function cloud metadata.
type FunctionCloudMetadata struct {
	Exports CloudExports
}

// GetARN returns the function ARN.
func (m *FunctionCloudMetadata) GetARN() string {
	return m.Exports.GetAtt(FunctionRefFunction, FunctionAttARN)
}

// GetName returns the function name.
func (m *FunctionCloudMetadata) GetName() string {
	return m.Exports.GetRef(FunctionRefFunction)
}

// Function describes a function.
type Function interface {
	Plugin
	GetConfig() *FunctionConfig
	GetDependencies() *FunctionDependencies
	GetLocalMetadata() *FunctionLocalMetadata
	GetCloudMetadata() *FunctionCloudMetadata
}

type functionImpl struct {
	cfgFunc       FunctionConfigFunc
	deps          *FunctionDependencies
	cfg           *FunctionConfig
	localMetadata *FunctionLocalMetadata
	cloudMetadata *FunctionCloudMetadata
}

// NewFunction initializes a new Function.
func NewFunction(cfgFunc FunctionConfigFunc, deps *FunctionDependencies) Function {
	deps.MustValidate()

	return &functionImpl{
		cfgFunc: cfgFunc,
		deps:    deps,
	}
}

// GetDisplayName implements the Plugin interface.
func (*functionImpl) GetDisplayName() string {
	return FunctionPluginDisplayName
}

// GetName implements the Plugin interface.
func (p *functionImpl) GetName() string {
	return FunctionPluginName
}

// GetInstanceName implements the Plugin interface.
func (p *functionImpl) GetInstanceName() *string {
	return stringz.Ptr(p.cfg.Name)
}

// GetDependenciesMap implements the Plugin interface.
func (p *functionImpl) GetDependenciesMap() map[Plugin]struct{} {
	dependenciesMap := map[Plugin]struct{}{
		p.deps.ArtifactsBucket: {},
	}

	if p.deps.Network != nil {
		dependenciesMap[p.deps.Network] = struct{}{}
	}

	for _, otherDependency := range p.deps.OtherDependencies {
		dependenciesMap[otherDependency] = struct{}{}
	}

	return dependenciesMap
}

// Configure implements the Plugin interface.
func (p *functionImpl) Configure(stage Stage) {
	p.cfg = p.cfgFunc(stage, p.deps)
	p.cfg.MustValidate(stage.GetTarget())
}

// GetStage implements the Plugin interface.
func (p *functionImpl) GetStage() Stage {
	errorz.Assertf(p.cfg != nil, "plugin not configured", errorz.Prefix(FunctionPluginName))
	return p.cfg.Stage
}

// GetConfig implements the Function interface.
func (p *functionImpl) GetConfig() *FunctionConfig {
	return p.cfg
}

// GetDependencies implements the Function interface.
func (p *functionImpl) GetDependencies() *FunctionDependencies {
	return p.deps
}

// GetLocalMetadata implements the Function interface.
func (p *functionImpl) GetLocalMetadata() *FunctionLocalMetadata {
	errorz.Assertf(p.localMetadata != nil, "local not deployed", errorz.Prefix(FunctionPluginName))
	return p.localMetadata
}

// GetCloudMetadata implements the Function interface.
func (p *functionImpl) GetCloudMetadata() *FunctionCloudMetadata {
	errorz.Assertf(p.cloudMetadata != nil, "cloud not deployed", errorz.Prefix(FunctionPluginName))
	return p.cloudMetadata
}

// IsDeployed implements the Plugin interface.
func (p *functionImpl) IsDeployed() bool {
	return p.cloudMetadata != nil
}

// UpdateLocalTemplate implements the Plugin interface.
func (p *functionImpl) UpdateLocalTemplate(tpl *dctypes.Config, buildDirPath string) {
	containerName := LocalGetContainerName(p)

	p.localMetadata = &FunctionLocalMetadata{
		ExternalURL: urlz.MustParse(fmt.Sprintf("http://localhost:%v/2015-03-31/functions/function/invocations", p.cfg.Local.ExternalPort)),
		InternalURL: urlz.MustParse(fmt.Sprintf("http://%v:%v/2015-03-31/functions/function/invocations", containerName, awsRuntimeInterfaceEmulatorPort)),
	}

	tpl.Services = append(tpl.Services, dctypes.ServiceConfig{
		Name: containerName,
		Build: dctypes.BuildConfig{
			Context: buildDirPath,
		},
		ContainerName: containerName,
		Image:         containerName,
		Environment: func() map[string]*string {
			e := make(map[string]*string)
			for k, v := range p.GetConfig().Environment {
				e[k] = stringz.Ptr(v)
			}
			return e
		}(),
		Networks: p.GetConfig().Stage.AsLocalStage().GetServiceNetworkConfig(),
		Ports: []dctypes.ServicePortConfig{
			{
				Target:    uint32(awsRuntimeInterfaceEmulatorPort),
				Published: uint32(p.cfg.Local.ExternalPort),
			},
		},
		Restart: "unless-stopped",
		Volumes: p.cfg.Builder.GetLocalServiceConfigVolumes(p, buildDirPath),
	})
}

// GetCloudTemplate implements the Plugin interface.
func (p *functionImpl) GetCloudTemplate(_ string) *gocf.Template {
	tpl := gocf.NewTemplate()

	tpl.Resources[FunctionRefRole.Ref()] = &goiam.Role{
		AssumeRolePolicyDocument: NewAssumeRolePolicyDocument("lambda.amazonaws.com"),
		ManagedPolicyArns: &[]string{
			"arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole",
		},
		Policies: &p.cfg.Cloud.RolePolicies,
		RoleName: stringz.Ptr(FunctionRefRole.Name(p)),
		Tags:     CloudGetDefaultTags(FunctionRefRole.Name(p)),
	}
	CloudAddExpRef(tpl, p, FunctionRefRole)
	CloudAddExpGetAtt(tpl, p, FunctionRefRole, FunctionAttARN)
	CloudAddExpGetAtt(tpl, p, FunctionRefRole, FunctionAttRoleID)

	tpl.Resources[FunctionRefLogGroup.Ref()] = &gologs.LogGroup{
		LogGroupName:    stringz.Ptr(FunctionRefLogGroup.Name(p)),
		RetentionInDays: intz.Ptr(90),
	}
	CloudAddExpRef(tpl, p, FunctionRefLogGroup)
	CloudAddExpGetAtt(tpl, p, FunctionRefLogGroup, FunctionAttARN)

	tpl.Resources[FunctionRefFunction.Ref()] = &golambda.Function{
		AWSCloudFormationDependsOn: []string{
			FunctionRefRole.Ref(),
			FunctionRefLogGroup.Ref(),
		},
		Code: &golambda.Function_Code{
			S3Bucket: stringz.Ptr(p.deps.ArtifactsBucket.GetCloudMetadata().GetName()),
			S3Key:    stringz.Ptr(p.cfg.Stage.AsCloudStage().GetArtifactsKeyPrefix(p, FunctionPackageFileName)),
		},
		Environment: &golambda.Function_Environment{
			Variables: func() *map[string]string {
				e := map[string]string{}
				for k, v := range p.cfg.Environment {
					e[k] = v
				}
				return &e
			}(),
		},
		FunctionName: stringz.Ptr(FunctionRefFunction.Name(p)),
		Handler:      stringz.Ptr(FunctionHandlerFileName),
		MemorySize:   intz.Ptr(p.cfg.Cloud.Memory),
		Role:         gocf.GetAtt(FunctionRefRole.Ref(), FunctionAttARN.Ref()),
		Runtime:      stringz.Ptr(p.cfg.Builder.GetCloudRuntime(p)),
		Timeout:      intz.Ptr(int(p.cfg.TimeoutSeconds)),
		VpcConfig: func() *golambda.Function_VpcConfig {
			if network := p.deps.Network; network != nil {
				return &golambda.Function_VpcConfig{
					SecurityGroupIds: &[]string{
						network.GetCloudMetadata().Exports.GetRef(NetworkRefSecurityGroup),
					},
					SubnetIds: &[]string{
						network.GetCloudMetadata().Exports.GetRef(NetworkRefSubnetPrivateA),
						network.GetCloudMetadata().Exports.GetRef(NetworkRefSubnetPrivateB),
					},
				}
			}
			return nil
		}(),
	}
	CloudAddExpRef(tpl, p, FunctionRefFunction)
	CloudAddExpGetAtt(tpl, p, FunctionRefFunction, FunctionAttARN)

	return tpl
}

// UpdateCloudMetadata implements the Plugin interface.
func (p *functionImpl) UpdateCloudMetadata(stack *awscft.Stack) {
	p.cloudMetadata = &FunctionCloudMetadata{
		Exports: NewCloudExports(stack),
	}
}

// BeforeDeployHook implements the Plugin interface.
func (p *functionImpl) BeforeDeployHook(buildDirPath string) {
	filez.MustPrepareDir(buildDirPath, 0777)

	if p.cfg.Stage.GetTarget().IsLocal() {
		p.cfg.Builder.LocalBeforeDeployHook(p, buildDirPath)
		return
	}

	p.cfg.Builder.BuildCloudPackage(p, buildDirPath)
	packageContents := filez.MustReadFile(filepath.Join(buildDirPath, FunctionPackageFileName))

	p.cfg.Stage.GetConfig().App.GetOperations().UploadFile(
		p.deps.ArtifactsBucket.GetCloudMetadata().GetName(),
		p.cfg.Stage.AsCloudStage().GetArtifactsKeyPrefix(p, FunctionPackageFileName),
		"application/zip",
		packageContents)
}

// AfterDeployHook implements the Plugin interface.
func (*functionImpl) AfterDeployHook(_ string) {
	// nothing to do here
}
