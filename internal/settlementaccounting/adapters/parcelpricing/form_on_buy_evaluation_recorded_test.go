package parcelpricing_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件证 sainbox.BuyEvaluationRecordedConsumer 的处理方（票 sa-cc/01，裁决 (c)「信封到未决」）：按信封
// 引用向 BuyEvaluationView 取评价 → 不是 BUY·SUPPLIER_COST 的信封不处理不报错 → 读不回是可见性滞后的未决
// → 命令还缺发生项 / 费用项目 / 供应商协议的来源引用，答未决并指名等评价请求记录（票 08 / 回指归 11）。处理方不判评价结果、不形成、
// 不读 08 的登记册——那是后继票的事。

type buyEvaluationViewDouble struct {
	adoption saports.BuyEvaluationAdoption
	found    bool
	err      error
	calls    []string
}

func (double *buyEvaluationViewDouble) LoadBuyEvaluation(
	_ context.Context, tenant sadomain.TenantID, evaluation sadomain.BuyEvaluationReference,
) (saports.BuyEvaluationAdoption, bool, error) {
	double.calls = append(double.calls, tenant.String()+"/"+evaluation.String())
	return double.adoption, double.found, double.err
}

func recorded() sainbox.RecordedBuyEvaluation {
	return sainbox.RecordedBuyEvaluation{TenantID: "tenant-a", EvaluationID: "SYN-EVAL-01"}
}

func newFormOnRecorded(t *testing.T, view *buyEvaluationViewDouble) *adapter.FormOnBuyEvaluationRecordedAdapter {
	t.Helper()
	return must[*adapter.FormOnBuyEvaluationRecordedAdapter](t)(adapter.NewFormOnBuyEvaluationRecordedAdapter(view))
}

// Covers: 做法 3 / 判据 2——BUY 信封按引用取回评价后，命令里的发生项 / 费用项目 / 供应商协议引用没有来源，
// 答未决并指名等评价请求记录；不造引用、不落任何一行。saports.BuyEvaluationOutcome 的每一格一视同仁：结果分格
// 是形成编排的事，命令凑不齐时到不了它。
func TestABuyEvaluationStopsUndecidedWaitingForTheEvaluationRequestRecord(t *testing.T) {
	for name, outcome := range map[string]saports.BuyEvaluationOutcome{
		"已完成":  saports.BuyEvaluationCompleted,
		"待判断":  saports.BuyEvaluationPending,
		"不可计价": saports.BuyEvaluationUnratable,
		"冲突":   saports.BuyEvaluationConflict,
		"未形成":  saports.BuyEvaluationNotFormed,
	} {
		t.Run(name, func(t *testing.T) {
			view := &buyEvaluationViewDouble{found: true, adoption: saports.BuyEvaluationAdoption{Outcome: outcome}}

			err := newFormOnRecorded(t, view).HandleRecordedBuyEvaluation(t.Context(), recorded())

			if !errors.Is(err, adapter.ErrSourceReferencesUnrecorded) {
				t.Fatalf("err = %v, want ErrSourceReferencesUnrecorded", err)
			}
			if len(view.calls) != 1 || view.calls[0] != "tenant-a/SYN-EVAL-01" {
				t.Fatalf("回查 = %v, want 按信封的租户与评价引用查一次", view.calls)
			}
		})
	}
}

// Covers: 做法 2——方向 / 目的不是 BUY·SUPPLIER_COST 的评价不是本消费者的信封：不处理、不报错，消费门据此
// 入账不重投。提供方对两个方向发同一种信封，SELL 那一半在这里安静地走掉。
func TestASellEvaluationIsNotThisConsumersEnvelopeAndIsSilentlyDone(t *testing.T) {
	view := &buyEvaluationViewDouble{err: fmt.Errorf("%w: SELL / CUSTOMER_CHARGE", adapter.ErrNotABuyEvaluation)}

	if err := newFormOnRecorded(t, view).HandleRecordedBuyEvaluation(t.Context(), recorded()); err != nil {
		t.Fatalf("err = %v, want nil——不是本消费者的信封不报错", err)
	}
}

// Covers: 信封与评价行在提供方同一事务落库，读不回只剩可见性滞后一种成因——未决重投，不当毒丸、不当提交矛盾。
// 读口本身答不出（库不可用）同格：等的都是提供方那一侧。
func TestAnInvisibleEvaluationIsUndecidedAndRetried(t *testing.T) {
	for name, view := range map[string]*buyEvaluationViewDouble{
		"评价还读不回": {found: false},
		"读口不可用":  {err: errors.New("load buy evaluation: connection refused")},
	} {
		t.Run(name, func(t *testing.T) {
			err := newFormOnRecorded(t, view).HandleRecordedBuyEvaluation(t.Context(), recorded())
			if !errors.Is(err, adapter.ErrEvaluationNotVisible) {
				t.Fatalf("err = %v, want ErrEvaluationNotVisible", err)
			}
		})
	}
}

// Covers: 提供方交出词汇表之外的内容、或价卡没声明取整策略——两者重投都不会变好，不得以未决之名反复重投；
// 读口的哨兵原样上抛，装配把它们留在未决名单之外。
func TestProviderShapeAndPolicyRefusalsAreNotUndecided(t *testing.T) {
	for name, sentinel := range map[string]error{
		"词汇表之外":   adapter.ErrUntranslatableAnswer,
		"未声明取整策略": adapter.ErrAmountPrecisionUndeclared,
	} {
		t.Run(name, func(t *testing.T) {
			view := &buyEvaluationViewDouble{err: fmt.Errorf("%w: evaluation SYN-EVAL-01", sentinel)}

			err := newFormOnRecorded(t, view).HandleRecordedBuyEvaluation(t.Context(), recorded())

			if !errors.Is(err, sentinel) {
				t.Fatalf("err = %v, want %v 原样可辨", err, sentinel)
			}
			if errors.Is(err, adapter.ErrEvaluationNotVisible) || errors.Is(err, adapter.ErrSourceReferencesUnrecorded) {
				t.Fatalf("err = %v, 不得被译成未决", err)
			}
		})
	}
}

// Covers: 消费者译码已拒过两维缺席，这里只剩两侧词汇分歧（空白串）：引用坏了不是等谁，响亮报错，不回查。
func TestABlankReferenceIsUntranslatableAndNotLookedUp(t *testing.T) {
	for name, mutate := range map[string]func(*sainbox.RecordedBuyEvaluation){
		"空白租户": func(r *sainbox.RecordedBuyEvaluation) { r.TenantID = "   " },
		"空白评价": func(r *sainbox.RecordedBuyEvaluation) { r.EvaluationID = " " },
	} {
		t.Run(name, func(t *testing.T) {
			view := &buyEvaluationViewDouble{found: true}
			blank := recorded()
			mutate(&blank)

			err := newFormOnRecorded(t, view).HandleRecordedBuyEvaluation(t.Context(), blank)

			if !errors.Is(err, adapter.ErrUntranslatableReference) {
				t.Fatalf("err = %v, want ErrUntranslatableReference", err)
			}
			if len(view.calls) != 0 {
				t.Fatalf("引用坏了还去回查了 %d 次", len(view.calls))
			}
		})
	}
}

func TestTheFormOnRecordedAdapterRefusesNilDependencies(t *testing.T) {
	if _, err := adapter.NewFormOnBuyEvaluationRecordedAdapter(nil); err == nil {
		t.Fatal("nil 读口被收下了")
	}
}
