package shipmenthttp_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// financialControlAsOf 是控制判断的回显时点。
func financialControlAsOf(t *testing.T) domain.JudgmentAsOf {
	t.Helper()
	echoed, err := domain.NewEchoedAsOfPolicy(
		domain.FinancialControlJudgmentKind,
		mustValue(t, domain.NewAsOfSemanticsReference, "ACCEPTANCE"),
		mustValue(t, domain.NewAsOfPolicyVersion, "asof-1"),
	)
	if err != nil {
		t.Fatalf("控制回显政策：%v", err)
	}
	asOf, err := domain.NewJudgmentAsOf(viewSubmittedAt.Add(95*time.Minute), echoed)
	if err != nil {
		t.Fatalf("控制时点：%v", err)
	}
	return asOf
}

// executedControl 按逐项造一份已执行的控制结果；结论由逐项推出（ADR-0125）。
func executedControl(t *testing.T, items ...domain.ControlItemResult) domain.FinancialControlResult {
	t.Helper()
	result, err := domain.NewExecutedFinancialControlResult(domain.ExecutedFinancialControlSpec{
		ResultID:  mustValue(t, domain.NewFinancialControlResultID, "CTRL-1"),
		Items:     items,
		JointPass: domain.AllControlsPass,
		AsOf:      financialControlAsOf(t),
	})
	if err != nil {
		t.Fatalf("财务控制结果：%v", err)
	}
	return result
}

func controlItemOf(t *testing.T, kind domain.ControlItemKind, order uint32, conclusion domain.ControlItemConclusion, basis string) domain.ControlItemResult {
	t.Helper()
	var reference domain.ControlBasisReference
	if basis != "" {
		reference = mustValue(t, domain.NewControlBasisReference, basis)
	}
	item, err := domain.NewControlItemResult(kind, order, conclusion, reference)
	if err != nil {
		t.Fatalf("控制项：%v", err)
	}
	return item
}

// recordedControlsByOutcome 是四种记下的结论各一份：预付冻结成立、信用校验成立、预付冻结因可用余额不足受限、合同说不要求控制。
func recordedControlsByOutcome(t *testing.T) map[string]domain.FinancialControlResult {
	t.Helper()
	inapplicable, err := domain.NewInapplicableFinancialControlResult(
		mustValue(t, domain.NewControlBasisReference, "CONTRACT-NO-CONTROL-1"), financialControlAsOf(t))
	if err != nil {
		t.Fatalf("明确无控制：%v", err)
	}
	return map[string]domain.FinancialControlResult{
		"HELD": executedControl(t,
			controlItemOf(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied, "")),
		"CREDIT_EXPOSED": executedControl(t,
			controlItemOf(t, domain.CreditCheckControlItem, 1, domain.ControlItemSatisfied, "")),
		"RESTRICTED": executedControl(t,
			controlItemOf(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemRestricted, "AVAILABLE_BALANCE_INSUFFICIENT")),
		"NOT_APPLICABLE": inapplicable,
	}
}

// rawAt 沿一串键取出答复里那一格的原文；某一层缺键交回 nil。
func rawAt(t *testing.T, body []byte, path ...string) json.RawMessage {
	t.Helper()
	raw := json.RawMessage(body)
	for _, key := range path {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("decode %s at %q: %v", raw, key, err)
		}
		next, present := fields[key]
		if !present {
			return nil
		}
		raw = next
	}
	return raw
}

