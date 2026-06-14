# Share one BatchRunner

Status: accepted for v6.0.0

`exec-tag`, `push-tag`, `pull-tag`, and `deploy run` share one BatchRunner. It owns stable result ordering, serial batches, parallel limits, failure thresholds, cancellation, skipped reasons, summary counts, and batch exit codes.

BatchRunner does not own operation confirmation, vault unlocking, host trust prompts, or operation-specific execution. These remain at the command and SSH layers.
