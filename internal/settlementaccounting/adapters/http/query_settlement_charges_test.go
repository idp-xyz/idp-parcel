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

// stubChargeCatalogue 是费用页两本册子的读口替身：记录收到的键，交回预置的册子或故障。
type stubChargeCatalogue struct {
	charges []ports.CustomerChargeCatalogueRow
	costs   []ports.SupplierExpectedCostCatalogueRow
	err     error

	gotTenant string
	gotLimit  int
}

func (stub *stubChargeCatalogue) ListCustomerCharges(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.CustomerChargeCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.charges, stub.err
}

func (stub *stubChargeCatalogue) ListSupplierExpectedCosts(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.SupplierExpectedCostCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.costs, stub.err
}

func chargesEndpoint(register *stubChargeCatalogue) http.Handler {
	return settlementhttp.NewQuerySettlementChargesEndpoint(grantedIntake(), register)
}

// Covers: 逐格转写 — 费用身份、阶段、币种三件组、确认留痕与依据数组原样透出；金额以
// 最小单位计数串在场；租户与页大小从作用域来，不采信请求自报。
func TestCustomerChargeQueryTranscribesChargesVerbatim(t *testing.T) {
	confirmedAt := catalogueBaseAt.Add(48 * time.Hour)
	register := &stubChargeCatalogue{
		charges: []ports.CustomerChargeCatalogueRow{
			{
				Charge:             "SYN-CHG-01",
				FeeItem:            "SYN-FEE-01",
				Evaluation:         "SYN-EVAL-01",
				Stage:              "CONFIRMED",
				OriginalCurrency:   "SYN-CUR-01",
				OriginalMinor:      120000,
				SettlementCurrency: "SYN-CUR-02",
				SettlementMinor:    98000,
				ConversionStep:     "SYN-FX-01",
				ConfirmationBasis:  "SYN-BASIS-01",
				FormedAt:           catalogueBaseAt,
				ConfirmedAt:        &confirmedAt,
				RequiredBasisKind:  "DELIVERY_PROOF",
				ConfirmationBases: []ports.ChargeConfirmationBasisEntry{
					{BasisKind: "DELIVERY_PROOF", Basis: "SYN-BASIS-01", RecordedAt: confirmedAt},
				},
			},
		},
	}
	response := serveGet(chargesEndpoint(register), "/settlement-charges?registry=customer-charge&tenant=TENANT-9&limit=9999")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d（自报查询参数被采信了）",
			register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Charges []struct {
			Charge             string          `json:"charge"`
			FeeItem            string          `json:"feeItem"`
			Evaluation         string          `json:"evaluation"`
			Stage              string          `json:"stage"`
			OriginalCurrency   string          `json:"originalCurrency"`
			OriginalAmount     string          `json:"originalAmount"`
			SettlementCurrency string          `json:"settlementCurrency"`
			SettlementAmount   string          `json:"settlementAmount"`
			ConversionStep     string          `json:"conversionStep"`
			ConfirmationBasis  string          `json:"confirmationBasis"`
			FormedAt           string          `json:"formedAt"`
			ConfirmedAt        string          `json:"confirmedAt"`
			RequiredBasisKind  string          `json:"requiredBasisKind"`
			ConfirmationBases  json.RawMessage `json:"confirmationBases"`
		} `json:"charges"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "CUSTOMER_CHARGES_LISTED" || len(body.Charges) != 1 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
	charge := body.Charges[0]
	if charge.Charge != "SYN-CHG-01" || charge.FeeItem != "SYN-FEE-01" ||
		charge.Evaluation != "SYN-EVAL-01" || charge.Stage != "CONFIRMED" ||
		charge.FormedAt != catalogueBaseAt.Format(time.RFC3339Nano) ||
		charge.ConfirmedAt != confirmedAt.Format(time.RFC3339Nano) ||
		charge.ConfirmationBasis != "SYN-BASIS-01" || charge.RequiredBasisKind != "DELIVERY_PROOF" {
		t.Fatalf("费用面转写走样：%+v", charge)
	}
	// 币种三件组整组在场：拆开呈现会让读者以为可以各自停在不同评价上。
	if charge.OriginalCurrency != "SYN-CUR-01" || charge.OriginalAmount != "120000" ||
		charge.SettlementCurrency != "SYN-CUR-02" || charge.SettlementAmount != "98000" ||
		charge.ConversionStep != "SYN-FX-01" {
		t.Fatalf("币种三件组走样：%+v", charge)
	}
	var bases []struct {
		BasisKind  string `json:"basisKind"`
		Basis      string `json:"basis"`
		RecordedAt string `json:"recordedAt"`
	}
	if err := json.Unmarshal(charge.ConfirmationBases, &bases); err != nil {
		t.Fatalf("decode bases %s: %v", charge.ConfirmationBases, err)
	}
	if len(bases) != 1 || bases[0].BasisKind != "DELIVERY_PROOF" ||
		bases[0].Basis != "SYN-BASIS-01" ||
		bases[0].RecordedAt != confirmedAt.Format(time.RFC3339Nano) {
		t.Fatalf("确认依据转写走样：%+v", bases)
	}
}

// Covers: 两种缺席分得开 — 未确认的费用不带确认三键（不编造零时刻）；「条件目录没
// 这一行」（无 requiredBasisKind）与「配了但依据没到」（有 requiredBasisKind、依据
// 数组为空）是两个答案，读面分两格摆开，不共用一格。
func TestCustomerChargeQueryKeepsUnconfiguredConditionApartFromMissingBasis(t *testing.T) {
	register := &stubChargeCatalogue{
		charges: []ports.CustomerChargeCatalogueRow{
			{
				Charge:             "SYN-CHG-UNCONFIGURED",
				FeeItem:            "SYN-FEE-NOCOND",
				Evaluation:         "SYN-EVAL-02",
				Stage:              "ESTIMATED",
				OriginalCurrency:   "SYN-CUR-01",
				OriginalMinor:      1000,
				SettlementCurrency: "SYN-CUR-01",
				SettlementMinor:    1000,
				FormedAt:           catalogueBaseAt,
			},
			{
				Charge:             "SYN-CHG-AWAITING",
				FeeItem:            "SYN-FEE-01",
				Evaluation:         "SYN-EVAL-03",
				Stage:              "PROVISIONAL",
				OriginalCurrency:   "SYN-CUR-01",
				OriginalMinor:      2000,
				SettlementCurrency: "SYN-CUR-01",
				SettlementMinor:    2000,
				FormedAt:           catalogueBaseAt,
				RequiredBasisKind:  "DELIVERY_PROOF",
			},
		},
	}
	response := serveGet(chargesEndpoint(register), "/settlement-charges?registry=customer-charge")

	var body struct {
		Charges []map[string]json.RawMessage `json:"charges"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if len(body.Charges) != 2 {
		t.Fatalf("行数走样：%s", response.Body.String())
	}

	unconfigured := body.Charges[0]
	for _, absent := range []string{"confirmedAt", "confirmationBasis", "requiredBasisKind", "conversionStep"} {
		if _, present := unconfigured[absent]; present {
			t.Fatalf("缺席的 %q 不该出现在报文里：%s", absent, response.Body.String())
		}
	}
	// 同币种时换算依据不在场是正面结果（库上 conversion_present 守着），不是缺数据；
	// 空依据数组仍须在场，页面判「没有依据」不该先判「有没有这个键」。
	if string(unconfigured["confirmationBases"]) != "[]" {
		t.Fatalf("空依据没有以空数组在场：%s", unconfigured["confirmationBases"])
	}

	awaiting := body.Charges[1]
	if string(awaiting["requiredBasisKind"]) != `"DELIVERY_PROOF"` {
		t.Fatalf("配了条件的那行丢了 requiredBasisKind：%s", response.Body.String())
	}
	if string(awaiting["confirmationBases"]) != "[]" {
		t.Fatalf("依据未到该是空数组：%s", awaiting["confirmationBases"])
	}
	if _, present := awaiting["confirmedAt"]; present {
		t.Fatalf("暂估行不该带确认时刻：%s", response.Body.String())
	}
}

