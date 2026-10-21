package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证委托协议换版这一种运营决定（ADR-0231 决定三）：先取换版锁再读委托，同一份依据分已有与冲突，声明那一半各格、后继在起用时刻
// 的四个协议否定格各答一格，成立时落下一版并同笔交门铃。夹具全是合成串，证据只记 S。

// revisionStoreDouble 照真册：锁记一次调用次序，落行按（号、依据）唯一。
type revisionStoreDouble struct {
	steps   *[]string
	saved   []ports.CommissionAgreementRevisionRecord
	holdErr error
	saveErr error
}

func (double *revisionStoreDouble) HoldRevisions(context.Context, ports.TransportCommissionKey) error {
	*double.steps = append(*double.steps, "hold")
	return double.holdErr
}

func (double *revisionStoreDouble) SaveRevision(_ context.Context, record ports.CommissionAgreementRevisionRecord) (ports.CommissionAgreementRevisionSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.CommissionAgreementRevisionSaveOutcomeInvalid, double.saveErr
	}
	for _, saved := range double.saved {
		if saved.Revision.Number() == record.Revision.Number() || saved.Revision.Basis() == record.Revision.Basis() {
			return ports.CommissionAgreementRevisionAlreadyRegistered, nil
		}
	}
	double.saved = append(double.saved, record)
	return ports.CommissionAgreementRevisionSaved, nil
}

type orderedCommissionView struct {
	steps  *[]string
	record ports.TransportCommissionRecord
	found  bool
	err    error
}

func (view orderedCommissionView) FindByKey(context.Context, ports.TransportCommissionKey) (ports.TransportCommissionRecord, bool, error) {
	*view.steps = append(*view.steps, "read")
	return view.record, view.found, view.err
}

type revisionDoorbellDouble struct {
	intents []ports.CommissionAgreementRevisedIntent
	err     error
}

func (double *revisionDoorbellDouble) HandOffCommissionAgreementRevision(_ context.Context, intent ports.CommissionAgreementRevisedIntent) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type revisionRig struct {
	steps      []string
	commission orderedCommissionView
	revisions  *revisionStoreDouble
	agreements *chargeAgreementSourceDouble
	doorbell   *revisionDoorbellDouble
}

func newRevisionRig(t *testing.T) *revisionRig {
	t.Helper()
	rig := &revisionRig{agreements: chargeRegisteredAgreement(t), doorbell: &revisionDoorbellDouble{}}
	rig.agreements.check.Snapshot = occurrenceValue(t, domain.NewAgreementSnapshotReference, revisedSnapshot)
	rig.agreements.check.LegalEntity = occurrenceValue(t, domain.NewProcurementLegalEntityReference, revisedLegal)
	rig.commission = orderedCommissionView{steps: &rig.steps, record: trunkCommission(t, commissionShape{startedAt: occurrenceEnteredAt}), found: true}
	rig.revisions = &revisionStoreDouble{steps: &rig.steps}
	return rig
}

func (rig *revisionRig) revise(t *testing.T, command application.ReviseCommissionAgreementCommand) application.CommissionAgreementRevisionResult {
	t.Helper()
	result, err := application.NewReviseCommissionAgreementHandler(application.ReviseCommissionAgreementDeps{
		Commissions: rig.commission, Revisions: rig.revisions, Agreements: rig.agreements, Doorbell: rig.doorbell, Clock: occurrenceClock{},
	}).Revise(t.Context(), command)
	if err != nil {
		t.Fatalf("换版不该上抛：%v", err)
	}
	return result
}

func revisionCommand(t *testing.T) application.ReviseCommissionAgreementCommand {
	t.Helper()
	return application.ReviseCommissionAgreementCommand{
		TenantID:         occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		DecidedBy:        registrant,
		Commission:       bkCommission,
		SuccessorObject:  trunkAgreement,
		SuccessorVersion: revisedTag,
		AppliesFrom:      bkSubmittedAt.Add(time.Hour),
		Basis:            "SYN-DECISION-REV-TRUNK-01",
	}
}

