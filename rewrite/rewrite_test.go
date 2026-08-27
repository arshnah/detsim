package rewrite

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	return filepath.Dir(filepath.Dir(file))
}

func TestRewritePipelineCompilesAndRunsDeterministically(t *testing.T) {
	root := moduleRoot(t)
	pkgDir := filepath.Join(root, "rewrite", "testdata", "pipeline")

	res, err := Rewrite(pkgDir, ".")
	if err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	defer res.Close()

	if len(res.RewrittenFiles) == 0 {
		t.Fatal("expected at least one rewritten file")
	}
	for _, w := range res.Warnings {
		t.Logf("warning: %s: %s", w.Pos, w.Message)
	}

	binPath := filepath.Join(t.TempDir(), "pipeline_driver")
	build := exec.Command("go", "build",
		"-overlay="+res.OverlayPath,
		"-o", binPath,
		"./rewrite/testdata/pipeline_driver",
	)
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	runOnce := func(seed string) []byte {
		cmd := exec.Command(binPath, "-seed="+seed)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("running driver with seed=%s: %v", seed, err)
		}
		return out
	}

	a := runOnce("42")
	b := runOnce("42")
	if !bytes.Equal(a, b) {
		t.Fatalf("same seed produced different output:\nrun1:\n%s\nrun2:\n%s", a, b)
	}
	if len(a) == 0 {
		t.Fatal("driver produced no output")
	}
}

func buildDriver(t *testing.T, root, overlayPath, driverPkg string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), filepath.Base(driverPkg))
	build := exec.Command("go", "build", "-overlay="+overlayPath, "-o", binPath, driverPkg)
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return binPath
}

func TestRewriteSelectCompilesAndRunsDeterministically(t *testing.T) {
	root := moduleRoot(t)
	pkgDir := filepath.Join(root, "rewrite", "testdata", "selectdemo")

	res, err := Rewrite(pkgDir, ".")
	if err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	defer res.Close()

	if len(res.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", res.Warnings)
	}
	if len(res.RewrittenFiles) == 0 {
		t.Fatal("expected at least one rewritten file")
	}

	binPath := buildDriver(t, root, res.OverlayPath, "./rewrite/testdata/selectdemo_driver")

	runOnce := func(seed string) []byte {
		out, err := exec.Command(binPath, "-seed="+seed).Output()
		if err != nil {
			t.Fatalf("running driver with seed=%s: %v", seed, err)
		}
		return out
	}

	a := runOnce("7")
	b := runOnce("7")
	if !bytes.Equal(a, b) {
		t.Fatalf("same seed produced different output:\nrun1:\n%s\nrun2:\n%s", a, b)
	}
	t.Logf("driver output: %s", a)
}

func TestUnsupportedSelectSkipsWholeFileInstead(t *testing.T) {
	root := moduleRoot(t)
	pkgDir := filepath.Join(root, "rewrite", "testdata", "unsupportedselect")

	res, err := Rewrite(pkgDir, ".")
	if err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	defer res.Close()

	if len(res.RewrittenFiles) != 0 {
		t.Fatalf("expected the whole file to be skipped, got rewritten files: %v", res.RewrittenFiles)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected a warning explaining why the file was skipped")
	}
	t.Logf("warning: %s", res.Warnings[0].Message)

	vet := exec.Command("go", "vet", "-overlay="+res.OverlayPath, "./rewrite/testdata/unsupportedselect")
	vet.Dir = root
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("go vet failed on the untouched file: %v\n%s", err, out)
	}
}

func TestRewriteOSFileCallsCompilesAndFaultInjects(t *testing.T) {
	root := moduleRoot(t)
	pkgDir := filepath.Join(root, "rewrite", "testdata", "filewal")

	res, err := Rewrite(pkgDir, ".")
	if err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	defer res.Close()

	if len(res.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", res.Warnings)
	}

	binPath := buildDriver(t, root, res.OverlayPath, "./rewrite/testdata/filewal_driver")

	wantClean := "roundtrip: hello world\n" +
		"readwritefile: second file\n" +
		"size: 11\n" +
		"moved size: 11\n" +
		"gone: true\n"

	clean, err := exec.Command(binPath, "-seed=1", "-corrupt=0").Output()
	if err != nil {
		t.Fatalf("running driver: %v", err)
	}
	if string(clean) != wantClean {
		t.Fatalf("expected clean round trip through Create/Open/WriteFile/ReadFile/Stat/Rename/Remove/IsNotExist:\ngot:\n%s\nwant:\n%s", clean, wantClean)
	}

	corrupted := false
	for seed := int64(1); seed <= 50; seed++ {
		out, err := exec.Command(binPath, fmt.Sprintf("-seed=%d", seed), "-corrupt=0.9").Output()
		if err != nil {
			t.Fatalf("running driver at seed %d: %v", seed, err)
		}
		if string(out) != wantClean {
			corrupted = true
			break
		}
	}
	if !corrupted {
		t.Fatal("expected byte corruption to show up in at least one of 50 seeds at corrupt=0.9")
	}
}

