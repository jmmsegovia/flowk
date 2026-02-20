# How to get `google_client_id` and `google_client_secret` (Gmail API)

## Required credentials file

This flow depends on the `flows/test/auth/flow_credentials/gmail_secrets_sf.json` subflow.
Without this file (including the required fields), the flow cannot authenticate with Gmail and **will not work**.

Expected format (FlowK subflow that sets variables):

```json
{
  "id": "gmail_secrets_sf",
  "description": "Define Gmail OAuth2 credentials for demo flows.",
  "tasks": [
    {
      "id": "vars.gmail_secrets",
      "action": "VARIABLES",
      "scope": "flow",
      "overwrite": true,
      "vars": [
        { "name": "google_client_id", "type": "string", "value": "<your_google_client_id>" },
        { "name": "google_client_secret", "type": "string", "value": "<your_google_client_secret>" },
        { "name": "gmail_refresh_token", "type": "string", "value": "<your_gmail_refresh_token>" },
        { "name": "gmail_from", "type": "string", "value": "<from_address>" },
        { "name": "gmail_to", "type": "string", "value": "<to_address>" }
      ]
    }
  ]
}
```

What is each field used for?
- `google_client_id`: identifies your OAuth2 application in Google Cloud.
- `google_client_secret`: secret associated with your OAuth2 client.
- `gmail_refresh_token`: allows the flow to obtain new `access_token` values without repeating user consent on every run.

> Requirement: update this file locally before running the flow. Do not commit real secrets.


## 1. Create or select a Google Cloud project
1. Go to **Google Cloud Console**:  
   https://console.cloud.google.com
2. In the **top navigation bar**, click the **project selector** (next to the Google Cloud logo).
3. Select an existing project or click **New Project** to create one.

---

## 2. Enable the Gmail API
1. Open the **left sidebar menu**.
2. Go to **APIs & Services → Library**.
3. Search for **Gmail API**.
4. Click **Gmail API**.
5. Click **Enable**.

---

## 3. Configure the OAuth consent screen
1. In the **left sidebar**, go to:  
   **Google Auth Platform → OAuth consent screen**  
   (in some interfaces this appears as **Audience / Branding / Data Access**).
2. Select **User type: External**.
3. Click **Create** or **Edit**.

### Basic app information
- **App name**: any name
- **User support email**: your email
- **Developer contact email**: your email

Click **Save and Continue**.

---

## 4. Configure OAuth scopes (Gmail permissions)
1. In the **left sidebar**, go to:  
   **Google Auth Platform → Data Access**
2. Click **Add or Remove Scopes**.
3. Add the minimum scopes you need:
   - **Read only**
     ```
     https://www.googleapis.com/auth/gmail.readonly
     ```
   - **Read and modify**
     ```
     https://www.googleapis.com/auth/gmail.modify
     ```
   - **Send emails**
     ```
     https://www.googleapis.com/auth/gmail.send
     ```
4. Click **Save**.

---

## 5. Add test users
1. In the **left sidebar**, go to:  
   **Google Auth Platform → Audience**
2. Scroll to **Test users**.
3. Click **Add users**.
4. Add the Gmail account that will authorize the app
5. Save.

> While the app is in **Testing** mode, only test users can authenticate.

---

## 6. Create OAuth credentials (Client ID & Client Secret)
1. In the **left sidebar**, go to:  
**APIs & Services → Credentials**
2. Click **Create credentials**.
3. Select **OAuth client ID**.

### Choose application type
- **Desktop app** → scripts, CLI tools, local applications
- **Web application** → web apps

If **Web application**, add an **Authorized redirect URI**, for example: http://localhost:8080/


4. Click **Create**.

---

## 7. Retrieve your credentials
A popup will display:
- **Client ID** → `google_client_id`
- **Client Secret** → `google_client_secret`

You can also find them later in:  
**APIs & Services → Credentials → OAuth 2.0 Client IDs**

---

## 8. Get a `gmail_refresh_token` (reusable)
The refresh token lets you run the flow multiple times without generating a new auth code each time.

### One-time setup
1. Make sure the flow has:
   - `access_type=offline`
   - `prompt=consent`
2. Run only the task that builds the authorize URL:
   ```bash
   ./bin/flowk run -flow ./flows/test/auth/gmail_oauth2_send.json -run-task build_authorize_url
   ```
3. Open the URL printed in the task log and authorize the app.
4. Copy the `code` from the redirect URL (it looks like `http://localhost/?code=...`).
5. Temporarily set that `code` in the flow (or in your credentials file) and run the flow once.
6. Read the `refresh_token` from the task log of `refresh_access_token` (or `exchange_code` if you still use code exchange).
   - Example log path:
     ```
     logs/gmail_oauth_send_demo/task-000X-refresh_access_token/task_log.json
     ```
7. Store the `refresh_token` in `flows/test/auth/flow_credentials/gmail_secrets_sf.json` under `gmail_refresh_token`.

### If no refresh token is returned
Google only issues a refresh token the **first time** a user consents to an app for a given client + scope set.
To force a new one:
1. Revoke the app in your Google account’s security settings, or
2. Change the OAuth client ID, or
3. Keep `prompt=consent` and remove the app’s access, then re-authorize.

## Notes
- Never expose your `client_secret` in public repositories.
- App publishing and Google verification are **not required** for testing.
- Only **test users** can authorize the app while it is in **Testing** mode.
