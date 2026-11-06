package nodeopshttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// assertPageQueryRefused 断言这一问被拒成 400 MALFORMED_REQUEST，理由散文在 detail 里（ADR-0144 决定一、四），不带业务结果格。
func assertPageQueryRefused(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400；body = %s", response.Code, response.Body)
	}
	var body struct {
		Error struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem %s: %v", response.Body.Bytes(), err)
	}
	if body.Error.Code != "MALFORMED_REQUEST" || body.Error.Detail == "" {
		t.Fatalf("应答 MALFORMED_REQUEST 且带理由散文，得 %s", response.Body.Bytes())
	}
	assertNoOutcome(t, response)
	return body.Error.Detail
}

// pageOf 读出答复里的 page 一格；三格都必须在场。
func pageOf(t *testing.T, response *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var body struct {
		Page map[string]json.RawMessage `json:"page"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	for _, key := range []string{"size", "next", "total"} {
		if _, present := body.Page[key]; !present {
			t.Fatalf("page 缺 %s：%s", key, response.Body.Bytes())
		}
	}
	return body.Page
}
