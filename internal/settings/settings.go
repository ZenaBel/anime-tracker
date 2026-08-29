// Package settings names the qBittorrent/SSH config keys shared by the CLI
// (cmd/config.go) and the TUI's settings overlay, so both stay in sync
// without duplicating the key list.
package settings

// Keys are the only settings anime-tracker's remote-download feature uses,
// in the order they should be shown/edited.
var Keys = []string{
	"qbt.url",
	"qbt.username",
	"qbt.password",
	"qbt.insecure_tls",
	"remote.ssh_target",
	"remote.root",
	"remote.host_root",
}

// PasswordKey never gets its stored value shown or pre-filled for editing.
const PasswordKey = "qbt.password"

// Valid reports whether key is one of Keys.
func Valid(key string) bool {
	for _, k := range Keys {
		if k == key {
			return true
		}
	}
	return false
}
