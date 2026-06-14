# Preserve transfer consistency and checksum semantics

Status: accepted for v6.0.0

SFTP is the authoritative transfer implementation. Files use SHA-256 by default; directories use sorted per-entry manifests containing relative path, type, mode, and file checksum.

Transfers reject symlinks and special files, write a temporary target, validate it, and rename it into place. Existing different targets require explicit overwrite or backup. Multi-host pull validates every final path and conflict before downloading.

rsync is only an acceleration path. It must use the same preflight, manifest, checksum, backup, temporary target, activation, and result rules. Auto mode falls back to SFTP when rsync cannot guarantee them; explicit rsync fails.
