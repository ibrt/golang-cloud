package opz

import (
	"embed"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ibrt/golang-bites/filez"
	"github.com/ibrt/golang-errors/errorz"
	"github.com/ibrt/golang-inject-pg/pgz/testpgz"
	"github.com/volatiletech/sqlboiler/v4/boilingcore"
	"github.com/volatiletech/sqlboiler/v4/drivers"
	"github.com/volatiletech/sqlboiler/v4/importers"

	_ "github.com/volatiletech/sqlboiler/v4/drivers/sqlboiler-psql/driver"    // SQLBoiler Postgres driver
	_ "github.com/volatiletech/sqlboiler/v4/drivers/sqlboiler-sqlite3/driver" // SQLBoiler SQLite driver
)

type sqlBoilerORMOptions struct {
	blacklist    []string
	tableAliases map[string]boilingcore.TableAlias
	typeReplaces []boilingcore.TypeReplace
}

// SQLBoilerORMOption describes an option for the SQLBoiler ORM generator.
type SQLBoilerORMOption func(options *sqlBoilerORMOptions)

// SQLBoilerORMOptionBlacklist is a SQLBoiler ORM generator option.
func SQLBoilerORMOptionBlacklist(blacklist ...string) SQLBoilerORMOption {
	return func(o *sqlBoilerORMOptions) {
		o.blacklist = blacklist
	}
}

// SQLBoilerORMOptionTableAliases is a SQLBoiler ORM generator option.
func SQLBoilerORMOptionTableAliases(tableAliases map[string]boilingcore.TableAlias) SQLBoilerORMOption {
	return func(o *sqlBoilerORMOptions) {
		o.tableAliases = tableAliases
	}
}

// SQLBoilerORMOptionTypeReplaces is a SQLBoiler ORM generator option.
func SQLBoilerORMOptionTypeReplaces(typeReplaces ...boilingcore.TypeReplace) SQLBoilerORMOption {
	return func(o *sqlBoilerORMOptions) {
		o.typeReplaces = typeReplaces
	}
}

// NewSQLBoilerORMTypeReplace generates a new TypeReplace for th SQLBoiler ORM generator.
func NewSQLBoilerORMTypeReplace(table, column string, nullable bool, fullType string) boilingcore.TypeReplace {
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
			Name:     column,
			Nullable: nullable,
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

// GeneratePostgresSQLBoilerORM generates a SQLBoiler ORM for a Postgres database.
func (o *operationsImpl) GeneratePostgresSQLBoilerORM(pgURL string, outDirPath string, options ...SQLBoilerORMOption) {
	filez.MustPrepareDir(outDirPath, 0777)

	parsedPGURL, err := url.Parse(pgURL)
	errorz.MaybeMustWrap(err)
	pass, ok := parsedPGURL.User.Password()
	errorz.Assertf(ok, "no password specified in pgURL")

	resolvedOptions := &sqlBoilerORMOptions{}
	for _, option := range options {
		option(resolvedOptions)
	}

	state, err := boilingcore.New(&boilingcore.Config{
		Aliases: boilingcore.Aliases{
			Tables: resolvedOptions.tableAliases,
		},
		DriverName: "psql",
		DriverConfig: map[string]interface{}{
			"dbname":    path.Base(parsedPGURL.Path),
			"host":      parsedPGURL.Hostname(),
			"port":      parsedPGURL.Port(),
			"user":      parsedPGURL.User.Username(),
			"pass":      pass,
			"sslmode":   parsedPGURL.Query().Get("sslmode"),
			"blacklist": resolvedOptions.blacklist,
		},
		PkgName:         filepath.Base(outDirPath),
		Imports:         importers.NewDefaultImports(),
		OutFolder:       outDirPath,
		NoHooks:         true,
		NoTests:         true,
		StructTagCasing: "camel",
		TypeReplaces:    resolvedOptions.typeReplaces,
		Wipe:            false,
	})
	errorz.MaybeMustWrap(err)
	errorz.MaybeMustWrap(state.Run())
	errorz.MaybeMustWrap(state.Cleanup())
}

// GenerateSQLiteSQLBoilerORM generates a SQLBoiler ORM for a SQLite database.
func (o *operationsImpl) GenerateSQLiteSQLBoilerORM(dbSpec string, outDirPath string, options ...SQLBoilerORMOption) {
	filez.MustPrepareDir(outDirPath, 0777)

	resolvedOptions := &sqlBoilerORMOptions{}
	for _, option := range options {
		option(resolvedOptions)
	}

	state, err := boilingcore.New(&boilingcore.Config{
		Aliases: boilingcore.Aliases{
			Tables: resolvedOptions.tableAliases,
		},
		DriverName: "sqlite3",
		DriverConfig: map[string]interface{}{
			"dbname":    dbSpec,
			"blacklist": resolvedOptions.blacklist,
		},
		PkgName:         filepath.Base(outDirPath),
		Imports:         importers.NewDefaultImports(),
		OutFolder:       outDirPath,
		NoHooks:         true,
		NoTests:         true,
		StructTagCasing: "camel",
		TypeReplaces:    resolvedOptions.typeReplaces,
		Wipe:            false,
	})
	errorz.MaybeMustWrap(err)
	errorz.MaybeMustWrap(state.Run())
	errorz.MaybeMustWrap(state.Cleanup())
}

// ApplyPostgresHasuraMigrations applies the Hasura migrations to the given Postgres database URL.
// Note that this is a partial implementation for testing purposes:
// - It does not check against nor update the "hdb_catalog.hdb_version" table.
// - It blindly applies all the migrations in a single transaction.
func (o *operationsImpl) ApplyPostgresHasuraMigrations(pgURL string, embedFS embed.FS, embedMigrationsDirPath string) {
	db := testpgz.MustOpen(pgURL)
	defer errorz.IgnoreClose(db)

	dirEntries, err := embedFS.ReadDir(embedMigrationsDirPath)
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

		migration, err := embedFS.ReadFile(filepath.Join(embedMigrationsDirPath, dirEntry.Name(), "up.sql"))
		errorz.MaybeMustWrap(err)

		_, err = tx.Exec(string(migration))
		errorz.MaybeMustWrap(err, errorz.M("migration", dirEntry.Name()))
	}

	errorz.MaybeMustWrap(tx.Commit())
}

// RevertPostgresHasuraMigrations reverts the Hasura migrations to the given Postgres database URL.
// Note that this is a partial implementation for testing purposes:
// - It does not check against nor update the "hdb_catalog.hdb_version" table.
// - It blindly reverts all the migrations in a single transaction.
func (o *operationsImpl) RevertPostgresHasuraMigrations(pgURL string, embedFS embed.FS, embedMigrationsDirPath string) {
	db := testpgz.MustOpen(pgURL)
	defer errorz.IgnoreClose(db)

	dirEntries, err := embedFS.ReadDir(embedMigrationsDirPath)
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

		migration, err := embedFS.ReadFile(filepath.Join(embedMigrationsDirPath, dirEntry.Name(), "down.sql"))
		errorz.MaybeMustWrap(err)

		_, err = tx.Exec(string(migration))
		errorz.MaybeMustWrap(err, errorz.M("migration", dirEntry.Name()))
	}

	errorz.MaybeMustWrap(tx.Commit())
}
