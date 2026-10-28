package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 同一个幂等键再到给同一个消息号：连接器在提交失败之后重发，服务商要认得出是同一次尝试。
func TestTheBrokerAnswersAStableMessageIDPerIdempotencyKey(t *testing.T) {
	ask := func(key string) map[string]string {
		request := httptest.NewRequest(http.MethodPost, "/declarations", strings.NewReader(`{"version":"DECLV-1"}`))
		request.Header.Set("Idempotency-Key", key)
		recorder := httptest.NewRecorder()
		handle(recorder, request)
		var answer map[string]string
		_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
		return answer
	}
	first, again, other := ask("SYN-TENANT-01/DECLV-1/1"), ask("SYN-TENANT-01/DECLV-1/1"), ask("SYN-TENANT-01/DECLV-1/2")
	if first["outcome"] != "ACCEPTED" || first["messageId"] != again["messageId"] || first["messageId"] == other["messageId"] {
		t.Fatalf("first=%v again=%v other=%v", first, again, other)
	}
	if ask("")["outcome"] != "REJECTED" {
		t.Fatal("没有幂等键应拒收")
	}
}
