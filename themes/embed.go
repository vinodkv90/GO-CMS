package themes

import (
	"embed"
	"io/fs"
)

//go:embed all:default
var DefaultThemeFS embed.FS

// GetDefaultThemeFS returns the embedded filesystem rooted at the default theme.
func GetDefaultThemeFS() (fs.FS, error) {
	return fs.Sub(DefaultThemeFS, "default")
}