func TestRewriteNetCallsCompilesAndRunsDeterministically(t *testing.T) {
	root := moduleRoot(t)
	pkgDir := filepath.Join(root, "rewrite", "testdata", "netecho")

	res, err := Rewrite(pkgDir, ".")
	if err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	defer res.Close()

	if len(res.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", res.Warnings)
	}
	if len(res.RewrittenFiles) == 0 {
		t.Fatal("expected at least one rewritten file")
	}

	binPath := buildDriver(t, root, res.OverlayPath, "./rewrite/testdata/netecho_driver")

	runOnce := func(seed string) []byte {
		out, err := exec.Command(binPath, "-seed="+seed).Output()
		if err != nil {
			t.Fatalf("running driver with seed=%s: %v", seed, err)
		}
		return out
	}

	wantOut := "ping\nremote addr: echo-server\n"
	if got := string(runOnce("1")); got != wantOut {
		t.Fatalf("expected clean echo plus a working net.Addr round trip:\ngot:\n%s\nwant:\n%s", got, wantOut)
	}

	a := runOnce("9")
	b := runOnce("9")
	if !bytes.Equal(a, b) {
		t.Fatalf("same seed produced different output:\nrun1:\n%s\nrun2:\n%s", a, b)
	}
}

func TestStructEmbeddedSyncFieldRefusesToRewrite(t *testing.T) {
	root := moduleRoot(t)
	pkgDir := filepath.Join(root, "rewrite", "testdata", "unsafestructfield")

	_, err := Rewrite(pkgDir, ".")
	if err == nil {
		t.Fatal("expected Rewrite() to refuse a package with a struct-embedded sync.Mutex field")
	}
	t.Logf("got expected error: %v", err)
}

func TestSelectCaseChannelExpressionEvaluatedExactlyOnce(t *testing.T) {
	root := moduleRoot(t)
	pkgDir := filepath.Join(root, "rewrite", "testdata", "selectcall")

	res, err := Rewrite(pkgDir, ".")
	if err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	defer res.Close()

	if len(res.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", res.Warnings)
	}

	binPath := buildDriver(t, root, res.OverlayPath, "./rewrite/testdata/selectcall_driver")

	out, err := exec.Command(binPath, "-seed=1").Output()
	if err != nil {
		t.Fatalf("running driver: %v", err)
	}
	// RecvFirst and SendFirst each call Next() exactly once. A rewriter that splices the
	// channel expression into both the ready check and the commit closure would report 4.
	const want = "got=1 sent=true evals=2\n"
	if string(out) != want {
		t.Fatalf("select case channel expressions must be evaluated exactly once:\ngot:  %s\nwant: %s", out, want)
	}
}

func TestAliasedStdlibImportsAreRewritten(t *testing.T) {
	root := moduleRoot(t)
	pkgDir := filepath.Join(root, "rewrite", "testdata", "aliasedstdlib")

	res, err := Rewrite(pkgDir, ".")
	if err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	defer res.Close()

	if len(res.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", res.Warnings)
	}

	binPath := buildDriver(t, root, res.OverlayPath, "./rewrite/testdata/aliasedstdlib_driver")

	out, err := exec.Command(binPath, "-seed=3").Output()
	if err != nil {
		t.Fatalf("running driver: %v\n%s", err, out)
	}
	// If any aliased import survived unrewritten, the driver hangs on a real blocking
	// call instead of printing this.
	const want = "count=5 order=[0 1 2 3 4] shadow intact closedOK=true\n"
	if string(out) != want {
		t.Fatalf("aliased sync/time/rand imports and builtin shadowing mishandled:\ngot:  %s\nwant: %s", out, want)
	}
}

func TestGeneratedOverlayFilesAreValidGo(t *testing.T) {
	root := moduleRoot(t)
	pkgDir := filepath.Join(root, "rewrite", "testdata", "pipeline")

	res, err := Rewrite(pkgDir, ".")
	if err != nil {
		t.Fatalf("Rewrite() = %v", err)
	}
	defer res.Close()

	vet := exec.Command("go", "vet", "-overlay="+res.OverlayPath, "./rewrite/testdata/pipeline")
	vet.Dir = root
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("go vet failed: %v\n%s", err, out)
	}
}
