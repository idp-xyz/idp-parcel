package nodeopshttp_test

import (
	"encoding/json"
	"net/http"
	"testing"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// Covers: ADR-0199 决定三「被重核那一版若是『未核对』，追加一版『待识别 · 已核对』」的命令口面——人工重核一件未核对的待识别，
// PS 答零候选：追加了一版，答 201 `IDENTITY_NOW_CHECKED`，带追加那一版的判断版本与 identityCheck CHECKED、没有候选；与零候选的
// 已核对待识别答 200 `STILL_UNIDENTIFIED`（判断不变）分得开。
func TestAManualRecheckOfAnUncheckedUnitWithNoCandidateAnswersNowChecked(t *testing.T) {
	fixture := newRecheckEndpointFixture(t)
	unchecked := fixture.pending
	unchecked.IdentityCheck = ports.IdentityUnchecked
	fixture.store.byKey[unchecked.Key] = unchecked
	endpoint := nodeopshttp.NewRecheckPendingIdentityEndpoint(namedIntake{}, fixture.handler)

	response := postRecheck(endpoint, "SYN-INTAKEV-NRS03-PENDING")
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s, want 201", response.Code, response.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v（%s）", err, response.Body.String())
	}
	if string(body["outcome"]) != `"IDENTITY_NOW_CHECKED"` || string(body["identityCheck"]) != `"CHECKED"` ||
		string(body["judgmentVersion"]) == "" || string(body["recheckedVersion"]) != `"SYN-INTAKEV-NRS03-PENDING"` {
		t.Fatalf("响应 = %s, want IDENTITY_NOW_CHECKED、identityCheck CHECKED、带追加那一版", response.Body.String())
	}
	if _, present := body["candidates"]; present {
		t.Fatalf("零候选的追加版带了候选：%s", response.Body.String())
	}
	if len(fixture.intents.intents) != 0 {
		t.Fatalf("意图 %d 封, want 0", len(fixture.intents.intents))
	}
}
