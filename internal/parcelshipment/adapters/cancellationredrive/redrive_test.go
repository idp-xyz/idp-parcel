package cancellationredrive_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/adoptconsume"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/cancellationredrive"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证续办适配器的编排纪律（ADR-0295 决定四）：先取包裹取消边界锁再读目标与原料；回放原料逐一交回各自的编排，用当前已接受委托作目标；
// 各条链的结果经它自己消费门的译表入账或回滚（这里的替身交回零值结果，译表必拒它，借此证明结果确实过了那张表）；冲突落在面单渠道服务的首次有效收寄
// 上才读 TF 链尾，且只在已形成时交面单终局判断。链上真正形成责任起点与终局的往返证明在 cmd/parcel-dispatch。

type steps struct{ log []string }

func (s *steps) add(step string) { s.log = append(s.log, step) }

type lockDouble struct {
	steps *steps
	err   error
}

func (double *lockDouble) LockCancellationBoundary(context.Context, psdomain.TenantID, psdomain.DeclaredParcelID) error {
	double.steps.add("lock")
	return double.err
}

type targetsDouble struct {
	steps  *steps
	target psdomain.CurrentAcceptedParcelTarget
	found  bool
	err    error
}

func (double *targetsDouble) FindCurrentAcceptedByParcel(
	context.Context, psdomain.TenantID, psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	double.steps.add("target")
	return double.target, double.found, double.err
}

type replaysDouble struct {
	steps   *steps
	replays []psports.CancellationBlockedReplay
	err     error
	asked   []string
}

func (double *replaysDouble) ListCancellationBlockedReplays(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID, cancellation psdomain.ParcelCancellationID,
) ([]psports.CancellationBlockedReplay, error) {
	double.steps.add("replays")
	double.asked = append(double.asked, tenant.String()+"/"+parcel.String()+"/"+cancellation.String())
	return double.replays, double.err
}

type pickupsDouble struct {
	steps    *steps
	boundary psports.CarrierPickupBoundary
	err      error
}

func (double *pickupsDouble) FindCurrentCarrierPickup(context.Context, psdomain.TenantID, psdomain.DeclaredParcelID) (psports.CarrierPickupBoundary, error) {
	double.steps.add("pickup")
	return double.boundary, double.err
}

type intakeDouble struct {
	steps    *steps
	commands []psapplication.AdoptNetworkIntakeCommand
	err      error
}

func (double *intakeDouble) Handle(_ context.Context, command psapplication.AdoptNetworkIntakeCommand) (psapplication.AdoptNetworkIntakeResult, error) {
	double.steps.add("intake")
	double.commands = append(double.commands, command)
	return psapplication.AdoptNetworkIntakeResult{}, double.err
}

type finalsDouble struct {
	steps    *steps
	commands []psapplication.FormParcelFinalCommand
	err      error
}

func (double *finalsDouble) Handle(_ context.Context, command psapplication.FormParcelFinalCommand) (psapplication.FormParcelFinalResult, error) {
	double.steps.add("final")
	double.commands = append(double.commands, command)
	return psapplication.FormParcelFinalResult{}, double.err
}

type labelDouble struct {
	steps   *steps
	pickups []psdomain.CarrierFirstEffectivePickupSpec
	err     error
}

func (double *labelDouble) JudgeParcel(_ context.Context, _ string, _ string, pickup psdomain.CarrierFirstEffectivePickupSpec) error {
	double.steps.add("label")
	double.pickups = append(double.pickups, pickup)
	return double.err
}

type rig struct {
	steps    *steps
	lock     *lockDouble
	targets  *targetsDouble
	replays  *replaysDouble
	pickups  *pickupsDouble
	intake   *intakeDouble
	finals   *finalsDouble
	label    *labelDouble
	redriver *cancellationredrive.Redriver
}

