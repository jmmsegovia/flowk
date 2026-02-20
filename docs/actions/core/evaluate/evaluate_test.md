# Functional Overview

`evaluate_test.go` validates that the Evaluate action correctly inspects task outcomes, interprets JSON-path expressions, handles control-flow scenarios, and reports meaningful errors. The tests simulate different task payloads and ensure the action returns the expected boolean result and `flow.ResultTypeBool` metadata.

# Technical Implementation Details

* **Test scaffolding:**
  * A `stubLogger` collects log messages so assertions can confirm when empty JSON-path results are reported to users.
  * `unmarshalSample` converts a constant JSON document into a `map[string]any`, mirroring how real task results appear.
* **Happy-path scenarios:**
  * `TestExecuteReturnsTrueWhenConditionsMet` proves that simple boolean checks and JSON-path filters succeed.
  * Additional tests (`TestExecuteSupportsJSONArrayBody`, `TestExecuteHandlesThenBranchFieldsInJSON`, `TestExecuteAllowsEmptyCollectionComparison`, `TestExecuteHandlesNumericComparisons`) cover arrays, nested maps, empty slices, and numeric comparisons (including `>`, `<`, `>=`, `<=`), ensuring type coercion logic works as designed.
* **Failure and error coverage:**
  * `TestExecuteReturnsFalseWhenConditionFails` confirms mismatching expectations lead to a false result without an error.
  * `TestExecuteLogsWhenJSONPathReturnsEmptyCollection` observes logging when JSON-path queries produce no matches.
  * `TestExecuteErrorsOnUnsupportedField`, `TestExecuteErrorsWhenJSONResultRequired`, and `TestExecuteErrorsWhenTaskIsNil` confirm the function rejects invalid configuration, missing JSON data, and nil inputs with explicit errors.
* **Validation of condition definitions:** `TestConditionValidateErrors` iterates over bad configurations to make sure `Condition.Validate` surfaces precise error text for missing fields or unsupported operations, while `TestConditionValidateSupportsComparisons` confirms all supported operators pass validation.
* **Branching behaviour:** Tests confirm that then/else branch JSON payloads (including nested `then`/`else` keys) are navigated correctly via JSON-path selectors.