// Covers: AT-TF-298——换版成立：先取锁、再读委托；在起用时刻按委托的服务方核后继；落下第 2 版（快照与法人取核对结果、决定人取认证、
// 登记时刻取时钟），同笔交一封门铃，旧版不动。同一份依据原样重交答已有、回显在册那一版、不再落行不再交门铃；换了后继或起用时刻答冲突。
func TestARevisionIsCheckedAtItsStartAndRingsTheCatchUpDoorbell(t *testing.T) {
	rig := newRevisionRig(t)
	result := rig.revise(t, revisionCommand(t))
	revision, present := result.Revision()
	if result.Outcome() != application.CommissionAgreementRevised || !present || revision.Number() != 2 ||
		revision.Snapshot().String() != revisedSnapshot || revision.LegalEntity().String() != revisedLegal ||
		revision.DecidedBy().String() != registrant || !revision.RegisteredAt().Equal(occurrenceNow) {
		t.Fatalf("换版 = %s %+v", result.Outcome(), revision)
	}
	if len(rig.steps) < 2 || rig.steps[0] != "hold" || rig.steps[1] != "read" {
		t.Fatalf("次序 = %v, want 先取锁再读委托", rig.steps)
	}
	if len(rig.agreements.asked) != 1 || rig.agreements.asked[0] != trunkAgreement+" "+revisedTag ||
		rig.agreements.provider[0] != trunkProvider || !rig.agreements.askedAt[0].Equal(bkSubmittedAt.Add(time.Hour)) {
		t.Fatalf("核后继 = %v %v %v", rig.agreements.asked, rig.agreements.provider, rig.agreements.askedAt)
	}
	if len(rig.revisions.saved) != 1 || len(rig.doorbell.intents) != 1 || rig.doorbell.intents[0].Record.Revision.Number() != 2 {
		t.Fatalf("落 %d 行、门铃 %d 封", len(rig.revisions.saved), len(rig.doorbell.intents))
	}

	replayed := newRevisionRig(t)
	replayed.commission.record.Commission = applyRevision(t, replayed.commission.record.Commission, revisionCommand(t))
	if again := replayed.revise(t, revisionCommand(t)); again.Outcome() != application.CommissionAgreementRevisionExisting ||
		len(replayed.revisions.saved) != 0 || len(replayed.doorbell.intents) != 0 || len(replayed.agreements.asked) != 0 {
		t.Fatalf("原样重交 = %s，落 %d 行、门铃 %d 封、问 PC %d 次", again.Outcome(), len(replayed.revisions.saved),
			len(replayed.doorbell.intents), len(replayed.agreements.asked))
	}
	moved := revisionCommand(t)
	moved.AppliesFrom = moved.AppliesFrom.Add(time.Minute)
	if conflict := replayed.revise(t, moved); conflict.Outcome() != application.CommissionAgreementRevisionConflict {
		t.Fatalf("同一份依据换了起用时刻 = %s", conflict.Outcome())
	} else if echoed, _ := conflict.Revision(); !echoed.AppliesFrom().Equal(revisionCommand(t).AppliesFrom) {
		t.Fatalf("冲突回显的不是在册那一版：%+v", echoed)
	}
}

func applyRevision(t *testing.T, commission domain.TransportCommission, command application.ReviseCommissionAgreementCommand) domain.TransportCommission {
	t.Helper()
	successor, _ := domain.NewSupplierAgreementVersion(command.SuccessorObject, command.SuccessorVersion)
	draft, err := commission.DraftAgreementRevision(domain.CommissionAgreementRevisionSpec{
		Successor: successor, AppliesFrom: command.AppliesFrom,
		Basis:     occurrenceValue(t, domain.NewCommissionAgreementRevisionBasisReference, command.Basis),
		DecidedBy: occurrenceValue(t, domain.NewRegistrantReference, command.DecidedBy),
	})
	if err != nil {
		t.Fatal(err)
	}
	revised, _, err := draft.Revise(occurrenceValue(t, domain.NewAgreementSnapshotReference, revisedSnapshot),
		occurrenceValue(t, domain.NewProcurementLegalEntityReference, revisedLegal), occurrenceNow)
	if err != nil {
		t.Fatal(err)
	}
	return revised
}

