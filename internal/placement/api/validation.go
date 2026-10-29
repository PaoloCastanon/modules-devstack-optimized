package api

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/placement/model"
)

// Schemas extracted without modification from Placement f4a89d3, an Apache-2.0
// project: placement/schemas/{resource_provider,inventory}.py. Only the selected
// 1.0 request schemas are embedded; this is not a general JSON Schema engine.
//
//go:embed schemas/v1.json
var schemaJSON []byte

//go:embed schemas/b08.json
var b08SchemaJSON []byte

//go:embed schemas/b09.json
var b09SchemaJSON []byte

var requestSchemas = func() jsonObject {
	value, err := orderedJSON(schemaJSON)
	if err != nil {
		panic("invalid embedded Placement schemas")
	}
	extra, err := orderedJSON(b08SchemaJSON)
	if err != nil {
		panic("invalid embedded B08 schemas")
	}
	b09, err := orderedJSON(b09SchemaJSON)
	if err != nil {
		panic("invalid embedded B09 schemas")
	}
	return append(append(value.(jsonObject), extra.(jsonObject)...), b09.(jsonObject)...)
}()

type schemaViolation struct {
	message, keyword         string
	schema                   jsonObject
	instance                 any
	schemaPath, instancePath []any
}

func (e *schemaViolation) detail(prefix string) error {
	index := func(parts []any) string {
		var b strings.Builder
		for _, key := range parts {
			b.WriteByte('[')
			switch key := key.(type) {
			case int:
				b.WriteString(strconv.Itoa(key))
			case string:
				b.WriteString(pythonString(key))
			}
			b.WriteByte(']')
		}
		return b.String()
	}
	pretty := func(value any) string { return strings.ReplaceAll(pythonPretty(value), "\n", "\n    ") }
	message := fmt.Sprintf("%s%s\n\nFailed validating %s in schema%s:\n    %s\n\nOn instance%s:\n    %s", prefix, e.message, pythonString(e.keyword), index(e.schemaPath), pretty(e.schema), index(e.instancePath), pretty(e.instance))
	// The pinned WebOb JSON formatter flattens line breaks inside the escaped
	// diagnostic fragment; the outer HTTP error paragraph is added by failure.
	message = strings.ReplaceAll(strings.ReplaceAll(message, "\n", " "), "\r", "")
	return model.Fail(400, message)
}

func validateRequest(value any, name, prefix string) error {
	schema := requestSchemas.field(name).(jsonObject)
	violations := schemaViolations(value, schema, nil, nil, strings.HasPrefix(name, "query"))
	if len(violations) == 0 {
		return nil
	}
	best := violations[0]
	for _, candidate := range violations[1:] {
		// jsonschema.best_match prefers the shortest instance path, then the
		// lexicographically greatest sibling path; equal keys retain first error.
		if len(candidate.instancePath) < len(best.instancePath) || len(candidate.instancePath) == len(best.instancePath) && comparePath(candidate.instancePath, best.instancePath) > 0 {
			best = candidate
		}
	}
	return best.detail(prefix)
}

func comparePath(a, b []any) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		c := 0
		switch left := a[i].(type) {
		case string:
			if right, ok := b[i].(string); ok {
				c = strings.Compare(left, right)
			} else {
				c = 1
			}
		case int:
			if right, ok := b[i].(int); ok {
				c = left - right
			} else {
				c = -1
			}
		}
		if c != 0 {
			return c
		}
	}
	return len(a) - len(b)
}
func childPath(path []any, keys ...any) []any {
	out := append([]any(nil), path...)
	return append(out, keys...)
}

