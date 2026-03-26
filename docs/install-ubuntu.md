# Install On Ubuntu 24

This is the blessed self-hosting path for GistClaw `v0.1.0`.

## What you get

- one GitHub Releases flow
- one self-contained binary release
- one installer script
- one `systemd` service
- obvious logs through `journalctl`

## Install from GitHub Releases

1. Download the installer from [GitHub Releases](https://github.com/canhta/gistclaw/releases).
2. Run it with the release version and provider config you want:

```bash
curl -fsSLO https://github.com/canhta/gistclaw/releases/download/v0.1.0/gistclaw-install.sh
chmod +x gistclaw-install.sh
sudo ./gistclaw-install.sh --version v0.1.0 --provider-name openai --provider-api-key YOUR_REAL_KEY
```

The installer writes:

- `/usr/local/bin/gistclaw`
- `/etc/gistclaw/config.yaml` as a root-owned file readable by the `gistclaw` service group
- `/etc/systemd/system/gistclaw.service`
- `/var/lib/gistclaw` as the service-owned state directory

## Verify the service

```bash
gistclaw version
systemctl status gistclaw
journalctl -u gistclaw -n 100 --no-pager
gistclaw doctor --config /etc/gistclaw/config.yaml
gistclaw security audit --config /etc/gistclaw/config.yaml
```

## Update cleanly

Back up first, then rerun the installer for the next version:

```bash
sudo gistclaw backup --db /var/lib/gistclaw/runtime.db
sudo ./gistclaw-install.sh --version v0.1.0 --provider-name openai --provider-api-key YOUR_REAL_KEY
systemctl status gistclaw
```

## Debugging

The boring operator path is:

```bash
systemctl status gistclaw
journalctl -u gistclaw --since "15 minutes ago"
gistclaw doctor --config /etc/gistclaw/config.yaml
gistclaw inspect status --config /etc/gistclaw/config.yaml
```
