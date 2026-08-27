package rewrite

import (
	"fmt"

	"golang.org/x/tools/go/packages"
)

const loadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedCompiledGoFiles |
	packages.NeedImports |
	packages.NeedTypes |
	packages.NeedTypesInfo |
	packages.NeedSyntax |
	packages.NeedModule

func loadPackage(dir, pattern string) (*packages.Package, error) {
	cfg := &packages.Config{
		Mode: loadMode,
		Dir:  dir,
	}
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, fmt.Errorf("rewrite: loading %q: %w", pattern, err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("rewrite: %q has type errors, refusing to rewrite", pattern)
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("rewrite: pattern %q must resolve to exactly one package, got %d", pattern, len(pkgs))
	}
	return pkgs[0], nil
}
