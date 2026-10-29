package api

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
)

// Schema integer means a mathematical integer, including 3.0 or 3e0. Parse
// decimal digits exactly; float64 rounding must never change a generation.
func integerNumber(value json.Number) (json.Number, error) {
	original := string(value)
	s := original
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	parts := strings.SplitN(strings.ToLower(s), "e", 2)
	mantissa := parts[0]
	fraction := 0
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		fraction = len(mantissa) - dot - 1
		mantissa = mantissa[:dot] + mantissa[dot+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return json.Number("0"), nil
	}
	bad := func() (json.Number, error) { return "", errors.New("not a signed 64-bit integer") }
	exponent := 0
	if len(parts) == 2 {
		var err error
		exponent, err = strconv.Atoi(parts[1])
		if err != nil {
			return bad()
		}
	}
	// Check bounds before subtraction, allocation or exponent expansion.
	if exponent > len(original)+19 || exponent < -len(original)-19 {
		return bad()
	}
	scale := exponent - fraction
	if scale < 0 {
		remove := -scale
		if remove > len(digits) || strings.Trim(digits[len(digits)-remove:], "0") != "" {
			return bad()
		}
		digits = digits[:len(digits)-remove]
	} else {
		if len(digits)+scale > 19 {
			return bad()
		}
		digits += strings.Repeat("0", scale)
	}
	if digits == "" {
		digits = "0"
	}
	if negative {
		digits = "-" + digits
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return bad()
	}
	return json.Number(strconv.FormatInt(n, 10)), nil
}

func schemaIntegers(value any, target reflect.Type) (any, error) {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	switch target.Kind() {
	case reflect.Int64:
		number, ok := value.(json.Number)
		if !ok {
			return nil, errors.New("expected integer")
		}
		return integerNumber(number)
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("expected object")
		}
		for n := 0; n < target.NumField(); n++ {
			field := target.Field(n)
			key := strings.Split(field.Tag.Get("json"), ",")[0]
			item, exists := object[key]
			if exists {
				v, err := schemaIntegers(item, field.Type)
				if err != nil {
					return nil, err
				}
				object[key] = v
			}
		}
	case reflect.Map:
		object, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("expected object")
		}
		for key, item := range object {
			v, err := schemaIntegers(item, target.Elem())
			if err != nil {
				return nil, err
			}
			object[key] = v
		}
	}
	return value, nil
}
