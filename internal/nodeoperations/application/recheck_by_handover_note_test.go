package application_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证「交付范围成立」门铃接到的重核（ADR-0212 决定六；票 ps-external-mark-relations/12）：按交接单重核，只看名下当前有效版本
// 属于重核对象的成员，判法是收寄与重核同一份身份判断。交接单册替身同时充作范围证据读口：重核读到的就是命令口刚落的结论与归入。
// 夹具全为合成（S 级）。

// FindRecheckableInHandoverNote 照真库的筛法在内存里找：成员（收寄与归入两种）里的作业实物，在交接单开立的节点上当前有效版本是
// 待识别、不带关联、不记身份冲突或冲突来由是只因歧义信号、来源已保全；按来源标识定序。
func (double *receptionStoreDouble) FindRecheckableInHandoverNote(
	_ context.Context,
	tenant domain.TenantID,
	note domain.DeliveryHandoverNoteID,
) ([]ports.ReceptionRecord, error) {
	found := []ports.ReceptionRecord{}
	if double.handover == nil {
		return found, nil
	}
	opened, registered := double.handover.notes[note.String()]
	if !registered {
		return found, nil
	}
	units := map[domain.HandlingUnitID]bool{}
	for _, member := range double.handover.members {
		if member.Note() == note {
			units[member.Unit()] = true
		}
	}
	for key, record := range double.byKey {
		contested := record.IdentityConflict && record.IdentityConflictCause != ports.ConflictByAmbiguityOnly
		if key.TenantID != tenant || record.Kind != ports.RecordPendingIdentification || contested || !record.Source.Preserved() ||
			record.JudgmentVersion.String() == "" || !units[record.Intake.Unit()] || record.Intake.Node() != opened.Node() {
			continue
		}
		if _, associated := record.Intake.Association(); associated {
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

// 交接单册替身充作范围证据读口的另两问：本文件的用例只用交接单，预报一份也没有，时窗也没有。
func (double *handoverRegisterDouble) FindDeliveryPreAdvice(context.Context, domain.TenantID, domain.DeliveryPreAdviceID) (domain.DeliveryPreAdviceState, bool, error) {
	return domain.DeliveryPreAdviceState{}, false, double.err
}

func (double *handoverRegisterDouble) FindPreAdviceWindowAt(context.Context, domain.TenantID, time.Time) (domain.PreAdviceValidityWindow, bool, error) {
	return domain.PreAdviceValidityWindow{}, false, double.err
}

// IsDeliveryHandoverMember 照真库在内存里认：这张交接单名下有这件实物（收寄带来的或归入的）。
func (double *handoverRegisterDouble) IsDeliveryHandoverMember(_ context.Context, _ domain.TenantID, note domain.DeliveryHandoverNoteID, unit domain.HandlingUnitID) (bool, error) {
	if double == nil {
		return false, nil
	}
	for _, member := range double.members {
		if member.Note() == note && member.Unit() == unit {
			return true, double.err
		}
	}
	return false, double.err
}

// IsDeliveryHandoverMember 让收寄编排那只范围证据替身也答成员一问：读同一夹具的交接单册替身，另认直接写进 assigned 的归入。
func (double *scopeEvidenceDouble) IsDeliveryHandoverMember(ctx context.Context, tenant domain.TenantID, note domain.DeliveryHandoverNoteID, unit domain.HandlingUnitID) (bool, error) {
	if assigned, found := double.assigned[unit.String()]; found && assigned == note {
		return true, double.err
	}
	member, err := double.handover.IsDeliveryHandoverMember(ctx, tenant, note, unit)
	if err != nil {
		return false, err
	}
	return member, double.err
}

var _ ports.DeliveryScopeEvidence = (*handoverRegisterDouble)(nil)

// Covers: `AT-NO-075`（编排半边）、ADR-0212 决定六「重核对象只看成员里属于那一组的」——交接单名下：一件没带依据、经客户包裹件号
// 命中而范围未证的待识别（归入），一件收寄带它、经渠道包裹件号已关联的（收寄成员），一件收寄带它、记多候选冲突的（收寄成员）。
// 最后一个绑定确认之后按交接单重核：只重核那件待识别，追加关联版、业务时间取原收寄时刻、控制不另起、意图一封；已关联与记冲突的
// 两件不追加任何一版。
func TestARecheckByHandoverNoteRechecksOnlyItsRecheckableMembers(t *testing.T) {
	fixture := newHandoverFixture(t)
	ctx := context.Background()
	if _, err := fixture.handover.Open(ctx, openCommand(t, "SYN-HANDOVER-50", handoverAccount)); err != nil {
		t.Fatal(err)
	}
	basis := basisOf(t, domain.BasisDeliveryHandoverNote, "SYN-HANDOVER-50")
	pending := fixture.receivePendingUnit(t, "delivery-50", "unit-50")
	formed := fixture.receiveUnit(t, "delivery-51", "unit-51", basis, nil, nil, "parcel-51")
	contested := fixture.receiveUnit(t, "delivery-52", "unit-52", basis, nil, nil, "parcel-52a", "parcel-52b")
	if formed.Kind != ports.RecordIntakeFormed || contested.IdentityConflictCause != ports.ConflictByMultipleCandidates {
		t.Fatalf("夹具：formed %s，contested %s", formed.Kind, contested.IdentityConflictCause)
	}
	if _, err := fixture.handover.Assign(ctx, assignCommand(t, "SYN-ASSIGN-50", "SYN-HANDOVER-50", "unit-50")); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.handover.ConfirmByOperationDecision(ctx, confirmCommand(t, "SYN-HANDOVER-50", handoverAccount)); err != nil {
		t.Fatal(err)
	}
	if len(fixture.scopeBells.rung) != 1 {
		t.Fatalf("门铃 %d 封, want 1", len(fixture.scopeBells.rung))
	}

	recheck, err := application.NewRecheckPendingIdentityHandler(application.RecheckPendingIdentityDeps{
		Identity:       fixture.identity,
		ServiceResults: fixture.serviceResults,
		Receptions:     fixture.receptions,
		Pending:        fixture.receptions,
		Versions:       fixture.versions,
		Downstream:     fixture.downstream,
		Scope:          fixture.notes,
		Clock:          fixedClock{at: deliveredAt.Add(3 * time.Hour)},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.answerCandidates(t, map[string]string{"parcel-unit-50": handoverAccount}, nil, "parcel-unit-50")
	intentsBefore, rowsBefore := len(fixture.downstream.intents), len(fixture.receptions.byKey)
	results, err := recheck.RecheckByHandoverNote(ctx, mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewDeliveryHandoverNoteID, "SYN-HANDOVER-50"))
	if err != nil {
		t.Fatalf("按交接单重核：%v", err)
	}
	if len(results) != 1 || results[0].Outcome() != application.RecheckIntakeFormed || results[0].Rechecked() != pending.JudgmentVersion {
		t.Fatalf("重核结果 = %+v, want 只重核那件待识别、追加关联版", results)
	}
	appended, _ := results[0].Record()
	association, associated := appended.Intake.Association()
	switch {
	case !associated || association.String() != "parcel-unit-50":
		t.Fatalf("追加版没关联：%+v", appended)
	case !appended.Intake.ReceivedAt().Equal(pending.Intake.ReceivedAt()) || appended.Control.Basis() != pending.Control.Basis():
		t.Fatalf("追加版的业务时间或控制另起了：%s %s", appended.Intake.ReceivedAt(), appended.Control.Basis())
	case appended.DeliveryScopeCheck.State() != domain.DeliveryScopeVerified:
		t.Fatalf("追加版的核对态 = %+v, want 已证", appended.DeliveryScopeCheck)
	}
	if len(fixture.receptions.byKey) != rowsBefore+1 || len(fixture.downstream.intents) != intentsBefore+1 {
		t.Fatalf("行 %d → %d、意图 %d → %d, want 各多一", rowsBefore, len(fixture.receptions.byKey), intentsBefore, len(fixture.downstream.intents))
	}
	for _, untouched := range []ports.ReceptionRecord{formed, contested} {
		if _, succeeded := fixture.receptions.successorOf(untouched.Key.TenantID, untouched.JudgmentVersion); succeeded {
			t.Fatalf("%s 被重核接续了", untouched.Intake.Unit())
		}
	}

	again, err := recheck.RecheckByHandoverNote(ctx, mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewDeliveryHandoverNoteID, "SYN-HANDOVER-50"))
	if err != nil || len(again) != 0 {
		t.Fatalf("门铃重投：%+v %v, want 名下已没有重核对象", again, err)
	}
	if _, err := recheck.RecheckByHandoverNote(ctx, domain.TenantID{}, mustValue(t, domain.NewDeliveryHandoverNoteID, "SYN-HANDOVER-50")); !errors.Is(err, application.ErrRecheckCommandRejected) {
		t.Fatalf("缺租户：err = %v", err)
	}
}
