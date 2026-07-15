package main

import (
	"encoding/json"
	"testing"
)

func TestCloud115ResponseOK(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]interface{}
		want bool
	}{
		{name: "state true", raw: map[string]interface{}{"state": true}, want: true},
		{name: "errno zero number", raw: map[string]interface{}{"errno": float64(0)}, want: true},
		{name: "errno zero text", raw: map[string]interface{}{"errno": "0"}, want: true},
		{name: "state false", raw: map[string]interface{}{"state": false}, want: false},
		{name: "missing state keeps compatibility", raw: map[string]interface{}{}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cloud115ResponseOK(tc.raw); got != tc.want {
				t.Fatalf("cloud115ResponseOK() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCloud115JSONHelpers(t *testing.T) {
	if got := jsonNumberToInt64(json.Number("42")); got != 42 {
		t.Fatalf("jsonNumberToInt64(json.Number) = %d, want 42", got)
	}
	if got := jsonNumberToInt64("17"); got != 17 {
		t.Fatalf("jsonNumberToInt64(string) = %d, want 17", got)
	}
	if !jsonBool("true") {
		t.Fatalf("jsonBool(true string) = false, want true")
	}
	if got := jsonString(float64(12)); got != "12" {
		t.Fatalf("jsonString(float64) = %q, want 12", got)
	}
	if got := firstJSONText(map[string]interface{}{"b": "fallback", "a": "primary"}, "a", "b"); got != "primary" {
		t.Fatalf("firstJSONText() = %q, want primary", got)
	}
}

func TestCloud115LifeEventName(t *testing.T) {
	if got := cloud115LifeEventName(1); got != "upload_image_file" {
		t.Fatalf("cloud115LifeEventName(1) = %q, want upload_image_file", got)
	}
	if got := cloud115LifeEventName(999); got != "" {
		t.Fatalf("cloud115LifeEventName(999) = %q, want empty string", got)
	}
}
