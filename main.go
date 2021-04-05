package main

import (
	"flag"
	"pinger/pinger"
)

func main() {
	var target string
	flag.StringVar(&target, "target", "", "echo ping target")

	pinger, err := pinger.NewPinger(target)
	if err != nil {
		panic(err)
	}

	err = pinger.Run()
	if err != nil {
		panic(err)
	}
}
