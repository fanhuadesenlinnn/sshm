# sshmd AI flow test plan

This document lists user-facing sshmd flows as executable test scripts for an AI
tester. Each flow states the purpose, setup, steps, and expected result.

Use a disposable SSHMD_HOME for every test run. Do not run these flows against a
real personal `~/.sshmd` directory.

Recommended common setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Optional remote setup:

- `web01`: reachable SSH host with either password or managed-key access.
- `web02`: second reachable SSH host for batch and tag flows.
- `prod` tag can point to one or both hosts.
- Remote test paths should be under a disposable directory such as `/tmp/sshmd-ai-test`.
- For non-interactive password-vault tests, set `SSHMD_MASTER_PASSWORD` in the
  command environment after creating the temporary vault. Do not export a real
  personal master password into a shared shell history or CI log.

Use `go run .` in development, or replace it with `sshmd` when testing an
installed binary.

## Result Rules

- PASS means the command exits successfully and the expected state/output is
  observed.
- FAIL means the command exits unexpectedly, corrupts config, writes outside the
  expected path, skips a required confirmation error, or produces unclear output.
- INTERACTIVE means the flow requires terminal input and should be tested by an
  AI/browser/terminal agent that can type responses.
- REMOTE means the flow requires a reachable SSH server.

## Local Config And Help Flows

### F001: Show first-run guidance before init

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
```

Steps:

```bash
go run .
```

Expected:

- Output says sshmd is not initialized.
- Output suggests `sshmd init`.
- Command does not create an invalid config.

### F002: Initialize a fresh workspace

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
```

Steps:

```bash
go run . init
test -f "$SSHMD_HOME/sshmd.yaml"
test -f "$SSHMD_HOME/deploy.yaml"
go run . deploy validate
go run . deploy list
test -d "$SSHMD_HOME/deploy.d"
test -d "$SSHMD_HOME/logs"
test -d "$SSHMD_HOME/backups"
test -d "$SSHMD_HOME/tmp"
```

Expected:

- Main and Deploy config files exist.
- The safe empty Deploy template validates and lists successfully.
- Owned directories exist.
- Re-running `go run . init` without force does not damage the config.

### F003: Force init creates a backup

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . init --force
find "$SSHMD_HOME/backups" -type f
```

Expected:

- Reinitialization succeeds.
- A backup file appears in `$SSHMD_HOME/backups`.

### F004: Print config path

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . config path
```

Expected:

- Output contains `$SSHMD_HOME/sshmd.yaml`.

### F005: Run doctor

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . doctor
```

Expected:

- Command exits successfully.
- Output reports config/environment status.

### F006: Show help and version

Type: local

Setup:

None.

Steps:

```bash
go run . --help
go run . --version
go run . host help
go run . key help
go run . tag help
go run . deploy help
go run . logs help
```

Expected:

- Each command exits successfully.
- Help output lists relevant commands.

### F007: Generate shell completion

Type: local

Setup:

None.

Steps:

```bash
go run . completion bash >/tmp/sshmd.bash
go run . completion zsh >/tmp/sshmd.zsh
go run . completion fish >/tmp/sshmd.fish
```

Expected:

- Each file is non-empty.
- Completion generation does not require `sshmd init`.

## Host Management Flows

### F010: Add one host

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . add web01 root@127.0.0.1:2222 --tags prod,web --note test
go run . list
go run . show web01
```

Expected:

- `web01` appears in list output.
- `show web01` displays user, host, port, tags, and note.

### F011: Add host through host subcommand

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . host add web01 root@127.0.0.1:2222
go run . host list
```

Expected:

- Host is added.
- `host list` shows the same host.

### F012: Batch add hosts

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . add-batch web01=root@127.0.0.1:2222 web02=deploy@127.0.0.1:2223
go run . list
```

Expected:

- Both hosts are present.
- Duplicate aliases are rejected if repeated.

### F013: Reject invalid host definitions

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . add 123 root@127.0.0.1:22
go run . add bad root@127.0.0.1:99999
go run . add web01 root@127.0.0.1:22 --auth invalid
```

Expected:

- Each command fails with a validation message.
- No invalid host is saved.

### F014: Search, show, compact list, wide list

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222 --tags prod,web --note nginx
go run . add db01 root@127.0.0.1:2223 --tags prod,db --note postgres
```

