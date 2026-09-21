// Package arch holds no code. It exists for the test in this file, which
// enforces the layering the rest of the service is built on.
//
// The reason this is a test and not a paragraph in a document is that
// documents get skimmed and then contradicted, usually by someone acting in
// good faith who could not have known. An import that breaks the layering is
// invisible in review, compiles perfectly, and passes every functional test.
// This is the only thing that notices.
package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The module this service lives in, and the shared geo module beside it.
const (
	modulePath = "github.com/marco/parkxchange/services/api"
	geoModule  = "github.com/marco/parkxchange/libs/go/geo"
)

// Short names for the packages the rules refer to.
const (
	pkgDomain       = "internal/domain"
	pkgAccounts     = "internal/accounts"
	pkgSpots        = "internal/spots"
	pkgVehicles     = "internal/vehicles"
	pkgOffers       = "internal/offers"
	pkgReservations = "internal/reservations"
	pkgAuth         = "internal/auth"
	pkgWeb          = "internal/web"
	pkgPostgres     = "internal/postgres"
	pkgAPI          = "internal/api"
	pkgRealtime     = "internal/realtime"
	pkgContract     = "internal/contract"
	pkgConfig       = "internal/config"
	pkgLogging      = "internal/logging"
	pkgSeed         = "internal/seed"
	pkgTestDB       = "internal/testdb"
	pkgArch         = "internal/arch"
	pkgSQL          = "migrations"
	pkgMigrate      = "internal/migrate"
	pkgMailer       = "internal/mailer"
	pkgGoogleAuth   = "internal/googleauth"
	pkgPush         = "internal/push"
)

// rule is what one package is permitted to depend on.
//
// Anything not listed is forbidden. An allowlist rather than a denylist,
// deliberately: a denylist silently permits every dependency nobody thought to
// ban, which is exactly how layering erodes.
type rule struct {
	// why explains the package's role, and is printed when a violation is
	// found so the failure teaches rather than just blocks.
	why string

	// packages are the repository packages this one may import, as the short
	// names above. An entry equal to geoModule allows the shared geo module.
	packages []string

	// thirdParty are the module path prefixes this package may import from
	// outside the standard library.
	thirdParty []string

	// wiringRoot exempts a package from the rules entirely. Exactly one place
	// has to know both the interfaces and the implementations, and that is the
	// point of a composition root.
	wiringRoot bool
}

