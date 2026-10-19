package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// 本文件对真实 PostgreSQL 证节点控制按权威运输交接转出（ADR-0214，UC-NO-005）：收寄经 NO 真收寄册落下，交接经 TF 生产登记编排登记
// （真交接册、真 outbox 交出），派发一拍把交接信封扇出给 NO 那一路；转出落在真转移册上，收寄读回经折叠看得见。直投那几条用与生产
// 同一个装配函数、同一份哨兵名单。夹具全部合成，只记 `S`。

const (
	nodeControlConsumerName = "node-operations/transfer-control-on-transport-handover"
	nodeControlTenant       = "SYN-TENANT-01"
	nodeControlNode         = "SYN-NODE-SHA-HUB"
)

var nodeControlReceivedAt = time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC)

// receiveIntoControl 在 NO 真收寄册上落一行收寄已形成：作业实物 unit 关联正式包裹 parcel，控制以这次收寄成立。
func receiveIntoControl(t *testing.T, db *bentopg.DB, unit, parcel string) noports.ReceptionRecord {
	t.Helper()
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("收寄册：%v", err)
	}
	tenant := mustNO(t, nodomain.NewTenantID, nodeControlTenant)
	version := mustNO(t, nodomain.NewIntakeResultVersion, "SYN-INTAKEV-"+unit)
	intake, err := nodomain.FormNodeIntake(nodomain.NodeIntakeSpec{
		TenantID:    tenant,
		Unit:        mustNO(t, nodomain.NewHandlingUnitID, unit),
		Node:        mustNO(t, nodomain.NewNodeReference, nodeControlNode),
		DeliveredBy: mustNO(t, nodomain.NewDeliveringPartyReference, "SYN-DRIVER-01"),
		Evidence:    mustNO(t, nodomain.NewReceptionEvidenceReference, "SYN-EVIDENCE/receipt-"+unit),
		Version:     version,
		Association: mustNO(t, nodomain.NewParcelAssociationReference, parcel),
		ReceivedAt:  nodeControlReceivedAt,
	})
	if err != nil {
		t.Fatalf("收寄：%v", err)
	}
	control, err := nodomain.EstablishPhysicalControl(nodomain.PhysicalControlSpec{
		TenantID: tenant, Unit: intake.Unit(), Node: intake.Node(), Kind: nodomain.EstablishedByNodeIntake,
		Basis:         mustNO(t, nodomain.NewControlBasisReference, "NODE-INTAKE/"+version.String()),
		EstablishedAt: nodeControlReceivedAt,
	})
	if err != nil {
		t.Fatalf("控制：%v", err)
	}
	record := noports.ReceptionRecord{
		Key:                noports.ReceptionKey{TenantID: tenant, SourceID: "SYN-DEVICE-SHA-HUB-01/" + unit},
		ContentDigest:      "SYN-DIGEST-" + unit,
		Kind:               noports.RecordIntakeFormed,
		JudgmentVersion:    version,
		Intake:             intake,
		Control:            control,
		ServiceResultCheck: noports.ServiceResultsChecked,
		RecordedAt:         nodeControlReceivedAt,
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, err := receptions.Save(txCtx, record)
		return err
	})
	return record
}

// handOverAtShanghai 是上海枢纽把包裹交出的一次登记命令：不进段，只登交接本身。
func handOverAtShanghai(t *testing.T, parcel, scope, version string, verdict tfdomain.HandoverVerdict) tfapplication.RegisterTransportHandoverCommand {
	t.Helper()
	command := tfapplication.RegisterTransportHandoverCommand{
		TenantID:   mustTF(t, tfdomain.NewTenantID, nodeControlTenant),
		Object:     parcel,
		Scope:      scope,
		ReleasedBy: nodeControlNode,
		ReceivedBy: "SYN-DRIVER-SHA-SZX-01",
		Verdict:    verdict,
		Version:    version,
		JudgedAt:   nodeControlReceivedAt.Add(3 * time.Hour),
	}
	if verdict == tfdomain.ObjectHandedOver {
		command.ReleasingEvidence, command.ReceivingEvidence, command.Rule = "SYN-LOAD-SCAN-61", "SYN-DRIVER-ACK-61", "SYN-HANDOVER-RULE/v1"
	} else {
		command.Basis = "SYN-SEAL-MISMATCH-61"
	}
	return command
}

