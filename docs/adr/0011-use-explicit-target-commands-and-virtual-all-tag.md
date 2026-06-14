# Use explicit target commands and a virtual all tag

Status: accepted for v6.0.0

Single-host operations use `exec`, `push`, and `pull`. Tag operations use `exec-tag`, `push-tag`, and `pull-tag`. The reserved virtual tag `all` selects every host.

The former `exec-all`, `push-all`, and `pull-all` commands are not registered. This keeps one command shape per operation and makes target scope explicit.
