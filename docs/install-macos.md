# Install On macOS Apple Silicon

The recommended path is Homebrew through the owned tap. It installs the single
embedded binary, writes a starter config if one does not exist yet, and
supports `brew services`.

## Install with Homebrew

```bash
brew install canhta/gistclaw/gistclaw
gistclaw version
```

The first install bootstraps:

- `/opt/homebrew/etc/gistclaw/config.yaml`
- `/opt/homebrew/var/gistclaw`

The config file is created only if it is missing, so upgrades keep your edits.

## Edit the starter config

Open `/opt/homebrew/etc/gistclaw/config.yaml` and replace the placeholder
provider key before starting the service:

```yaml
storage_root: /opt/homebrew/var/gistclaw
state_dir: /opt/homebrew/var/gistclaw
database_path: /opt/homebrew/var/gistclaw/runtime.db
provider:
  name: openai
  api_key: REPLACE_WITH_REAL_KEY
web:
  listen_addr: 127.0.0.1:8080
```

## Start and stop the service

```bash
brew services start gistclaw
brew services stop gistclaw
```

Then open `http://127.0.0.1:8080`.

## Fallback: raw GitHub release archive

If you do not want Homebrew, download the self-contained `darwin_arm64` archive
from [GitHub Releases](https://github.com/canhta/gistclaw/releases) and unpack
it:

```bash
tar -xzf gistclaw_v0.1.0_darwin_arm64.tar.gz
chmod +x gistclaw
./gistclaw version
./gistclaw --config ~/.config/gistclaw/config.yaml serve
```

For the raw archive path, create `~/.config/gistclaw/config.yaml` first with the
same fields shown above.
