# Import file keys into the managed vault

sshm supports ordinary private-key files only as an import source. After explicit user approval, the key is converted into a master-password-protected managed key and future connections use the decrypted in-memory key rather than depending on the original file path, preserving a single understandable credential model while supporting migration from existing OpenSSH setups.
