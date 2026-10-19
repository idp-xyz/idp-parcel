package main

import (
	"context"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证节点控制按权威运输交接转入（ADR-0214 决定七与文末票 62 补记，UC-NO-005 转入一半）：交接经 TF 生产登记
// 编排登记、声明节点（声明了交出节点的`已交接`按 ADR-0213 决定四配交出门禁读数），派发把信封交给 NO 那一路，转入落在真转移册上、
// 在身控制经真折叠读出。夹具全部合成，只记 `S`。

const (
	inShanghaiHub = "SYN-NODE-SHA-HUB"
	inShenzhenGW  = "SYN-NODE-SZX-GATE"
	inSingaporeHB = "SYN-NODE-SIN-HUB"
)

// declaredHandover 是一次声明了节点的`已交接`登记命令，不进段。
func declaredHandover(parcel, scope, version, releasing, receiving, onward string, at time.Time) tfapplication.RegisterTransportHandoverCommand {
	return tfapplication.RegisterTransportHandoverCommand{
		Object:            parcel,
		Scope:             scope,
		ReleasedBy:        "SYN-PARTY-RELEASER-62",
		ReceivedBy:        "SYN-PARTY-RECEIVER-62",
		Verdict:           tfdomain.ObjectHandedOver,
		ReleasingEvidence: "SYN-LOAD-SCAN-62",
		ReceivingEvidence: "SYN-RECEIPT-ACK-62",
		Rule:              "SYN-HANDOVER-RULE/v1",
		Version:           version,
		JudgedAt:          at,
		ReleasingNode:     releasing,
		ReceivingNode:     receiving,
		OnwardNode:        onward,
	}
}

// registerDeclared 经 TF 生产登记编排登一次交接：声明了交出节点的`已交接`带一份问过关务的交出门禁读数（意图照声明，去向在场就带
// 上），其余不带。读数取「不受门禁」，只要登记过得去：交出门禁归 ADR-0213 决定四，不是本文件要证的。
func registerDeclared(t *testing.T, db *bentopg.DB, store *outbox.Store, command tfapplication.RegisterTransportHandoverCommand) {
	t.Helper()
	command.TenantID = mustTF(t, tfdomain.NewTenantID, nodeControlTenant)
	register := newHandoverRegistration(t, db, store)
	var reading tfdomain.HandoverGateReading
	if command.ReleasingNode != "" {
		finding, err := tfdomain.NewGateFinding(mustTF(t, tfdomain.NewCarriedObjectReference, command.Object), tfdomain.GateNotGuarded,
			"AADJ-"+command.Object, []tfdomain.GateBasis{{Action: "CROSS_CUSTOMS_MOVEMENT", Cause: "CUSTOMS_SCOPE_UNRESOLVED", Sources: []string{"SYN-SOURCE-62"}}})
		if err != nil {
			t.Fatalf("交出门禁那一格：%v", err)
		}
		spec := tfdomain.HandoverGateReadingSpec{Node: mustTF(t, tfdomain.NewNetworkNodeReference, command.ReleasingNode), Finding: finding}
		if command.OnwardNode != "" {
			spec.Onward = mustTF(t, tfdomain.NewNetworkNodeReference, command.OnwardNode)
		}
		if reading, err = tfdomain.FormHandoverGateReading(spec); err != nil {
			t.Fatalf("交出门禁读数：%v", err)
		}
	}
	var result tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = register.RegisterWithGateReading(txCtx, command, reading)
		return err
	})
	if result.Outcome() != tfapplication.HandoverRegistered {
		t.Fatalf("登记交接 %s：%s", command.Version, result.Outcome())
	}
}

func controlTransitionsOf(t *testing.T, db *bentopg.DB, version string) map[string]noports.PhysicalControlTransitionCatalogueRow {
	t.Helper()
	rows := map[string]noports.PhysicalControlTransitionCatalogueRow{}
	for _, row := range controlTransitions(t, db) {
		if row.HandoverVersion == version {
			rows[row.Direction] = row
		}
	}
	return rows
}

