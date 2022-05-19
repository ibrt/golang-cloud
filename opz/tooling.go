package opz

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ibrt/golang-bites/filez"
	"github.com/ibrt/golang-bites/jsonz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-shell/shellz"

	"github.com/ibrt/golang-cloud/opz/internal/assets"
)

// GoTool describes a Go tool.
type GoTool string

// Known Go tools.
const (
	GoCov       GoTool = "github.com/axw/gocov/gocov@v1.0.0"
	GoCovHTML   GoTool = "github.com/matm/gocov-html@v0.0.0-20200509184451-71874e2e203b"
	GoLint      GoTool = "golang.org/x/lint/golint@v0.0.0-20210508222113-6edffad5e616"
	GoTest      GoTool = "github.com/rakyll/gotest@v0.0.6"
	StaticCheck GoTool = "honnef.co/go/tools/cmd/staticcheck@2022.1"
)

// NodeTool describes a Node tool.
type NodeTool struct {
	Packages map[string]string
	Command  string
}

// Known node tools.
var (
	GraphQURL = &NodeTool{
		Packages: map[string]string{
			"graphqurl": "1.0.1",
		},
		Command: "graphqurl",
	}

	GraphQLCodeGen = &NodeTool{
		Packages: map[string]string{
			"@graphql-codegen/cli":                     "2.6.2",
			"@graphql-codegen/typescript":              "2.4.8",
			"@graphql-codegen/typescript-operations":   "2.3.5",
			"@graphql-codegen/typescript-react-apollo": "3.2.11",
			"graphql": "16.5.0",
		},
		Command: "graphql-codegen",
	}
)

// GenerateCommitVersion generates a version using the current  git commit.
func (o *operationsImpl) GenerateCommitVersion() string {
	return strings.TrimSpace(shellz.
		NewCommand("git", "rev-parse", "--short", "HEAD").
		SetLogf(nil).
		MustOutput())
}

// GenerateTimestampAndCommitVersion generates a version using the current time and git commit.
func (o *operationsImpl) GenerateTimestampAndCommitVersion() string {
	return fmt.Sprintf("%v-%v", time.Now().UTC().Format("20060102T150405"), o.GenerateCommitVersion())
}

// GetGoToolCommand returns a *shellz.Command ready to run a command provided as Go package.
func (o *operationsImpl) GetGoToolCommand(goTool GoTool) *shellz.Command {
	return shellz.NewCommand("go", "run", string(goTool))
}

// GetNodeToolCommand returns a *shellz.Command ready to run a command provided as node package.
func (o *operationsImpl) GetNodeToolCommand(nodeTool *NodeTool) *shellz.Command {
	nodeDirPath := filepath.Join(o.buildDirPath, "node-tools")
	packageJSONFilePath := filepath.Join(nodeDirPath, "package.json")
	errorz.MaybeMustWrap(os.MkdirAll(nodeDirPath, 0777))

	if !filez.MustCheckExists(packageJSONFilePath) {
		filez.MustWriteFile(packageJSONFilePath, 0777, 0666, assets.NodeToolsPackageJSONAsset)
	}

	pkgJSON := &struct {
		Name            string            `json:"name"`
		Private         bool              `json:"private"`
		DevDependencies map[string]string `json:"devDependencies"`
	}{}

	errorz.MaybeMustWrap(json.Unmarshal(filez.MustReadFile(packageJSONFilePath), pkgJSON))
	for k, v := range nodeTool.Packages {
		pkgJSON.DevDependencies[k] = v
	}
	filez.MustWriteFile(packageJSONFilePath, 0777, 0666, jsonz.MustMarshalIndentDefault(pkgJSON))

	shellz.NewCommand("yarn", "install").SetDir(nodeDirPath).MustRun()
	return shellz.NewCommand("yarn", "--silent", nodeTool.Command).SetDir(nodeDirPath)
}

