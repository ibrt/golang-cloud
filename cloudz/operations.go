package cloudz

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscf "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	awsecr "github.com/aws/aws-sdk-go-v2/service/ecr"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/ibrt/golang-bites/filez"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-inject-pg/pgz/testpgz"
	"github.com/ibrt/golang-shell/shellz"
	"github.com/volatiletech/sqlboiler/v4/boilingcore"
	"github.com/volatiletech/sqlboiler/v4/drivers"
	"github.com/volatiletech/sqlboiler/v4/importers"

	_ "github.com/volatiletech/sqlboiler/v4/drivers/sqlboiler-psql/driver" // SQLBoiler postgres driver
)

const (
	toolGoCov       = "github.com/axw/gocov/gocov@v1.0.0"
	toolGoCovHTML   = "github.com/matm/gocov-html@v0.0.0-20200509184451-71874e2e203b"
	toolGoLint      = "golang.org/x/lint/golint@v0.0.0-20210508222113-6edffad5e616"
	toolGoTest      = "github.com/rakyll/gotest@v0.0.6"
	toolMockGen     = "github.com/golang/mock/mockgen@v1.6.0"
	toolStaticCheck = "honnef.co/go/tools/cmd/staticcheck@v0.2.2"
)

// Operations provides a collection of utilities for performing operations.
type Operations interface {
	GenerateTimestampAndCommitVersion() string
	DockerLogin()
	DockerPush(imageAndTag string)
	GoCrossBuildForLinuxAMD64(workDirPath, packageName, binFilePath string, injectValues map[string]string)
	GoPackageFunction(handlerFilePath, functionHandlerFileName, packageFilePath string)
	GoTest(rootDirPath string, packages []string, filter string, force, cover bool, outDirPath string)
	UploadFile(bucketName, key, contentType string, body []byte)
	Decrypt(keyAlias string, ciphertext []byte) []byte
	Encrypt(keyAlias string, plaintext []byte) []byte
	CreateStack(name string, templateBody string, tagsMap map[string]string) *awscft.Stack
	DescribeStack(name string) *awscft.Stack
	UpdateStack(name string, templateBody string, tagsMap map[string]string) *awscft.Stack
	UpsertStack(name string, templateBody string, tagsMap map[string]string) *awscft.Stack
	GeneratePostgresORM(pgURL string, outDirPath string, tableAliases map[string]boilingcore.TableAlias, typeReplaces []boilingcore.TypeReplace)
	NewPostgresORMTypeReplace(table, column, fullType string) boilingcore.TypeReplace
	ApplyHasuraMigrations(pgURL string, embedFS embed.FS, rootDirPath string)
	RevertHasuraMigrations(pgURL string, embedFS embed.FS, rootDirPath string)
}

var (
	_ Operations = &operationsImpl{}
)

type operationsImpl struct {
	awsCF  *awscf.Client
	awsECR *awsecr.Client
	awsKMS *awskms.Client
	awsS3  *awss3.Client
}

// NewOperations initializes a new Operations.
func NewOperations(awsCfg *aws.Config) Operations {
	return &operationsImpl{
		awsCF:  awscf.NewFromConfig(*awsCfg),
		awsECR: awsecr.NewFromConfig(*awsCfg),
		awsKMS: awskms.NewFromConfig(*awsCfg),
		awsS3:  awss3.NewFromConfig(*awsCfg),
	}
}

// GenerateTimestampAndCommitVersion generates a version using the current time and git commit.
func (o *operationsImpl) GenerateTimestampAndCommitVersion() string {
	gitCommit := strings.TrimSpace(shellz.
		NewCommand("git", "rev-parse", "--short", "HEAD").
		SetLogf(nil).
		MustOutput())

	return fmt.Sprintf("%v-%v", time.Now().UTC().Format("20060102T150405"), gitCommit)
}

