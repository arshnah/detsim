// Package rewrite source-rewrites a Go package's goroutines, channels, sync locals, and a subset
// of time/rand/os/net calls to run on the rt package's deterministic scheduler, via a go build
// overlay rather than touching files on disk.
package rewrite

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
)

// Result is the outcome of a Rewrite call.
type Result struct {
	OverlayPath    string
	PackageName    string
	PackageDir     string
	RewrittenFiles []string
	Warnings       []Warning

	tmpDir string
}

// Close removes the temp directory holding the overlay and rewritten copies.
func (r *Result) Close() error {
	if r.tmpDir == "" {
		return nil
	}
	return os.RemoveAll(r.tmpDir)
}

// Rewrite loads the package at pattern rooted at dir and rewrites it to run on rt.
func Rewrite(dir, pattern string) (*Result, error) {
	pkg, err := loadPackage(dir, pattern)
	if err != nil {
		return nil, err
	}
	if len(pkg.Syntax) == 0 {
		return nil, fmt.Errorf("rewrite: package %q has no source files", pattern)
	}

	if unsafe := findUnsafeStructEmbeddedSyncFields(pkg); len(unsafe) > 0 {
		msg := fmt.Sprintf("rewrite: package %q is not safe to rewrite:\n", pattern)
		for _, w := range unsafe {
			msg += fmt.Sprintf("  %s: %s\n", w.Pos, w.Message)
		}
		return nil, fmt.Errorf("%s", msg)
	}

	tmpDir, err := os.MkdirTemp("", "detsim-rewrite-")
	if err != nil {
		return nil, fmt.Errorf("rewrite: creating temp dir: %w", err)
	}

	res := &Result{
		PackageName: pkg.Name,
		tmpDir:      tmpDir,
	}
	replace := map[string]string{}

	for i, file := range pkg.Syntax {
		origPath := pkg.CompiledGoFiles[i]
		original, err := os.ReadFile(origPath)
		if err != nil {
			res.Close()
			return nil, fmt.Errorf("rewrite: reading %s: %w", origPath, err)
		}

		ctx := &fileContext{
			pkg:   pkg,
			fset:  pkg.Fset,
			file:  file,
			info:  pkg.TypesInfo,
			warns: &res.Warnings,
		}

		if reason, unsupported := fileHasUnsupportedSelect(ctx); unsupported {
			ctx.warn(file.Pos(), "skipping this whole file: %s", reason)
			continue
		}

		applyAllPasses(ctx)

		rendered, err := renderFile(ctx)
		if err != nil {
			res.Close()
			return nil, err
		}
		if bytes.Equal(original, rendered) {
			continue
		}

		tmpPath, err := writeTempCopy(tmpDir, origPath, rendered)
		if err != nil {
			res.Close()
			return nil, err
		}
		replace[origPath] = tmpPath
		res.RewrittenFiles = append(res.RewrittenFiles, origPath)
	}

	res.PackageDir = filepath.Dir(pkg.CompiledGoFiles[0])

	if len(replace) > 0 {
		genPath := filepath.Join(res.PackageDir, "detsim_generated_sched.go")
		genTmpPath := filepath.Join(tmpDir, "detsim_generated_sched.go")
		if err := os.WriteFile(genTmpPath, []byte(generatedSchedFile(pkg.Name)), 0o644); err != nil {
			res.Close()
			return nil, fmt.Errorf("rewrite: writing generated sched file: %w", err)
		}
		replace[genPath] = genTmpPath
	}

	overlayPath, err := writeOverlay(tmpDir, replace)
	if err != nil {
		res.Close()
		return nil, err
	}
	res.OverlayPath = overlayPath

	return res, nil
}

func applyAllPasses(ctx *fileContext) {
	warnRangeOverChannel(ctx)
	rewriteSelectStatements(ctx)
	rewriteMakeChan(ctx)
	rewriteChanTypes(ctx)
	rewriteChanSend(ctx)
	rewriteChanRecvOKAssign(ctx)
	rewriteChanRecvExpr(ctx)
	rewriteChanClose(ctx)
	rewriteGoStatements(ctx)
	rewriteSyncLocals(ctx)
	rewriteTimeCalls(ctx)
	rewriteRandCalls(ctx)
	rewriteOSFileCalls(ctx)
	rewriteNetCalls(ctx)
	finalizeImports(ctx)
}

// warnRangeOverChannel flags `for range ch` loops: they aren't rewritten, and once ch's
// type becomes *rt.Chan the loop stops compiling, so the user deserves a precise note.
func warnRangeOverChannel(ctx *fileContext) {
	ast.Inspect(ctx.file, func(n ast.Node) bool {
		rng, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}
		if t := ctx.typeOf(rng.X); t != nil {
			if _, isChan := t.Underlying().(*types.Chan); isChan {
				ctx.warn(rng.Pos(), "range over a channel is not rewritten; after rewriting it won't compile against rt.Chan, rewrite the loop as an explicit Recv/RecvOK loop")
			}
		}
		return true
	})
}