// GoTest runs Go tests.
func (o *operationsImpl) GoTest(dirPath string, packages []string, filter string, force, cover bool) {
	outDirPath := filepath.Join(o.buildDirPath, "test", "coverage", "go")
	filez.MustPrepareDir(outDirPath, 0777)
	rawCoverageFilePath := filepath.Join(outDirPath, "coverage.out")
	htmlCoverageFilePath := filepath.Join(outDirPath, "coverage.html")

	shellz.NewCommand("go", "mod", "tidy").SetDir(dirPath).MustRun()
	shellz.NewCommand("go", "generate", "./...").SetDir(dirPath).MustRun()
	shellz.NewCommand("go", "build", "-v", "./...").SetDir(dirPath).MustRun()
	o.GetGoToolCommand(GoLint).AddParams("-set_exit_status", "./...").SetDir(dirPath).MustRun()
	shellz.NewCommand("go", "vet", "./...").SetDir(dirPath).MustRun()
	o.GetGoToolCommand(StaticCheck).AddParams("./...").SetDir(dirPath).MustRun()

	cmd := o.GetGoToolCommand(GoTest).
		AddParams("-v", "-p", fmt.Sprintf("%v", runtime.NumCPU()),
			"-race", "-shuffle=on",
			"-covermode=atomic", fmt.Sprintf("-coverprofile=%v", rawCoverageFilePath))

	if force {
		cmd.AddParams("-count=1")
	}

	if len(packages) == 0 {
		cmd.AddParams("./...")
	} else {
		cmd.AddParamsString(packages...)
	}

	if filter != "" {
		cmd = cmd.AddParams("--run", filter)
	}

	cmd.SetDir(dirPath).MustRun()

	if cover {
		coverageJSON := o.GetGoToolCommand(GoCov).AddParams("convert", rawCoverageFilePath).SetDir(dirPath).MustOutput()
		coverageHTML := o.GetGoToolCommand(GoCovHTML).SetStdin(strings.NewReader(coverageJSON)).SetDir(dirPath).MustOutput()
		filez.MustWriteFile(htmlCoverageFilePath, 0777, 0666, []byte(coverageHTML))
		shellz.NewCommand("open", htmlCoverageFilePath).SetDir(dirPath).MustRun()
	}
}

// GoCrossBuildForLinuxAMD64 builds a Go binary for linux/amd64.
func (o *operationsImpl) GoCrossBuildForLinuxAMD64(workDirPath, packageName, binFilePath string, injectValues map[string]string) {
	ldFlags := []string{
		"-ldflags=-s", "-w", "-extldflags", "-static",
	}

	for k, v := range injectValues {
		ldFlags = append(ldFlags, fmt.Sprintf("-X' %v=%v'", k, v))
	}

	shellz.NewCommand("go", "build", "-v",
		"-trimpath",
		strings.Join(ldFlags, " "),
		"-tags=netgo osusergo",
		"-o", binFilePath, packageName).
		SetEnv("CGO_ENABLED", "0").
		SetEnv("GOOS", "linux").
		SetEnv("GOARCH", "amd64").
		SetDir(workDirPath).
		MustRun()
}

// PackageLambdaFunctionHandler packages a self-contained, executable Lambda function handler.
func (o *operationsImpl) PackageLambdaFunctionHandler(binFilePath, functionHandlerFileName, packageFilePath string) {
	zipBuf := &bytes.Buffer{}
	w := zip.NewWriter(zipBuf)

	fw, err := w.CreateHeader(&zip.FileHeader{
		Name:           functionHandlerFileName,
		CreatorVersion: 3 << 8,     // Unix
		ExternalAttrs:  0555 << 16, // Permissions
		Method:         zip.Deflate,
	})
	errorz.MaybeMustWrap(err)

	_, err = io.Copy(fw, bytes.NewReader(filez.MustReadFile(binFilePath)))
	errorz.MaybeMustWrap(err)
	errorz.MaybeMustWrap(w.Close())
	filez.MustWriteFile(packageFilePath, 0777, 0666, zipBuf.Bytes())
}
