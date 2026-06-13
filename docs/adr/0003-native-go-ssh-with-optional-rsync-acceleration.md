# Native Go SSH with optional rsync acceleration

SSH connection, authentication, host trust, remote execution, and forwarding are implemented natively by sshm in Go. File transfer may use rsync as an optional acceleration path when available and applicable, but must preserve the same host-verification and credential-safety guarantees and must always have a native fallback; arbitrary OpenSSH arguments and external SSH compatibility fallbacks remain unsupported.