// rules is the architecture, written down in the only form that is checked.
//
// The shape to notice: dependencies point inwards. domain depends on nothing,
// the services depend only on domain, and the adapters depend on the services.
// No arrow runs the other way, which is what keeps the business rules testable
// without a database and reusable by the WebSocket hub and the expiry sweeper.
var rules = map[string]rule{
	pkgDomain: {
		why: "the domain holds entities and rules, and must stay free of " +
			"infrastructure so the same rules can be driven by HTTP, by the " +
			"WebSocket hub or by a background job",
		packages: []string{geoModule},
	},

	pkgAccounts: {
		why: "a use case package declares the ports it needs and depends only " +
			"on the domain, so it can be tested without a database",
		packages: []string{pkgDomain},
	},

	pkgSpots: {
		why: "a use case package declares the ports it needs and depends only " +
			"on the domain, so it can be tested without a database",
		packages: []string{pkgDomain, geoModule},
	},

	pkgReservations: {
		why: "a use case package declares the ports it needs and depends only " +
			"on the domain, so it can be tested without a database",
		packages: []string{pkgDomain},
	},

	pkgOffers: {
		why: "a use case package declares the ports it needs and depends only " +
			"on the domain, so it can be tested without a database",
		packages: []string{pkgDomain},
	},

	pkgVehicles: {
		why: "a use case package declares the ports it needs and depends only " +
			"on the domain, so it can be tested without a database",
		packages: []string{pkgDomain},
	},

	pkgAuth: {
		why: "auth is a cryptography adapter: it implements the hashing and " +
			"token ports and must not reach for HTTP or the database",
		packages:   []string{pkgDomain},
		thirdParty: []string{"golang.org/x/crypto", "github.com/golang-jwt/jwt"},
	},

	pkgMailer: {
		why: "mailer is an email delivery adapter for account use cases",
		packages: []string{pkgDomain, pkgAccounts},
	},

	pkgPush: {
		why: "push is an Expo delivery adapter for reservation notifications",
		packages:   []string{pkgDomain, pkgReservations},
		thirdParty: []string{},
	},

	pkgGoogleAuth: {
		why: "googleauth verifies Google ID tokens for the accounts port",
		packages:   []string{pkgAccounts},
		thirdParty: []string{"google.golang.org/api"},
	},

	pkgWeb: {
		why: "web is HTTP plumbing shared by handlers, and translates domain " +
			"errors into status codes; it must not know about persistence",
		packages:   []string{pkgDomain},
		thirdParty: []string{"golang.org/x/time/rate"},
	},

	pkgPostgres: {
		why: "postgres is the database adapter: it implements the ports the " +
			"use cases declare, which is why it may import them, and it is " +
			"the only package allowed to speak SQL",
		packages:   []string{pkgDomain, pkgAccounts, pkgSpots, pkgVehicles, pkgOffers, pkgReservations, geoModule},
		thirdParty: []string{"github.com/jackc/pgx"},
	},

	pkgAPI: {
		why: "api is the HTTP adapter: it decodes requests, calls use cases " +
			"and serialises results, and must reach the database only " +
			"through a port",
		packages:   []string{pkgDomain, pkgAccounts, pkgSpots, pkgVehicles, pkgOffers, pkgReservations, pkgWeb, pkgConfig, pkgRealtime, geoModule},
		thirdParty: []string{"github.com/coder/websocket"},
	},

	pkgRealtime: {
		why: "realtime is the fan-out adapter: it matches events to viewports " +
			"and must not talk to the database or to HTTP",
		packages: []string{pkgDomain, geoModule},
	},

	pkgContract: {
		why: "contract holds types generated from the OpenAPI document; it " +
			"must not import adapters, use cases or HTTP",
		thirdParty: []string{"github.com/oapi-codegen/runtime"},
	},

	pkgConfig: {
		why: "config reads the environment and depends on nothing",
	},

	pkgLogging: {
		why:      "logging configures slog from the configuration",
		packages: []string{pkgConfig},
	},

	pkgSeed: {
		why:        "the seeder is development tooling that writes fixtures directly",
		packages:   []string{pkgAuth},
		thirdParty: []string{"github.com/jackc/pgx"},
	},

	pkgTestDB: {
		why:        "testdb is test infrastructure that hands out real transactions",
		thirdParty: []string{"github.com/jackc/pgx"},
	},

	pkgSQL: {
		why: "the migrations package only embeds SQL files",
	},

	pkgMigrate: {
		why: "migrate applies the embedded SQL via goose; it is an adapter " +
			"used by cmd/api and cmd/migrate, not by use cases",
		packages:   []string{pkgSQL},
		thirdParty: []string{"github.com/pressly/goose"},
	},

	pkgArch: {
		why: "this package holds the architecture test itself",
	},

	// The composition root. main is the single place that is allowed to know
	// that the accounts service is backed by Postgres, because somebody has to
	// connect the two and doing it anywhere else would put an implementation
	// behind an interface that was meant to hide it.
	"cmd/api":     {why: "the composition root wires interfaces to implementations", wiringRoot: true},
	"cmd/migrate": {why: "a command-line entry point", wiringRoot: true},
}

