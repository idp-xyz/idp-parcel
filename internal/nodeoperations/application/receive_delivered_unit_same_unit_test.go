package application_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证同一作业实物在同一节点不另起控制起点（ADR-0199 决定四，票 no-reception-source/04）：已有当前收寄时三种观察都并入，
// 仅扫描的待确认由后来的观察续办，以及两者都不碰的那几格（别的节点、未收寄之后）。收寄库替身沿用收寄编排那一只，在这里补上
// 按实物与节点找各链当前有效版本的一问。

// FindCurrentJudgmentsAtNode 照真库的筛法在内存里找：同租户、有判断版本、不是并入行、没有后继；形成与待识别按收寄上的实物与
// 节点认，其余按来源文档认；按来源标识定序。
func (double *receptionStoreDouble) FindCurrentJudgmentsAtNode(
	_ context.Context,
	tenant domain.TenantID,
	unit domain.HandlingUnitID,
	node domain.NodeReference,
) ([]ports.ReceptionRecord, error) {
	double.lookups = append(double.lookups, unit.String()+"@"+node.String())
	if double.currentErr != nil {
		return nil, double.currentErr
	}
	found := []ports.ReceptionRecord{}
	for key, record := range double.byKey {
		if key.TenantID != tenant || record.Kind == ports.RecordMergedIntoExistingIntake || record.JudgmentVersion.String() == "" {
			continue
		}
		recordUnit, recordNode := record.Source.Unit, record.Source.Node
		if !record.Intake.ReceivedAt().IsZero() {
			recordUnit, recordNode = record.Intake.Unit(), record.Intake.Node()
		}
		if recordUnit != unit || recordNode != node {
			continue
		}
		if _, succeeded := double.successorOf(tenant, record.JudgmentVersion); succeeded {
			continue
		}
		found = append(found, record)
	}
	slices.SortFunc(found, func(left, right ports.ReceptionRecord) int {
		return strings.Compare(left.Key.SourceID, right.Key.SourceID)
	})
	return found, nil
}

// controlStartsOf 数这件实物在册上的控制起点：带控制的各行里控制依据不同的有几个。追加的冲突版与重核版照原判断的控制，
// 不是新起点。
func controlStartsOf(fixture *receptionFixture, unit string) int {
	bases := map[string]bool{}
	for _, record := range fixture.receptions.recordsOfUnit(unit) {
		if record.Control.Active() {
			bases[record.Control.Basis().String()] = true
		}
	}
	return len(bases)
}

func sameUnitDelivery(t *testing.T) application.ReceiveDeliveredUnitCommand {
	t.Helper()
	return application.ReceiveDeliveredUnitCommand{
		TenantID:    mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		SourceID:    "SYN-DELIVERY-NRS04-1",
		Node:        mustValue(t, domain.NewNodeReference, "SYN-NODE-NRS04"),
		DeliveredBy: mustValue(t, domain.NewDeliveringPartyReference, "SYN-CUSTOMER-NRS04"),
		Unit:        mustValue(t, domain.NewHandlingUnitID, "SYN-UNIT-NRS04"),
		Mark:        ports.ExternalMarkObservation{Mark: "SYN-BARCODE-NRS04"},
		Claim:       application.ExplicitReception,
		Evidence:    mustValue(t, domain.NewReceptionEvidenceReference, "SYN-SIGN-NRS04-1"),
		OccurredAt:  deliveredAt,
	}
}

// deliveredAgain 是同一件实物的又一次送站：来源标识与发生时刻换了，接收观察取 claim，证据与原因随观察取或空。
func deliveredAgain(
	t *testing.T,
	earlier application.ReceiveDeliveredUnitCommand,
	sourceID string,
	claim application.ReceptionClaimKind,
) application.ReceiveDeliveredUnitCommand {
	t.Helper()
	command := earlier
	command.SourceID = sourceID
	command.OccurredAt = earlier.OccurredAt.Add(time.Hour)
	command.Claim = claim
	command.Evidence, command.Refusal = domain.ReceptionEvidenceReference{}, ""
	switch claim {
	case application.ExplicitReception:
		command.Evidence = mustValue(t, domain.NewReceptionEvidenceReference, "SYN-SIGN-"+sourceID)
	case application.ExplicitRefusal:
		command.Refusal = "SYN-REFUSED-NRS04"
	}
	return command
}

