package opz

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gobuffalo/flect"
	"github.com/iancoleman/strcase"
	"github.com/ibrt/golang-bites/enumz"
	"github.com/ibrt/golang-bites/filez"
	"github.com/ibrt/golang-bites/jsonz"
	"github.com/ibrt/golang-bites/templatez"
	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"

	"github.com/ibrt/golang-cloud/opz/internal/assets"
)

// GenerateHasuraGraphQLSchema generates a GraphQL schema file from a running Hasura endpoint.
func (o *operationsImpl) GenerateHasuraGraphQLSchema(hsURL, adminSecret, role, outFilePath string) {
	schema := o.GetNodeToolCommand(GraphQURL).
		AddParams(hsURL).
		AddParams("--introspect").
		AddParams("--format", "graphql").
		AddParams("-H", fmt.Sprintf("X-Hasura-Admin-Secret: %v", adminSecret)).
		AddParams("-H", fmt.Sprintf("X-Hasura-Role: %v", role)).
		MustOutput()

	filez.MustWriteFile(outFilePath, 0777, 0666, []byte(schema))
}

// GenerateHasuraGraphQLEnumsGoBinding generates a Go binding for enums from a Hasura GraphQL schema.
func (o *operationsImpl) GenerateHasuraGraphQLEnumsGoBinding(schemaFilePath, outDirPath string) {
	rawSchema := filez.MustReadFile(schemaFilePath)
	schema := gqlparser.MustLoadSchema(&ast.Source{Input: string(rawSchema)})
	enumSpecs := make([]*enumz.EnumSpec, 0)

	for _, t := range schema.Types {
		if t.Kind == ast.Enum && strings.HasSuffix(t.Name, "_enum") {
			namePlural := strcase.ToCamel(strings.TrimSuffix(t.Name, "_enum"))

			enumSpec := &enumz.EnumSpec{
				EnumNamePlural:     namePlural,
				EnumNameSingular:   flect.Singularize(namePlural),
				EnumNameInComments: strcase.ToDelimited(namePlural, ' '),
				FileName:           t.Name + ".go",
				Values:             make([]*enumz.EnumSpecValue, 0),
			}

			for _, v := range t.EnumValues {
				enumSpec.Values = append(enumSpec.Values, &enumz.EnumSpecValue{
					Name:  strcase.ToCamel(v.Name),
					Value: v.Name,
					Label: v.Description,
				})
			}

			enumSpecs = append(enumSpecs, enumSpec)
		}
	}

	enumz.MustGenerateEnums(outDirPath, true, filepath.Base(outDirPath), enumSpecs)
}

// GenerateHasuraGraphQLEnumsJSONBinding generates a JSON binding for enums from a Hasura GraphQL schema.
func (o *operationsImpl) GenerateHasuraGraphQLEnumsJSONBinding(schemaFilePath, outFilePath string) {
	rawSchema := filez.MustReadFile(schemaFilePath)
	schema := gqlparser.MustLoadSchema(&ast.Source{Input: string(rawSchema)})

	type jsonBinding struct {
		Labels map[string]string `json:"labels"`
	}

	jsonBindings := make(map[string]*jsonBinding)

	for _, t := range schema.Types {
		if t.Kind == ast.Enum && strings.HasSuffix(t.Name, "_enum") {
			jsonBindings[t.Name] = &jsonBinding{
				Labels: make(map[string]string),
			}

			for _, v := range t.EnumValues {
				jsonBindings[t.Name].Labels[v.Name] = v.Description
			}
		}
	}

	filez.MustWriteFile(outFilePath, 0777, 0666, jsonz.MustMarshalIndentDefault(jsonBindings))
}

// GenerateHasuraGraphQLTypescriptBinding generates a TypeScript binding from a Hasura GraphQL schema and a set of queries.
func (o *operationsImpl) GenerateHasuraGraphQLTypescriptBinding(schemaFilePath, queriesGlobPath, outFilePath string) {
	configFilePath := filez.MustAbs(filez.MustWriteFile(
		filepath.Join(o.buildDirPath, "node-tools", "graphql-codegen", "config.yml"), 0777, 0666,
		templatez.MustParseAndExecuteText(
			assets.NodeToolsGraphQLCodeGenYMLTemplateAsset,
			assets.NodeToolsGraphQLCodeGenYMLTemplateData{
				SchemaFilePath:  filez.MustAbs(schemaFilePath),
				QueriesGlobPath: filez.MustAbs(queriesGlobPath),
				OutFilePath:     filez.MustAbs(outFilePath),
			})))

	o.GetNodeToolCommand(GraphQLCodeGen).AddParams("-c", configFilePath).MustRun()
}
