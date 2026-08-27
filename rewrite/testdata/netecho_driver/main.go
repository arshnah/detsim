package main

import (
	"flag"
	"fmt"

	"github.com/arshnah/detsim/rewrite/testdata/netecho"
	"github.com/arshnah/detsim/rt"
)

func main() {
	seed := flag.Int64("seed", 1, "scheduler seed")
	flag.Parse()

	sched := rt.NewSched(*seed)
	netecho.DetsimSetSched(sched)
	network := rt.NewNetwork(sched)
	network.SetDelayRange(1, 10)
	netecho.DetsimSetNetwork(network)

	var reply []byte
	var echoErr error

	sched.Go(func() {
		if err := netecho.Serve("echo-server"); err != nil {
			echoErr = err
			return
		}
		reply, echoErr = netecho.Echo("echo-server", []byte("ping"))
	})

	if err := sched.Run(); err != nil {
		fmt.Println("error:", err)
		return
	}
	if echoErr != nil {
		fmt.Println("error:", echoErr)
		return
	}
	fmt.Printf("%s\n", reply)

	var addr string
	var addrErr error
	sched2 := rt.NewSched(*seed)
	netecho.DetsimSetSched(sched2)
	network2 := rt.NewNetwork(sched2)
	netecho.DetsimSetNetwork(network2)
	sched2.Go(func() {
		l, err := network2.Listen("echo-server")
		if err != nil {
			addrErr = err
			return
		}
		sched2.Go(func() {
			l.Accept()
		})
		a, err := netecho.RemoteAddrOf("echo-server")
		if err != nil {
			addrErr = err
			return
		}
		addr = a.String()
	})
	if err := sched2.Run(); err != nil {
		fmt.Println("error:", err)
		return
	}
	if addrErr != nil {
		fmt.Println("error:", addrErr)
		return
	}
	fmt.Printf("remote addr: %s\n", addr)
}
