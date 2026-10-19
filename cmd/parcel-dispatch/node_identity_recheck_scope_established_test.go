package main

import (
	"context"
	"slices"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	noidentity "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	nopc "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/partycommercial"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	noapplication "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证「交付范围成立」门铃经生产装配续得上（ADR-0212 决定六；票 ps-external-mark-relations/12，UC-NO-006
// `AT-NO-075`）：路由表认得出这一类、投给 NO 同一个重核消费者；交接单最后一个绑定经运营决定确认后，名下没带依据而落待识别的同号
// 实物经重核追加关联版。夹具全为合成（S 级）。

const (
	scopeEstablishedDoorbellType = "node-operations.delivery-scope.established"
	scopeMark                    = "SYN-CUSTOMER-PARCEL-PSXM12"
	scopeNode                    = "SYN-NODE-01"
	scopeOwnerParty              = "SYN-PARTY-OWNER-PSXM12"
)

// Covers: 路由表认得出「交付范围成立」门铃这一类，投给重核消费者。手法同别的门铃：毒丸载荷让消费门显式拒收入账并交回 nil，于是
// 这一条定稿；漏挂或挂错撞的是无订阅者。
func TestAScopeEstablishedDoorbellReachesTheRecheckConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "scope-established-1", noinbox.DeliveryScopeEstablishedEventType, `{}`)
	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把门铃投给重核消费者", published, recordedFailureCode(t, db, "scope-established-1"))
	}
}

// Covers: NO 消费者自写的门铃类型串与 NO 门铃适配器写出的那一个相等，照别的自投门铃那一条在组合根钉住。
func TestTheRecheckConsumerAndTheScopeDoorbellAgreeOnTheEventType(t *testing.T) {
	if string(noinbox.DeliveryScopeEstablishedEventType) != nopostgres.DeliveryScopeEstablishedEventType ||
		nopostgres.DeliveryScopeEstablishedEventType != scopeEstablishedDoorbellType {
		t.Fatalf("NO 消费者认 %q，NO 门铃写出 %q", noinbox.DeliveryScopeEstablishedEventType, nopostgres.DeliveryScopeEstablishedEventType)
	}
}

