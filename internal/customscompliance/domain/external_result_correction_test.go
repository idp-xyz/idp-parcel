package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// correcting 把一份事实改成指名 target 的明确更正：其余各件照旧，只多一格「更正哪一份」。
func correcting(t *testing.T, fact domain.LayerFact, target string) domain.LayerFact {
	t.Helper()
	return reinterpreted(t, fact, func(spec *domain.ExternalResultSpec) { spec.Corrects = target })
}

// inScope 把一份事实挪到另一个申报单元上；放行事实随结果范围一起挪。
func inScope(t *testing.T, fact domain.LayerFact, unit string) domain.LayerFact {
	t.Helper()
	moved := reinterpreted(t, fact, func(spec *domain.ExternalResultSpec) {
		spec.Scope = mustValue(t, domain.NewDecisionScopeReference, unit)
	})
	if fact.Release != nil {
		condition, _ := fact.Release.Condition()
		release, err := domain.ReceiveReleaseOutcome(fact.Release.Kind(), fact.Release.Authority(),
			moved.Result.Scope(), condition, fact.Release.ReceivedAt())
		if err != nil {
			t.Fatalf("release outcome: %v", err)
		}
		moved.Release = &release
	}
	return moved
}

func reinterpreted(t *testing.T, fact domain.LayerFact, change func(*domain.ExternalResultSpec)) domain.LayerFact {
	t.Helper()
	result := fact.Result
	corrects, _ := result.Corrects()
	spec := domain.ExternalResultSpec{
		Layer:        result.Layer(),
		SourceID:     result.SourceID(),
		Role:         result.Role(),
		RawSemantics: result.RawSemantics(),
		Rule:         result.Rule(),
		Version:      result.Version(),
		Attempt:      result.Attempt(),
		Scope:        result.Scope(),
		OccurredAt:   result.OccurredAt(),
		ReceivedAt:   result.ReceivedAt(),
		Corrects:     corrects,
	}
	change(&spec)
	rebuilt, err := domain.InterpretExternalResult(spec)
	if err != nil {
		t.Fatalf("interpret %s: %v", spec.SourceID, err)
	}
	return domain.LayerFact{Result: rebuilt, Release: fact.Release, CorrectionInForce: fact.CorrectionInForce}
}

// inForce 是一份已经落册、更正关系在接收时生效的更正。
func inForce(fact domain.LayerFact) domain.LayerFact {
	fact.CorrectionInForce = true
	return fact
}

// Covers: AT-CC-156 的领域半边（范围只到申报单元一级）——来源明确指名更正哪一份时形成更正关系，不进
// 与所更正那份的矛盾判断：核定税费换金额、处置决定换种类、放行收窄这三种今天一律判同层冲突的明确更正，
// 都不判冲突，而且更正关系在同一范围上生效（所更正那份退出当前判断）。不带更正指向的同一份内容照判冲突。
func TestAnExplicitSameLayerCorrectionFormsACorrectionRelationInsteadOfAConflict(t *testing.T) {
	for name, test := range map[string]struct {
		original, correction domain.LayerFact
	}{
		"监管核定税费换金额": {
			layerFact(t, "duty-1", domain.AssessedDutyLayer, "DUTY/USD-120.00", domain.ReleaseKindInvalid, earlierAt),
			layerFact(t, "duty-2", domain.AssessedDutyLayer, "DUTY/USD-95.50", domain.ReleaseKindInvalid, laterAt),
		},
		"监管处置决定换种类": {
			layerFact(t, "disp-1", domain.DispositionDecisionLayer, "DISPOSITION/DESTROY", domain.ReleaseKindInvalid, earlierAt),
			layerFact(t, "disp-2", domain.DispositionDecisionLayer, "DISPOSITION/RE-EXPORT", domain.ReleaseKindInvalid, laterAt),
		},
		"放行收窄": {
			release(t, "rel-1", domain.FullRelease, earlierAt),
			release(t, "rel-2", domain.PartialRelease, laterAt),
		},
		"同一时刻的更正": {
			layerFact(t, "acc-1", domain.BusinessAcceptanceLayer, "ACCEPTED", domain.ReleaseKindInvalid, laterAt),
			layerFact(t, "acc-2", domain.BusinessAcceptanceLayer, "NOT_ACCEPTED", domain.ReleaseKindInvalid, laterAt),
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.CheckLayerConsistency([]domain.LayerFact{test.original}, test.correction); !errors.Is(err, domain.ErrLayerConflict) {
				t.Fatalf("对照：不带更正指向时 err = %v，want ErrLayerConflict——不从业务时间猜更正", err)
			}
			correction := correcting(t, test.correction, test.original.Result.SourceID())
			supersedes, err := domain.CheckLayerConsistency([]domain.LayerFact{test.original}, correction)
			if err != nil {
				t.Fatalf("明确更正被判 %v——来源已经说了这是对原结果的更正", err)
			}
			if !supersedes {
				t.Fatal("同一范围上的明确更正没有生效：交集该按更正后的结果，原结果退出当前判断")
			}
		})
	}
}

