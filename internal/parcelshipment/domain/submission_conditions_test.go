package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉提交条件在「必要资料」组的判法（ADR-0376 决定一，票 declared-goods-lines/09）：委托级条件缺了出一条版本级`未定`，
// 箱标号按缺的包裹各出一条，续办方都是客户；全满足或没有条件出一条版本级通过。Decide 停进等待受控补充时把缺的各项记在任务上。

var fbaConditions = func(t *testing.T) domain.SubmissionConditions {
	t.Helper()
	conditions, err := domain.NewSubmissionConditions(
		domain.PlatformShipmentDeclaredCondition, domain.EveryParcelHasPlatformBoxLabelCondition, domain.PackingDeclaredCondition)
	if err != nil {
		t.Fatal(err)
	}
	return conditions
}

// fbaSubmitted 造一份两件的已提交委托：platform 决定报不报平台货件，labelled 是报了箱标号的包裹，packed 决定报不报装箱。
func fbaSubmitted(t *testing.T, platform bool, packed bool, labelled ...string) domain.ShipmentRequest {
	t.Helper()
	spec := submitSpec(t, "parcel-1", "parcel-2")
	if platform {
		shipment, err := domain.DeclarePlatformShipment("SYN-MARKETPLACE-01", mustValue(t, domain.NewExternalIdentifierNumber, "SYN-FBA-1"), "")
		if err != nil {
			t.Fatal(err)
		}
		spec.PlatformShipment = shipment
	}
	for index, parcel := range labelled {
		label, err := domain.DeclarePlatformBoxLabel(mustValue(t, domain.NewDeclaredParcelID, parcel),
			mustValue(t, domain.NewExternalIdentifierNumber, "SYN-FBA-1U00000"+string(rune('1'+index))))
		if err != nil {
			t.Fatal(err)
		}
		spec.PlatformBoxLabels = append(spec.PlatformBoxLabels, label)
	}
	if packed {
		spec.Goods = goodsOf(t, [][2]string{{"T-shirt", "2"}}, [3]string{"parcel-1", "1", "1"}, [3]string{"parcel-2", "1", "1"})
	} else {
		spec.Goods = goodsOf(t, [][2]string{{"T-shirt", "2"}})
	}
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("建单：%v", err)
	}
	return request
}

type gapView struct{ parcel, reason string }

func gapsOf(checks []domain.AcceptanceCheck) []gapView {
	var gaps []gapView
	for _, check := range checks {
		if check.Outcome() != domain.CheckUndetermined {
			continue
		}
		gaps = append(gaps, gapView{parcel: check.DeclaredParcelID().String(), reason: check.Reason().String()})
	}
	return gaps
}