// Covers: 版本链 — 首版的 priorVersion/correctionReason 成对缺席，纠错版本两者成对
// 在场；原版本保留，一份成本的历史是多行而不是一行被改写。
func TestSupplierExpectedCostQueryKeepsTheVersionChain(t *testing.T) {
	register := &stubChargeCatalogue{
		costs: []ports.SupplierExpectedCostCatalogueRow{
			{
				Version:             "SYN-COST-V1",
				Occurrence:          "SYN-OCC-01",
				OccurrenceReason:    "BOOKING",
				OccurrenceVersion:   "V1",
				OccurredAt:          catalogueBaseAt,
				FeeItem:             "SYN-FEE-01",
				PurchaseRuleVersion: "SYN-RULE-01",
				Agreement:           "SYN-AGR-01",
				Evaluation:          "SYN-BUY-01",
				OriginalCurrency:    "SYN-CUR-01",
				OriginalMinor:       40000,
				SettlementCurrency:  "SYN-CUR-01",
				SettlementMinor:     40000,
				RecordedAt:          catalogueBaseAt,
			},
			{
				Version:             "SYN-COST-V2",
				Occurrence:          "SYN-OCC-01",
				OccurrenceReason:    "BOOKING",
				OccurrenceVersion:   "V1",
				OccurredAt:          catalogueBaseAt,
				FeeItem:             "SYN-FEE-01",
				PurchaseRuleVersion: "SYN-RULE-02",
				Agreement:           "SYN-AGR-01",
				Evaluation:          "SYN-BUY-02",
				OriginalCurrency:    "SYN-CUR-01",
				OriginalMinor:       41000,
				SettlementCurrency:  "SYN-CUR-01",
				SettlementMinor:     41000,
				PriorVersion:        "SYN-COST-V1",
				CorrectionReason:    "PRICING_CORRECTION",
				RecordedAt:          catalogueBaseAt.Add(time.Hour),
			},
		},
	}
	response := serveGet(chargesEndpoint(register), "/settlement-charges?registry=supplier-expected-cost")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Outcome string                       `json:"outcome"`
		Costs   []map[string]json.RawMessage `json:"costs"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "SUPPLIER_EXPECTED_COSTS_LISTED" || len(body.Costs) != 2 {
		t.Fatalf("响应走样：%s", response.Body.String())
	}

	first := body.Costs[0]
	if string(first["version"]) != `"SYN-COST-V1"` ||
		string(first["purchaseRuleVersion"]) != `"SYN-RULE-01"` ||
		string(first["evaluation"]) != `"SYN-BUY-01"` ||
		string(first["originalAmount"]) != `"40000"` ||
		string(first["settlementAmount"]) != `"40000"` {
		t.Fatalf("首版转写走样：%s", response.Body.String())
	}
	for _, absent := range []string{"priorVersion", "correctionReason", "conversionStep"} {
		if _, present := first[absent]; present {
			t.Fatalf("首版不该带 %q：%s", absent, response.Body.String())
		}
	}

	// 整组重述：计价纠错换评价，原币与结算金额随新评价一起改（同币种两额必须相等，
	// 迁移 0012 依 ADR-0067 撤了纠错版本的例外支）。
	correction := body.Costs[1]
	if string(correction["priorVersion"]) != `"SYN-COST-V1"` ||
		string(correction["correctionReason"]) != `"PRICING_CORRECTION"` ||
		string(correction["evaluation"]) != `"SYN-BUY-02"` ||
		string(correction["purchaseRuleVersion"]) != `"SYN-RULE-02"` ||
		string(correction["originalAmount"]) != `"41000"` ||
		string(correction["settlementAmount"]) != `"41000"` {
		t.Fatalf("纠错版本转写走样：%s", response.Body.String())
	}
	// 账单主张、审核应付与付款字段不在预期成本行上——那道分界在表上与在报文上同样
	// 是结构性的。
	for _, absent := range []string{"claim", "auditedPayable", "paidAt"} {
		if _, present := correction[absent]; present {
			t.Fatalf("预期成本行长出了 %q：%s", absent, response.Body.String())
		}
	}
}

// Covers: ADR-0077 Decision 四 — 两本册子空册各自是 2xx + 空数组（不是 null）。
func TestEmptyChargeRegistersAnswerEmptyArrays(t *testing.T) {
	endpoint := chargesEndpoint(&stubChargeCatalogue{})
	for registry, key := range map[string]string{
		"customer-charge":        "charges",
		"supplier-expected-cost": "costs",
	} {
		t.Run(registry, func(t *testing.T) {
			response := serveGet(endpoint, "/settlement-charges?registry="+registry)
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
func TestFailingChargeReadsAreNoAnswerRatherThanEmptyRegisters(t *testing.T) {
	endpoint := chargesEndpoint(&stubChargeCatalogue{err: errors.New("connection refused")})
	for _, registry := range []string{"customer-charge", "supplier-expected-cost"} {
		t.Run(registry, func(t *testing.T) {
			response := serveGet(endpoint, "/settlement-charges?registry="+registry)
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
