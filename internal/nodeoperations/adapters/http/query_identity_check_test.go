package nodeopshttp_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// Covers: 票 no-reception-source/05「复核目录待识别行带核对态」的传输面（ADR-0199 决定五）：待识别册与收寄册的待识别行各带
// identityCheck，取写侧答复同一个词（CHECKED / UNCHECKED）；收寄册上关联收寄行整键不出现，同 serviceResultCheck 的缺席惯例。
func TestCatalogueRowsCarryTheIdentityCheck(t *testing.T) {
	register := &stubReviewCatalogue{
		items: []ports.UnidentifiedItemCatalogueRow{
			{
				SourceID: "scan-unchecked", Unit: "unit-1", Node: "node-1", Candidates: []string{},
				ReceivedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt.Add(time.Minute),
				JudgmentVersion: "SYN-INTAKEV-NRS05-1", SourcePreserved: true, Mark: "SYN-MARK-NRS05",
				IdentityCheck: ports.IdentityUnchecked,
			},
			{
				SourceID: "scan-checked", Unit: "unit-2", Node: "node-1", Candidates: []string{},
				ReceivedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt.Add(time.Minute),
				JudgmentVersion: "SYN-INTAKEV-NRS05-2", SourcePreserved: true, Mark: "SYN-MARK-NRS05",
				IdentityCheck: ports.IdentityChecked,
			},
		},
		receptions: []ports.ReceptionCatalogueRow{
			{
				SourceID: "scan-unchecked", Kind: "PENDING_IDENTIFICATION", Unit: "unit-1", Node: "node-1",
				ReceivedAt: catalogueBaseAt, ControlKind: "NODE_INTAKE", ControlEstablishedAt: catalogueBaseAt,
				ServiceMarkers: []string{}, RecordedAt: catalogueBaseAt, IdentityCheck: ports.IdentityUnchecked,
			},
			{
				SourceID: "scan-formed", Kind: "INTAKE_FORMED", Unit: "unit-3", Node: "node-1",
				ReceivedAt: catalogueBaseAt, ControlKind: "NODE_INTAKE", ControlEstablishedAt: catalogueBaseAt,
				ServiceMarkers: []string{}, ServiceResultCheck: ports.ServiceResultsChecked, RecordedAt: catalogueBaseAt,
			},
		},
	}

	items := serveGet(recordsEndpoint(register), "/node-operations-records?registry=unidentified-item")
	var itemBody struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if items.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", items.Code, items.Body.String())
	}
	if err := json.Unmarshal(items.Body.Bytes(), &itemBody); err != nil || len(itemBody.Items) != 2 {
		t.Fatalf("decode %s: %v", items.Body.Bytes(), err)
	}
	if string(itemBody.Items[0]["identityCheck"]) != `"UNCHECKED"` || string(itemBody.Items[1]["identityCheck"]) != `"CHECKED"` {
		t.Fatalf("待识别册的 identityCheck 走样：%s", items.Body.String())
	}

	receptions := serveGet(recordsEndpoint(register), "/node-operations-records?registry=reception")
	var receptionBody struct {
		Receptions []map[string]json.RawMessage `json:"receptions"`
	}
	if receptions.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", receptions.Code, receptions.Body.String())
	}
	if err := json.Unmarshal(receptions.Body.Bytes(), &receptionBody); err != nil || len(receptionBody.Receptions) != 2 {
		t.Fatalf("decode %s: %v", receptions.Body.Bytes(), err)
	}
	if string(receptionBody.Receptions[0]["identityCheck"]) != `"UNCHECKED"` {
		t.Fatalf("收寄册待识别行的 identityCheck 走样：%s", receptions.Body.String())
	}
	if _, present := receptionBody.Receptions[1]["identityCheck"]; present {
		t.Fatalf("关联收寄行带了 identityCheck：%s", receptions.Body.String())
	}
}
