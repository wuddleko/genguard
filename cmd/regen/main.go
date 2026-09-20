package main

import (
	"os"

	"github.com/regen-check/regen/internal/cli"
)

var version = "dev"

func main() {
	cli.Version = version
	os.Exit(cli.Run(os.Args[1:]))
}
