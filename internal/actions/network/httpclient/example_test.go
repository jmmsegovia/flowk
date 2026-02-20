package httpclient

import (
	"fmt"
	"strings"

	"github.com/PaesslerAG/jsonpath"
)

func ExampleResponse_bodyJSONPath() {
	response := Response{
		Body: []any{
			map[string]any{
				"tenantName": "tenant1",
				"name":       "key1",
				"state":      "ACTIVE",
			},
		},
	}

	expr := "$[?(@.name == 'key1')].state"
	expr = strings.ReplaceAll(expr, "'", "\"")

	state, err := jsonpath.Get(expr, response.Body)
	if err != nil {
		panic(err)
	}

	values := state.([]any)
	fmt.Println(values[0])
	// Output: ACTIVE
}
