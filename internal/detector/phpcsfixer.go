package detector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JacobJoergensen/laravel-fmt/internal/parser/php"
	"github.com/JacobJoergensen/laravel-fmt/internal/upward"
)

// The config is executable PHP, so it is parsed and never run: only a literal
// setRules array on a new Config() chain is read, and anything else counts as
// not found.
func detectPhpCsFixer(root string) (Pint, bool, error) {
	var path string

	for _, name := range []string{".php-cs-fixer.php", ".php-cs-fixer.dist.php"} {
		candidate := filepath.Join(root, name)

		info, err := os.Stat(candidate)
		if err != nil {
			if upward.Absent(err) {
				continue
			}

			return Pint{}, false, fmt.Errorf("stat %s: %w", candidate, err)
		}

		if info.IsDir() {
			continue
		}

		path = candidate

		break
	}

	if path == "" {
		return Pint{}, false, nil
	}

	src, err := os.ReadFile(path)
	if err != nil {
		return Pint{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	rules, ok := extractSetRules(src)
	if !ok || len(rules) == 0 {
		return Pint{}, false, nil
	}

	pint := rulesToPint(rules)
	pint.Path = path

	return pint, true, nil
}

func extractSetRules(src []byte) (map[string]json.RawMessage, bool) {
	file, errs := php.Parse(src)
	if len(errs) > 0 {
		return nil, false
	}

	var ret *php.ReturnStmt

	for _, stmt := range file.Stmts {
		r, ok := stmt.(*php.ReturnStmt)
		if !ok {
			continue
		}

		if ret != nil {
			return nil, false
		}

		ret = r
	}

	if ret == nil || ret.Expr == nil {
		return nil, false
	}

	arr, ok := findSetRulesArray(ret.Expr)
	if !ok {
		return nil, false
	}

	return transcodeRules(arr)
}

// setRules is rarely the outermost call, so the walk descends to the
// new Config() at the base.
func findSetRulesArray(expr php.Expr) (*php.ArrayExpr, bool) {
	var setRulesArgs []php.Expr

	found := false
	cur := expr

	for {
		cur = unwrapParen(cur)

		switch e := cur.(type) {
		case *php.CallExpr:
			member, ok := e.Callee.(*php.MemberExpr)
			if !ok {
				return nil, false
			}

			if identName(member.Property) == "setRules" {
				if found {
					return nil, false
				}

				found = true
				setRulesArgs = e.Args
			}

			cur = member.Object
		case *php.NewExpr:
			if !found || identName(e.Class) != "Config" {
				return nil, false
			}

			if len(setRulesArgs) != 1 {
				return nil, false
			}

			arr, ok := unwrapParen(setRulesArgs[0]).(*php.ArrayExpr)
			if !ok {
				return nil, false
			}

			return arr, true
		default:
			return nil, false
		}
	}
}

// Rule sets such as @PSR12 are skipped: laravel-fmt does not model what they
// expand to, so only rules written out are read.
func transcodeRules(arr *php.ArrayExpr) (map[string]json.RawMessage, bool) {
	rules := make(map[string]json.RawMessage, len(arr.Elements))

	for _, el := range arr.Elements {
		if el.Spread || el.ByRef {
			return nil, false
		}

		key, ok := singleQuotedString(el.Key)
		if !ok {
			return nil, false
		}

		if strings.HasPrefix(key, "@") {
			continue
		}

		value, ok := transcodeValue(el.Value)
		if !ok {
			return nil, false
		}

		rules[key] = value
	}

	return rules, true
}

func transcodeValue(expr php.Expr) (json.RawMessage, bool) {
	switch e := unwrapParen(expr).(type) {
	case *php.BoolExpr:
		if e.Value {
			return json.RawMessage("true"), true
		}

		return json.RawMessage("false"), true
	case *php.NullExpr:
		return json.RawMessage("null"), true
	case *php.StringExpr:
		value, ok := singleQuotedString(e)
		if !ok {
			return nil, false
		}

		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, false
		}

		return encoded, true
	case *php.IntExpr:
		return numberLiteral(e.Token.Text)
	case *php.FloatExpr:
		return numberLiteral(e.Token.Text)
	case *php.ArrayExpr:
		return transcodeArray(e)
	default:
		return nil, false
	}
}

func transcodeArray(arr *php.ArrayExpr) (json.RawMessage, bool) {
	if len(arr.Elements) == 0 {
		return json.RawMessage("[]"), true
	}

	if arr.Elements[0].Key != nil {
		obj := make(map[string]json.RawMessage, len(arr.Elements))

		for _, el := range arr.Elements {
			if el.Spread || el.Key == nil {
				return nil, false
			}

			key, ok := singleQuotedString(el.Key)
			if !ok {
				return nil, false
			}

			value, ok := transcodeValue(el.Value)
			if !ok {
				return nil, false
			}

			obj[key] = value
		}

		encoded, err := json.Marshal(obj)
		if err != nil {
			return nil, false
		}

		return encoded, true
	}

	list := make([]json.RawMessage, 0, len(arr.Elements))

	for _, el := range arr.Elements {
		if el.Spread || el.Key != nil {
			return nil, false
		}

		value, ok := transcodeValue(el.Value)
		if !ok {
			return nil, false
		}

		list = append(list, value)
	}

	encoded, err := json.Marshal(list)
	if err != nil {
		return nil, false
	}

	return encoded, true
}

// Double-quoted strings and any backslash escape are refused: PHP and JSON
// escape rules differ, so transcoding them could corrupt the value. Every rule
// key and value laravel-fmt reads is plain single-quoted ASCII.
func singleQuotedString(expr php.Expr) (string, bool) {
	s, ok := expr.(*php.StringExpr)
	if !ok {
		return "", false
	}

	text := s.Token.Text

	if len(text) < 2 || text[0] != '\'' || text[len(text)-1] != '\'' {
		return "", false
	}

	inner := text[1 : len(text)-1]

	if bytes.IndexByte(inner, '\\') >= 0 {
		return "", false
	}

	return string(inner), true
}

// A numeric literal passes through only if its text is already valid JSON; PHP
// forms JSON rejects (digit separators, hex, octal, leading zeros) bail.
func numberLiteral(text []byte) (json.RawMessage, bool) {
	if !json.Valid(text) {
		return nil, false
	}

	return text, true
}

func identName(expr php.Expr) string {
	id, ok := expr.(*php.IdentExpr)
	if !ok {
		return ""
	}

	if n := len(id.Parts); n > 0 {
		return string(id.Parts[n-1].Text)
	}

	return string(id.Token.Text)
}

func unwrapParen(expr php.Expr) php.Expr {
	for {
		paren, ok := expr.(*php.ParenExpr)
		if !ok {
			return expr
		}

		expr = paren.Expr
	}
}