func activeControlsOf(t *testing.T, db *bentopg.DB, unit string) []nodomain.PhysicalControl {
	t.Helper()
	transitions, err := nopostgres.NewPhysicalControlTransitions(db)
	if err != nil {
		t.Fatalf("转移册：%v", err)
	}
	var active []nodomain.PhysicalControl
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		active, err = transitions.FindActiveControls(txCtx, mustNO(t, nodomain.NewTenantID, nodeControlTenant),
			mustNO(t, nodomain.NewHandlingUnitID, unit))
		return err
	})
	return active
}

// Covers: `AT-NO-113`、`AT-NO-114`、`AT-NO-115`、`AT-NO-116`、`AT-NO-125`（ADR-0214 决定四、七）——生产路由表上一件包裹的控制跟着交接走：
// 上海枢纽收寄在身；自营段交接进深圳口岸（声明交出上海、接收深圳）一拍之后，上海那一份转出、深圳立 HANDOVER_IN；深圳口岸交给干线
// 承运商（声明交出深圳、去向新加坡，不声明接收）一拍之后，深圳那一份按同一份依据转出、转入零写入；承运商交进新加坡枢纽（只声明接收）
// 一拍之后，转出一侧入账「无在身控制」、新加坡立 HANDOVER_IN。每一封都 PUBLISHED、NO 名下各一行 inbox；最后在身的只有新加坡那一份。
func TestAParcelsNodeControlFollowsItsHandoversThroughTheProductionRoute(t *testing.T) {
	beat, db, store := wiredBeat(t)
	received := receiveIntoControl(t, db, "SYN-HU-62-A", "SYN-PARCEL-62-A")
	steps := []struct {
		command tfapplication.RegisterTransportHandoverCommand
		scope   string
	}{
		{scope: "SYN-SCOPE-62-SZX", command: declaredHandover("SYN-PARCEL-62-A", "SYN-SCOPE-62-SZX", "SYN-HO-62-A/v1",
			inShanghaiHub, inShenzhenGW, "", nodeControlReceivedAt.Add(3*time.Hour))},
		{scope: "SYN-SCOPE-62-TRUNK", command: declaredHandover("SYN-PARCEL-62-A", "SYN-SCOPE-62-TRUNK", "SYN-HO-62-B/v1",
			inShenzhenGW, "", inSingaporeHB, nodeControlReceivedAt.Add(9*time.Hour))},
		{scope: "SYN-SCOPE-62-SIN", command: declaredHandover("SYN-PARCEL-62-A", "SYN-SCOPE-62-SIN", "SYN-HO-62-C/v1",
			"", inSingaporeHB, "", nodeControlReceivedAt.Add(20*time.Hour))},
	}
	for _, step := range steps {
		registerDeclared(t, db, store, step.command)
		if _, err := beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("一拍（%s）：%v", step.command.Version, err)
		}
		eventID := nodeControlEventID("SYN-PARCEL-62-A", step.scope, step.command.Version)
		if status := outboxStatus(t, db, eventID); status != "PUBLISHED" {
			t.Fatalf("%s 交接信封 status = %s，失败码 = %q", step.command.Version, status, recordedFailureCode(t, db, eventID))
		}
		if n := inboxRowsOf(t, db, nodeControlConsumerName, eventID); n != 1 {
			t.Fatalf("%s：NO 名下 inbox 行 = %d, want 1", step.command.Version, n)
		}
	}

	intoGateway := controlTransitionsOf(t, db, "SYN-HO-62-A/v1")
	if out, in := intoGateway["OUT"], intoGateway["IN"]; out.Outcome != "ADVANCED" || out.Node != inShanghaiHub ||
		out.MismatchedReleasingNode != "" || in.Outcome != "ADVANCED" || in.Node != inShenzhenGW || in.ControlKind != "HANDOVER_IN" {
		t.Fatalf("进深圳口岸那一次 = %+v", intoGateway)
	}
	toCarrier := controlTransitionsOf(t, db, "SYN-HO-62-B/v1")
	if out, transferredIn := toCarrier["OUT"], toCarrier["IN"]; out.Outcome != "ADVANCED" || out.Node != inShenzhenGW ||
		out.ControlKind != "HANDOVER_IN" || out.ControlBasis != intoGateway["IN"].ControlBasis || transferredIn.Direction != "" {
		t.Fatalf("交给干线承运商那一次 = %+v, want 深圳那一份转出、不转入", toCarrier)
	}
	intoHub := controlTransitionsOf(t, db, "SYN-HO-62-C/v1")
	if out, in := intoHub["OUT"], intoHub["IN"]; out.Reason != "NO_ACTIVE_CONTROL" || in.Outcome != "ADVANCED" || in.Node != inSingaporeHB {
		t.Fatalf("交进新加坡枢纽那一次 = %+v", intoHub)
	}

	active := activeControlsOf(t, db, "SYN-HU-62-A")
	if len(active) != 1 || active[0].Node().String() != inSingaporeHB || active[0].EstablishmentKind() != nodomain.EstablishedByHandoverIn {
		t.Fatalf("在身控制 = %v, want 只有新加坡枢纽那一份", active)
	}
	if reference, _, released := readReception(t, db, received).Control.Release(); !released ||
		reference.String() != "TRANSPORT-HANDOVER/SYN-HO-62-A/v1" {
		t.Fatalf("上海收寄那一份 released=%v %q", released, reference)
	}
}

