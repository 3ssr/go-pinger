package main

import (
	"flag"
	"fmt"
	ping "pinger"
	"time"
)

func main() {
	var target string
	flag.StringVar(&target, "target", "", "echo ping target")
	flag.Parse()

	pinger, err := ping.NewPinger(target, 5, 30*time.Second, 1*time.Second)
	if err != nil {
		panic(err)
	}

	pinger.OnRecv = func(pkg *ping.Packet) {
		fmt.Printf("%d bytes from %s: icmp_seq=%d time=%v ttl=%v\n", pkg.NBytes, pkg.IPAddr, pkg.Seq, pkg.Rtt, pkg.Ttl)
	}

	pinger.OnFinish = func(statistic ping.PingStatistic) {
		fmt.Printf("on finish %+v\n", statistic)
	}

	err = pinger.Run()
	if err != nil {
		panic(err)
	}
}
