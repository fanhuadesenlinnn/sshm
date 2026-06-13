# Use one authoritative config without backups or migration

sshm v4 owns one authoritative `sshm.yaml` containing public configuration, managed host trust, managed-key metadata, and the encrypted vault. Updates use locking and atomic replacement without generating `.bak` files. v4 intentionally does not read or migrate the older split-file configuration, because a clean single-document model is preferred over carrying ambiguous synchronization and recovery behavior into the new major version.
