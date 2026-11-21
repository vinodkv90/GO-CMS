package main

import (
	"fmt"
	"os"

	"github.com/go-cms/cms/internal/cli"
)

func main() {
	if err := cli.Execute(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Error: %v\n\n", err)
		os.Exit(1)
	}
}

