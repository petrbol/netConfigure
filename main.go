package main

import (
	"flag"
	"precioz.net/netConfigure/lib"
)

func main() {
	flag.StringVar(&lib.StartFlags.ListenAddr, "listenAddr", "", "Server listen address")
	flag.StringVar(&lib.StartFlags.ListenPort, "listenPort", "8080", "Server listen port")
	flag.Parse()

	// Start app
	lib.Start()
}
