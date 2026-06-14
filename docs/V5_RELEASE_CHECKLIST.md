# v5 Release Checklist

## Deploy

- Multi-file discovery and repeated explicit `-f` loading work as documented.
- Duplicate profiles, invalid variables, invalid selectors, invalid strategies, and invalid steps fail before execution.
- Plan displays resolved hosts, final paths, commands, steps, and strategy.
- Hidden mode runs hosts concurrently without interleaving terminal output.
- Visible mode streams output serially.
- Ctrl+C preserves partial results and deploy logs.
- JSON stdout contains valid JSON only.

## Regression

```bash
go test ./...
go test -race ./...
go vet ./...
go test -cover ./...
```

Verify all six release targets build and `go install github.com/fanhuadesenlinnn/sshm/v5@latest` installs the release.
