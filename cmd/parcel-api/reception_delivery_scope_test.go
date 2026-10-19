package main

import (
	"slices"
	"testing"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件经生产装配 buildReceptionOrchestration 在真库上钉可验证交付范围落到身份判断上的答法（ADR-0212 决定四、五、七；UC-NO-002
// `AT-NO-063`、`AT-NO-064`，UC-NO-006 `AT-NO-070` 的收寄半）：委托经真提交编排带号建单、经真仓储接受，预报经 NO 的预报册落册，送站
// 带不带预报标识各走一遍。夹具全为合成（S 级）。

func scopeCheckText(record noports.ReceptionRecord) string {
	check := record.DeliveryScopeCheck
	return check.State().String() + "/" + check.Reason().String() + "/" + check.Inapplicability().String()
}

// Covers: `AT-NO-064` 的真库面——两个客户账户的已接受委托用了同一个号：不带依据送站答身份冲突、两个候选（没有范围时照旧）；带客户
// 账户一的预报送站，关联客户账户一那一件、形成收寄与控制、意图一封，候选两件都记，交付范围核对态已证，来源文档里存着那份预报。
func TestTheWiredReceptionIdentifiesAPreAdvisedDeliveryAmongCandidates(t *testing.T) {
	db, reception := wiredReceptionOnRealDatabase(t)
	for _, command := range []struct{ customer, key, member string }{
		{"SYN-CUSTOMER-1", "SYN-KEY-SCOPE-1", "SYN-PARCEL-SCOPE-1"},
		{"SYN-CUSTOMER-2", "SYN-KEY-SCOPE-2", "SYN-PARCEL-SCOPE-2"},
	} {
		submission := numberedSubmission(t, command.customer, command.key, command.member, "SYN-SCOPE-NUMBER-0001")
		submitOnRealAssembly(t, db, submission)
		acceptNumbered(t, db, submission)
	}

	unadvised := deliverMarked(t, reception, "SCOPE-UNADVISED", "SYN-CUSTOMER-1", "SYN-SCOPE-NUMBER-0001")
	assertContestedBy(t, unadvised, "SYN-PARCEL-SCOPE-1", "SYN-PARCEL-SCOPE-2")

	basis := preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", "SYN-SCOPE-NUMBER-0001")
	command := receptionCommand(t, "SYN-DELIVERY-SCOPE-ADVISED", nodeopsapp.ExplicitReception)
	command.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-SCOPE-ADVISED")
	command.Mark = noports.ExternalMarkObservation{Mark: "SYN-SCOPE-NUMBER-0001"}
	command.ScopeBasis = basis
	intents := envelopeCountOfType(t, db, nodeIntakeEnvelopeType)
	result, err := reception.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("带预报收寄：%v", err)
	}
	record, _ := result.Record()
	association, associated := record.Intake.Association()
	if result.Outcome() != nodeopsapp.NodeIntakeFormed || !associated || association.String() != "SYN-PARCEL-SCOPE-1" ||
		!record.Control.Active() {
		t.Fatalf("带预报收寄 outcome %v 关联 %v, want INTAKE_FORMED 关联 SYN-PARCEL-SCOPE-1", result.Outcome(), association)
	}
	if got := candidatesOf(record); !slices.Equal(got, []string{"SYN-PARCEL-SCOPE-1", "SYN-PARCEL-SCOPE-2"}) {
		t.Fatalf("候选 = %v, want 两件都记——范围只消歧不收窄", got)
	}
	if got := scopeCheckText(record); got != "VERIFIED//" {
		t.Fatalf("核对态 = %s, want VERIFIED", got)
	}
	if n := envelopeCountOfType(t, db, nodeIntakeEnvelopeType); n != intents+1 {
		t.Fatalf("收寄意图 %d → %d, want 多一封", intents, n)
	}

	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	stored, found, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: command.TenantID, SourceID: command.SourceID})
	if err != nil || !found || stored.Source.ScopeBasis != basis || stored.DeliveryScopeCheck != record.DeliveryScopeCheck {
		t.Fatalf("读回 = %+v（found %v err %v）, want 来源带预报、核对态已证", stored, found, err)
	}
}