Steps:

```bash
go run . list --compact
go run . list --wide
go run . search web
go run . search postgres
go run . show 1
```

Expected:

- List variants render without error.
- Search returns matching hosts.
- Numeric ID lookup works.

### F015: Pin, unpin, and recent

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
```

Steps:

```bash
go run . pin web01
go run . recent
go run . unpin web01
go run . recent 5
```

Expected:

- Pinned host appears in recent/favorites output.
- Unpinned host no longer appears as pinned.

### F016: Delete host

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
```

Steps:

```bash
go run . delete web01 --yes
go run . list
```

Expected:

- Host is removed.
- Deleting a missing host returns a clear error.

### F017: Interactive host edit

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
```

Steps:

```bash
go run . edit web01
```

During prompts:

- Change alias to `web01-renamed`.
- Keep user and host.
- Change port to `2224`.
- Add note and tags.
- Decline password saving.

Expected:

- Host is saved as `web01-renamed`.
- Old alias no longer resolves.
- New port and tags appear in `show`.

### F018: Config edit rejects invalid YAML

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . config edit
```

During editor session:

- Break the YAML or set an invalid `version`.
- Save and exit.

Expected:

- Command rejects the edited config.
- Original config remains usable.

## Tag Flows

### F030: Create, list, show, edit tag

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
```

Steps:

```bash
go run . tag create prod --note production
go run . tag list
go run . tag show prod
go run . tag edit prod --note production-updated
go run . tag show prod
```

Expected:

- Tag exists.
- Note is displayed and updates correctly.

### F031: Add and remove tag from selected hosts

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
go run . add web02 root@127.0.0.1:2223
```

Steps:

```bash
go run . tag add prod web01 web02
go run . tag show prod
go run . tag remove prod web02
go run . tag show prod
```

Expected:

- Both hosts receive `prod`.
- `web02` is removed from `prod`.

### F032: Replace and clear host tags

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222 --tags old
```

Steps:

```bash
go run . tag set web01 --tags prod,web
go run . show web01
go run . tag clear web01
go run . tag show --untagged
```

Expected:

- `old` is replaced by `prod` and `web`.
- Tags are cleared.
- Host appears in untagged output.

### F033: Rename and delete tag

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222 --tags prod
```

Steps:

```bash
go run . tag rename prod production
go run . show web01
go run . tag delete production --yes
go run . show web01
```

Expected:

- Host reference changes from `prod` to `production`.
- Deleting the tag removes host references.

### F034: Selector variants for tag operations

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222 --tags web
go run . add web02 root@127.0.0.1:2223 --tags web
```

Steps:

```bash
go run . tag add prod --tag web
go run . tag show prod
go run . tag clear --all
go run . tag show --untagged
```

Expected:

- `--tag web` selects both hosts.
- `--all` clears both hosts.

## Credential And Managed-Key Flows

### F050: Save and forget SSH password

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
```

Steps:

```bash
go run . passwd web01
go run . show web01
go run . forget-pass web01 --yes
go run . show web01
```

During prompts:

- Create an sshmd master password if asked.
- Enter matching SSH password twice.

Expected:

- Password is saved in encrypted vault.
- Host gets `password_ref`.
- Forget removes `password_ref`.

### F051: Lock current vault session

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
go run . passwd web01
```

Steps:

```bash
go run . lock
```

Expected:

- Command reports current session vault locked.
- Next vault operation asks for master password again.

### F052: Create managed key and list it

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . key create personal --default
go run . key list
go run . key default
```

During prompts:

- Create an sshmd master password if asked.

Expected:

- Key is stored.
- `personal` is marked as default.
- Public fingerprint is shown in list output.

### F053: Create managed keys in batch

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . key create-batch personal deploy
go run . key list
```

Expected:

- Both keys are created.
- Duplicate key creation is rejected.

### F054: Import an existing private key

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
ssh-keygen -t ed25519 -N "" -f /tmp/sshmd-ai-id-ed25519
```

