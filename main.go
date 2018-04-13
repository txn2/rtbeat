package main

import (
	"os"

	"github.com/cjimti/rtbeat/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
