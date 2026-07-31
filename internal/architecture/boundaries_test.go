// Package architecture holds the module dependency gates of the Parcel modular
// monolith. The rules live in tests so a boundary violation fails the build
// rather than a review.
package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "go.idp.xyz/idp-parcel"

// infrastructureImports must never reach a domain package. A domain type is
// expressed with the standard library and the framework domain event contract
// only.
var infrastructureImports = []string{
	"github.com/jackc/pgx",
	"github.com/go-chi/chi",
	"database/sql",
	"net/http",
	"log/slog",
	"os",
	"go.idp.xyz/idp-bento-go/postgres",
	"go.idp.xyz/idp-bento-go/eventing",
}

// businessModules are the bounded context roots under internal/. One module
// never reaches into the internals of another.
var businessModules = []string{"parcelshipment", "partycommercial", "networkrouting", "settlementaccounting"}

// frameworkTestkit must never enter a production package.
const frameworkTestkit = "go.idp.xyz/idp-bento-go/testkit"

type sourceFile struct {
	pkg     string
	path    string
	imports []string
}

func TestDomainPackagesDoNotDependOnInfrastructure(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		if !isDomainPackage(file.pkg) {
			continue
		}
		for _, imported := range file.imports {
			for _, forbidden := range infrastructureImports {
				if imported == forbidden || strings.HasPrefix(imported, forbidden+"/") {
					t.Errorf("%s: domain package imports infrastructure %q", file.path, imported)
				}
			}
			if strings.Contains(imported, "/adapters/") {
				t.Errorf("%s: domain package imports adapter %q", file.path, imported)
			}
		}
	}
}

func TestBusinessModulesDoNotReachIntoEachOther(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		owner, owned := moduleOf(file.pkg)
		if !owned {
			continue
		}
		for _, imported := range file.imports {
			target, targeted := moduleOf(imported)
			if !targeted || target == owner {
				continue
			}
			t.Errorf("%s: module %q imports internals of module %q via %q",
				file.path, owner, target, imported)
		}
	}
}

func TestProductionPackagesDoNotImportTheFrameworkTestkit(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		for _, imported := range file.imports {
			if strings.HasPrefix(imported, frameworkTestkit) {
				t.Errorf("%s: production package imports %q", file.path, frameworkTestkit)
			}
		}
	}
}

// TestBusinessPackagesDoNotTouchTheDriverDirectly proves that a write always
// goes through the framework transaction rather than a raw pgx transaction.
func TestBusinessPackagesDoNotTouchTheDriverDirectly(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		module, owned := moduleOf(file.pkg)
		if !owned || strings.Contains(file.pkg, "/adapters/postgres") {
			continue
		}
		for _, imported := range file.imports {
			if strings.HasPrefix(imported, "github.com/jackc/pgx") {
				t.Errorf("%s: module %q reaches the driver outside its persistence adapter via %q",
					file.path, module, imported)
			}
		}
	}
}

func TestApplicationPackagesDoNotDependOnAdapters(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		if !strings.HasSuffix(file.pkg, "/application") {
			continue
		}
		for _, imported := range file.imports {
			if strings.Contains(imported, "/adapters/") {
				t.Errorf("%s: application package imports adapter %q", file.path, imported)
			}
			if strings.HasPrefix(imported, "net/http") || strings.HasPrefix(imported, "github.com/go-chi") {
				t.Errorf("%s: application package imports transport %q", file.path, imported)
			}
		}
	}
}

func TestHTTPAdaptersDoNotExecuteSQL(t *testing.T) {
	t.Parallel()

	for _, file := range loadSources(t) {
		if !strings.Contains(file.pkg, "/adapters/http") {
			continue
		}
		for _, imported := range file.imports {
			if strings.HasPrefix(imported, "github.com/jackc/pgx") ||
				imported == "database/sql" ||
				strings.Contains(imported, "/adapters/postgres") {
				t.Errorf("%s: inbound HTTP adapter reaches persistence via %q", file.path, imported)
			}
		}
	}
}

func isDomainPackage(pkg string) bool {
	return strings.HasSuffix(pkg, "/domain") || strings.Contains(pkg, "/domain/")
}

func moduleOf(pkg string) (string, bool) {
	prefix := modulePath + "/internal/"
	if !strings.HasPrefix(pkg, prefix) {
		return "", false
	}
	remainder := strings.TrimPrefix(pkg, prefix)
	name, _, _ := strings.Cut(remainder, "/")
	for _, module := range businessModules {
		if name == module {
			return name, true
		}
	}
	return "", false
}

// loadSources parses the non-test Go files of the repository. Test files are
// excluded so a test-only dependency cannot fail a production boundary rule.
func loadSources(t *testing.T) []sourceFile {
	t.Helper()

	root := repositoryRoot(t)
	var files []sourceFile

	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); name == ".git" || name == "docs" || name == "backup" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}

		file := sourceFile{
			pkg:  filepath.ToSlash(filepath.Join(modulePath, relative)),
			path: filepath.ToSlash(relative) + "/" + filepath.Base(path),
		}
		for _, spec := range parsed.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			file.imports = append(file.imports, imported)
		}
		files = append(files, file)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk repository: %v", walkErr)
	}
	if len(files) == 0 {
		t.Fatal("no Go sources found; the boundary gates would pass vacuously")
	}
	return files
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("go.mod not found above the test working directory")
		}
		directory = parent
	}
}
