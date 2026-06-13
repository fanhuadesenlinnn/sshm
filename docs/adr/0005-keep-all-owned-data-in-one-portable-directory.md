# Keep all owned data in one portable directory

All configuration, personal state, managed-key metadata, passwords, and managed private keys owned by sshm live in one portable data directory, with sensitive values encrypted. sshm will not provide cloud sync, backup archives, or restore workflows; users may copy or synchronize the complete directory with tools they choose, so features must avoid hidden required state outside that directory.
