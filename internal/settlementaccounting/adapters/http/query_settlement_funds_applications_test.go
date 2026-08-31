package settlementhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// stubFundsApplicationCatalogue 是收付款核销页的读口替身。
type stubFundsApplicationCatalogue struct {
	facts []ports.ExternalFundsFactCatalogueRow
	err   error

	gotTenant string
	gotLimit  int
}

func (stub *stubFundsApplicationCatalogue) ListExternalFundsFacts(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.ExternalFundsFactCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.facts, stub.err
}

func fundsEndpoint(register *stubFundsApplicationCatalogue) http.Handler {
	return settlementhttp.NewQuerySettlementFundsApplicationsEndpoint(grantedIntake(), register)
}

// Covers: 三个金额各算各的 — 已核销、已撤销与未核销分别在场，不合成净额；映射与核销
// 分两个数组；未撤销的核销不带撤销两键。
func TestExternalFundsFactQuerySplitsAppliedFromReversed(t *testing.T) {
	reversedAt := catalogueBaseAt.Add(48 * time.Hour)
	register := &stubFundsApplicationCatalogue{
		facts: []ports.ExternalFundsFactCatalogueRow{
			{
				Fact:             "SYN-FACT-01",
				Source:           "SYN-SRC-01",
				Kind:             "RECEIPT_CONFIRMED",
				Currency:         "SYN-CUR-01",
				AmountMinor:      100000,
				Version:          "V1",
				OccurredAt:       catalogueBaseAt,
				AppliedMinor:     60000,
				ReversedMinor:    25000,
				UnappliedMinor:   40000,
				ApplicationCount: 2,
				Mappings: []ports.FundsMappingEntry{
					{
						Mapping:    "SYN-MAP-01",
						TargetKind: "STATEMENT",
						Target:     "SYN-STMT-01",
						Basis:      "SYN-MAP-BASIS-01",
						MappedAt:   catalogueBaseAt.Add(time.Hour),
					},
				},
				Applications: []ports.SettlementApplicationEntry{
					{
						Application:     "SYN-APP-LIVE",
						AppliedMinor:    60000,
						Basis:           "SYN-APP-BASIS-01",
						AppliedAt:       catalogueBaseAt.Add(2 * time.Hour),
						AllocationCount: 3,
					},
					{
						Application:     "SYN-APP-REVERSED",
						AppliedMinor:    25000,
						Basis:           "SYN-APP-BASIS-02",
						AppliedAt:       catalogueBaseAt.Add(3 * time.Hour),
						AllocationCount: 1,
						ReversalBasis:   "SYN-REV-BASIS-01",
						ReversedAt:      &reversedAt,
					},
				},
			},
		},
	}
	response := serveGet(fundsEndpoint(register), "/settlement-funds-applications?tenant=TENANT-9&limit=9999")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d（自报查询参数被采信了）",
			register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Facts   []struct {
			Fact             string                       `json:"fact"`
			Source           string                       `json:"source"`
			Kind             string                       `json:"kind"`
			Currency         string                       `json:"currency"`
			Amount           string                       `json:"amount"`
			Version          string                       `json:"version"`
			OccurredAt       string                       `json:"occurredAt"`
			AppliedAmount    string                       `json:"appliedAmount"`
			ReversedAmount   string                       `json:"reversedAmount"`
			UnappliedAmount  string                       `json:"unappliedAmount"`
			ApplicationCount int64                        `json:"applicationCount"`
			Mappings         []map[string]json.RawMessage `json:"mappings"`
			Applications     []map[string]json.RawMessage `json:"applications"`
		} `json:"facts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "EXTERNAL_FUNDS_FACTS_LISTED" || len(body.Facts) != 1 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	fact := body.Facts[0]
	// kind 照实透出不译成收付方向：三格里只有一格是「收到了钱」。
	if fact.Fact != "SYN-FACT-01" || fact.Source != "SYN-SRC-01" ||
		fact.Kind != "RECEIPT_CONFIRMED" || fact.Currency != "SYN-CUR-01" ||
		fact.Amount != "100000" || fact.Version != "V1" ||
		fact.OccurredAt != catalogueBaseAt.Format(time.RFC3339Nano) {
		t.Fatalf("事实面转写走样：%+v", fact)
	}
	// 「从未核销过」与「核销过又撤销了」在净额上长着同一张脸，所以三个数分别在场。
	if fact.AppliedAmount != "60000" || fact.ReversedAmount != "25000" ||
		fact.UnappliedAmount != "40000" || fact.ApplicationCount != 2 {
		t.Fatalf("三笔金额被合成了净额：%+v", fact)
	}

	if len(fact.Mappings) != 1 {
		t.Fatalf("映射数走样：%s", response.Body.String())
	}
	mapping := fact.Mappings[0]
	if string(mapping["mapping"]) != `"SYN-MAP-01"` ||
		string(mapping["targetKind"]) != `"STATEMENT"` ||
		string(mapping["target"]) != `"SYN-STMT-01"` ||
		string(mapping["basis"]) != `"SYN-MAP-BASIS-01"` {
		t.Fatalf("映射转写走样：%s", response.Body.String())
	}
	// 映射不是核销：两者分两个数组，不合成一格。
	if _, present := mapping["appliedAmount"]; present {
		t.Fatalf("映射行长出了核销金额：%s", response.Body.String())
	}

	if len(fact.Applications) != 2 {
		t.Fatalf("核销数走样：%s", response.Body.String())
	}
	live := fact.Applications[0]
	if string(live["appliedAmount"]) != `"60000"` || string(live["allocationCount"]) != "3" {
		t.Fatalf("未撤销核销转写走样：%s", response.Body.String())
	}
	for _, absent := range []string{"reversalBasis", "reversedAt"} {
		if _, present := live[absent]; present {
			t.Fatalf("未撤销的核销不该带 %q：%s", absent, response.Body.String())
		}
	}
	reversed := fact.Applications[1]
	if string(reversed["reversalBasis"]) != `"SYN-REV-BASIS-01"` ||
		string(reversed["reversedAt"]) != `"`+reversedAt.Format(time.RFC3339Nano)+`"` ||
		string(reversed["appliedAmount"]) != `"25000"` {
		t.Fatalf("已撤销核销转写走样（撤销不删历史）：%s", response.Body.String())
	}
}

// Covers: 从未核销的事实与核销后撤销的事实分得开 — 前者三个数是 0/0/全额，且两个
// 数组都以空数组在场。
func TestExternalFundsFactQueryKeepsNeverAppliedFactsDistinct(t *testing.T) {
	register := &stubFundsApplicationCatalogue{
		facts: []ports.ExternalFundsFactCatalogueRow{
			{
				Fact:           "SYN-FACT-UNTOUCHED",
				Source:         "SYN-SRC-01",
				Kind:           "PAYMENT_FAILED",
				Currency:       "SYN-CUR-01",
				AmountMinor:    70000,
				Version:        "V1",
				OccurredAt:     catalogueBaseAt,
				UnappliedMinor: 70000,
			},
		},
	}
	response := serveGet(fundsEndpoint(register), "/settlement-funds-applications")

	var body struct {
		Facts []map[string]json.RawMessage `json:"facts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if len(body.Facts) != 1 {
		t.Fatalf("行数走样：%s", response.Body.String())
	}
	fact := body.Facts[0]
	if string(fact["appliedAmount"]) != `"0"` || string(fact["reversedAmount"]) != `"0"` ||
		string(fact["unappliedAmount"]) != `"70000"` || string(fact["applicationCount"]) != "0" {
		t.Fatalf("从未核销的事实走样：%s", response.Body.String())
	}
	for _, key := range []string{"mappings", "applications"} {
		if string(fact[key]) != "[]" {
			t.Fatalf("%q 没有以空数组在场：%s", key, response.Body.String())
		}
	}
	// 更正留痕两键成对缺席：这笔事实没有更正过谁。
	for _, absent := range []string{"corrects", "correctedAt"} {
		if _, present := fact[absent]; present {
			t.Fatalf("未更正的事实不该带 %q：%s", absent, response.Body.String())
		}
	}
}

// Covers: ADR-0077 Decision 四 — 空册是 2xx + 空数组（不是 null）。
func TestAnEmptyExternalFundsRegisterAnswersAnEmptyArray(t *testing.T) {
	response := serveGet(fundsEndpoint(&stubFundsApplicationCatalogue{}), "/settlement-funds-applications")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if got := arrayAt(t, response, "facts"); got != "[]" {
		t.Fatalf("空册没有以空数组在场：%s", got)
	}
}

// Covers: ADR-0022/ADR-0029 — 读不回是答案未形成（5xx），不伪装成空册。
func TestAFailingExternalFundsReadIsNoAnswerRatherThanAnEmptyRegister(t *testing.T) {
	endpoint := fundsEndpoint(&stubFundsApplicationCatalogue{err: errors.New("connection refused")})
	response := serveGet(endpoint, "/settlement-funds-applications")

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
	}
	assertNoOutcome(t, response)
}