func schemaViolations(value any, schema jsonObject, sp, ip []any, strictUUID bool) []*schemaViolation {
	var out []*schemaViolation
	fail := func(keyword, message string) {
		out = append(out, &schemaViolation{message, keyword, schema, value, childPath(sp), childPath(ip)})
	}
	array, isArray := value.([]any)
	object, isObject := value.(jsonObject)
	text, isString := value.(string)
	number, isNumber := value.(json.Number)
	for _, rule := range schema {
		switch rule.key {
		case "type":
			typ := rule.value.(string)
			valid := false
			switch typ {
			case "object":
				valid = isObject
			case "array":
				valid = isArray
			case "string":
				valid = isString
			case "integer":
				valid = isNumber && mathematicalInteger(number)
			case "number":
				valid = isNumber
			}
			if !valid {
				fail("type", pythonRepr(value)+" is not of type "+pythonString(typ))
			}
		case "items":
			if isArray {
				for index, item := range array {
					out = append(out, schemaViolations(item, rule.value.(jsonObject), childPath(sp, "items"), childPath(ip, index), strictUUID)...)
				}
			}
		case "uniqueItems":
			if isArray && rule.value == true {
				seen := map[string]bool{}
				for _, item := range array {
					key, _ := json.Marshal(plainJSON(item))
					if seen[string(key)] {
						fail("uniqueItems", pythonRepr(array)+" has non-unique elements")
						break
					}
					seen[string(key)] = true
				}
			}
		case "minItems":
			if isArray && len(array) < 1 {
				fail("minItems", pythonRepr(value)+" should be non-empty")
			}
		case "minProperties":
			if isObject && len(object) < 1 {
				fail("minProperties", pythonRepr(value)+" should be non-empty")
			}
		case "minLength":
			limit, _ := strconv.Atoi(string(rule.value.(json.Number)))
			if isString && utf8.RuneCountInString(text) < limit {
				message := "is too short"
				if limit == 1 {
					message = "should be non-empty"
				}
				fail("minLength", pythonString(text)+" "+message)
			}
		case "pattern":
			if isString && !regexp.MustCompile(rule.value.(string)).MatchString(text) {
				fail("pattern", pythonString(text)+" does not match "+pythonString(rule.value.(string)))
			}
		case "maxLength":
			limit, _ := strconv.Atoi(string(rule.value.(json.Number)))
			if isString && utf8.RuneCountInString(text) > limit {
				fail("maxLength", pythonString(text)+" is too long")
			}
		case "format":
			if isString {
				_, err := model.NormalizeUUID(text)
				// extract_json uses oslo's UUID checker; query validation uses the
				// standard checker and requires hyphenated UUIDs.
				if err != nil || strictUUID && len(text) != 36 {
					fail("format", pythonString(text)+" is not a 'uuid'")
				}
			}
		case "minimum", "maximum":
			if !isNumber {
				continue
			}
			limit := rule.value.(json.Number)
			comparison := compareDecimal(number, limit)
			if rule.key == "minimum" && comparison < 0 {
				fail("minimum", pythonRepr(number)+" is less than the minimum of "+pythonRepr(limit))
			}
			if rule.key == "maximum" && comparison > 0 {
				fail("maximum", pythonRepr(number)+" is greater than the maximum of "+pythonRepr(limit))
			}
		case "properties":
			if !isObject {
				continue
			}
			for _, property := range rule.value.(jsonObject) {
				if item, exists := object.get(property.key); exists {
					out = append(out, schemaViolations(item, property.value.(jsonObject), childPath(sp, "properties", property.key), childPath(ip, property.key), strictUUID)...)
				}
			}
		case "patternProperties":
			if !isObject {
				continue
			}
			for _, pattern := range rule.value.(jsonObject) {
				re := regexp.MustCompile(pattern.key) // embedded schema, never user regex
				for _, property := range object {
					if re.MatchString(property.key) {
						out = append(out, schemaViolations(property.value, pattern.value.(jsonObject), childPath(sp, "patternProperties", pattern.key), childPath(ip, property.key), strictUUID)...)
					}
				}
			}
		case "required":
			if !isObject {
				continue
			}
			for _, key := range rule.value.([]any) {
				if _, exists := object.get(key.(string)); !exists {
					fail("required", pythonString(key.(string))+" is a required property")
				}
			}
		case "additionalProperties":
			if !isObject || rule.value != false {
				continue
			}
			properties, _ := schema.field("properties").(jsonObject)
			var extra []string
			for _, member := range object {
				if _, exists := properties.get(member.key); !exists {
					patterns, _ := schema.field("patternProperties").(jsonObject)
					matches := false
					for _, pattern := range patterns {
						if regexp.MustCompile(pattern.key).MatchString(member.key) {
							matches = true
							break
						}
					}
					if matches {
						continue
					}
					extra = append(extra, member.key)
				}
			}
			if len(extra) > 0 {
				sort.Strings(extra)
				for i, key := range extra {
					extra[i] = pythonString(key)
				}
				if patterns, ok := schema.field("patternProperties").(jsonObject); ok {
					names := []string{}
					for _, p := range patterns {
						names = append(names, pythonString(p.key))
					}
					sort.Strings(names)
					verb := "does"
					if len(extra) > 1 {
						verb = "do"
					}
					fail("additionalProperties", strings.Join(extra, ", ")+" "+verb+" not match any of the regexes: "+strings.Join(names, ", "))
					continue
				}
				verb := "was"
				if len(extra) > 1 {
					verb = "were"
				}
				fail("additionalProperties", "Additional properties are not allowed ("+strings.Join(extra, ", ")+" "+verb+" unexpected)")
			}
		default:
			panic("unsupported embedded schema keyword")
		}
	}
	return out
}

