package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

func factAt(t *testing.T, parcel, reference string, occurredAt time.Time) domain.AcceptedSourceFact {
	t.Helper()
	fact, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
		Source:      domain.SourceNodeOperations,
		Parcel:      mustValue(t, domain.NewTrackedParcelReference, parcel),
		Fact:        mustValue(t, domain.NewSourceFactReference, reference),
		Kind:        mustValue(t, domain.NewSourceFactKind, "node-intake"),
		Version:     mustValue(t, domain.NewSourceFactVersion, reference+"/v1"),
		OccurredAt:  occurredAt,
		EffectiveAt: occurredAt,
		ReceivedAt:  occurredAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("new accepted source fact: %v", err)
	}
	return fact
}

// Covers: VE CONTEXT「多个有效事实冲突时，依据……业务发生时间……形成版本化投影判断，
// 不采用全局来源排名或最后消息覆盖」——业务时间严格可排即裁决（接收时间晚到的事实
// 不因晚到而赢，排序只看发生时间），裁决依据随判断给出。
func TestBusinessTimeResolvesWhatArrivalOrderMustNot(t *testing.T) {
	early := factAt(t, "parcel-1", "NODE-INTAKE/a", time.Date(2026, 8, 9, 7, 0, 0, 0, time.UTC))
	late := factAt(t, "parcel-1", "TRANSPORT-MOVE/b", time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC))

	// 传入顺序故意倒着放——到达顺序不是裁决维度。
	judgment, err := domain.ResolveByBusinessTime([]domain.AcceptedSourceFact{late, early})
	if err != nil {
		t.Fatalf("resolve by business time: %v", err)
	}

	if !judgment.Resolved() {
		t.Fatal("严格可排的冲突没有裁决")
	}
	basis, present := judgment.Basis()
	if !present || basis != domain.ResolvedByBusinessTime {
		t.Fatalf("basis = %s present = %v", basis, present)
	}
	ordered := judgment.Ordered()
	if len(ordered) != 2 || ordered[0].Fact().String() != "NODE-INTAKE/a" {
		t.Fatalf("ordered = %v; 全序必须按业务发生时间不按传入顺序", ordered)
	}
}

// Covers: VE CONTEXT「冲突仍无法裁决时，必须保留各项事实及冲突关系，投影保持信息
// 待确认并形成适用异常信号」——同刻事实排不出全序（硬凑与最后消息覆盖没有区别），
// 全部事实一份不少地保留；异常信号携带全部保留事实，已裁决的判断形不成冲突信号。
func TestAnUnresolvableConflictRetainsEverythingAndRaisesASignal(t *testing.T) {
	tied := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	first := factAt(t, "parcel-1", "NODE-INTAKE/a", tied)
	second := factAt(t, "parcel-1", "TRANSPORT-MOVE/b", tied)

	judgment, err := domain.ResolveByBusinessTime([]domain.AcceptedSourceFact{first, second})
	if err != nil {
		t.Fatalf("resolve by business time: %v", err)
	}
	if judgment.Resolved() {
		t.Fatal("同刻事实被硬凑出了顺序")
	}
	if len(judgment.Retained()) != 2 {
		t.Fatalf("retained = %d; 各项事实必须一份不少地保留", len(judgment.Retained()))
	}

	signal, err := domain.RaiseConflictSignal(
		mustValue(t, domain.NewExceptionSignalKindReference, "FACT_CONFLICT_UNRESOLVABLE"),
		judgment,
		tied.Add(3*time.Hour),
	)
	if err != nil {
		t.Fatalf("raise conflict signal: %v", err)
	}
	if len(signal.Facts()) != 2 || signal.Parcel().String() != "parcel-1" {
		t.Fatalf("signal = %#v; 信号必须携带全部保留事实", signal)
	}

	resolved, err := domain.ResolveByBusinessTime([]domain.AcceptedSourceFact{
		factAt(t, "parcel-1", "NODE-INTAKE/a", tied),
		factAt(t, "parcel-1", "TRANSPORT-MOVE/b", tied.Add(time.Hour)),
	})
	if err != nil {
		t.Fatalf("resolve orderable: %v", err)
	}
	if _, err := domain.RaiseConflictSignal(
		mustValue(t, domain.NewExceptionSignalKindReference, "FACT_CONFLICT_UNRESOLVABLE"),
		resolved,
		tied.Add(3*time.Hour),
	); !errors.Is(err, domain.ErrInvalidExceptionSignal) {
		t.Fatalf("err = %v; 已裁决的判断形成了冲突信号", err)
	}
}

// Covers: 输入防线——少于两份构不成冲突；混入别的包裹的事实不是同一场冲突。
func TestConflictInputDemandsTwoFactsOfTheSameParcel(t *testing.T) {
	single := factAt(t, "parcel-1", "NODE-INTAKE/a", time.Date(2026, 8, 9, 7, 0, 0, 0, time.UTC))
	if _, err := domain.ResolveByBusinessTime([]domain.AcceptedSourceFact{single}); !errors.Is(err, domain.ErrInvalidConflictInput) {
		t.Fatalf("err = %v; 一份事实构成了冲突", err)
	}

	foreign := factAt(t, "parcel-9", "TRANSPORT-MOVE/b", time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC))
	if _, err := domain.ResolveByBusinessTime([]domain.AcceptedSourceFact{single, foreign}); !errors.Is(err, domain.ErrInvalidConflictInput) {
		t.Fatalf("err = %v; 混入别的包裹的事实被当成了同一场冲突", err)
	}
}
