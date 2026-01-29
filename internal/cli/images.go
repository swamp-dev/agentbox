package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

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
		status := "not installed"
		if installed[fullName] {
			status = "installed"
		}
		fmt.Printf("%-25s %-10s %-10s %s\n", img.Name, img.Tag, status, img.Description)
	}

	fmt.Println("───────────────────────────────────────────────────────────────")
	fmt.Println("\nUse 'agentbox images pull <image>' to download images")

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

		io.Copy(os.Stdout, reader)
		reader.Close()
		fmt.Println()
	}

	return nil
}

func runImagesBuild(cmd *cobra.Command, args []string) error {
	fmt.Println("Building images from local Dockerfiles...")
	fmt.Println("This feature requires the images/ directory with Dockerfiles.")
	fmt.Println("\nTo build manually:")
	fmt.Println("  docker build -t agentbox/node:20 -f images/node/Dockerfile .")
	fmt.Println("  docker build -t agentbox/full:latest -f images/full/Dockerfile .")

	return nil
}
