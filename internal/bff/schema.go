package bff

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/leohteixeira/advisor-radar/internal/screen"
)

// schemaHeader is the optional screen request header with the range of
// schema_version values the client renders.
const schemaHeader = "X-SDUI-Schema"

// errSchemaRange marks a malformed X-SDUI-Schema value or an invalid range.
var errSchemaRange = errors.New("bff: invalid schema range")

// schemaRange is an inclusive range of screen schema_version values.
type schemaRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// contains reports whether v is inside the range.
func (r schemaRange) contains(v int) bool { return r.Min <= v && v <= r.Max }

// supportedSchema is the range the BFF serves: only its current version,
// never a lower one.
var supportedSchema = schemaRange{Min: screen.SchemaVersion, Max: screen.SchemaVersion}

// parseSchemaRange parses an X-SDUI-Schema value after trimming the
// whitespace around it: one integer ("1") or an inclusive range ("1-2"),
// written with digits only and with 1 ≤ min ≤ max. Anything else wraps
// errSchemaRange.
func parseSchemaRange(value string) (schemaRange, error) {
	lo, hi, isRange := strings.Cut(strings.TrimSpace(value), "-")
	if !isRange {
		hi = lo
	}
	minimum, err := schemaNumber(lo)
	if err != nil {
		return schemaRange{}, err
	}
	maximum, err := schemaNumber(hi)
	if err != nil {
		return schemaRange{}, err
	}
	if minimum > maximum {
		return schemaRange{}, fmt.Errorf("%w: min %d is above max %d", errSchemaRange, minimum, maximum)
	}
	return schemaRange{Min: minimum, Max: maximum}, nil
}

// schemaNumber parses one bound: a decimal integer of at least 1, with no
// sign and no inner space.
func schemaNumber(s string) (int, error) {
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return 0, fmt.Errorf("%w: %q is not a version", errSchemaRange, s)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errSchemaRange, err)
	}
	if n < 1 {
		return 0, fmt.Errorf("%w: version %d is below 1", errSchemaRange, n)
	}
	return n, nil
}

// acceptSchema applies X-SDUI-Schema to a screen request before anything is
// fetched and reports whether the screen may be served. Without the header
// the current version is served. A malformed header, a repeated one, or an
// invalid range answers 400 {"error":"invalid_schema_range"}; a range that
// excludes the current version answers 406 with the supported range.
func acceptSchema(w http.ResponseWriter, r *http.Request) bool {
	values := r.Header.Values(schemaHeader)
	if len(values) == 0 {
		return true
	}
	accepted, err := parseSchemaRange(values[0])
	if err != nil || len(values) > 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_schema_range"})
		return false
	}
	if !accepted.contains(screen.SchemaVersion) {
		writeJSON(w, http.StatusNotAcceptable, map[string]schemaRange{"supported": supportedSchema})
		return false
	}
	return true
}
