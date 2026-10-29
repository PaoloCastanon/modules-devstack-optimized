package api

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
)

func TestPinnedSchemaOracle(t *testing.T) {
	data, err := os.ReadFile("../../../tests/compatibility/fixtures/placement-schema-errors.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name, Schema, Body string
			Status             int
			Instance           json.RawMessage
			Response           struct{ Errors []struct{ Detail string } }
		}
	}
	if json.Unmarshal(data, &fixture) != nil || len(fixture.Cases) < 50 {
		t.Fatal("invalid oracle fixture")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			input := []byte(tc.Body)
			prefix := "JSON does not validate: "
			if tc.Schema == "query" {
				input = tc.Instance
				prefix = "Invalid query string parameters: "
			}
			value, err := orderedJSON(input)
			if err != nil {
				t.Fatal(err)
			}
			err = validateRequest(value, tc.Schema, prefix)
			if tc.Status == 200 {
				if err != nil {
					t.Fatal("valid input rejected", err)
				}
				return
			}
			var failure *model.Error
			if !errors.As(err, &failure) || failure.HTTPStatus != tc.Status || len(tc.Response.Errors) != 1 {
				t.Fatal(err)
			}
			got := "The server could not comply with the request since it is either malformed or otherwise incorrect.\n\n " + failure.Detail + "  "
			if got != tc.Response.Errors[0].Detail {
				t.Errorf("got: %s\nwant: %s", got, tc.Response.Errors[0].Detail)
			}
		})
	}
}

func TestDecimalComparisonWithoutRounding(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"1.0", "1", 0}, {"1e99999999999999999999", "2147483647", 1},
		{"-1e99999999999999999999", "0", -1}, {"1e-99999999999999999999", "0", 1},
		{"-0e99999999999999999999", "0", 0}, {"2147483647.00000000001", "2147483647", 1},
		{"0.999999999999999999999", "1", -1}, {"-2", "-1", -1}, {"-1", "-2", 1},
	} {
		if got := compareDecimal(json.Number(tc.a), json.Number(tc.b)); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestOrderedJSONBoundsAndDuplicates(t *testing.T) {
	value, err := orderedJSON([]byte(`{"name":null,"extra":1,"name":"last"}`))
	if err != nil || pythonRepr(value) != "{'name': 'last', 'extra': 1}" {
		t.Fatal(value, err)
	}
	for _, invalid := range []string{`{} {}`, `{"x":}`, `[}`, `{"x":1`, `{"x":1,}`} {
		if _, err := orderedJSON([]byte(invalid)); err == nil {
			t.Fatal("invalid JSON accepted", invalid)
		}
	}
	for _, depth := range []int{128, 129} {
		_, err := orderedJSON([]byte(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth)))
		if (err != nil) != (depth > 128) {
			t.Fatal("JSON nesting bound", depth, err)
		}
	}
}
