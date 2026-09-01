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

// stubOperatingCatalogue 是经营核算页两本册子的读口替身。
type stubOperatingCatalogue struct {
	results     []ports.OperatingResultCatalogueRow
	allocations []ports.CostAllocationCatalogueRow
	err         error

	gotTenant string
	gotLimit  int
}

func (stub *stubOperatingCatalogue) ListOperatingResults(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.OperatingResultCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.results, stub.err
}

func (stub *stubOperatingCatalogue) ListCostAllocations(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.CostAllocationCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.allocations, stub.err
}

func operatingEndpoint(register *stubOperatingCatalogue) http.Handler {
	return settlementhttp.NewQuerySettlementOperatingResultsEndpoint(grantedIntake(), register)
}

// Covers: 组成逐项在场不净额 — 增减两向的组成各占一行；毛利照册转写，读口不重算；
// 负毛利（经营损失）仍走同一个键。
func TestOperatingResultQueryTranscribesComponentsWithoutNetting(t *testing.T) {
	register := &stubOperatingCatalogue{
		results: []ports.OperatingResultCatalogueRow{
			{
				Scope:       "SYN-SCOPE-01",
				Period:      "2026-07",
				Basis:       "CONFIRMED",
				Currency:    "SYN-CUR-01",
				MarginMinor: 30000,
				Version:     "V1",
				AsOf:        catalogueBaseAt,
				RecordedAt:  catalogueBaseAt,
				Components: []ports.OperatingComponentEntry{
					{Source: "SYN-RECEIVABLE-01", Role: "CUSTOMER_OPERATING_RECEIVABLE", Effect: "INCREASES", AmountMinor: 100000},
					{Source: "SYN-PAYABLE-01", Role: "AUDITED_PAYABLE", Effect: "DECREASES", AmountMinor: 80000},
					{Source: "SYN-CREDIT-NOTE-01", Role: "SUPPLIER_CREDIT_NOTE", Effect: "INCREASES", AmountMinor: 10000},
				},
			},
			{
				Scope:       "SYN-SCOPE-02",
				Period:      "2026-07",
				Basis:       "ESTIMATED",
				Currency:    "SYN-CUR-01",
				MarginMinor: -15000,
				Version:     "V1",
				AsOf:        catalogueBaseAt,
				Corrects:    "SYN-RESULT-PRIOR",
				RecordedAt:  catalogueBaseAt,
				Components: []ports.OperatingComponentEntry{
					{Source: "SYN-EXPECTED-COST-01", Role: "SUPPLIER_EXPECTED_COST", Effect: "DECREASES", AmountMinor: 15000},
				},
			},
		},
	}
	response := serveGet(operatingEndpoint(register), "/settlement-operating-results?registry=operating-result&tenant=TENANT-9")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Results []struct {
			Scope      string          `json:"scope"`
			Period     string          `json:"period"`
			Basis      string          `json:"basis"`
			Currency   string          `json:"currency"`
			Margin     string          `json:"margin"`
			Version    string          `json:"version"`
			AsOf       string          `json:"asOf"`
			Corrects   string          `json:"corrects"`
			RecordedAt string          `json:"recordedAt"`
			Components json.RawMessage `json:"components"`
		} `json:"results"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "OPERATING_RESULTS_LISTED" || len(body.Results) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	result := body.Results[0]
	if result.Scope != "SYN-SCOPE-01" || result.Period != "2026-07" ||
		result.Basis != "CONFIRMED" || result.Currency != "SYN-CUR-01" ||
		result.Margin != "30000" || result.Version != "V1" ||
		result.AsOf != catalogueBaseAt.Format(time.RFC3339Nano) {
		t.Fatalf("经营结果面转写走样：%+v", result)
	}
	var components []struct {
		Source string `json:"source"`
		Role   string `json:"role"`
		Effect string `json:"effect"`
		Amount string `json:"amount"`
	}
	if err := json.Unmarshal(result.Components, &components); err != nil {
		t.Fatalf("decode components %s: %v", result.Components, err)
	}
	// 三项分别在场：审核应付与贷项按各自方向各计一次，净额会把这条要求抹掉。
	if len(components) != 3 ||
		components[0].Source != "SYN-RECEIVABLE-01" || components[0].Effect != "INCREASES" ||
		components[0].Amount != "100000" ||
		components[1].Source != "SYN-PAYABLE-01" || components[1].Effect != "DECREASES" ||
		components[1].Amount != "80000" ||
		components[2].Source != "SYN-CREDIT-NOTE-01" || components[2].Effect != "INCREASES" ||
		components[2].Amount != "10000" {
		t.Fatalf("组成被净额化或走样：%+v", components)
	}
	// 角色照册透出（ADR-0087 决定三）：哪一项是审核应付、哪一项是贷项，页面从此读得出
	// 而不是按 source 猜——票 04 撤掉经营页四栏正是因为那时只能猜。
	if components[0].Role != "CUSTOMER_OPERATING_RECEIVABLE" ||
		components[1].Role != "AUDITED_PAYABLE" ||
		components[2].Role != "SUPPLIER_CREDIT_NOTE" {
		t.Fatalf("组成项角色透出走样：%+v", components)
	}

	// 负毛利即经营损失，走同一个键——按正负分两键会让「零」落进两键都不占的缝里。
	// 组成不测空数组：库上 operating_result_components_shaped 要求非空数组，这一册
	// 拿不出「零个组成」的行，测一个登记册长不出的状态只会让后来的人以为它能出现。
	loss := body.Results[1]
	if loss.Margin != "-15000" || loss.Corrects != "SYN-RESULT-PRIOR" {
		t.Fatalf("经营损失行转写走样：%+v", loss)
	}
}

// Covers: 未分摊是第一类结果 — 份额与未分摊余额分别在场；全额未分摊时份额是空数组
// 而不是被凑平的一份。
func TestCostAllocationQueryTranscribesPortionsAndUnallocated(t *testing.T) {
	register := &stubOperatingCatalogue{
		allocations: []ports.CostAllocationCatalogueRow{
			{
				Allocation:       "SYN-ALLOC-SPLIT",
				Source:           "SYN-COST-01",
				SourceMinor:      90000,
				Currency:         "SYN-CUR-01",
				Rule:             "SYN-RULE-V1",
				UnallocatedMinor: 0,
				Version:          "V1",
				AllocatedAt:      catalogueBaseAt,
				RecordedAt:       catalogueBaseAt,
				Portions: []ports.AllocationPortionEntry{
					{Target: "SYN-TARGET-01", AmountMinor: 60000},
					{Target: "SYN-TARGET-02", AmountMinor: 30000},
				},
			},
			{
				Allocation:       "SYN-ALLOC-NONE",
				Source:           "SYN-COST-02",
				SourceMinor:      50000,
				Currency:         "SYN-CUR-01",
				Rule:             "SYN-RULE-V1",
				UnallocatedMinor: 50000,
				Version:          "V1",
				AllocatedAt:      catalogueBaseAt,
				RecordedAt:       catalogueBaseAt,
			},
		},
	}
	response := serveGet(operatingEndpoint(register), "/settlement-operating-results?registry=cost-allocation")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome     string                       `json:"outcome"`
		Allocations []map[string]json.RawMessage `json:"allocations"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "COST_ALLOCATIONS_LISTED" || len(body.Allocations) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}

	split := body.Allocations[0]
	if string(split["allocation"]) != `"SYN-ALLOC-SPLIT"` ||
		string(split["source"]) != `"SYN-COST-01"` ||
		string(split["sourceAmount"]) != `"90000"` ||
		string(split["rule"]) != `"SYN-RULE-V1"` ||
		string(split["unallocatedAmount"]) != `"0"` {
		t.Fatalf("分摊面转写走样：%s", response.Body.String())
	}
	var portions []struct {
		Target string `json:"target"`
		Amount string `json:"amount"`
	}
	if err := json.Unmarshal(split["portions"], &portions); err != nil {
		t.Fatalf("decode portions %s: %v", split["portions"], err)
	}
	if len(portions) != 2 ||
		portions[0].Target != "SYN-TARGET-01" || portions[0].Amount != "60000" ||
		portions[1].Target != "SYN-TARGET-02" || portions[1].Amount != "30000" {
		t.Fatalf("份额转写走样：%+v", portions)
	}

	// 全额未分摊：来源金额整笔留在未分摊余额上等新依据，份额是空数组，不折进份额凑平。
	none := body.Allocations[1]
	if string(none["unallocatedAmount"]) != `"50000"` ||
		string(none["sourceAmount"]) != `"50000"` ||
		string(none["portions"]) != "[]" {
		t.Fatalf("全额未分摊走样：%s", response.Body.String())
	}
}

