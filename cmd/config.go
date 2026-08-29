package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"anime-tracker/internal/settings"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Get or set remote qBittorrent/SSH settings, stored in the local db",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> [value]",
	Short: "Set a config value (qbt.url, qbt.username, qbt.password, qbt.insecure_tls, remote.ssh_target, remote.root, remote.host_root)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		if !settings.Valid(key) {
			return fmt.Errorf("unknown config key %q (see `config set --help`)", key)
		}

		var value string
		if len(args) == 2 {
			value = args[1]
		} else if key == settings.PasswordKey {
			fmt.Print("qbt.password: ")
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
			value = string(b)
		} else {
			return fmt.Errorf("missing value for %q", key)
		}

		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		if err := store.SetSetting(cmd.Context(), key, value); err != nil {
			return err
		}
		fmt.Printf("set %s\n", key)
		return nil
	},
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset <key>",
	Short: "Remove a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		if !settings.Valid(key) {
			return fmt.Errorf("unknown config key %q", key)
		}

		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		if err := store.UnsetSetting(cmd.Context(), key); err != nil {
			return err
		}
		fmt.Printf("unset %s\n", key)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print current config values (qbt.password is masked)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		all, err := store.AllSettings(cmd.Context())
		if err != nil {
			return err
		}

		for _, k := range settings.Keys {
			v, ok := all[k]
			switch {
			case !ok:
				fmt.Printf("%-18s (not set)\n", k)
			case k == settings.PasswordKey:
				fmt.Printf("%-18s ********\n", k)
			default:
				fmt.Printf("%-18s %s\n", k, v)
			}
		}
		return nil
	},
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configUnsetCmd)
	configCmd.AddCommand(configShowCmd)
}
