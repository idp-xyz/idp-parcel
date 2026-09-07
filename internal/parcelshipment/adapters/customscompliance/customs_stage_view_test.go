package customscompliance_test

import (
	"context"
	"errors"
	"testing"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	pscustoms "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/customscompliance"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// factsLookupDouble 是关务读面的替身（隔离合成 `S`）：按键交回登记的事实，记下被问的键，供
// 断言适配器把消费方话语译成了提供方的哪一把键。未登记的键交回空事实——那是读面自己的语义
// （空册三格皆否），不是替身的发明。
type factsLookupDouble struct {
	facts       map[string]ccports.ParcelDeclarationFacts
	err         error
	askedTenant string
	askedParcel string
}

func (double *factsLookupDouble) LoadParcelDeclarationFacts(
	_ context.Context,
	tenant ccdomain.TenantID,
	parcel ccdomain.DeclaredParcelReference,
) (ccports.ParcelDeclarationFacts, error) {
	double.askedTenant = tenant.String()
	double.askedParcel = parcel.String()
	if double.err != nil {
		return ccports.ParcelDeclarationFacts{}, double.err
	}
	return double.facts[parcel.String()], nil
}

func newStageView(t *testing.T, double *factsLookupDouble) pscustoms.CustomsStageView {
	t.Helper()
	view, err := pscustoms.NewCustomsStageView(double)
	if err != nil {
		t.Fatalf("构造关务阶段读口：%v", err)
	}
	return view
}

func loadStageFacts(t *testing.T, view pscustoms.CustomsStageView, tenant, parcel string) psports.CustomsStageFacts {
	t.Helper()
	facts, err := view.LoadCustomsStageFacts(t.Context(),
		mustValue(t, psdomain.NewTenantID, tenant),
		mustValue(t, psdomain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("读关务阶段事实：%v", err)
	}
	return facts
}

func mustValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

// Covers: 逐格布尔到三态——关务说「有」译`在`、「没有」译`不在`，三格各译各的、不互相影响；
// 消费方的租户与包裹字面原样成为提供方的键。
func TestCustomsFactsAreTranslatedCellByCellIntoStageFacts(t *testing.T) {
	double := &factsLookupDouble{facts: map[string]ccports.ParcelDeclarationFacts{
		"SYN-PARCEL-01": {MemberOfUnsubmittedUnit: true},
		"SYN-PARCEL-02": {InFixedSubmissionVersion: true, InClosedCase: true},
		"SYN-PARCEL-03": {MemberOfUnsubmittedUnit: true, InFixedSubmissionVersion: true},
	}}
	view := newStageView(t, double)

	got := loadStageFacts(t, view, "SYN-TENANT-01", "SYN-PARCEL-01")
	if double.askedTenant != "SYN-TENANT-01" || double.askedParcel != "SYN-PARCEL-01" {
		t.Fatalf("问关务用的键走样：tenant=%q parcel=%q", double.askedTenant, double.askedParcel)
	}
	want := psports.CustomsStageFacts{
		DataForming: psdomain.StageFactPresent,
		Submitted:   psdomain.StageFactAbsent,
		CaseClosed:  psdomain.StageFactAbsent,
	}
	if got != want {
		t.Fatalf("PARCEL-01 facts = %+v, want %+v", got, want)
	}

	got = loadStageFacts(t, view, "SYN-TENANT-01", "SYN-PARCEL-02")
	want = psports.CustomsStageFacts{
		DataForming: psdomain.StageFactAbsent,
		Submitted:   psdomain.StageFactPresent,
		CaseClosed:  psdomain.StageFactPresent,
	}
	if got != want {
		t.Fatalf("PARCEL-02 facts = %+v, want %+v", got, want)
	}

	// 两格同时在：适配器不替 PS 选哪一格，两格都译`在`，交给 JudgeAmendmentStage 按次序压。
	got = loadStageFacts(t, view, "SYN-TENANT-01", "SYN-PARCEL-03")
	want = psports.CustomsStageFacts{
		DataForming: psdomain.StageFactPresent,
		Submitted:   psdomain.StageFactPresent,
		CaseClosed:  psdomain.StageFactAbsent,
	}
	if got != want {
		t.Fatalf("PARCEL-03 facts = %+v, want %+v", got, want)
	}
}

// Covers: 读面存在而关务对该包裹没有任何单元、版本或案件——三格都是`不在`，不是`不知道`。
// 这一格正是它与 Unconnected 答复的分界：接真之后，一个从未进关的包裹让阶段判得出来。
func TestAParcelUnknownToCustomsIsAbsentInEveryCellNotUnknown(t *testing.T) {
	view := newStageView(t, &factsLookupDouble{})

	got := loadStageFacts(t, view, "SYN-TENANT-01", "SYN-PARCEL-09")
	want := psports.CustomsStageFacts{
		DataForming: psdomain.StageFactAbsent,
		Submitted:   psdomain.StageFactAbsent,
		CaseClosed:  psdomain.StageFactAbsent,
	}
	if got != want {
		t.Fatalf("facts = %+v, want 三格都是不在", got)
	}
	stage := psdomain.JudgeAmendmentStage(psdomain.AmendmentStageEvidence{
		Accepted:                     psdomain.StageFactPresent,
		Received:                     psdomain.StageFactAbsent,
		LabelledOrBagged:             psdomain.StageFactAbsent,
		CustomsDataForming:           got.DataForming,
		CustomsSubmitted:             got.Submitted,
		CaseClosedOrServiceCompleted: got.CaseClosed,
	})
	if stage != psdomain.StageAcceptedNotYetReceived {
		t.Fatalf("stage = %v, want 已接受尚未收寄——关务读面答得出`不在`，阶段就判得出", stage)
	}
}

// Covers: 读面调不通作为错误上抛，不折成任何一格——编排据以落在事实读不回（等依赖恢复），
// 与`不知道`（等读面接上）分开。
func TestALookupFailureIsAnErrorNotAFact(t *testing.T) {
	failure := errors.New("customs unreachable")
	view := newStageView(t, &factsLookupDouble{err: failure})

	_, err := view.LoadCustomsStageFacts(t.Context(),
		mustValue(t, psdomain.NewTenantID, "SYN-TENANT-01"),
		mustValue(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01"))
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want 包住读面的原始错误", err)
	}
}

func TestTheStageViewRefusesANilLookup(t *testing.T) {
	if _, err := pscustoms.NewCustomsStageView(nil); err == nil {
		t.Fatal("nil 读面装出了一只适配器——它会在第一次调用时 panic，而不是在装配期说清")
	}
}
