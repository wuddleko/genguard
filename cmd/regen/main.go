package main

import (
	"os"

	"github.com/wuddleko/regen/internal/cli"
)

var version = "dev"

func main() {
	cli.Version = version
	os.Exit(cli.Run(os.Args[1:]))
}
