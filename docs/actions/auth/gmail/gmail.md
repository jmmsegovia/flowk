# GMAIL action

The `GMAIL` action sends an email message through the Gmail API with OAuth2.

Supported operations:

- `SEND_MESSAGE`

## Payload fields

- `operation` (required): Must be `SEND_MESSAGE`.
- `access_token` (required): OAuth2 access token with Gmail send scope.
- `raw_message` (required): Raw RFC 2822 email encoded in base64url format (no `=` padding).
- `user_id` (optional): Gmail user id for API path (`me` by default).
- `headers` (optional): Additional HTTP headers.
- `expected_status_codes` (optional): Defaults to `[200]`.
- `timeoutSeconds` (optional): Timeout in seconds.

## Example task

```json
{
  "id": "gmail_send_message",
  "action": "GMAIL",
  "operation": "SEND_MESSAGE",
  "access_token": "${gmail_access_token}",
  "raw_message": "${gmail_message_raw_base64url}",
  "user_id": "me"
}
```

## End-to-end flow snippet

```json
{
  "id": "gmail_send_message_demo",
  "description": "Refresh token and send Gmail message",
  "tasks": [
    {
      "id": "refresh_access_token",
      "action": "OAUTH2",
      "operation": "REFRESH_TOKEN",
      "token_url": "https://oauth2.googleapis.com/token",
      "client_id": "${google_client_id}",
      "client_secret": "${google_client_secret}",
      "refresh_token": "${gmail_refresh_token}"
    },
    {
      "id": "send_message",
      "action": "GMAIL",
      "operation": "SEND_MESSAGE",
      "access_token": "${from.task:refresh_access_token.result$.response.body.access_token}",
      "raw_message": "${gmail_message_raw_base64url}"
    }
  ]
}
```