// handoverHandlerForDispatch 照 parcel-api 的生产装配用 NO 的真库口装交接单编排：交接单册、收寄库、授权交付判断经 NO 的防腐适配器
// 问 PC 的真库读口、「交付范围成立」门铃写 outbox。
func handoverHandlerForDispatch(t *testing.T, db *bentopg.DB) *noapplication.DeliveryHandoverNoteHandler {
	t.Helper()
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := nopostgres.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatal(err)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatal(err)
	}
	judgments, err := pcpostgres.NewAuthorizedDeliveryJudgments(db)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := nopc.NewAuthorizedDeliveryView(judgments)
	if err != nil {
		t.Fatal(err)
	}
	doorbell, err := nopostgres.NewOutboxDeliveryScopeEstablishedDoorbell(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := noapplication.NewDeliveryHandoverNoteHandler(noapplication.DeliveryHandoverDeps{
		Notes: notes, Receptions: receptions, AuthorizedDelivery: authorized, Doorbell: doorbell, Clock: systemClock{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// receiveWithoutBasisForDispatch 经收寄编排（生产适配器）收一件不带依据、号码只经客户包裹件号命中的实物：落范围未证的待识别。
func receiveWithoutBasisForDispatch(t *testing.T, db *bentopg.DB, tenant nodomain.TenantID, receivedAt time.Time) noports.ReceptionRecord {
	t.Helper()
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := noidentity.NewIntakeResultVersions()
	if err != nil {
		t.Fatal(err)
	}
	downstream, err := nopostgres.NewOutboxNodeIntakeHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	doorbell, err := nopostgres.NewOutboxIdentityRecheckDoorbell(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	identifiers, err := pspostgres.NewExternalIdentifiers(db)
	if err != nil {
		t.Fatal(err)
	}
	view, err := noparcel.NewParcelIdentityView(identifiers)
	if err != nil {
		t.Fatal(err)
	}
	services, err := nodeIdentityRecheckServiceResults(db)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := nopostgres.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := noapplication.NewReceiveDeliveredUnitHandler(noapplication.ReceiveDeliveredUnitDeps{
		Identity: view, ServiceResults: services, Receptions: receptions, Versions: versions, Downstream: downstream,
		Doorbell: doorbell, Scope: deliveryScopeEvidenceFor(t, db), Handover: notes, Clock: systemClock{},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := noapplication.ReceiveDeliveredUnitCommand{
		TenantID:    tenant,
		SourceID:    "SYN-DELIVERY-PSXM12",
		Node:        mustNO(t, nodomain.NewNodeReference, scopeNode),
		DeliveredBy: mustNO(t, nodomain.NewDeliveringPartyReference, "SYN-DRIVER-PSXM12"),
		Unit:        mustNO(t, nodomain.NewHandlingUnitID, "SYN-UNIT-PSXM12"),
		Mark:        noports.ExternalMarkObservation{Mark: scopeMark},
		Claim:       noapplication.ExplicitReception,
		Evidence:    mustNO(t, nodomain.NewReceptionEvidenceReference, "SYN-SIGN-PSXM12"),
		OccurredAt:  receivedAt,
	}
	var result noapplication.ReceiveDeliveredUnitResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = handler.Handle(txCtx, command)
		return handleErr
	})
	record, recorded := result.Record()
	if result.Outcome() != noapplication.UnitPendingIdentification || !recorded || record.IdentityConflict ||
		record.DeliveryScopeCheck.State() != nodomain.DeliveryScopeUnverified {
		t.Fatalf("不带依据收寄 = %v %+v, want 范围未证、不记冲突的待识别", result.Outcome(), record)
	}
	return record
}

// memberRecordForDispatch 直接在收寄册上落一件这张交接单名下的实物：associated 时是已关联到别的包裹的收寄，否则是记多候选冲突的
// 待识别；并经交接单册把它记成收寄带来的成员。它们是「名下已关联或记冲突」的两种，重核不该碰它们。
func memberRecordForDispatch(t *testing.T, db *bentopg.DB, tenant nodomain.TenantID, note nodomain.DeliveryHandoverNoteID, suffix string, receivedAt time.Time, associated bool) noports.ReceptionRecord {
	t.Helper()
	unit := mustNO(t, nodomain.NewHandlingUnitID, "SYN-UNIT-PSXM12-"+suffix)
	node := mustNO(t, nodomain.NewNodeReference, scopeNode)
	version := mustNO(t, nodomain.NewIntakeResultVersion, "SYN-INTAKEV-PSXM12-"+suffix)
	spec := nodomain.NodeIntakeSpec{
		TenantID: tenant, Unit: unit, Node: node, Version: version, ReceivedAt: receivedAt,
		DeliveredBy: mustNO(t, nodomain.NewDeliveringPartyReference, "SYN-DRIVER-PSXM12"),
		Evidence:    mustNO(t, nodomain.NewReceptionEvidenceReference, "SYN-SIGN-PSXM12-"+suffix),
	}
	record := noports.ReceptionRecord{
		Key:             noports.ReceptionKey{TenantID: tenant, SourceID: "SYN-DELIVERY-PSXM12-" + suffix},
		ContentDigest:   "SYN-DIGEST-PSXM12-" + suffix,
		Kind:            noports.RecordPendingIdentification,
		JudgmentVersion: version,
		Source: nodomain.DeliverySource{
			Unit: unit, Mark: "SYN-MARK-PSXM12-" + suffix, Claim: "RECEIVED", Evidence: spec.Evidence, DeliveredBy: spec.DeliveredBy,
			Node: node, OccurredAt: receivedAt,
			ScopeBasis: mustNO(t, func(raw string) (nodomain.DeliveryScopeBasisReference, error) {
				return nodomain.NewDeliveryScopeBasisReference(nodomain.BasisDeliveryHandoverNote, raw)
			}, note.String()),
		},
		Candidates: []nodomain.ParcelAssociationReference{
			mustNO(t, nodomain.NewParcelAssociationReference, "SYN-PARCEL-PSXM12-"+suffix+"-A"),
			mustNO(t, nodomain.NewParcelAssociationReference, "SYN-PARCEL-PSXM12-"+suffix+"-B"),
		},
		IdentityConflict:      true,
		IdentityConflictCause: noports.ConflictByMultipleCandidates,
		IdentityCheck:         noports.IdentityChecked,
		RecordedAt:            receivedAt,
	}
	if associated {
		spec.Association = mustNO(t, nodomain.NewParcelAssociationReference, "SYN-PARCEL-PSXM12-"+suffix)
		record.Kind, record.IdentityConflict, record.IdentityConflictCause, record.IdentityCheck = noports.RecordIntakeFormed, false, noports.IdentityConflictCauseAbsent, noports.IdentityCheckAbsent
		record.Candidates = []nodomain.ParcelAssociationReference{spec.Association}
		record.ServiceResultCheck = noports.ServiceResultsChecked
	}
	intake, err := nodomain.FormNodeIntake(spec)
	if err != nil {
		t.Fatal(err)
	}
	control, err := nodomain.EstablishPhysicalControl(nodomain.PhysicalControlSpec{
		TenantID: tenant, Unit: unit, Node: node, Kind: nodomain.EstablishedByNodeIntake,
		Basis: mustNO(t, nodomain.NewControlBasisReference, "NODE-INTAKE/"+version.String()), EstablishedAt: receivedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	record.Intake, record.Control = intake, control
	member, err := nodomain.FormDeliveryHandoverMember(nodomain.DeliveryHandoverMemberSpec{
		TenantID: tenant, Note: note, Unit: unit, Kind: nodomain.MembershipByReception, FactID: record.Key.SourceID, FormedAt: receivedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := nopostgres.NewDeliveryHandoverNotes(db)
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		if _, err := receptions.Save(txCtx, record); err != nil {
			return err
		}
		_, _, err := notes.AppendDeliveryHandoverMember(txCtx, member)
		return err
	})
	return record
}

// registerScopeOwnerInPartyCommercial 在 party-commercial 登记 SYN 夹具的客户账户，所属参与方是 scopeOwnerParty：运营决定指名它作
// 交付方参与方时，授权交付判断答「交付方就是该客户账户所属参与方本身」。
func registerScopeOwnerInPartyCommercial(t *testing.T, db *bentopg.DB, tenant, account string) {
	t.Helper()
	registrations, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatal(err)
	}
	rehydrated, err := pcdomain.RehydrateCustomerAccount(mustPC(t, pcdomain.NewTenantID, tenant), mustPC(t, pcdomain.NewCustomerAccountID, account), mustPC(t, pcdomain.NewPartyID, scopeOwnerParty))
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := pcdomain.NewIdentityLifecycle(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	registration, err := pcdomain.NewCustomerAccountRegistration(rehydrated, 1, mustPC(t, pcdomain.NewIdentityBasisReference, "SYN-BASIS-PSXM12"), lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	var outcome pcports.PartyRegistrySaveOutcome
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = registrations.SaveCustomerAccount(txCtx, registration)
		return saveErr
	})
	if outcome != pcports.PartyRegistrySaved {
		t.Fatalf("登记客户账户：%s", outcome)
	}
}

// Covers: `AT-NO-075`（UC-NO-006）与票 12 完成判据第五格的 parcel-dispatch 半边：委托带客户包裹件号接受；同号实物不带依据送站落范围
// 未证的待识别；开立交接单、把它归入；名下另有一件已关联、一件记多候选冲突；运营决定确认（交付方就是账户所属参与方本身）→ 同一笔
// 门铃一封 → 派发投给重核消费者 → 追加一版`节点收寄已形成`（前版是待识别那一版、关联那件包裹、核对态已证），业务时间取原收寄时刻、
// 控制不另起，收寄意图恰一封；名下已关联与记冲突的两件不追加任何一版。
func TestTheWiredDispatchRechecksAHandoverNoteOnceItsScopeIsEstablished(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	tenant := mustNO(t, nodomain.NewTenantID, fixture.identity.TenantID().String())
	account := fixture.identity.CustomerAccountID().String()
	receivedAt := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)

	submitWithCustomerParcelNumber(t, fixture, scopeMark)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("PS 门铃那一拍：%v", err)
	}
	registerScopeOwnerInPartyCommercial(t, fixture.db, tenant.String(), account)
	pending := receiveWithoutBasisForDispatch(t, fixture.db, tenant, receivedAt)

	handover := handoverHandlerForDispatch(t, fixture.db)
	note := mustNO(t, nodomain.NewDeliveryHandoverNoteID, "SYN-HANDOVER-PSXM12-D75")
	node := mustNO(t, nodomain.NewNodeReference, scopeNode)
	open := noapplication.OpenDeliveryHandoverNoteCommand{
		TenantID: tenant, Node: node, Note: note,
		DeliveringParty: mustNO(t, nodomain.NewDeliveringPartyReference, "SYN-DRIVER-PSXM12"),
		ClaimedAccounts: []nodomain.CustomerAccountReference{mustNO(t, nodomain.NewCustomerAccountReference, account)},
		OpenedAt:        receivedAt.Add(-time.Minute),
	}
	var outcomes []noapplication.DeliveryHandoverOutcome
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		opened, err := handover.Open(txCtx, open)
		if err != nil {
			return err
		}
		assigned, err := handover.Assign(txCtx, noapplication.AssignToDeliveryHandoverNoteCommand{
			TenantID: tenant, Node: node, FactID: "SYN-ASSIGN-PSXM12-D75", Note: note, Unit: pending.Intake.Unit(), AssignedAt: receivedAt.Add(time.Minute),
		})
		outcomes = append(outcomes, opened.Outcome(), assigned.Outcome())
		return err
	})
	if !slices.Equal(outcomes, []noapplication.DeliveryHandoverOutcome{noapplication.DeliveryHandoverNoteOpened, noapplication.DeliveryHandoverUnitAssigned}) {
		t.Fatalf("开立与归入 = %v", outcomes)
	}
	formed := memberRecordForDispatch(t, fixture.db, tenant, note, "FORMED", receivedAt, true)
	contested := memberRecordForDispatch(t, fixture.db, tenant, note, "CONTESTED", receivedAt, false)

	confirm := noapplication.ConfirmDeliveryHandoverCommand{
		TenantID: tenant, Operator: "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-PSXM12", Note: note,
		Account:               mustNO(t, nodomain.NewCustomerAccountReference, account),
		DeliveringParticipant: mustNO(t, nodomain.NewDeliveringPartyReference, scopeOwnerParty),
		Basis:                 "SYN-BASIS-COUNTER-PSXM12",
	}
	var confirmed noapplication.DeliveryHandoverResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		confirmed, err = handover.ConfirmByOperationDecision(txCtx, confirm)
		return err
	})
	if confirmed.Outcome() != noapplication.DeliveryHandoverBindingConfirmed {
		t.Fatalf("运营决定确认 = %v, want 绑定已确认", confirmed.Outcome())
	}
	doorbells := fixture.outboxIDsOfType(t, scopeEstablishedDoorbellType)
	if len(doorbells) != 1 {
		t.Fatalf("范围成立后门铃 %d 封, want 1", len(doorbells))
	}
	intentsBefore := len(intakeIntentSources(t, fixture.db))

	for attempt := 0; fixture.countInbox(t, recheckConsumerName, doorbells[0]) == 0; attempt++ {
		if attempt == 20 {
			t.Fatalf("门铃 %d 拍仍未处理，失败码 %q", attempt, recordedFailureCode(t, fixture.db, doorbells[0]))
		}
		time.Sleep(5 * time.Millisecond)
		if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
			t.Fatalf("派发那一拍：%v", err)
		}
	}

	receptions, err := nopostgres.NewReceptions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	derived := noports.ReceptionKey{TenantID: tenant, SourceID: "IDENTITY-RECHECK/" + pending.JudgmentVersion.String()}
	appended, found, err := receptions.FindByKey(ctx, derived)
	if err != nil || !found {
		t.Fatalf("追加版没落册：found = %v err = %v", found, err)
	}
	association, associated := appended.Intake.Association()
	switch {
	case appended.Kind != noports.RecordIntakeFormed || !associated || association.String() != "SYN-PARCEL-01":
		t.Fatalf("追加版 = %+v, want 节点收寄已形成、关联 SYN-PARCEL-01", appended)
	case appended.Predecessor != pending.JudgmentVersion:
		t.Fatalf("追加版前版 = %s, want 待识别那一版 %s", appended.Predecessor, pending.JudgmentVersion)
	case !appended.Intake.ReceivedAt().Equal(receivedAt) || appended.Control.Basis() != pending.Control.Basis():
		t.Fatalf("追加版接收 %s 控制 %s, want 照原收寄 %s、%s", appended.Intake.ReceivedAt(), appended.Control.Basis(), receivedAt, pending.Control.Basis())
	case appended.DeliveryScopeCheck.State() != nodomain.DeliveryScopeVerified:
		t.Fatalf("追加版核对态 = %+v, want 已证", appended.DeliveryScopeCheck)
	}
	intents := intakeIntentSources(t, fixture.db)
	if len(intents) != intentsBefore+1 || !slices.Contains(intents, derived.SourceID) {
		t.Fatalf("收寄意图 = %v（之前 %d 封）, want 多恰一封、指回追加版 %s", intents, intentsBefore, derived.SourceID)
	}
	for _, untouched := range []noports.ReceptionRecord{formed, contested} {
		current, err := receptions.CurrentVersion(ctx, tenant, untouched.JudgmentVersion)
		if err != nil || current != untouched.JudgmentVersion {
			t.Fatalf("%s 被重核接续了：当前 %s（err %v）", untouched.Intake.Unit(), current, err)
		}
	}
}
