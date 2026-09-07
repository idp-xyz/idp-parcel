package nodeoperations_test

import (
	"context"
	"errors"
	"testing"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psnode "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// containmentLookupDouble 是节点作业容纳读面的替身（隔离合成 `S`）：按键交回登记的三值，记下
// 被问的键。未登记的键交回`不在`——那是读面自己对「从未关联」的语义，不是替身的发明。
type containmentLookupDouble struct {
	answers     map[string]noports.ParcelContainment
	err         error
	askedTenant string
	askedParcel string
}

func (double *containmentLookupDouble) LoadParcelContainment(
	_ context.Context,
	tenant nodomain.TenantID,
	parcel nodomain.ParcelAssociationReference,
) (noports.ParcelContainment, error) {
	double.askedTenant = tenant.String()
	double.askedParcel = parcel.String()
	if double.err != nil {
		return noports.ParcelContainmentInvalid, double.err
	}
	if answer, registered := double.answers[parcel.String()]; registered {
		return answer, nil
	}
	return noports.ParcelNotContained, nil
}

func newBaggingView(t *testing.T, double *containmentLookupDouble) psnode.ConsolidationStageView {
	t.Helper()
	view, err := psnode.NewConsolidationStageView(double)
	if err != nil {
		t.Fatalf("构造装袋读口：%v", err)
	}
	return view
}

func loadBagging(t *testing.T, view psnode.ConsolidationStageView, tenant, parcel string) psdomain.StageFact {
	t.Helper()
	bagged, err := view.LoadBaggingFact(t.Context(),
		value(t, psdomain.NewTenantID, tenant),
		value(t, psdomain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("读装袋事实：%v", err)
	}
	return bagged
}

// Covers: 全函数翻译——节点作业三值各有落点：在→在、不在→不在、不可归属→不知道；消费方的
// 租户与包裹字面原样成为节点作业的关联引用键。
func TestParcelContainmentIsTranslatedTotallyIntoTheBaggingFact(t *testing.T) {
	double := &containmentLookupDouble{answers: map[string]noports.ParcelContainment{
		"SYN-PARCEL-01": noports.ParcelContained,
		"SYN-PARCEL-02": noports.ParcelNotContained,
		"SYN-PARCEL-03": noports.ParcelContainmentUnattributable,
	}}
	view := newBaggingView(t, double)

	if got := loadBagging(t, view, "SYN-TENANT-01", "SYN-PARCEL-01"); got != psdomain.StageFactPresent {
		t.Fatalf("在袋里译成 %v, want 在", got)
	}
	if double.askedTenant != "SYN-TENANT-01" || double.askedParcel != "SYN-PARCEL-01" {
		t.Fatalf("问节点作业用的键走样：tenant=%q parcel=%q", double.askedTenant, double.askedParcel)
	}
	if got := loadBagging(t, view, "SYN-TENANT-01", "SYN-PARCEL-02"); got != psdomain.StageFactAbsent {
		t.Fatalf("不在袋里译成 %v, want 不在", got)
	}
	// 不可归属是节点作业自己的「说不出」：本适配器不替它判成任何一边，搬进 PS 的`不知道`。
	if got := loadBagging(t, view, "SYN-TENANT-01", "SYN-PARCEL-03"); got != psdomain.StageFactUnknown {
		t.Fatalf("不可归属译成 %v, want 不知道", got)
	}
}

// Covers: 节点作业从未关联该包裹——读面答`不在`，适配器照译`不在`，与本上下文已知`不在`的面单
// 结果并成「已制签或已装袋」整格`不在`：一个尚未到站的包裹因此判得出阶段。这一格是它与
// Unconnected 答复的分界。
func TestAParcelNeverAssociatedByAnyNodeIsAbsentAndLetsTheStageResolve(t *testing.T) {
	view := newBaggingView(t, &containmentLookupDouble{})

	bagged := loadBagging(t, view, "SYN-TENANT-01", "SYN-PARCEL-09")
	if bagged != psdomain.StageFactAbsent {
		t.Fatalf("bagged = %v, want 不在", bagged)
	}
	if got := psdomain.EitherStageFact(psdomain.StageFactAbsent, bagged); got != psdomain.StageFactAbsent {
		t.Fatalf("面单不在 + 装袋不在 并成 %v, want 不在", got)
	}
}

// Covers: 读面调不通作为错误上抛，不折成任何一格。
func TestAContainmentLookupFailureIsAnErrorNotAFact(t *testing.T) {
	failure := errors.New("node operations unreachable")
	view := newBaggingView(t, &containmentLookupDouble{err: failure})

	_, err := view.LoadBaggingFact(t.Context(),
		value(t, psdomain.NewTenantID, "SYN-TENANT-01"),
		value(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01"))
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want 包住读面的原始错误", err)
	}
}

// Covers: 集外取值不静默落进任何一格——提供方长了新取值而本处没跟上时上抛，让它在这里被看见。
func TestAnOutOfSetContainmentValueIsUntranslatable(t *testing.T) {
	view := newBaggingView(t, &containmentLookupDouble{answers: map[string]noports.ParcelContainment{
		"SYN-PARCEL-01": noports.ParcelContainment(99),
	}})

	_, err := view.LoadBaggingFact(t.Context(),
		value(t, psdomain.NewTenantID, "SYN-TENANT-01"),
		value(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01"))
	if !errors.Is(err, psnode.ErrUntranslatableAnswer) {
		t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
	}
}

func TestTheBaggingViewRefusesANilLookup(t *testing.T) {
	if _, err := psnode.NewConsolidationStageView(nil); err == nil {
		t.Fatal("nil 读面装出了一只适配器——它会在第一次调用时 panic，而不是在装配期说清")
	}
}
