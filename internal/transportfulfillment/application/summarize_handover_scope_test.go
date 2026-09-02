package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证交接范围汇总读面（票 tf-unwired-seven/03）：汇总只由 SummarizeHandovers 派生，
// 读面不自己数；空范围「不成立汇总」与「三个零的汇总」是两种答案；读失败是可续办的未决，
// 不冒充空范围。夹具全部为合成登记（S 级）。

var summaryJudgedAt = time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC)

type handoverScopeViewDouble struct {
	records []ports.TransportHandoverRecord
	listErr error
	calls   int
	// 记下被问到的租户与范围：CONTEXT 要求汇总只含「这一次交接」的成员，
	// 而读面若不把租户传下去，隔离就只剩适配器一层。
	askedTenant domain.TenantID
	askedScope  domain.HandoverScopeReference
}

func (double *handoverScopeViewDouble) ListByScope(
	_ context.Context,
	tenant domain.TenantID,
	scope domain.HandoverScopeReference,
) ([]ports.TransportHandoverRecord, error) {
	double.calls++
	double.askedTenant = tenant
	double.askedScope = scope
	if double.listErr != nil {
		return nil, double.listErr
	}
	return double.records, nil
}

func summaryValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func summaryRecord(t *testing.T, object string, verdict domain.HandoverVerdict) ports.TransportHandoverRecord {
	t.Helper()
	spec := domain.TransportHandoverSpec{
		TenantID:   summaryValue(t, domain.NewTenantID, "tenant-1"),
		Object:     summaryValue(t, domain.NewCarriedObjectReference, object),
		Scope:      summaryValue(t, domain.NewHandoverScopeReference, "scope-1"),
		ReleasedBy: summaryValue(t, domain.NewHandoverPartyReference, "node-1"),
		ReceivedBy: summaryValue(t, domain.NewHandoverPartyReference, "carrier-1"),
		Verdict:    verdict,
		Version:    summaryValue(t, domain.NewHandoverResultVersion, "v1"),
		JudgedAt:   summaryJudgedAt,
	}
	// 三裁决的完备性各不相同：已交接要双方证据加适用规则且不带依据，另两格必须带依据。
	if verdict == domain.ObjectHandedOver {
		spec.ReleasingEvidence = summaryValue(t, domain.NewHandoverEvidenceReference, "seal-out")
		spec.ReceivingEvidence = summaryValue(t, domain.NewHandoverEvidenceReference, "seal-in")
		spec.Rule = summaryValue(t, domain.NewHandoverRuleReference, "rule/v1")
	} else {
		spec.Basis = summaryValue(t, domain.NewHandoverBasisReference, "basis-"+object)
	}
	handover, err := domain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("构造交接夹具 %s：%v", object, err)
	}
	return ports.TransportHandoverRecord{
		Key: ports.TransportHandoverKey{
			TenantID: handover.TenantID(),
			Object:   handover.Object(),
			Scope:    handover.Scope(),
			Version:  handover.Version(),
		},
		Handover:   handover,
		RecordedAt: summaryJudgedAt,
	}
}

func summarizeQuery(t *testing.T) application.SummarizeHandoverScopeQuery {
	t.Helper()
	return application.SummarizeHandoverScopeQuery{
		TenantID: summaryValue(t, domain.NewTenantID, "tenant-1"),
		Scope:    "scope-1",
	}
}

// Covers: CONTEXT「共享实际履约段中的每个载运对象分别成立、结束和更正，不能由整段结果
// 覆盖成员差异」在汇总侧的对应面——三裁决各自计数照实透出，不压成一个「整批成功」。
func TestScopeSummaryTranscribesEachVerdictSeparately(t *testing.T) {
	view := &handoverScopeViewDouble{records: []ports.TransportHandoverRecord{
		summaryRecord(t, "parcel-1", domain.ObjectHandedOver),
		summaryRecord(t, "parcel-2", domain.ObjectHandedOver),
		summaryRecord(t, "parcel-3", domain.HandoverRefused),
		summaryRecord(t, "parcel-4", domain.HandoverPendingConfirmation),
	}}

	result, err := application.NewSummarizeHandoverScopeHandler(view).
		Summarize(t.Context(), summarizeQuery(t))
	if err != nil {
		t.Fatalf("汇总：%v", err)
	}
	if result.Outcome() != application.HandoverScopeSummarized {
		t.Fatalf("outcome = %q, want SCOPE_SUMMARIZED", result.Outcome())
	}
	summary, present := result.Summary()
	if !present {
		t.Fatal("成立的汇总没有交出汇总本体")
	}
	if summary.HandedOver() != 2 || summary.Refused() != 1 || summary.Unconfirmed() != 1 {
		t.Fatalf("三格计数 = %d/%d/%d, want 2/1/1",
			summary.HandedOver(), summary.Refused(), summary.Unconfirmed())
	}
	if summary.Total() != 4 {
		t.Fatalf("total = %d, want 4", summary.Total())
	}
	if summary.AllHandedOver() {
		t.Fatal("有拒收与待确认在场，AllHandedOver 仍为真——部分接收被读成了整批成功")
	}
	if summary.Scope().String() != "scope-1" {
		t.Fatalf("scope = %q", summary.Scope())
	}
}

