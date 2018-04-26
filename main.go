package main

import (
	"os"

	"github.com/txn2/rtbeat/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