func registerAt(t *testing.T, db *bentopg.DB, store *outbox.Store, command tfapplication.RegisterTransportHandoverCommand) {
	t.Helper()
	register := newHandoverRegistration(t, db, store)
	var result tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = register.Register(txCtx, command)
		return err
	})
	if result.Outcome() != tfapplication.HandoverRegistered {
		t.Fatalf("登记交接 %s：%s", command.Version, result.Outcome())
	}
}

// nodeControlEventID 与生产交出的信封 ID 同一拼法（四维直拼）：生产路由那一条用例按它去库里找真交出的那一封。
func nodeControlEventID(parcel, scope, version string) string {
	return nodeControlTenant + "/" + parcel + "/" + scope + "/" + version
}

// readReception 按键读回那一行收寄，控制经折叠读出。
func readReception(t *testing.T, db *bentopg.DB, record noports.ReceptionRecord) noports.ReceptionRecord {
	t.Helper()
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("收寄册：%v", err)
	}
	read, found, err := receptions.FindByKey(t.Context(), record.Key)
	if err != nil || !found {
		t.Fatalf("读回收寄：found=%v err=%v", found, err)
	}
	return read
}

func controlTransitions(t *testing.T, db *bentopg.DB) []noports.PhysicalControlTransitionCatalogueRow {
	t.Helper()
	catalogue, err := nopostgres.NewReviewCatalogue(db)
	if err != nil {
		t.Fatalf("查阅目录：%v", err)
	}
	rows, err := catalogue.ListPhysicalControlTransitions(t.Context(), mustNO(t, nodomain.NewTenantID, nodeControlTenant), 50)
	if err != nil {
		t.Fatalf("上列转移册：%v", err)
	}
	return rows
}

// Covers: `AT-NO-101`、`AT-NO-112`（ADR-0214 决定一、三、六）——生产路由表上交接信封扇出到 NO 这一路：上海枢纽收寄在身的那件包裹交接
// `已交接`之后派发一拍，信封 PUBLISHED、NO 名下 inbox 一行；转移册落一行 OUT·ADVANCED，收寄按键读回控制已转出、controlReleasedBy 是
// `TRANSPORT-HANDOVER/<版本>`。三路认的事件类型与生产交出的是同一个串。
func TestAHandoverTransfersOutTheNodeControlThroughTheProductionRoute(t *testing.T) {
	if noinbox.TransportHandoverRegisteredEventType != veinbox.TransportHandoverRegisteredEventType {
		t.Fatalf("NO 认 %q、VE 认 %q——同一封信两路认的不是同一个类型",
			noinbox.TransportHandoverRegisteredEventType, veinbox.TransportHandoverRegisteredEventType)
	}
	beat, db, store := wiredBeat(t)
	received := receiveIntoControl(t, db, "SYN-HU-61-A", "SYN-PARCEL-61-A")
	registerAt(t, db, store, handOverAtShanghai(t, "SYN-PARCEL-61-A", "SYN-SCOPE-61", "SYN-HO-61-A/v1", tfdomain.ObjectHandedOver))

	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("一拍：%v", err)
	}
	eventID := nodeControlEventID("SYN-PARCEL-61-A", "SYN-SCOPE-61", "SYN-HO-61-A/v1")
	if status := outboxStatus(t, db, eventID); status != "PUBLISHED" {
		t.Fatalf("交接信封 status = %s，失败码 = %q", status, recordedFailureCode(t, db, eventID))
	}
	if n := inboxRowsOf(t, db, nodeControlConsumerName, eventID); n != 1 {
		t.Fatalf("NO 名下 inbox 行 = %d, want 1——FanOut 少了这一路", n)
	}
	rows := controlTransitions(t, db)
	if len(rows) != 1 || rows[0].Outcome != "ADVANCED" || rows[0].Direction != "OUT" || rows[0].Unit != "SYN-HU-61-A" ||
		rows[0].Node != nodeControlNode || rows[0].TransferReference != "TRANSPORT-HANDOVER/SYN-HO-61-A/v1" {
		t.Fatalf("转移册 = %+v", rows)
	}
	read := readReception(t, db, received)
	if reference, _, released := read.Control.Release(); !released || reference.String() != "TRANSPORT-HANDOVER/SYN-HO-61-A/v1" {
		t.Fatalf("收寄读回的控制 released=%v %q", released, reference)
	}
}

