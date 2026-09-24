package main

import (
	"os"

	"github.com/wuddleko/genguard/internal/cli"
)

var version = "dev"

var osExit = os.Exit

func main() {
	osExit(run(os.Args[1:]))
}

func run(args []string) int {
	cli.Version = version
	return cli.Run(args)
}
