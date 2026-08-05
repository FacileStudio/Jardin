package main

import (
	"os"

	"github.com/FacileStudio/Jardin/cmd"
	"github.com/FacileStudio/tronc/healthcheck"
)

func main() {
	if healthcheck.Handle(os.Args) {
		return
	}
	cmd.Execute()
}
