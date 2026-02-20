# Functional Overview

`example_test.go` contains a documentation example demonstrating how to query the JSON body of an HTTP response returned by the HTTP_REQUEST action. The example shows how to use JSONPath expressions to extract specific fields from the `Response.Body` structure.

# Technical Implementation Details

* **Example-driven documentation:** Go examples within `*_test.go` files double as executable documentation. Running `go test` verifies that the printed output matches the comment following `// Output:`.
* **JSONPath usage:** The example builds a `Response` with a body containing an array of dictionaries. It defines a JSONPath expression that filters by the `name` attribute and selects the associated `state` field. Because the jsonpath library expects double quotes, the snippet replaces single quotes via `strings.ReplaceAll`. The same JSONPath syntax is supported by VARIABLES tasks when they reference previous HTTP responses.
* **Library integration:** `github.com/PaesslerAG/jsonpath` is used to evaluate the expression against the generic `[]any` representation. The resulting value is a slice so the code indexes `[0]` and prints it, yielding `ACTIVE` as documented.
