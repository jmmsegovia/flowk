# Security Actions

Actions for encryption and security operations.

## PGP

Perform PGP encryption, decryption, and key management.

### Action: `PGP`

| Property | Type | Description |
| :--- | :--- | :--- |
| `steps` | Array | **Required**. List of PGP operations. |

#### Step Object
Specific properties depend on the `operation`.

**Operation: `ENCRYPT`**
- `message` or `messagePath`: Input data.
- `recipients`: Array of key aliases/emails.
- `outputPath`: Destination file.

**Operation: `DECRYPT`**
- `messagePath`: Encrypted file.
- `passwords`: Passwords if using symmetric encryption.
- `keyAliases`: Key aliases if using asymmetric.

### Example
```json
{
  "id": "encrypt_backup",
  "action": "PGP",
  "steps": [
    {
      "operation": "ENCRYPT",
      "messagePath": "./backup.zip",
      "recipients": ["admin@example.com"],
      "outputPath": "./backup.zip.gpg",
      "armor": true
    }
  ]
}
```
