package secretz

import (
	"encoding/base64"
	"encoding/json"
	"reflect"

	gocf "github.com/awslabs/goformation/v6/cloudformation"
	gokms "github.com/awslabs/goformation/v6/cloudformation/kms"
	"github.com/ibrt/golang-bites/boolz"
	"github.com/ibrt/golang-bites/filez"
	"github.com/ibrt/golang-bites/jsonz"
	"github.com/ibrt/golang-bites/numeric/intz"
	"github.com/ibrt/golang-bites/stringz"
	"github.com/ibrt/golang-edit-prompt/editz"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-validation/vz"

	"github.com/ibrt/golang-cloud/opz"
)

// Secrets describes a set of encrypted secrets.
type Secrets interface {
	Load() interface{}
	EditPrompt()
}

type secretsImpl struct {
	contextName   string
	filePath      string
	ops           opz.Operations
	defaultValues interface{}
	keyAlias      string
	templateName  string
	valuesType    reflect.Type
}

// NewSecrets initializes a new Secrets.
func NewSecrets(contextName, filePath string, ops opz.Operations, defaultValues interface{}) Secrets {
	t := reflect.TypeOf(defaultValues)

	errorz.Assertf(t.Kind() == reflect.Ptr, "defaultValues must be a struct pointer")
	errorz.Assertf(t.Elem().Kind() == reflect.Struct, "defaultValues must be a struct pointer")
	errorz.Assertf(vz.IsValidatable(defaultValues), "defaultValues must be validatable")

	s := &secretsImpl{
		contextName:   contextName,
		filePath:      filePath,
		ops:           ops,
		defaultValues: defaultValues,
		keyAlias:      contextName + "-secrets-key",
		templateName:  contextName + "-secrets",
		valuesType:    t.Elem(),
	}

	s.ensureKeyInitialized()
	return s
}

// Load implements the Secrets interface.
func (s *secretsImpl) Load() interface{} {
	s.ensureFileInitialized()

	enc := filez.MustReadFile(s.filePath)
	buf, err := base64.StdEncoding.DecodeString(string(enc))
	errorz.MaybeMustWrap(err)
	buf = s.ops.Decrypt(s.keyAlias, buf)
	v := reflect.New(s.valuesType).Interface()
	errorz.MaybeMustWrap(json.Unmarshal(buf, v))

	return v
}

// EditPrompt implements the Secrets interface.
func (s *secretsImpl) EditPrompt() {
	s.ensureFileInitialized()

	filez.WithMustWriteTempFile(
		"golang-cloud",
		s.ops.Decrypt(s.keyAlias, filez.MustReadFile(s.filePath)),
		func(tmpFilePath string) {
			buf, isChanged, err := editz.Edit(tmpFilePath, func(buf []byte) error {
				v := reflect.New(s.valuesType).Interface()
				if err := json.Unmarshal(buf, v); err != nil {
					return errorz.Wrap(err)
				}
				return errorz.MaybeWrap(vz.Validate(v))
			})
			errorz.MaybeMustWrap(err)

			if !isChanged {
				return
			}

			v := reflect.New(s.valuesType).Interface()
			errorz.MaybeMustWrap(json.Unmarshal(buf, v))
			s.save(v)
		})
}

func (s *secretsImpl) ensureKeyInitialized() {
	const (
		refKey      = "Key"
		refKeyAlias = "KeyAlias"
	)

	tpl := gocf.NewTemplate()

	tpl.Resources[refKey] = &gokms.Key{
		EnableKeyRotation: boolz.Ptr(false),
		Enabled:           boolz.Ptr(true),
		KeyPolicy: map[string]interface{}{
			"Version": "2012-10-17",
			"Statement": []map[string]interface{}{
				{
					"Effect":   "Allow",
					"Action":   "kms:*",
					"Resource": "*",
					"Principal": map[string]interface{}{
						"AWS": gocf.Sub("arn:aws:iam::${AWS::AccountId}:root"),
					},
				},
			},
		},
		KeySpec:             stringz.Ptr("SYMMETRIC_DEFAULT"),
		KeyUsage:            stringz.Ptr("ENCRYPT_DECRYPT"),
		PendingWindowInDays: intz.Ptr(7),
	}

	tpl.Resources[refKeyAlias] = &gokms.Alias{
		AliasName:   "alias/" + s.keyAlias,
		TargetKeyId: gocf.Ref(refKey),
	}

	buf, err := tpl.JSON()
	errorz.MaybeMustWrap(err)
	s.ops.UpsertStack(s.templateName, string(buf), nil)
}

func (s *secretsImpl) ensureFileInitialized() {
	if !filez.MustCheckExists(s.filePath) {
		s.save(s.defaultValues)
	}
}

func (s *secretsImpl) save(v interface{}) {
	errorz.MaybeMustWrap(vz.Validate(v))
	buf := s.ops.Encrypt(s.keyAlias, jsonz.MustMarshalIndentDefault(v))
	enc := base64.StdEncoding.EncodeToString(buf)
	filez.MustWriteFile(s.filePath, 0777, 0666, []byte(enc))
}
