package main

import (
	"os"

	"github.com/wuddleko/genguard/internal/cli"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cli.Version = version
	return cli.Run(args)
}
