package oapigen

import (
	"strconv"
	"strings"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// numberKind tells boundKeywords whether the field counts in whole numbers,
// where an exclusive bound has an inclusive equivalent one step away.
type numberKind int

const (
	wholeNumber numberKind = iota
	fractional
)

// boundedTypes are the numeric rule prefixes whose bounds become JSON numbers.
//
// The 64-bit integers are absent on purpose: JSON carries them as strings, and
// protoc-gen-openapiv2 types them `{"type": "string", "format": "int64"}`, where
// a `minimum` describes nothing. `bytes` is absent for the same reason — its
// length rules count raw bytes, not the base64 the client sends.
var boundedTypes = map[string]numberKind{
	"int32": wholeNumber, "sint32": wholeNumber, "sfixed32": wholeNumber,
	"uint32": wholeNumber, "fixed32": wholeNumber,
	"float": fractional, "double": fractional,
}

// stringFormats are the string rules that name a JSONSchema `format`.
var stringFormats = map[string]string{
	"uuid":     "uuid",
	"email":    "email",
	"uri":      "uri",
	"hostname": "hostname",
	"ipv4":     "ipv4",
	"ipv6":     "ipv6",
}

// keyword is one JSONSchema keyword and its value, already rendered as YAML.
type keyword struct {
	key   string
	value string
}

// ruleKeywords maps one protovalidate rule onto JSONSchema keywords. A rule
// with no equivalent yields none: under-narrowing is the safe direction.
func ruleKeywords(rule parser.Rule) []keyword {
	prefix, leaf, ok := strings.Cut(strings.TrimPrefix(rule.Kind, repeatedItems), ".")
	if !ok {
		return nil
	}

	if kind, ok := boundedTypes[prefix]; ok {
		return boundKeywords(leaf, rule.Value, kind)
	}

	switch {
	case prefix == "string":
		return stringKeywords(leaf, rule.Value)
	case prefix == "repeated":
		return repeatedKeywords(leaf, rule.Value)
	case prefix == "map":
		return mapKeywords(leaf, rule.Value)
	}
	return nil
}

// boundKeywords maps one numeric bound onto `minimum` or `maximum`.
//
// A whole-number exclusive bound is rewritten as the inclusive bound one step
// away — `gt: 0` is `minimum: 1` — which keeps it out of the zero that
// jsonNumber has to drop, and keeps draft-4's `exclusiveMinimum` out of the
// output for every integer field.
func boundKeywords(leaf, value string, kind numberKind) []keyword {
	if kind == wholeNumber {
		switch leaf {
		case "gt":
			if shifted, ok := shiftedBound(value, 1); ok {
				leaf, value = "gte", shifted
			}
		case "lt":
			if shifted, ok := shiftedBound(value, -1); ok {
				leaf, value = "lte", shifted
			}
		}
	}

	number, ok := jsonNumber(value)
	if !ok {
		return nil
	}
	switch leaf {
	case "gte":
		return []keyword{{"minimum", number}}
	case "gt":
		return []keyword{{"minimum", number}, {"exclusiveMinimum", "true"}}
	case "lte":
		return []keyword{{"maximum", number}}
	case "lt":
		return []keyword{{"maximum", number}, {"exclusiveMaximum", "true"}}
	}
	return nil
}

func stringKeywords(leaf, value string) []keyword {
	if format, ok := stringFormats[leaf]; ok && value == "true" {
		return []keyword{{"format", strconv.Quote(format)}}
	}
	switch leaf {
	case "len":
		return []keyword{{"maxLength", value}, {"minLength", value}}
	case "max_len":
		return []keyword{{"maxLength", value}}
	case "min_len":
		return []keyword{{"minLength", value}}
	case "pattern":
		// A RE2 source string, straight from the proto: quote it.
		return []keyword{{"pattern", strconv.Quote(value)}}
	}
	return nil
}

func repeatedKeywords(leaf, value string) []keyword {
	switch leaf {
	case "max_items":
		return []keyword{{"maxItems", value}}
	case "min_items":
		return []keyword{{"minItems", value}}
	case "unique":
		if value == "true" {
			return []keyword{{"uniqueItems", "true"}}
		}
	}
	return nil
}

func mapKeywords(leaf, value string) []keyword {
	switch leaf {
	case "max_pairs":
		return []keyword{{"maxProperties", value}}
	case "min_pairs":
		return []keyword{{"minProperties", value}}
	}
	return nil
}

// shiftedBound turns an exclusive whole-number bound into the inclusive one a
// step away. It reports false when the parser did not print an integer, which
// leaves the exclusive form in place.
func shiftedBound(value string, step int64) (string, bool) {
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return "", false
	}
	return strconv.FormatInt(number+step, 10), true
}

// jsonNumber re-renders a bound as a plain decimal. The parser hands back
// whatever protoreflect printed, and a float in exponent form ("1e+06") is not
// a number to the YAML parser grpc-gateway loads the config with.
//
// A zero is reported absent. grpc-gateway's swagger writer tags `minimum` and
// `maximum` `omitempty`, so a zero bound never reaches the output — and what it
// leaves behind, an `exclusiveMinimum: true` with no `minimum`, describes
// nothing at all.
func jsonNumber(value string) (string, bool) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || number == 0 {
		return "", false
	}
	return strconv.FormatFloat(number, 'f', -1, 64), true
}
