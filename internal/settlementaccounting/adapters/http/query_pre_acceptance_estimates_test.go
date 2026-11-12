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

type estimateReaderDouble struct {
	entries     []ports.PreAcceptanceEstimateEntry
	err         error
	askedTenant string
	askedFor    string
}

func (double *estimateReaderDouble) ListPreAcceptanceEstimates(
	_ context.Context, tenant domain.TenantID, shipmentRequest string,
) ([]ports.PreAcceptanceEstimateEntry, error) {
	double.askedTenant, double.askedFor = tenant.String(), shipmentRequest
	return double.entries, double.err
}

// Covers: 票 declared-goods-lines/19 钉住的答复形状（ADR-0379 决定二）——逐版一份：价卡拆成方案、版本与指纹，合计与逐件合计是最小币
// 单位整数串配币种，费用行是十进制原文配卡本币，占用是数组（冻结与信用暴露各用各的状态词，释放、转出时刻没有即缺键）；补记之前的
// 一份 detailRecorded 为假、没有产品与逐件明细；受限无占用答空数组。租户取自准入，委托原样交读口。
func TestPreAcceptanceEstimatesAnswerThePinnedShape(t *testing.T) {
	at := time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)
	released := at.Add(4 * time.Hour)
	reader := &estimateReaderDouble{entries: []ports.PreAcceptanceEstimateEntry{
		{
			SubmissionVersion: "SUBV-1", Form: "PER_PARCEL_SELL_TRIAL_SUM", PricingScope: "SYN-SCOPE-SELL",
			PriceCard:      domain.VersionReferenceParts{Kind: "pricing-plan", ID: "SYN-PLAN-FBA", Version: "v2", Fingerprint: "c0ffee"},
			ServiceProduct: "SYN-PRODUCT-FBA", DetailRecorded: true, TotalMinor: 1536, Currency: "CNY",
			TrialAt: at, RecordedAt: at.Add(time.Minute),
			Parcels: []ports.PreAcceptanceEstimateParcel{{
				Parcel: "DPCL-1", Evaluation: "EVAL-1", TotalMinor: 1536, Currency: "CNY",
				PricingWeight: &ports.PreAcceptanceParcelPricingWeight{Value: "12.5", Unit: "KG"},
				ChargeLines:   []ports.PreAcceptanceParcelChargeLine{{Code: "BASE_FREIGHT", Effect: "ADD", Amount: "15.36", Currency: "CNY"}},
			}},
			Occupations: []ports.PreAcceptanceOccupation{
				{Kind: ports.OccupationFundsFreeze, ID: "FRZ-1", Status: "HELD", AmountMinor: 1536, Currency: "CNY",
					FormedAt: at.Add(time.Minute), ConvertedMinor: 0},
				{Kind: ports.OccupationCreditExposure, ID: "EXP-1", Status: "RELEASED", AmountMinor: 1536, Currency: "CNY",
					FormedAt: at.Add(time.Minute), ReleasedAt: &released},
			},
		},
		{
			SubmissionVersion: "SUBV-0", Form: "PER_PARCEL_SELL_TRIAL_SUM", PricingScope: "SYN-SCOPE-SELL",
			PriceCard:  domain.VersionReferenceParts{Kind: "pricing-plan", ID: "SYN-PLAN-FBA", Version: "v1"},
			TotalMinor: 800, Currency: "CNY", TrialAt: at.Add(-time.Hour), RecordedAt: at.Add(-time.Hour),
			Parcels:     []ports.PreAcceptanceEstimateParcel{{Parcel: "DPCL-1", Evaluation: "EVAL-0", TotalMinor: 800, Currency: "CNY"}},
			Occupations: []ports.PreAcceptanceOccupation{},
		},
	}}
	response := serveGet(settlementhttp.NewQueryPreAcceptanceEstimatesEndpoint(grantedIntake(), reader),
		"/settlement-pre-acceptance-estimates?shipmentRequest=SHR-1")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
	if reader.askedTenant != "TENANT-1" || reader.askedFor != "SHR-1" {
		t.Fatalf("读口被问 %q / %q", reader.askedTenant, reader.askedFor)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := map[string]any{
		"outcome": "PRE_ACCEPTANCE_ESTIMATES_LISTED",
		"estimates": []any{
			map[string]any{
				"submissionVersion": "SUBV-1", "form": "PER_PARCEL_SELL_TRIAL_SUM", "pricingScope": "SYN-SCOPE-SELL",
				"priceCard":      map[string]any{"plan": "SYN-PLAN-FBA", "version": "v2", "fingerprint": "c0ffee"},
				"serviceProduct": "SYN-PRODUCT-FBA", "detailRecorded": true,
				"totalAmount": "1536", "currency": "CNY",
				"trialAt": "2026-11-11T08:00:00Z", "recordedAt": "2026-11-11T08:01:00Z",
				"parcels": []any{map[string]any{
					"parcel": "DPCL-1", "evaluation": "EVAL-1", "totalAmount": "1536", "currency": "CNY",
					"pricingWeight": map[string]any{"value": "12.5", "unit": "KG"},
					"chargeLines":   []any{map[string]any{"code": "BASE_FREIGHT", "effect": "ADD", "amount": "15.36", "currency": "CNY"}},
				}},
				"occupations": []any{
					map[string]any{"kind": "FUNDS_FREEZE", "id": "FRZ-1", "status": "HELD", "amount": "1536", "currency": "CNY",
						"formedAt": "2026-11-11T08:01:00Z", "convertedAmount": "0"},
					map[string]any{"kind": "CREDIT_EXPOSURE", "id": "EXP-1", "status": "RELEASED", "amount": "1536", "currency": "CNY",
						"formedAt": "2026-11-11T08:01:00Z", "releasedAt": "2026-11-11T12:00:00Z", "convertedAmount": "0"},
				},
			},
			map[string]any{
				"submissionVersion": "SUBV-0", "form": "PER_PARCEL_SELL_TRIAL_SUM", "pricingScope": "SYN-SCOPE-SELL",
				"priceCard":      map[string]any{"plan": "SYN-PLAN-FBA", "version": "v1"},
				"detailRecorded": false,
				"totalAmount":    "800", "currency": "CNY",
				"trialAt": "2026-11-11T07:00:00Z", "recordedAt": "2026-11-11T07:00:00Z",
				"parcels":     []any{map[string]any{"parcel": "DPCL-1", "evaluation": "EVAL-0", "totalAmount": "800", "currency": "CNY"}},
				"occupations": []any{},
			},
		},
	}
	got, _ := json.Marshal(body)
	expected, _ := json.Marshal(want)
	if string(got) != string(expected) {
		t.Fatalf("答复\n%s\n想要\n%s", got, expected)
	}
}

