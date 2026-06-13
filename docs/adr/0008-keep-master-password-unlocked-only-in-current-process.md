# Keep the master password unlocked only in the current process

The password vault is unlocked on demand and remains available only within the current sshm process until `lock` or process exit. sshm will not remember the master password, cache it in a background service, or store it in an operating-system credential manager, keeping the portable data directory and credential model independent from device-specific secret stores.
