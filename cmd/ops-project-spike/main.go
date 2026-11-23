package main

import (
	"fmt"
	"os"

	"opscopilot/pkg/projectspike"
)

func main() {
	if err := projectspike.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