type nodeControlLane struct {
	db    *bentopg.DB
	store *outbox.Store
	lane  dispatch.Consumer
}

// newNodeControlLane 接一条与生产同形的 NO 这一路（同一个装配函数、同一份哨兵名单），信封由用例直投。
func newNodeControlLane(t *testing.T) *nodeControlLane {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	fixture := &nodeControlLane{db: db}
	if fixture.store, err = outbox.NewStore(db); err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	inboxStore, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	consumer, err := transferNodeControlOnHandoverConsumer(db, inboxStore, systemClock{})
	if err != nil {
		t.Fatalf("装配 NO 这一路：%v", err)
	}
	if fixture.lane, err = dispatch.WithUndecidedSentinels(consumer, nodeControlOnHandoverUndecidedSentinels...); err != nil {
		t.Fatalf("包未决哨兵：%v", err)
	}
	return fixture
}

func (fixture *nodeControlLane) deliver(parcel, scope, version string) error {
	at := beatInstant()
	return fixture.lane.Consume(context.Background(), eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(nodeControlEventID(parcel, scope, version)),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         noinbox.TransportHandoverRegisteredEventType,
		Version:      1,
		Scope:        nodeControlTenant,
		Subject:      parcel,
		PartitionKey: nodeControlTenant + "/" + parcel + "/transport-handover-registration",
		OccurredAt:   at,
		RecordedAt:   at,
		ContentType:  eventing.JSONContentType,
		Payload: []byte(`{"tenantId":"` + nodeControlTenant + `","object":"` + parcel + `","scope":"` + scope +
			`","version":"` + version + `"}`),
	})
}

// Covers: `AT-NO-102`、`AT-NO-103`（ADR-0214 决定四、六）——同一封重投在消费门停下、不落第二行；拒收与待确认零写入、控制照旧在身。
func TestRedeliveriesAndRefusalsDoNotMoveTheControlAgain(t *testing.T) {
	fixture := newNodeControlLane(t)
	transferred := receiveIntoControl(t, fixture.db, "SYN-HU-61-B", "SYN-PARCEL-61-B")
	kept := receiveIntoControl(t, fixture.db, "SYN-HU-61-C", "SYN-PARCEL-61-C")
	registerAt(t, fixture.db, fixture.store, handOverAtShanghai(t, "SYN-PARCEL-61-B", "SYN-SCOPE-61", "SYN-HO-61-B/v1", tfdomain.ObjectHandedOver))
	registerAt(t, fixture.db, fixture.store, handOverAtShanghai(t, "SYN-PARCEL-61-C", "SYN-SCOPE-61", "SYN-HO-61-C/v1", tfdomain.HandoverRefused))
	registerAt(t, fixture.db, fixture.store, handOverAtShanghai(t, "SYN-PARCEL-61-C", "SYN-SCOPE-61-P", "SYN-HO-61-C/v1", tfdomain.HandoverPendingConfirmation))

	for _, delivery := range [][3]string{
		{"SYN-PARCEL-61-B", "SYN-SCOPE-61", "SYN-HO-61-B/v1"},
		{"SYN-PARCEL-61-B", "SYN-SCOPE-61", "SYN-HO-61-B/v1"},
		{"SYN-PARCEL-61-C", "SYN-SCOPE-61", "SYN-HO-61-C/v1"},
		{"SYN-PARCEL-61-C", "SYN-SCOPE-61-P", "SYN-HO-61-C/v1"},
	} {
		if err := fixture.deliver(delivery[0], delivery[1], delivery[2]); err != nil {
			t.Fatalf("投递 %v：%v", delivery, err)
		}
	}
	if rows := controlTransitions(t, fixture.db); len(rows) != 1 || rows[0].Unit != "SYN-HU-61-B" {
		t.Fatalf("转移册 = %+v, want 只有 B 那一次转出", rows)
	}
	if readReception(t, fixture.db, transferred).Control.Active() || !readReception(t, fixture.db, kept).Control.Active() {
		t.Fatal("B 该已转出、C 该仍在身")
	}
}

