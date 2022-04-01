package cloudz

import (
	"path/filepath"
	"runtime"
	"strings"

	dctypes "github.com/docker/cli/cli/compose/types"
	"github.com/ibrt/golang-bites/filez"
	"github.com/ibrt/golang-bites/templatez"

	"github.com/ibrt/golang-cloud/cloudz/internal/assets"
)

var (
	_ FunctionBuilder = &goFunctionBuilder{}
)

// FunctionBuilder describes a function builder.
type FunctionBuilder interface {
	GetLocalServiceConfigVolumes(p Function, buildDirPath string) []dctypes.ServiceVolumeConfig
	LocalBeforeCreateEventHook(p Function, buildDirPath string)
	GetCloudRuntime(p Function) string
	BuildCloudPackage(p Function, buildDirPath string)
}

type goFunctionBuilder struct {
	workDirPath  string
	packageName  string
	injectValues map[string]string
}

// NewGoFunctionBuilder initializes a new Go function builder.
func NewGoFunctionBuilder(workDirPath, packageName string, injectValues map[string]string) FunctionBuilder {
	return &goFunctionBuilder{
		workDirPath:  workDirPath,
		packageName:  packageName,
		injectValues: injectValues,
	}
}

// GetLocalServiceConfigVolumes implements the FunctionBuilder interface.
func (b *goFunctionBuilder) GetLocalServiceConfigVolumes(_ Function, _ string) []dctypes.ServiceVolumeConfig {
	return []dctypes.ServiceVolumeConfig{
		{
			Type:   "bind",
			Source: filez.MustAbs(b.workDirPath),
			Target: "/src",
		},
	}
}

// LocalBeforeCreateEventHook implements the FunctionBuilder interface.
func (b *goFunctionBuilder) LocalBeforeCreateEventHook(p Function, buildDirPath string) {
	filez.MustWriteFile(
		filepath.Join(buildDirPath, "Dockerfile"), 0777, 0666,
		templatez.MustParseAndExecuteText(
			assets.GoFunctionDockerfileTemplateAsset,
			assets.GoFunctionDockerfileTemplateData{
				GoVersion:      strings.TrimPrefix(runtime.Version(), "go"),
				FunctionName:   FunctionRefFunction.Name(p),
				TimeoutSeconds: p.GetConfig().TimeoutSeconds,
			}))

	filez.MustWriteFile(
		filepath.Join(buildDirPath, "air.toml"), 0777, 0666,
		templatez.MustParseAndExecuteText(
			assets.GoFunctionAirTOMLTemplateAsset,
			assets.GoFunctionAirTOMLTemplateData{
				PackageName:             b.packageName,
				BuildDirPath:            buildDirPath,
				FunctionHandlerFileName: FunctionHandlerFileName,
			}))
}

// GetCloudRuntime implements the FunctionBuilder interface.
func (b *goFunctionBuilder) GetCloudRuntime(p Function) string {
	return "go1.x"
}

// BuildCloudPackage implements the FunctionBuilder interface.
func (b *goFunctionBuilder) BuildCloudPackage(p Function, buildDirPath string) {
	ops := p.GetStage().GetConfig().App.GetOperations()

	handlerFilePath := filepath.Join(buildDirPath, FunctionHandlerFileName)
	packageFilePath := filepath.Join(buildDirPath, FunctionPackageFileName)

	ops.GoCrossBuildForLinuxAMD64(b.workDirPath, b.packageName, handlerFilePath, b.injectValues)
	ops.PackageLambdaFunctionHandler(handlerFilePath, FunctionHandlerFileName, packageFilePath)
}
