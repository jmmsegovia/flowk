# Functional Overview

`flow_test.go` confirms that flow definitions load correctly, task payloads are preserved, validation rules fire when necessary, and schema validation is enforced. The tests mimic real flow files by writing temporary JSON definitions and rely on the embedded schema during validation.

# Technical Implementation Details

* **Schema preparation:** Tests register action schema fragments via `SetupSchemaProviderForTesting` so validation can merge them with the embedded base schema.
* **Payload retention:** `TestTaskUnmarshalStoresPayload` ensures the custom JSON unmarshaller stores the raw JSON blob, enabling action-specific decoders and the variable expansion routine to process it later.
* **Status initialisation:** `TestLoadDefinitionInitializesTaskStatus` verifies that tasks start in the `not started` state after loading.
* **Payload decoding:** `TestLoadDefinitionSleepActionPayload` unmarshals the stored payload to confirm that action-specific parameters (like `seconds`) remain intact.
* **Validation errors:**
  * `TestLoadDefinitionRequiresID`, `TestLoadDefinitionRequiresAction`, and `TestLoadDefinitionRejectsDuplicateIDs` expect errors when mandatory fields are missing or IDs conflict.
  * `TestLoadDefinitionUsesEmbeddedSchema` confirms schema validation succeeds without requiring a schema file on disk.
* **Testing tools:** The file uses temporary directories, runtime caller information, and Go's standard `testing` utilities to isolate each scenario and clean up after execution.
