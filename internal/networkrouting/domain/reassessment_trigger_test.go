package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func triggerSpec(t *testing.T, control domain.ControlEvidenceKind) domain.ReassessmentTriggerSpec {
	t.Helper()
	return domain.ReassessmentTriggerSpec{
		Correlation:   mustValue(t, domain.NewRequestCorrelationID, "trigger-1"),
		Key:           judgmentKeyFor(t, "parcel-1"),
		Control:       control,
		Location:      mustValue(t, domain.NewActualLocationReference, "node-actual"),
		SourceVersion: mustValue(t, domain.NewSourceFactVersionReference, "intake-fact/v1"),
		OccurredAt:    time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC),
		AcceptedAt:    time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC),
	}
}

// Covers: UC-NR-003 复核层次 1「收寄、控制、实测及业务时间是否权威、当前且可关联」——
// 节点收寄与权威运输交接两类控制依据立得起触发，位置、来源版本与业务时间随触发齐备。
func TestAuthoritativeControlEvidenceEstablishesATrigger(t *testing.T) {
	for _, control := range []domain.ControlEvidenceKind{
		domain.NodeIntakeControl,
		domain.TransportHandoverControl,
	} {
		trigger, err := domain.NewReassessmentTrigger(triggerSpec(t, control))
		if err != nil {
			t.Fatalf("new trigger with %s: %v", control, err)
		}
		if trigger.Location().String() != "node-actual" || trigger.SourceVersion().String() != "intake-fact/v1" {
			t.Fatalf("trigger = %#v; 位置与来源版本没有随触发保全", trigger)
		}
	}
}

// Covers: CONTEXT「当前可控节点」硬句与 UC-NR-003「普通扫描、位置消息、装卸或车辆到场
// 只能作为来源线索，不能……建立当前可控节点」——线索级依据用自己的哨兵拒绝，接入层据以
// 把「保全线索等权威依据」与「修坏触发」分开。
func TestASourceHintNeverTriggersReassessment(t *testing.T) {
	_, err := domain.NewReassessmentTrigger(triggerSpec(t, domain.SourceHint))
	if !errors.Is(err, domain.ErrSourceHintDoesNotTrigger) {
		t.Fatalf("err = %v, want ErrSourceHintDoesNotTrigger", err)
	}

	cases := map[string]func(domain.ReassessmentTriggerSpec) domain.ReassessmentTriggerSpec{
		"no location": func(spec domain.ReassessmentTriggerSpec) domain.ReassessmentTriggerSpec {
			spec.Location = domain.ActualLocationReference{}
			return spec
		},
		"no source version": func(spec domain.ReassessmentTriggerSpec) domain.ReassessmentTriggerSpec {
			spec.SourceVersion = domain.SourceFactVersionReference{}
			return spec
		},
		"no business time": func(spec domain.ReassessmentTriggerSpec) domain.ReassessmentTriggerSpec {
			spec.OccurredAt = time.Time{}
			return spec
		},
		"no acceptance time": func(spec domain.ReassessmentTriggerSpec) domain.ReassessmentTriggerSpec {
			spec.AcceptedAt = time.Time{}
			return spec
		},
		"no control kind": func(spec domain.ReassessmentTriggerSpec) domain.ReassessmentTriggerSpec {
			spec.Control = domain.ControlEvidenceKindInvalid
			return spec
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewReassessmentTrigger(mutate(triggerSpec(t, domain.NodeIntakeControl))); !errors.Is(err, domain.ErrInvalidReassessmentTrigger) {
				t.Fatalf("err = %v, want ErrInvalidReassessmentTrigger", err)
			}
		})
	}
}

// Covers: AT-NR-079、AT-NR-081（领域那一半）——触发只在业务时间严格早于当前位置时才算早；同一时刻不算更早，较晚的更不算。
// 比的是业务时间：触发换了时区写法也是同一时刻。
func TestATriggerPrecedesThePositionOnlyWhenItsBusinessTimeIsStrictlyEarlier(t *testing.T) {
	trigger, err := domain.NewReassessmentTrigger(triggerSpec(t, domain.TransportHandoverControl))
	if err != nil {
		t.Fatalf("new trigger: %v", err)
	}
	at := trigger.OccurredAt()
	shanghai := time.FixedZone("UTC+8", 8*60*60)
	for name, tc := range map[string]struct {
		positionAt time.Time
		want       bool
	}{
		"当前位置更晚":     {at.Add(time.Minute), true},
		"同一时刻":       {at, false},
		"同一时刻换了时区写法": {at.In(shanghai), false},
		"当前位置更早":     {at.Add(-time.Minute), false},
	} {
		if got := trigger.PrecedesPosition(tc.positionAt); got != tc.want {
			t.Errorf("%s：PrecedesPosition = %v, want %v", name, got, tc.want)
		}
	}
}

