# Remove Deploy v2 and migrate to a single v3 engine

Status: accepted for v6.1.0

Deploy v2 (nested action DSL with profiles/steps/handlers) is removed in v6.1.0. sshm runs exactly one deploy engine, the modular v3 playbook model, and no longer loads `version: 2` deploy files.

## Context

Since v6.0.11 the project has shipped two deploy engines. v3 covers the full v2 feature surface with modules and conditionals, and v2 only remained for compatibility. Maintaining two execution paths duplicates become/quoting/condition logic and forces the CLI to detect file versions on every command.

## Decision

- Delete `internal/deploy` and the v2 CLI paths; `deploy` commands are v3-only.
- Remove `deploy migrate`; there is no v2 import path at all.
- Preserve the pieces v3 actually shares: `TargetSelector`, `Condition`, `Discover`, and `ResolveTargets` live in the single `internal/deploy` package.
- Fill the only functional gap left by v2's `wait` with a new v3 `sleep` module, and replace v2's serial-batch `confirm` step with a task-level `confirm` field.
- The main configuration schema stays `version: 2`; only the deploy schema is reduced to `version: 3`.

## Consequences

Users with v2 deploy files must rewrite them as v3 playbooks; loading a v2 file is a hard error with a clear message. Documentation, templates, and tests are reduced to a single engine, and future engine work (for example connection reuse) touches one execution path.
