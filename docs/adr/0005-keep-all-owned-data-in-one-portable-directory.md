# Keep all owned data in one portable directory

Status: accepted for v6.0.0

All mutable state owned by sshm lives under one portable data directory. The default is `~/.sshm`; `SSHM_HOME` is the only supported path override.

The directory contains the authoritative `sshm.yaml`, logs, backups, temporary data, and optional user-level Deploy files. Project Deploy files may live elsewhere, but they are loaded only through explicit `--file` arguments and are never treated as sshm-owned mutable state.

sshm does not support `SSHM_CONFIG_FILE` and does not read, migrate, or delete the legacy `~/.config/sshm` directory.