func keyOf(command application.ReceiveDeliveredUnitCommand) ports.ReceptionKey {
	return ports.ReceptionKey{TenantID: command.TenantID, SourceID: command.SourceID}
}

func parcelsOf(t *testing.T, raws ...string) []domain.ParcelAssociationReference {
	t.Helper()
	parcels := []domain.ParcelAssociationReference{}
	for _, raw := range raws {
		parcels = append(parcels, mustValue(t, domain.NewParcelAssociationReference, raw))
	}
	return parcels
}

var everyClaim = map[string]application.ReceptionClaimKind{
	"明确接收": application.ExplicitReception,
	"明确拒收": application.ExplicitRefusal,
	"仅扫描":  application.ScanOnlyObservation,
}

// Covers: `AT-NO-049`「同一作业实物换来源，再次交到已有当前收寄的同一节点——并入那一条收寄并返回它的结果，这次的来源照样保全；
// 不另起控制起点、不重复发布」的编排面（ADR-0199 决定四）：当前收寄是形成格与待识别格各一种，换来源再来明确接收、明确拒收、
// 仅扫描，三种观察都落一行「并入已有收寄」——来源文档照存、指名并入的那一版、不带收寄与控制、不签判断版本、不指名前版；不再问
// PS、不交意图；答已有结果，带回那一条收寄，原判断种类与当前有效版本是那一版的。册上只有一个控制起点。重投并入的那一来源同答，
// 不再落行。
func TestTheSameUnitAtTheSameNodeIsMergedIntoItsCurrentIntake(t *testing.T) {
	currents := map[string]struct {
		parcels []string
		want    application.ReceptionOutcome
	}{
		"收寄已形成": {parcels: []string{"SYN-PARCEL-NRS04"}, want: application.NodeIntakeFormed},
		"待识别":   {want: application.UnitPendingIdentification},
	}
	for currentName, current := range currents {
		for claimName, claim := range everyClaim {
			t.Run(currentName+"·"+claimName, func(t *testing.T) {
				fixture := newReceptionFixture(t)
				fixture.identity.candidates = parcelsOf(t, current.parcels...)
				first := sameUnitDelivery(t)
				formed, err := fixture.handler.Handle(context.Background(), first)
				if err != nil || formed.Outcome() != current.want {
					t.Fatalf("首投 outcome = %q err = %v, want %q", formed.Outcome(), err, current.want)
				}
				existing, _ := formed.Record()
				// 并入在身份核对之前认出：再问 PS 就答不出。
				fixture.identity.err = errors.New("并入不该再问 PS")
				saved, signed := fixture.receptions.saved, fixture.versions.next
				intents, asked := len(fixture.downstream.intents), len(fixture.serviceResults.asked)

				again := deliveredAgain(t, first, "SYN-DELIVERY-NRS04-2", claim)
				for _, attempt := range []string{"并入", "重投并入的那一来源"} {
					result, err := fixture.handler.Handle(context.Background(), again)
					if err != nil || result.Outcome() != application.ReceptionExistingResult {
						t.Fatalf("%s outcome = %q err = %v, want EXISTING_RESULT", attempt, result.Outcome(), err)
					}
					if result.RecordedOutcome() != current.want || result.CurrentVersion() != existing.JudgmentVersion ||
						result.MergedInto() != existing.JudgmentVersion {
						t.Fatalf("%s 原判断种类 = %q 当前有效版本 = %q 并入版本 = %q, want %q 与 %q", attempt,
							result.RecordedOutcome(), result.CurrentVersion(), result.MergedInto(), current.want, existing.JudgmentVersion)
					}
					if answered, _ := result.Record(); answered.Key != existing.Key {
						t.Fatalf("%s 答的是 %q, want 并入的那一条 %q", attempt, answered.Key.SourceID, existing.Key.SourceID)
					}
					if fixture.receptions.saved != saved+1 || fixture.versions.next != signed {
						t.Fatalf("%s 之后落册 %d 行、签版本 %d 个, want 只多一行并入、不签版本", attempt,
							fixture.receptions.saved-saved, fixture.versions.next-signed)
					}
					if len(fixture.downstream.intents) != intents || len(fixture.serviceResults.asked) != asked {
						t.Fatalf("%s 之后意图 %d → %d、问服务结果 %d → %d, want 都不变", attempt,
							intents, len(fixture.downstream.intents), asked, len(fixture.serviceResults.asked))
					}
				}

				merged := fixture.receptions.byKey[keyOf(again)]
				if merged.Kind != ports.RecordMergedIntoExistingIntake || merged.MergedInto != existing.JudgmentVersion {
					t.Fatalf("并入行 种类 = %s 并入版本 = %q, want MERGED_INTO_EXISTING_INTAKE %q", merged.Kind, merged.MergedInto, existing.JudgmentVersion)
				}
				if want := sourceOf(again, claim.String()); merged.Source != want {
					t.Fatalf("并入行的来源文档 = %+v, want %+v", merged.Source, want)
				}
				if merged.JudgmentVersion.String() != "" || merged.Predecessor.String() != "" ||
					!merged.Intake.ReceivedAt().IsZero() || merged.Control.Active() {
					t.Fatalf("并入行带了判断版本 %q、前版 %q、收寄或控制：%+v", merged.JudgmentVersion, merged.Predecessor, merged)
				}
				if starts := controlStartsOf(fixture, "SYN-UNIT-NRS04"); starts != 1 {
					t.Fatalf("册上 %d 个控制起点, want 1", starts)
				}
			})
		}
	}
}

