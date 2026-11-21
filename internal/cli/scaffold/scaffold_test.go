package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScaffoldTemplatedBlog(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scaffold-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := Options{
		TargetDir:   tempDir,
		ProjectName: "my-blog-cms",
		Mode:        ModeTemplated,
		Template:    TemplateBlog,
		ServerPort:  8085,
	}

	if err := Generate(opts); err != nil {
		t.Fatalf("Scaffold Generate failed: %v", err)
	}

	requiredFiles := []string{
		"config.yaml",
		".env",
		"package.json",
		"README.md",
		".gitignore",
		filepath.Join("scripts", "go-cms-runner.js"),
		filepath.Join("themes", "default", "theme.json"),
		filepath.Join("themes", "default", "templates", "pages", "home.html"),
		filepath.Join("themes", "default", "templates", "layouts", "base.html"),
	}

	for _, rel := range requiredFiles {
		p := filepath.Join(tempDir, rel)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("expected file %s does not exist", rel)
		}
	}

	// Verify .env has generated secrets
	envContent, err := os.ReadFile(filepath.Join(tempDir, ".env"))
	if err != nil {
		t.Fatalf("failed to read .env: %v", err)
	}
	envStr := string(envContent)
	if len(envStr) < 100 {
		t.Errorf(".env file is too short: %s", envStr)
	}
}

func TestScaffoldHeadlessBlank(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scaffold-headless-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	opts := Options{
		TargetDir:   tempDir,
		ProjectName: "my-api-cms",
		Mode:        ModeHeadless,
		Template:    TemplateBlank,
		ServerPort:  8090,
	}

	if err := Generate(opts); err != nil {
		t.Fatalf("Scaffold Generate failed: %v", err)
	}

	requiredFiles := []string{
		"config.yaml",
		".env",
		"package.json",
		"README.md",
		".gitignore",
	}

	for _, rel := range requiredFiles {
		p := filepath.Join(tempDir, rel)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("expected file %s does not exist", rel)
		}
	}
}
