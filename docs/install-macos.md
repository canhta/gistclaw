# Install On macOS Apple Silicon

This path is for local Apple Silicon use, not `launchd` service management.

## Download and unpack

Download the `darwin_arm64` archive from [GitHub Releases](https://github.com/canhta/gistclaw/releases) and unpack it:

```bash
tar -xzf gistclaw_v0.1.0_darwin_arm64.tar.gz
chmod +x gistclaw
```

## Create config

Write `~/.config/gistclaw/config.yaml`:

```yaml
provider:
  name: openai
  api_key: REPLACE_WITH_REAL_KEY
web:
  listen_addr: 127.0.0.1:8080
```

## Run locally

```bash
./gistclaw version
./gistclaw serve
```

Then open `http://127.0.0.1:8080`.
