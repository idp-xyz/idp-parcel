package pricinghttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 线上那一格：plan、选用区间、交集、是不是同一方案、收尾种类。无上界不写终点。
// 发布带着重叠仍是 201，批准仍是 200，结果码不变。

func overlapAt(t *testing.T, year int, month time.Month) time.Time {
	t.Helper()
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
}

func overlapPeriod(t *testing.T, start, end time.Time) domain.EffectivePeriod {
	t.Helper()
	period, err := domain.NewEffectivePeriod(start, end)
	if err != nil {
		t.Fatal(err)
	}
	return period
}

func overlapRef(t *testing.T, id, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, id, version)
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

func threeOverlapResolutions(t *testing.T) []domain.PriceCardOverlap {
	t.Helper()
	january := overlapAt(t, 2026, time.January)
	june := overlapAt(t, 2026, time.June)
	september := overlapAt(t, 2026, time.September)
	openJanuary := overlapPeriod(t, january, time.Time{})
	openJune := overlapPeriod(t, june, time.Time{})
	bounded := overlapPeriod(t, june, september)
	sameV1 := overlapRef(t, "SYN-PLAN", "v1")
	sameV2 := overlapRef(t, "SYN-PLAN", "v2")
	other := overlapRef(t, "SYN-OTHER", "v1")
	return []domain.PriceCardOverlap{
		classified(t, sameV2, openJune, sameV1, openJanuary),
		classified(t, sameV2, openJanuary, sameV1, openJune),
		classified(t, sameV2, bounded, sameV1, openJanuary),
		classified(t, other, openJune, sameV1, openJanuary),
	}
}

func classified(t *testing.T, candidate domain.VersionReference, candidatePeriod domain.EffectivePeriod, registered domain.VersionReference, registeredPeriod domain.EffectivePeriod) domain.PriceCardOverlap {
	t.Helper()
	item, ok := domain.ClassifyPriceCardOverlap(candidate, candidatePeriod, nil, registered, registeredPeriod, nil)
	if !ok {
		t.Fatal("这组适用期没有交")
	}
	return item
}

type overlapWire struct {
	Plan           map[string]string `json:"plan"`
	SelectionStart string            `json:"selectionStart"`
	SelectionEnd   string            `json:"selectionEnd"`
	OverlapStart   string            `json:"overlapStart"`
	OverlapEnd     string            `json:"overlapEnd"`
	SamePlan       bool              `json:"samePlan"`
	Resolution     string            `json:"resolution"`
}

