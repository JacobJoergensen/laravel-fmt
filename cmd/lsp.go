package cmd

import (
	"github.com/spf13/cobra"

	"github.com/JacobJoergensen/laravel-fmt/internal/lsp"
)

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Run the language server over stdio for editor integration",
	Long: `lsp starts a Language Server Protocol server speaking JSON-RPC over
stdin and stdout. Editors that support LSP (VS Code, PhpStorm, Zed, Neovim,
Helix) spawn this to format every file laravel-fmt formats on save. It serves
document and range formatting; configuration is resolved per file exactly as
the CLI resolves it.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return lsp.Serve(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), releaseVersion())
	},
}
