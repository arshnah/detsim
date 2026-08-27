package rewrite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
)

type overlayFile struct {
	Replace map[string]string
}

func writeOverlay(tmpDir string, replace map[string]string) (string, error) {
	data, err := json.MarshalIndent(overlayFile{Replace: replace}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("rewrite: marshaling overlay: %w", err)
	}
	path := filepath.Join(tmpDir, "overlay.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("rewrite: writing overlay: %w", err)
	}
	return path, nil
}

func renderFile(ctx *fileContext) ([]byte, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, ctx.fset, ctx.file); err != nil {
		return nil, fmt.Errorf("rewrite: rendering %s: %w", ctx.fset.File(ctx.file.Pos()).Name(), err)
	}
	clean, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("rewrite: gofmt-normalizing %s: %w", ctx.fset.File(ctx.file.Pos()).Name(), err)
	}
	return clean, nil
}

func writeTempCopy(tmpDir, origPath string, content []byte) (string, error) {
	path := filepath.Join(tmpDir, filepath.Base(origPath))
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", fmt.Errorf("rewrite: writing rewritten copy of %s: %w", origPath, err)
	}
	return path, nil
}
