# Security Policy

## Supported versions

The latest released minor version is supported for security updates.

## Reporting a vulnerability

Please report vulnerabilities privately to **flowkengine@gmail.com** and include:

- Description of the issue
- Affected versions/commit
- Reproduction steps or PoC
- Suggested remediation (if available)

We aim to acknowledge reports within 48 hours and provide a remediation plan as
quickly as possible.

## Secrets handling guidance

- Never commit real credentials, private keys, or production certificates.
- Use environment variables or secret managers for sensitive values.
- Prefer placeholders in examples and fixtures.
