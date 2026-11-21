package cli

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/templates"
)

// RunBuild validates project templates, schemas, and configuration for production readiness.
func RunBuild(configPath string) error {
	fmt.Println("🏗️  Building and validating Go CMS project...")
	fmt.Println(strings.Repeat("─", 60))

	// 1. Validate Configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}
	fmt.Printf(" [✓] Configuration file valid (target environment: %s)\n", cfg.Server.Env)

	// 2. Validate Templates (if themes directory exists)
	if _, err := os.Stat("themes"); err == nil {
		templateCount := 0
		themeEntries, err := os.ReadDir("themes")
		if err != nil {
			return fmt.Errorf("failed to read themes directory: %w", err)
		}

		for _, t := range themeEntries {
			if !t.IsDir() {
				continue
			}
			themeDir := filepath.Join("themes", t.Name())
			themeJSON := filepath.Join(themeDir, "theme.json")
			if _, err := os.Stat(themeJSON); err != nil {
				return fmt.Errorf("theme %q missing theme.json manifest: %w", t.Name(), err)
			}

			// Validate all templates parse cleanly with SafeFuncMap
			funcMap := templates.SafeFuncMap(t.Name())
			tmpl := template.New("validate").Funcs(funcMap)

			var templateFiles []string
			tmplDir := filepath.Join(themeDir, "templates")
			if _, err := os.Stat(tmplDir); err == nil {
				_ = filepath.Walk(tmplDir, func(path string, info os.FileInfo, walkErr error) error {
					if walkErr != nil || info == nil || info.IsDir() {
						return walkErr
					}
					if strings.HasSuffix(info.Name(), ".html") {
						templateFiles = append(templateFiles, path)
					}
					return nil
				})
			}

			if len(templateFiles) > 0 {
				if _, parseErr := tmpl.ParseFiles(templateFiles...); parseErr != nil {
					return fmt.Errorf("template syntax error in theme %q: %w", t.Name(), parseErr)
				}
				templateCount += len(templateFiles)
			}
		}
		fmt.Printf(" [✓] Template engine validation passed (%d HTML templates parsed)\n", templateCount)
	} else {
		fmt.Println(" [✓] Headless CMS mode active (templates omitted)")
	}

	// 3. Validate Content Type Schemas (if schemas directory exists)
	if _, err := os.Stat("schemas"); err == nil {
		schemaCount := 0
		entries, err := os.ReadDir("schemas")
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
					data, readErr := os.ReadFile(filepath.Join("schemas", e.Name()))
					if readErr != nil {
						return fmt.Errorf("failed reading schema %q: %w", e.Name(), readErr)
					}
					var parsed map[string]any
					if jsonErr := json.Unmarshal(data, &parsed); jsonErr != nil {
						return fmt.Errorf("invalid JSON schema in %q: %w", e.Name(), jsonErr)
					}
					schemaCount++
				}
			}
		}
		if schemaCount > 0 {
			fmt.Printf(" [✓] Content type schemas validated (%d schemas)\n", schemaCount)
		}
	}

	// 4. Verify Media Storage Directory
	uploadDir := cfg.Media.UploadDir
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return fmt.Errorf("failed to prepare uploads directory %q: %w", uploadDir, err)
	}
	fmt.Printf(" [✓] Storage directory verified (%s)\n", uploadDir)

	fmt.Println(strings.Repeat("─", 60))
	fmt.Println("✨ Go CMS project validated and ready for production deployment.")
	return nil
}

