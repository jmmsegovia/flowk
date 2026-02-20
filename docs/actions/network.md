# Network Actions

Actions for interacting with network services and remote servers.

## HTTP_REQUEST

Performs an HTTP or HTTPS request.

### Action: `HTTP_REQUEST`

| Property | Type | Description |
| :--- | :--- | :--- |
| `protocol` | String | **Required**. `HTTP` or `HTTPS`. |
| `method` | String | **Required**. `GET`, `POST`, `PUT`, `DELETE`. |
| `url` | String | **Required**. Target URL (e.g., `https://api.example.com/v1/resource` or `//api.example.com/v1/resource`). |
| `headers` | Object | Optional key-value pairs for headers. |
| `body` | String | Optional raw request body. |
| `body_file` | String | Optional path to a file to use as the body. |

### Example
```json
{
  "id": "get_user_data",
  "action": "HTTP_REQUEST",
  "protocol": "HTTPS",
  "method": "GET",
  "url": "https://api.example.com/users/123",
  "headers": {
    "Authorization": "Bearer ${token}"
  }
}
```

---

## SSH

Executes commands or scripts on a remote server via SSH.

### Action: `SSH`

| Property | Type | Description |
| :--- | :--- | :--- |
| `connection` | Object | **Required**. Connection details (address, user, auth). |
| `steps` | Array | **Required**. Sequence of SSH operations. |

#### Connection Object
| Property | Description |
| :--- | :--- |
| `address` | `host:port`. |
| `username` | SSH user. |
| `auth` | Object containing `method` (`password`, `private_key` etc.) and credentials. |

#### Step Object (Operation: `RUN_COMMAND`)
| Property | Description |
| :--- | :--- |
| `operation` | `RUN_COMMAND`. |
| `commands` | Array of command strings to execute. |

### Example
```json
{
  "id": "uptime_check",
  "action": "SSH",
  "connection": {
    "address": "192.168.1.10:22",
    "username": "admin",
    "auth": { "method": "password", "password": "${ssh_pass}" }
  },
  "steps": [
    { "operation": "RUN_COMMAND", "commands": ["uptime", "whoami"] }
  ]
}
```

---

## TELNET

Interacts with a TCP service via Telnet-like session (send/expect).

### Action: `TELNET`

| Property | Type | Description |
| :--- | :--- | :--- |
| `host` | String | **Required**. Hostname or IP. |
| `port` | Integer | Optional. Port number (defaults to 23). |
| `steps` | Array | **Required**. List of interaction steps. |

#### Step Object
Can be one of:
- `connect`: Establish connection.
- `send`: Send data (`data` property).
- `expect`: Wait for pattern (`pattern` property).
- `close`: Close connection.

### Example
```json
{
  "id": "smtp_check",
  "action": "TELNET",
  "host": "smtp.example.com",
  "port": 25,
  "steps": [
    { "connect": {} },
    { "expect": { "pattern": "220" } },
    { "send": { "data": "EHLO client" } },
    { "close": {} }
  ]
}
```
