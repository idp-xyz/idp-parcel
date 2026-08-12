package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var restrictedAt = time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

func restriction(t *testing.T, id string, constrains ...domain.GuardedAction) domain.RegulatoryRestriction {
	t.Helper()
	built, err := domain.EstablishRestriction(domain.RegulatoryRestrictionSpec{
		ID:          mustValue(t, domain.NewRestrictionID, id),
		Decision:    mustValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
		Scope:       mustValue(t, domain.NewDecisionScopeReference, "parcel-1"),
		Constrains:  constrains,
		EffectiveAt: restrictedAt,
	})
	if err != nil {
		t.Fatalf("establish restriction %q: %v", id, err)
	}
	return built
}

// Covers: CC CONTEXT「尚未放行或存在方向性限制时，必须阻断其明确约束的出库、装载
// 出发、跨关务区域移动或交付」——限制只阻断它明确约束的动作（没列的照行），阻断清单
// 列全（处置者要知道等谁）；接收、隔离、测量在动作枚举里没有格——「不因此阻止」是
// 结构性的，问不出「被阻断了吗」。
func TestARestrictionBlocksOnlyWhatItExplicitlyConstrains(t *testing.T) {
	hold := restriction(t, "restriction-1", domain.LoadingDeparture, domain.FinalDelivery)
	scope := mustValue(t, domain.NewDecisionScopeReference, "parcel-1")

	blocked, err := domain.JudgeActionAdmissibility(domain.LoadingDeparture, scope,
		[]domain.RegulatoryRestriction{hold})
	if err != nil {
		t.Fatalf("judge loading departure: %v", err)
	}
	if blocked.Admissible() || len(blocked.BlockedBy()) != 1 || blocked.BlockedBy()[0].String() != "restriction-1" {
		t.Fatalf("admissibility = %v blocked = %v", blocked.Admissible(), blocked.BlockedBy())
	}

	unconstrained, err := domain.JudgeActionAdmissibility(domain.OutboundRelease, scope,
		[]domain.RegulatoryRestriction{hold})
	if err != nil {
		t.Fatalf("judge outbound: %v", err)
	}
	if !unconstrained.Admissible() {
		t.Fatal("没列的动作被阻断了——限制只管它明确约束的")
	}

	foreignScope, err := domain.JudgeActionAdmissibility(domain.LoadingDeparture,
		mustValue(t, domain.NewDecisionScopeReference, "parcel-9"),
		[]domain.RegulatoryRestriction{hold})
	if err != nil {
		t.Fatalf("judge foreign scope: %v", err)
	}
	if !foreignScope.Admissible() {
		t.Fatal("别的对象被这份限制阻断了")
	}
}

// Covers: CC CONTEXT「执行方形成的控制或隔离事实不能解除扣留，只有责任来源接受的
// 监管结果才能解除相应监管限制」与「全部阻断性限制均已解除，相应动作才可继续」——
// 解除只凭监管结果引用（专名类型，隔离事实换不成它）；两限制解除其一仍阻断，全部
// 解除才放行；重复解除与早于生效的解除拒。
func TestReleaseComesOnlyFromRegulatoryOutcomesAndMustBeComplete(t *testing.T) {
	first := restriction(t, "restriction-1", domain.LoadingDeparture)
	second := restriction(t, "restriction-2", domain.LoadingDeparture)
	scope := mustValue(t, domain.NewDecisionScopeReference, "parcel-1")

	releasedFirst, err := first.ReleaseByRegulatoryOutcome(
		mustValue(t, domain.NewRegulatoryReleaseReference, "CUSTOMS-RELEASE/R-9"),
		restrictedAt.Add(6*time.Hour),
	)
	if err != nil {
		t.Fatalf("release first: %v", err)
	}
	if releasedFirst.Current() {
		t.Fatal("已解除的限制还有效")
	}
	if first.Current() != true {
		t.Fatal("原限制记录被改写了")
	}

	partial, err := domain.JudgeActionAdmissibility(domain.LoadingDeparture, scope,
		[]domain.RegulatoryRestriction{releasedFirst, second})
	if err != nil {
		t.Fatalf("judge partial: %v", err)
	}
	if partial.Admissible() {
		t.Fatal("部分解除放行了——全部阻断性限制均已解除才可继续")
	}
	if len(partial.BlockedBy()) != 1 || partial.BlockedBy()[0].String() != "restriction-2" {
		t.Fatalf("blocked = %v", partial.BlockedBy())
	}

	releasedSecond, err := second.ReleaseByRegulatoryOutcome(
		mustValue(t, domain.NewRegulatoryReleaseReference, "CUSTOMS-RELEASE/R-10"),
		restrictedAt.Add(7*time.Hour),
	)
	if err != nil {
		t.Fatalf("release second: %v", err)
	}
	admissible, err := domain.JudgeActionAdmissibility(domain.LoadingDeparture, scope,
		[]domain.RegulatoryRestriction{releasedFirst, releasedSecond})
	if err != nil {
		t.Fatalf("judge admissible: %v", err)
	}
	if !admissible.Admissible() {
		t.Fatal("全部解除后动作仍被阻断")
	}

	if _, err := releasedFirst.ReleaseByRegulatoryOutcome(
		mustValue(t, domain.NewRegulatoryReleaseReference, "CUSTOMS-RELEASE/R-11"),
		restrictedAt.Add(8*time.Hour),
	); !errors.Is(err, domain.ErrRestrictionNotCurrent) {
		t.Fatalf("err = %v; 解除解了两次", err)
	}
	if _, err := second.ReleaseByRegulatoryOutcome(
		mustValue(t, domain.NewRegulatoryReleaseReference, "CUSTOMS-RELEASE/R-12"),
		restrictedAt.Add(-time.Hour),
	); !errors.Is(err, domain.ErrInvalidRestriction) {
		t.Fatalf("err = %v; 早于生效的解除被收下了", err)
	}
}

// Covers: 构造防线——约束集必须显式非空且不重（「阻断其明确约束的」动作，说不出约束
// 什么的限制立不起来）；封闭四值外没有格。
func TestARestrictionDemandsItsExplicitConstraints(t *testing.T) {
	base := domain.RegulatoryRestrictionSpec{
		ID:          mustValue(t, domain.NewRestrictionID, "restriction-1"),
		Decision:    mustValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
		Scope:       mustValue(t, domain.NewDecisionScopeReference, "parcel-1"),
		EffectiveAt: restrictedAt,
	}

	if _, err := domain.EstablishRestriction(base); !errors.Is(err, domain.ErrInvalidRestriction) {
		t.Fatalf("err = %v; 空约束集立起了限制", err)
	}

	duplicated := base
	duplicated.Constrains = []domain.GuardedAction{domain.FinalDelivery, domain.FinalDelivery}
	if _, err := domain.EstablishRestriction(duplicated); !errors.Is(err, domain.ErrInvalidRestriction) {
		t.Fatalf("err = %v; 重复动作被收下了", err)
	}

	invalid := base
	invalid.Constrains = []domain.GuardedAction{domain.GuardedActionInvalid}
	if _, err := domain.EstablishRestriction(invalid); !errors.Is(err, domain.ErrInvalidRestriction) {
		t.Fatalf("err = %v; 封闭四值之外立起了约束", err)
	}
}
