package visibilityhttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// assertPageQueryRefused 断言这一问被拒成 400 MALFORMED_REQUEST，理由散文在 detail 里（ADR-0144 决定一、四），
// 且答复只有问题体、不带任何行。
func assertPageQueryRefused(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400；body = %s", response.Code, response.Body)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode problem %s: %v", response.Body.Bytes(), err)
	}
	if _, present := fields["error"]; !present || len(fields) != 1 {
		t.Fatalf("拒绝答复应只有问题体：%s", response.Body.Bytes())
	}
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(fields["error"], &problem); err != nil {
		t.Fatalf("decode problem %s: %v", response.Body.Bytes(), err)
	}
	if problem.Code != "MALFORMED_REQUEST" || problem.Detail == "" {
		t.Fatalf("应答 MALFORMED_REQUEST 且带理由散文，得 %s", response.Body.Bytes())
	}
	return problem.Detail
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
