package fields

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// normalizeJSON accepts ordinary JSON-compatible Go values, never custom
// marshalers, structs or pointers. This bounds work before calling json.Marshal
// (including cyclic values) and prevents invalid UTF-8 replacement. At the
// field boundary, legacy strings are parsed as encoded JSON, not rewritten.
func normalizeJSON(value any, parseString bool, maxDepth int) (any, error) {
	if raw, ok := value.(string); ok && parseString {
		if len(raw) > MaxValueBytes || !utf8.ValidString(raw) {
			return nil, fmt.Errorf("JSON exceeds size limit or is not UTF-8")
		}
		// Token preflight limits allocation before decoding the full document.
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		depth, nodes := 0, 0
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("invalid JSON syntax")
			}
			nodes++
			if nodes > MaxNodes {
				return nil, fmt.Errorf("JSON exceeds node limit")
			}
			if delim, ok := token.(json.Delim); ok {
				if delim == '{' || delim == '[' {
					depth++
					if depth > maxDepth {
						return nil, fmt.Errorf("JSON exceeds depth limit %d", maxDepth)
					}
				} else {
					depth--
				}
			}
		}
		decoder = json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("invalid JSON syntax")
		}
		if _, err := decoder.Token(); err != io.EOF {
			return nil, fmt.Errorf("JSON must contain one value")
		}
	}
	budget := jsonBudget{maxDepth: maxDepth}
	if err := budget.visit(value, 0); err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > MaxValueBytes {
		return nil, fmt.Errorf("JSON exceeds size limit or contains invalid values")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var result any
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("invalid JSON value")
	}
	return result, nil
}

type jsonBudget struct{ nodes, size, maxDepth int }

func (b *jsonBudget) visit(value any, depth int) error {
	b.nodes++
	b.size++
	if b.nodes > MaxNodes || b.size > MaxValueBytes {
		return fmt.Errorf("JSON exceeds node or size limit")
	}
	switch v := value.(type) {
	case nil, bool:
	case string:
		if !utf8.ValidString(v) {
			return fmt.Errorf("JSON strings must be UTF-8")
		}
		b.size += len(v)
	case map[string]any:
		if depth >= b.maxDepth {
			return fmt.Errorf("JSON exceeds depth limit %d", b.maxDepth)
		}
		if len(v) > MaxNodes {
			return fmt.Errorf("JSON exceeds node limit")
		}
		for key, child := range v {
			if !utf8.ValidString(key) {
				return fmt.Errorf("JSON keys must be UTF-8")
			}
			b.size += len(key)
			if err := b.visit(child, depth+1); err != nil {
				return err
			}
		}
	case []any, []string, []map[string]any:
		if depth >= b.maxDepth {
			return fmt.Errorf("JSON exceeds depth limit %d", b.maxDepth)
		}
		items, _ := array(v)
		if len(items) > MaxNodes {
			return fmt.Errorf("JSON exceeds node limit")
		}
		for _, child := range items {
			if err := b.visit(child, depth+1); err != nil {
				return err
			}
		}
	default:
		if _, ok := numeric(value); !ok {
			return fmt.Errorf("expected JSON-compatible value with finite numbers")
		}
		b.size += 24
	}
	if b.size > MaxValueBytes {
		return fmt.Errorf("JSON exceeds size limit")
	}
	return nil
}

// JSONSchemaKeywords is the entire supported subset. type is a single string;
// items/properties contain object schemas; additionalProperties is boolean.
// All other keywords (including $ref, $schema, format, combinators and regex
// property names) are rejected, including in nested schemas. No remote fetches.
// Bounds apply to matching instance types, as in JSON Schema. Root schemas are
// objects; boolean schemas and tuples are deliberately unsupported.
func JSONSchemaKeywords() []string {
	return []string{"type", "enum", "const", "properties", "required", "additionalProperties", "items", "minItems", "maxItems", "uniqueItems", "minLength", "maxLength", "pattern", "minimum", "maximum", "title", "description"}
}

func validateSchema(schema map[string]any) error {
	value, err := normalizeJSON(schema, false, 2*MaxDepth+2)
	if err != nil {
		return err
	}
	return validateSchemaNode(value.(map[string]any), 1)
}

