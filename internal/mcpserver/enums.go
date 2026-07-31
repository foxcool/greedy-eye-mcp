package mcpserver

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// enumAliases maps caller words that are not enum members onto the member they
// mean, keyed by enum full name then lowercase alias.
//
// ETF is deliberately not a proto value: an ETF is a fund by construction and
// the exchange-traded part is already carried by the asset's market.
var enumAliases = map[protoreflect.FullName]map[string]string{
	"eye.v1.AssetType": {"etf": "ASSET_TYPE_FUND"},
}

// resolveEnum maps a caller-supplied string onto an enum member. The full member
// name (ASSET_TYPE_FUND), the short form (fund) and registered aliases (etf) are
// all accepted, case-insensitively. Anything else fails with an error naming the
// field and listing every value the caller may use — guessing a member from an
// enum the caller cannot see is not a reasonable ask.
func resolveEnum(ed protoreflect.EnumDescriptor, field, raw string) (protoreflect.EnumValueDescriptor, error) {
	trimmed := strings.TrimSpace(raw)
	key := strings.ToUpper(strings.NewReplacer(" ", "_", "-", "_").Replace(trimmed))

	candidates := make([]string, 0, 3)
	if alias, ok := enumAliases[ed.FullName()][strings.ToLower(trimmed)]; ok {
		candidates = append(candidates, alias)
	}
	candidates = append(candidates, key, enumPrefix(ed)+key)

	for _, name := range candidates {
		if v := ed.Values().ByName(protoreflect.Name(name)); v != nil && v.Number() != 0 {
			return v, nil
		}
	}
	return nil, fmt.Errorf("invalid value %q for %s: expected one of %s",
		raw, field, strings.Join(enumChoices(ed), ", "))
}

// enumValue resolves a tool string parameter to its enum number. An empty string
// yields the zero value: the parameter is simply absent. An unknown value is an
// error, never a silent fallback to the zero value — the backend reads that zero
// as "default" and would create something under the wrong identity.
func enumValue[E ~int32](ed protoreflect.EnumDescriptor, field, raw string) (E, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	v, err := resolveEnum(ed, field, raw)
	if err != nil {
		return 0, err
	}
	return E(v.Number()), nil
}

// normalizeEnumFields rewrites enum-typed string fields of a JSON object to their
// canonical member names, so short forms and aliases survive protojson's strict
// matching. Only top-level fields are walked: the import item types carry no
// nested messages holding enums.
func normalizeEnumFields(md protoreflect.MessageDescriptor, raw json.RawMessage) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw, nil // not an object: let protojson report the real problem
	}

	changed := false
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if fd.Kind() != protoreflect.EnumKind || fd.IsList() || fd.IsMap() {
			continue
		}
		key, ok := jsonKey(obj, fd)
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(obj[key], &s); err != nil {
			continue // numbers and nulls are protojson's business
		}
		v, err := resolveEnum(fd.Enum(), string(fd.Name()), s)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(string(v.Name()))
		if err != nil {
			return nil, err
		}
		obj[key] = encoded
		changed = true
	}
	if !changed {
		return raw, nil
	}
	return json.Marshal(obj)
}

// jsonKey finds the field in a decoded object under either the proto name
// (asset_type) or the JSON name (assetType); protojson accepts both.
func jsonKey(obj map[string]json.RawMessage, fd protoreflect.FieldDescriptor) (string, bool) {
	for _, name := range []string{string(fd.Name()), fd.JSONName()} {
		if _, ok := obj[name]; ok {
			return name, true
		}
	}
	return "", false
}

// enumPrefix returns the shared member prefix ("ASSET_TYPE_"), taken from the
// zero value, which protobuf style names <ENUM>_UNSPECIFIED.
func enumPrefix(ed protoreflect.EnumDescriptor) string {
	zero := ed.Values().ByNumber(0)
	if zero == nil {
		return ""
	}
	name := string(zero.Name())
	if i := strings.LastIndex(name, "_"); i >= 0 {
		return name[:i+1]
	}
	return ""
}

// enumChoices lists the short forms a caller may pass, aliases included. The zero
// value is left out: UNSPECIFIED means "not given", not a selectable value.
func enumChoices(ed protoreflect.EnumDescriptor) []string {
	prefix := enumPrefix(ed)
	vals := ed.Values()
	out := make([]string, 0, vals.Len())
	for i := range vals.Len() {
		v := vals.Get(i)
		if v.Number() == 0 {
			continue
		}
		out = append(out, strings.ToLower(strings.TrimPrefix(string(v.Name()), prefix)))
	}

	aliases := make([]string, 0, len(enumAliases[ed.FullName()]))
	for alias := range enumAliases[ed.FullName()] {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases) // map order is random; error text must not be
	return append(out, aliases...)
}
