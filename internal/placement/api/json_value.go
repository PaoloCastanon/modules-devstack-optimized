package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Keep object insertion order and numeric lexemes for schema diagnostics.
// Duplicate members replace their value without moving their first position,
// matching the pinned Python JSON decoder. Production never invokes Python.
type jsonMember struct {
	key   string
	value any
}
type jsonObject []jsonMember

func (o jsonObject) get(key string) (any, bool) {
	for _, m := range o {
		if m.key == key {
			return m.value, true
		}
	}
	return nil, false
}
func (o jsonObject) field(key string) any { value, _ := o.get(key); return value }

func orderedJSON(data []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		if depth > 128 {
			return nil, errors.New("JSON nesting limit exceeded")
		}
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch token {
		case json.Delim('{'):
			object := jsonObject{}
			positions := map[string]int{}
			for d.More() {
				keyToken, err := d.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errors.New("invalid JSON member")
				}
				value, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				if index, ok := positions[key]; ok {
					object[index].value = value
				} else {
					positions[key] = len(object)
					object = append(object, jsonMember{key, value})
				}
			}
			if _, err := d.Token(); err != nil {
				return nil, err
			}
			return object, nil
		case json.Delim('['):
			array := []any{}
			for d.More() {
				value, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				array = append(array, value)
			}
			if _, err := d.Token(); err != nil {
				return nil, err
			}
			return array, nil
		default:
			if _, invalid := token.(json.Delim); invalid {
				return nil, errors.New("unexpected JSON delimiter")
			}
			return token, nil
		}
	}
	value, err := read(0)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("trailing JSON data")
	}
	return value, nil
}

// Typed decoding is still separate from diagnostics. Convert only after the
// complete schema has passed, so normalization cannot change an error instance.
func plainJSON(value any) any {
	switch value := value.(type) {
	case jsonObject:
		out := make(map[string]any, len(value))
		for _, member := range value {
			out[member.key] = plainJSON(member.value)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = plainJSON(item)
		}
		return out
	default:
		return value
	}
}
