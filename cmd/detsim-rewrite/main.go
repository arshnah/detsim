package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/arshnah/detsim/rewrite"
)

func main() {
	os.Exit(run())
}

func run() int {
	dir := flag.String("dir", ".", "directory to load the package from")
	pkg := flag.String("pkg", ".", "package pattern to rewrite")
	printOverlay := flag.Bool("print-overlay", false, "print the overlay JSON path and exit without building")
	vetAfter := flag.Bool("vet", true, "run go vet against the rewritten overlay to confirm it compiles")
	flag.Parse()

	res, err := rewrite.Rewrite(*dir, *pkg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer res.Close()

	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s: %s\n", w.Pos, w.Message)
	}

	if len(res.RewrittenFiles) == 0 {
		fmt.Println("no goroutine/channel/sync/time/rand constructs found, nothing to rewrite")
		return 0
	}

	fmt.Println("rewrote:")
	for _, f := range res.RewrittenFiles {
		fmt.Println(" ", f)
	}
	fmt.Println("overlay:", res.OverlayPath)

	if *printOverlay {
		return 0
	}

	if *vetAfter {
		vet := exec.Command("go", "vet", "-overlay="+res.OverlayPath, *pkg)
		vet.Dir = *dir
		vet.Stdout = os.Stdout
		vet.Stderr = os.Stderr
		if err := vet.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "rewritten package fails go vet:", err)
			return 1
		}
		fmt.Println("go vet: ok")
	}
	return 0
}