func must[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func newRig(t *testing.T) *rig {
	t.Helper()
	recorded := &steps{}
	identity, err := psdomain.NewSourceIdentity(
		must(t, psdomain.NewTenantID, "tenant-1"), must(t, psdomain.NewCustomerAccountID, "customer-1"),
		must(t, psdomain.NewSource, "source-a"), must(t, psdomain.NewSourceRequestKey, "key-1"))
	if err != nil {
		t.Fatalf("source identity: %v", err)
	}
	target, err := psdomain.NewCurrentAcceptedParcelTarget(identity,
		must(t, psdomain.NewShipmentRequestID, "request-1"), must(t, psdomain.NewSubmissionVersionID, "version-2"))
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	parcel := must(t, psdomain.NewDeclaredParcelID, "parcel-1")
	intakeSpec := psdomain.IntakeSourceSpec{
		Kind: psdomain.NodeIntakeSource, Object: must(t, psdomain.NewSourceObjectReference, "HU-1"), Parcel: parcel,
		Place: must(t, psdomain.NewIntakePlaceReference, "NODE-1"), Control: must(t, psdomain.NewIntakeControlReference, "NODE-INTAKE/E-1"),
		Version: must(t, psdomain.NewSourceResultVersion, "v1"), OccurredAt: time.Date(2026, 10, 4, 7, 0, 0, 0, time.UTC),
	}
	finalSpec := psdomain.ResponsibilityOutcomeSpec{
		Kind: psdomain.EffectiveDeliveryOutcome, Parcel: parcel,
		Decision:  must(t, psdomain.NewResponsibilityDecisionReference, "DELIVERY-JUDGMENT/parcel-1/ATT-1"),
		Execution: must(t, psdomain.NewExecutionEvidenceReference, "POD-1"),
		Version:   must(t, psdomain.NewResponsibilityOutcomeVersion, "DLV-v1"), OccurredAt: time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC),
	}
	cancellation := must(t, psdomain.NewParcelCancellationID, "PCXL-1")
	built := &rig{
		steps:   recorded,
		lock:    &lockDouble{steps: recorded},
		targets: &targetsDouble{steps: recorded, target: target, found: true},
		replays: &replaysDouble{steps: recorded, replays: []psports.CancellationBlockedReplay{
			{Cancellation: cancellation, Intake: &intakeSpec},
			{Cancellation: cancellation, Final: &finalSpec},
		}},
		pickups: &pickupsDouble{steps: recorded},
		intake:  &intakeDouble{steps: recorded},
		finals:  &finalsDouble{steps: recorded},
		label:   &labelDouble{steps: recorded},
	}
	built.redriver, err = cancellationredrive.NewRedriver(built.replays, built.targets, built.pickups, built.intake, built.finals, built.label, built.lock)
	if err != nil {
		t.Fatalf("NewRedriver: %v", err)
	}
	return built
}

func notUpheld(kind string) psinbox.NotUpheldCancellation {
	return psinbox.NotUpheldCancellation{
		TenantID: "tenant-1", Parcel: "parcel-1", Cancellation: "PCXL-1", BoundaryKind: kind, BoundaryRef: "NODE_INTAKE/v1",
	}
}

// Covers: AT-PS-173；ADR-0295 决定四——先取锁，再读目标与原料；收寄原料交收寄采用（目标取当前已接受委托与当前提交版本，来源规格原样），终局原料交终局采用；
// 收寄与终局两处的结果都经各自消费门的译表（替身的零值结果被译表拒绝，错误原样上抛）；网络服务的冲突不读 TF 链尾也不判面单终局。
func TestTheRedriveLocksFirstAndHandsEachReplayBackToItsOwnChain(t *testing.T) {
	built := newRig(t)

	err := built.redriver.HandleNotUpheldCancellation(t.Context(), notUpheld("NETWORK_INTAKE"))
	if !errors.Is(err, adoptconsume.ErrUnexpectedAdoptionOutcome) {
		t.Fatalf("err = %v, want 收寄结果过了 adoptconsume 的译表（零值结果被拒）", err)
	}
	if got, want := strings.Join(built.steps.log, ","), "lock,target,replays,intake"; got != want {
		t.Fatalf("步骤 = %s, want %s——收寄原料先交，译表拒了就整笔回滚，不往下走", got, want)
	}
	command := built.intake.commands[0]
	if command.SubmissionVersion.String() != "version-2" || command.ShipmentRequestID.String() != "request-1" ||
		command.Identity.TenantID().String() != "tenant-1" || command.Source.Version.String() != "v1" {
		t.Fatalf("收寄命令 = %+v, want 当前已接受委托作目标、来源规格原样", command)
	}
	if built.replays.asked[0] != "tenant-1/parcel-1/PCXL-1" {
		t.Fatalf("取原料的范围 = %v", built.replays.asked)
	}
}

