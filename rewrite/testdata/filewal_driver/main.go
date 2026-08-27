package main

import (
	"flag"
	"fmt"

	"github.com/arshnah/detsim"
	"github.com/arshnah/detsim/rewrite/testdata/filewal"
	"github.com/arshnah/detsim/rt"
)

func main() {
	seed := flag.Int64("seed", 1, "scheduler seed")
	corrupt := flag.Float64("corrupt", 0.0, "byte corruption rate")
	flag.Parse()

	sched := rt.NewSched(*seed)
	filewal.DetsimSetSched(sched)
	filewal.DetsimSetFileSystem(rt.NewFileSystem(sched, detsim.FaultProfile{
		CorruptByteRate: *corrupt,
	}))

	got, err := filewal.WriteAndReadBack("data.wal", []byte("hello world"))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("roundtrip: %s\n", got)

	rt2, err := filewal.WriteReadFileRoundTrip("data2.wal", []byte("second file"))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("readwritefile: %s\n", rt2)

	size, err := filewal.SizeOf("data2.wal")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("size: %d\n", size)

	if err := filewal.MoveFile("data2.wal", "data3.wal"); err != nil {
		fmt.Println("error:", err)
		return
	}
	moved, err := filewal.SizeOf("data3.wal")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("moved size: %d\n", moved)

	gone, err := filewal.DeleteThenCheckGone("data3.wal")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("gone: %v\n", gone)
}
