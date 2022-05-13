package opz

import (
	"embed"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscf "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	awsecr "github.com/aws/aws-sdk-go-v2/service/ecr"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/ibrt/golang-shell/shellz"
)

var (
	_ Operations = &operationsImpl{}
)

// Operations implements various ops-related tasks.
type Operations interface {
	GenerateCommitVersion() string
	GenerateTimestampAndCommitVersion() string
	GetGoToolCommand(goTool GoTool) *shellz.Command
	GetNodeToolCommand(nodeTool *NodeTool) *shellz.Command
	GoTest(rootDirPath string, packages []string, filter string, force, cover bool)
	GoCrossBuildForLinuxAMD64(workDirPath, packageName, binFilePath string, injectValues map[string]string)
	PackageLambdaFunctionHandler(handlerFilePath, functionHandlerFileName, packageFilePath string)

	UploadFile(bucketName, key, contentType string, body []byte)
	Decrypt(keyAlias string, ciphertext []byte) []byte
	Encrypt(keyAlias string, plaintext []byte) []byte
	CreateStack(name string, templateBody string, tagsMap map[string]string) *awscft.Stack
	DescribeStack(name string) *awscft.Stack
	UpdateStack(name string, templateBody string, tagsMap map[string]string) *awscft.Stack
	UpsertStack(name string, templateBody string, tagsMap map[string]string) *awscft.Stack
	DockerLoginToECR()

	GenerateHasuraGraphQLSchema(hsURL, adminSecret, role, outFilePath string)
	GenerateHasuraGraphQLEnumsGoBinding(schemaFilePath, outDirPath string)
	GenerateHasuraGraphQLEnumsJSONLabels(schemaFilePath, outFilePath string)
	GenerateHasuraGraphQLTypescriptBinding(schemaFilePath, queriesGlobPath, outFilePath string)

	GeneratePostgresSQLBoilerORM(pgURL string, outDirPath string, options ...SQLBoilerORMOption)
	GenerateSQLiteSQLBoilerORM(dbSpec string, outDirPath string, options ...SQLBoilerORMOption)
	ApplyPostgresHasuraMigrations(pgURL string, embedFS embed.FS, embedMigrationsDirPath string)
	RevertPostgresHasuraMigrations(pgURL string, embedFS embed.FS, embedMigrationsDirPath string)
}

type operationsImpl struct {
	buildDirPath string
	awsCF        *awscf.Client
	awsECR       *awsecr.Client
	awsKMS       *awskms.Client
	awsS3        *awss3.Client
}

// NewOperations initializes a new Operations.
func NewOperations(buildDirPath string, awsCfg *aws.Config) Operations {
	return &operationsImpl{
		buildDirPath: buildDirPath,
		awsCF:        awscf.NewFromConfig(*awsCfg),
		awsECR:       awsecr.NewFromConfig(*awsCfg),
		awsKMS:       awskms.NewFromConfig(*awsCfg),
		awsS3:        awss3.NewFromConfig(*awsCfg),
	}
}
