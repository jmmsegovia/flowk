# Storage Actions

Actions for file system and cloud storage.

## GCLOUD_STORAGE

Interacts with Google Cloud Storage blocks.

### Action: `GCLOUD_STORAGE`

| Property | Type | Description |
| :--- | :--- | :--- |
| `operation` | String | **Required**. `CP` (copy), `MV` (move), `RM` (remove), `LS` (list). |
| `copy` | Object | Params for `CP`. |
| `move` | Object | Params for `MV`. |
| `remove` | Object | Params for `RM`. |
| `list` | Object | Params for `LS`. |

### Example (List Bucket)
```json
{
  "id": "list_backups",
  "action": "GCLOUD_STORAGE",
  "operation": "LS",
  "list": {
    "target": "gs://my-backup-bucket/logs/",
    "recursive": true
  }
}
```

### Example (Upload File)
```json
{
  "id": "upload_log",
  "action": "GCLOUD_STORAGE",
  "operation": "CP",
  "copy": {
    "source": "./local/app.log",
    "destination": "gs://my-app-logs/app.log"
  }
}
```
