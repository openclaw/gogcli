package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/openclaw/gogcli"

func TestInternalPackagesDoNotDependOnCommandLayer(t *testing.T) {
	root := repositoryRoot(t)

	walkProductionGoFiles(t, filepath.Join(root, "internal"), func(path string, file *ast.File) {
		if strings.HasPrefix(path, filepath.Join(root, "internal", "cmd")+string(filepath.Separator)) {
			return
		}

		for _, imported := range file.Imports {
			if importPath(t, imported) == modulePath+"/internal/cmd" {
				t.Errorf("%s imports the command layer", relativePath(t, root, path))
			}
		}
	})
}

func TestExtractedPurePackagesKeepRuntimeBoundaries(t *testing.T) {
	root := repositoryRoot(t)
	forbidden := []string{
		modulePath + "/internal/app",
		modulePath + "/internal/cmd",
		modulePath + "/internal/config",
		modulePath + "/internal/googleauth",
		modulePath + "/internal/oauthclient",
		modulePath + "/internal/outfmt",
		modulePath + "/internal/secrets",
		modulePath + "/internal/ui",
		"github.com/alecthomas/kong",
		"net/http",
		"os",
		"os/exec",
	}
	packages := []string{"gmailcontent", "mailmime", "slidesmarkdown"}

	for _, name := range packages {
		t.Run(name, func(t *testing.T) {
			walkProductionGoFiles(t, filepath.Join(root, "internal", name), func(path string, file *ast.File) {
				for _, imported := range file.Imports {
					value := importPath(t, imported)
					if slices.Contains(forbidden, value) {
						t.Errorf("%s imports forbidden runtime dependency %q", relativePath(t, root, path), value)
					}
				}
			})
		})
	}
}

func TestGlobalHomeOverridesStayDeleted(t *testing.T) {
	root := repositoryRoot(t)

	walkProductionGoFiles(t, filepath.Join(root, "internal"), func(path string, file *ast.File) {
		for _, decl := range file.Decls {
			switch value := decl.(type) {
			case *ast.FuncDecl:
				if value.Recv == nil && (value.Name.Name == "SetHomeOverride" || value.Name.Name == "GetHomeOverride") {
					t.Errorf("%s contains deleted global home function %q", relativePath(t, root, path), value.Name.Name)
				}
			case *ast.GenDecl:
				if value.Tok != token.VAR {
					continue
				}
				for _, spec := range value.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, name := range valueSpec.Names {
						if name.Name == "homeOverride" {
							t.Errorf("%s contains deleted global home variable %q", relativePath(t, root, path), name.Name)
						}
					}
				}
			}
		}
	})
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
}

func walkProductionGoFiles(t *testing.T, root string, check func(string, *ast.File)) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		check(path, file)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func importPath(t *testing.T, imported *ast.ImportSpec) string {
	t.Helper()

	value, err := strconv.Unquote(imported.Path.Value)
	if err != nil {
		t.Fatalf("parse import %s: %v", imported.Path.Value, err)
	}

	return value
}

func relativePath(t *testing.T, root, path string) string {
	t.Helper()

	value, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("relative path for %s: %v", path, err)
	}

	return value
}