// Covers: ADR-0199 决定四「当前收寄」与续办的边界——同一件实物到了另一个节点照常形成收寄、另起那个节点的控制起点，不指名
// 前版；`节点收寄未形成`之后的再次交付是另一次交付尝试，照常形成判断，不续办那次拒收、不指名前版。
func TestTheSameUnitElsewhereOrAfterARefusalFormsAFreshJudgment(t *testing.T) {
	t.Run("另一个节点", func(t *testing.T) {
		fixture := newReceptionFixture(t)
		fixture.identity.candidates = parcelsOf(t, "SYN-PARCEL-NRS04")
		first := sameUnitDelivery(t)
		if _, err := fixture.handler.Handle(context.Background(), first); err != nil {
			t.Fatalf("首投：%v", err)
		}
		elsewhere := deliveredAgain(t, first, "SYN-DELIVERY-NRS04-ELSEWHERE", application.ExplicitReception)
		elsewhere.Node = mustValue(t, domain.NewNodeReference, "SYN-NODE-NRS04-OTHER")
		result, err := fixture.handler.Handle(context.Background(), elsewhere)
		if err != nil || result.Outcome() != application.NodeIntakeFormed {
			t.Fatalf("另一个节点 outcome = %q err = %v, want INTAKE_FORMED", result.Outcome(), err)
		}
		record, _ := result.Record()
		if record.Predecessor.String() != "" || result.MergedInto().String() != "" || record.Intake.Node() != elsewhere.Node {
			t.Fatalf("另一个节点的收寄 前版 = %q 并入 = %q 节点 = %q, want 首判、在那个节点", record.Predecessor, result.MergedInto(), record.Intake.Node())
		}
		if starts := controlStartsOf(fixture, "SYN-UNIT-NRS04"); starts != 2 || len(fixture.downstream.intents) != 2 {
			t.Fatalf("控制起点 %d、意图 %d 份, want 两个节点各一", starts, len(fixture.downstream.intents))
		}
	})

	wants := map[application.ReceptionClaimKind]application.ReceptionOutcome{
		application.ExplicitReception:   application.NodeIntakeFormed,
		application.ExplicitRefusal:     application.NodeIntakeNotFormed,
		application.ScanOnlyObservation: application.ReceptionUndecided,
	}
	for claimName, claim := range everyClaim {
		t.Run("拒收之后·"+claimName, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			fixture.identity.candidates = parcelsOf(t, "SYN-PARCEL-NRS04")
			refused := deliveredAgain(t, sameUnitDelivery(t), "SYN-DELIVERY-NRS04-REFUSED", application.ExplicitRefusal)
			if result, err := fixture.handler.Handle(context.Background(), refused); err != nil || result.Outcome() != application.NodeIntakeNotFormed {
				t.Fatalf("拒收 outcome = %q err = %v", result.Outcome(), err)
			}
			later := deliveredAgain(t, refused, "SYN-DELIVERY-NRS04-LATER", claim)
			result, err := fixture.handler.Handle(context.Background(), later)
			if err != nil || result.Outcome() != wants[claim] {
				t.Fatalf("拒收之后 outcome = %q err = %v, want %q", result.Outcome(), err, wants[claim])
			}
			if record, _ := result.Record(); record.Predecessor.String() != "" {
				t.Fatalf("拒收之后的判断指名了前版 %q, want 不续办那次拒收", record.Predecessor)
			}
		})
	}
}

