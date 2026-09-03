package main

import (
	"fmt"
	"opscopilot/internal/shellsidecar"
	"os"
)

var version = "dev"

func main() {
	if err := shellsidecar.Run(version, os.Args[1:], ""); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
