package rewrite

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/packages"
)

func findUnsafeStructEmbeddedSyncFields(pkg *packages.Package) []Warning {
	var warns []Warning
	for _, file := range pkg.Syntax {
		if !fileImportsPackage(file, syncPkgPath) {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			field, ok := n.(*ast.Field)
			if !ok {
				return true
			}
			if _, known := syncConstructorFor(pkg.TypesInfo, field.Type); known {
				name := "embedded"
				if len(field.Names) > 0 {
					name = field.Names[0].Name
				}
				warns = append(warns, Warning{
					Pos:     pkg.Fset.Position(field.Pos()),
					Message: unsafeSyncFieldMessage(name),
				})
			}
			return true
		})
	}
	return warns
}

func unsafeSyncFieldMessage(fieldName string) string {
	return fmt.Sprintf(
		"struct field %q embeds a sync.* type directly; detsim/rt only rewrites local var declarations of sync.Mutex/RWMutex/WaitGroup/Once, so this field would stay a real stdlib type that blocks the real OS thread instead of yielding to the scheduler, which can silently hang the whole test binary if it's ever used from rt-scheduled code. Refusing to rewrite this package. Give the type an explicit *rt.Sched-backed constructor by hand instead of relying on the zero value",
		fieldName,
	)
}
