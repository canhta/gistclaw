package app

import "fmt"

const (
	SystemdServiceUser      = "gistclaw"
	SystemdServiceGroup     = "gistclaw"
	SystemdBinaryPath       = "/usr/local/bin/gistclaw"
	SystemdConfigPath       = "/etc/gistclaw/config.yaml"
	SystemdWorkingDirectory = "/var/lib/gistclaw"
	SystemdServiceUnitPath  = "/etc/systemd/system/gistclaw.service"
)

func RenderSystemdUnit(binaryPath, configPath string) string {
	if binaryPath == "" {
		binaryPath = SystemdBinaryPath
	}
	if configPath == "" {
		configPath = SystemdConfigPath
	}

	return fmt.Sprintf(`[Unit]
Description=GistClaw service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=%s
Group=%s
WorkingDirectory=%s
ExecStart=%s --config %s serve
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
`, SystemdServiceUser, SystemdServiceGroup, SystemdWorkingDirectory, binaryPath, configPath)
}