func validateSchemaNode(schema map[string]any, depth int) error {
	if depth > MaxDepth {
		return fmt.Errorf("schema exceeds depth limit %d", MaxDepth)
	}
	allowed := make(map[string]bool)
	for _, keyword := range JSONSchemaKeywords() {
		allowed[keyword] = true
	}
	for key, value := range schema {
		if !allowed[key] {
			return fmt.Errorf("unsupported keyword %q", key)
		}
		switch key {
		case "type":
			s, ok := value.(string)
			if !ok || !contains([]string{"null", "boolean", "object", "array", "number", "integer", "string"}, s) {
				return fmt.Errorf("type must be one supported type string")
			}
		case "title", "description":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s must be a string", key)
			}
		case "enum":
			items, ok := array(value)
			if !ok || len(items) == 0 || len(items) > 256 {
				return fmt.Errorf("enum must contain 1 to 256 values")
			}
			seen := make(map[string]bool)
			for _, item := range items {
				k := jsonKey(item)
				if seen[k] {
					return fmt.Errorf("enum values must be distinct")
				}
				seen[k] = true
			}
		case "const": // Any bounded JSON value is supported.
		case "properties":
			properties, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("properties must be an object")
			}
			for _, child := range properties {
				s, ok := child.(map[string]any)
				if !ok {
					return fmt.Errorf("property schemas must be objects")
				}
				if err := validateSchemaNode(s, depth+1); err != nil {
					return err
				}
			}
		case "items":
			s, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("items must be an object schema")
			}
			if err := validateSchemaNode(s, depth+1); err != nil {
				return err
			}
		case "required":
			items, ok := array(value)
			if !ok {
				return fmt.Errorf("required must be an array of property names")
			}
			seen := make(map[string]bool)
			for _, item := range items {
				s, ok := item.(string)
				if !ok || seen[s] {
					return fmt.Errorf("required must contain distinct strings")
				}
				seen[s] = true
			}
		case "additionalProperties", "uniqueItems":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s must be boolean", key)
			}
		case "minItems", "maxItems", "minLength", "maxLength":
			if _, ok := schemaCount(value); !ok {
				return fmt.Errorf("%s must be an integer between 0 and %d", key, MaxValueBytes)
			}
		case "minimum", "maximum":
			if _, ok := numeric(value); !ok {
				return fmt.Errorf("%s must be finite numeric value", key)
			}
		case "pattern":
			s, ok := value.(string)
			if !ok {
				return fmt.Errorf("pattern must be a string")
			}
			if err := validPattern(s); err != nil {
				return fmt.Errorf("invalid pattern: %v", err)
			}
		}
	}
	for _, keys := range [][2]string{{"minItems", "maxItems"}, {"minLength", "maxLength"}, {"minimum", "maximum"}} {
		lo, lok := schema[keys[0]]
		hi, hik := schema[keys[1]]
		if lok && hik {
			l, _ := numeric(lo)
			h, _ := numeric(hi)
			if l.Cmp(h) > 0 {
				return fmt.Errorf("%s exceeds %s", keys[0], keys[1])
			}
		}
	}
	return nil
}

func contains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func schemaCount(value any) (int, bool) {
	n, ok := numeric(value)
	if !ok || !n.IsInt() || !n.Num().IsInt64() {
		return 0, false
	}
	x := n.Num().Int64()
	return int(x), x >= 0 && x <= MaxValueBytes
}

func checkSchema(schema map[string]any, value any, path string) error {
	budget := MaxNodes * MaxDepth
	return checkSchemaNode(schema, value, path, &budget)
}

