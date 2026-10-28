package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	pidManualResultURL = "/post-intake-disposition-manual-item-results"
	pidManualItemsURL  = "/post-intake-disposition-manual-items"
)

func (fixture *postIntakeE2E) manualPost(token, body string) (int, string) {
	request := httptest.NewRequest(http.MethodPost, pidManualResultURL, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.endpoints[pidManualResultURL].ServeHTTP(response, request)
	return response.Code, response.Body.String()
}

func (fixture *postIntakeE2E) manualGet(token, query string) (int, string) {
	request := httptest.NewRequest(http.MethodGet, pidManualItemsURL+query, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.endpoints[pidManualItemsURL].ServeHTTP(response, request)
	return response.Code, response.Body.String()
}

func manualResultBody(parcel string, version int, owner, action, outcome, note string) string {
	return `{"parcel":"` + parcel + `","decisionVersion":` + itoa(version) + `,"owner":"` + owner + `","action":"` + action +
		`","outcome":"` + outcome + `","note":"` + note + `"}`
}

// Covers: AT-PS-088 的人工项半边；ADR-0340 决定二、三——一个「拦截」决定同笔落两项待人工处理项（NO 扣住、TF 扣住）：清单可查，只持决定授予的人登记不了（403），
// 持登记授予的人逐项登记（201），结果取自认证的操作者；重放答 200，同项异内容答 409 且不覆盖；系统执行的动作、不在册的项、他租户的项各得 400 与统一的 404；按包裹读回带结果。
func TestRegisteringManualItemResultsIsAuthorizedIdempotentAndReadBackPerParcel(t *testing.T) {
	fixture := newPostIntakeE2E(t)
	fixture.arrangeCrossed(t, "SYN-PARCEL-M1", "cancel-key-m1")
	if status, raw := fixture.post(pidDecider, pidBody("SYN-PARCEL-M1", 0, "INTERCEPT", "cancel-key-m1", "")); status != http.StatusCreated {
		t.Fatalf("决定 = %d %s", status, raw)
	}

	status, raw := fixture.manualGet(pidDecider, "")
	var listed struct {
		PendingCount int `json:"pendingCount"`
		Items        []struct {
			Owner, Action string
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &listed); err != nil || status != http.StatusOK || listed.PendingCount != 2 || len(listed.Items) != 2 {
		t.Fatalf("清单 = %d %s, want 200 与 2 项", status, raw)
	}

	body := manualResultBody("SYN-PARCEL-M1", 1, "NODE_OPERATIONS", "HOLD_AT_CONTROL", "HANDLED", "已在控制处扣住")
	if status, _ := fixture.manualPost(pidDecider, body); status != http.StatusForbidden {
		t.Fatalf("只持决定授予的人登记 = %d, want 403", status)
	}
	if status, raw := fixture.manualPost(pidHandler, body); status != http.StatusCreated || !strings.Contains(raw, `"REGISTERED"`) {
		t.Fatalf("首次登记 = %d %s, want 201 REGISTERED", status, raw)
	}
	var handledBy string
	if err := fixture.pool.QueryRow(t.Context(), `SELECT handled_by FROM parcel_shipment.post_intake_disposition_manual_result`).Scan(&handledBy); err != nil ||
		handledBy != customsDecisionIssuer+"#"+pidHandler {
		t.Fatalf("操作者留痕 = %q (%v), want 认证出的「发行方#sub」", handledBy, err)
	}
	if status, raw := fixture.manualPost(pidHandler, body); status != http.StatusOK || !strings.Contains(raw, `"ALREADY_REGISTERED"`) {
		t.Fatalf("重放 = %d %s, want 200 ALREADY_REGISTERED", status, raw)
	}
	other := manualResultBody("SYN-PARCEL-M1", 1, "NODE_OPERATIONS", "HOLD_AT_CONTROL", "UNABLE", "改口")
	if status, raw := fixture.manualPost(pidHandler, other); status != http.StatusConflict || !strings.Contains(raw, `"RESULT_CONFLICT"`) || !strings.Contains(raw, "已在控制处扣住") {
		t.Fatalf("异内容 = %d %s, want 409 且读回先前结果", status, raw)
	}
	if status, _ := fixture.manualPost(pidHandler, manualResultBody("SYN-PARCEL-M1", 1, "TRANSPORT_FULFILLMENT", "START_RETURN_JOURNEY", "HANDLED", "x")); status != http.StatusBadRequest {
		t.Fatalf("系统执行的动作 = %d, want 400", status)
	}
	if status, _ := fixture.manualPost(pidHandler, manualResultBody("SYN-PARCEL-M1", 2, "NODE_OPERATIONS", "HOLD_AT_CONTROL", "HANDLED", "x")); status != http.StatusNotFound {
		t.Fatalf("不在册的项 = %d, want 404", status)
	}
	if status, _ := fixture.manualPost(pidForeignHandler, body); status != http.StatusNotFound {
		t.Fatalf("他租户登记 = %d, want 404 统一不可见", status)
	}
	if status, _ := fixture.manualPost(pidHandler, manualResultBody("SYN-PARCEL-M1", 1, "TRANSPORT_FULFILLMENT", "HOLD_ONWARD_MOVEMENT", "UNABLE", "承运商已提货")); status != http.StatusCreated {
		t.Fatalf("第二项登记 = %d, want 201", status)
	}

	if status, raw = fixture.manualGet(pidDecider, ""); status != http.StatusOK || !strings.Contains(raw, `"pendingCount":0`) {
		t.Fatalf("登记后清单 = %d %s, want 待处理 0", status, raw)
	}
	if status, raw = fixture.manualGet(pidDecider, "?parcel=SYN-PARCEL-M1"); status != http.StatusOK ||
		!strings.Contains(raw, "承运商已提货") || !strings.Contains(raw, "已在控制处扣住") || !strings.Contains(raw, `"UNABLE"`) {
		t.Fatalf("按包裹读回 = %d %s", status, raw)
	}
	if status, raw = fixture.manualGet(pidForeigner, "?parcel=SYN-PARCEL-M1"); status != http.StatusOK || strings.Contains(raw, "承运商已提货") {
		t.Fatalf("他租户读回 = %d %s, want 空", status, raw)
	}
}
