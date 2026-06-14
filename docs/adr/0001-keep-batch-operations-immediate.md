# Keep batch operations immediate and Deploy lightweight

Status: accepted for v6.0.0

sshm executes batch commands and Deploy profiles only while the invoking process is active. It does not provide a background scheduler, server, desired-state convergence, dependency graph, roles, facts, loops, or an Ansible-compatible language.

Deploy v2 may use ordered actions, simple rc conditions, notify/handlers, check/diff, and become. These features make common personal operations safer without changing sshm into a general automation platform.
