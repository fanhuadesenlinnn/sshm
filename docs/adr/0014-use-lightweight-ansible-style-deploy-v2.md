# Use a lightweight Ansible-style Deploy v2 model

Status: accepted for v6.0.0; superseded by ADR 0015 in v6.1.0 (Deploy v2 removed)

Deploy v2 uses a nested action DSL. Each step or handler contains exactly one of `exec`, `push`, `pull`, `mkdir`, `wait`, or `confirm`.

The model supports static plans, check/diff, changed and would-change states, notify/handlers, ignore_error, simple rc conditions, become, and BatchRunner rolling execution. It intentionally excludes roles, facts, loops, complex expressions, desired-state convergence, and Ansible compatibility.

Plans never connect to remote hosts. Check mode may read remote state but cannot modify final local or remote targets. Diff output can contain sensitive content and is not written to logs by default.
