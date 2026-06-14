# Keep owned runtime state portable while allowing project manifests

All mutable runtime state owned by sshm, including hosts, tags, trust, managed-key metadata, encrypted credentials, and operation logs, lives in one portable data directory. User-authored deploy manifests are declarative inputs rather than sshm-owned state and may live in project directories or the data directory.

Deploy manifests never contain SSH credentials, private keys, SSH users, ports, or host trust. Copying the data directory preserves sshm-owned state; project manifests remain portable with their projects.