// Covers: ADR-0077 Decision 四 — 两本册子空册各自是 2xx + 空数组。经营口径的数字造
// 不得（见 spec 事实基线），空册就是空册。
func TestEmptyOperatingRegistersAnswerEmptyArrays(t *testing.T) {
	endpoint := operatingEndpoint(&stubOperatingCatalogue{})
	for registry, key := range map[string]string{
		"operating-result": "results",
		"cost-allocation":  "allocations",
	} {
		t.Run(registry, func(t *testing.T) {
			response := serveGet(endpoint, "/settlement-operating-results?registry="+registry)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if got := arrayAt(t, response, key); got != "[]" {
				t.Fatalf("空册没有以空数组在场：%s", got)
			}
		})
	}
}

// Covers: ADR-0022/ADR-0029 — 读不回是答案未形成（5xx），不伪装成空册。
func TestFailingOperatingReadsAreNoAnswerRatherThanEmptyRegisters(t *testing.T) {
	endpoint := operatingEndpoint(&stubOperatingCatalogue{err: errors.New("connection refused")})
	for _, registry := range []string{"operating-result", "cost-allocation"} {
		t.Run(registry, func(t *testing.T) {
			response := serveGet(endpoint, "/settlement-operating-results?registry="+registry)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
			}
			if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
				t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
			}
			assertNoOutcome(t, response)
		})
	}
}