Steps:

```bash
go run . key import imported /tmp/sshmd-ai-id-ed25519 --default
go run . key show imported
```

Expected:

- Key is imported.
- Public key is printed.

### F055: Bind managed key to host and show pubkey

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
go run . key create personal --default
```

Steps:

```bash
go run . key use personal web01
go run . show web01
go run . show-pubkey web01
```

Expected:

- Host identity becomes `managed:personal`.
- Public key is printed.

### F056: Managed key status and delete

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
go run . key create personal --default
go run . key use personal web01
go run . key create unused
```

Steps:

```bash
go run . key status
go run . key delete-unused --yes
go run . key list
go run . key delete personal --yes
```

Expected:

- Status shows host/key binding.
- Unused key is deleted.
- Deleting a used key should fail or require bindings to be removed first.

### F057: Change host auth strategy

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
```

Steps:

```bash
go run . auth web01
```

During prompts:

- Enter `password`, then verify with `show web01`.
- Repeat and enter `auto`.

Expected:

- Auth strategy changes only to valid values: `auto`, `key`, or `password`.

## SSH Connection And Remote Execution Flows

### F070: Ping one host

Type: remote

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 "$SSHMD_TEST_USER@$SSHMD_TEST_HOST:$SSHMD_TEST_PORT"
```

Also configure password or managed-key access for `web01`.

Steps:

```bash
go run . ping web01
```

Expected:

- SSH connection succeeds.
- Output contains success status.

### F071: Ping all hosts

