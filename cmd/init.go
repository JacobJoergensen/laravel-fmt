package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
	"github.com/JacobJoergensen/laravel-fmt/internal/detector"
	"github.com/JacobJoergensen/laravel-fmt/internal/terminal"
)

var initForce bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a default laravel-fmt.json configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		workDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve working directory: %w", err)
		}

		ctx, err := detector.Detect(workDir)
		if err != nil {
			return fmt.Errorf("detect project context: %w", err)
		}

		path := filepath.Join(ctx.ConfigDir(), config.FileName)

		shown, err := filepath.Rel(workDir, path)
		if err != nil {
			shown = path
		}

		_, err = os.Stat(path)

		if err == nil && !initForce {
			return fmt.Errorf("%s already exists (use --force to overwrite)", shown)
		}

		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("check %s: %w", shown, err)
		}

		cfg := config.Default()
		detector.Apply(cfg, ctx)
		contents, err := config.Generate(cfg)
		if err != nil {
			return fmt.Errorf("generate config: %w", err)
		}

		if err := os.WriteFile(path, contents, 0o644); err != nil { //nolint:gosec // a config file is read like source, not a secret
			return fmt.Errorf("write %s: %w", shown, err)
		}

		cmd.PrintErrf("%s%s%s %sCreated %s%s\n", terminal.Green, terminal.CheckMark, terminal.Reset, terminal.Bold, shown, terminal.Reset)

		return nil
	},
}

func init() {
	initCmd.Flags().BoolVar(&initForce, "force", false, "overwrite an existing laravel-fmt.json")
}
