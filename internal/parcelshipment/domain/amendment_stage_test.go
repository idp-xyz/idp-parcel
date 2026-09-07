package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// allAbsentAfterAcceptance 是「已接受、尚未收寄」的证据：第一格在，其余五格已知不在。
func allAbsentAfterAcceptance() domain.AmendmentStageEvidence {
	return domain.AmendmentStageEvidence{
		Accepted:                     domain.StageFactPresent,
		Received:                     domain.StageFactAbsent,
		LabelledOrBagged:             domain.StageFactAbsent,
		CustomsDataForming:           domain.StageFactAbsent,
		CustomsSubmitted:             domain.StageFactAbsent,
		CaseClosedOrServiceCompleted: domain.StageFactAbsent,
	}
}

// Covers: 六格次序——靠后的格压过靠前的格。同一份证据里多格同时`在`时，阶段取最靠后那一格
// （CONTEXT 把收寄、制签、申报、提交、关闭排成一句；UC-PS-002 按它分行），六格各自能被判出。
func TestTheFurthestPresentFactNamesTheStage(t *testing.T) {
	cases := map[string]struct {
		mutate func(*domain.AmendmentStageEvidence)
		want   domain.AmendmentStage
	}{
		"只接受": {
			mutate: func(*domain.AmendmentStageEvidence) {},
			want:   domain.StageAcceptedNotYetReceived,
		},
		"已收寄": {
			mutate: func(e *domain.AmendmentStageEvidence) { e.Received = domain.StageFactPresent },
			want:   domain.StageReceivedOrMeasured,
		},
		"已收寄且已制签或已装袋": {
			mutate: func(e *domain.AmendmentStageEvidence) {
				e.Received = domain.StageFactPresent
				e.LabelledOrBagged = domain.StageFactPresent
			},
			want: domain.StageLabelledOrBagged,
		},
		"尚未收寄但关务资料已在形成": {
			mutate: func(e *domain.AmendmentStageEvidence) { e.CustomsDataForming = domain.StageFactPresent },
			want:   domain.StageCustomsDataFormingNotSubmitted,
		},
		"已提交关务压过资料形成中": {
			mutate: func(e *domain.AmendmentStageEvidence) {
				e.CustomsDataForming = domain.StageFactPresent
				e.CustomsSubmitted = domain.StageFactPresent
			},
			want: domain.StageCustomsSubmitted,
		},
		"案件已关闭或服务已完成压过一切": {
			mutate: func(e *domain.AmendmentStageEvidence) {
				e.Received = domain.StageFactPresent
				e.CustomsSubmitted = domain.StageFactPresent
				e.CaseClosedOrServiceCompleted = domain.StageFactPresent
			},
			want: domain.StageCaseClosedOrServiceCompleted,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			evidence := allAbsentAfterAcceptance()
			test.mutate(&evidence)
			if got := domain.JudgeAmendmentStage(evidence); got != test.want {
				t.Fatalf("stage = %q, want %q", got, test.want)
			}
		})
	}
}