// Covers: AT-CC-156「没有更正语义的矛盾结果保持冲突」的领域半边——更正关系只解释更正与它所
// 更正那份之间的差别，不解释与别的在册事实的差别；被取代的那份退出比对，此后到达的结果与更正后的那份比。
func TestACorrectionExplainsOnlyItsOwnTargetAndTheCorrectedFactLeavesTheComparison(t *testing.T) {
	original := layerFact(t, "duty-1", domain.AssessedDutyLayer, "DUTY/USD-120.00", domain.ReleaseKindInvalid, earlierAt)
	sameAsOriginal := layerFact(t, "duty-1b", domain.AssessedDutyLayer, "DUTY/USD-120.00", domain.ReleaseKindInvalid, earlierAt)
	correction := correcting(t,
		layerFact(t, "duty-2", domain.AssessedDutyLayer, "DUTY/USD-95.50", domain.ReleaseKindInvalid, laterAt), "duty-1")

	// 另一份未被更正的在册事实仍说 120：更正只指名了 duty-1，与 duty-1b 的矛盾没有更正语义可解释。
	if _, err := domain.CheckLayerConsistency([]domain.LayerFact{original, sameAsOriginal}, correction); !errors.Is(err, domain.ErrLayerConflict) {
		t.Fatalf("err = %v；与未被更正的同层事实矛盾，照判冲突", err)
	}

	booked := []domain.LayerFact{original, inForce(correction)}
	agreeing := layerFact(t, "duty-3", domain.AssessedDutyLayer, "DUTY/USD-95.50", domain.ReleaseKindInvalid, laterAt)
	if supersedes, err := domain.CheckLayerConsistency(booked, agreeing); err != nil || supersedes {
		t.Fatalf("与更正后的结果一致：supersedes=%v err=%v——被取代的那份不该再拿来比", supersedes, err)
	}
	stale := layerFact(t, "duty-4", domain.AssessedDutyLayer, "DUTY/USD-120.00", domain.ReleaseKindInvalid, laterAt)
	if _, err := domain.CheckLayerConsistency(booked, stale); !errors.Is(err, domain.ErrLayerConflict) {
		t.Fatalf("err = %v；再说回旧金额而不带更正指向，与当前那份矛盾", err)
	}

	// 更正链：更正那份本身再被更正，所指的是链尾，照样生效。
	again := correcting(t,
		layerFact(t, "duty-5", domain.AssessedDutyLayer, "DUTY/USD-101.00", domain.ReleaseKindInvalid, laterAt.Add(time.Minute)), "duty-2")
	if supersedes, err := domain.CheckLayerConsistency(booked, again); err != nil || !supersedes {
		t.Fatalf("更正链尾：supersedes=%v err=%v", supersedes, err)
	}
}

// Covers: 票 docs-consistency/code/04「所指的那一份本身已被更正过时」的代决——只许更正链尾。同一份原结果
// 已被另一份更正取代之后，又来一份指名它的更正，两份更正都声称取代它：按到达先后或业务时间择一都是替来源
// 判断谁更正了谁，所以后到的那份记冲突，原有的更正关系不变。
func TestASecondCorrectionOfAnAlreadyCorrectedResultIsAConflict(t *testing.T) {
	original := release(t, "rel-1", domain.FullRelease, earlierAt)
	first := inForce(correcting(t, release(t, "rel-2", domain.PartialRelease, laterAt), "rel-1"))
	second := correcting(t, release(t, "rel-3", domain.PartialRelease, laterAt), "rel-1")
	if _, err := domain.CheckLayerConsistency([]domain.LayerFact{original, first}, second); !errors.Is(err, domain.ErrLayerConflict) {
		t.Fatalf("err = %v；所指的那份已被更正过，第二份更正该记冲突", err)
	}
}

