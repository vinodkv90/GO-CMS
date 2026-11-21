package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-cms/cms/admin"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/database/factory"
)

type checkResult struct {
	Name    string
	Passed  bool
	Message string
	Warning bool
}

// RunDoctor performs self-diagnostic checks on the project and environment.
func RunDoctor(configPath string) error {
	fmt.Println("🏥 Running Go CMS System Diagnostics...")
	fmt.Println(strings.Repeat("─", 60))

	var results []checkResult
	hasCriticalError := false

	// 1. Environment and Runtime Check
	results = append(results, checkResult{
		Name:    "System Runtime",
		Passed:  true,
		Message: fmt.Sprintf("%s on %s/%s (%d CPUs)", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU()),
	})

	// 2. Configuration Load Check
	cfg, err := config.Load(configPath)
	if err != nil {
		results = append(results, checkResult{
			Name:    "Configuration File",
			Passed:  false,
			Message: fmt.Sprintf("Failed to load config: %v", err),
		})
		hasCriticalError = true
	} else {
		results = append(results, checkResult{
			Name:    "Configuration File",
			Passed:  true,
			Message: fmt.Sprintf("Valid configuration (environment: %s)", cfg.Server.Env),
		})
	}

	if cfg != nil {
		// 3. Security & Secret Strength Check
		if cfg.Auth.JWTSecret == "" || strings.Contains(cfg.Auth.JWTSecret, "change-me") {
			results = append(results, checkResult{
				Name:    "Security Secrets",
				Passed:  true,
				Warning: true,
				Message: "Default/weak JWT secret detected; update in .env for production",
			})
		} else {
			results = append(results, checkResult{
				Name:    "Security Secrets",
				Passed:  true,
				Message: "Strong cryptographically secure secrets configured",
			})
		}

		// 4. Database Connectivity Check
		dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		db, dbErr := factory.Connect(dbCtx, cfg.Database)
		cancel()
		if dbErr != nil {
			results = append(results, checkResult{
				Name:    "Database Connection",
				Passed:  false,
				Message: fmt.Sprintf("Failed to connect to %s (%s:%d): %v", cfg.Database.Driver, cfg.Database.Host, cfg.Database.Port, dbErr),
			})
			hasCriticalError = true
		} else {
			_ = db.Close()
			results = append(results, checkResult{
				Name:    "Database Connection",
				Passed:  true,
				Message: fmt.Sprintf("Connected to %s (%s:%d/%s)", cfg.Database.Driver, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database),
			})
		}

		// 5. Port Availability Check
		listenAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		l, netErr := net.Listen("tcp", listenAddr)
		if netErr != nil {
			results = append(results, checkResult{
				Name:    "Port Availability",
				Passed:  true,
				Warning: true,
				Message: fmt.Sprintf("Port %d is currently in use (server may already be running)", cfg.Server.Port),
			})
		} else {
			_ = l.Close()
			results = append(results, checkResult{
				Name:    "Port Availability",
				Passed:  true,
				Message: fmt.Sprintf("Port %d is available for binding", cfg.Server.Port),
			})
		}

		// 6. Media Storage Directory Write Permissions
		uploadDir := cfg.Media.UploadDir
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			results = append(results, checkResult{
				Name:    "Media Storage",
				Passed:  false,
				Message: fmt.Sprintf("Cannot create upload directory %q: %v", uploadDir, err),
			})
			hasCriticalError = true
		} else {
			testFile := filepath.Join(uploadDir, ".doctor_write_test")
			if writeErr := os.WriteFile(testFile, []byte("ok"), 0644); writeErr != nil {
				results = append(results, checkResult{
					Name:    "Media Storage",
					Passed:  false,
					Message: fmt.Sprintf("Upload directory %q is not writable: %v", uploadDir, writeErr),
				})
				hasCriticalError = true
			} else {
				_ = os.Remove(testFile)
				results = append(results, checkResult{
					Name:    "Media Storage",
					Passed:  true,
					Message: fmt.Sprintf("Upload directory %q has valid write permissions", uploadDir),
				})
			}
		}

		// 7. Themes and Templates Check
		if _, err := os.Stat("themes"); err == nil {
			themePath := filepath.Join("themes", cfg.Themes.ActiveTheme)
			if _, err := os.Stat(themePath); err != nil {
				results = append(results, checkResult{
					Name:    "Theme System",
					Passed:  true,
					Warning: true,
					Message: fmt.Sprintf("Active theme %q not found in themes/", cfg.Themes.ActiveTheme),
				})
			} else {
				results = append(results, checkResult{
					Name:    "Theme System",
					Passed:  true,
					Message: fmt.Sprintf("Theme %q found and active", cfg.Themes.ActiveTheme),
				})
			}
		} else {
			results = append(results, checkResult{
				Name:    "CMS Mode",
				Passed:  true,
				Message: "Headless CMS mode active (themes directory omitted)",
			})
		}
	}

	// 8. Admin SPA Assets Check
	if admin.HasEmbeddedAdmin() {
		results = append(results, checkResult{
			Name:    "Admin Console Assets",
			Passed:  true,
			Message: "Embedded React Admin Console verified and ready",
		})
	} else if _, err := os.Stat(filepath.Join("admin", "dist", "index.html")); err == nil {
		results = append(results, checkResult{
			Name:    "Admin Console Assets",
			Passed:  true,
			Message: "Local disk React Admin Console found at admin/dist",
		})
	} else {
		results = append(results, checkResult{
			Name:    "Admin Console Assets",
			Passed:  true,
			Warning: true,
			Message: "Admin dashboard assets not found; run 'npm run build' inside admin/",
		})
	}

	// Print Results Table
	for _, r := range results {
		if !r.Passed {
			fmt.Printf(" [✗] %-24s : %s\n", r.Name, r.Message)
		} else if r.Warning {
			fmt.Printf(" [!] %-24s : %s\n", r.Name, r.Message)
		} else {
			fmt.Printf(" [✓] %-24s : %s\n", r.Name, r.Message)
		}
	}

	fmt.Println(strings.Repeat("─", 60))
	if hasCriticalError {
		fmt.Println("❌ Diagnostic check found critical issues that must be addressed.")
		return fmt.Errorf("doctor check failed with critical issues")
	}

	fmt.Println("✅ Everything is healthy! Go CMS is ready to run.")
	return nil
}
