# Functional Overview

`vars_expansion.go` is responsible for interpolating flow variables into raw task payloads before each action decoder runs. It walks arbitrary JSON structures, replacing `${name}` tokens with the values stored in the shared runtime context so that every action receives concrete configuration data.

# Technical Implementation Details

* **Recursive traversal:**
  * `expandTaskPayload` unmarshals the raw JSON payload into generic Go types, invokes `expandVars` to perform substitutions, and marshals the expanded structure back into `json.RawMessage`.
  * `expandVars` handles maps, arrays, and primitive strings by recursing into nested values and calling `expandString` when a string is encountered.
* **Placeholder parsing:**
  * `expandString` leverages `variablePattern` to recognise `${var}` placeholders while deliberately leaving `${from.task:...}` expressions untouched so the Variables action can resolve them later.
  * `replaceVariable` looks up each name in the shared variables map, returning informative errors when a variable is undefined or when its value cannot be stringified.
* **Value formatting:** `stringifyVariable` renders complex types such as maps and slices as JSON strings, supports `fmt.Stringer`, and gracefully converts byte slices or primitive values so interpolated strings match user expectations.
* **Error propagation:** Any lookup or formatting error aborts the expansion and bubbles up to the caller, preventing partially substituted payloads from reaching action decoders.