// Covers: AT-PS-173；终局原料交终局采用，结果过 finalconsume 的译表；仅有终局原料时不碰收寄采用。
func TestAFinalReplayIsHandedBackToTheFinalAdoption(t *testing.T) {
	built := newRig(t)
	built.replays.replays = built.replays.replays[1:]

	err := built.redriver.HandleNotUpheldCancellation(t.Context(), notUpheld("NETWORK_INTAKE"))
	if !errors.Is(err, finalconsume.ErrUnexpectedFinalOutcome) {
		t.Fatalf("err = %v, want 终局结果过了 finalconsume 的译表", err)
	}
	if got, want := strings.Join(built.steps.log, ","), "lock,target,replays,final"; got != want {
		t.Fatalf("步骤 = %s, want %s", got, want)
	}
	if command := built.finals.commands[0]; command.Outcome.Version.String() != "DLV-v1" || command.ShipmentRequestID.String() != "request-1" {
		t.Fatalf("终局命令 = %+v, want 责任结果规格原样、目标取当前已接受委托", command)
	}
}

// Covers: AT-PS-172；冲突落在面单渠道服务的首次有效收寄上：读 TF 链尾，已形成才交面单终局判断（带链尾那一代的规格）；没有或待确认不判；网络服务的冲突不读链尾。
func TestACarrierPickupConflictRejudgesTheCurrentPickupOnlyWhenFormed(t *testing.T) {
	spec := psdomain.CarrierFirstEffectivePickupSpec{
		Fact:        must(t, psdomain.NewCarrierFirstEffectivePickupFactReference, "TF-TRACK-FACT-7"),
		Version:     must(t, psdomain.NewCarrierFirstEffectivePickupFactVersion, "TF-TRACK-FACT-7/v2"),
		EffectiveAt: time.Date(2026, 10, 4, 7, 0, 0, 0, time.UTC),
	}
	for name, test := range map[string]struct {
		state     psports.CarrierPickupBoundaryState
		wantSteps string
	}{
		"已形成": {psports.CarrierPickupBoundaryFormed, "lock,target,replays,pickup,label"},
		"没有":  {psports.CarrierPickupBoundaryAbsent, "lock,target,replays,pickup"},
		"待确认": {psports.CarrierPickupBoundaryPending, "lock,target,replays,pickup"},
	} {
		t.Run(name, func(t *testing.T) {
			built := newRig(t)
			built.replays.replays = nil
			built.pickups.boundary = psports.CarrierPickupBoundary{State: test.state, Pickup: spec}

			if err := built.redriver.HandleNotUpheldCancellation(t.Context(), notUpheld("CARRIER_FIRST_EFFECTIVE_PICKUP")); err != nil {
				t.Fatalf("err = %v", err)
			}
			if got := strings.Join(built.steps.log, ","); got != test.wantSteps {
				t.Fatalf("步骤 = %s, want %s", got, test.wantSteps)
			}
			if test.state == psports.CarrierPickupBoundaryFormed && (len(built.label.pickups) != 1 || built.label.pickups[0] != spec) {
				t.Fatalf("交面单终局判断的规格 = %+v, want 链尾那一代", built.label.pickups)
			}
		})
	}

	built := newRig(t)
	built.replays.replays = nil
	if err := built.redriver.HandleNotUpheldCancellation(t.Context(), notUpheld("NETWORK_INTAKE")); err != nil {
		t.Fatalf("err = %v", err)
	}
	if contains(built.steps.log, "pickup") || contains(built.steps.log, "label") {
		t.Fatalf("网络服务的冲突读了 TF 链尾或判了面单终局：%v", built.steps.log)
	}
}

