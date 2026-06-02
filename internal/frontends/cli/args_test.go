package cli

import (
	"reflect"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestParseFlags_NilSchema_NoArgs_ReturnsNil(t *testing.T) {
	got, err := parseFlags(nil, nil)
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("parseFlags = %v, want nil", got)
	}
}

func TestParseFlags_NilSchema_WithArgs_Errors(t *testing.T) {
	if _, err := parseFlags(nil, []string{"--body", "hi"}); err == nil {
		t.Fatal("parseFlags returned nil, want error")
	}
}

func TestParseFlags_MapsKebabFlagsToSnakeKeys(t *testing.T) {
	schema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"page_id":         {Type: "string"},
			"ticket":          {Type: "string"},
			"attachment_type": {Type: "string"},
		},
	}
	got, err := parseFlags(schema, []string{
		"--page-id", "12345",
		"--ticket", "JIRA-1",
		"--attachment-type", "markdown",
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	want := map[string]any{
		"page_id":         "12345",
		"ticket":          "JIRA-1",
		"attachment_type": "markdown",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseFlags = %v, want %v", got, want)
	}
}

func TestParseFlags_CoercesTypedValues(t *testing.T) {
	schema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"limit":   {Type: "integer"},
			"score":   {Type: "number"},
			"verbose": {Type: "boolean"},
		},
	}
	got, err := parseFlags(schema, []string{
		"--limit", "42",
		"--score", "1.5",
		"--verbose", "true",
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if got["limit"] != int64(42) {
		t.Errorf("limit = %v (%T), want int64(42)", got["limit"], got["limit"])
	}
	if got["score"] != 1.5 {
		t.Errorf("score = %v, want 1.5", got["score"])
	}
	if got["verbose"] != true {
		t.Errorf("verbose = %v, want true", got["verbose"])
	}
}

func TestParseFlags_UnknownFlag_Errors(t *testing.T) {
	schema := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{"body": {Type: "string"}},
	}
	if _, err := parseFlags(schema, []string{"--nope", "x"}); err == nil {
		t.Fatal("parseFlags returned nil, want error")
	}
}

func TestParseFlags_MissingValue_Errors(t *testing.T) {
	schema := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{"body": {Type: "string"}},
	}
	if _, err := parseFlags(schema, []string{"--body"}); err == nil {
		t.Fatal("parseFlags returned nil, want error")
	}
}

func TestParseFlags_NonFlagToken_Errors(t *testing.T) {
	schema := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{"body": {Type: "string"}},
	}
	if _, err := parseFlags(schema, []string{"positional"}); err == nil {
		t.Fatal("parseFlags returned nil, want error")
	}
}
