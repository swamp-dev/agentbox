// Package cli provides the command-line interface for agentbox.
package cli

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	verbose bool
	logger  *slog.Logger
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:   "agentbox",
	Short: "Docker-sandboxed AI coding agent CLI",
	Long: `Agentbox is a CLI tool that spins up Docker-sandboxed coding agents
with support for Ralph loops and multi-agent orchestration.

It provides isolated environments for running AI coding agents like
Claude Code, Amp, and Aider safely and efficiently.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logLevel := slog.LevelInfo
		if verbose {
			logLevel = slog.LevelDebug
		}

		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: logLevel,
		}))
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./agentbox.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(ralphCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(imagesCmd)
	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("agentbox")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil && verbose {
		logger.Debug("using config file", "path", viper.ConfigFileUsed())
	}
}