// Mathematical integrality and decimal comparison never round generation or
// allocate an exponent-sized integer. Inputs are already valid JSON numbers.
func mathematicalInteger(value json.Number) bool {
	s := strings.TrimPrefix(string(value), "-")
	parts := strings.SplitN(strings.ToLower(s), "e", 2)
	mantissa := parts[0]
	fraction := 0
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		fraction = len(mantissa) - dot - 1
		mantissa = mantissa[:dot] + mantissa[dot+1:]
	}
	trimmed := strings.TrimRight(mantissa, "0")
	if strings.Trim(trimmed, "0") == "" {
		return true
	}
	fraction -= len(mantissa) - len(trimmed)
	exponent := int64(0)
	if len(parts) == 2 {
		var err error
		exponent, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return !strings.HasPrefix(parts[1], "-")
		}
	}
	return exponent >= int64(fraction)
}

func compareDecimal(a, b json.Number) int {
	type decimal struct {
		negative bool
		digits   string
		position int64
	}
	parse := func(value json.Number) decimal {
		s := string(value)
		negative := strings.HasPrefix(s, "-")
		s = strings.TrimPrefix(s, "-")
		parts := strings.SplitN(strings.ToLower(s), "e", 2)
		mantissa, fraction := parts[0], 0
		if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
			fraction = len(mantissa) - dot - 1
			mantissa = mantissa[:dot] + mantissa[dot+1:]
		}
		digits := strings.TrimLeft(mantissa, "0")
		if digits == "" {
			return decimal{digits: "0"}
		}
		exponent := int64(0)
		if len(parts) == 2 {
			var err error
			exponent, err = strconv.ParseInt(parts[1], 10, 64)
			bound := int64(2*(len(a)+len(b)) + 100)
			if err != nil || exponent > bound || exponent < -bound {
				exponent = bound
				if strings.HasPrefix(parts[1], "-") {
					exponent = -bound
				}
			}
		}
		return decimal{negative, digits, int64(len(digits)-fraction) + exponent}
	}
	x, y := parse(a), parse(b)
	if x.digits == "0" && y.digits == "0" {
		return 0
	}
	if x.negative != y.negative {
		if x.negative {
			return -1
		}
		return 1
	}
	sign := 1
	if x.negative {
		sign = -1
	}
	if x.digits == "0" {
		return -sign
	}
	if y.digits == "0" {
		return sign
	}
	if x.position < y.position {
		return -sign
	}
	if x.position > y.position {
		return sign
	}
	for i := 0; i < len(x.digits) || i < len(y.digits); i++ {
		left, right := byte('0'), byte('0')
		if i < len(x.digits) {
			left = x.digits[i]
		}
		if i < len(y.digits) {
			right = y.digits[i]
		}
		if left < right {
			return -sign
		}
		if left > right {
			return sign
		}
	}
	return 0
}
