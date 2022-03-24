module github.com/ibrt/golang-cloud

go 1.17

require (
	github.com/aws/aws-sdk-go-v2 v1.16.0
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.20.1
	github.com/aws/aws-sdk-go-v2/service/ecr v1.17.1
	github.com/aws/aws-sdk-go-v2/service/kms v1.16.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.26.1
	github.com/awslabs/goformation/v6 v6.0.3
	github.com/docker/cli v20.10.13+incompatible
	github.com/go-playground/validator/v10 v10.10.1
	github.com/iancoleman/strcase v0.2.0
	github.com/ibrt/golang-bites v1.7.0
	github.com/ibrt/golang-edit-prompt v1.0.1
	github.com/ibrt/golang-errors v1.1.3
	github.com/ibrt/golang-inject-pg v1.2.0
	github.com/ibrt/golang-lambda v0.3.0
	github.com/ibrt/golang-shell v1.0.2
	github.com/ibrt/golang-validation v1.0.2
	github.com/volatiletech/sqlboiler/v4 v4.8.6
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

require (
	github.com/aws/aws-lambda-go v1.28.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.4.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.7 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.9.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.13.1 // indirect
	github.com/aws/smithy-go v1.11.1 // indirect
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/codegangsta/inject v0.0.0-20150114235600-33e0aa1cb7c0 // indirect
	github.com/codeskyblue/go-sh v0.0.0-20200712050446-30169cf553fe // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/friendsofgo/errors v0.9.2 // indirect
	github.com/georgysavva/scany v0.3.0 // indirect
	github.com/getsentry/sentry-go v0.13.0 // indirect
	github.com/go-playground/locales v0.14.0 // indirect
	github.com/go-playground/universal-translator v0.18.0 // indirect
	github.com/gofrs/uuid v4.2.0+incompatible // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/gorilla/schema v1.2.0 // indirect
	github.com/ibrt/golang-fixtures v1.2.4 // indirect
	github.com/ibrt/golang-inject v1.1.0 // indirect
	github.com/ibrt/golang-inject-clock v1.2.2 // indirect
	github.com/ibrt/golang-inject-http v1.3.0 // indirect
	github.com/ibrt/golang-inject-logs v1.3.0 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.11.0 // indirect
	github.com/jackc/pgerrcode v0.0.0-20201024163028-a0d42d470451 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.2.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20200714003250-2b9c44734f2b // indirect
	github.com/jackc/pgtype v1.10.0 // indirect
	github.com/jackc/pgx/v4 v4.15.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kataras/tablewriter v0.0.0-20180708051242-e063d29b7c23 // indirect
	github.com/labstack/echo/v4 v4.7.2 // indirect
	github.com/labstack/gommon v0.3.1 // indirect
	github.com/lensesio/tableprinter v0.0.0-20201125135848-89e81fc956e7 // indirect
	github.com/leodido/go-urn v1.2.1 // indirect
	github.com/lib/pq v1.10.2 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/sanathkr/go-yaml v0.0.0-20170819195128-ed9d249f429b // indirect
	github.com/sanathkr/yaml v0.0.0-20170819201035-0056894fa522 // indirect
	github.com/sanity-io/litter v1.5.4 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/spf13/cast v1.4.1 // indirect
	github.com/stretchr/testify v1.7.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.1 // indirect
	github.com/volatiletech/inflect v0.0.1 // indirect
	github.com/volatiletech/strmangle v0.0.2 // indirect
	github.com/yosssi/gohtml v0.0.0-20201013000340-ee4748c638f4 // indirect
	golang.org/x/crypto v0.0.0-20220321153916-2c7772ba3064 // indirect
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f // indirect
	golang.org/x/sys v0.0.0-20220319134239-a9b59b0215f8 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20220224211638-0e9765cccd65 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)
