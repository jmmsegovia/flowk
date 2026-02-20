# PGP Action

The **PGP** action lets you manage OpenPGP keys and work with encrypted messages
from a FlowK flow. All operations run in memory within a single step, so
imported or generated keys only live for the duration of the task.

Each payload defines a `steps` array. Steps run in order and can import keys,
encrypt or decrypt data, sign content, and verify signatures. The task result
includes a JSON summary of each step.

## `GENERATE_KEY`

Generates an RSA key pair and registers it under an alias for use in later
steps. The result includes ASCII armored keys and fingerprints. Optionally you
can persist the files to disk.

```jsonc
{
  "operation": "GENERATE_KEY",
  "id": "keypair",
  "alias": "demo",
  "name": "Demo User",
  "email": "demo@example.com",
  "rsaBits": 3072,
  "privateKeyPath": "./flows/test/artifacts/demo_private.asc",
  "publicKeyPath": "./flows/test/artifacts/demo_public.asc"
}
```

The alias is available immediately after creation, so it can be used in
subsequent steps to sign or decrypt.

## `IMPORT_KEY`

Imports one or more OpenPGP `Entity` values into an alias that can be used in
later steps. Keys can be provided as ASCII armored text or binary. If the
private key is protected with a passphrase, supply it in the same step to
unlock it.

```jsonc
{
  "operation": "IMPORT_KEY",
  "alias": "deploy",
  "keyPath": "./certs/deploy.asc",
  "passphrase": "${secrets.deploy_passphrase}"
}
```

## `ENCRYPT`

Encrypts a message using the public keys associated with the given aliases. The
result is armored (`armor: true`, default) or binary if you disable it.
Optionally, the message can be signed with a private key imported under an
alias.

You can also encrypt symmetrically using `password` instead of `recipients`. In
that case the message is protected with the provided password and signing is
not supported.

```jsonc
{
  "operation": "ENCRYPT",
  "recipients": ["deploy"],
  "message": "Version 1.2.3 ready to publish",
  "signWith": "deploy",
  "outputPath": "./flows/test/artifacts/message.asc"
}

// symmetric encryption
{
  "operation": "ENCRYPT",
  "password": "${secrets.release_password}",
  "messagePath": "./flows/test/artifacts/package.tar.gz",
  "outputPath": "./flows/test/artifacts/package.tar.gz.pgp"
}
```

## `DECRYPT`

Decrypts a message using the available private keys. If the step specifies
`keyAliases`, only those aliases are considered; otherwise all imported keys
are used. The step can require a signature (`requireSignature: true`) and
restrict who may sign via `allowedSigners`.

When decrypting symmetric messages, include `passwords` with a list of
candidate passwords. The first correct value unlocks the content.

```jsonc
{
  "operation": "DECRYPT",
  "messagePath": "./flows/test/artifacts/message.asc",
  "keyAliases": ["deploy"],
  "requireSignature": true,
  "allowedSigners": ["deploy"],
  "outputEncoding": "utf8"
}

// symmetric decryption
{
  "operation": "DECRYPT",
  "messagePath": "./flows/test/artifacts/package.tar.gz.pgp",
  "passwords": ["${secrets.release_password}"]
}
```

## `SIGN_DETACHED`

Generates a detached signature for the provided content. You can request
armored output (`armor: true`) or binary (`armor: false` +
`resultEncoding: "base64"` to obtain the signature encoded in Base64).

```jsonc
{
  "operation": "SIGN_DETACHED",
  "signWith": "deploy",
  "message": "checksum=7b3c0d3f",
  "armor": true
}
```

## `VERIFY_DETACHED`

Verifies a detached signature using the aliases that hold the authorized
public keys.

```jsonc
{
  "operation": "VERIFY_DETACHED",
  "messagePath": "./download/package.tgz",
  "signature": "-----BEGIN PGP SIGNATURE-----...",
  "keyAliases": ["deploy"]
}
```

## Result

The action result is a JSON object with the `steps` list. Each entry includes
`operation`, `id` (if provided), and a `result` block with the relevant data for
the step (for example, imported aliases, fingerprints, decrypted text, or
verification metadata). Encryption steps indicate whether the message was
protected with a password (`symmetric: true`), and decryption reports whether a
password was used (`usedPassword: true`).
