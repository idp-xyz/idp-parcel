package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var intakeOccurredAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

func intakeSpec(t *testing.T, kind domain.IntakeSourceKind) domain.IntakeSourceSpec {
	t.Helper()
	return domain.IntakeSourceSpec{
		Kind:       kind,
		Object:     mustValue(t, domain.NewSourceObjectReference, "handling-unit-1"),
		Parcel:     mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		Place:      mustValue(t, domain.NewIntakePlaceReference, "node-origin"),
		Control:    mustValue(t, domain.NewIntakeControlReference, "NODE-CONTROL/NO-7"),
		Version:    mustValue(t, domain.NewSourceResultVersion, "intake-result/v1"),
		OccurredAt: intakeOccurredAt,
	}
}

func adoptedIntake(t *testing.T, kind domain.IntakeSourceKind) domain.EffectiveNetworkIntake {
	t.Helper()
	source, err := domain.NewIntakeSource(intakeSpec(t, kind))
	if err != nil {
		t.Fatalf("new intake source: %v", err)
	}
	intake, err := domain.AdoptNetworkIntake(source, mustValue(t, domain.NewSubmissionVersionID, "version-1"))
	if err != nil {
		t.Fatalf("adopt network intake: %v", err)
	}
	return intake
}

// Covers: `AT-PS-038`/`AT-PS-039`「合格节点收寄/场外揽收——以来源业务时间形成有效网络
// 收寄、责任起点和正式承诺，同语义、不虚构节点到站」——两种来源同一条采用链，责任起点
// 与承诺生效时间都恒等于物理收寄的实际发生时间（构造上收不进处理时间）。
func TestBothIntakeSourcesFormTheSameCommitmentSemantics(t *testing.T) {
	for _, kind := range []domain.IntakeSourceKind{domain.NodeIntakeSource, domain.OffsitePickupSource} {
		intake := adoptedIntake(t, kind)
		if !intake.ResponsibilityStart().Equal(intakeOccurredAt) {
			t.Fatalf("responsibility start = %s, want the physical occurrence time", intake.ResponsibilityStart())
		}

		commitment, err := domain.FormFormalCommitment(
			mustValue(t, domain.NewCommitmentVersionID, "commitment-1/v1"),
			intake,
			mustValue(t, domain.NewExpectedCommitmentReference, "expected/RES-1"),
		)
		if err != nil {
			t.Fatalf("form formal commitment (%s): %v", kind, err)
		}
		if !commitment.EffectiveAt().Equal(intakeOccurredAt) {
			t.Fatalf("effective at = %s; 生效时间必须来自实际收寄时间", commitment.EffectiveAt())
		}
		if commitment.Expected().String() != "expected/RES-1" {
			t.Fatal("正式承诺没有引用预计承诺")
		}
		if _, adjusted := commitment.PriorVersion(); adjusted {
			t.Fatal("首个版本凭空长出了前版")
		}
	}
}

// Covers: `AT-PS-048`「正式承诺形成后路由或 ETA 变化——原正式承诺不变；允许调整时形成
// 带原因的新版本」与 `AT-PS-050` 的承诺半边——调整换版本号、带原因、指回前版；不带
// 原因或重号的调整立不成。
func TestACommitmentAdjustsOnlyByAReasonedNewVersion(t *testing.T) {
	commitment, err := domain.FormFormalCommitment(
		mustValue(t, domain.NewCommitmentVersionID, "commitment-1/v1"),
		adoptedIntake(t, domain.NodeIntakeSource),
		mustValue(t, domain.NewExpectedCommitmentReference, "expected/RES-1"),
	)
	if err != nil {
		t.Fatalf("form formal commitment: %v", err)
	}

	adjusted, err := commitment.Adjust(
		mustValue(t, domain.NewCommitmentVersionID, "commitment-1/v2"),
		mustValue(t, domain.NewCommitmentAdjustmentReason, "SOURCE_CORRECTED/intake-result/v2"),
	)
	if err != nil {
		t.Fatalf("adjust: %v", err)
	}
	prior, present := adjusted.PriorVersion()
	if !present || prior.String() != "commitment-1/v1" {
		t.Fatalf("prior = %s present = %v; 调整必须指回前版", prior, present)
	}
	reason, present := adjusted.AdjustmentReason()
	if !present || reason.String() != "SOURCE_CORRECTED/intake-result/v2" {
		t.Fatal("调整没带原因——与静默改写分不开")
	}
	if commitment.Version().String() != "commitment-1/v1" {
		t.Fatal("原承诺被改写了")
	}

	if _, err := commitment.Adjust(commitment.Version(),
		mustValue(t, domain.NewCommitmentAdjustmentReason, "SOURCE_CORRECTED")); !errors.Is(err, domain.ErrInvalidFormalCommitment) {
		t.Fatalf("err = %v; 重号的调整分不出两版", err)
	}
	if _, err := commitment.Adjust(
		mustValue(t, domain.NewCommitmentVersionID, "commitment-1/v3"),
		domain.CommitmentAdjustmentReason{}); !errors.Is(err, domain.ErrInvalidFormalCommitment) {
		t.Fatalf("err = %v; 不带原因的调整被收下了", err)
	}
}

// Covers: `AT-PS-043` 的领域面——来源联合封闭二值，普通扫描、车辆到场或任务创建没有格
// 可落；来源五件（对象/地点/控制/版本/时间）缺一立不起来，采用缺基线锚挂不回接受。
func TestAnIntakeSourceRefusesHintsAndHalfShapes(t *testing.T) {
	if _, err := domain.NewIntakeSource(intakeSpec(t, domain.IntakeSourceKindInvalid)); !errors.Is(err, domain.ErrInvalidIntakeSource) {
		t.Fatalf("err = %v, want ErrInvalidIntakeSource", err)
	}

	cases := map[string]func(domain.IntakeSourceSpec) domain.IntakeSourceSpec{
		"no control": func(spec domain.IntakeSourceSpec) domain.IntakeSourceSpec {
			spec.Control = domain.IntakeControlReference{}
			return spec
		},
		"no place": func(spec domain.IntakeSourceSpec) domain.IntakeSourceSpec {
			spec.Place = domain.IntakePlaceReference{}
			return spec
		},
		"no source version": func(spec domain.IntakeSourceSpec) domain.IntakeSourceSpec {
			spec.Version = domain.SourceResultVersion{}
			return spec
		},
		"no occurrence time": func(spec domain.IntakeSourceSpec) domain.IntakeSourceSpec {
			spec.OccurredAt = time.Time{}
			return spec
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewIntakeSource(mutate(intakeSpec(t, domain.NodeIntakeSource))); !errors.Is(err, domain.ErrInvalidIntakeSource) {
				t.Fatalf("err = %v, want ErrInvalidIntakeSource", err)
			}
		})
	}

	source, err := domain.NewIntakeSource(intakeSpec(t, domain.NodeIntakeSource))
	if err != nil {
		t.Fatalf("new intake source: %v", err)
	}
	if _, err := domain.AdoptNetworkIntake(source, domain.SubmissionVersionID{}); !errors.Is(err, domain.ErrInvalidNetworkIntake) {
		t.Fatalf("err = %v; 没有基线锚的采用挂不回任何一次接受", err)
	}
}
