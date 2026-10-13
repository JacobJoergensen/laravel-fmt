package php

import (
	"errors"
	"fmt"
	"testing"
)

func TestCollectErrors(t *testing.T) {
	first := Error{Pos: Position{Line: 3, Column: 15}, Msg: "unexpected token"}
	second := Error{Pos: Position{Line: 4, Column: 2}, Msg: "expected ]"}

	wrapped := fmt.Errorf("parse: %w", errors.Join(error(first), error(second)))

	got := CollectErrors(wrapped)

	if len(got) != 2 {
		t.Fatalf("CollectErrors returned %d errors, want 2: %v", len(got), got)
	}

	if got[0] != first || got[1] != second {
		t.Fatalf("CollectErrors order/content wrong: %v", got)
	}

	if CollectErrors(errors.New("not a parse error")) != nil {
		t.Fatal("CollectErrors on a chain with no Error: want nil")
	}

	if CollectErrors(nil) != nil {
		t.Fatal("CollectErrors(nil): want nil")
	}
}
