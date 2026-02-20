# Functional Overview

`validation.go` ensures that flow definitions conform to the JSON schema bundled with the project. It uses the embedded `values.schema.json` combined with action schema fragments, caches compiled schemas, and reports human-readable validation errors when the flow does not match the expected structure.

# Technical Implementation Details

* **Schema discovery:** The base schema (`values.schema.json`) is embedded into the binary, so no filesystem searches are performed during validation.
* **Schema loading and caching:** `loadFlowSchema` merges the embedded base schema with the registered action fragments, then compiles the combined schema with `gojsonschema.NewSchema`. Compiled schemas are stored in a `sync.Map` so subsequent validations avoid rebuilding the merged schema.
* **Validation routine:** `validateDefinitionAgainstSchema` retrieves the schema, validates the provided JSON bytes, and aggregates all validation messages into a single error string when the document is invalid. Successful validation returns nil so the caller can proceed with unmarshalling.
* **Dependencies:** Besides standard library packages for filesystem access and string manipulation, the code uses `github.com/xeipuuv/gojsonschema` for JSON Schema validation.
