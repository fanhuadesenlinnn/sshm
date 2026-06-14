# Support explicit local deployment workflows

sshm supports immediate batch operations and explicit local deployment workflows composed of sequential copy and exec steps. Deploy workflows are loaded from user-maintained YAML files, always resolve to concrete sshm-managed hosts, show an execution plan, require confirmation by default, and run only while the invoking process is active.

sshm remains a lightweight personal operations tool. It will not provide background scheduling, a server, desired-state convergence, dependency graphs, roles, handlers, facts, conditions, loops, or an Ansible-compatible language.