// Covers: ADR-0376 决定一——平台货件、箱标号、装箱全没报：委托级两条出版本级`未定`，箱标号逐件各出一条；续办方都是客户。
// 只缺一箱箱标号时只点那一件，见 TestADecisionWaitingOnSupplementRecordsWhatIsMissing。
func TestUnmetSubmissionConditionsAreUndeterminedAndNameTheMissingParcels(t *testing.T) {
	checks, err := fbaSubmitted(t, false, false).RequiredDocumentChecks(fbaConditions(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []gapView{
		{"", "PLATFORM_SHIPMENT_DECLARED"},
		{"parcel-1", "EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL"},
		{"parcel-2", "EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL"},
		{"", "PACKING_DECLARED"},
	}
	got := gapsOf(checks)
	if len(got) != len(want) {
		t.Fatalf("缺口 = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("缺口 = %v, want %v", got, want)
		}
	}
	for _, check := range checks {
		if check.Group() != domain.RequiredDocumentCheck || check.ResumePath() != domain.ResumeByCustomerSupplement {
			t.Fatalf("判组 = %v、续办 = %v, want 必要资料、受控补充", check.Group(), check.ResumePath())
		}
	}
}

// Covers: 全满足、或快照上没有条件，出一条版本级通过——规则包声明了这一组适用，这一组就得算判过。
func TestSatisfiedOrAbsentSubmissionConditionsPassTheGroupOnce(t *testing.T) {
	for name, item := range map[string]struct {
		request    domain.ShipmentRequest
		conditions domain.SubmissionConditions
	}{
		"全满足":  {fbaSubmitted(t, true, true, "parcel-1", "parcel-2"), fbaConditions(t)},
		"没有条件": {fbaSubmitted(t, false, false), domain.SubmissionConditions{}},
	} {
		checks, err := item.request.RequiredDocumentChecks(item.conditions)
		if err != nil {
			t.Fatal(err)
		}
		if len(checks) != 1 || checks[0].Outcome() != domain.CheckPassed || checks[0].Group() != domain.RequiredDocumentCheck {
			t.Fatalf("%s：checks = %+v, want 一条必要资料通过", name, checks)
		}
	}
}

// Covers: Decide 停进等待受控补充时把缺的各项记在任务上，客户与运营看得到缺哪一条、哪几箱；补齐之后同一组通过即接受。
func TestADecisionWaitingOnSupplementRecordsWhatIsMissing(t *testing.T) {
	without := func(checks []domain.AcceptanceCheck, group domain.AcceptanceCheckGroup) []domain.AcceptanceCheck {
		var kept []domain.AcceptanceCheck
		for _, check := range checks {
			if check.Group() != group {
				kept = append(kept, check)
			}
		}
		return kept
	}
	missing := fbaSubmitted(t, true, true, "parcel-1")
	documents, err := missing.RequiredDocumentChecks(fbaConditions(t))
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := missing.Decide(decisionSpec(t, append(without(allGroupsPassing(t), domain.RequiredDocumentCheck), documents...)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if path, waitingOn := waiting.AcceptanceDecisionTask().WaitingOn(); !waitingOn || path != domain.ResumeByCustomerSupplement {
		t.Fatalf("waitingOn = %v, want 等待受控补充", path)
	}
	gaps := waiting.AcceptanceDecisionTask().SupplementGaps()
	if len(gaps) != 1 || gaps[0].Reason().String() != "EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL" {
		t.Fatalf("缺口 = %+v, want parcel-2 缺箱标号", gaps)
	}
	if parcel, named := gaps[0].Parcel(); !named || parcel.String() != "parcel-2" {
		t.Fatalf("缺口包裹 = %v", parcel)
	}

	complete := fbaSubmitted(t, true, true, "parcel-1", "parcel-2")
	documents, err = complete.RequiredDocumentChecks(fbaConditions(t))
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := complete.Decide(decisionSpec(t, append(without(allGroupsPassing(t), domain.RequiredDocumentCheck), documents...)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if accepted.State() != domain.ShipmentRequestAccepted || len(accepted.AcceptanceDecisionTask().SupplementGaps()) != 0 {
		t.Fatalf("补齐后：state = %v, gaps = %d", accepted.State(), len(accepted.AcceptanceDecisionTask().SupplementGaps()))
	}
}

// Covers: 条件码是封闭集合——参考配置里出现集合外的码立不起来；位集与条件码往返不丢。
func TestSubmissionConditionsAreAClosedSet(t *testing.T) {
	if _, err := domain.NewSubmissionConditions(domain.SubmissionConditionInvalid); !errors.Is(err, domain.ErrInvalidSubmissionCondition) {
		t.Fatalf("err = %v, want ErrInvalidSubmissionCondition", err)
	}
	for _, condition := range fbaConditions(t).Conditions() {
		parsed, ok := domain.ParseSubmissionCondition(condition.String())
		if !ok || parsed != condition {
			t.Fatalf("%v 往返成 %v", condition, parsed)
		}
	}
	if _, ok := domain.ParseSubmissionCondition("CONTENT_CATEGORY_DECLARED"); ok {
		t.Fatal("集合外的码认成了")
	}
}

// Covers: 补齐后重判（ADR-0376 决定一、决定二）——缺一箱箱标号停在等客户补；客户经受控补充交来带全部箱标号的新提交版本，新
// 任务不带旧缺口，重判同一组通过即接受。
func TestASupplementCarryingTheMissingBoxLabelResumesToAcceptance(t *testing.T) {
	without := func(checks []domain.AcceptanceCheck, group domain.AcceptanceCheckGroup) []domain.AcceptanceCheck {
		var kept []domain.AcceptanceCheck
		for _, check := range checks {
			if check.Group() != group {
				kept = append(kept, check)
			}
		}
		return kept
	}
	waiting := fbaSubmitted(t, true, true, "parcel-1")
	documents, err := waiting.RequiredDocumentChecks(fbaConditions(t))
	if err != nil {
		t.Fatal(err)
	}
	if waiting, err = waiting.Decide(decisionSpec(t, append(without(allGroupsPassing(t), domain.RequiredDocumentCheck), documents...))); err != nil {
		t.Fatalf("decide: %v", err)
	}
	if len(waiting.AcceptanceDecisionTask().SupplementGaps()) != 1 {
		t.Fatal("缺一箱箱标号却没记下缺口")
	}

	spec := supersessionSpecFor(t, "parcel-1", "parcel-2")
	spec.PlatformShipment, _ = waiting.PlatformShipment()
	for index, parcel := range []string{"parcel-1", "parcel-2"} {
		label, err := domain.DeclarePlatformBoxLabel(mustValue(t, domain.NewDeclaredParcelID, parcel),
			mustValue(t, domain.NewExternalIdentifierNumber, "SYN-FBA-1U00000"+string(rune('1'+index))))
		if err != nil {
			t.Fatal(err)
		}
		spec.PlatformBoxLabels = append(spec.PlatformBoxLabels, label)
	}
	spec.Goods = goodsOf(t, [][2]string{{"T-shirt", "2"}}, [3]string{"parcel-1", "1", "1"}, [3]string{"parcel-2", "1", "1"})
	supplemented, err := waiting.FormNewSubmissionVersion(spec)
	if err != nil {
		t.Fatalf("受控补充：%v", err)
	}
	if len(supplemented.AcceptanceDecisionTask().SupplementGaps()) != 0 {
		t.Fatal("新任务带着旧缺口")
	}
	if documents, err = supplemented.RequiredDocumentChecks(fbaConditions(t)); err != nil {
		t.Fatal(err)
	}
	accepted, err := supplemented.Decide(decisionSpec(t, append(without(allGroupsPassing(t), domain.RequiredDocumentCheck), documents...)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if accepted.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("补齐后 state = %v, want 已接受", accepted.State())
	}
}
