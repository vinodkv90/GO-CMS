package cli

import (
	"fmt"
	"runtime"
)

// Version information set at build time via -ldflags.
var (
	Version   = "v0.1.0"
	GitCommit = "HEAD"
	BuildDate = "2026-11-21"
)

// RunVersion prints detailed version information.
func RunVersion() error {
	fmt.Printf("Go CMS %s (%s, build date: %s)\n", Version, GitCommit, BuildDate)
	fmt.Printf("Go Runtime: %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Println("License:    MIT - https://github.com/go-cms/cms")
	return nil
}

