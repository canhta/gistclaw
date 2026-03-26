# Backup, Restore, And Rollback

This runbook is the day-2 operator contract for the first public release.

## Backup before upgrades

```bash
sudo gistclaw backup --db /var/lib/gistclaw/runtime.db
ls -1 /var/lib/gistclaw/*.db.bak /var/lib/gistclaw/backups 2>/dev/null
```

## Restore

1. Stop the service.
2. Copy a known-good backup over the active database.
3. Start the service again.
4. Verify with doctor and logs.

```bash
sudo systemctl stop gistclaw
sudo cp /var/lib/gistclaw/runtime.20260326-010203.db.bak /var/lib/gistclaw/runtime.db
sudo systemctl start gistclaw
gistclaw doctor --config /etc/gistclaw/config.yaml
journalctl -u gistclaw -n 100 --no-pager
```

## Rollback

Binary rollback is only safe if you also understand the database state. The safe path is:

1. back up first
2. reinstall the older release from the GitHub release URL
3. restore the matching database backup if the new binary changed storage expectations

```bash
curl -fsSLO https://github.com/canhta/gistclaw/releases/download/v0.1.0/gistclaw-install.sh
sudo ./gistclaw-install.sh --version v0.1.0 --provider-name openai --provider-api-key YOUR_REAL_KEY
```

## Smoke checklist after publish

- install from the public GitHub release URL on one Ubuntu 24 VPS
- confirm `systemctl status gistclaw`
- confirm `journalctl -u gistclaw`
- run `gistclaw doctor`
- run `gistclaw security audit`
- rehearse one backup and one restore
