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

If you also want the installer to manage the public HTTPS front door, add `--public-domain your.domain.example`:

```bash
sudo ./gistclaw-install.sh --version v0.1.0 --provider-name openai --provider-api-key YOUR_REAL_KEY --public-domain your.domain.example
```

If you already have a full operator config, install from that exact config file instead of re-entering fields. That file should already include `database_path` and `storage_root`.

```bash
sudo ./gistclaw-install.sh --version v0.1.0 --config-file /path/to/gistclaw-config.yaml
```

That mode also supports public-domain setup:

```bash
sudo ./gistclaw-install.sh --version v0.1.0 --config-file /path/to/gistclaw-config.yaml --public-domain your.domain.example
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

## Bootstrap browser access

Before you expose a public domain, set the built-in operator password locally on the VPS:

```bash
sudo gistclaw auth set-password --config /etc/gistclaw/config.yaml
```

Keep the web host bound to loopback in `/etc/gistclaw/config.yaml`:

```yaml
web:
  listen_addr: 127.0.0.1:8080
```

Do not bind GistClaw directly to `0.0.0.0`. The supported public path is HTTPS reverse proxying into loopback.

## Expose a public domain with Caddy

The installer can manage Caddy for you when you pass `--public-domain your.domain.example`. In that mode it:

- installs `caddy`
- writes `/etc/caddy/Caddyfile`
- enables and restarts the `caddy` service
- keeps GistClaw behind the reverse proxy on loopback

Open ports `80` and `443` to Caddy. Do not open `8080` publicly.

Verify the public path:

```bash
gistclaw security audit --config /etc/gistclaw/config.yaml
systemctl status caddy
curl -I http://127.0.0.1:8080/login
curl -I https://your.domain.example/login
```

Then open `https://your.domain.example/login` in the browser and sign in with the password you set through `gistclaw auth set-password`.

## Update cleanly

Back up first, then rerun the installer for the next version:

```bash
sudo gistclaw backup --db /var/lib/gistclaw/runtime.db
sudo ./gistclaw-install.sh --version v0.1.0 --provider-name openai --provider-api-key YOUR_REAL_KEY
systemctl status gistclaw
```

For VPS re-installs and upgrades, prefer the exact config file path so comments, extra blocks, and custom provider settings survive unchanged:

```bash
sudo gistclaw backup --db /var/lib/gistclaw/runtime.db
sudo ./gistclaw-install.sh --version v0.1.0 --config-file /path/to/gistclaw-config.yaml
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