Type: remote

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 "$SSHMD_TEST_USER@$SSHMD_TEST_HOST:$SSHMD_TEST_PORT"
go run . add web02 "$SSHMD_TEST_USER@$SSHMD_TEST_HOST2:$SSHMD_TEST_PORT2"
```

Also configure credentials.

Steps:

```bash
go run . ping --yes
go run . ping --yes --quiet
```

Expected:

- Batch summary is printed.
- Quiet mode reduces per-host output.

### F072: Connect by alias and direct root shortcut

Type: remote, interactive

Setup:

Use a reachable configured host `web01`.

Steps:

```bash
go run . connect web01
go run . web01
```

Expected:

- Both commands open an interactive SSH session.
- Exiting remote shell returns to local terminal.

### F073: Execute command on one host

Type: remote

Setup:

Use a reachable configured host `web01`.

Steps:

```bash
go run . exec --yes web01 "echo sshmd-ok"
go run . exec --yes --quiet --no-log web01 "hostname"
```

Expected:

- First command prints `sshmd-ok`.
- `--quiet --no-log` succeeds without writing a new operation log.

### F074: Execute command by tag

Type: remote

Setup:

Configure reachable hosts tagged `prod`.

Steps:

```bash
go run . exec-tag prod "echo sshmd-prod" --yes
go run . exec-tag all "hostname" --parallel 2 --serial 1 --yes
```

Expected:

- Commands run on selected hosts.
- Batch summary matches number of hosts.

### F075: Batch failure controls

Type: remote

Setup:

Configure at least two hosts, with one unreachable or command designed to fail.

Steps:

```bash
go run . exec-tag all "false" --fail-fast --yes
go run . exec-tag all "false" --max-fail 1 --yes
go run . exec-tag all "false" --max-fail-percent 50 --yes
```

Expected:

- Some hosts are skipped after failure policy triggers.
- Exit code indicates failure.

### F076: Host trust policies

Type: remote, interactive

Setup:

Use a new SSH server whose host key is not yet trusted.

Steps:

```bash
go run . add web01 "$SSHMD_TEST_USER@$SSHMD_TEST_HOST:$SSHMD_TEST_PORT" --host-key-policy strict
go run . ping web01
go run . add web02 "$SSHMD_TEST_USER@$SSHMD_TEST_HOST:$SSHMD_TEST_PORT" --host-key-policy accept-new
go run . ping web02
```

Expected:

- `strict` prompts for first host key in a terminal.
- `accept-new` trusts new key without prompt.
- A changed key is rejected.

## File Transfer Flows

### F090: Push a file with SFTP

Type: remote

Setup:

Use a reachable configured host `web01`.

Steps:

```bash
printf 'hello\n' >/tmp/sshmd-ai-file.txt
go run . push web01 /tmp/sshmd-ai-file.txt /tmp/sshmd-ai-test/file.txt --method sftp --yes
go run . exec --yes web01 "cat /tmp/sshmd-ai-test/file.txt"
```

Expected:

- Remote file exists.
- Remote content is `hello`.

### F091: Push refuses overwrite without explicit flag

Type: remote

Setup:

Complete F090 first.

Steps:

```bash
printf 'changed\n' >/tmp/sshmd-ai-file.txt
go run . push web01 /tmp/sshmd-ai-file.txt /tmp/sshmd-ai-test/file.txt --method sftp --yes
go run . push web01 /tmp/sshmd-ai-file.txt /tmp/sshmd-ai-test/file.txt --method sftp --overwrite --yes
```

Expected:

- First push fails because remote target differs.
- Second push succeeds.

### F092: Push with backup

Type: remote

Setup:

Remote file exists at `/tmp/sshmd-ai-test/file.txt`.

Steps:

```bash
printf 'backup-change\n' >/tmp/sshmd-ai-file.txt
go run . push web01 /tmp/sshmd-ai-file.txt /tmp/sshmd-ai-test/file.txt --backup --yes
go run . exec --yes web01 "ls /tmp/sshmd-ai-test/file.txt.bak.*"
```

Expected:

- Push succeeds.
- Backup file exists.

### F093: Pull a file

Type: remote

Setup:

Remote file exists at `/tmp/sshmd-ai-test/file.txt`.

Steps:

```bash
rm -rf /tmp/sshmd-ai-pull
go run . pull web01 /tmp/sshmd-ai-test/file.txt /tmp/sshmd-ai-pull/file.txt --yes
cat /tmp/sshmd-ai-pull/file.txt
```

Expected:

- Local file exists.
- Content matches remote file.

### F094: Push and pull a directory

Type: remote

Setup:

Use reachable host `web01`.

Steps:

```bash
rm -rf /tmp/sshmd-ai-dir /tmp/sshmd-ai-dir-pull
mkdir -p /tmp/sshmd-ai-dir/nested
printf 'a\n' >/tmp/sshmd-ai-dir/a.txt
printf 'b\n' >/tmp/sshmd-ai-dir/nested/b.txt
go run . push web01 /tmp/sshmd-ai-dir /tmp/sshmd-ai-test/dir --yes
go run . pull web01 /tmp/sshmd-ai-test/dir /tmp/sshmd-ai-dir-pull --yes
find /tmp/sshmd-ai-dir-pull -type f | sort
```

Expected:

- Directory tree is preserved.
- File contents match.

### F095: Reject unsafe remote paths

Type: remote

Setup:

Use reachable host `web01`.

Steps:

```bash
printf 'x\n' >/tmp/sshmd-ai-file.txt
go run . push web01 /tmp/sshmd-ai-file.txt "~/.bad" --yes
go run . push web01 /tmp/sshmd-ai-file.txt "../bad" --yes
go run . pull web01 "/" /tmp/sshmd-ai-root --yes
```

Expected:

- Each command fails before changing files.

### F096: Pull by tag with host directories

Type: remote

Setup:

Configure two reachable hosts tagged `prod`, each with `/tmp/sshmd-ai-test/file.txt`.

Steps:

```bash
rm -rf /tmp/sshmd-ai-batch-pull
go run . pull-tag prod /tmp/sshmd-ai-test/file.txt /tmp/sshmd-ai-batch-pull --yes
find /tmp/sshmd-ai-batch-pull -type f | sort
```

Expected:

- Files are saved under per-host directories.
- No host overwrites another host.

### F097: Pull by tag with --flat collision handling

Type: remote

Setup:

Configure two reachable hosts tagged `prod`.

Steps:

```bash
rm -rf /tmp/sshmd-ai-flat
go run . pull-tag prod /tmp/sshmd-ai-test/file.txt /tmp/sshmd-ai-flat --flat --yes
```

Expected:

- If destinations collide, command fails before transfer.
- If only one host is selected, flat pull succeeds.

### F098: rsync method

Type: remote

Setup:

- Use managed-key authentication for `web01`.
- Local and remote `rsync` must be installed.
- Host cannot use a jump host.

Steps:

```bash
printf 'rsync\n' >/tmp/sshmd-ai-rsync.txt
go run . push web01 /tmp/sshmd-ai-rsync.txt /tmp/sshmd-ai-test/rsync.txt --method rsync --yes
go run . pull web01 /tmp/sshmd-ai-test/rsync.txt /tmp/sshmd-ai-rsync-pull.txt --method rsync --yes
```

Expected:

- Explicit rsync succeeds when all safety requirements are met.
- If requirements are not met, explicit rsync fails clearly.
- `--method auto` falls back to SFTP when rsync is unavailable.

## Deploy Flows

### F110: Inspect initialized Deploy template and sample

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
test -f "$SSHMD_HOME/deploy.yaml"
go run . deploy validate
go run . deploy list
go run . deploy init --stdout >/tmp/sshmd-deploy-sample.yaml
```

