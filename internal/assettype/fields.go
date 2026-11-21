package assettype

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type FieldType string

const (
	FieldText     FieldType = "text"
	FieldLongText FieldType = "long_text"
	FieldNumber   FieldType = "number"
	FieldDate     FieldType = "date"
	FieldURL      FieldType = "url"
	FieldBoolean  FieldType = "boolean"
	FieldSelect   FieldType = "select"
)

var FieldTypes = []FieldType{FieldText, FieldLongText, FieldNumber, FieldDate, FieldURL, FieldBoolean, FieldSelect}

func (f FieldType) Label() string {
	switch f {
	case FieldLongText:
		return "Long text"
	case FieldURL:
		return "URL"
	}
	s := string(f)
	return strings.ToUpper(s[:1]) + s[1:]
}

func ParseFieldType(s string) (FieldType, bool) {
	for _, t := range FieldTypes {
		if string(t) == s {
			return t, true
		}
	}
	return "", false
}

type Field struct {
	ID        string
	TypeID    string
	Key       string
	Label     string
	Type      FieldType
	Options   []string
	Required  bool
	SortOrder int
}

type Values map[string]any

func DecodeValues(raw []byte) Values {
	v := Values{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

func (v Values) Encode() []byte {
	if v == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func (v Values) Input(key string) string {
	val, ok := v[key]
	if !ok || val == nil {
		return ""
	}
	switch x := val.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return ""
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(val)
}

func (v Values) Display(f Field) string {
	val, ok := v[f.Key]
	if !ok || val == nil {
		return ""
	}
	switch f.Type {
	case FieldBoolean:
		if b, _ := val.(bool); b {
			return "Yes"
		}
		return "No"
	case FieldDate:
		if s, ok := val.(string); ok {
			if t, err := time.Parse("2006-01-02", s); err == nil {
				return t.Format("02 Jan 2006")
			}
		}
	}
	return v.Input(f.Key)
}

func ParseValues(fields []Field, get func(key string) (string, bool)) (Values, map[string]string) {
	out := Values{}
	errs := map[string]string{}
	for _, f := range fields {
		raw, present := get(f.Key)
		raw = strings.TrimSpace(raw)
		if f.Type == FieldBoolean {
			out[f.Key] = present && (raw == "true" || raw == "on" || raw == "1")
			continue
		}
		if raw == "" {
			if f.Required {
				errs[f.Key] = f.Label + " is required."
			}
			continue
		}
		switch f.Type {
		case FieldNumber:
			n, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				errs[f.Key] = f.Label + " must be a number."
				continue
			}
			out[f.Key] = n
		case FieldDate:
			if _, err := time.Parse("2006-01-02", raw); err != nil {
				errs[f.Key] = f.Label + " must be a date."
				continue
			}
			out[f.Key] = raw
		case FieldURL:
			if !ValidURL(raw) {
				errs[f.Key] = f.Label + " must be a valid http or https URL."
				continue
			}
			out[f.Key] = raw
		case FieldSelect:
			if !contains(f.Options, raw) {
				errs[f.Key] = f.Label + " must be one of the listed options."
				continue
			}
			out[f.Key] = raw
		default:
			out[f.Key] = raw
		}
	}
	return out, errs
}

func ValidURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func ParseOffsets(s string) ([]int32, error) {
	seen := map[int32]bool{}
	var out []int32
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > 3650 {
			return nil, fmt.Errorf("%q is not a positive whole number of days", part)
		}
		if seen[int32(n)] {
			return nil, fmt.Errorf("offset %d is listed twice", n)
		}
		seen[int32(n)] = true
		out = append(out, int32(n))
	}
	if len(out) == 0 {
		return nil, errors.New("at least one offset is required")
	}
	sort.Slice(out, func(i, j int) bool { return out[i] > out[j] })
	return out, nil
}

func FormatOffsets(o []int32) string {
	parts := make([]string, len(o))
	for i, v := range o {
		parts[i] = strconv.Itoa(int(v))
	}
	return strings.Join(parts, ", ")
}

func ParseOptions(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	return out
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func KeyFromLabel(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "_"), "_")
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func ValidKey(k string) bool { return keyPattern.MatchString(k) }
