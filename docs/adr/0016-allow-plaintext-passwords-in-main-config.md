# Allow plaintext passwords in the main configuration alongside the vault

Status: accepted

Host passwords may be written directly as a plaintext `password` field in
`sshm.yaml`, coexisting with the encrypted vault (`password_ref`). The vault
remains the recommended storage; plaintext is an explicit, documented option
for hand-edited batch configuration.

## Context

Passwords are encrypted in the vault and can only be written interactively via
`sshm passwd`. Users who edit `sshm.yaml` directly to batch-add hosts cannot
express passwords in the file, so every host must be added first and then
updated one by one through an interactive prompt. The product principle
"configuration files never hold credentials" made this flow awkward for
scripted and hand-written setups.

## Decision

- A host may set exactly one of `password` (plaintext) or `password_ref`
  (vault); both together is a validation error.
- Plaintext passwords are first-class: no global switch is required. `sshm
  doctor` reports how many hosts use them as an advisory, and the 0600 file
  permission is the primary protection.
- Authentication resolves through a single path: `password` first, then
  `password_ref` via the vault. Plaintext hosts need no vault at all.
- `sshm passwd <host>` upgrades a plaintext host by encrypting the current
  value into the vault and clearing the `password` field.
- Deploy orchestration files still prohibit secrets; this relaxation applies
  only to the main configuration.

## Consequences

Anyone who can read `sshm.yaml` can read plaintext passwords; the 0600
permission and `doctor` advisory mitigate but do not remove this risk. Backups
created by `sshm init --force` may contain plaintext and are kept at the same
permission level. The vault path, managed keys, host trust, and log redaction
are unchanged.
