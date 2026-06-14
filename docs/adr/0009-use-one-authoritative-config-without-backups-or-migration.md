# Use one authoritative state file plus composable deploy manifests

sshm owns one authoritative mutable state file, `sshm.yaml`, containing hosts, tags, managed host trust, managed-key metadata, and the encrypted vault. Updates use locking and atomic replacement without generating `.bak` files.

Deploy workflows are read-only user inputs loaded from explicit `-f` files or from the documented global, fragment, and project locations. Each file owns its variables and defaults; profiles are combined into one catalog, and duplicate profile names are rejected instead of silently overridden. Deploy files are never merged into or written back to `sshm.yaml`.
