package wizard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		expected string
	}{
		{
			name:     "detects node from package.json",
			files:    map[string]string{"package.json": `{"name": "test"}`},
			expected: "node",
		},
		{
			name:     "detects go from go.mod",
			files:    map[string]string{"go.mod": "module example.com/test"},
			expected: "go",
		},
		{
			name:     "detects rust from Cargo.toml",
			files:    map[string]string{"Cargo.toml": "[package]"},
			expected: "rust",
		},
		{
			name:     "detects python from pyproject.toml",
			files:    map[string]string{"pyproject.toml": "[tool.poetry]"},
			expected: "python",
		},
		{
			name:     "detects python from requirements.txt",
			files:    map[string]string{"requirements.txt": "flask==2.0"},
			expected: "python",
		},
		{
			name:     "returns full when no manifests found",
			files:    map[string]string{},
			expected: "full",
		},
		{
			name:     "prefers package.json over go.mod when both exist",
			files:    map[string]string{"package.json": `{}`, "go.mod": "module test"},
			expected: "node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			result := DetectLanguage(dir)
			if result != tt.expected {
				t.Errorf("DetectLanguage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDetectProjectName(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		dirName  string
		expected string
	}{
		{
			name:     "reads name from package.json",
			files:    map[string]string{"package.json": `{"name": "my-app"}`},
			expected: "my-app",
		},
		{
			name:     "reads name from go.mod module path",
			files:    map[string]string{"go.mod": "module github.com/user/cool-project\n\ngo 1.21"},
			expected: "cool-project",
		},
		{
			name:     "falls back to directory name",
			files:    map[string]string{},
			dirName:  "fallback-project",
			expected: "fallback-project",
		},
		{
			name:     "handles package.json without name field",
			files:    map[string]string{"package.json": `{"version": "1.0"}`},
			dirName:  "dir-name",
			expected: "dir-name",
		},
		{
			name:     "handles malformed package.json",
			files:    map[string]string{"package.json": `not json`},
			dirName:  "dir-name",
			expected: "dir-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dir string
			if tt.dirName != "" {
				parent := t.TempDir()
				dir = filepath.Join(parent, tt.dirName)
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
			} else {
				dir = t.TempDir()
			}

			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			result := DetectProjectName(dir)
			if result != tt.expected {
				t.Errorf("DetectProjectName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDetectQualityChecks_Node(t *testing.T) {
	dir := t.TempDir()
	pkg := `{
		"scripts": {
			"test": "jest",
			"lint": "eslint .",
			"typecheck": "tsc --noEmit",
			"build": "tsc",
			"dev": "nodemon"
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644); err != nil {
		t.Fatal(err)
	}

	checks := DetectQualityChecks(dir, "node")

	// Should detect test, lint, typecheck, build but not dev
	checkNames := map[string]bool{}
	for _, c := range checks {
		checkNames[c.Name] = true
	}

	for _, expected := range []string{"test", "lint", "typecheck", "build"} {
		if !checkNames[expected] {
			t.Errorf("expected quality check %q not found", expected)
		}
	}

	if checkNames["dev"] {
		t.Error("unexpected quality check 'dev' found")
	}
}

func TestDetectQualityChecks_Go(t *testing.T) {
	dir := t.TempDir()
	makefile := `
.PHONY: test lint vet

test:
	go test ./...

lint:
	golangci-lint run

vet:
	go vet ./...
`
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(makefile), 0644); err != nil {
		t.Fatal(err)
	}

	checks := DetectQualityChecks(dir, "go")

	checkNames := map[string]bool{}
	for _, c := range checks {
		checkNames[c.Name] = true
	}

	for _, expected := range []string{"test", "lint", "vet"} {
		if !checkNames[expected] {
			t.Errorf("expected quality check %q not found", expected)
		}
	}
}

func TestDetectQualityChecks_Rust(t *testing.T) {
	dir := t.TempDir()
	checks := DetectQualityChecks(dir, "rust")

	if len(checks) < 2 {
		t.Fatalf("expected at least 2 quality checks for rust, got %d", len(checks))
	}

	checkNames := map[string]bool{}
	for _, c := range checks {
		checkNames[c.Name] = true
	}

	if !checkNames["test"] {
		t.Error("expected 'test' quality check for rust")
	}
	if !checkNames["clippy"] {
		t.Error("expected 'clippy' quality check for rust")
	}
}

func TestDetectQualityChecks_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	checks := DetectQualityChecks(dir, "node")

	// Should return empty or default checks, not panic
	if checks == nil {
		t.Error("expected non-nil slice, got nil")
	}
}

func TestDetectAgents(t *testing.T) {
	// Save and clear env vars
	origAnthropic := os.Getenv("ANTHROPIC_API_KEY")
	origAmp := os.Getenv("AMP_API_KEY")
	origOpenAI := os.Getenv("OPENAI_API_KEY")
	defer func() {
		os.Setenv("ANTHROPIC_API_KEY", origAnthropic)
		os.Setenv("AMP_API_KEY", origAmp)
		os.Setenv("OPENAI_API_KEY", origOpenAI)
	}()

	os.Setenv("ANTHROPIC_API_KEY", "test-key")
	os.Setenv("AMP_API_KEY", "")
	os.Setenv("OPENAI_API_KEY", "")
	os.Unsetenv("AMP_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")

	agents := DetectAgents()

	// Should find at least claude agent from ANTHROPIC_API_KEY
	found := false
	for _, a := range agents {
		if a.Name == "claude" && a.Available {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find available 'claude' agent from ANTHROPIC_API_KEY")
	}
}

func TestDetectAgents_AmpKey(t *testing.T) {
	origAmp := os.Getenv("AMP_API_KEY")
	defer os.Setenv("AMP_API_KEY", origAmp)

	os.Setenv("AMP_API_KEY", "test-amp-key")

	agents := DetectAgents()

	found := false
	for _, a := range agents {
		if a.Name == "amp" && a.Available {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find available 'amp' agent from AMP_API_KEY")
	}
}

func TestDetectQualityChecks_Python(t *testing.T) {
	dir := t.TempDir()
	pyproject := `
[tool.poetry.scripts]
test = "pytest"

[tool.pytest.ini_options]
testpaths = ["tests"]

[tool.mypy]
strict = true

[tool.ruff]
line-length = 88
`
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(pyproject), 0644); err != nil {
		t.Fatal(err)
	}

	checks := DetectQualityChecks(dir, "python")

	checkNames := map[string]bool{}
	for _, c := range checks {
		checkNames[c.Name] = true
	}

	if !checkNames["test"] {
		t.Error("expected 'test' quality check for python with pytest")
	}
}