// Covers: AT-NR-087（领域那一半）——触发只在业务时间严格早于这一判断键的委托接受时刻时才算早于接受；同一时刻不算更早，
// 较晚的更不算。比的是业务时间：接受时刻换了时区写法也是同一时刻。
func TestATriggerPrecedesTheAcceptanceOnlyWhenItsBusinessTimeIsStrictlyEarlier(t *testing.T) {
	shanghai := time.FixedZone("UTC+8", 8*60*60)
	at := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		acceptedAt time.Time
		want       bool
	}{
		"接受更晚":       {at.Add(time.Minute), true},
		"同一时刻":       {at, false},
		"同一时刻换了时区写法": {at.In(shanghai), false},
		"接受更早":       {at.Add(-time.Minute), false},
	} {
		spec := triggerSpec(t, domain.NodeIntakeControl)
		spec.OccurredAt = at
		spec.AcceptedAt = tc.acceptedAt
		trigger, err := domain.NewReassessmentTrigger(spec)
		if err != nil {
			t.Fatalf("%s：new trigger: %v", name, err)
		}
		if got := trigger.PrecedesAcceptance(); got != tc.want {
			t.Errorf("%s：PrecedesAcceptance = %v, want %v", name, got, tc.want)
		}
		if !trigger.AcceptedAt().Equal(tc.acceptedAt) {
			t.Errorf("%s：AcceptedAt = %s, want %s", name, trigger.AcceptedAt(), tc.acceptedAt)
		}
	}
}

// Covers: 实测变化重判抄锚点立得起来，线索级控制依据照旧立不成；缺位置、缺时间、缺来源版本也拒。
// 关联前缀是 measurement-change/，带包裹与来源身份，过得了改路触发引用的门，来源身份能从关联还原（含斜杠）。
// 四段的关联不是这种形状，还原来源身份时拒绝。
func TestAMeasurementChangeRejudgeCopiesTheAnchorAndRefusesASourceHint(t *testing.T) {
	spec := triggerSpec(t, domain.NodeIntakeControl)
	rejudge, err := domain.NewMeasurementChangeRejudge(spec)
	if err != nil {
		t.Fatalf("重判：%v", err)
	}
	if rejudge.Control() != domain.NodeIntakeControl || rejudge.Location().String() != spec.Location.String() ||
		!rejudge.OccurredAt().Equal(spec.OccurredAt) {
		t.Fatalf("重判没有抄下锚点：%s %s %s", rejudge.Control(), rejudge.Location(), rejudge.OccurredAt())
	}
	plain, err := domain.NewReassessmentTrigger(spec)
	if err != nil {
		t.Fatal(err)
	}
	if plain.ParcelFactsPoint().Kind() == domain.MeasurementChangeFactsPoint {
		t.Fatal("既有控制触发的判断点被改成了实测变化重判")
	}

	hint := spec
	hint.Control = domain.SourceHint
	if _, err := domain.NewMeasurementChangeRejudge(hint); !errors.Is(err, domain.ErrSourceHintDoesNotTrigger) {
		t.Fatalf("线索：err = %v, want ErrSourceHintDoesNotTrigger", err)
	}
	for name, mutate := range map[string]func(domain.ReassessmentTriggerSpec) domain.ReassessmentTriggerSpec{
		"没位置": func(spec domain.ReassessmentTriggerSpec) domain.ReassessmentTriggerSpec {
			spec.Location = domain.ActualLocationReference{}
			return spec
		},
		"没业务时间": func(spec domain.ReassessmentTriggerSpec) domain.ReassessmentTriggerSpec {
			spec.OccurredAt = time.Time{}
			return spec
		},
		"没来源版本": func(spec domain.ReassessmentTriggerSpec) domain.ReassessmentTriggerSpec {
			spec.SourceVersion = domain.SourceFactVersionReference{}
			return spec
		},
	} {
		if _, err := domain.NewMeasurementChangeRejudge(mutate(spec)); !errors.Is(err, domain.ErrInvalidReassessmentTrigger) {
			t.Fatalf("%s：err = %v, want ErrInvalidReassessmentTrigger", name, err)
		}
	}

	tenant := spec.Key.TenantID
	parcel := spec.Key.DeclaredParcelID
	correlation, err := domain.NewMeasurementChangeCorrelation(tenant, parcel, "src/1")
	if err != nil {
		t.Fatalf("关联：%v", err)
	}
	if !strings.HasPrefix(correlation.String(), "measurement-change/") {
		t.Fatalf("关联 = %q，want 前缀 measurement-change/", correlation)
	}
	if _, err := domain.NewRerouteTriggerReference(correlation.String()); err != nil {
		t.Fatalf("关联当不了改路触发引用：%v", err)
	}
	source, err := domain.MeasurementChangeSourceVersion(correlation)
	if err != nil || source.String() != "src/1" {
		t.Fatalf("还原来源身份 = %q, err = %v, want src/1", source, err)
	}
	if _, err := domain.NewMeasurementChangeCorrelation(tenant, parcel, " "); !errors.Is(err, domain.ErrInvalidReassessmentTrigger) {
		t.Fatalf("空白来源：err = %v", err)
	}
	four, err := domain.NewRequestCorrelationID("measurement-change/1:t/1:p/3:src/4:more")
	if err != nil {
		t.Fatalf("四段关联：%v", err)
	}
	if _, err := domain.MeasurementChangeSourceVersion(four); !errors.Is(err, domain.ErrInvalidReassessmentTrigger) {
		t.Fatalf("四段关联：err = %v, want ErrInvalidReassessmentTrigger", err)
	}
}