Expected:

- Global init creates a valid Deploy file with no active plays.
- `--stdout` prints sample YAML without writing.

### F111: Deploy init refuses overwrite by default

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . deploy init
go run . deploy init --overwrite
```

Expected:

- First command fails because deploy file exists.
- Second command overwrites successfully.

### F112: Validate and list deploy plays

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222 --tags prod
go run . deploy init --overwrite
```

Steps:

```bash
go run . deploy validate
go run . deploy validate --output json
go run . deploy list
go run . deploy list --output json
```

Expected:

- Validation succeeds.
- Text and JSON output modes work.

### F113: Deploy plan and show

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222 --tags prod
cat > "$SSHMD_HOME/deploy.yaml" <<'EOF'
version: 3
plays:
  - name: update-app
    hosts:
      tags: [prod]
    tasks:
      - name: probe
        command:
          cmd: hostname
EOF
```

Steps:

```bash
go run . deploy plan update-app
go run . deploy show update-app
go run . deploy plan update-app --host web01 --output json
go run . deploy plan update-app --tag prod
```

Expected:

- Plans are generated without connecting to remote hosts.
- Target overrides work.

### F114: Deploy run in check mode

Type: remote

Setup:

Write an active `update-app` playbook as in F113, with at least one reachable
host tagged `prod`.

Steps:

```bash
go run . deploy run update-app --check --yes
```

Expected:

- Run completes without making final changes.
- Changed operations are reported as `would-change`.

### F115: Deploy run with diff

Type: remote

Setup:

Use a reachable host tagged `prod` and a text `template` action:

```bash
mkdir -p "$SSHMD_HOME/templates"
printf 'listen {{ port }};\n' > "$SSHMD_HOME/templates/app.conf.tmpl"
cat > "$SSHMD_HOME/deploy.yaml" <<'EOF'
version: 3
plays:
  - name: update-app
    hosts:
      tags: [prod]
    vars:
      port: 8080
    tasks:
      - name: render config
        template:
          src: ./templates/app.conf.tmpl
          dest: /tmp/sshmd-ai-app.conf
EOF
```

Steps:

```bash
go run . deploy run update-app --diff --yes
```

Expected:

- Diff output is visible for text changes.
- Logs do not leak diff unexpectedly beyond intended operation output.

### F116: Deploy run with batch options

Type: remote

Setup:

Use at least two reachable hosts tagged `prod` with the F113 playbook.

Steps:

```bash
go run . deploy run update-app --serial 1 --parallel 2 --max-fail 1 --yes
```

Expected:

- Hosts run according to serial/parallel settings.
- Failure policy is honored.

### F117: Deploy with explicit files

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222 --tags prod
cat > /tmp/sshmd-ai-deploy.yaml <<'EOF'
version: 3
plays:
  - name: update-app
    hosts:
      tags: [prod]
    tasks:
      - name: probe
        command:
          cmd: hostname
EOF
```

Steps:

