package parser

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

func TestCollectErrorsKeepsEveryJoinedErrorInOrder(t *testing.T) {
	first := Error{Pos: Position{Line: 3, Column: 15}, Msg: "unexpected token"}
	second := Error{Pos: Position{Line: 4, Column: 2}, Msg: "expected ]"}

	cases := []struct {
		name string
		err  error
		want []Error
	}{
		{"joined inside a wrapper keeps both in order", fmt.Errorf("parse: %w", errors.Join(first, second)), []Error{first, second}},
		{"a chain with no parse error", errors.New("not a parse error"), nil},
		{"nil", nil, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CollectErrors(tc.err); !slices.Equal(got, tc.want) {
				t.Fatalf("CollectErrors = %v, want %v", got, tc.want)
			}
		})
	}
}
