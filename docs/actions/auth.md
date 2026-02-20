# Authentication Actions

Actions for OAuth and identity provider integrations.

## GMAIL

Sends Gmail messages using an OAuth2 access token.

### Action: `GMAIL`

| Property | Type | Description |
| :--- | :--- | :--- |
| `operation` | String | **Required**. Gmail operation (`SEND_MESSAGE`). |
| `access_token` | String | **Required**. OAuth2 access token with `https://www.googleapis.com/auth/gmail.send` scope. |
| `raw_message` | String | **Required**. RFC 2822 email encoded as base64url (no padding). |
| `user_id` | String | Optional Gmail user id in request path (`me` by default). |
| `headers` | Object | Optional custom HTTP headers. |
| `expected_status_codes` | Array<Integer> | Optional accepted HTTP status codes (defaults to `[200]`). |
| `timeoutSeconds` | Number | Optional timeout for the request in seconds. |

### Example

```json
{
  "id": "gmail_send",
  "action": "GMAIL",
  "operation": "SEND_MESSAGE",
  "access_token": "${gmail_access_token}",
  "raw_message": "${gmail_message_raw_base64url}",
  "user_id": "me"
}
```

For complete details and practical examples, see [`docs/actions/auth/gmail/gmail.md`](./auth/gmail/gmail.md).

## OAUTH2

Generates OAuth authorization URLs and performs token endpoint operations (`authorization_code`, `refresh_token`, device flow, client credentials, password, introspection, and revocation).

### Action: `OAUTH2`

| Property | Type | Description |
| :--- | :--- | :--- |
| `operation` | String | **Required**. OAuth operation (`AUTHORIZE_URL`, `EXCHANGE_CODE`, `REFRESH_TOKEN`, `DEVICE_CODE`, `DEVICE_TOKEN`, `CLIENT_CREDENTIALS`, `PASSWORD`, `INTROSPECT`, `REVOKE`). |
| `client_id` | String | Usually required by OAuth providers. |
| `client_secret` | String | Required for confidential client operations. |
| `auth_url` | String | Authorization endpoint for `AUTHORIZE_URL`. |
| `token_url` | String | Token endpoint for exchange/refresh/device/client credentials/password. |
| `device_url` | String | Device authorization endpoint for `DEVICE_CODE`. |
| `redirect_uri` | String | Redirect URI used in authorization code flow. |
| `scopes` | String or Array | Scope list as a space-delimited string or array of strings. |
| `code` | String | Authorization code for `EXCHANGE_CODE`. |
| `refresh_token` | String | Refresh token for `REFRESH_TOKEN`. |
| `device_code` | String | Device code for `DEVICE_TOKEN`. |
| `token` | String | Token for `INTROSPECT` and `REVOKE`. |
| `headers` | Object | Optional custom HTTP headers. |
| `extra_params` | Object | Optional provider-specific parameters. |
| `pkce` | Object | Optional PKCE settings (`enabled`, `verifier`, `challenge`, `challenge_method`). |
| `expected_status_codes` | Array<Integer> | Optional accepted HTTP status codes (defaults to `[200]`). |

### Example

```json
{
  "id": "oauth2_authorize",
  "action": "OAUTH2",
  "operation": "AUTHORIZE_URL",
  "auth_url": "https://accounts.google.com/o/oauth2/v2/auth",
  "client_id": "${google_client_id}",
  "redirect_uri": "https://developers.google.com/oauthplayground",
  "scopes": [
    "https://www.googleapis.com/auth/gmail.send"
  ],
  "extra_params": {
    "access_type": "offline",
    "prompt": "consent"
  },
  "pkce": {
    "enabled": true,
    "verifier": "replace-with-a-random-verifier"
  }
}
```

For full payload patterns and a Gmail end-to-end flow example, see [`docs/actions/auth/oauth2/oauth2.md`](./auth/oauth2/oauth2.md).
