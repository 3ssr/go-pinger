package main

import (
	"flag"
	"fmt"
	"pinger/ping"
	"time"
)

func main() {
	var target string
	flag.StringVar(&target, "target", "", "echo ping target")
	flag.Parse()

	pinger, err := ping.NewPinger(target, 5, 1*time.Second)
	if err != nil {
		panic(err)
	}

	pinger.OnRecv = func(pkg *ping.Packet) {
		fmt.Printf("%d bytes from %s: icmp_seq=%d time=%v ttl=%v\n", pkg.Bytes, pkg.IPAddr, pkg.Seq, pkg.Rtt, pkg.Ttl)
	}

	pinger.OnFinish = func() {

	}

	err = pinger.Run()
	if err != nil {
		panic(err)
	}
}
