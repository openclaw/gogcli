package cmd

import (
	"fmt"
	"reflect"
	"strings"

	"google.golang.org/api/sheets/v4"
)

const sheetsUserEnteredFormatPrefix = "userEnteredFormat"

func normalizeFormatMask(mask string) (string, []string) {
	parts := splitFieldMask(mask)
	if len(parts) == 0 {
		return "", nil
	}

	normalized := make([]string, 0, len(parts))
	formatJSONPaths := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}

		switch {
		case part == sheetsUserEnteredFormatPrefix:
			normalized = append(normalized, part)
		case strings.HasPrefix(part, sheetsUserEnteredFormatPrefix+"."):
			formatPath := strings.TrimPrefix(part, sheetsUserEnteredFormatPrefix+".")
			normalized = append(normalized, part)
			if formatPath != "" {
				formatJSONPaths = append(formatJSONPaths, formatPath)
			}
		default:
			if isFormatJSONPath(part) {
				normalized = append(normalized, sheetsUserEnteredFormatPrefix+"."+part)
				formatJSONPaths = append(formatJSONPaths, part)
			} else {
				normalized = append(normalized, part)
			}
		}
	}

	return strings.Join(normalized, ","), formatJSONPaths
}

func applyForceSendFields(format *sheets.CellFormat, formatPaths []string) error {
	if format == nil {
		return fmt.Errorf("format is required")
	}

	for _, path := range formatPaths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if err := forceSendJSONField(format, path); err != nil {
			return fmt.Errorf("invalid format field %q: %w", path, err)
		}
	}
	return nil
}

func splitFieldMask(mask string) []string {
	if strings.TrimSpace(mask) == "" {
		return nil
	}
	parts := strings.Split(mask, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func isFormatJSONPath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	var format sheets.CellFormat
	return forceSendJSONField(&format, path) == nil
}

func forceSendJSONField(root any, jsonPath string) error {
	parent, fieldValue, fieldName, err := resolveJSONField(root, jsonPath)
	if err != nil {
		return err
	}
	if fieldValue.Kind() == reflect.Pointer && fieldValue.IsNil() && fieldValue.Type().Elem().Kind() == reflect.Struct {
		fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
	}
	return addForceSendField(parent, fieldName)
}

func findJSONField(v reflect.Value, jsonName string) (reflect.Value, string, bool) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			continue
		}
		if name == jsonName {
			return v.Field(i), field.Name, true
		}
	}
	return reflect.Value{}, "", false
}

func addForceSendField(v reflect.Value, fieldName string) error {
	fs := v.FieldByName("ForceSendFields")
	if !fs.IsValid() {
		return fmt.Errorf("missing ForceSendFields")
	}
	if fs.Kind() != reflect.Slice || fs.Type().Elem().Kind() != reflect.String {
		return fmt.Errorf("invalid ForceSendFields")
	}
	for i := 0; i < fs.Len(); i++ {
		if fs.Index(i).String() == fieldName {
			return nil
		}
	}
	fs.Set(reflect.Append(fs, reflect.ValueOf(fieldName)))
	return nil
}

func resolveJSONField(root any, jsonPath string) (reflect.Value, reflect.Value, string, error) {
	current := reflect.ValueOf(root)
	if current.Kind() != reflect.Pointer || current.IsNil() {
		return reflect.Value{}, reflect.Value{}, "", fmt.Errorf("format must be a non-nil pointer")
	}

	parts := strings.Split(jsonPath, ".")
	for i, part := range parts {
		structValue, err := ensureStructValue(current, part)
		if err != nil {
			return reflect.Value{}, reflect.Value{}, "", err
		}

		fieldValue, fieldName, ok := findJSONField(structValue, part)
		if !ok {
			return reflect.Value{}, reflect.Value{}, "", fmt.Errorf("unknown field %q", part)
		}
		if i == len(parts)-1 {
			return structValue, fieldValue, fieldName, nil
		}

		next, err := nextStructPointer(fieldValue, part)
		if err != nil {
			return reflect.Value{}, reflect.Value{}, "", err
		}
		current = next
	}

	return reflect.Value{}, reflect.Value{}, "", fmt.Errorf("empty format field")
}

func ensureStructValue(value reflect.Value, label string) (reflect.Value, error) {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			if value.Type().Elem().Kind() != reflect.Struct {
				return reflect.Value{}, fmt.Errorf("field %q is not a struct", label)
			}
			value.Set(reflect.New(value.Type().Elem()))
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("field %q is not a struct", label)
	}
	return value, nil
}

func nextStructPointer(value reflect.Value, label string) (reflect.Value, error) {
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			if value.Type().Elem().Kind() != reflect.Struct {
				return reflect.Value{}, fmt.Errorf("field %q is not a struct", label)
			}
			value.Set(reflect.New(value.Type().Elem()))
		}
		return value, nil
	case reflect.Struct:
		if !value.CanAddr() {
			return reflect.Value{}, fmt.Errorf("field %q is not addressable", label)
		}
		return value.Addr(), nil
	default:
		return reflect.Value{}, fmt.Errorf("field %q is not a struct", label)
	}
}
