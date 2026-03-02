package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	mode := flag.String("mode", "ssh", "capture mode: ssh|pcap|stdin")
	flag.Parse()

	fmt.Printf("hasstcp starting (mode=%s)\n", *mode)
	// Wire up later
	if *mode == "ssh" {
		log.Println("SSH capture not yet implemented")
		os.Exit(0)
	}
	if *mode == "pcap" {
		log.Println("PCAP capture not yet implemented")
		os.Exit(0)
	}
	if *mode == "stdin" {
		log.Println("STDIN capture not yet implemented")
		os.Exit(0)
	}
}

