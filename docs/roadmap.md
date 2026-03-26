# Roadmap

This roadmap assumes the current shipped surface described in [docs/system.md](system.md). It only covers what should change next.

## Near-Term Priorities

1. extend external surfaces beyond the current Telegram and WhatsApp coverage without breaking the session kernel
2. deepen the operator control plane around routing, recovery, session collaboration, and schedule visibility
3. improve packaging, deployment guidance, and extension workflows around the shipped runtime surface
4. keep the new hardening surfaces stable: security audit, connector supervision, team profiles, and storage health

## Explicit Non-Goals Right Now

These remain out of scope for the current implementation slice:

- rebuilding the full OpenClaw channel matrix
- adding a plugin marketplace or installation UX
- broad automation expansion
- weakening the journal-first runtime contract to move faster at the surface

## Remaining Gap To Broader Assistant-Platform Behavior

- the broader channel and gateway matrix is still intentionally narrow
- teams are now operator-selectable per project, but higher-level sharing and installation workflows still do not exist
- extension seams exist, but higher-level installation and sharing workflows do not
- the control plane is strong locally, but still not the full platform surface the vision describes

## Next Slice

The next slice should make the current system more operator-complete without reopening platform sprawl:

1. deepen route and delivery recovery without bypassing the journaled runtime
2. keep the session model central as more external surfaces are added
3. add explicit operator maintenance and packaging workflows instead of breadth-first platform features