// forbidden names the mistakes worth explaining rather than merely rejecting.
//
// Any unlisted import is refused anyway; these produce a better message,
// because "you may not import pgx here" is far less useful than knowing what
// to do instead.
var forbidden = []struct {
	importer string
	imported string
	because  string
}{
	{
		importer: pkgDomain,
		imported: "github.com/jackc/pgx",
		because: "the domain must not know how it is stored. Put the SQL in " +
			"internal/postgres and describe what you need as a method on the " +
			"port in the use case package",
	},
	{
		importer: pkgDomain,
		imported: "net/http",
		because: "the domain must not know how it is called. Return a " +
			"domain.Error with the right Kind and let internal/web map it to " +
			"a status code",
	},
	{
		importer: pkgAccounts,
		imported: pkgPostgres,
		because: "a use case must not depend on an adapter. Add the method to " +
			"the Store interface in accounts/ports.go instead; the Postgres " +
			"adapter already implements that interface and the assertion in " +
			"postgres/ports.go will tell you if it stops",
	},
	{
		importer: pkgSpots,
		imported: pkgPostgres,
		because: "a use case must not depend on an adapter. Add the method to " +
			"the Store interface in spots/ports.go instead",
	},
	{
		importer: pkgReservations,
		imported: pkgPostgres,
		because: "a use case must not depend on an adapter. Add the method to " +
			"the Store interface in reservations/ports.go instead",
	},
	{
		importer: pkgOffers,
		imported: pkgPostgres,
		because: "a use case must not depend on an adapter. Add the method to " +
			"the Store interface in offers/ports.go instead",
	},
	{
		importer: pkgVehicles,
		imported: pkgPostgres,
		because: "a use case must not depend on an adapter. Add the method to " +
			"the Store interface in vehicles/ports.go instead",
	},
	{
		importer: pkgReservations,
		imported: "net/http",
		because: "a use case must work without a request. The expiry sweeper " +
			"and the WebSocket hub call these same rules with no HTTP in sight",
	},
	{
		importer: pkgOffers,
		imported: "net/http",
		because: "a use case must work without a request. If you need " +
			"something from the request, pass it as an argument",
	},
	{
		importer: pkgAccounts,
		imported: "net/http",
		because: "a use case must work without a request. If you need " +
			"something from the request, pass it as an argument",
	},
	{
		importer: pkgSpots,
		imported: "net/http",
		because: "a use case must work without a request. The expiry sweeper " +
			"and the WebSocket hub call these same rules with no HTTP in sight",
	},
	{
		importer: pkgVehicles,
		imported: "net/http",
		because: "a use case must work without a request. If you need " +
			"something from the request, pass it as an argument",
	},
	{
		importer: pkgAPI,
		imported: "github.com/jackc/pgx",
		because: "a handler must not talk to the database directly. Call a use " +
			"case, which reaches the database through its port",
	},
	{
		importer: pkgRealtime,
		imported: pkgPostgres,
		because: "the hub must not talk to the database. Listen in " +
			"internal/postgres and Publish into the hub from cmd/api",
	},
	{
		importer: pkgRealtime,
		imported: "net/http",
		because: "the hub is driven by events, not by requests. The HTTP " +
			"upgrade lives in internal/api",
	},
	{
		importer: pkgWeb,
		imported: "github.com/jackc/pgx",
		because:  "HTTP plumbing must not talk to the database",
	},
}

func TestDependenciesPointInwards(t *testing.T) {
	t.Parallel()

	// The test runs in its own directory, so the module root is two levels up.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}

	// Sanity check, so a future directory move fails with a clear message
	// rather than by silently finding no packages and passing.
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("expected the module root at %s, but found no go.mod: %v", root, err)
	}

	imports, err := collectImports(root)
	if err != nil {
		t.Fatalf("collect imports: %v", err)
	}

	if len(imports) == 0 {
		t.Fatal("found no packages to check, which means this test is not testing anything")
	}

	for _, pkg := range sortedKeys(imports) {
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()

			pkgRule, declared := rules[pkg]
			if !declared {
				// A new package has to state its layer. Defaulting to
				// "anything goes" would let the next feature quietly sit
				// outside the architecture.
				t.Fatalf("package %q has no rule in arch_test.go.\n\n"+
					"Every package declares what it may depend on. Decide which "+
					"layer this belongs to and add it to the rules map:\n"+
					"  - a use case package may import only the domain\n"+
					"  - an adapter may import the domain and the ports it implements\n"+
					"  - the domain may import nothing but libs/go/geo", pkg)
			}

			if pkgRule.wiringRoot {
				return
			}

			for _, imported := range imports[pkg] {
				checkImport(t, pkg, imported, pkgRule)
			}
		})
	}
}

