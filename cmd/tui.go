package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"anime-tracker/internal/tui"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive TUI (also the default when no command is given)",
	RunE:  runTUI,
}

func runTUI(cmd *cobra.Command, args []string) error {
	store, cfg, closeStore, err := openStore(cmd)
	if err != nil {
		return err
	}
	defer closeStore()

	model := tui.NewModel(store, cfg.Dir)
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}