func TestPriceCardOverlapJSONNamesTheResolutionAndLeavesAnOpenEndAbsent(t *testing.T) {
	overlaps := threeOverlapResolutions(t)
	plan := estimatePlan(t)
	authorization, err := domain.NewVersionReferenceIdentity(domain.ArtifactCommercialAuthorization, "SYN-AUTH", "v1")
	if err != nil {
		t.Fatal(err)
	}
	recorder := postPriceCardPreview(priceCardPreviewIntakeDouble{}, priceCardPreviewerDouble{preview: application.PriceCardImportPreview{
		Outcome: application.PriceCardImportValidated, Plan: plan.Reference(), Overlaps: overlaps,
		Content: &ports.PriceCardTemplateContent{Plan: plan, DirectionAuthorization: authorization},
	}})
	if recorder.Code != http.StatusOK {
		t.Fatalf("预览 status = %d body = %s", recorder.Code, recorder.Body)
	}
	var preview struct {
		Outcome  string        `json:"outcome"`
		Overlaps []overlapWire `json:"overlaps"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Outcome != "VALIDATED" || len(preview.Overlaps) != 4 {
		t.Fatalf("预览 = %s", recorder.Body)
	}
	assertOverlapWire(t, preview.Overlaps[0], "SYN-PLAN", "v1", true, "SUCCESSOR_CUT", "2026-06-01T00:00:00Z", "")
	assertOverlapWire(t, preview.Overlaps[1], "SYN-PLAN", "v1", true, "RETIRE_PREDECESSOR", "2026-06-01T00:00:00Z", "")
	assertOverlapWire(t, preview.Overlaps[2], "SYN-PLAN", "v1", true, "GOVERNANCE_DECISION", "2026-06-01T00:00:00Z", "2026-09-01T00:00:00Z")
	assertOverlapWire(t, preview.Overlaps[3], "SYN-PLAN", "v1", false, "GOVERNANCE_DECISION", "2026-06-01T00:00:00Z", "")
	var envelope struct {
		Overlaps []map[string]json.RawMessage `json:"overlaps"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if _, present := envelope.Overlaps[0]["selectionEnd"]; present {
		t.Fatal("无上界的选用终点不该出现")
	}
	if _, present := envelope.Overlaps[0]["overlapEnd"]; present {
		t.Fatal("无上界的交集终点不该出现")
	}
	if _, present := envelope.Overlaps[2]["overlapEnd"]; !present {
		t.Fatal("有上界的交集终点该出现")
	}
	if !strings.Contains(string(envelope.Overlaps[3]["samePlan"]), "false") {
		t.Fatal("不同方案的 samePlan 应是 false，不能缺席")
	}

	approved := approvedDraftForAnswer(t)
	approvalRecorder := httptest.NewRecorder()
	pricinghttp.NewApprovePriceCardDraftEndpoint(approvalIntakeDouble{}, approverDouble{
		result: application.ApprovePriceCardDraftResult{
			Outcome: application.PriceCardDraftApproved, Draft: approved, HasDraft: true, Overlaps: overlaps[:1],
		},
	}).ServeHTTP(approvalRecorder, progressRequest("/pricing-price-card-draft-approvals", "t", draftReference))
	approval := decodeProgress(t, approvalRecorder)
	if approvalRecorder.Code != http.StatusOK || approval.Outcome != "DRAFT_APPROVED" || len(approval.Overlaps) != 1 ||
		approval.Overlaps[0].Resolution != "SUCCESSOR_CUT" {
		t.Fatalf("批准：%d %+v", approvalRecorder.Code, approval)
	}

	published, err := approved.MarkPublished(time.Date(2026, 10, 10, 5, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	publicationRecorder := httptest.NewRecorder()
	pricinghttp.NewPublishPriceCardDraftEndpoint(publicationIntakeDouble{}, publisherDouble{
		result: application.PublishPriceCardDraftResult{
			Outcome: application.PriceCardDraftPublished, Registration: application.PriceCardRecorded,
			HasRegistration: true, Draft: published, HasDraft: true, Overlaps: overlaps[:1],
		},
	}).ServeHTTP(publicationRecorder, progressRequest("/pricing-price-card-draft-publications", "t", draftReference))
	publication := decodeProgress(t, publicationRecorder)
	if publicationRecorder.Code != http.StatusCreated || publication.Outcome != "DRAFT_PUBLISHED" || publication.Registration != "RECORDED" ||
		len(publication.Overlaps) != 1 || publication.Overlaps[0].Resolution != "SUCCESSOR_CUT" || !publication.Overlaps[0].SamePlan {
		t.Fatalf("发布仍应 201，重叠只是多一格：%d %+v", publicationRecorder.Code, publication)
	}
}

func assertOverlapWire(t *testing.T, item overlapWire, planID, version string, same bool, resolution, overlapStart, overlapEnd string) {
	t.Helper()
	if item.Plan["id"] != planID || item.Plan["version"] != version || item.SamePlan != same || item.Resolution != resolution ||
		item.OverlapStart != overlapStart || item.OverlapEnd != overlapEnd || item.SelectionStart == "" {
		t.Fatalf("%s = %+v", resolution, item)
	}
}

func decodeProgress(t *testing.T, recorder *httptest.ResponseRecorder) struct {
	Outcome      string        `json:"outcome"`
	Registration string        `json:"registration"`
	Overlaps     []overlapWire `json:"overlaps"`
} {
	t.Helper()
	var body struct {
		Outcome      string        `json:"outcome"`
		Registration string        `json:"registration"`
		Overlaps     []overlapWire `json:"overlaps"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("答复不是 JSON：%s", recorder.Body)
	}
	return body
}