func checkImport(t *testing.T, pkg, imported string, pkgRule rule) {
	t.Helper()

	// The standard library is always available. Restricting it would be
	// bureaucracy, except for net/http in the inner layers, which the
	// forbidden list below catches by name.
	short, isRepo := shorten(imported)

	for _, rejected := range forbidden {
		if rejected.importer != pkg {
			continue
		}

		target := imported
		if isRepo {
			target = short
		}
		if target == rejected.imported || strings.HasPrefix(target, rejected.imported+"/") {
			t.Errorf("%s must not import %s.\n\n%s.\n\nThe rule for %s: %s.",
				pkg, imported, rejected.because, pkg, pkgRule.why)
			return
		}
	}

	switch {
	case isRepo:
		if !allowed(short, pkgRule.packages) && !allowed(imported, pkgRule.packages) {
			t.Errorf("%s must not import %s.\n\nThe rule for %s: %s.\n"+
				"It may import: %s.",
				pkg, imported, pkg, pkgRule.why, describe(pkgRule.packages))
		}

	case isStdlib(imported):
		// Allowed, except where the forbidden list said otherwise.

	default:
		if !allowed(imported, pkgRule.thirdParty) {
			t.Errorf("%s must not import the third-party package %s.\n\n"+
				"The rule for %s: %s.\nIt may import: %s.",
				pkg, imported, pkg, pkgRule.why, describe(pkgRule.thirdParty))
		}
	}
}

// collectImports maps each package, named by its path relative to the module
// root, to the packages it imports.
//
// Test files are skipped. A test legitimately wires a real adapter behind a
// real use case, which is exactly what the integration tests do, and forbidding
// that would mean testing the architecture instead of the product.
func collectImports(root string) (map[string][]string, error) {
	imports := make(map[string][]string)
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if name := entry.Name(); name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}

		dir, relErr := filepath.Rel(root, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}
		pkg := filepath.ToSlash(dir)

		for _, spec := range parsed.Imports {
			unquoted, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			imports[pkg] = append(imports[pkg], unquoted)
		}

		// Registered even with no imports, so a package cannot escape having a
		// declared rule by importing nothing today.
		if _, seen := imports[pkg]; !seen {
			imports[pkg] = nil
		}

		return nil
	})

	return imports, err
}

// shorten converts a repository import path into the short name the rules use.
func shorten(importPath string) (string, bool) {
	if trimmed, found := strings.CutPrefix(importPath, modulePath+"/"); found {
		return trimmed, true
	}
	if importPath == geoModule || strings.HasPrefix(importPath, geoModule+"/") {
		return importPath, true
	}
	if strings.HasPrefix(importPath, "github.com/marco/parkxchange/") {
		return importPath, true
	}
	return importPath, false
}

func allowed(candidate string, list []string) bool {
	for _, entry := range list {
		if candidate == entry || strings.HasPrefix(candidate, entry+"/") {
			return true
		}
	}
	return false
}

// isStdlib reports whether an import path belongs to the standard library.
//
// The test is whether the first path segment contains a dot: every module path
// starts with a domain name, and no standard library package does.
func isStdlib(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}

func describe(list []string) string {
	if len(list) == 0 {
		return "nothing but the standard library"
	}
	return strings.Join(list, ", ")
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
