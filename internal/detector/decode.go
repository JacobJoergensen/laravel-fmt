package detector

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strconv"
)

// lastKeyWins rewrites raw so a key written twice keeps only its last value, at
// any depth, as PHP's json_decode, Mago's serde_json and Node's JSON.parse read
// it; encoding/json would merge the two values instead. The key keeps its first
// place, as a PHP array does, since rule order decides what an @ set overrides.
// A syntax error is reported with its position in raw.
func lastKeyWins(raw []byte, opts ...json.Options) ([]byte, error) {
	value := jsontext.Value(raw)
	if value.IsValid() {
		return raw, nil
	}

	options := append([]json.Options{jsontext.AllowDuplicateNames(true)}, opts...)

	switch value.Kind() {
	case jsontext.KindBeginObject:
		members, err := decodeMembers(value, options...)
		if err != nil {
			return nil, fmt.Errorf("decode object: %w", err)
		}

		for i, member := range members {
			kept, err := lastKeyWins(member.value, opts...)
			if err != nil {
				return nil, err
			}

			members[i].value = kept
		}

		return encodeMembers(members, options...)
	case jsontext.KindBeginArray:
		var elements []jsontext.Value

		if err := json.Unmarshal(raw, &elements, options...); err != nil {
			return nil, fmt.Errorf("decode list: %w", err)
		}

		for i, element := range elements {
			kept, err := lastKeyWins(element, opts...)
			if err != nil {
				return nil, err
			}

			elements[i] = kept
		}

		encoded, err := json.Marshal(elements, options...)
		if err != nil {
			return nil, fmt.Errorf("encode json: %w", err)
		}

		return encoded, nil
	default:
		return raw, nil
	}
}

type jsonMember struct {
	name  string
	value jsontext.Value
}

func decodeMembers(raw jsontext.Value, opts ...json.Options) ([]jsonMember, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(raw), opts...)

	if _, err := dec.ReadToken(); err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}

	var members []jsonMember

	place := map[string]int{}

	for dec.PeekKind() == '"' {
		token, err := dec.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("read name: %w", err)
		}

		name := token.String()

		value, err := dec.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}

		if at, seen := place[name]; seen {
			members[at].value = bytes.Clone(value)

			continue
		}

		place[name] = len(members)
		members = append(members, jsonMember{name: name, value: bytes.Clone(value)})
	}

	if _, err := dec.ReadToken(); err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}

	return members, nil
}

func encodeMembers(members []jsonMember, opts ...json.Options) ([]byte, error) {
	var buf bytes.Buffer

	enc := jsontext.NewEncoder(&buf, opts...)

	err := enc.WriteToken(jsontext.BeginObject)
	for _, member := range members {
		err = errors.Join(err, enc.WriteToken(jsontext.String(member.name)), enc.WriteValue(member.value))
	}

	if err = errors.Join(err, enc.WriteToken(jsontext.EndObject)); err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}

	return bytes.TrimSpace(buf.Bytes()), nil
}

func configNumber(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	}

	return 0, false
}

func describeConfigValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return strconv.Quote(v)
	case []any:
		return "a list"
	case map[string]any:
		return "an object"
	}

	return fmt.Sprint(value)
}
