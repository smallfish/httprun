package jsonpath

import (
	"fmt"
	"strconv"
	"strings"
)

func Lookup(value any, path string) (any, bool, error) {
	current := value
	for _, part := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[part]
			if !ok {
				return nil, false, nil
			}
			current = next
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil {
				return nil, false, fmt.Errorf("json path %q requires numeric index at %q", path, part)
			}
			if index < 0 || index >= len(typed) {
				return nil, false, nil
			}
			current = typed[index]
		default:
			return nil, false, nil
		}
	}
	return current, true, nil
}