// Covers: AT-PS-173；失败都响亮并整笔回滚：取不到锁什么也不读；目标还不可见是续办（回滚重投）；信封里译不出的维、原料被改坏、读口出错各自上抛；
// 面单终局判断的错误原样上抛。
func TestTheRedriveFailsLoudlyAndNothingRunsBeforeTheLock(t *testing.T) {
	boom := errors.New("boom")
	t.Run("取不到锁", func(t *testing.T) {
		built := newRig(t)
		built.lock.err = boom
		if err := built.redriver.HandleNotUpheldCancellation(t.Context(), notUpheld("NETWORK_INTAKE")); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
		if got := strings.Join(built.steps.log, ","); got != "lock" {
			t.Fatalf("步骤 = %s, want 只有取锁", got)
		}
	})
	t.Run("目标还不可见", func(t *testing.T) {
		built := newRig(t)
		built.targets.found = false
		if err := built.redriver.HandleNotUpheldCancellation(t.Context(), notUpheld("NETWORK_INTAKE")); !errors.Is(err, cancellationredrive.ErrRedriveTargetNotFound) {
			t.Fatalf("err = %v, want 目标未找到（续办）", err)
		}
	})
	for name, notice := range map[string]psinbox.NotUpheldCancellation{
		"租户译不出":  {TenantID: " ", Parcel: "parcel-1", Cancellation: "PCXL-1", BoundaryKind: "NETWORK_INTAKE", BoundaryRef: "r"},
		"包裹译不出":  {TenantID: "tenant-1", Parcel: " ", Cancellation: "PCXL-1", BoundaryKind: "NETWORK_INTAKE", BoundaryRef: "r"},
		"取消译不出":  {TenantID: "tenant-1", Parcel: "parcel-1", Cancellation: " ", BoundaryKind: "NETWORK_INTAKE", BoundaryRef: "r"},
		"边界种类不认": {TenantID: "tenant-1", Parcel: "parcel-1", Cancellation: "PCXL-1", BoundaryKind: "SCAN", BoundaryRef: "r"},
	} {
		t.Run(name, func(t *testing.T) {
			built := newRig(t)
			if err := built.redriver.HandleNotUpheldCancellation(t.Context(), notice); !errors.Is(err, cancellationredrive.ErrRedriveUntranslatable) {
				t.Fatalf("err = %v, want 译不出", err)
			}
			if len(built.steps.log) != 0 {
				t.Fatalf("译不出仍在跑：%v", built.steps.log)
			}
		})
	}
	t.Run("原料两件都缺", func(t *testing.T) {
		built := newRig(t)
		built.replays.replays = []psports.CancellationBlockedReplay{{}}
		if err := built.redriver.HandleNotUpheldCancellation(t.Context(), notUpheld("NETWORK_INTAKE")); !errors.Is(err, cancellationredrive.ErrRedriveUntranslatable) {
			t.Fatalf("err = %v", err)
		}
	})
	for name, arrange := range map[string]func(*rig){
		"读目标":  func(built *rig) { built.targets.err = boom },
		"读原料":  func(built *rig) { built.replays.err = boom },
		"收寄采用": func(built *rig) { built.intake.err = boom },
		"终局采用": func(built *rig) { built.replays.replays = built.replays.replays[1:]; built.finals.err = boom },
		"读链尾": func(built *rig) {
			built.replays.replays = nil
			built.pickups.err = boom
		},
		"面单终局判断": func(built *rig) {
			built.replays.replays = nil
			built.pickups.boundary.State = psports.CarrierPickupBoundaryFormed
			built.label.err = boom
		},
	} {
		t.Run(name, func(t *testing.T) {
			built := newRig(t)
			arrange(built)
			kind := "NETWORK_INTAKE"
			if name == "读链尾" || name == "面单终局判断" {
				kind = "CARRIER_FIRST_EFFECTIVE_PICKUP"
			}
			if err := built.redriver.HandleNotUpheldCancellation(t.Context(), notUpheld(kind)); !errors.Is(err, boom) {
				t.Fatalf("err = %v, want 原样上抛", err)
			}
		})
	}
}

func TestTheRedriverRefusesToBeBuiltWithAMissingPart(t *testing.T) {
	built := newRig(t)
	if _, err := cancellationredrive.NewRedriver(nil, built.targets, built.pickups, built.intake, built.finals, built.label, built.lock); err == nil {
		t.Error("缺原料读口")
	}
	if _, err := cancellationredrive.NewRedriver(built.replays, nil, built.pickups, built.intake, built.finals, built.label, built.lock); err == nil {
		t.Error("缺目标视图")
	}
	if _, err := cancellationredrive.NewRedriver(built.replays, built.targets, nil, built.intake, built.finals, built.label, built.lock); err == nil {
		t.Error("缺链尾视图")
	}
	if _, err := cancellationredrive.NewRedriver(built.replays, built.targets, built.pickups, nil, built.finals, built.label, built.lock); err == nil {
		t.Error("缺收寄采用")
	}
	if _, err := cancellationredrive.NewRedriver(built.replays, built.targets, built.pickups, built.intake, nil, built.label, built.lock); err == nil {
		t.Error("缺终局采用")
	}
	if _, err := cancellationredrive.NewRedriver(built.replays, built.targets, built.pickups, built.intake, built.finals, nil, built.lock); err == nil {
		t.Error("缺面单终局判断")
	}
	if _, err := cancellationredrive.NewRedriver(built.replays, built.targets, built.pickups, built.intake, built.finals, built.label, nil); err == nil {
		t.Error("缺锁")
	}
}

func contains(log []string, step string) bool {
	for _, got := range log {
		if got == step {
			return true
		}
	}
	return false
}