// Covers: 没有留痕答 LISTED 加空数组，不是 404；shipmentRequest 缺即坏请求；读口故障是没形成答案的 500，不折成空列表。
func TestPreAcceptanceEstimatesAnswerEmptyMalformedAndFailure(t *testing.T) {
	empty := serveGet(settlementhttp.NewQueryPreAcceptanceEstimatesEndpoint(grantedIntake(), &estimateReaderDouble{}),
		"/settlement-pre-acceptance-estimates?shipmentRequest=SHR-NONE")
	if empty.Code != http.StatusOK || arrayAt(t, empty, "estimates") != "[]" {
		t.Fatalf("没有留痕：%d %s", empty.Code, empty.Body)
	}
	for _, target := range []string{"/settlement-pre-acceptance-estimates", "/settlement-pre-acceptance-estimates?shipmentRequest="} {
		malformed := serveGet(settlementhttp.NewQueryPreAcceptanceEstimatesEndpoint(grantedIntake(), unreachableCatalogues{t: t}), target)
		if malformed.Code != http.StatusBadRequest || problemCode(t, malformed) != "MALFORMED_REQUEST" {
			t.Fatalf("%s：%d %s", target, malformed.Code, malformed.Body)
		}
	}
	down := serveGet(settlementhttp.NewQueryPreAcceptanceEstimatesEndpoint(grantedIntake(), &estimateReaderDouble{err: errors.New("db down")}),
		"/settlement-pre-acceptance-estimates?shipmentRequest=SHR-1")
	if down.Code != http.StatusInternalServerError || problemCode(t, down) != "NO_ANSWER_FORMED" {
		t.Fatalf("读口故障：%d %s", down.Code, down.Body)
	}
	assertNoOutcome(t, down)
}
