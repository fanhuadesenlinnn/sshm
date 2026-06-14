# Use strict YAML schema v2 for configuration and Deploy

Status: accepted for v6.0.0

The main configuration and Deploy manifests both use YAML with an explicit top-level `version: 2`. The schemas are independent but share strict behavior: a missing version, unsupported version, or unknown field is an error.

The main configuration keeps all authoritative sshm state. Deploy YAML is declarative input and cannot contain SSH credentials or host trust. Neither schema is automatically migrated from an older version.
