package main

import (
	"net/http"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// 本文件对真实 PostgreSQL 证更正两口的生产装配（票 demo-chain-gaps/57，ADR-0232）：委托提交、段采用、发生项登记、发生项失效与解除
// 采用都经操作者渠道「运营决定」能力面走到真编排；干线协议照种子发布批经 PC 发布。与生产装配的差别只在令牌核验方与操作者册是
// 替身。夹具全为合成（S 级）。

const occurrenceInvalidationPath = "/transport-fulfillment-charge-occurrence-invalidations"

type correctionEndpoints struct {
	commission   http.Handler
	adoption     http.Handler
	charge       http.Handler
	invalidation http.Handler
}

// correctionFacesForTest 装生产的委托提交、段采用、发生项登记与两口更正；操作者持有其中各种决定。
func correctionFacesForTest(t *testing.T, db *bentopg.DB) correctionEndpoints {
	t.Helper()
	kinds := []accessidentity.DecisionKind{
		accessidentity.DecisionTransportCommissionSubmission,
		accessidentity.DecisionFulfillmentSegmentCommissionAdoption,
		accessidentity.DecisionTransportChargeOccurrenceRegistration,
		accessidentity.DecisionTransportChargeOccurrenceInvalidation,
	}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, kinds...)
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, decisionRegister{kinds: kinds}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	commission, err := buildTransportCommissionSubmission(db)
	if err != nil {
		t.Fatalf("装配委托提交：%v", err)
	}
	adopter, err := buildSegmentCommissionAdoption(db)
	if err != nil {
		t.Fatalf("装配采用：%v", err)
	}
	registrar, err := buildChargeOccurrenceRegistration(db)
	if err != nil {
		t.Fatalf("装配发生项登记：%v", err)
	}
	invalidator, err := buildChargeOccurrenceInvalidation(db)
	if err != nil {
		t.Fatalf("装配发生项失效：%v", err)
	}
	return correctionEndpoints{
		commission:   tfhttp.NewSubmitTransportCommissionEndpoint(decisions.transport, commission),
		adoption:     tfhttp.NewAdoptSegmentCommissionEndpoint(decisions.transport, adopter),
		charge:       tfhttp.NewRegisterChargeOccurrenceEndpoint(decisions.transport, registrar),
		invalidation: tfhttp.NewInvalidateChargeOccurrenceEndpoint(decisions.transport, invalidator),
	}
}

// adoptedTrunkOccurrence 在真库上立干线段、提交指名 v3 的干线委托、采用它并登一项实际履约，交回那一项的引用。
func adoptedTrunkOccurrence(t *testing.T, db *bentopg.DB, endpoints correctionEndpoints) (occurrence, provider string) {
	t.Helper()
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	provider = trunk.Declarations.SupplierAgreementBody.Supplier
	mustPost(t, endpoints.commission, commissionSubmissionPath,
		commissionBody(adoptedChargeCommiss, provider, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"),
		http.StatusCreated, "COMMISSION_SUBMITTED")
	saveEndedTrunkSegment(t, db)
	mustPost(t, endpoints.adoption, adoptionPath, adoptionBody(adoptedChargeSegment, adoptedChargeCommiss, provider),
		http.StatusCreated, "COMMISSION_ADOPTED")
	answer := mustPost(t, endpoints.charge, chargeOccurrencePath, adoptedFulfillmentBody(), http.StatusCreated, "REGISTERED")
	registered, _ := answer["occurrence"].(map[string]any)
	occurrence, _ = registered["occurrence"].(string)
	return occurrence, provider
}

func adoptedFulfillmentBody() string {
	return `{"reason": "ACTUAL_FULFILLMENT", "segment": "` + adoptedChargeSegment + `", "objects": ["` + adoptedChargeParcel + `"],` +
		` "journey": "SYN-JRN-` + adoptedChargeParcel + `"}`
}

func invalidationBody(occurrence, version, basis string) string {
	return `{"occurrence": "` + occurrence + `", "version": "` + version + `", "basis": "` + basis + `"}`
}

// Covers: AT-TF-311、AT-TF-312 的生产装配半边——已采用委托的段上登了一项实际履约（V1），经失效口答 201、长出 V2 失效版本并同笔交出
// 一封信封；原样再交答 200 EXISTING_INVALIDATION、不多一行也不多一封。同一件事实再登不再答已存在，答 201、长出 V3 替代版本并交出
// 信封；再登答 EXISTING 回显 V3。
func TestAnOccurrenceIsInvalidatedAndReestablishedThroughTheProductionFaces(t *testing.T) {
	db := migratedChargeDB(t)
	endpoints := correctionFacesForTest(t, db)
	occurrence, _ := adoptedTrunkOccurrence(t, db, endpoints)

	answer := mustPost(t, endpoints.invalidation, occurrenceInvalidationPath,
		invalidationBody(occurrence, "V1", "SYN-EVIDENCE-DCG57"), http.StatusCreated, "OCCURRENCE_INVALIDATED")
	if invalidated, _ := answer["occurrence"].(map[string]any); invalidated["version"] != "V2" || invalidated["corrects"] != "V1" ||
		invalidated["revisionKind"] != "INVALIDATED" || invalidated["revisedBy"] == nil {
		t.Fatalf("失效回显 = %v", answer)
	}
	mustPost(t, endpoints.invalidation, occurrenceInvalidationPath,
		invalidationBody(occurrence, "V1", "SYN-EVIDENCE-DCG57"), http.StatusOK, "EXISTING_INVALIDATION")
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 2 || envelopes != 2 {
		t.Fatalf("失效后 %d 行、%d 封，want 2 / 2", occurrences, envelopes)
	}

	answer = mustPost(t, endpoints.charge, chargeOccurrencePath, adoptedFulfillmentBody(), http.StatusCreated, "REGISTERED")
	if reestablished, _ := answer["occurrence"].(map[string]any); reestablished["occurrence"] != occurrence ||
		reestablished["version"] != "V3" || reestablished["corrects"] != "V2" || reestablished["revisionKind"] != "SUPERSEDED" {
		t.Fatalf("重新成立 = %v", answer)
	}
	answer = mustPost(t, endpoints.charge, chargeOccurrencePath, adoptedFulfillmentBody(), http.StatusOK, "EXISTING")
	if echoed, _ := answer["occurrence"].(map[string]any); echoed["version"] != "V3" {
		t.Fatalf("重放回显 = %v", answer)
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 3 || envelopes != 3 {
		t.Fatalf("重新成立后 %d 行、%d 封，want 3 / 3", occurrences, envelopes)
	}
	mustPost(t, endpoints.invalidation, occurrenceInvalidationPath,
		invalidationBody(occurrence, "V2", "SYN-EVIDENCE-DCG57"), http.StatusOK, "VERSION_NOT_CURRENT")
}
