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

// Covers: `AT-PS-050` 的来源半边（ADR-0117 决定一）——来源可以自报「更正哪一版」，那一格
// 只从来源所有者的事实带出：不声明即首登（`Corrects` 缺席），声明了就得指向另一个版本，
// 自指的更正立不起来。
func TestAnIntakeSourceCarriesTheCorrectedVersionOnlyWhenTheSourceDeclaresIt(t *testing.T) {
	first, err := domain.NewIntakeSource(intakeSpec(t, domain.OffsitePickupSource))
	if err != nil {
		t.Fatalf("new intake source: %v", err)
	}
	if _, corrects := first.Corrects(); corrects {
		t.Fatal("首登来源凭空长出了被更正版本")
	}

	spec := intakeSpec(t, domain.OffsitePickupSource)
	spec.Version = mustValue(t, domain.NewSourceResultVersion, "intake-result/v2")
	spec.Corrects = mustValue(t, domain.NewSourceResultVersion, "intake-result/v1")
	corrected, err := domain.NewIntakeSource(spec)
	if err != nil {
		t.Fatalf("new corrected intake source: %v", err)
	}
	if prior, corrects := corrected.Corrects(); !corrects || prior.String() != "intake-result/v1" {
		t.Fatalf("corrects = %s present = %v; 更正来源没带回它更正的那一版", prior, corrects)
	}

	spec.Corrects = spec.Version
	if _, err := domain.NewIntakeSource(spec); !errors.Is(err, domain.ErrInvalidIntakeSource) {
		t.Fatalf("err = %v; 自指的更正被收下了", err)
	}
}

// Covers: `AT-PS-050` 的承诺半边（ADR-0117 决定三）——以更正后的收寄重述承诺：新版本号、
// 收寄换成更正后那一份、生效时间随更正后的发生时刻走、指回前版、带原因；预计承诺引用不变。
// 换包裹、换接受基线、换来源种类的「更正」都立不成——那是另一份采用，不是这份的更正；
// 重号与不带原因照 Adjust 的纪律同拒。
func TestACommitmentIsRestatedOnTheCorrectedIntake(t *testing.T) {
	original, err := domain.FormFormalCommitment(
		mustValue(t, domain.NewCommitmentVersionID, "commitment-1/v1"),
		adoptedIntake(t, domain.OffsitePickupSource),
		mustValue(t, domain.NewExpectedCommitmentReference, "expected/RES-1"),
	)
	if err != nil {
		t.Fatalf("form formal commitment: %v", err)
	}

	correctedAt := intakeOccurredAt.Add(-10 * time.Minute)
	correctedSpec := intakeSpec(t, domain.OffsitePickupSource)
	correctedSpec.Version = mustValue(t, domain.NewSourceResultVersion, "intake-result/v2")
	correctedSpec.Corrects = mustValue(t, domain.NewSourceResultVersion, "intake-result/v1")
	correctedSpec.Place = mustValue(t, domain.NewIntakePlaceReference, "kerbside-7")
	correctedSpec.OccurredAt = correctedAt
	correctedSource, err := domain.NewIntakeSource(correctedSpec)
	if err != nil {
		t.Fatalf("new corrected source: %v", err)
	}
	correctedIntake, err := domain.AdoptNetworkIntake(correctedSource, mustValue(t, domain.NewSubmissionVersionID, "version-1"))
	if err != nil {
		t.Fatalf("adopt corrected intake: %v", err)
	}
	reason := mustValue(t, domain.NewCommitmentAdjustmentReason, "SOURCE_CORRECTED/OFFSITE_PICKUP/intake-result/v1")

	restated, err := original.RestateOnCorrectedIntake(
		mustValue(t, domain.NewCommitmentVersionID, "commitment-1/v2"), correctedIntake, reason)
	if err != nil {
		t.Fatalf("restate on corrected intake: %v", err)
	}
	if !restated.EffectiveAt().Equal(correctedAt) || !restated.Intake().ResponsibilityStart().Equal(correctedAt) {
		t.Fatalf("effective at = %s; 生效时间必须随更正后的发生时刻走", restated.EffectiveAt())
	}
	if restated.Intake().Source().Place().String() != "kerbside-7" || restated.Intake().Source().Version().String() != "intake-result/v2" {
		t.Fatal("重述的承诺没有换上更正后的收寄")
	}
	if prior, present := restated.PriorVersion(); !present || prior.String() != "commitment-1/v1" {
		t.Fatalf("prior = %s present = %v; 重述必须指回前版", prior, present)
	}
	if got, present := restated.AdjustmentReason(); !present || got != reason {
		t.Fatal("重述没带原因——与静默改写分不开")
	}
	if restated.Expected() != original.Expected() || restated.Parcel() != original.Parcel() {
		t.Fatal("重述换了预计承诺引用或包裹")
	}
	if !original.EffectiveAt().Equal(intakeOccurredAt) || original.Intake().Source().Version().String() != "intake-result/v1" {
		t.Fatal("原承诺被改写了")
	}

	otherParcel := correctedSpec
	otherParcel.Parcel = mustValue(t, domain.NewDeclaredParcelID, "parcel-2")
	otherBaseline := mustValue(t, domain.NewSubmissionVersionID, "version-2")
	otherKind := correctedSpec
	otherKind.Kind = domain.NodeIntakeSource
	cases := map[string]func() (domain.EffectiveNetworkIntake, error){
		"another parcel": func() (domain.EffectiveNetworkIntake, error) {
			source, err := domain.NewIntakeSource(otherParcel)
			if err != nil {
				return domain.EffectiveNetworkIntake{}, err
			}
			return domain.AdoptNetworkIntake(source, correctedIntake.Baseline())
		},
		"another acceptance baseline": func() (domain.EffectiveNetworkIntake, error) {
			return domain.AdoptNetworkIntake(correctedSource, otherBaseline)
		},
		"another source kind": func() (domain.EffectiveNetworkIntake, error) {
			source, err := domain.NewIntakeSource(otherKind)
			if err != nil {
				return domain.EffectiveNetworkIntake{}, err
			}
			return domain.AdoptNetworkIntake(source, correctedIntake.Baseline())
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			intake, err := build()
			if err != nil {
				t.Fatalf("build intake: %v", err)
			}
			if _, err := original.RestateOnCorrectedIntake(
				mustValue(t, domain.NewCommitmentVersionID, "commitment-1/v9"), intake, reason); !errors.Is(err, domain.ErrInvalidFormalCommitment) {
				t.Fatalf("err = %v; 不是这份承诺的收寄被当成了它的更正", err)
			}
		})
	}
	if _, err := original.RestateOnCorrectedIntake(original.Version(), correctedIntake, reason); !errors.Is(err, domain.ErrInvalidFormalCommitment) {
		t.Fatalf("err = %v; 重号的重述分不出两版", err)
	}
	if _, err := original.RestateOnCorrectedIntake(
		mustValue(t, domain.NewCommitmentVersionID, "commitment-1/v3"), correctedIntake, domain.CommitmentAdjustmentReason{}); !errors.Is(err, domain.ErrInvalidFormalCommitment) {
		t.Fatalf("err = %v; 不带原因的重述被收下了", err)
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
