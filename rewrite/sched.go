package rewrite

import "fmt"

func generatedSchedFile(pkgName string) string {
	return fmt.Sprintf(`package %s

import "github.com/arshnah/detsim/rt"

var %s *rt.Sched
var %s *rt.FileSystem
var %s *rt.Network

func DetsimSetSched(s *rt.Sched) {
	%s = s
}

func DetsimSetFileSystem(fs *rt.FileSystem) {
	%s = fs
}

func DetsimSetNetwork(n *rt.Network) {
	%s = n
}
`, pkgName, schedVarName, fsVarName, netVarName, schedVarName, fsVarName, netVarName)
}
