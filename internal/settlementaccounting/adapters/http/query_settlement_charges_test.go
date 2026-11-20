package settlementhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	sapricing "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
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
	_ context.Context, tenant domain.TenantID, limit int, _ cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerChargeCatalogueRow], error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return cataloguepage.Result[ports.CustomerChargeCatalogueRow]{Rows: stub.charges, Total: int64(len(stub.charges))}, stub.err
}

func (stub *stubChargeCatalogue) ListSupplierExpectedCosts(
	_ context.Context, tenant domain.TenantID, limit int, _ cataloguepage.Query,
) (cataloguepage.Result[ports.SupplierExpectedCostCatalogueRow], error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return cataloguepage.Result[ports.SupplierExpectedCostCatalogueRow]{Rows: stub.costs, Total: int64(len(stub.costs))}, stub.err
}

func chargesEndpoint(register *stubChargeCatalogue) http.Handler {
	return settlementhttp.NewQuerySettlementChargesEndpoint(grantedIntake(), register, nil)
}

// Covers: 逐格转写 — 费用身份、阶段、币种三件组、确认留痕与依据数组原样透出；金额以
// 最小单位计数串在场；租户与页大小从作用域来：查询串里自报的租户与页大小是集外键，答 400 不带任何行（ADR-0144 决定四），
// 不带它们照答本租户的行。
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
				// 确认时固定的七项（ADR-0087 决定一）。
				ResponsibleEntity:    "SYN-ENTITY-01",
				Counterparty:         "SYN-PARTY-01",
				ChargeDirection:      "RECEIVABLE",
				SettlementAccount:    "SYN-ACCT-01",
				ContractBasis:        "SYN-CONTRACT-01",
				PrimaryChargingScope: "SYN-SCOPE-01",
				SourceFact:           "SYN-FACT-01",
				FormedAt:             catalogueBaseAt,
				ConfirmedAt:          &confirmedAt,
				RequiredBasisKind:    "DELIVERY_PROOF",
				ConfirmationBases: []ports.ChargeConfirmationBasisEntry{
					{BasisKind: "DELIVERY_PROOF", Basis: "SYN-BASIS-01", RecordedAt: confirmedAt},
				},
			},
		},
	}
	selfReported := serveGet(chargesEndpoint(register), "/settlement-charges?registry=customer-charge&tenant=TENANT-9&limit=9999")
	assertPageQueryRefused(t, selfReported)
	assertNoOutcome(t, selfReported)
	if register.gotTenant != "" {
		t.Fatalf("自报租户与页大小的请求触到了读口：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}

	response := serveGet(chargesEndpoint(register), "/settlement-charges?registry=customer-charge")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d（不取自作用域）",
			register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Charges []struct {
			Charge               string          `json:"charge"`
			FeeItem              string          `json:"feeItem"`
			Evaluation           string          `json:"evaluation"`
			Stage                string          `json:"stage"`
			OriginalCurrency     string          `json:"originalCurrency"`
			OriginalAmount       string          `json:"originalAmount"`
			SettlementCurrency   string          `json:"settlementCurrency"`
			SettlementAmount     string          `json:"settlementAmount"`
			ConversionStep       string          `json:"conversionStep"`
			ConfirmationBasis    string          `json:"confirmationBasis"`
			ResponsibleEntity    string          `json:"responsibleEntity"`
			Counterparty         string          `json:"counterparty"`
			ChargeDirection      string          `json:"chargeDirection"`
			SettlementAccount    string          `json:"settlementAccount"`
			ContractBasis        string          `json:"contractBasis"`
			PrimaryChargingScope string          `json:"primaryChargingScope"`
			SourceFact           string          `json:"sourceFact"`
			FormedAt             string          `json:"formedAt"`
			ConfirmedAt          string          `json:"confirmedAt"`
			RequiredBasisKind    string          `json:"requiredBasisKind"`
			ConfirmationBases    json.RawMessage `json:"confirmationBases"`
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
	// 七项逐格点名：七个键同型，报文里错位一个照样每格都有值。
	for name, pair := range map[string][2]string{
		"responsibleEntity":    {charge.ResponsibleEntity, "SYN-ENTITY-01"},
		"counterparty":         {charge.Counterparty, "SYN-PARTY-01"},
		"chargeDirection":      {charge.ChargeDirection, "RECEIVABLE"},
		"settlementAccount":    {charge.SettlementAccount, "SYN-ACCT-01"},
		"contractBasis":        {charge.ContractBasis, "SYN-CONTRACT-01"},
		"primaryChargingScope": {charge.PrimaryChargingScope, "SYN-SCOPE-01"},
		"sourceFact":           {charge.SourceFact, "SYN-FACT-01"},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s 透出成了 %q，要 %q", name, pair[0], pair[1])
		}
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
	for _, absent := range []string{
		"confirmedAt", "confirmationBasis", "requiredBasisKind", "conversionStep",
		// 未确认行整键不出现，与 confirmedAt 同一处置：那是「这一行还没确认」的正面
		// 形状，透出七个空串会让页面以为册上记漏了（ADR-0087 决定一）。
		"responsibleEntity", "counterparty", "chargeDirection", "settlementAccount",
		"contractBasis", "primaryChargingScope", "sourceFact",
	} {
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

// Covers: 被替代的预估在报文上看得见——有后继时 superseded 与 supersededBy 都在；新评价合计为零、没有后继时只有 superseded 在，
// supersededBy 整键不出现；没被替代的行两键都不出现（为假时省略）。
func TestCustomerChargeQueryShowsASupersessionWithoutASuccessor(t *testing.T) {
	row := func(charge string) ports.CustomerChargeCatalogueRow {
		return ports.CustomerChargeCatalogueRow{
			Charge: charge, FeeItem: "SYN-FEE-01", Evaluation: "SYN-EVAL-01", Stage: "ESTIMATED",
			OriginalCurrency: "SYN-CUR-01", OriginalMinor: 1000, SettlementCurrency: "SYN-CUR-01", SettlementMinor: 1000,
			FormedAt: catalogueBaseAt,
		}
	}
	replaced, zeroed, current := row("SYN-CHG-REPLACED"), row("SYN-CHG-ZEROED"), row("SYN-CHG-CURRENT")
	replaced.Superseded, replaced.SupersededBy = true, "SYN-CHG-NEXT"
	zeroed.Superseded = true
	register := &stubChargeCatalogue{charges: []ports.CustomerChargeCatalogueRow{replaced, zeroed, current}}
	response := serveGet(chargesEndpoint(register), "/settlement-charges?registry=customer-charge")

	var body struct {
		Charges []map[string]json.RawMessage `json:"charges"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Charges) != 3 {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
	if string(body.Charges[0]["superseded"]) != "true" || string(body.Charges[0]["supersededBy"]) != `"SYN-CHG-NEXT"` {
		t.Fatalf("有后继的替代走样：%v", body.Charges[0])
	}
	if string(body.Charges[1]["superseded"]) != "true" {
		t.Fatalf("没有后继的替代没有 superseded：%v", body.Charges[1])
	}
	if _, present := body.Charges[1]["supersededBy"]; present {
		t.Fatalf("没有后继时 supersededBy 不该出现：%v", body.Charges[1])
	}
	for _, absent := range []string{"superseded", "supersededBy"} {
		if _, present := body.Charges[2][absent]; present {
			t.Fatalf("没被替代的行不该带 %q：%v", absent, body.Charges[2])
		}
	}
}

// Covers: 版本链 — 首版的 priorVersion/correctionReason 成对缺席，纠错版本两者成对
// 在场；原版本保留，一份成本的历史是多行而不是一行被改写。`AT-SA-236`（读口半边）：记了供应商的带 supplier，没记的整键不出现。
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
				Supplier:            "SYN-PARTY-01",
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

	// 供应商（ADR-0358 决定四）：记了的照写；首版这一行是没补记的旧行，整键不出现，不填空串或「未知」。
	if string(body.Costs[1]["supplier"]) != `"SYN-PARTY-01"` {
		t.Fatalf("纠错版本丢了供应商：%s", response.Body.String())
	}
	if _, present := body.Costs[0]["supplier"]; present {
		t.Fatalf("没记供应商的旧行带了 supplier 键：%s", response.Body.String())
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

// Covers: AT-SA-223（读口半边）——客户费用行透出归属日与所在对账单，名字是 attributionDate、billedOn；没形成或没出账就不出现这一键，
// 不印成空串。
func TestCustomerChargeQueryShowsTheAttributionDateAndTheStatementItIsOn(t *testing.T) {
	register := &stubChargeCatalogue{
		charges: []ports.CustomerChargeCatalogueRow{
			{Charge: "SYN-CHG-BILLED", Stage: "CONFIRMED", FormedAt: catalogueBaseAt, AttributionDate: "2026-10-31", BilledOn: "SYN-STMT-01"},
			{Charge: "SYN-CHG-OPEN", Stage: "ESTIMATED", FormedAt: catalogueBaseAt},
		},
	}
	response := serveGet(chargesEndpoint(register), "/settlement-charges?registry=customer-charge")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Charges []map[string]any `json:"charges"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Charges[0]["attributionDate"] != "2026-10-31" || body.Charges[0]["billedOn"] != "SYN-STMT-01" {
		t.Fatalf("出账那一行 = %v", body.Charges[0])
	}
	if _, has := body.Charges[1]["attributionDate"]; has {
		t.Fatalf("没形成归属日也印了这一键：%v", body.Charges[1])
	}
	if _, has := body.Charges[1]["billedOn"]; has {
		t.Fatalf("没出账也印了这一键：%v", body.Charges[1])
	}
}

// stubPricedWeights 是计价重量与来源读口的替身：记下被问的次数与两组标识，交回预置的计价重量与来源或故障。
type stubPricedWeights struct {
	origins map[string]settlementhttp.PricedWeightAndOrigin
	err     error
	asked   []string
	strict  []string
	lenient []string
	calls   int
}

func (stub *stubPricedWeights) PricedWeightsAndOrigins(
	_ context.Context,
	_ domain.TenantID,
	mustTranslate settlementhttp.MustTranslateEvaluationRefs,
	absentIfUntranslatable settlementhttp.AbsentIfUntranslatableEvaluationRefs,
) (map[string]settlementhttp.PricedWeightAndOrigin, error) {
	stub.calls++
	stub.strict = append(stub.strict, mustTranslate...)
	stub.lenient = append(stub.lenient, absentIfUntranslatable...)
	stub.asked = append(stub.asked, mustTranslate...)
	stub.asked = append(stub.asked, absentIfUntranslatable...)
	return stub.origins, stub.err
}

// Covers: 客户费用行带上计价重量与它的来源，各占自己的键——pricedWeight 是评价进位之后的计价重量，actualWeight 与
// dimensionSides 是评价用的实重与尺寸，weightSource 说这两样出自实测还是申报。实重不印在计价重量的键上：复测只把尺寸
// 量大时两个数不同。申报没报尺寸就没有尺寸两键；读口没答的评价这几键整个不出现，不印成空串或空数组。
func TestCustomerChargeQueryCarriesThePricedWeightAndWhatItCameFrom(t *testing.T) {
	register := &stubChargeCatalogue{
		charges: []ports.CustomerChargeCatalogueRow{
			{Charge: "SYN-CHG-MEASURED", Evaluation: "SYN-EVAL-01", Stage: "ESTIMATED", FormedAt: catalogueBaseAt},
			{Charge: "SYN-CHG-DECLARED", Evaluation: "SYN-EVAL-02", Stage: "ESTIMATED", FormedAt: catalogueBaseAt},
			{Charge: "SYN-CHG-UNANSWERED", Evaluation: "SYN-EVAL-03", Stage: "ESTIMATED", FormedAt: catalogueBaseAt},
		},
	}
	weights := &stubPricedWeights{origins: map[string]settlementhttp.PricedWeightAndOrigin{
		"SYN-EVAL-01": {
			Kind: "actual-measurement", PricedWeight: "3", PricedWeightUnit: "KG",
			ActualWeight: "2.1", ActualWeightUnit: "KG",
			DimensionSides: []string{"30", "25", "20"}, DimensionUnit: "CM",
			MeasurementSource: "SYN-WEIGH-M1",
		},
		"SYN-EVAL-02": {
			Kind: "declared-measurement", PricedWeight: "1.5", PricedWeightUnit: "KG",
			ActualWeight: "1.2", ActualWeightUnit: "KG",
		},
	}}
	endpoint := settlementhttp.NewQuerySettlementChargesEndpoint(grantedIntake(), register, weights)
	response := serveGet(endpoint, "/settlement-charges?registry=customer-charge")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if asked := "SYN-EVAL-01,SYN-EVAL-02,SYN-EVAL-03"; strings.Join(weights.asked, ",") != asked || weights.calls != 1 {
		t.Fatalf("问来源读口 %d 次、评价 = %v，want 一次且 %s", weights.calls, weights.asked, asked)
	}
	var body struct {
		Charges []map[string]any `json:"charges"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	originKeys := []string{
		"weightSource", "pricedWeight", "pricedWeightUnit", "actualWeight", "actualWeightUnit", "dimensionSides", "dimensionUnit",
		"measurementSource",
	}
	originOf := func(row map[string]any) map[string]any {
		picked := map[string]any{}
		for _, key := range originKeys {
			if value, has := row[key]; has {
				picked[key] = value
			}
		}
		return picked
	}
	want := []map[string]any{
		{
			"weightSource": "actual-measurement", "pricedWeight": "3", "pricedWeightUnit": "KG",
			"actualWeight": "2.1", "actualWeightUnit": "KG",
			"dimensionSides": []any{"30", "25", "20"}, "dimensionUnit": "CM",
			"measurementSource": "SYN-WEIGH-M1",
		},
		{
			"weightSource": "declared-measurement", "pricedWeight": "1.5", "pricedWeightUnit": "KG",
			"actualWeight": "1.2", "actualWeightUnit": "KG",
		},
		{},
	}
	for index, row := range body.Charges {
		if got := originOf(row); !reflect.DeepEqual(got, want[index]) {
			t.Fatalf("%v 的计价重量与来源 = %v\nwant %v", row["charge"], got, want[index])
		}
	}
}

// Covers: 来源读口读不回时整份答未形成。照样列出费用而把这几格留空，会把一次该重试的故障读成「这些费用没有计价重量」。
func TestAnUnreadablePricedWeightAndOriginIsNoAnswerRatherThanBlankCells(t *testing.T) {
	register := &stubChargeCatalogue{
		charges: []ports.CustomerChargeCatalogueRow{
			{Charge: "SYN-CHG-01", Evaluation: "SYN-EVAL-01", Stage: "ESTIMATED", FormedAt: catalogueBaseAt},
		},
	}
	weights := &stubPricedWeights{err: errors.New("evaluation register unreachable")}
	endpoint := settlementhttp.NewQuerySettlementChargesEndpoint(grantedIntake(), register, weights)
	response := serveGet(endpoint, "/settlement-charges?registry=customer-charge")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d；body = %s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
	if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
	}
	assertNoOutcome(t, response)
}

// Covers: 调整的计价重量与来源来自调整自己的评价，不来自费用行那一份——两份评价的计价重量不同，串起来才证得出取的是哪一份。
// 商业让利没有评价，JSON 里不出现 evaluation 与重量各键。既有四键的名字与值不动。费用行与调整只问读口一次。
func TestCustomerChargeAdjustmentsCarryTheWeightOfTheirOwnEvaluation(t *testing.T) {
	register := &stubChargeCatalogue{
		charges: []ports.CustomerChargeCatalogueRow{{
			Charge: "SYN-CHG-01", Evaluation: "SYN-EVAL-CHARGE", Stage: "CONFIRMED",
			OriginalCurrency: "CNY", OriginalMinor: 10000, SettlementCurrency: "CNY", SettlementMinor: 10000,
			FormedAt: catalogueBaseAt,
			Adjustments: []ports.CustomerChargeAdjustmentEntry{
				{
					Adjustment: "SYN-ADJ-DEBIT", Kind: "PRICING_CORRECTION", Direction: "DEBIT",
					SettlementMinor: 1500, Evaluation: "SYN-EVAL-REWEIGH",
				},
				{
					Adjustment: "SYN-ADJ-CONC", Kind: "COMMERCIAL_CONCESSION", Direction: "CREDIT",
					SettlementMinor: 400,
				},
			},
		}},
	}
	weights := &stubPricedWeights{origins: map[string]settlementhttp.PricedWeightAndOrigin{
		"SYN-EVAL-CHARGE": {
			Kind: "actual-measurement", PricedWeight: "2.5", PricedWeightUnit: "KG",
			ActualWeight: "2.1", ActualWeightUnit: "KG",
			DimensionSides: []string{"22", "16", "11"}, DimensionUnit: "CM",
			MeasurementSource: "SYN-WEIGH-M1",
		},
		"SYN-EVAL-REWEIGH": {
			Kind: "actual-measurement", PricedWeight: "3", PricedWeightUnit: "KG",
			ActualWeight: "2.1", ActualWeightUnit: "KG",
			DimensionSides: []string{"30", "25", "20"}, DimensionUnit: "CM",
			MeasurementSource: "SYN-WEIGH-M2",
		},
	}}
	response := serveGet(settlementhttp.NewQuerySettlementChargesEndpoint(grantedIntake(), register, weights),
		"/settlement-charges?registry=customer-charge")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if weights.calls != 1 {
		t.Fatalf("问了 %d 次，want 一次", weights.calls)
	}
	if !reflect.DeepEqual(weights.strict, []string{"SYN-EVAL-CHARGE"}) ||
		!reflect.DeepEqual(weights.lenient, []string{"SYN-EVAL-REWEIGH"}) {
		t.Fatalf("两组标识 strict=%v lenient=%v", weights.strict, weights.lenient)
	}
	var body struct {
		Charges []map[string]any `json:"charges"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	row := body.Charges[0]
	if row["pricedWeight"] != "2.5" || row["measurementSource"] != "SYN-WEIGH-M1" {
		t.Fatalf("费用行取成了调整的重量：%v", row)
	}
	adjustments, _ := row["adjustments"].([]any)
	if len(adjustments) != 2 {
		t.Fatalf("调整 = %v", row["adjustments"])
	}
	debit, _ := adjustments[0].(map[string]any)
	if debit["adjustment"] != "SYN-ADJ-DEBIT" || debit["kind"] != "PRICING_CORRECTION" ||
		debit["direction"] != "DEBIT" || debit["settlementAmount"] != "1500" ||
		debit["evaluation"] != "SYN-EVAL-REWEIGH" || debit["pricedWeight"] != "3" ||
		debit["weightSource"] != "actual-measurement" || debit["measurementSource"] != "SYN-WEIGH-M2" ||
		!reflect.DeepEqual(debit["dimensionSides"], []any{"30", "25", "20"}) || debit["dimensionUnit"] != "CM" {
		t.Fatalf("计价纠错 = %v", debit)
	}
	concession, _ := adjustments[1].(map[string]any)
	if concession["adjustment"] != "SYN-ADJ-CONC" || concession["kind"] != "COMMERCIAL_CONCESSION" ||
		concession["direction"] != "CREDIT" || concession["settlementAmount"] != "400" {
		t.Fatalf("商业让利的既有四格 = %v", concession)
	}
	for _, key := range []string{"evaluation", "pricedWeight", "pricedWeightUnit", "weightSource", "actualWeight", "actualWeightUnit", "dimensionSides", "dimensionUnit", "measurementSource"} {
		if _, present := concession[key]; present {
			t.Fatalf("商业让利不该有 %s：%v", key, concession)
		}
	}
}

// Covers: 调整的评价在计价一侧造不出标识，或登记册里没有，整页仍 200：这笔调整只带 evaluation，没有重量各键；同页另一笔费用的计价重量还在。
// 费用行自己的评价造不出标识仍是 500，登记册不被问。同一条造不出的引用两边都在时也是 500。
func TestAnAdjustmentEvaluationThatCannotBeReadDoesNotBlankThePage(t *testing.T) {
	chargeEval := completedChargeEvaluation(t, "SYN-EVAL-CHARGE", "2.10")
	sibling := completedChargeEvaluation(t, "SYN-EVAL-SIBLING", "5")
	chargeWeight, chargePresent := chargeEval.PricingWeight()
	siblingWeight, siblingPresent := sibling.PricingWeight()
	if !chargePresent || !siblingPresent {
		t.Fatal("夹具没有计价重量")
	}
	wantCharge := chargeWeight.RoundedWeight().Value().String()
	wantSibling := siblingWeight.RoundedWeight().Value().String()
	registered := map[string]ppdomain.PricingEvaluation{
		chargeEval.ID().String(): chargeEval,
		sibling.ID().String():    sibling,
	}

	page := func(chargeEvaluation, adjustmentEvaluation string) []ports.CustomerChargeCatalogueRow {
		return []ports.CustomerChargeCatalogueRow{
			{
				Charge: "SYN-CHG-ADJ", Evaluation: chargeEvaluation, Stage: "CONFIRMED",
				OriginalCurrency: "CNY", OriginalMinor: 10000, SettlementCurrency: "CNY", SettlementMinor: 10000,
				FormedAt: catalogueBaseAt,
				Adjustments: []ports.CustomerChargeAdjustmentEntry{{
					Adjustment: "SYN-ADJ-01", Kind: "PRICING_CORRECTION", Direction: "DEBIT",
					SettlementMinor: 1500, Evaluation: adjustmentEvaluation,
				}},
			},
			{
				Charge: "SYN-CHG-OK", Evaluation: "SYN-EVAL-SIBLING", Stage: "CONFIRMED",
				OriginalCurrency: "CNY", OriginalMinor: 800, SettlementCurrency: "CNY", SettlementMinor: 800,
				FormedAt: catalogueBaseAt,
			},
		}
	}
	assertAdjustmentAbsent := func(t *testing.T, responseBody []byte) {
		t.Helper()
		var body struct {
			Charges []map[string]any `json:"charges"`
		}
		if err := json.Unmarshal(responseBody, &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Charges) != 2 || body.Charges[0]["pricedWeight"] != wantCharge ||
			body.Charges[1]["charge"] != "SYN-CHG-OK" || body.Charges[1]["pricedWeight"] != wantSibling ||
			body.Charges[1]["settlementAmount"] != "800" {
			t.Fatalf("同页费用行 = %v，want 计价重量 %s 与 %s 仍在", body.Charges, wantCharge, wantSibling)
		}
		adjustments, _ := body.Charges[0]["adjustments"].([]any)
		adjustment, _ := adjustments[0].(map[string]any)
		if adjustment["evaluation"] == "" || adjustment["settlementAmount"] != "1500" {
			t.Fatalf("调整 = %v", adjustment)
		}
		for _, key := range []string{"pricedWeight", "pricedWeightUnit", "weightSource", "actualWeight", "actualWeightUnit", "dimensionSides", "dimensionUnit", "measurementSource"} {
			if _, present := adjustment[key]; present {
				t.Fatalf("读不出的调整不该有 %s：%v", key, adjustment)
			}
		}
	}

	t.Run("untranslatable", func(t *testing.T) {
		register := &weightRegisterDouble{byID: registered}
		response := serveGet(chargesEndpointWithRegister(t, register, page("SYN-EVAL-CHARGE", " eval-padded ")),
			"/settlement-charges?registry=customer-charge")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		if register.calls != 1 || !reflect.DeepEqual(register.asked, []string{"SYN-EVAL-CHARGE", "SYN-EVAL-SIBLING"}) {
			t.Fatalf("问登记册 %d 次 %v，want 一次且不把造不出的标识拿去问", register.calls, register.asked)
		}
		assertAdjustmentAbsent(t, response.Body.Bytes())
		var body struct {
			Charges []map[string]any `json:"charges"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		adjustments, _ := body.Charges[0]["adjustments"].([]any)
		adjustment, _ := adjustments[0].(map[string]any)
		if adjustment["evaluation"] != " eval-padded " {
			t.Fatalf("evaluation = %v", adjustment["evaluation"])
		}
	})

	t.Run("not in the register", func(t *testing.T) {
		register := &weightRegisterDouble{byID: registered}
		response := serveGet(chargesEndpointWithRegister(t, register, page("SYN-EVAL-CHARGE", "SYN-EVAL-MISSING")),
			"/settlement-charges?registry=customer-charge")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		if register.calls != 1 || !reflect.DeepEqual(register.asked, []string{"SYN-EVAL-CHARGE", "SYN-EVAL-SIBLING", "SYN-EVAL-MISSING"}) {
			t.Fatalf("问登记册 %d 次 %v", register.calls, register.asked)
		}
		assertAdjustmentAbsent(t, response.Body.Bytes())
	})

	t.Run("the charge's own reference", func(t *testing.T) {
		register := &weightRegisterDouble{byID: registered}
		response := serveGet(chargesEndpointWithRegister(t, register, page(" eval-padded ", " eval-padded ")),
			"/settlement-charges?registry=customer-charge")
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d body = %s，want 500", response.Code, response.Body.String())
		}
		if got := problemCode(t, response); got != "NO_ANSWER_FORMED" {
			t.Fatalf("code = %q, want NO_ANSWER_FORMED", got)
		}
		assertNoOutcome(t, response)
		if register.calls != 0 {
			t.Fatalf("费用行自己的引用造不出标识仍去问了登记册 %d 次", register.calls)
		}
	})
}

// weightRegisterDouble 是计价评价登记册按标识读的替身：只交回被问到且预置了的评价。
type weightRegisterDouble struct {
	byID  map[string]ppdomain.PricingEvaluation
	calls int
	asked []string
}

func (double *weightRegisterDouble) FindByIDs(
	_ context.Context, _ ppdomain.TenantID, ids []ppdomain.EvaluationID,
) ([]ppdomain.PricingEvaluation, error) {
	double.calls++
	found := make([]ppdomain.PricingEvaluation, 0, len(ids))
	for _, id := range ids {
		double.asked = append(double.asked, id.String())
		if evaluation, ok := double.byID[id.String()]; ok {
			found = append(found, evaluation)
		}
	}
	return found, nil
}

func chargesEndpointWithRegister(t *testing.T, register *weightRegisterDouble, charges []ports.CustomerChargeCatalogueRow) http.Handler {
	t.Helper()
	origins, err := sapricing.NewPricedWeightsAndOrigins(register)
	if err != nil {
		t.Fatal(err)
	}
	return settlementhttp.NewQuerySettlementChargesEndpoint(grantedIntake(), &stubChargeCatalogue{charges: charges}, httpWeightBridge{origins: origins})
}

type httpWeightBridge struct {
	origins *sapricing.PricedWeightsAndOrigins
}

func (bridge httpWeightBridge) PricedWeightsAndOrigins(
	ctx context.Context,
	tenant domain.TenantID,
	mustTranslate settlementhttp.MustTranslateEvaluationRefs,
	absentIfUntranslatable settlementhttp.AbsentIfUntranslatableEvaluationRefs,
) (map[string]settlementhttp.PricedWeightAndOrigin, error) {
	loaded, err := bridge.origins.PricedWeightsAndOrigins(ctx, tenant,
		sapricing.MustTranslateEvaluationRefs(mustTranslate),
		sapricing.AbsentIfUntranslatableEvaluationRefs(absentIfUntranslatable))
	if err != nil {
		return nil, err
	}
	answered := make(map[string]settlementhttp.PricedWeightAndOrigin, len(loaded))
	for id, origin := range loaded {
		answered[id] = settlementhttp.PricedWeightAndOrigin(origin)
	}
	return answered, nil
}

func completedChargeEvaluation(t *testing.T, id, kilograms string) ppdomain.PricingEvaluation {
	t.Helper()
	period := pptest.Period{
		StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	card := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-SELL-CARD-HALF-KG", "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-SELL-TABLE-HALF-KG", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-SELL-WEIGHT-HALF-KG", "v1"),
		Scope:                 "scope-1",
		Direction:             ppdomain.PricingDirectionSell,
		Purpose:               ppdomain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period:                period,
		RateEntryID:           "entry",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "10",
		RateAmount:            "100",
		WeightRounding:        ppdomain.RoundingCeiling,
		WeightStepKilograms:   "0.5",
		AmountRounding: &pptest.AmountRoundingSpec{
			Mode:      ppdomain.RoundingHalfUp,
			Increment: "0.01",
			Points:    []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		},
	})
	evaluation := pptest.Evaluate(t, id, card, pptest.Input(t, pptest.InputSpec{
		Tenant: "TENANT-1", Scope: "scope-1", PackageID: "pkg-1", Zone: "Z1", Kilograms: kilograms, BusinessAt: catalogueBaseAt,
	}))
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("夹具 %s 状态 = %s %#v", id, evaluation.Status(), evaluation.Issues())
	}
	return evaluation
}
