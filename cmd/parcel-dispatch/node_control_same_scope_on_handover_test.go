package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证 NO 认交接更正链只按 TF 的更正回指（ADR-0220）：交接经 TF 生产登记与更正编排落在真交接册上，派发一拍把
// 信封交给 NO 那一路，NO 经消费侧适配器沿真交接册上的回指上溯前版。场景照演示动线重走（psb/05 停点一）的那件包裹：交进深圳口岸与
// 从深圳口岸交给干线承运商两次交接的范围写成同一个。夹具全部合成，只记 `S`。

// Covers: `AT-NO-126`（ADR-0220）、`AT-NO-108` 的派发面——生产路由表上，同一范围 `SZX` 下先交进深圳口岸、再从深圳口岸交给干线承运商，
// 两次都是首登、各自推进控制：进口岸那一次在深圳立 HANDOVER_IN，交出那一次转出的就是这一份、不记信息格；此后交进新加坡枢纽的那一次
// 转出一侧入账「无在身控制」——深圳那一份不留到错的那次交接去转出。交出那一次随后经 TF 更正口更正、仍`已交接`：更正版的信封零写入，
// 两个方向都不落行。
func TestHandoversUnderOneScopeEachAdvanceControlThroughTheProductionRoute(t *testing.T) {
	beat, db, store := wiredBeat(t)
	received := receiveIntoControl(t, db, "SYN-HU-95-A", "SYN-PARCEL-95-A")
	const parcel, gatewayScope = "SYN-PARCEL-95-A", "SZX"
	steps := []tfapplication.RegisterTransportHandoverCommand{
		declaredHandover(parcel, "SYN-SCOPE-95-SHA", "SYN-HO-95-H1/v1", inShanghaiHub, "", inShenzhenGW,
			nodeControlReceivedAt.Add(2*time.Hour)),
		declaredHandover(parcel, gatewayScope, "SYN-HO-95-H2/v1", "", inShenzhenGW, "", nodeControlReceivedAt.Add(5*time.Hour)),
		declaredHandover(parcel, gatewayScope, "SYN-HO-95-H3/v1", inShenzhenGW, "", inSingaporeHB, nodeControlReceivedAt.Add(9*time.Hour)),
		declaredHandover(parcel, "SIN", "SYN-HO-95-H4/v1", "", inSingaporeHB, "", nodeControlReceivedAt.Add(20*time.Hour)),
	}
	for _, command := range steps {
		registerDeclared(t, db, store, command)
		dispatchHandoverOf(t, beat, db, parcel, command.Scope, command.Version)
	}

	intoGateway := controlTransitionsOf(t, db, "SYN-HO-95-H2/v1")
	if out, in := intoGateway["OUT"], intoGateway["IN"]; out.Reason != "NO_ACTIVE_CONTROL" ||
		in.Outcome != "ADVANCED" || in.Node != inShenzhenGW || in.ControlKind != "HANDOVER_IN" {
		t.Fatalf("交进深圳口岸那一次 = %+v", intoGateway)
	}
	outOfGateway := controlTransitionsOf(t, db, "SYN-HO-95-H3/v1")
	if out, transferredIn := outOfGateway["OUT"], outOfGateway["IN"]; out.Outcome != "ADVANCED" || out.Node != inShenzhenGW ||
		out.ControlBasis != intoGateway["IN"].ControlBasis || out.TransferReference != "TRANSPORT-HANDOVER/SYN-HO-95-H3/v1" ||
		out.MismatchedReleasingNode != "" || transferredIn.Direction != "" {
		t.Fatalf("从深圳口岸交出那一次 = %+v, want 转出同一范围下交进那一次立起的那一份、不转入", outOfGateway)
	}
	intoHub := controlTransitionsOf(t, db, "SYN-HO-95-H4/v1")
	if out, in := intoHub["OUT"], intoHub["IN"]; out.Outcome != "NOT_ADVANCED" || out.Reason != "NO_ACTIVE_CONTROL" ||
		in.Outcome != "ADVANCED" || in.Node != inSingaporeHB {
		t.Fatalf("交进新加坡枢纽那一次 = %+v, want 转出一侧无在身控制可转", intoHub)
	}
	if reference, _, released := readReception(t, db, received).Control.Release(); !released ||
		reference.String() != "TRANSPORT-HANDOVER/SYN-HO-95-H1/v1" {
		t.Fatalf("上海收寄那一份 released=%v %q", released, reference)
	}

	correctHandedOver(t, db, store, tfapplication.CorrectTransportHandoverCommand{
		Object: parcel, Scope: gatewayScope, PredecessorVersion: "SYN-HO-95-H3/v1", NewVersion: "SYN-HO-95-H3/v2",
		CorrectedAt: nodeControlReceivedAt.Add(21 * time.Hour),
	}, inShenzhenGW, inSingaporeHB)
	dispatchHandoverOf(t, beat, db, parcel, gatewayScope, "SYN-HO-95-H3/v2")
	if rows := controlTransitionsOf(t, db, "SYN-HO-95-H3/v2"); len(rows) != 0 {
		t.Fatalf("仍`已交接`的更正版 = %+v, want 两个方向都零写入", rows)
	}

	active := activeControlsOf(t, db, "SYN-HU-95-A")
	if len(active) != 1 || active[0].Node().String() != inSingaporeHB || active[0].EstablishmentKind() != nodomain.EstablishedByHandoverIn {
		t.Fatalf("在身控制 = %v, want 只有新加坡枢纽那一份", active)
	}
}

// dispatchHandoverOf 跑一拍，断言那一版的交接信封已发布、NO 名下恰一行 inbox。
func dispatchHandoverOf(t *testing.T, beat Beat, db *bentopg.DB, parcel, scope, version string) {
	t.Helper()
	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("一拍（%s）：%v", version, err)
	}
	eventID := nodeControlEventID(parcel, scope, version)
	if status := outboxStatus(t, db, eventID); status != "PUBLISHED" {
		t.Fatalf("%s 交接信封 status = %s，失败码 = %q", version, status, recordedFailureCode(t, db, eventID))
	}
	if n := inboxRowsOf(t, db, nodeControlConsumerName, eventID); n != 1 {
		t.Fatalf("%s：NO 名下 inbox 行 = %d, want 1", version, n)
	}
}

// correctHandedOver 经 TF 生产更正编排把一版交接更正成仍`已交接`：更正承继前版的节点声明，所以门禁读数照前版的交出与去向节点问。
func correctHandedOver(
	t *testing.T, db *bentopg.DB, store *outbox.Store, command tfapplication.CorrectTransportHandoverCommand, releasing, onward string,
) {
	t.Helper()
	command.TenantID = mustTF(t, tfdomain.NewTenantID, nodeControlTenant)
	command.Verdict = tfdomain.ObjectHandedOver
	command.ReleasingEvidence, command.ReceivingEvidence, command.Rule = "SYN-LOAD-SCAN-95", "SYN-RECEIPT-ACK-95", "SYN-HANDOVER-RULE/v1"
	register := newHandoverRegistration(t, db, store)
	var result tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = register.CorrectWithGateReading(txCtx, command, declaredGateReading(t, command.Object, releasing, onward))
		return err
	})
	if result.Outcome() != tfapplication.HandoverCorrected {
		t.Fatalf("更正 %s：%s", command.NewVersion, result.Outcome())
	}
}
