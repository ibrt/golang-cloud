package opz

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscf "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	awscft "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	awsecr "github.com/aws/aws-sdk-go-v2/service/ecr"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-shell/shellz"
)

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
	errorz.MaybeMustWrap(err, errorz.M("stackName", name))

	errorz.MaybeMustWrap(awscf.NewStackCreateCompleteWaiter(o.awsCF).Wait(
		context.Background(),
		&awscf.DescribeStacksInput{
			StackName: aws.String(name),
		},
		30*time.Minute),
		errorz.M("stackName", name))

	return o.DescribeStack(name)
}

// DescribeStack describes a CloudFormation stack.
func (o *operationsImpl) DescribeStack(name string) *awscft.Stack {
	out, err := o.awsCF.DescribeStacks(context.Background(), &awscf.DescribeStacksInput{
		StackName: aws.String(name),
	})
	if err != nil {
		// TODO(ibrt): Better error handling.
		if strings.Contains(err.Error(), "does not exist") {
			return nil
		}
		errorz.MaybeMustWrap(err, errorz.M("stackName", name))
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
	if err != nil {
		if strings.Contains(err.Error(), "No updates are to be performed") {
			return o.DescribeStack(name)
		}
		errorz.MaybeMustWrap(err, errorz.M("stackName", name))
	}

	errorz.MaybeMustWrap(awscf.NewStackUpdateCompleteWaiter(o.awsCF).Wait(
		context.Background(),
		&awscf.DescribeStacksInput{
			StackName: aws.String(name),
		},
		30*time.Minute),
		errorz.M("stackName", name))

	return o.DescribeStack(name)
}

// UpsertStack creates or updates a CloudFormation stack.
func (o *operationsImpl) UpsertStack(name string, templateBody string, tagsMap map[string]string) *awscft.Stack {
	if o.DescribeStack(name) == nil {
		return o.CreateStack(name, templateBody, tagsMap)
	}
	return o.UpdateStack(name, templateBody, tagsMap)
}

// DockerLoginToECR runs "docker login" with credentials that allow access to ECR image repositories.
func (o *operationsImpl) DockerLoginToECR() {
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
