package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var (
	factOccurredAt  = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)
	factEffectiveAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)
	factReceivedAt  = time.Date(2026, 8, 9, 9, 45, 0, 0, time.UTC)
)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func factSpec(t *testing.T) domain.AcceptedSourceFactSpec {
	t.Helper()
	return domain.AcceptedSourceFactSpec{
		Source:      domain.SourceNodeOperations,
		Parcel:      mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Fact:        mustValue(t, domain.NewSourceFactReference, "NODE-INTAKE/intake-result"),
		Kind:        mustValue(t, domain.NewSourceFactKind, "node-intake"),
		Version:     mustValue(t, domain.NewSourceFactVersion, "intake-result/v1"),
		OccurredAt:  factOccurredAt,
		EffectiveAt: factEffectiveAt,
		ReceivedAt:  factReceivedAt,
	}
}

func classified(t *testing.T, parcel string) domain.MilestoneClassification {
	t.Helper()
	spec := factSpec(t)
	spec.Parcel = mustValue(t, domain.NewTrackedParcelReference, parcel)
	fact, err := domain.NewAcceptedSourceFact(spec)
	if err != nil {
		t.Fatalf("new accepted source fact: %v", err)
	}
	classification, err := domain.ClassifyMilestone(
		fact,
		mustValue(t, domain.NewMilestoneReference, "PICKED_UP"),
		mustValue(t, domain.NewMappingVersionReference, "milestone-map/v1"),
	)
	if err != nil {
		t.Fatalf("classify milestone: %v", err)
	}
	return classification
}

// Covers: VE CONTEXT「来源原文、来源代码、业务发生时间、有效时间和接收时间分别保存」
// 与「投影只消费已接受事实」——三时间各归各位（迟到事实的「什么时候发生」与「什么
// 时候才知道」分得开），源上下文封闭五值，来源消息与外部状态码没有格可落。
func TestAnAcceptedFactKeepsItsThreeTimesApart(t *testing.T) {
	fact, err := domain.NewAcceptedSourceFact(factSpec(t))
	if err != nil {
		t.Fatalf("new accepted source fact: %v", err)
	}
	if !fact.OccurredAt().Equal(factOccurredAt) ||
		!fact.EffectiveAt().Equal(factEffectiveAt) ||
		!fact.ReceivedAt().Equal(factReceivedAt) {
		t.Fatalf("times = %s/%s/%s; 三时间必须分别保存",
			fact.OccurredAt(), fact.EffectiveAt(), fact.ReceivedAt())
	}
	if fact.Kind().String() != "node-intake" {
		t.Fatalf("kind = %q; 已接受事实必须携带源上下文拥有的事实类型", fact.Kind())
	}

	invalid := factSpec(t)
	invalid.Source = domain.SourceContextInvalid
	if _, err := domain.NewAcceptedSourceFact(invalid); !errors.Is(err, domain.ErrInvalidSourceFact) {
		t.Fatalf("err = %v; 封闭五值之外立起了源事实", err)
	}
	missingReceived := factSpec(t)
	missingReceived.ReceivedAt = time.Time{}
	if _, err := domain.NewAcceptedSourceFact(missingReceived); !errors.Is(err, domain.ErrInvalidSourceFact) {
		t.Fatalf("err = %v; 缺接收时间的事实被收下了", err)
	}
	missingKind := factSpec(t)
	missingKind.Kind = domain.SourceFactKind{}
	if _, err := domain.NewAcceptedSourceFact(missingKind); !errors.Is(err, domain.ErrInvalidSourceFact) {
		t.Fatalf("err = %v; 缺事实类型的事实被收下了", err)
	}
}

// Covers: VE CONTEXT「标准追踪里程碑映射必须具有版本和适用范围；无法可靠映射时保持
// 未归类，不能为了得到完整时间线强行映射为『运输中』」——未归类是显式构造的真话且
// 映射版本仍必备；归类与未归类都能进投影。
func TestUnclassifiableFactsStayHonestlyUnclassified(t *testing.T) {
	fact, err := domain.NewAcceptedSourceFact(factSpec(t))
	if err != nil {
		t.Fatalf("new accepted source fact: %v", err)
	}

	unclassified, err := domain.LeaveUnclassified(fact,
		mustValue(t, domain.NewMappingVersionReference, "milestone-map/v1"))
	if err != nil {
		t.Fatalf("leave unclassified: %v", err)
	}
	if _, has := unclassified.Milestone(); has {
		t.Fatal("未归类凭空长出了里程碑")
	}
	if unclassified.MappingVersion().String() != "milestone-map/v1" {
		t.Fatal("未归类没带映射版本——连按哪套话语归不进都说不出")
	}

	if _, err := domain.LeaveUnclassified(fact, domain.MappingVersionReference{}); !errors.Is(err, domain.ErrInvalidClassification) {
		t.Fatalf("err = %v; 没有映射版本的未归类被收下了", err)
	}
	if _, err := domain.ClassifyMilestone(fact, domain.MilestoneReference{},
		mustValue(t, domain.NewMappingVersionReference, "milestone-map/v1")); !errors.Is(err, domain.ErrInvalidClassification) {
		t.Fatalf("err = %v; 空里程碑的归类被收下了", err)
	}
}

// Covers: VE CONTEXT 全程追踪投影生命周期「迟到事实、来源更正或映射版本适用性变化→
// 形成新的当前投影；原投影版本继续保留」与「面向明确包裹」——重派生换版本指回原版、
// 原投影不可变；混入别的包裹的事实拼不成一条旅程。
func TestAProjectionRederivesWithoutRewritingItsHistory(t *testing.T) {
	first, err := domain.DeriveTrackingProjection(
		mustValue(t, domain.NewProjectionVersionID, "projection-1/v1"),
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		[]domain.MilestoneClassification{classified(t, "parcel-1")},
		factReceivedAt,
	)
	if err != nil {
		t.Fatalf("derive projection: %v", err)
	}
	if _, rederived := first.PriorVersion(); rederived {
		t.Fatal("首个版本凭空长出了前版")
	}

	second, err := first.Rederive(
		mustValue(t, domain.NewProjectionVersionID, "projection-1/v2"),
		[]domain.MilestoneClassification{classified(t, "parcel-1"), classified(t, "parcel-1")},
		factReceivedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("rederive: %v", err)
	}
	prior, present := second.PriorVersion()
	if !present || prior.String() != "projection-1/v1" {
		t.Fatalf("prior = %s present = %v; 重派生必须指回原版", prior, present)
	}
	if len(first.Entries()) != 1 {
		t.Fatal("原投影被改写了")
	}

	if _, err := first.Rederive(first.Version(),
		[]domain.MilestoneClassification{classified(t, "parcel-1")}, factReceivedAt); !errors.Is(err, domain.ErrInvalidTrackingProjection) {
		t.Fatalf("err = %v; 重号的重派生分不出两版", err)
	}
	if _, err := domain.DeriveTrackingProjection(
		mustValue(t, domain.NewProjectionVersionID, "projection-9/v1"),
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		[]domain.MilestoneClassification{classified(t, "parcel-9")},
		factReceivedAt,
	); !errors.Is(err, domain.ErrInvalidTrackingProjection) {
		t.Fatalf("err = %v; 混入别的包裹的事实拼成了一条旅程", err)
	}
}
