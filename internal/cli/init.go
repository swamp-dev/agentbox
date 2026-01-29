package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/swamp-dev/agentbox/internal/config"
	"github.com/swamp-dev/agentbox/internal/ralph"
)

var (
	initTemplate string
	initLanguage string
	initName     string
	initForce    bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize project with agentbox templates",
	Long: `Initialize creates the necessary files for using agentbox in a project.

This includes:
- agentbox.yaml - configuration file
- prd.json - product requirements document
- progress.txt - Ralph loop progress tracking

Examples:
  agentbox init
  agentbox init --template standard --language node
  agentbox init --name my-project --force`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initTemplate, "template", "t", "standard", "template to use (standard, minimal)")
	initCmd.Flags().StringVarP(&initLanguage, "language", "l", "", "language/runtime (node, python, go, rust)")
	initCmd.Flags().StringVarP(&initName, "name", "n", "", "project name (defaults to directory name)")
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "overwrite existing files")
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	if initName == "" {
		initName = filepath.Base(cwd)
	}

	logger.Info("initializing agentbox project",
		"name", initName,
		"template", initTemplate,
		"language", initLanguage,
	)

	if err := createConfigFile(cwd, initName); err != nil {
		return err
	}

	if err := createPRDFile(cwd, initName); err != nil {
		return err
	}

	if err := createProgressFile(cwd, initName); err != nil {
		return err
	}

	if err := createAgentsMD(cwd); err != nil {
		return err
	}

	fmt.Printf("\n✓ Initialized agentbox project: %s\n", initName)
	fmt.Println("\nCreated files:")
	fmt.Println("  - agentbox.yaml  (configuration)")
	fmt.Println("  - prd.json       (task definitions)")
	fmt.Println("  - progress.txt   (execution log)")
	fmt.Println("  - AGENTS.md      (agent patterns)")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Edit prd.json to define your tasks")
	fmt.Println("  2. Run 'agentbox ralph' to start the loop")

	return nil
}

func createConfigFile(dir, name string) error {
	path := filepath.Join(dir, "agentbox.yaml")

	if !initForce {
		if _, err := os.Stat(path); err == nil {
			logger.Info("agentbox.yaml already exists, skipping")
			return nil
		}
	}

	cfg := config.DefaultConfig()
	cfg.Project.Name = name

	if initLanguage != "" {
		cfg.Docker.Image = initLanguage
	}

	if initTemplate == "minimal" {
		cfg.Ralph.QualityChecks = nil
	} else {
		cfg.Ralph.QualityChecks = []config.QualityCheck{
			{Name: "typecheck", Command: "npm run typecheck 2>/dev/null || true"},
			{Name: "test", Command: "npm test 2>/dev/null || true"},
		}
	}

	if err := cfg.Save(path); err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}

	logger.Info("created agentbox.yaml")
	return nil
}

func createPRDFile(dir, name string) error {
	path := filepath.Join(dir, "prd.json")

	if !initForce {
		if _, err := os.Stat(path); err == nil {
			logger.Info("prd.json already exists, skipping")
			return nil
		}
	}

	prd := ralph.CreateDefaultPRD(name)
	if err := prd.Save(path); err != nil {
		return fmt.Errorf("creating PRD file: %w", err)
	}

	logger.Info("created prd.json")
	return nil
}

func createProgressFile(dir, name string) error {
	path := filepath.Join(dir, "progress.txt")

	if !initForce {
		if _, err := os.Stat(path); err == nil {
			logger.Info("progress.txt already exists, skipping")
			return nil
		}
	}

	if err := ralph.CreateProgressFile(path, name); err != nil {
		return fmt.Errorf("creating progress file: %w", err)
	}

	logger.Info("created progress.txt")
	return nil
}

func createAgentsMD(dir string) error {
	path := filepath.Join(dir, "AGENTS.md")

	if !initForce {
		if _, err := os.Stat(path); err == nil {
			logger.Info("AGENTS.md already exists, skipping")
			return nil
		}
	}

	content := `# AGENTS.md

This file documents patterns and learnings discovered by AI agents working on this project.

## Conventions

<!-- Document coding conventions discovered during development -->

## Patterns

<!-- Document recurring patterns in the codebase -->

## Gotchas

<!-- Document tricky areas or non-obvious behaviors -->

## Commands

<!-- Document useful commands for this project -->

## Notes

<!-- Additional notes from agent sessions -->
`

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("creating AGENTS.md: %w", err)
	}

	logger.Info("created AGENTS.md")
	return nil
}