// Covers: `AT-NO-050`「只有到站扫描的待确认之后，同一实物来了明确接收或拒收——追加一版判断续办原待确认，原待确认照留；不回头把
// 它改成确定结果」的编排面（ADR-0199 决定四）：仅扫描落下待确认之后，同一实物在同一节点换来源来明确接收（唯一识别形成收寄、
// 零候选落待识别，都建控制）、明确拒收（未收寄）或又一次仅扫描（又一版待确认），新判断都以那条待确认为前版；待确认原行逐字
// 不变。重投待确认的来源答已有结果，原判断种类是 RECEPTION_UNDECIDED，当前有效版本是新判断的版本。
func TestAScanOnlyJudgmentIsContinuedByTheLaterObservation(t *testing.T) {
	cases := map[string]struct {
		claim   application.ReceptionClaimKind
		parcels []string
		want    application.ReceptionOutcome
	}{
		"明确接收·唯一识别": {claim: application.ExplicitReception, parcels: []string{"SYN-PARCEL-NRS04"}, want: application.NodeIntakeFormed},
		"明确接收·零候选":  {claim: application.ExplicitReception, want: application.UnitPendingIdentification},
		"明确拒收":      {claim: application.ExplicitRefusal, want: application.NodeIntakeNotFormed},
		"又一次仅扫描":    {claim: application.ScanOnlyObservation, want: application.ReceptionUndecided},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			fixture.identity.candidates = parcelsOf(t, testCase.parcels...)
			scan := sameUnitDelivery(t)
			scan.Claim, scan.Evidence = application.ScanOnlyObservation, domain.ReceptionEvidenceReference{}
			undecided, err := fixture.handler.Handle(context.Background(), scan)
			if err != nil || undecided.Outcome() != application.ReceptionUndecided {
				t.Fatalf("仅扫描 outcome = %q err = %v, want RECEPTION_UNDECIDED", undecided.Outcome(), err)
			}
			before := fixture.receptions.byKey[keyOf(scan)]

			later := deliveredAgain(t, scan, "SYN-DELIVERY-NRS04-LATER", testCase.claim)
			result, err := fixture.handler.Handle(context.Background(), later)
			if err != nil || result.Outcome() != testCase.want {
				t.Fatalf("后来的观察 outcome = %q err = %v, want %q", result.Outcome(), err, testCase.want)
			}
			continued, _ := result.Record()
			if continued.Predecessor != before.JudgmentVersion || continued.JudgmentVersion.String() == "" ||
				continued.JudgmentVersion == before.JudgmentVersion {
				t.Fatalf("新判断 版本 = %q 前版 = %q, want 自己签一版、以待确认 %q 为前版", continued.JudgmentVersion, continued.Predecessor, before.JudgmentVersion)
			}
			if continued.Source != sourceOf(later, testCase.claim.String()) {
				t.Fatalf("新判断的来源文档 = %+v, want 后来那次送站的", continued.Source)
			}
			establishes := testCase.claim == application.ExplicitReception
			if continued.Control.Active() != establishes || (controlStartsOf(fixture, "SYN-UNIT-NRS04") == 1) != establishes {
				t.Fatalf("控制成立 = %v、控制起点 %d, want 只有明确接收建控制、一个起点", continued.Control.Active(), controlStartsOf(fixture, "SYN-UNIT-NRS04"))
			}
			if wantIntents := map[bool]int{true: 1, false: 0}[testCase.want == application.NodeIntakeFormed]; len(fixture.downstream.intents) != wantIntents {
				t.Fatalf("意图 %d 份, want %d", len(fixture.downstream.intents), wantIntents)
			}
			if kept := fixture.receptions.byKey[keyOf(scan)]; !reflect.DeepEqual(kept, before) {
				t.Fatalf("待确认原行被改了：%+v, want %+v", kept, before)
			}

			replay, err := fixture.handler.Handle(context.Background(), scan)
			if err != nil || replay.Outcome() != application.ReceptionExistingResult {
				t.Fatalf("重投待确认 outcome = %q err = %v, want EXISTING_RESULT", replay.Outcome(), err)
			}
			if replay.RecordedOutcome() != application.ReceptionUndecided || replay.CurrentVersion() != continued.JudgmentVersion {
				t.Fatalf("重投待确认 原判断种类 = %q 当前有效版本 = %q, want RECEPTION_UNDECIDED %q",
					replay.RecordedOutcome(), replay.CurrentVersion(), continued.JudgmentVersion)
			}
			if replay.MergedInto().String() != "" {
				t.Fatalf("重投待确认带了并入版本 %q", replay.MergedInto())
			}
		})
	}
}

