package nodeopshttp_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// Covers: 票 no-reception-source/03「待识别行带来源文档里的外部标识观察（旧行如实写来源未保全）」的传输面：每行带判断版本；
// 来源在时 sourcePreserved 为 true、externalMark 原样（空号也照列空串），来源缺席时 sourcePreserved 为 false、externalMark 整键
// 不出现；已被接续的那一版带 succeededBy，当前有效的不带。
func TestUnidentifiedItemsCarryTheMarkObservationAndTheChainPlace(t *testing.T) {
	register := &stubReviewCatalogue{
		items: []ports.UnidentifiedItemCatalogueRow{
			{
				SourceID: "scan-current", Unit: "unit-1", Node: "node-1", Candidates: []string{},
				ReceivedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt.Add(time.Minute),
				JudgmentVersion: "SYN-INTAKEV-NRS03-CURRENT", SourcePreserved: true, Mark: " SYN-MARK-NRS03\t",
			},
			{
				SourceID: "scan-blank", Unit: "unit-2", Node: "node-1", Candidates: []string{},
				ReceivedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt.Add(time.Minute),
				JudgmentVersion: "SYN-INTAKEV-NRS03-BLANK", SourcePreserved: true,
				SucceededBy: "SYN-INTAKEV-NRS03-NEXT",
			},
			{
				SourceID: "scan-old", Unit: "unit-3", Node: "node-1", Candidates: []string{},
				ReceivedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt.Add(time.Minute),
				JudgmentVersion: "SYN-INTAKEV-NRS03-OLD",
			},
		},
	}
	response := serveGet(recordsEndpoint(register), "/node-operations-records?registry=unidentified-item")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Items) != 3 {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	current, blank, old := body.Items[0], body.Items[1], body.Items[2]
	if string(current["judgmentVersion"]) != `"SYN-INTAKEV-NRS03-CURRENT"` || string(current["sourcePreserved"]) != `true` ||
		string(current["externalMark"]) != `" SYN-MARK-NRS03\t"` {
		t.Fatalf("当前行走样：%s", response.Body.String())
	}
	if _, present := current["succeededBy"]; present {
		t.Fatalf("当前有效的那一版带了 succeededBy：%s", response.Body.String())
	}
	if string(blank["externalMark"]) != `""` || string(blank["succeededBy"]) != `"SYN-INTAKEV-NRS03-NEXT"` {
		t.Fatalf("空号且已被接续的那一行走样：%s", response.Body.String())
	}
	if _, present := old["externalMark"]; present || string(old["sourcePreserved"]) != `false` {
		t.Fatalf("来源未保全的旧行走样：%s", response.Body.String())
	}
}