// Covers: `AT-NO-117`（ADR-0214 决定七）——交进节点的包裹在本租户册上没有关联到任何作业实物：转入一侧入账「无关联作业实物」，收寄册
// 一行不添——转入不造待识别实物。
func TestATransferInWithoutAUnitRecordsItAndCreatesNoUnit(t *testing.T) {
	fixture := newNodeControlLane(t)
	registerDeclared(t, fixture.db, fixture.store, declaredHandover("SYN-PARCEL-62-B", "SYN-SCOPE-62-SIN", "SYN-HO-62-D/v1",
		"", inSingaporeHB, "", nodeControlReceivedAt.Add(20*time.Hour)))

	if err := fixture.deliver("SYN-PARCEL-62-B", "SYN-SCOPE-62-SIN", "SYN-HO-62-D/v1"); err != nil {
		t.Fatalf("投递：%v", err)
	}
	rows := controlTransitionsOf(t, fixture.db, "SYN-HO-62-D/v1")
	if in := rows["IN"]; in.Outcome != "NOT_ADVANCED" || in.Reason != "NO_ASSOCIATED_UNIT" || in.Unit != "" {
		t.Fatalf("转入一侧 = %+v", rows)
	}
	var receptions int
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := querier.QueryRow(t.Context(), `SELECT count(*) FROM node_operations.reception WHERE tenant_id = $1`,
		nodeControlTenant).Scan(&receptions); err != nil || receptions != 0 {
		t.Fatalf("收寄册 %d 行 err=%v, want 0——转入不造实物", receptions, err)
	}
}

