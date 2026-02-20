# OAUTH2 action

The `OAUTH2` action centralizes common OAuth 2.0 workflows in one task type. It supports:

- Generating an authorization URL (`AUTHORIZE_URL`)
- Exchanging authorization codes (`EXCHANGE_CODE`)
- Refreshing access tokens (`REFRESH_TOKEN`)
- Device flow (`DEVICE_CODE` and `DEVICE_TOKEN`)
- Machine-to-machine authentication (`CLIENT_CREDENTIALS`)
- Legacy password flow (`PASSWORD`)
- Token introspection (`INTROSPECT`)
- Token revocation (`REVOKE`)

## Payload model

```jsonc
{
  "id": "oauth2.task",
  "action": "OAUTH2",
  "operation": "EXCHANGE_CODE",
  "token_url": "https://oauth2.googleapis.com/token",
  "client_id": "${google_client_id}",
  "client_secret": "${google_client_secret}",
  "redirect_uri": "https://developers.google.com/oauthplayground",
  "code": "${oauth_authorization_code}",
  "pkce": {
    "enabled": true,
    "verifier": "${oauth_pkce_verifier}",
    "challenge_method": "S256"
  },
  "headers": {
    "Accept": "application/json"
  },
  "extra_params": {
    "access_type": "offline"
  },
  "timeoutSeconds": 20,
  "accepted_status_codes": [200]
}
```

Notes:

- The action always returns JSON with the request/response envelope for HTTP operations.
- Sensitive fields (`client_secret`, `password`, `token`, `access_token`, `refresh_token`, etc.) are redacted in logs and structured results.
- `scopes` accepts either a string (`"openid profile"`) or an array (`["openid", "profile"]`).

## Gmail flow example (authorize + exchange + send email)

The following flow demonstrates:

1. Building a Google authorization URL.
2. Exchanging a manually provided authorization code for an access token.
3. Sending an email through Gmail API with `HTTP_REQUEST`.

> Before running it, set values for placeholders like `${google_client_id}`, `${google_client_secret}`, `${gmail_auth_code}` and `${gmail_message_raw_base64url}`.

```jsonc
{
  "id": "gmail_oauth_send_demo",
  "description": "Authorize with Google OAuth2 and send a message with Gmail API",
  "tasks": [
    {
      "id": "build_authorize_url",
      "action": "OAUTH2",
      "operation": "AUTHORIZE_URL",
      "auth_url": "https://accounts.google.com/o/oauth2/v2/auth",
      "client_id": "${google_client_id}",
      "redirect_uri": "https://developers.google.com/oauthplayground",
      "scopes": [
        "https://www.googleapis.com/auth/gmail.send"
      ],
      "state": "flowk-gmail-demo",
      "extra_params": {
        "access_type": "offline",
        "prompt": "consent"
      }
    },
    {
      "id": "exchange_code",
      "action": "OAUTH2",
      "operation": "EXCHANGE_CODE",
      "token_url": "https://oauth2.googleapis.com/token",
      "client_id": "${google_client_id}",
      "client_secret": "${google_client_secret}",
      "redirect_uri": "https://developers.google.com/oauthplayground",
      "code": "${gmail_auth_code}",
      "accepted_status_codes": [200]
    },
    {
      "id": "send_email",
      "action": "HTTP_REQUEST",
      "protocol": "HTTPS",
      "method": "POST",
      "url": "https://gmail.googleapis.com/gmail/v1/users/me/messages/send",
      "headers": {
        "Authorization": "Bearer ${from.task:exchange_code.result$.response.body.access_token}",
        "Content-Type": "application/json"
      },
      "body": "{\"raw\":\"${gmail_message_raw_base64url}\"}",
      "accepted_status_codes": [200]
    }
  ]
}
```

`from.task` placeholders are resolved during payload expansion for all actions, so the Authorization header above can reference the access token directly.

`gmail_message_raw_base64url` should be a base64url-encoded RFC822 message, for example:

```text
From: me\nTo: you@example.com\nSubject: FlowK OAuth2 demo\n\nHello from FlowK!
```
