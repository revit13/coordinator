/*
Copyright 2026 The llm-d Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gateway

import (
	"fmt"
	"strings"
	"testing"
)

const longString = "this string is definitely longer than the fifty character redaction limit"

func TestRedactBody_RedactsLongStringsByPrefix(t *testing.T) {
	body := fmt.Sprintf(`{"image":"data:image/png;base64,%s","ref":"https://example.com/%s","blob":"%s","short":"keep"}`,
		longString, longString, longString)

	out, ok := RedactBody([]byte(body)).(map[string]any)
	if !ok {
		t.Fatalf("RedactBody returned %T, want map", RedactBody([]byte(body)))
	}

	if out["image"] != "[base64]" {
		t.Errorf("image = %v, want [base64]", out["image"])
	}
	if out["ref"] != "[url]" {
		t.Errorf("ref = %v, want [url]", out["ref"])
	}
	if out["blob"] != "..." {
		t.Errorf("blob = %v, want ...", out["blob"])
	}
	if out["short"] != "keep" {
		t.Errorf("short = %v, want keep (under limit, unredacted)", out["short"])
	}
}

func TestRedactBody_TruncatesLongArrays(t *testing.T) {
	ids := make([]string, 0, 25)
	for i := 0; i < 25; i++ {
		ids = append(ids, fmt.Sprintf("%d", i))
	}
	body := fmt.Sprintf(`{"token_ids":[%s]}`, strings.Join(ids, ","))

	out := RedactBody([]byte(body)).(map[string]any)
	arr, ok := out["token_ids"].([]any)
	if !ok {
		t.Fatalf("token_ids = %T, want []any", out["token_ids"])
	}
	// 10 kept elements plus the "... +N more" marker.
	if len(arr) != 11 {
		t.Fatalf("len(token_ids) = %d, want 11", len(arr))
	}
	if last, _ := arr[10].(string); !strings.Contains(last, "+15 more") {
		t.Errorf("last element = %v, want it to mention +15 more", arr[10])
	}
}

func TestRedactBody_NonJSONFallback(t *testing.T) {
	short := RedactBody([]byte("not json"))
	if short != "not json" {
		t.Errorf("short non-JSON = %v, want passthrough", short)
	}

	raw := strings.Repeat("x", 250)
	long, ok := RedactBody([]byte(raw)).(string)
	if !ok {
		t.Fatalf("long non-JSON returned %T, want string", RedactBody([]byte(raw)))
	}
	if !strings.HasSuffix(long, "...") || len(long) != 203 {
		t.Errorf("long non-JSON = %q (len %d), want 200 chars + \"...\"", long, len(long))
	}
}
