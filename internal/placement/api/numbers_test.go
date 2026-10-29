package api

import (
	"encoding/json"
	"testing"
)

func TestExactSchemaInteger(t *testing.T) {
	for input, want := range map[string]string{"3.0": "3", "3e0": "3", "30e-1": "3", "-0.0": "0", "0e999999999999999999": "0", "9223372036854775807.0": "9223372036854775807", "-9223372036854775808.0": "-9223372036854775808", "9.223372036854775807e18": "9223372036854775807"} {
		got, err := integerNumber(json.Number(input))
		if err != nil || string(got) != want {
			t.Fatal(input, got, err)
		}
	}
	for _, input := range []string{"3.1", "1e999999999999", "1e-999999999999", "9223372036854775808.0", "-9223372036854775809.0", "9223372036854775807.000000000000001"} {
		if _, err := integerNumber(json.Number(input)); err == nil {
			t.Fatal("invalid integer accepted", input)
		}
	}
}