// DockerLogin runs "docker login" with credentials that allow access to ECR image repositories.
func (o *operationsImpl) DockerLogin() {
	out, err := o.awsECR.GetAuthorizationToken(context.Background(), &awsecr.GetAuthorizationTokenInput{})
	errorz.MaybeMustWrap(err)

	buf, err := base64.StdEncoding.DecodeString(*out.AuthorizationData[0].AuthorizationToken)
	errorz.MaybeMustWrap(err)

	userPass := strings.SplitN(string(buf), ":", 2)
	errorz.Assertf(len(userPass) == 2, "malformed authorization data")

	shellz.NewCommand("docker", "login",
		"--username", userPass[0],
		"--password-stdin",
		strings.TrimPrefix(*out.AuthorizationData[0].ProxyEndpoint, "https://")).
		SetStdin(strings.NewReader(userPass[1])).
		MustRun()
}

// DockerPush runs "docker push".
func (o *operationsImpl) DockerPush(imageAndTag string) {
	shellz.NewCommand("docker", "push", imageAndTag).MustRun()
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

// GoPackageFunction packages a Go function.
func (o *operationsImpl) GoPackageFunction(binFilePath, functionHandlerFileName, packageFilePath string) {
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

// GoTest runs Go tests.
func (o *operationsImpl) GoTest(dirPath string, packages []string, filter string, force, cover bool, outDirPath string) {
	filez.MustPrepareDir(outDirPath, 0777)
	rawCoverageFilePath := filepath.Join(outDirPath, "coverage.out")
	htmlCoverageFilePath := filepath.Join(outDirPath, "coverage.html")

	shellz.NewCommand("go", "mod", "tidy").SetDir(dirPath).MustRun()
	shellz.NewCommand("go", "generate", "./...").SetDir(dirPath).MustRun()
	shellz.NewCommand("go", "build", "-v", "./...").SetDir(dirPath).MustRun()
	shellz.NewCommand("go", "run", toolGoLint, "-set_exit_status", "./...").SetDir(dirPath).MustRun()
	shellz.NewCommand("go", "vet", "./...").SetDir(dirPath).MustRun()
	shellz.NewCommand("go", "run", toolStaticCheck, "./...").SetDir(dirPath).MustRun()

	cmd := shellz.NewCommand(
		"go", "run", toolGoTest,
		"-v", "-p", fmt.Sprintf("%v", runtime.NumCPU()),
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
		coverageJSON := shellz.NewCommand("go", "run", toolGoCov, "convert", rawCoverageFilePath).SetDir(dirPath).MustOutput()
		coverageHTML := shellz.NewCommand("go", "run", toolGoCovHTML).SetStdin(strings.NewReader(coverageJSON)).SetDir(dirPath).MustOutput()
		filez.MustWriteFile(htmlCoverageFilePath, 0777, 0666, []byte(coverageHTML))
		shellz.NewCommand("open", htmlCoverageFilePath).SetDir(dirPath).MustRun()
	}
}

// UploadFile uploads a file to awss3.
func (o *operationsImpl) UploadFile(bucketName, key, contentType string, body []byte) {
	_, err := o.awsS3.PutObject(context.Background(), &awss3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String(contentType),
	})
	errorz.MaybeMustWrap(err)
}

// Decrypt decrypts some data using a KMS key.
func (o *operationsImpl) Decrypt(keyAlias string, ciphertext []byte) []byte {
	resp, err := o.awsKMS.Decrypt(context.Background(), &awskms.DecryptInput{
		KeyId:          aws.String("alias/" + keyAlias),
		CiphertextBlob: ciphertext,
	})
	errorz.MaybeMustWrap(err)
	return resp.Plaintext
}

// Encrypt encrypts some data using a KMS key.
func (o *operationsImpl) Encrypt(keyAlias string, plaintext []byte) []byte {
	resp, err := o.awsKMS.Encrypt(context.Background(), &awskms.EncryptInput{
		KeyId:     aws.String("alias/" + keyAlias),
		Plaintext: plaintext,
	})
	errorz.MaybeMustWrap(err)
	return resp.CiphertextBlob
}

// CreateStack creates a CloudFormation stack.
func (o *operationsImpl) CreateStack(name string, templateBody string, tagsMap map[string]string) *awscft.Stack {
	_, err := o.awsCF.CreateStack(context.Background(), &awscf.CreateStackInput{
		Capabilities: []awscft.Capability{
			awscft.CapabilityCapabilityIam,
			awscft.CapabilityCapabilityNamedIam,
		},
		EnableTerminationProtection: aws.Bool(false),
		OnFailure:                   awscft.OnFailureRollback,
		StackName:                   aws.String(name),
		Tags: func() []awscft.Tag {
			tags := make([]awscft.Tag, 0)
			for k, v := range tagsMap {
				tags = append(tags, awscft.Tag{
					Key:   aws.String(k),
					Value: aws.String(v),
				})
			}
			return tags
		}(),
		TemplateBody:     aws.String(templateBody),
		TimeoutInMinutes: aws.Int32(30),
	})
	errorz.MaybeMustWrap(err)

	errorz.MaybeMustWrap(awscf.NewStackCreateCompleteWaiter(o.awsCF).Wait(
		context.Background(),
		&awscf.DescribeStacksInput{
			StackName: aws.String(name),
		},
		30*time.Minute))

	return o.DescribeStack(name)
}

// DescribeStack describes a CloudFormation stack.
func (o *operationsImpl) DescribeStack(name string) *awscft.Stack {
	out, err := o.awsCF.DescribeStacks(context.Background(), &awscf.DescribeStacksInput{
		StackName: aws.String(name),
	})
	if err != nil {
		var notFound *awscft.StackNotFoundException
		if errors.As(err, &notFound) {
			return nil
		}
		errorz.MaybeMustWrap(err)
	}

	errorz.Assertf(len(out.Stacks) == 1, "unexpected number of stacks")
	return &out.Stacks[0]
}

// UpdateStack updates a CloudFormation stack.
func (o *operationsImpl) UpdateStack(name string, templateBody string, tagsMap map[string]string) *awscft.Stack {
	_, err := o.awsCF.UpdateStack(context.Background(), &awscf.UpdateStackInput{
		Capabilities: []awscft.Capability{
			awscft.CapabilityCapabilityIam,
			awscft.CapabilityCapabilityNamedIam,
		},
		StackName: aws.String(name),
		Tags: func() []awscft.Tag {
			tags := make([]awscft.Tag, 0)
			for k, v := range tagsMap {
				tags = append(tags, awscft.Tag{
					Key:   aws.String(k),
					Value: aws.String(v),
				})
			}
			return tags
		}(),
		TemplateBody: aws.String(templateBody),
	})
	errorz.MaybeMustWrap(err)

	errorz.MaybeMustWrap(awscf.NewStackUpdateCompleteWaiter(o.awsCF).Wait(
		context.Background(),
		&awscf.DescribeStacksInput{
			StackName: aws.String(name),
		},
		30*time.Minute))

	return o.DescribeStack(name)
}

// UpsertStack creates or updates a CloudFormation stack.
func (o *operationsImpl) UpsertStack(name string, templateBody string, tagsMap map[string]string) *awscft.Stack {
	if o.DescribeStack(name) == nil {
		return o.CreateStack(name, templateBody, tagsMap)
	}
	return o.UpdateStack(name, templateBody, tagsMap)
}

// GeneratePostgresORM generates a Postgres ORM.
func (o *operationsImpl) GeneratePostgresORM(pgURL string, outDirPath string, tableAliases map[string]boilingcore.TableAlias, typeReplaces []boilingcore.TypeReplace) {
	filez.MustPrepareDir(outDirPath, 0777)

	parsedPGURL, err := url.Parse(pgURL)
	errorz.MaybeMustWrap(err)
	pass, ok := parsedPGURL.User.Password()
	errorz.Assertf(ok, "no password specified in pgURL")

	state, err := boilingcore.New(&boilingcore.Config{
		Aliases: boilingcore.Aliases{
			Tables: tableAliases,
		},
		DriverName: "psql",
		DriverConfig: map[string]interface{}{
			"dbname":  path.Base(parsedPGURL.Path),
			"host":    parsedPGURL.Hostname(),
			"port":    parsedPGURL.Port(),
			"user":    parsedPGURL.User.Username(),
			"pass":    pass,
			"sslmode": parsedPGURL.Query().Get("sslmode"),
		},
		PkgName:         filepath.Base(outDirPath),
		Imports:         importers.NewDefaultImports(),
		OutFolder:       outDirPath,
		NoHooks:         true,
		NoTests:         true,
		StructTagCasing: "camel",
		TypeReplaces:    typeReplaces,
		Wipe:            false,
	})
	errorz.MaybeMustWrap(err)
	errorz.MaybeMustWrap(state.Run())
	errorz.MaybeMustWrap(state.Cleanup())
}

// NewPostgresORMTypeReplace generates a new TypeReplace for GeneratePostgresORM.
func (o *operationsImpl) NewPostgresORMTypeReplace(table, column, fullType string) boilingcore.TypeReplace {
	typePackage := ""
	typeName := fullType

	if i := strings.LastIndex(fullType, "."); i >= 0 {
		typePackage = fullType[:i]

		if j := strings.LastIndex(fullType, "/"); j >= 0 {
			typeName = fullType[j+1:]
		}
	}

	return boilingcore.TypeReplace{
		Tables: []string{table},
		Match: drivers.Column{
			Name: column,
		},
		Replace: drivers.Column{
			Type: typeName,
		},
		Imports: importers.Set{
			ThirdParty: func() importers.List {
				if typePackage != "" {
					return importers.List{
						fmt.Sprintf(`"%v"`, typePackage),
					}
				}
				return nil
			}(),
		},
	}
}

// ApplyHasuraMigrations applies the Hasura migrations to the given database URL.
// Note that this is a partial implementation for testing purposes:
// - It does not check against nor update the "hdb_catalog.hdb_version" table.
// - It blindly applies all the migrations in a single transaction.
func (o *operationsImpl) ApplyHasuraMigrations(pgURL string, embedFS embed.FS, rootDirPath string) {
	db := testpgz.MustOpen(pgURL)
	defer errorz.IgnoreClose(db)

	dirEntries, err := embedFS.ReadDir(rootDirPath)
	errorz.MaybeMustWrap(err)

	sort.Slice(dirEntries, func(i, j int) bool {
		return dirEntries[i].Name() < dirEntries[j].Name()
	})

	tx, err := db.Begin()
	errorz.MaybeMustWrap(err)
	defer func() {
		_ = tx.Rollback()
	}()

	for _, dirEntry := range dirEntries {
		if !dirEntry.IsDir() {
			continue
		}

		migration, err := embedFS.ReadFile(filepath.Join(rootDirPath, dirEntry.Name(), "up.sql"))
		errorz.MaybeMustWrap(err)

		_, err = tx.Exec(string(migration))
		errorz.MaybeMustWrap(err, errorz.M("migration", dirEntry.Name()))
	}

	errorz.MaybeMustWrap(tx.Commit())
}

// RevertHasuraMigrations reverts the Hasura migrations to the given database URL.
// Note that this is a partial implementation for testing purposes:
// - It does not check against nor update the "hdb_catalog.hdb_version" table.
// - It blindly reverts all the migrations in a single transaction.
func (o *operationsImpl) RevertHasuraMigrations(pgURL string, embedFS embed.FS, rootDirPath string) {
	db := testpgz.MustOpen(pgURL)
	defer errorz.IgnoreClose(db)

	dirEntries, err := embedFS.ReadDir(rootDirPath)
	errorz.MaybeMustWrap(err)

	sort.Slice(dirEntries, func(i, j int) bool {
		return dirEntries[i].Name() >= dirEntries[j].Name() // reverse order
	})

	tx, err := db.Begin()
	errorz.MaybeMustWrap(err)
	defer func() {
		_ = tx.Rollback()
	}()

	for _, dirEntry := range dirEntries {
		if !dirEntry.IsDir() {
			continue
		}

		migration, err := embedFS.ReadFile(filepath.Join(rootDirPath, dirEntry.Name(), "down.sql"))
		errorz.MaybeMustWrap(err)

		_, err = tx.Exec(string(migration))
		errorz.MaybeMustWrap(err, errorz.M("migration", dirEntry.Name()))
	}

	errorz.MaybeMustWrap(tx.Commit())
}