// Covers: AT-CC-156「新扩展范围独立校验」的单元一级退化——更正与所更正那份范围不同时交集为空：原结果在它的
// 范围上保持原判断，更正关系记下但不取代它；更正自己的范围照同层判法独立判断。
func TestACorrectionOnAnotherScopeLeavesTheOriginalAndIsJudgedOnItsOwnScope(t *testing.T) {
	original := release(t, "rel-1", domain.FullRelease, earlierAt)
	moved := correcting(t, inScope(t, release(t, "rel-2", domain.PartialRelease, laterAt), "declaration-unit-2"), "rel-1")
	supersedes, err := domain.CheckLayerConsistency([]domain.LayerFact{original}, moved)
	if err != nil || supersedes {
		t.Fatalf("交集为空：supersedes=%v err=%v——原范围保持原判断，更正不取代它", supersedes, err)
	}
	fullThere := inScope(t, release(t, "rel-0", domain.FullRelease, earlierAt), "declaration-unit-2")
	if _, err := domain.CheckLayerConsistency([]domain.LayerFact{original, fullThere}, moved); !errors.Is(err, domain.ErrLayerConflict) {
		t.Fatalf("err = %v；更正自己的范围上已有更早的全部放行，回退成部分照判冲突", err)
	}
}

// Covers: CC CONTEXT「任何前一层成功都不能自动生成后一层结果」与 UC-CC-006「同一层次和业务事项的后续明确
// 更正」——更正只在同层、同一提交版本的在册事实之间成立：所指那份不在这一版提交的在册事实里、在另一层，
// 或指名自己，都是矛盾输入；在册事实自称更正生效却说不出更正哪一份，是仓储不变量已破。
func TestACorrectionMustNameASameLayerFactOfTheSameSubmission(t *testing.T) {
	original := layerFact(t, "duty-1", domain.AssessedDutyLayer, "DUTY/USD-120.00", domain.ReleaseKindInvalid, earlierAt)
	stranger := correcting(t,
		layerFact(t, "duty-2", domain.AssessedDutyLayer, "DUTY/USD-95.50", domain.ReleaseKindInvalid, laterAt), "duty-elsewhere")
	if _, err := domain.CheckLayerConsistency([]domain.LayerFact{original}, stranger); !errors.Is(err, domain.ErrCorrectionOutsideSubmission) {
		t.Fatalf("err = %v, want ErrCorrectionOutsideSubmission", err)
	}
	otherVersion := reinterpreted(t, original, func(spec *domain.ExternalResultSpec) {
		spec.Version = mustValue(t, domain.NewSubmissionVersionID, "submission-1/v2")
	})
	pointing := correcting(t,
		layerFact(t, "duty-3", domain.AssessedDutyLayer, "DUTY/USD-95.50", domain.ReleaseKindInvalid, laterAt), "duty-1")
	if _, err := domain.CheckLayerConsistency([]domain.LayerFact{otherVersion}, pointing); !errors.Is(err, domain.ErrCorrectionOutsideSubmission) {
		t.Fatalf("err = %v；所指那份属另一版提交", err)
	}
	acrossLayers := correcting(t,
		layerFact(t, "disp-1", domain.DispositionDecisionLayer, "DISPOSITION/DESTROY", domain.ReleaseKindInvalid, laterAt), "duty-1")
	if _, err := domain.CheckLayerConsistency([]domain.LayerFact{original}, acrossLayers); !errors.Is(err, domain.ErrCorrectionAcrossLayers) {
		t.Fatalf("err = %v, want ErrCorrectionAcrossLayers", err)
	}

	self := domain.ExternalResultSpec{
		Layer:        domain.AssessedDutyLayer,
		SourceID:     "duty-9",
		Role:         original.Result.Role(),
		RawSemantics: "DUTY/USD-1.00",
		Rule:         original.Result.Rule(),
		Version:      original.Result.Version(),
		Attempt:      1,
		Scope:        original.Result.Scope(),
		OccurredAt:   laterAt,
		ReceivedAt:   resultReceivedAt,
		Corrects:     "duty-9",
	}
	if _, err := domain.InterpretExternalResult(self); !errors.Is(err, domain.ErrInvalidExternalResult) {
		t.Fatalf("err = %v；一份结果不能更正它自己", err)
	}

	claimless := inForce(layerFact(t, "duty-4", domain.AssessedDutyLayer, "DUTY/USD-95.50", domain.ReleaseKindInvalid, laterAt))
	if _, err := domain.CheckLayerConsistency([]domain.LayerFact{claimless}, original); !errors.Is(err, domain.ErrInvalidExternalResult) {
		t.Fatalf("err = %v；更正生效却不指名所更正那份", err)
	}
}