// Covers: 票 declared-goods-lines/20、ADR-0379 决定三——委托详情的 financialControl：已冻结、已占信用、受限（可用余额不足）、明确无控制
// 四种各照登记透出，与复核详情 recordedJudgments.financialControl 逐字节相同（同一个编码函数）；复核详情的 request 不再带第二份；
// 不带任何金额。没判过即整键缺席。
func TestTheShipmentDetailShowsTheRecordedFinancialControlConclusion(t *testing.T) {
	for outcome, result := range recordedControlsByOutcome(t) {
		t.Run(outcome, func(t *testing.T) {
			detail := viewDetailRecord(t)
			detail.FinancialControl = result
			views := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{detail: detail, found: true},
				http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1")
			if views.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", views.Code, views.Body)
			}
			shown := rawAt(t, views.Body.Bytes(), "request", "financialControl")
			if shown == nil {
				t.Fatalf("委托详情缺 financialControl：%s", views.Body)
			}
			var control struct {
				Outcome            string `json:"outcome"`
				Basis              string `json:"basis"`
				JointPassCondition string `json:"jointPassCondition"`
				Items              []struct {
					Kind       string `json:"kind"`
					Conclusion string `json:"conclusion"`
					Basis      string `json:"basis"`
				} `json:"items"`
			}
			if err := json.Unmarshal(shown, &control); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if control.Outcome != outcome {
				t.Fatalf("结论 = %q, 想要 %q", control.Outcome, outcome)
			}
			switch outcome {
			case "RESTRICTED":
				if control.Basis != "AVAILABLE_BALANCE_INSUFFICIENT" || len(control.Items) != 1 ||
					control.Items[0].Conclusion != "RESTRICTED" || control.Items[0].Basis != "AVAILABLE_BALANCE_INSUFFICIENT" {
					t.Fatalf("受限 = %s", shown)
				}
			case "NOT_APPLICABLE":
				// 合同说不要求控制：没有条件、没有项，依据说明为什么不该控制。
				if control.Basis != "CONTRACT-NO-CONTROL-1" || control.JointPassCondition != "" || control.Items == nil || len(control.Items) != 0 {
					t.Fatalf("明确无控制 = %s", shown)
				}
			default:
				if control.JointPassCondition != "ALL_CONTROLS_PASS" || len(control.Items) != 1 || control.Items[0].Conclusion != "SATISFIED" {
					t.Fatalf("%s = %s", outcome, shown)
				}
			}
			for _, money := range []string{"amount", "Amount", "Minor", "currency"} {
				if strings.Contains(string(shown), money) {
					t.Fatalf("结论里带了金额（%s）：%s", money, shown)
				}
			}

			// 复核详情的 request 复用委托详情，读模型上带着结论也不交：复核只看 recordedJudgments 那一份。
			reviewDetail := reviewCaseDetail(t)
			reviewDetail.FinancialControl = result
			review := getReviewQueue(t, &viewsIntakeDouble{scope: viewsScope(t)},
				&reviewQueueReaderDouble{detail: reviewDetail, found: true},
				&judgmentsReaderDouble{recorded: ports.RecordedJudgments{FinancialControl: result}},
				http.MethodGet, "/acceptance-review-queue?shipmentRequestId=REQ-1")
			if judged := rawAt(t, review.Body.Bytes(), "recordedJudgments", "financialControl"); string(judged) != string(shown) {
				t.Fatalf("两处形状不同：\n委托详情 %s\n复核详情 %s", shown, judged)
			}
			if duplicate := rawAt(t, review.Body.Bytes(), "request", "financialControl"); duplicate != nil {
				t.Fatalf("复核详情的 request 又带了一份：%s", duplicate)
			}
		})
	}
}

// Covers: 没判过的委托详情不带 financialControl；列表行任何时候都不带。
func TestTheFinancialControlIsAbsentWhenNeverJudgedAndOnListRows(t *testing.T) {
	never := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)}, &viewsReaderDouble{detail: viewDetailRecord(t), found: true},
		http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1")
	if never.Code != http.StatusOK {
		t.Fatalf("status = %d", never.Code)
	}
	if raw := rawAt(t, never.Body.Bytes(), "request", "financialControl"); raw != nil {
		t.Fatalf("没判过却长出了 financialControl：%s", raw)
	}

	list := getViews(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&viewsReaderDouble{rows: []ports.ShipmentRequestSummaryRecord{viewSummaryRecord(t, "REQ-1")}},
		http.MethodGet, "/shipment-request-views")
	var body struct {
		Requests []map[string]json.RawMessage `json:"requests"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &body); err != nil || len(body.Requests) != 1 {
		t.Fatalf("list = %s err = %v", list.Body, err)
	}
	if _, present := body.Requests[0]["financialControl"]; present {
		t.Fatalf("列表行带了 financialControl：%s", list.Body)
	}
}