// Covers: `AT-NO-122`（ADR-0214 决定五、七）——节点之间直接交接推进了两半之后被更正成拒收：更正版的信封在两个方向各入账「控制来源链
// 待重算」，转出一侧带先前那次转出，转入一侧带先前立起的那一份；深圳口岸那一份照旧在身，上海那一份不回退。
func TestAWithdrawingCorrectionOfATransferInLeavesTheControlInPlace(t *testing.T) {
	fixture := newNodeControlLane(t)
	received := receiveIntoControl(t, fixture.db, "SYN-HU-62-C", "SYN-PARCEL-62-C")
	registerDeclared(t, fixture.db, fixture.store, declaredHandover("SYN-PARCEL-62-C", "SYN-SCOPE-62-SZX", "SYN-HO-62-E/v1",
		inShanghaiHub, inShenzhenGW, "", nodeControlReceivedAt.Add(3*time.Hour)))
	if err := fixture.deliver("SYN-PARCEL-62-C", "SYN-SCOPE-62-SZX", "SYN-HO-62-E/v1"); err != nil {
		t.Fatalf("投递前版：%v", err)
	}
	register := newHandoverRegistration(t, fixture.db, fixture.store)
	var corrected tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, fixture.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		corrected, err = register.Correct(txCtx, tfapplication.CorrectTransportHandoverCommand{
			TenantID: mustTF(t, tfdomain.NewTenantID, nodeControlTenant), Object: "SYN-PARCEL-62-C", Scope: "SYN-SCOPE-62-SZX",
			PredecessorVersion: "SYN-HO-62-E/v1", Verdict: tfdomain.HandoverRefused, Basis: "SYN-GATE-DISPUTE-62",
			NewVersion: "SYN-HO-62-E/v2", CorrectedAt: nodeControlReceivedAt.Add(4 * time.Hour),
		})
		return err
	})
	if corrected.Outcome() != tfapplication.HandoverCorrected {
		t.Fatalf("更正：%s", corrected.Outcome())
	}

	if err := fixture.deliver("SYN-PARCEL-62-C", "SYN-SCOPE-62-SZX", "SYN-HO-62-E/v2"); err != nil {
		t.Fatalf("投递更正版：%v", err)
	}
	rows := controlTransitionsOf(t, fixture.db, "SYN-HO-62-E/v2")
	if out, in := rows["OUT"], rows["IN"]; out.Reason != "CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE" ||
		out.TransferReference != "TRANSPORT-HANDOVER/SYN-HO-62-E/v1" || in.Reason != "CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE" ||
		in.Node != inShenzhenGW || in.ControlKind != "HANDOVER_IN" || in.TransferReference != "" {
		t.Fatalf("更正版 = %+v", rows)
	}
	if active := activeControlsOf(t, fixture.db, "SYN-HU-62-C"); len(active) != 1 || active[0].Node().String() != inShenzhenGW {
		t.Fatalf("在身控制 = %v, want 深圳口岸那一份照旧在身", active)
	}
	if readReception(t, fixture.db, received).Control.Active() {
		t.Fatal("撤回控制的更正把上海那一份回退了")
	}
}

// Covers: `AT-NO-124`（ADR-0214 决定四、七）——同一件实物两份交进不同节点的`已交接`并发到达：实物锁把两路串起来，不论谁先到，最后在身
// 的只有一份，是业务时间晚的那一次在它的接收节点立的。
func TestConcurrentHandoversIntoNodesLeaveOneControl(t *testing.T) {
	fixture := newNodeControlLane(t)
	receiveIntoControl(t, fixture.db, "SYN-HU-62-F", "SYN-PARCEL-62-F")
	registerDeclared(t, fixture.db, fixture.store, declaredHandover("SYN-PARCEL-62-F", "SYN-SCOPE-62-OUT", "SYN-HO-62-F/v1",
		inShanghaiHub, "", "", nodeControlReceivedAt.Add(time.Hour)))
	if err := fixture.deliver("SYN-PARCEL-62-F", "SYN-SCOPE-62-OUT", "SYN-HO-62-F/v1"); err != nil {
		t.Fatalf("先交给运输方：%v", err)
	}
	arrivals := []struct{ scope, version, node string }{
		{"SYN-SCOPE-62-EARLY", "SYN-HO-62-G/v1", inShenzhenGW},
		{"SYN-SCOPE-62-LATE", "SYN-HO-62-H/v1", inSingaporeHB},
	}
	for index, arrival := range arrivals {
		registerDeclared(t, fixture.db, fixture.store, declaredHandover("SYN-PARCEL-62-F", arrival.scope, arrival.version,
			"", arrival.node, "", nodeControlReceivedAt.Add(time.Duration(5+index)*time.Hour)))
	}

	var wait sync.WaitGroup
	errs := make([]error, len(arrivals))
	for index, arrival := range arrivals {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs[index] = fixture.deliver("SYN-PARCEL-62-F", arrival.scope, arrival.version)
		}()
	}
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("投递 %s：%v", arrivals[index].version, err)
		}
	}
	active := activeControlsOf(t, fixture.db, "SYN-HU-62-F")
	if len(active) != 1 || active[0].Node().String() != inSingaporeHB {
		t.Fatalf("在身控制 = %v, want 只有业务时间晚的那一次立在新加坡的那一份", active)
	}
}
