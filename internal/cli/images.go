package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

// execCommand is a variable for testability (can be overridden in tests).
var execCommand = exec.Command

var imagesCmd = &cobra.Command{
	Use:   "images",
	Short: "Manage agentbox Docker images",
	Long: `Images provides commands for managing agentbox Docker images.

Subcommands:
  list   - List available agentbox images
  pull   - Pull agentbox images from registry
  build  - Build images from local Dockerfiles`,
}

var imagesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available agentbox images",
	RunE:  runImagesList,
}

var imagesPullCmd = &cobra.Command{
	Use:   "pull [image]",
	Short: "Pull agentbox images",
	Long: `Pull agentbox Docker images from the registry.

Examples:
  agentbox images pull          # Pull all images
  agentbox images pull node     # Pull only node image
  agentbox images pull full     # Pull the full image`,
	RunE: runImagesPull,
}

var imagesBuildCmd = &cobra.Command{
	Use:   "build [image]",
	Short: "Build agentbox images locally",
	Long: `Build agentbox Docker images from local Dockerfiles.

Examples:
  agentbox images build          # Build all images
  agentbox images build node     # Build only node image`,
	RunE: runImagesBuild,
}

func init() {
	imagesCmd.AddCommand(imagesListCmd)
	imagesCmd.AddCommand(imagesPullCmd)
	imagesCmd.AddCommand(imagesBuildCmd)
}

var availableImages = []struct {
	Name        string
	Tag         string
	Description string
}{
	{"agentbox/node", "20", "Node.js 20, npm, pnpm, Claude Code"},
	{"agentbox/python", "3.12", "Python 3.12, pip, poetry, uv"},
	{"agentbox/go", "1.22", "Go 1.22, common tools"},
	{"agentbox/rust", "1.77", "Rust, cargo"},
	{"agentbox/full", "latest", "All languages + all agents"},
}

func runImagesList(cmd *cobra.Command, args []string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()

	ctx := context.Background()
	images, err := cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing images: %w", err)
	}

	installed := make(map[string]bool)
	for _, img := range images {
		for _, tag := range img.RepoTags {
			installed[tag] = true
		}
	}

	fmt.Println("Agentbox Docker Images")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("%-25s %-10s %-10s %s\n", "IMAGE", "TAG", "STATUS", "DESCRIPTION")
	fmt.Println("───────────────────────────────────────────────────────────────")

	for _, img := range availableImages {
		fullName := fmt.Sprintf("%s:%s", img.Name, img.Tag)
		status := "not built"
		if installed[fullName] {
			status = "installed"
		}
		fmt.Printf("%-25s %-10s %-10s %s\n", img.Name, img.Tag, status, img.Description)
	}

	fmt.Println("───────────────────────────────────────────────────────────────")
	fmt.Println("\nUse 'agentbox images build <image>' to build images locally")

	return nil
}

func runImagesPull(cmd *cobra.Command, args []string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer cli.Close()

	ctx := context.Background()

	var imagesToPull []struct {
		Name string
		Tag  string
	}

	if len(args) == 0 {
		for _, img := range availableImages {
			imagesToPull = append(imagesToPull, struct {
				Name string
				Tag  string
			}{img.Name, img.Tag})
		}
	} else {
		for _, arg := range args {
			for _, img := range availableImages {
				shortName := strings.TrimPrefix(img.Name, "agentbox/")
				if arg == shortName || arg == img.Name {
					imagesToPull = append(imagesToPull, struct {
						Name string
						Tag  string
					}{img.Name, img.Tag})
				}
			}
		}
	}

	if len(imagesToPull) == 0 {
		return fmt.Errorf("no matching images found")
	}

	for _, img := range imagesToPull {
		fullName := fmt.Sprintf("%s:%s", img.Name, img.Tag)
		logger.Info("pulling image", "image", fullName)

		reader, err := cli.ImagePull(ctx, fullName, image.PullOptions{})
		if err != nil {
			logger.Error("failed to pull image", "image", fullName, "error", err)
			continue
		}

		_, _ = io.Copy(os.Stdout, reader)
		reader.Close()
		fmt.Println()
	}

	return nil
}

func runImagesBuild(cmd *cobra.Command, args []string) error {
	// Find the images directory (next to binary, or in CWD)
	imagesDir := "images"
	if _, err := os.Stat(imagesDir); err != nil {
		// Try relative to executable
		if exe, err := os.Executable(); err == nil {
			candidate := filepath.Join(filepath.Dir(exe), "..", "images")
			if _, err := os.Stat(candidate); err == nil {
				imagesDir = candidate
			}
		}
	}

	if _, err := os.Stat(imagesDir); err != nil {
		return fmt.Errorf("images directory not found. Run from the agentbox repo root, or ensure images/ is next to the binary")
	}

	var toBuild []struct {
		Name string
		Tag  string
	}

	if len(args) == 0 {
		for _, img := range availableImages {
			shortName := strings.TrimPrefix(img.Name, "agentbox/")
			dockerfilePath := filepath.Join(imagesDir, shortName, "Dockerfile")
			if _, err := os.Stat(dockerfilePath); err == nil {
				toBuild = append(toBuild, struct {
					Name string
					Tag  string
				}{img.Name, img.Tag})
			}
		}
	} else {
		for _, arg := range args {
			for _, img := range availableImages {
				shortName := strings.TrimPrefix(img.Name, "agentbox/")
				if arg == shortName || arg == img.Name {
					toBuild = append(toBuild, struct {
						Name string
						Tag  string
					}{img.Name, img.Tag})
				}
			}
		}
	}

	if len(toBuild) == 0 {
		return fmt.Errorf("no matching images found with Dockerfiles in %s", imagesDir)
	}

	for _, img := range toBuild {
		fullName := fmt.Sprintf("%s:%s", img.Name, img.Tag)
		shortName := strings.TrimPrefix(img.Name, "agentbox/")
		dockerfilePath := filepath.Join(imagesDir, shortName, "Dockerfile")

		fmt.Printf("Building %s...\n", fullName)

		// Use docker CLI via exec since the SDK's ImageBuild requires a tar context
		buildCmd := fmt.Sprintf("docker build -t %s -f %s .", fullName, dockerfilePath)
		c := execCommand("sh", "-c", buildCmd)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Printf("Failed to build %s: %v\n", fullName, err)
			continue
		}
		fmt.Printf("Successfully built %s\n\n", fullName)
	}

	return nil
}
