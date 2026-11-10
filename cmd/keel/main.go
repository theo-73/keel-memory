package main

import (
	"os"

	"keel/pkg/cli"
)

func main() {
	app := cli.NewApp(os.Stdin, os.Stdout, os.Stderr)
	os.Exit(app.Run(os.Args[1:]))
}