// Covers: `AT-NO-063` 的真库面与决定十的翻转——号码经客户包裹件号在册里只标识一件、没有信号，送站不带依据：答待识别、候选记那一件、
// 不记冲突、核对态未证·没有依据，控制成立，不关联、不交意图。翻转之前这一格答的是唯一识别。
func TestTheWiredReceptionLeavesAnUnadvisedCustomerNumberPending(t *testing.T) {
	db, reception := wiredReceptionOnRealDatabase(t)
	submission := numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-UNADVISED-1", "SYN-PARCEL-UNADVISED-1", "SYN-UNADVISED-NUMBER-0001")
	submitOnRealAssembly(t, db, submission)
	acceptNumbered(t, db, submission)

	intents := envelopeCountOfType(t, db, nodeIntakeEnvelopeType)
	result := deliverMarked(t, reception, "UNADVISED", "SYN-CUSTOMER-1", "SYN-UNADVISED-NUMBER-0001")
	assertScopeUnverifiedOnRealAssembly(t, result, "UNVERIFIED/NO_BASIS/", "SYN-PARCEL-UNADVISED-1")
	if n := envelopeCountOfType(t, db, nodeIntakeEnvelopeType); n != intents {
		t.Fatalf("收寄意图 %d → %d, want 不交", intents, n)
	}
}

// Covers: UC-NO-006 `AT-NO-070` 的收寄半——客户账户撤回自己的预报之后，带它的作业实物到站，按依据不适用·已撤回判，落范围未证的待
// 识别；撤回之前据它形成的关联不回滚。
func TestTheWiredReceptionTreatsAWithdrawnPreAdviceAsNotApplicable(t *testing.T) {
	db, reception := wiredReceptionOnRealDatabase(t)
	submission := numberedSubmission(t, "SYN-CUSTOMER-1", "SYN-KEY-WITHDRAWN-1", "SYN-PARCEL-WITHDRAWN-1", "SYN-WITHDRAWN-NUMBER-0001")
	submitOnRealAssembly(t, db, submission)
	acceptNumbered(t, db, submission)
	basis := preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", "SYN-WITHDRAWN-NUMBER-0001", "SYN-WITHDRAWN-NUMBER-0002")

	before := receptionCommand(t, "SYN-DELIVERY-WITHDRAWN-BEFORE", nodeopsapp.ExplicitReception)
	before.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-WITHDRAWN-BEFORE")
	before.Mark = noports.ExternalMarkObservation{Mark: "SYN-WITHDRAWN-NUMBER-0001"}
	before.ScopeBasis = basis
	formed, err := reception.Handle(t.Context(), before)
	if err != nil || formed.Outcome() != nodeopsapp.NodeIntakeFormed {
		t.Fatalf("撤回之前 outcome %v err %v, want INTAKE_FORMED", formed.Outcome(), err)
	}

	withdrawPreAdviceOnRealDatabase(t, db, basis, "SYN-CUSTOMER-1")
	after := receptionCommand(t, "SYN-DELIVERY-WITHDRAWN-AFTER", nodeopsapp.ExplicitReception)
	after.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-WITHDRAWN-AFTER")
	after.Mark = noports.ExternalMarkObservation{Mark: "SYN-WITHDRAWN-NUMBER-0001"}
	after.ScopeBasis = basis
	result, err := reception.Handle(t.Context(), after)
	if err != nil {
		t.Fatalf("撤回之后收寄：%v", err)
	}
	assertScopeUnverifiedOnRealAssembly(t, result, "UNVERIFIED/BASIS_NOT_APPLICABLE/WITHDRAWN", "SYN-PARCEL-WITHDRAWN-1")

	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	kept, found, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: before.TenantID, SourceID: before.SourceID})
	if association, associated := kept.Intake.Association(); err != nil || !found || kept.Kind != noports.RecordIntakeFormed ||
		!associated || association.String() != "SYN-PARCEL-WITHDRAWN-1" {
		t.Fatalf("撤回之前的关联 = %+v（found %v err %v）, want 原样留着", kept, found, err)
	}
}

// assertScopeUnverifiedOnRealAssembly 断言这一次送站落范围未证的待识别（ADR-0212 决定五）。
func assertScopeUnverifiedOnRealAssembly(t *testing.T, result nodeopsapp.ReceiveDeliveredUnitResult, check string, parcels ...string) {
	t.Helper()
	if got := result.Outcome(); got != nodeopsapp.UnitPendingIdentification {
		t.Fatalf("outcome = %v, want PENDING_IDENTIFICATION", got)
	}
	record, _ := result.Record()
	if _, associated := record.Intake.Association(); associated || record.IdentityConflict || !record.Control.Active() ||
		record.IdentityCheck != noports.IdentityChecked {
		t.Fatalf("记录 = %+v, want 不关联、不记冲突、核对过、控制成立", record)
	}
	if got := candidatesOf(record); !slices.Equal(got, parcels) {
		t.Fatalf("候选 = %v, want %v", got, parcels)
	}
	if got := scopeCheckText(record); got != check {
		t.Fatalf("核对态 = %s, want %s", got, check)
	}
}