// Covers: 判不出阶段不猜、不默认最早阶段——任何一格`不知道`都让阶段判不出，哪怕更早的格已知
// `在`；只有`不知道`落在已判出阶段之前（更早的格）时才不碍事，因为靠后的格已经压过它。
// 零值证据（六格全不知道）同样判不出；委托未接受也判不出。
func TestAnUnknownFactBehindThePresentOneLeavesTheStageUndetermined(t *testing.T) {
	cases := map[string]struct {
		mutate func(*domain.AmendmentStageEvidence)
		want   domain.AmendmentStage
	}{
		"零值证据": {
			mutate: func(e *domain.AmendmentStageEvidence) { *e = domain.AmendmentStageEvidence{} },
			want:   domain.AmendmentStageUndetermined,
		},
		"关务读面未接（三格不知道）": {
			mutate: func(e *domain.AmendmentStageEvidence) {
				e.CustomsDataForming = domain.StageFactUnknown
				e.CustomsSubmitted = domain.StageFactUnknown
				e.CaseClosedOrServiceCompleted = domain.StageFactUnknown
			},
			want: domain.AmendmentStageUndetermined,
		},
		"只有装袋不知道而其余都不在": {
			mutate: func(e *domain.AmendmentStageEvidence) { e.LabelledOrBagged = domain.StageFactUnknown },
			want:   domain.AmendmentStageUndetermined,
		},
		"更早的格不知道不碍靠后的格": {
			mutate: func(e *domain.AmendmentStageEvidence) {
				e.Received = domain.StageFactUnknown
				e.CustomsSubmitted = domain.StageFactPresent
			},
			want: domain.StageCustomsSubmitted,
		},
		"委托未接受": {
			mutate: func(e *domain.AmendmentStageEvidence) { e.Accepted = domain.StageFactAbsent },
			want:   domain.AmendmentStageUndetermined,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			evidence := allAbsentAfterAcceptance()
			test.mutate(&evidence)
			got := domain.JudgeAmendmentStage(evidence)
			if got != test.want {
				t.Fatalf("stage = %q, want %q", got, test.want)
			}
			if got.Determined() != (test.want != domain.AmendmentStageUndetermined) {
				t.Fatalf("Determined() = %v 与阶段 %q 不符", got.Determined(), got)
			}
		})
	}
}

// Covers: 两个上下文各出一半的格按「任一在即在」并成，且一边不在、另一边不知道时整格仍不知道
// ——已知的那半是「不在」不能替另一半作答。
func TestEitherStageFactOnlyKnowsAbsenceWhenBothHalvesDo(t *testing.T) {
	cases := []struct {
		first, second, want domain.StageFact
	}{
		{domain.StageFactPresent, domain.StageFactUnknown, domain.StageFactPresent},
		{domain.StageFactUnknown, domain.StageFactPresent, domain.StageFactPresent},
		{domain.StageFactPresent, domain.StageFactAbsent, domain.StageFactPresent},
		{domain.StageFactAbsent, domain.StageFactAbsent, domain.StageFactAbsent},
		{domain.StageFactAbsent, domain.StageFactUnknown, domain.StageFactUnknown},
		{domain.StageFactUnknown, domain.StageFactAbsent, domain.StageFactUnknown},
		{domain.StageFactUnknown, domain.StageFactUnknown, domain.StageFactUnknown},
	}
	for _, test := range cases {
		if got := domain.EitherStageFact(test.first, test.second); got != test.want {
			t.Fatalf("Either(%v, %v) = %v, want %v", test.first, test.second, got, test.want)
		}
	}
}

// Covers: 委托级阶段取成员中最靠后的一格；任一成员判不出或没有成员，委托级判不出。
func TestTheShipmentStageIsTheFurthestOfItsMembers(t *testing.T) {
	if got := domain.FurthestAmendmentStage(
		domain.StageAcceptedNotYetReceived, domain.StageCustomsSubmitted, domain.StageReceivedOrMeasured,
	); got != domain.StageCustomsSubmitted {
		t.Fatalf("furthest = %q, want CUSTOMS_SUBMITTED", got)
	}
	if got := domain.FurthestAmendmentStage(
		domain.StageCustomsSubmitted, domain.AmendmentStageUndetermined,
	); got != domain.AmendmentStageUndetermined {
		t.Fatalf("一个成员判不出时委托级 = %q, want 判不出", got)
	}
	if got := domain.FurthestAmendmentStage(); got != domain.AmendmentStageUndetermined {
		t.Fatalf("没有成员时 = %q, want 判不出", got)
	}
}

// Covers: 六格各有名字，零值没有——枚举门禁认 String()，这里另钉零值不得冒充一格。
func TestEveryDeterminedStageHasANameAndTheZeroValueDoesNot(t *testing.T) {
	if domain.AmendmentStageUndetermined.String() != "" || domain.AmendmentStageUndetermined.Determined() {
		t.Fatal("零值不该有名字，也不该算已判出")
	}
	for stage := domain.StageAcceptedNotYetReceived; stage <= domain.StageCaseClosedOrServiceCompleted; stage++ {
		if stage.String() == "" || !stage.Determined() {
			t.Fatalf("阶段 %d 没有名字或不算已判出", stage)
		}
	}
}
