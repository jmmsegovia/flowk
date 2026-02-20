# BASE64 action

The **BASE64** action uses Go's `encoding/base64` package so flows can encode and
decode text or file contents without depending on external binaries.

## Supported operations

| Operation | Behavior |
| --- | --- |
| `ENCODE` | Standard base64 encoding with optional line wrapping (+ optional write to `outputFile`). |
| `DECODE` | Standard base64 decoding with optional garbage filtering (+ optional write to `outputFile`). |

## Field reference

| Field | Type | Description |
| --- | --- | --- |
| `operation` | string | Required. `ENCODE` or `DECODE`. |
| `input` | string | Inline input string. Mutually exclusive with `inputFile`. |
| `inputFile` | string | Input file path. Mutually exclusive with `input`. |
| `outputFile` | string | Optional output file path where resulting data will be written. |
| `wrap` | integer | Optional, only for `ENCODE`. `0` disables wrapping. When omitted, output wraps at 76 characters. |
| `ignoreGarbage` | boolean | Optional, only for `DECODE`. When true, non-base64 bytes are stripped before decoding. |
| `urlSafe` | boolean | Optional, only for `ENCODE`. When true, output is base64url (no padding, `+`/`/` → `-`/`_`). |

## Examples

### Encode inline text

```json
{
  "id": "encode.inline",
  "action": "BASE64",
  "operation": "ENCODE",
  "input": "hola flowk",
  "wrap": 0
}
```

### Decode from file into another file

```json
{
  "id": "decode.file",
  "action": "BASE64",
  "operation": "DECODE",
  "inputFile": "./tmp/payload.txt.b64",
  "outputFile": "./tmp/payload.txt",
  "ignoreGarbage": true
}
```
