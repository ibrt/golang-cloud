package assets

import (
	"embed"
)

const (
	// HasuraConsoleDefaultConfigPathPrefix describes the path prefix for HasuraConsoleDefaultConfigAssetFS.
	HasuraConsoleDefaultConfigPathPrefix = "hasura-console/default-config"
)

// Embedded assets.
var (
	//go:embed go-function/air.toml.gotpl
	GoFunctionAirTOMLTemplateAsset string

	//go:embed go-function/Dockerfile.gotpl
	GoFunctionDockerfileTemplateAsset string

	//go:embed hasura/Dockerfile.gotpl
	HasuraDockerfileTemplateAsset string

	//go:embed hasura-console/default-config
	HasuraConsoleDefaultConfigAssetFS embed.FS

	//go:embed hasura-console/docker-entrypoint.sh.gotpl
	HasuraConsoleDockerEntrypointSHTemplateAsset string

	//go:embed hasura-console/Dockerfile.gotpl
	HasuraConsoleDockerfileTemplateAsset string

	//go:embed http-api/Dockerfile.gotpl
	HTTPAPIDockerfileTemplateAsset string

	//go:embed load-balancer/not-found.html.asset
	LoadBalancerNotFoundHTMLAsset string

	//go:embed node-tools/package.json.asset
	NodeToolsPackageJSONAsset []byte

	//go:embed node-tools/graphql-codegen.yml.gotpl
	NodeToolsGraphQLCodeGenYMLTemplateAsset string

	//go:embed postgres/Dockerfile.gotpl
	PostgresDockerfileTemplateAsset string

	//go:embed postgres/init.sh.asset
	PostgresInitSHAsset []byte

	//go:embed postgres/pgpass.gotpl
	PostgresPGPassTemplateAsset string

	//go:embed postgres/servers.json.gotpl
	PostgresServersJSONTemplateAsset string
)

// GoFunctionAirTOMLTemplateData describes the template data for GoFunctionAirTOMLTemplateAsset.
type GoFunctionAirTOMLTemplateData struct {
	PackageName             string
	BuildDirPath            string
	FunctionHandlerFileName string
}

// GoFunctionDockerfileTemplateData describes the template data for HasuraDockerfileTemplateAsset.
type GoFunctionDockerfileTemplateData struct {
	GoVersion      string
	FunctionName   string
	TimeoutSeconds uint16
}

// HasuraDockerfileTemplateData describes the template data for HasuraDockerfileTemplateAsset.
type HasuraDockerfileTemplateData struct {
	Version string
}

// HasuraConsoleDockerEntrypointSHTemplateData describes the template data for HasuraConsoleDockerEntrypointSHTemplateAsset.
type HasuraConsoleDockerEntrypointSHTemplateData struct {
	Host           string
	Port           uint16
	ConsolePort    uint16
	ConsoleAPIPort uint16
}

// HasuraConsoleDockerfileTemplateData describes the template data for HasuraConsoleDockerfileTemplateAsset.
type HasuraConsoleDockerfileTemplateData struct {
	Version     string
	Port        uint16
	AdminSecret string
}

// HTTPAPIDockerfileTemplateData describes the template data for HTTPAPIDockerfileTemplateAsset.
type HTTPAPIDockerfileTemplateData struct {
	GoVersion  string
	ListenAddr string
}

// NodeToolsGraphQLCodeGenYMLTemplateData describes the template data for NodeToolsGraphQLCodeGenYMLTemplateAsset.
type NodeToolsGraphQLCodeGenYMLTemplateData struct {
	SchemaFilePath  string
	QueriesGlobPath string
	OutFilePath     string
}

// PostgresDockerfileTemplateData describes the template data for PostgresDockerfileTemplateAsset.
type PostgresDockerfileTemplateData struct {
	Version string
}

// PostgresPGPassTemplateData describes the template data for PostgresPGPassTemplateAsset.
type PostgresPGPassTemplateData struct {
	Port     uint16
	Host     string
	Username string
	Password string
	Database string
}

// PostgresServersJSONTemplateData describes the template data for PostgresServersJSONTemplateAsset.
type PostgresServersJSONTemplateData struct {
	Name     string
	Port     uint16
	Host     string
	Username string
	Database string
}