```bash
go run . deploy validate -f /tmp/sshmd-ai-deploy.yaml
go run . deploy list -f /tmp/sshmd-ai-deploy.yaml
go run . deploy plan update-app -f /tmp/sshmd-ai-deploy.yaml
```

Expected:

- Explicit file is loaded.
- Current directory `sshmd.deploy.yaml` is not loaded unless passed with `-f`.

### F118: Deploy rejects removed v2 files and unknown modules

Type: local

Setup:

Create a removed v2 file and a playbook with an unknown module:

```bash
printf 'version: 2\nprofiles: []\n' > /tmp/sshmd-v2-deploy.yaml
printf 'version: 3\nplays:\n  - name: bad\n    hosts:\n      tags: [prod]\n    tasks:\n      - name: x\n        no_such_module: {}\n' > /tmp/sshmd-unknown-module.yaml
```

Steps:

```bash
go run . deploy validate -f /tmp/sshmd-v2-deploy.yaml
go run . deploy validate -f /tmp/sshmd-unknown-module.yaml
```

Expected:

- The v2 file is rejected with a "Deploy v2" removal message.
- The unknown module is rejected during validation.

## SSH Config Import And Export Flows

### F130: Export OpenSSH config

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
```

Steps:

```bash
go run . export-ssh-config /tmp/sshmd-ai-ssh-config
cat /tmp/sshmd-ai-ssh-config
go run . export-ssh-config /tmp/sshmd-ai-ssh-config
go run . export-ssh-config --force /tmp/sshmd-ai-ssh-config
```

Expected:

- File is written.
- Output contains `Host web01`, `HostName`, `User`, and `Port`.
- The second export refuses to overwrite the existing file; the explicit `--force` export succeeds.

### F131: Import OpenSSH config

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
cat >/tmp/sshmd-ai-import-config <<'EOF'
Host imported
    HostName 127.0.0.1
    User root
    Port 2222
EOF
```

Steps:

```bash
go run . import-ssh-config /tmp/sshmd-ai-import-config
go run . show imported
```

Expected:

- Host is imported.
- Re-import skips duplicate alias.

### F132: Import keeps host metadata and gives IdentityFile follow-up

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
cat >/tmp/sshmd-ai-import-key-config <<'EOF'
Host keyhost
    HostName 127.0.0.1
    User root
    IdentityFile ~/.ssh/id_ed25519
EOF
```

Steps:

```bash
go run . import-ssh-config /tmp/sshmd-ai-import-key-config
go run . show keyhost
```

Expected:

- Host metadata is imported and its `identity` remains empty.
- Output gives directly executable `sshmd key import` and `sshmd key use` follow-up commands.

### F133: Export and import preserve a single-level ProxyJump

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add bastion root@127.0.0.1:2222
go run . add inner deploy@10.0.0.11:22 --jump-host bastion
```

Steps:

```bash
go run . export-ssh-config /tmp/sshmd-ai-jump-config
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . import-ssh-config /tmp/sshmd-ai-jump-config
go run . show inner
```

Expected:

- `inner` still uses `bastion` as its jump host after the round trip.
- Unsupported OpenSSH multi-hop or `user@host:port` ProxyJump forms are imported with a warning and the unsupported value cleared instead of being silently discarded.

### F134: Import applies OpenSSH wildcard defaults

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
cat >/tmp/sshmd-ai-default-config <<'EOF'
Host imported
    HostName 127.0.0.1
Host *
    User deploy
    Port 2222
EOF
```

Steps:

```bash
go run . import-ssh-config /tmp/sshmd-ai-default-config
go run . show imported
```

Expected:

- The literal alias is imported with `User deploy` and `Port 2222` from `Host *`.
- The wildcard itself is not imported as a host.

## Logs Flows

### F140: Logs are written for operations

Type: remote

Setup:

Use reachable host `web01`.

Steps:

```bash
go run . exec --yes web01 "echo log-test"
go run . logs
find "$SSHMD_HOME/logs" -type f
```

Expected:

- Log directory is listed.
- Per-host log and summary exist.

### F141: No-log prevents operation log

Type: remote

Setup:

Use reachable host `web01`; record current log count.

Steps:

```bash
before=$(find "$SSHMD_HOME/logs" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l)
go run . exec --yes --no-log web01 "echo no-log"
after=$(find "$SSHMD_HOME/logs" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l)
test "$before" = "$after"
```

Expected:

- Log directory count does not increase.

### F142: Clean logs

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
mkdir -p "$SSHMD_HOME/logs/manual"
```

