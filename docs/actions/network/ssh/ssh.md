# SSH action

The SSH action exposes the complete surface offered by [`github.com/helloyi/go-sshclient`](https://pkg.go.dev/github.com/helloyi/go-sshclient)
so FlowK flows can run remote commands, spawn shells, and manipulate remote file systems over SFTP within a single task.  It wraps
all exported operations of the upstream library in a declarative payload that keeps connections, commands, and file transfers fully
automatable.

The action establishes a single SSH session using the connection block and then evaluates every declared step sequentially.  Each
step chooses an operation and feeds the required parameters.  Results are captured as structured JSON, so subsequent `EVALUATE`
or `VARIABLES` tasks can assert on command output, remote file metadata, or directory listings.

## Connection block

```jsonc
{
  "action": "SSH",
  "connection": {
    "network": "tcp",              // optional, defaults to "tcp"
    "address": "cicd.example.com:22",
    "username": "deploy",
    "auth": {
      "method": "private_key",
      "privateKeyPath": "${secrets.deploy_key}",
      "passphrase": "${secrets.deploy_passphrase}"
    },
    "timeoutSeconds": 10,
    "keepAliveSeconds": 30,
    "hostKey": {
      "mode": "known_hosts",
      "knownHostsFiles": ["./certs/hosts"]
    },
    "preferredCiphers": ["aes256-gcm@openssh.com"],
    "clientVersion": "SSH-2.0-FlowK"
  }
}
```

Supported authentication modes are:

| `auth.method` value | Required fields | Description |
|---------------------|-----------------|-------------|
| `password` | `password` | Uses username/password authentication. |
| `private_key` / `private-key` / `keyfile` | `privateKey`, `privateKeyPEM`, or `privateKeyPath` | Loads an unencrypted private key from the inline string or the referenced file. |
| `private_key_with_passphrase` | Same as above plus `passphrase` | Decrypts the private key with the provided passphrase. |

Host-key verification supports two strategies:

- `mode: "insecure"` (default) skips verification, mirroring the helper functions in the upstream package.
- `mode: "known_hosts"` loads entries from `knownHostsFiles` and/or inline host-key lines.  Inline entries are written to a temporary file
  and automatically removed after the session terminates.

## Step types

Every step defines an `operation` key.  The action accepts the following categories:

### Command execution (`RUN_COMMAND*`)

Runs discrete commands via `Client.Cmd`.  Provide a `commands` array; each entry becomes a line in the generated remote script.  The
operation suffix selects how the command is evaluated:

- `RUN_COMMAND` – executes the commands and only reports success or failure.  Set `"stdout": "capture"` and/or `"stderr": "capture"` to
  store the transcript in the `output` property.
- `RUN_COMMAND_OUTPUT` – captures standard output and returns it as a string.
- `RUN_COMMAND_SMART_OUTPUT` – mirrors `RemoteScript.SmartOutput`, returning stdout on success or stderr on failure.

Non-zero exit statuses normally fail the step.  Provide an `allowedExitCodes` array to treat specific codes as success.  When a
command is allowed to fail, any captured stdout/stderr is still returned (e.g. use `RUN_COMMAND` with `"stdout": "capture"` to
read partial output from utilities such as `df`).

```jsonc
{
  "id": "ssh.command.sample",
  "operation": "RUN_COMMAND_SMART_OUTPUT",
  "commands": ["uname -a", "whoami"],
  "stdout": "capture"
}
```

### Raw script execution (`RUN_SCRIPT*`)

Feeds a multi-line shell script to `Client.Script`.  The suffix options mirror the command execution behaviour.

```jsonc
{
  "id": "ssh.script.deploy",
  "operation": "RUN_SCRIPT",
  "script": "#!/bin/bash\nset -euo pipefail\nsystemctl restart app.service",
  "stderr": "capture"
}
```

### Local script execution (`RUN_SCRIPT_FILE*`)

Uses `Client.ScriptFile` to stream a local script file to the remote host.  Paths are resolved relative to the FlowK working
directory, so they can be versioned alongside the flow definition.

```jsonc
{
  "id": "ssh.scriptfile.seed",
  "operation": "RUN_SCRIPT_FILE_OUTPUT",
  "path": "./scripts/seed.sh"
}
```

### Shell sessions (`EXECUTE_SHELL`)

Spawns either an interactive PTY (`requestPty: true`) or a non-interactive shell.  Provide `input` to seed data that the shell should
consume and enable `captureStdout` / `captureStderr` to capture the transcript.

```jsonc
{
  "id": "ssh.shell.login",
  "operation": "EXECUTE_SHELL",
  "requestPty": true,
  "terminal": {
    "term": "xterm-256color",
    "height": 40,
    "width": 120
  },
  "input": "export ENV=prod\nprintenv ENV\nexit\n",
  "captureStdout": true
}
```

### SFTP operations (`SFTP`)

Any step with `operation: "SFTP"` must supply a `method` and a `params` object.  The action lazily initialises a single
`RemoteFileSystem` (respecting the optional `sftp` block at the top level) and then routes the call to the named method.  All
the functions exported by `RemoteFileSystem` are available:

| Method | Required parameters | Behaviour |
|--------|---------------------|-----------|
| `CHMOD` | `path`, `mode` | Updates a file's permissions.  Accepts octal strings like `"0755"` or decimal integers. |
| `CHOWN` | `path`, `uid`, `gid` | Changes ownership. |
| `CHTIMES` | `path`, `atime`, `mtime` (RFC3339 timestamps) | Adjusts access and modification timestamps. |
| `CREATE` | `path`, optional `content` | Creates or truncates a remote file and optionally writes inline content. |
| `DOWNLOAD` | `remotePath`, `localPath` | Copies a remote file to the control host. |
| `GETWD` | – | Returns the working directory path. |
| `GLOB` | `pattern` | Returns the list of matches. |
| `LINK` | `old`, `new` | Creates a hard link. |
| `LSTAT` | `path` | Returns metadata without following symlinks. |
| `MKDIR` | `path` | Creates a single directory. |
| `MKDIR_ALL` | `path` | Creates a directory tree. |
| `OPEN` | `path`, optional `mode` (`"stat"` or `"read"`) | Opens a file, returns metadata or entire content depending on the mode. |
| `OPEN_FILE` | `path`, optional `flags`, `write`, `read` | Opens a file with custom flags (`O_RDONLY|O_CREATE` etc.), writes inline content, and/or reads it back. |
| `POSIX_RENAME` | `old`, `new` | Renames using the POSIX atomic semantics. |
| `READ_DIR` | `path` | Returns metadata for every entry. |
| `READ_FILE` | `path` | Reads the file and returns its contents as a string. |
| `READ_LINK` | `path` | Resolves a symlink target. |
| `REAL_PATH` | `path` | Returns the canonical path. |
| `REMOVE` | `path` | Deletes a file. |
| `REMOVE_DIRECTORY` | `path` | Deletes an empty directory. |
| `RENAME` | `old`, `new` | Renames a file or directory. |
| `STAT` | `path` | Returns metadata (follows symlinks). |
| `STAT_VFS` | `path` | Returns the filesystem statistics reported by the server. |
| `SYMLINK` | `old`, `new` | Creates a symbolic link. |
| `TRUNCATE` | `path`, `size` | Truncates or extends a file. |
| `UPLOAD` | `localPath`, `remotePath` | Streams a local file to the remote host. |
| `WAIT` | – | Waits for pending SFTP operations to finish. |
| `WALK` | `root` | Walks the directory tree and returns a list of paths with metadata. |
| `WRITE_FILE` | `path`, `content`, optional `mode` | Writes an entire file in a single call. |

Example: upload a configuration file and verify its checksum.

```jsonc
{
  "id": "ssh.sftp.push-config",
  "operation": "SFTP",
  "method": "UPLOAD",
  "params": {
    "localPath": "./configs/app.yaml",
    "remotePath": "/etc/app/app.yaml"
  }
},
{
  "id": "ssh.sftp.verify",
  "operation": "RUN_COMMAND_OUTPUT",
  "commands": ["sha256sum /etc/app/app.yaml"],
  "stdout": "capture"
}
```

When the top-level payload includes an `sftp` object, the action enables advanced tuning before the first SFTP call:

```jsonc
"sftp": {
  "maxConcurrentRequestsPerFile": 32,
  "maxPacket": 32768,
  "concurrentReads": true,
  "concurrentWrites": true,
  "useFstat": true
}
```

## Result payload

The action returns a JSON object with the resolved connection summary and an ordered list of step results.  Each entry contains the
step `id`, the chosen `operation`, a `success` flag, and an optional `output` field whose shape depends on the method:

```jsonc
{
  "connection": {
    "address": "cicd.example.com:22",
    "network": "tcp",
    "username": "deploy"
  },
  "steps": [
    {
      "id": "ssh.command.sample",
      "operation": "RUN_COMMAND_SMART_OUTPUT",
      "success": true,
      "output": "Linux cicd 6.6.30 #1 SMP ..."
    },
    {
      "id": "ssh.sftp.read_config",
      "operation": "SFTP",
      "success": true,
      "output": "apiVersion: v1\nkind: ConfigMap\n..."
    }
  ]
}
```

The aggregated result can be inspected from later steps via `${last_result.output.steps[0].output}` expressions, enabling complex
multi-step provisioning flows.

## Example task

```jsonc
{
  "id": "ssh.provision.node",
  "description": "Provision application dependencies over SSH",
  "action": "SSH",
  "connection": {
    "address": "app-01.internal:22",
    "username": "deploy",
    "auth": {
      "method": "password",
      "password": "${secrets.ssh_password}"
    }
  },
  "steps": [
    {
      "id": "packages.update",
      "operation": "RUN_COMMAND",
      "commands": ["sudo apt-get update"]
    },
    {
      "id": "packages.install",
      "operation": "RUN_COMMAND_SMART_OUTPUT",
      "commands": ["sudo apt-get install -y nginx"],
      "stdout": "capture"
    },
    {
      "id": "nginx.config.upload",
      "operation": "SFTP",
      "method": "UPLOAD",
      "params": {
        "localPath": "./configs/nginx.conf",
        "remotePath": "/etc/nginx/nginx.conf"
      }
    },
    {
      "id": "nginx.reload",
      "operation": "RUN_COMMAND",
      "commands": ["sudo systemctl reload nginx"]
    }
  ]
}
```

This single FlowK task upgrades package metadata, installs Nginx, uploads its configuration, and triggers a reload, while
capturing any command output that later actions might need to validate.
