# Use one authoritative state file without automatic migration

Status: accepted for v6.0.0

`<SSHM_HOME>/sshm.yaml` is the only authoritative mutable state file. It contains defaults, hosts, tags, managed-key metadata, host trust, and the encrypted vault. Normal updates use locking and atomic replacement.

v6 does not automatically read or migrate legacy configuration. `sshm init --force` is the only operation that creates a configuration backup before explicit replacement.

Deploy files are strict read-only schema v2 inputs. Without `--file`, sshm loads the user-level `deploy.yaml` and sorted `deploy.d/*.yaml`; with `--file`, it loads only the named files. Current-directory Deploy files are never discovered implicitly.