// Covers: 领域「空集不成立汇总」——读面必须把它当成一种答案交回，而不是一份三个零的汇总。
// 零与不成立是两件事：前者说「这个范围有交接，结果都不是这一格」，后者说「这个范围还没有
// 交接」，下游要做的事不同。
func TestEmptyScopeIsNotSummarizableRatherThanAllZeroes(t *testing.T) {
	view := &handoverScopeViewDouble{}

	result, err := application.NewSummarizeHandoverScopeHandler(view).
		Summarize(t.Context(), summarizeQuery(t))
	if err != nil {
		t.Fatalf("汇总：%v", err)
	}
	if result.Outcome() != application.HandoverScopeNotSummarizable {
		t.Fatalf("outcome = %q, want SCOPE_NOT_SUMMARIZABLE", result.Outcome())
	}
	if _, present := result.Summary(); present {
		t.Fatal("空范围交出了一份汇总——三个零会被下游读成「这个范围全部待确认为零」")
	}
}

// Covers: 本仓编排纪律「依赖失败形成本上下文自己的未决结果而不上抛技术错误，且未决结果
// 携带封闭原因与续办引用」。读不回来与「范围里没有交接」在库面都可能表现为零行，这里
// 必须分得开。
func TestUnreadableRegistryIsUndecidedNotAnEmptyScope(t *testing.T) {
	view := &handoverScopeViewDouble{listErr: errors.New("registry down")}

	result, err := application.NewSummarizeHandoverScopeHandler(view).
		Summarize(t.Context(), summarizeQuery(t))
	if err != nil {
		t.Fatalf("读失败不该上抛技术错误：%v", err)
	}
	if result.Outcome() != application.HandoverScopeUndecided {
		t.Fatalf("outcome = %q, want SCOPE_UNDECIDED", result.Outcome())
	}
	if result.Reason() != application.HandoverScopeRegistryUnavailable {
		t.Fatalf("reason = %q, want HANDOVER_REGISTRY_UNAVAILABLE", result.Reason())
	}
	if result.Continuation() == "" {
		t.Fatal("未决没有续办引用——调用方无从重试同一次查询")
	}
	if _, present := result.Summary(); present {
		t.Fatal("读失败交出了汇总")
	}
}

// Covers: 本仓编排纪律「租户显式入参而不从 context 里补」，以及「最小身份先于任何权威读取」
// ——身份立不住时不得先去读库。
func TestBadIdentityIsRefusedBeforeAnyRead(t *testing.T) {
	for name, query := range map[string]application.SummarizeHandoverScopeQuery{
		"空租户": {Scope: "scope-1"},
		"空范围": {TenantID: summaryValue(t, domain.NewTenantID, "tenant-1")},
	} {
		t.Run(name, func(t *testing.T) {
			view := &handoverScopeViewDouble{}
			result, err := application.NewSummarizeHandoverScopeHandler(view).
				Summarize(t.Context(), query)
			if err != nil {
				t.Fatalf("身份不受理不该上抛：%v", err)
			}
			if result.Outcome() != application.HandoverScopeInputNotAccepted {
				t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
			}
			if view.calls != 0 {
				t.Fatalf("身份立不住却读了 %d 次库", view.calls)
			}
		})
	}
}

// Covers: 租户与范围都必须传到读口上。读面若只传范围，跨租户隔离就只剩适配器一层，
// 而本仓把租户当身份的最高隔离边界。
func TestQueryPassesTenantAndScopeDownToTheView(t *testing.T) {
	view := &handoverScopeViewDouble{records: []ports.TransportHandoverRecord{
		summaryRecord(t, "parcel-1", domain.ObjectHandedOver),
	}}

	if _, err := application.NewSummarizeHandoverScopeHandler(view).
		Summarize(t.Context(), summarizeQuery(t)); err != nil {
		t.Fatalf("汇总：%v", err)
	}
	if view.askedTenant.String() != "tenant-1" {
		t.Fatalf("读口收到的租户 = %q", view.askedTenant)
	}
	if view.askedScope.String() != "scope-1" {
		t.Fatalf("读口收到的范围 = %q", view.askedScope)
	}
}
