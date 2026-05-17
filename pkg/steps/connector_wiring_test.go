package steps

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/llm-d/coordinator/pkg/config"
	"github.com/llm-d/coordinator/pkg/gateway"
	"github.com/llm-d/coordinator/pkg/pipeline"
)

// TestPrefillStep_ConnectorShapesPrefillBody verifies that the connector
// selected via params controls the kv_transfer_params shape on the prefill
// request. nixlv2 emits the full six-field map; shared_storage emits only do_remote_decode.
func TestPrefillStep_ConnectorShapesPrefillBody(t *testing.T) {
	cases := []struct {
		connector  string
		wantFields []string // must be present
		denyFields []string // must NOT be present
	}{
		{
			connector:  "nixlv2",
			wantFields: []string{"do_remote_decode", "do_remote_prefill"},
			denyFields: []string{"remote_engine_id", "remote_host"},
		},
		{
			connector:  "shared_storage",
			wantFields: []string{"do_remote_decode"},
			denyFields: []string{"remote_engine_id", "remote_host"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.connector, func(t *testing.T) {
			var captured map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var parsed map[string]any
				_ = json.Unmarshal(body, &parsed)
				captured, _ = parsed["kv_transfer_params"].(map[string]any)
				_ = json.NewEncoder(w).Encode(map[string]any{"kv_transfer_params": map[string]any{}})
			}))
			defer srv.Close()

			gwClient := gateway.New(config.GatewayConfig{Address: srv.URL})
			step, err := NewPrefillStep(map[string]any{
				"gateway_path": "/inference/v1/generate",
				"kv_connector": tc.connector,
			})
			if err != nil {
				t.Fatalf("NewPrefillStep: %v", err)
			}
			step.(*PrefillStep).SetGatewayClient(gwClient)

			reqCtx := &pipeline.RequestContext{
				RequestID:        "req",
				Model:            "m",
				TokenIDs:         []int{1, 2},
				KVTransferParams: make(map[string]any),
			}
			if err := step.Execute(context.Background(), reqCtx); err != nil {
				t.Fatalf("Execute: %v", err)
			}

			if captured == nil {
				t.Fatal("kv_transfer_params not sent to gateway")
			}
			for _, f := range tc.wantFields {
				if _, ok := captured[f]; !ok {
					t.Errorf("missing field %q; got %v", f, captured)
				}
			}
			for _, f := range tc.denyFields {
				if _, ok := captured[f]; ok {
					t.Errorf("unexpected field %q; got %v", f, captured)
				}
			}
		})
	}
}

// TestDecodeStep_ConnectorShapesDecodeBody verifies the per-connector
// kv_transfer_params shape sent on the decode request. nixlv2 forwards the
// prefill response verbatim plus do_remote_prefill: true; the others emit
// only do_remote_prefill: true.
func TestDecodeStep_ConnectorShapesDecodeBody(t *testing.T) {
	cases := []struct {
		connector  string
		wantFields map[string]any
		denyFields []string
	}{
		{
			connector: "nixlv2",
			wantFields: map[string]any{
				"do_remote_prefill": true,
				"block_id":          "from-prefill",
			},
		},
		{
			connector:  "shared_storage",
			wantFields: map[string]any{"do_remote_prefill": true},
			denyFields: []string{"block_id"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.connector, func(t *testing.T) {
			var captured map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var parsed map[string]any
				_ = json.Unmarshal(body, &parsed)
				captured, _ = parsed["kv_transfer_params"].(map[string]any)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "ok"}}},
				})
			}))
			defer srv.Close()

			gwClient := gateway.New(config.GatewayConfig{Address: srv.URL})
			step, err := NewDecodeStep(map[string]any{"kv_connector": tc.connector})
			if err != nil {
				t.Fatalf("NewDecodeStep: %v", err)
			}
			step.(*DecodeStep).SetGatewayClient(gwClient)

			recorder := httptest.NewRecorder()
			reqCtx := &pipeline.RequestContext{
				RequestID:        "req",
				OriginalPath:     "/v1/chat/completions",
				Model:            "m",
				KVTransferParams: map[string]any{"block_id": "from-prefill"},
				Body:             map[string]any{"model": "m"},
				ResponseWriter:   recorder,
				Flusher:          recorder,
			}
			if err := step.Execute(context.Background(), reqCtx); err != nil {
				t.Fatalf("Execute: %v", err)
			}

			if captured == nil {
				t.Fatal("kv_transfer_params not sent to gateway")
			}
			for k, want := range tc.wantFields {
				if got := captured[k]; got != want {
					t.Errorf("field %q = %v, want %v", k, got, want)
				}
			}
			for _, f := range tc.denyFields {
				if _, ok := captured[f]; ok {
					t.Errorf("unexpected field %q; got %v", f, captured)
				}
			}
		})
	}
}

func TestPrefillStep_UnknownConnectorRejected(t *testing.T) {
	if _, err := NewPrefillStep(map[string]any{"kv_connector": "bogus"}); err == nil {
		t.Fatal("expected error for unknown connector")
	}
}

func TestDecodeStep_UnknownConnectorRejected(t *testing.T) {
	if _, err := NewDecodeStep(map[string]any{"kv_connector": "bogus"}); err == nil {
		t.Fatal("expected error for unknown connector")
	}
}
