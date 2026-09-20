package main

import (
	"os"

	"github.com/wuddleko/genguard/internal/cli"
)

var version = "dev"

func main() {
	cli.Version = version
	os.Exit(cli.Run(os.Args[1:]))
}
