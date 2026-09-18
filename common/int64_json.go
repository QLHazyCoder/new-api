package common

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Int64Value accepts both JSON numbers and decimal strings. It is used at
// write boundaries where JavaScript clients may need to preserve all int64
// digits without changing the existing request shape.
type Int64Value int64

func (value *Int64Value) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("expected int64 value")
	}
	if trimmed[0] == '"' {
		var text string
		if err := Unmarshal(trimmed, &text); err != nil {
			return err
		}
		trimmed = []byte(strings.TrimSpace(text))
	}
	parsed, err := strconv.ParseInt(string(trimmed), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid int64 value: %w", err)
	}
	*value = Int64Value(parsed)
	return nil
}

func (value Int64Value) Int64() int64 {
	return int64(value)
}
