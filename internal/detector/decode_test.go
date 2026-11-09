package detector

import (
	"bytes"
	"encoding/json/v2"
	"reflect"
	"testing"
)

func TestLastKeyWinsKeepsTheFirstPlaceOfARepeatedKey(t *testing.T) {
	for range 20 {
		got, err := lastKeyWins([]byte(`{"d":1,"c":2,"b":3,"a":4,"d":5}`))
		if err != nil {
			t.Fatalf("lastKeyWins: %v", err)
		}

		if string(got) != `{"d":5,"c":2,"b":3,"a":4}` {
			t.Fatalf("got %s, want the written order with d's last value", got)
		}
	}
}

func TestLastKeyWinsReturnsJSONWithoutARepeatedKeyAsWritten(t *testing.T) {
	raw := []byte("{ \"a\" : [1,  2] }")

	kept, err := lastKeyWins(raw)
	if err != nil {
		t.Fatalf("lastKeyWins: %v", err)
	}

	if !bytes.Equal(kept, raw) {
		t.Fatalf("lastKeyWins = %s, want the input unchanged", kept)
	}
}

func TestLastKeyWinsKeepsOnlyTheLastValueOfARepeatedKey(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"scalar", `{"a": 1, "a": 2}`, `{"a": 2}`},
		{"value of another type", `{"a": "x", "a": {"b": 1}}`, `{"a": {"b": 1}}`},
		{"object replaced, not merged", `{"a": {"b": 1}, "a": {"c": 2}}`, `{"a": {"c": 2}}`},
		{"nested object", `{"a": {"b": 1, "b": 2}}`, `{"a": {"b": 2}}`},
		{"object in a list", `[{"a": 1, "a": 2}]`, `[{"a": 2}]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kept, err := lastKeyWins([]byte(tc.raw))
			if err != nil {
				t.Fatalf("lastKeyWins: %v", err)
			}

			var got, want any
			if err := json.Unmarshal(kept, &got); err != nil {
				t.Fatalf("decode %s: %v", kept, err)
			}

			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatalf("decode want: %v", err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("lastKeyWins(%s) = %s, want %s", tc.raw, kept, tc.want)
			}
		})
	}
}