// Covers: `AT-NO-109`（ADR-0214 决定五）——撤回控制的更正：`已交接`转出之后，同一交接更正成拒收，更正版的信封入账「控制来源链待重算」，
// 控制不回退；查阅口看得见这一行，带先前那次转出。
func TestAWithdrawingCorrectionIsVisibleAndDoesNotRevertTheControl(t *testing.T) {
	fixture := newNodeControlLane(t)
	received := receiveIntoControl(t, fixture.db, "SYN-HU-61-D", "SYN-PARCEL-61-D")
	registerAt(t, fixture.db, fixture.store, handOverAtShanghai(t, "SYN-PARCEL-61-D", "SYN-SCOPE-61", "SYN-HO-61-D/v1", tfdomain.ObjectHandedOver))
	if err := fixture.deliver("SYN-PARCEL-61-D", "SYN-SCOPE-61", "SYN-HO-61-D/v1"); err != nil {
		t.Fatalf("投递前版：%v", err)
	}
	register := newHandoverRegistration(t, fixture.db, fixture.store)
	var corrected tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, fixture.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		corrected, err = register.Correct(txCtx, tfapplication.CorrectTransportHandoverCommand{
			TenantID: mustTF(t, tfdomain.NewTenantID, nodeControlTenant), Object: "SYN-PARCEL-61-D", Scope: "SYN-SCOPE-61",
			PredecessorVersion: "SYN-HO-61-D/v1", Verdict: tfdomain.HandoverRefused, Basis: "SYN-DRIVER-DISPUTE-61",
			NewVersion: "SYN-HO-61-D/v2", CorrectedAt: nodeControlReceivedAt.Add(4 * time.Hour),
		})
		return err
	})
	if corrected.Outcome() != tfapplication.HandoverCorrected {
		t.Fatalf("更正：%s", corrected.Outcome())
	}

	if err := fixture.deliver("SYN-PARCEL-61-D", "SYN-SCOPE-61", "SYN-HO-61-D/v2"); err != nil {
		t.Fatalf("投递更正版：%v", err)
	}
	rows := controlTransitions(t, fixture.db)
	if len(rows) != 2 || rows[0].Reason != "CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE" || rows[0].HandoverVersion != "SYN-HO-61-D/v2" ||
		rows[0].TransferReference != "TRANSPORT-HANDOVER/SYN-HO-61-D/v1" {
		t.Fatalf("转移册 = %+v, want 新近一行是待重算、带先前那次转出", rows)
	}
	if readReception(t, fixture.db, received).Control.Active() {
		t.Fatal("撤回控制的更正把控制回退了")
	}
}

// Covers: `AT-NO-110`（ADR-0214 决定四）——同一件实物的两份`已交接`并发到达：实物锁把两路串起来，只落一行转出；后到的一方在锁下读到
// 控制已转出，入账「无在身控制」。
func TestConcurrentHandoversOfOneUnitTransferItOnce(t *testing.T) {
	fixture := newNodeControlLane(t)
	receiveIntoControl(t, fixture.db, "SYN-HU-61-E", "SYN-PARCEL-61-E")
	scopes := []string{"SYN-SCOPE-61-E1", "SYN-SCOPE-61-E2"}
	for _, scope := range scopes {
		registerAt(t, fixture.db, fixture.store, handOverAtShanghai(t, "SYN-PARCEL-61-E", scope, "SYN-HO-61-E/v1", tfdomain.ObjectHandedOver))
	}

	var wait sync.WaitGroup
	errs := make([]error, len(scopes))
	for index, scope := range scopes {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs[index] = fixture.deliver("SYN-PARCEL-61-E", scope, "SYN-HO-61-E/v1")
		}()
	}
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("投递 %s：%v", scopes[index], err)
		}
	}
	advanced, idle := 0, 0
	for _, row := range controlTransitions(t, fixture.db) {
		switch {
		case row.Outcome == "ADVANCED":
			advanced++
		case row.Reason == "NO_ACTIVE_CONTROL":
			idle++
		}
	}
	if advanced != 1 || idle != 1 {
		t.Fatalf("转出 %d 行、无在身控制 %d 行, want 各 1", advanced, idle)
	}
}
