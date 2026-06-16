package main

// This file is mandatory as otherwise the rtbeat.test binary is not generated correctly.

import (
	"flag"
	"testing"

	"github.com/txn2/rtbeat/cmd"
)

var systemTest *bool

func init() {
	systemTest = flag.Bool("systemTest", false, "Set to true when running system tests")

	// Expose select testing flags on the beat's root command so the
	// system-test harness can drive the binary. The testing package
	// registers flags such as test.coverprofile lazily, so they may be
	// absent at init() time on modern Go — guard against nil lookups.
	for _, name := range []string{"systemTest", "test.coverprofile"} {
		if f := flag.CommandLine.Lookup(name); f != nil {
			cmd.RootCmd.PersistentFlags().AddGoFlag(f)
		}
	}
}

// Test started when the test binary is started. Only calls main.
func TestSystem(t *testing.T) {

	if *systemTest {
		main()
	}
}
