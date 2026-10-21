package settlementhttp_test

import (
	"encoding/json"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// Covers: AT-SA-200（读面半边）——资金事实册上列透出链头那一版的提交方：记了的照原样在场，不记的一路缺席，不写空串冒充「无人提交」。
func TestExternalFundsFactQueryShowsTheSubmitterOnlyWhenRecorded(t *testing.T) {
	register := &stubFundsApplicationCatalogue{
		facts: []ports.ExternalFundsFactCatalogueRow{
			{Fact: "SYN-FACT-MANUAL", Source: "SYN-SRC-01", Kind: "RECEIPT_CONFIRMED", Currency: "SYN-CUR-01", AmountMinor: 100,
				Version: "V1", OccurredAt: catalogueBaseAt, UnappliedMinor: 100,
				OperationEvidence: "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-01"},
			{Fact: "SYN-FACT-CLIENT", Source: "SYN-SRC-01", Kind: "RECEIPT_CONFIRMED", Currency: "SYN-CUR-01", AmountMinor: 100,
				Version: "V1", OccurredAt: catalogueBaseAt, UnappliedMinor: 100},
		},
	}
	response := serveGet(fundsEndpoint(register), "/settlement-funds-applications")
	var body struct {
		Facts []map[string]json.RawMessage `json:"facts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Facts) != 2 {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if got := string(body.Facts[0]["operationEvidence"]); got != `"OPERATOR/https://id.syn.example/dex#SYN-FINANCE-01"` {
		t.Fatalf("手工采用那一笔的提交方 = %s", got)
	}
	if _, present := body.Facts[1]["operationEvidence"]; present {
		t.Fatalf("不记提交方的一笔带了 operationEvidence：%s", response.Body.String())
	}
}
