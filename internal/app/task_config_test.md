# Functional Overview

`task_config_test.go` verifies that each decoder in `task_config.go` correctly interprets task payloads, applies validation rules, and honours legacy field names. It covers success paths, error scenarios, and edge cases for Cassandra, Sleep, HTTP, and Evaluate tasks. Coverage for the new Variables decoder is planned so that placeholder validation and overwrite safeguards are exercised.

# Technical Implementation Details

* **Cassandra tests:**
  * `TestDecodeCassandraTask` checks that skip tables, platform, and operation fields are captured.
  * `TestDecodeCassandraTaskValidate` iterates over payloads missing required fields to ensure validation errors are returned.
* **Sleep tests:**
  * `TestDecodeSleepTask` validates that the seconds field is decoded.
  * `TestDecodeSleepTaskValidate` confirms that missing or non-positive durations trigger an error.
* **HTTP tests:**
  * `TestDecodeHTTPTask` writes a temporary body file, then ensures all fields (including TLS settings, authentication, and timeout) are populated in the resulting `RequestConfig`.
  * `TestDecodeHTTPTaskInlineBodyAndLegacyFields` verifies inline bodies and legacy field names (`timeoutSeconds`, `insecureSkipVerify`) are handled.
  * `TestDecodeHTTPTaskBodyFileReference` demonstrates using `body:"@path"` syntax to include external content.
  * `TestDecodeHTTPTaskValidate` feeds malformed inputs to guarantee that required fields and mutual exclusivity checks raise errors.
* **Evaluate tests:**
  * `TestDecodeEvaluateTask` confirms that condition lists and branch actions are parsed, producing default continue behaviour for then-branch and sleep/goto for else-branch.
  * `TestDecodeEvaluateTaskValidate` covers multiple invalid branch combinations (missing conditions, zero sleep, non-string continue, exit conflicts).
  * `TestDecodeEvaluateTaskExit` ensures exit branches mark `Exit=true` with the message and default the alternate branch to continue.
* **Variables tests:** Future additions will cover `decodeVariablesTask`, ensuring the payload validation errors bubble up and that duplicate declarations respect the `overwrite` flag semantics.
* **Test utilities:** Temporary directories (`t.TempDir()`), `os.WriteFile`, and `fmt.Sprintf` are used to construct payloads and ensure file-based features are exercised.