// Covers: ADR-0199 决定四「串行：认同一实物之前，按（租户，作业实物）取事务级咨询锁，所有分支都取」的编排面——三种观察首投都先
// 按实物与节点问一次各链的当前有效版本（真库在这一问里取实物锁）；同一来源的重投在这一问之前已按幂等键认出，不再问。这一问读
// 不动就不形成答案：不落册、不交意图（ADR-0186）。
func TestEveryObservationAsksForTheUnitsCurrentJudgmentsFirst(t *testing.T) {
	for claimName, claim := range everyClaim {
		t.Run(claimName, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			command := deliveredAgain(t, sameUnitDelivery(t), "SYN-DELIVERY-NRS04-1", claim)
			if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
				t.Fatalf("首投：%v", err)
			}
			if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
				t.Fatalf("重投：%v", err)
			}
			if want := []string{"SYN-UNIT-NRS04@SYN-NODE-NRS04"}; !slices.Equal(fixture.receptions.lookups, want) {
				t.Fatalf("按实物与节点问了 %v, want 首投问一次 %v", fixture.receptions.lookups, want)
			}
		})
		t.Run(claimName+"·读不动", func(t *testing.T) {
			fixture := newReceptionFixture(t)
			fixture.receptions.currentErr = errors.New("reception register unreadable")
			command := deliveredAgain(t, sameUnitDelivery(t), "SYN-DELIVERY-NRS04-1", claim)
			result, err := fixture.handler.Handle(context.Background(), command)
			if err == nil || errors.Is(err, application.ErrReceptionCommandRejected) {
				t.Fatalf("outcome = %q err = %v, want 依赖故障类 error", result.Outcome(), err)
			}
			if fixture.receptions.saved != 0 || len(fixture.downstream.intents) != 0 {
				t.Fatalf("落册 %d、意图 %d, want 都没有", fixture.receptions.saved, len(fixture.downstream.intents))
			}
		})
	}
}