// Covers: AT-TF-297——前置条件各答一格、都不落行不交门铃：委托不在册、后继换了协议对象、起用早于提交、后继在起用时刻不在册 / 未生效 /
// 已终止 / 供应商不是委托的服务方（带回约定的供应商）；声明缺格答未受理；锁、委托、核对口、落行、门铃读写不动各答未决并带续办引用。
func TestARevisionAnswersEachUnmetPreconditionWithoutWriting(t *testing.T) {
	cases := map[string]struct {
		mutate func(*revisionRig, *application.ReviseCommissionAgreementCommand)
		want   application.CommissionAgreementRevisionOutcome
	}{
		"委托不在册": {func(rig *revisionRig, _ *application.ReviseCommissionAgreementCommand) { rig.commission.found = false },
			application.CommissionAgreementRevisionCommissionNotFound},
		"换了协议对象": {func(_ *revisionRig, command *application.ReviseCommissionAgreementCommand) {
			command.SuccessorObject = "SYN-SUPPLIER-OTHER-01"
		}, application.CommissionAgreementRevisionObjectMismatch},
		"起用早于提交": {func(_ *revisionRig, command *application.ReviseCommissionAgreementCommand) {
			command.AppliesFrom = bkSubmittedAt.Add(-time.Second)
		}, application.CommissionAgreementRevisionAppliesBeforeSubmission},
		"后继不在册": {func(rig *revisionRig, _ *application.ReviseCommissionAgreementCommand) {
			rig.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementNotRegistered}
		}, application.CommissionAgreementRevisionAgreementNotRegistered},
		"后继未生效": {func(rig *revisionRig, _ *application.ReviseCommissionAgreementCommand) {
			rig.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementNotEffective}
		}, application.CommissionAgreementRevisionAgreementNotEffective},
		"后继已终止": {func(rig *revisionRig, _ *application.ReviseCommissionAgreementCommand) {
			rig.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementTerminated}
		}, application.CommissionAgreementRevisionAgreementTerminated},
		"供应商不符": {func(rig *revisionRig, _ *application.ReviseCommissionAgreementCommand) {
			rig.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementProviderMismatch,
				Supplier: occurrenceValue(t, domain.NewServiceProviderReference, "SYN-PARTY-OTHER-01")}
		}, application.CommissionAgreementRevisionProviderMismatch},
		"缺依据": {func(_ *revisionRig, command *application.ReviseCommissionAgreementCommand) { command.Basis = " " },
			application.CommissionAgreementRevisionNotAccepted},
		"缺起用时刻": {func(_ *revisionRig, command *application.ReviseCommissionAgreementCommand) {
			command.AppliesFrom = time.Time{}
		},
			application.CommissionAgreementRevisionNotAccepted},
		"缺决定人": {func(_ *revisionRig, command *application.ReviseCommissionAgreementCommand) { command.DecidedBy = "" },
			application.CommissionAgreementRevisionNotAccepted},
		"锁取不到": {func(rig *revisionRig, _ *application.ReviseCommissionAgreementCommand) {
			rig.revisions.holdErr = errors.New("db down")
		},
			application.CommissionAgreementRevisionUndecided},
		"委托读不动": {func(rig *revisionRig, _ *application.ReviseCommissionAgreementCommand) {
			rig.commission.err = errors.New("db down")
		},
			application.CommissionAgreementRevisionUndecided},
		"核对口读不动": {func(rig *revisionRig, _ *application.ReviseCommissionAgreementCommand) {
			rig.agreements.err = errors.New("pc down")
		},
			application.CommissionAgreementRevisionUndecided},
		"落行写不动": {func(rig *revisionRig, _ *application.ReviseCommissionAgreementCommand) {
			rig.revisions.saveErr = errors.New("db down")
		},
			application.CommissionAgreementRevisionUndecided},
		"门铃入不了队": {func(rig *revisionRig, _ *application.ReviseCommissionAgreementCommand) {
			rig.doorbell.err = errors.New("outbox down")
		},
			application.CommissionAgreementRevisionUndecided},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			rig := newRevisionRig(t)
			command := revisionCommand(t)
			testCase.mutate(rig, &command)
			result := rig.revise(t, command)
			if result.Outcome() != testCase.want {
				t.Fatalf("outcome = %s, want %s", result.Outcome(), testCase.want)
			}
			if testCase.want == application.CommissionAgreementRevisionUndecided && (result.UndecidedReason() ==
				application.CommissionAgreementRevisionUndecidedReasonNone || result.ContinuationReference() == "") {
				t.Fatalf("未决没指名停在哪一口：%s %q", result.UndecidedReason(), result.ContinuationReference())
			}
			if testCase.want == application.CommissionAgreementRevisionProviderMismatch && result.AgreedSupplier() != "SYN-PARTY-OTHER-01" {
				t.Fatalf("供应商不符没带回约定的供应商：%q", result.AgreedSupplier())
			}
			if testCase.want != application.CommissionAgreementRevisionUndecided && len(rig.revisions.saved)+len(rig.doorbell.intents) != 0 {
				t.Fatalf("答 %s 却写了：落 %d 行、门铃 %d 封", result.Outcome(), len(rig.revisions.saved), len(rig.doorbell.intents))
			}
		})
	}
}