func checkSchemaNode(schema map[string]any, value any, path string, budget *int) error {
	*budget--
	if *budget < 0 {
		return fmt.Errorf("JSON schema validation work limit exceeded")
	}
	fail := func(message string) error { return fmt.Errorf("JSON %s: %s", path, message) }
	if t, ok := schema["type"].(string); ok && !schemaTypeMatches(t, value) {
		return fail("does not match schema type " + t)
	}
	if expected, ok := schema["const"]; ok && !jsonEqual(expected, value) {
		return fail("does not match const")
	}
	if raw, ok := schema["enum"]; ok {
		items, _ := array(raw)
		found := false
		key := jsonKey(value)
		for _, item := range items {
			if jsonKey(item) == key {
				found = true
				break
			}
		}
		if !found {
			return fail("does not match enum")
		}
	}
	if n, ok := numeric(value); ok {
		if raw, exists := schema["minimum"]; exists {
			bound, _ := numeric(raw)
			if n.Cmp(bound) < 0 {
				return fail("below minimum")
			}
		}
		if raw, exists := schema["maximum"]; exists {
			bound, _ := numeric(raw)
			if n.Cmp(bound) > 0 {
				return fail("above maximum")
			}
		}
	}
	if s, ok := value.(string); ok {
		var min, max *int
		if raw, exists := schema["minLength"]; exists {
			n, _ := schemaCount(raw)
			min = &n
		}
		if raw, exists := schema["maxLength"]; exists {
			n, _ := schemaCount(raw)
			max = &n
		}
		pattern, _ := schema["pattern"].(string)
		if err := validateString(s, min, max, pattern); err != nil {
			return fail(err.Error())
		}
	}
	if object, ok := value.(map[string]any); ok {
		properties, _ := schema["properties"].(map[string]any)
		if required, ok := array(schema["required"]); ok {
			for _, name := range required {
				if _, exists := object[name.(string)]; !exists {
					return fail("required property is missing")
				}
			}
		}
		for name, child := range object {
			if raw, exists := properties[name]; exists {
				if err := checkSchemaNode(raw.(map[string]any), child, path+"["+strconv.Quote(name)+"]", budget); err != nil {
					return err
				}
			} else if allow, exists := schema["additionalProperties"].(bool); exists && !allow {
				return fail("additional properties are forbidden")
			}
		}
	}
	if items, ok := array(value); ok {
		if raw, exists := schema["minItems"]; exists {
			min, _ := schemaCount(raw)
			if len(items) < min {
				return fail("too few items")
			}
		}
		if raw, exists := schema["maxItems"]; exists {
			max, _ := schemaCount(raw)
			if len(items) > max {
				return fail("too many items")
			}
		}
		if unique, _ := schema["uniqueItems"].(bool); unique {
			seen := make(map[string]bool, len(items))
			for _, item := range items {
				key := jsonKey(item)
				if seen[key] {
					return fail("duplicate items")
				}
				seen[key] = true
			}
		}
		if child, ok := schema["items"].(map[string]any); ok {
			for i, item := range items {
				if err := checkSchemaNode(child, item, fmt.Sprintf("%s[%d]", path, i), budget); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func schemaTypeMatches(t string, value any) bool {
	switch t {
	case "null":
		return value == nil
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := array(value)
		return ok
	case "number":
		_, ok := numeric(value)
		return ok
	case "integer":
		n, ok := numeric(value)
		return ok && n.IsInt()
	}
	return false
}

// jsonKey uses type tags, quoted strings, sorted keys and exact rational numbers
// for linear-space equality/uniqueItems (1 and 1.0 are the same JSON number).
func jsonKey(value any) string {
	var b strings.Builder
	var write func(any)
	write = func(value any) {
		if n, ok := numeric(value); ok {
			b.WriteString("n" + n.RatString() + ";")
			return
		}
		switch v := value.(type) {
		case nil:
			b.WriteString("z;")
		case bool:
			b.WriteString("b" + strconv.FormatBool(v) + ";")
		case string:
			b.WriteString("s" + strconv.Quote(v))
		case map[string]any:
			b.WriteString("{")
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				b.WriteString(strconv.Quote(key))
				write(v[key])
			}
			b.WriteString("}")
		default:
			if items, ok := array(v); ok {
				b.WriteString("[")
				for _, item := range items {
					write(item)
				}
				b.WriteString("]")
			}
		}
	}
	write(value)
	return b.String()
}

func jsonEqual(a, b any) bool { return jsonKey(a) == jsonKey(b) }
