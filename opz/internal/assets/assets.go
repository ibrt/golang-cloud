package assets

import (
	_ "embed" // embed
)

// Embedded assets.
var (
	//go:embed node-tools/package.json.asset
	NodeToolsPackageJSONAsset []byte

	//go:embed node-tools/graphql-codegen.yml.gotpl
	NodeToolsGraphQLCodeGenYMLTemplateAsset string
)

// NodeToolsGraphQLCodeGenYMLTemplateData describes the template data for NodeToolsGraphQLCodeGenYMLTemplateAsset.
type NodeToolsGraphQLCodeGenYMLTemplateData struct {
	SchemaFilePath  string
	QueriesGlobPath string
	OutFilePath     string
}
