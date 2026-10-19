package nodeopshttp_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// Covers: ADR-0211 决定三「人工重核口对这类冲突行同样放开」的传输面：指名一行只因歧义信号的冲突，号码仍被另一份`已提交`委托报着
// 时答 200 `STILL_AMBIGUOUS`，带那一版自己与它的冲突来由；信号没了再指名，答 201 `INTAKE_FORMED`，带追加的关联版、不再带来由。
// 来由是多候选的冲突行照旧答 `IDENTITY_CONFLICT_RECORDED`。夹具全为合成（S 级）。
func TestAManualRecheckReleasesAnAmbiguityOnlyConflictOnceTheSignalIsGone(t *testing.T) {
	fixture := newRecheckEndpointFixture(t)
	contested := fixture.pending
	contested.Key.SourceID, contested.JudgmentVersion = "SYN-DELIVERY-NRI07-AMB", adapterValue(t, domain.NewIntakeResultVersion, "SYN-INTAKEV-NRI07-AMB")
	contested.Candidates = []domain.ParcelAssociationReference{adapterValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI07")}
	contested.IdentityConflict, contested.IdentityConflictCause = true, ports.ConflictByAmbiguityOnly
	contested.IdentityCheck = ports.IdentityChecked
	many := contested
	many.Key.SourceID, many.JudgmentVersion = "SYN-DELIVERY-NRI07-MANY", adapterValue(t, domain.NewIntakeResultVersion, "SYN-INTAKEV-NRI07-MANY")
	many.IdentityConflictCause = ports.ConflictByMultipleCandidates
	fixture.store.byKey[contested.Key] = contested
	fixture.store.byKey[many.Key] = many
	endpoint := nodeopshttp.NewRecheckPendingIdentityEndpoint(namedIntake{}, fixture.handler)

	fixture.identity.candidates, fixture.identity.ambiguous = []string{"SYN-PARCEL-NRI07"}, true
	still := postRecheck(endpoint, "SYN-INTAKEV-NRI07-AMB")
	var stillBody map[string]json.RawMessage
	if err := json.Unmarshal(still.Body.Bytes(), &stillBody); err != nil || still.Code != http.StatusOK {
		t.Fatalf("信号仍在：status = %d body = %s", still.Code, still.Body.String())
	}
	if string(stillBody["outcome"]) != `"STILL_AMBIGUOUS"` || string(stillBody["identityConflictCause"]) != `"AMBIGUITY_ONLY"` {
		t.Fatalf("信号仍在的响应 = %s, want STILL_AMBIGUOUS、来由 AMBIGUITY_ONLY", still.Body.String())
	}
	if _, present := stillBody["judgmentVersion"]; present {
		t.Fatalf("没追加却带了 judgmentVersion：%s", still.Body.String())
	}
	if got := decodeRecheck(t, postRecheck(endpoint, "SYN-INTAKEV-NRI07-MANY")); got.Outcome != "IDENTITY_CONFLICT_RECORDED" {
		t.Fatalf("多候选的冲突行 = %+v, want IDENTITY_CONFLICT_RECORDED", got)
	}

	fixture.identity.ambiguous = false
	released := postRecheck(endpoint, "SYN-INTAKEV-NRI07-AMB")
	if released.Code != http.StatusCreated {
		t.Fatalf("信号没了：status = %d body = %s", released.Code, released.Body.String())
	}
	var releasedBody map[string]json.RawMessage
	if err := json.Unmarshal(released.Body.Bytes(), &releasedBody); err != nil {
		t.Fatalf("响应不是 JSON：%v", err)
	}
	if string(releasedBody["outcome"]) != `"INTAKE_FORMED"` || string(releasedBody["association"]) != `"SYN-PARCEL-NRI07"` {
		t.Fatalf("信号没了的响应 = %s, want INTAKE_FORMED、关联 SYN-PARCEL-NRI07", released.Body.String())
	}
	if _, present := releasedBody["identityConflictCause"]; present {
		t.Fatalf("关联版带了冲突来由：%s", released.Body.String())
	}
	if len(fixture.intents.intents) != 1 {
		t.Fatalf("意图 %d 封, want 1", len(fixture.intents.intents))
	}
}

// Covers: ADR-0211 决定一的复核目录传输面——待识别行记了冲突来由就带 identityConflictCause（取原名），没记的（不记冲突，或来由这一格
// 落地之前的冲突行）整键不出现。
func TestUnidentifiedItemsCarryTheConflictCause(t *testing.T) {
	register := &stubReviewCatalogue{
		items: []ports.UnidentifiedItemCatalogueRow{
			{
				SourceID: "scan-ambiguous", Unit: "unit-1", Node: "node-1", Candidates: []string{"SYN-PARCEL-NRI07"},
				IdentityConflict: true, IdentityConflictCause: ports.ConflictByAmbiguityOnly,
				ReceivedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt.Add(time.Minute),
			},
			{
				SourceID: "scan-legacy", Unit: "unit-2", Node: "node-1", Candidates: []string{"SYN-PARCEL-NRI07"},
				IdentityConflict: true, ReceivedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt.Add(time.Minute),
			},
		},
	}
	response := serveGet(recordsEndpoint(register), "/node-operations-records?registry=unidentified-item")
	var body struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusOK || len(body.Items) != 2 {
		t.Fatalf("status = %d body = %s err = %v", response.Code, response.Body.String(), err)
	}
	if string(body.Items[0]["identityConflictCause"]) != `"AMBIGUITY_ONLY"` {
		t.Fatalf("只因歧义信号那一行走样：%s", response.Body.String())
	}
	if _, present := body.Items[1]["identityConflictCause"]; present {
		t.Fatalf("没记来由的旧冲突行带了 identityConflictCause：%s", response.Body.String())
	}
}
