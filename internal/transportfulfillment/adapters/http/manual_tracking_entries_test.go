package tfhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

type capturingAdopter struct {
	materials []ports.TrackingMaterial
}

func (fake *capturingAdopter) Adopt(_ context.Context, material ports.TrackingMaterial) (application.AdoptTrackingMaterialResult, error) {
	fake.materials = append(fake.materials, material)
	return application.AdoptTrackingMaterialResult{}, nil
}

func manualTrackingEndpoint(t *testing.T, asked *[]tfhttp.OperatorDecision) (http.Handler, *capturingAdopter) {
	t.Helper()
	intake, err := tfhttp.NewOperatorDecisionIntake(authenticatorFake{asked: asked})
	if err != nil {
		t.Fatal(err)
	}
	adopter := &capturingAdopter{}
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	return tfhttp.NewManualTrackingEntriesEndpoint(intake, adopter, func() time.Time { return now }), adopter
}

func postTracking(handler http.Handler, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// Covers: dox/decisions.md「产品形态」——轨迹手工录入口按「录入承运商轨迹」这一种决定认证；租户与录入人只取认证结果，
// 接收时刻由服务端给；一批逐条转成素材，源事件标识加 TRKMANUAL/ 前缀，没给事件号的按内容派生（同一条两次导入派生出同一个
// 号、内容不同号不同）；承运商没给发生时间就不填、不拿录入时间顶替；各条的答复逐条交回。
func TestTheManualTrackingFaceTurnsEachEntryIntoMaterialUnderItsOwnDecisionKind(t *testing.T) {
	var asked []tfhttp.OperatorDecision
	endpoint, adopter := manualTrackingEndpoint(t, &asked)
	body := `{"entries":[` +
		`{"source":"SYN-CARRIER-X","credential":"SYN-CARRIER-X/1Z-000001","eventId":"EVT-9","occurredAt":"2026-11-12T08:00:00Z","status":"IN_TRANSIT","original":" SYN-MAIL-1 "},` +
		`{"source":"SYN-CARRIER-X","credential":"SYN-CARRIER-X/1Z-000001","occurredAt":"2026-11-12T10:00:00Z","status":"ARRIVED","original":"SYN-MAIL-1"},` +
		`{"source":"SYN-CARRIER-X","credential":"SYN-CARRIER-X/1Z-000001","status":"DELIVERED","original":"SYN-MAIL-1"}]}`

	response := postTracking(endpoint, "t", body)
	var answer struct {
		Outcome string
		Counts  map[string]int
		Entries []struct {
			Index           int
			Outcome         string
			EventID         string
			AnswerNotFormed bool
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("响应不是 JSON：%s", response.Body.String())
	}
	if response.Code != http.StatusOK || answer.Outcome != "ENTRIES_PROCESSED" || len(answer.Entries) != 3 || len(adopter.materials) != 3 {
		t.Fatalf("answer = %d %s", response.Code, response.Body.String())
	}
	if len(asked) != 1 || asked[0] != "EXTERNAL_TRACKING_ENTRY" {
		t.Fatalf("asked = %v", asked)
	}
	first, second, third := adopter.materials[0], adopter.materials[1], adopter.materials[2]
	if first.Subject.Tenant.String() != operatorTenant || first.EnteredBy != "OPERATOR/"+operatorSubject || first.Original != "SYN-MAIL-1" ||
		first.SourceEventID != "TRKMANUAL/EVT-9" || !first.OccurredAt.Given || first.ReceivedAt.Hour() != 9 || first.StatusReference != "IN_TRANSIT" {
		t.Fatalf("第一条素材 = %+v", first)
	}
	if !strings.HasPrefix(second.SourceEventID, "TRKMANUAL/") || second.SourceEventID == "TRKMANUAL/" || second.SourceEventID == third.SourceEventID {
		t.Fatalf("派生的事件号 = %q / %q", second.SourceEventID, third.SourceEventID)
	}
	if third.OccurredAt.Given {
		t.Fatal("承运商没给发生时间，素材上却有了一个")
	}
	// 没形成答案的条如实标出，不混进业务答复里。
	if !answer.Entries[0].AnswerNotFormed || answer.Counts["ANSWER_NOT_FORMED"] != 3 || answer.Entries[1].EventID != second.SourceEventID {
		t.Fatalf("各条答复 = %+v", answer)
	}

	replay, replayAdopter := manualTrackingEndpoint(t, nil)
	postTracking(replay, "t", body)
	if replayAdopter.materials[1].SourceEventID != second.SourceEventID {
		t.Fatal("同一条两次导入派生出了不同的事件号")
	}
}

// Covers: 认证方的格先于载荷；载荷封闭；一批 1 到上限条；原件必填；发生时间给了却不是 RFC 3339 是坏报文。坏报文整批不进编排。
func TestTheManualTrackingFaceRefusesMalformedBatchesBeforeAnyEntryIsAdopted(t *testing.T) {
	endpoint, adopter := manualTrackingEndpoint(t, nil)
	entry := `{"source":"S","credential":"C","status":"X","original":"O"}`
	for name, body := range map[string]string{
		"空批":      `{"entries":[]}`,
		"超过上限":    `{"entries":[` + strings.TrimSuffix(strings.Repeat(entry+",", tfhttp.ManualTrackingEntryLimit+1), ",") + `]}`,
		"没有原件":    `{"entries":[{"source":"S","credential":"C","status":"X"}]}`,
		"时间不成形":   `{"entries":[{"source":"S","credential":"C","status":"X","original":"O","occurredAt":"昨天"}]}`,
		"自报租户":    `{"tenantId":"other","entries":[` + entry + `]}`,
		"条上自报录入人": `{"entries":[{"source":"S","credential":"C","status":"X","original":"O","enteredBy":"someone"}]}`,
	} {
		if response := postTracking(endpoint, "t", body); response.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s，想要 400", name, response.Code, response.Body.String())
		}
	}
	if response := postTracking(endpoint, "", `{"entries":[`+entry+`]}`); response.Code != http.StatusUnauthorized {
		t.Fatalf("没有凭据 = %d，想要 401", response.Code)
	}
	if len(adopter.materials) != 0 {
		t.Fatalf("坏报文到了编排：%d 条", len(adopter.materials))
	}
}