Steps:

```bash
go run . logs
go run . logs clean --yes
test ! -e "$SSHMD_HOME/logs"
```

Expected:

- Logs are listed before clean.
- Logs directory is removed.
- Command refuses unsafe root-like SSHMD_HOME values.

## Port Forward Flow

### F150: Local port forwarding

Type: remote, interactive

Setup:

- Use reachable host `web01`.
- Remote host has a service listening on `127.0.0.1:80`, or replace with a known
  remote test service.

Steps:

```bash
go run . forward web01 127.0.0.1:18080 127.0.0.1:80
```

In another terminal:

```bash
curl -v http://127.0.0.1:18080/
```

Expected:

- Forward command reports that forwarding started.
- Local curl reaches remote service.
- Ctrl+C stops forwarding cleanly.

## Interactive Centers

### F160: Root interactive mode

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run .
```

During prompts:

- Enter `help`.
- Enter `list`.
- Enter `q`.

Expected:

- Help appears.
- Commands execute inside interactive mode.
- `q` exits.

### F161: Host center

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . host
```

During prompts:

- Enter `add web01 root@127.0.0.1:2222`.
- Enter `list`.
- Enter `back`.

Expected:

- Host center accepts short commands.
- `back` returns/exits center.

### F162: Key center

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . key
```

During prompts:

- Enter `list`.
- Enter `back`.

Expected:

- Key center renders help/list.
- `back` exits center.

### F163: Tag center

Type: interactive

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . tag
```

During prompts:

- Enter `create prod --note production`.
- Enter `list`.
- Enter `back`.

Expected:

- Tag center accepts subcommands.
- Tag persists after exit.

## Legacy Compatibility Flows

### F180: Legacy root options

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
```

Steps:

```bash
go run . --list
go run . -l
go run . --show web01
go run . --search web
go run . --tag list
```

Expected:

- Legacy options dispatch to the same behavior as modern commands.

### F181: Unknown command suggestions

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
```

Steps:

```bash
go run . lst
go run . --lis
```

Expected:

- Error suggests the closest valid command or option.

## Negative And Safety Flows

### F200: Commands requiring config fail before init

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
```

Steps:

```bash
go run . list
go run . add web01 root@127.0.0.1
go run . deploy list
```

Expected:

- Commands fail with not-initialized guidance.
- `doctor`, `init`, `config path`, and `completion` remain available as designed.

### F201: Non-interactive confirmation guard

Type: local/remote depending on command

Setup:

Use a command that normally requires confirmation, such as deleting a tagged host
or running a batch operation.

Steps:

```bash
go run . delete web01
go run . exec-tag all "hostname"
go run . logs clean
```

Expected:

- In non-terminal execution, commands that need confirmation fail and instruct
  the user to pass `--yes`.

### F202: Config rejects broken references

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
go run . init
go run . add web01 root@127.0.0.1:2222
```

Steps:

- Edit `$SSHMD_HOME/sshmd.yaml` manually and set `identity: managed:missing`.
- Run `go run . list`.
- Restore config, then set `jump_host: missing`.
- Run `go run . list`.

Expected:

- Broken managed-key reference is rejected.
- Broken jump-host reference is rejected.
- Error explains the broken reference.

### F203: No accidental use of old config paths

Type: local

Setup:

```bash
export SSHMD_HOME="$(mktemp -d)"
export SSHMD_CONFIG_FILE="/tmp/should-not-be-used.yaml"
go run . init
```

Steps:

```bash
go run . config path
test ! -e /tmp/should-not-be-used.yaml
```

Expected:

- Only `SSHMD_HOME` controls the data directory.
- `SSHMD_CONFIG_FILE` is ignored.
