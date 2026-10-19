package main

import (
	"encoding/json"
	"testing"
	"time"

	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件对真实 PostgreSQL 证同一作业实物在同一节点不另起控制起点（ADR-0199 决定四，票 no-reception-source/04）的生产装配面：
// 编排、收寄库、实物锁、意图交付与身份核对都是生产装配的。夹具全为合成（S 级）。

// sameUnitAgain 是同一件实物在同一节点的又一次送站：来源标识、接收观察与发生时刻换了，实物、节点与号码照原样。
func sameUnitAgain(
	t *testing.T,
	earlier nodeopsapp.ReceiveDeliveredUnitCommand,
	sourceID string,
	claim nodeopsapp.ReceptionClaimKind,
	after time.Duration,
) nodeopsapp.ReceiveDeliveredUnitCommand {
	t.Helper()
	command := receptionCommand(t, sourceID, claim)
	command.Unit, command.Node, command.Mark = earlier.Unit, earlier.Node, earlier.Mark
	command.OccurredAt = earlier.OccurredAt.Add(after)
	return command
}

// Covers: `AT-NO-049`「同一作业实物换来源，再次交到已有当前收寄的同一节点——并入那一条收寄并返回它的结果，这次的来源照样保全；
// 不另起控制起点、不重复发布」的真库面（ADR-0199 决定四）：委托带客户包裹件号建单并接受，一件实物贴着它经生产装配送站，
// 形成收寄、意图一封。同一件实物换来源再来明确接收、明确拒收、仅扫描：经 HTTP 端点各答 200 EXISTING_RESULT，`recordedOutcome`
// 是 INTAKE_FORMED、`currentVersion` 是那一版；册上各落一行并入，指名那一版、来源文档原样、不带收寄与控制；控制起点一个、
// 意图仍是一封。到了另一个节点照常形成收寄、另起那个节点的控制起点。首投与到另一个节点那一次各带那个节点的送站预报（ADR-0212
// 决定四：客户包裹件号的唯一识别要有可验证交付范围；预报只对它所列的节点算数，决定二）。
func TestTheWiredReceptionMergesTheSameUnitAtTheSameNode(t *testing.T) {
	pool, db, reception, receptions := wiredReceptionWithStore(t)
	acceptSubmitted(t, db, submitWithCustomerParcelNumber(t, db, "SYN-CUSTOMER-PARCEL-NRS04"))
	first := receptionCommand(t, "SYN-DELIVERY-NRS04-1", nodeopsapp.ExplicitReception)
	first.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-NRS04")
	first.Mark = noports.ExternalMarkObservation{Mark: "SYN-CUSTOMER-PARCEL-NRS04"}
	first.ScopeBasis = preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", "SYN-CUSTOMER-PARCEL-NRS04")
	formed, err := reception.Handle(t.Context(), first)
	if err != nil || formed.Outcome() != nodeopsapp.NodeIntakeFormed {
		t.Fatalf("首投 outcome = %v err = %v, want INTAKE_FORMED", formed.Outcome(), err)
	}
	existing, _ := formed.Record()
	controlStarts := func() int {
		return countRows(t, pool, `SELECT count(DISTINCT control ->> 'basis') FROM node_operations.reception
		  WHERE tenant_id = $1 AND control IS NOT NULL AND intake ->> 'unit' = $2`, first.TenantID.String(), first.Unit.String())
	}

	for index, delivery := range []struct {
		claim nodeopsapp.ReceptionClaimKind
		name  string
	}{{nodeopsapp.ExplicitReception, "RECEIVED"}, {nodeopsapp.ExplicitRefusal, "REFUSED"}, {nodeopsapp.ScanOnlyObservation, "SCAN_ONLY"}} {
		again := sameUnitAgain(t, first, "SYN-DELIVERY-NRS04-AGAIN-"+delivery.name, delivery.claim, time.Duration(index+1)*time.Hour)
		answer := postWiredReception(t, reception, again)
		var cells struct {
			RecordedOutcome string `json:"recordedOutcome"`
			CurrentVersion  string `json:"currentVersion"`
		}
		if err := json.Unmarshal([]byte(answer.raw), &cells); err != nil {
			t.Fatalf("解码响应 %q：%v", answer.raw, err)
		}
		if answer.status != 200 || answer.Outcome != "EXISTING_RESULT" || answer.Association != "syn-parcel-1" {
			t.Fatalf("%s 再来 %d %q 关联 %q, want 200 EXISTING_RESULT 带那一条收寄（%s）", delivery.name, answer.status, answer.Outcome, answer.Association, answer.raw)
		}
		if cells.RecordedOutcome != "INTAKE_FORMED" || cells.CurrentVersion != existing.JudgmentVersion.String() {
			t.Fatalf("%s 再来 recordedOutcome %q currentVersion %q, want INTAKE_FORMED %q", delivery.name, cells.RecordedOutcome, cells.CurrentVersion, existing.JudgmentVersion)
		}
		merged, found, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: again.TenantID, SourceID: again.SourceID})
		if err != nil || !found {
			t.Fatalf("读回 %s 的并入行：found=%v err=%v", delivery.name, found, err)
		}
		if merged.Kind != noports.RecordMergedIntoExistingIntake || merged.MergedInto != existing.JudgmentVersion ||
			merged.Source != commandSource(again, delivery.name) || merged.Control.Active() || !merged.Intake.ReceivedAt().IsZero() {
			t.Fatalf("%s 的并入行 = %+v, want 指名 %q、来源原样、不带收寄与控制", delivery.name, merged, existing.JudgmentVersion)
		}
	}
	if starts, intents := controlStarts(), nodeIntakeIntents(t, pool); starts != 1 || intents != 1 {
		t.Fatalf("控制起点 %d、意图 %d 封, want 各一", starts, intents)
	}

	elsewhere := sameUnitAgain(t, first, "SYN-DELIVERY-NRS04-ELSEWHERE", nodeopsapp.ExplicitReception, 4*time.Hour)
	elsewhere.Node = mustValue(t, nodomain.NewNodeReference, "SYN-NODE-NRS04-OTHER")
	elsewhere.ScopeBasis = preAdviseAtNodeOnRealDatabase(t, db, "SYN-NODE-NRS04-OTHER", "SYN-CUSTOMER-1", "SYN-CUSTOMER-PARCEL-NRS04")
	if result, err := reception.Handle(t.Context(), elsewhere); err != nil || result.Outcome() != nodeopsapp.NodeIntakeFormed {
		t.Fatalf("另一个节点 outcome = %v err = %v, want INTAKE_FORMED", result.Outcome(), err)
	}
	if starts, intents := controlStarts(), nodeIntakeIntents(t, pool); starts != 2 || intents != 2 {
		t.Fatalf("到另一个节点之后控制起点 %d、意图 %d 封, want 各二", starts, intents)
	}
}

// Covers: `AT-NO-050`「只有到站扫描的待确认之后，同一实物来了明确接收或拒收——追加一版判断续办原待确认，原待确认照留；不回头
// 把它改成确定结果」的真库面（ADR-0199 决定四）：仅扫描落下待确认，同一件实物换来源来明确接收，号码不在关联册里，答待识别、
// 建控制，新判断指名那条待确认为前版；待确认原行逐字不变。重投待确认的来源答已有结果，原判断种类 RECEPTION_UNDECIDED、当前
// 有效版本是新判断的版本。再来一次明确拒收，并入那条待识别，不另起判断。
func TestTheWiredReceptionContinuesAScanOnlyJudgment(t *testing.T) {
	pool, _, reception, receptions := wiredReceptionWithStore(t)
	scan := receptionCommand(t, "SYN-DELIVERY-NRS04-SCAN", nodeopsapp.ScanOnlyObservation)
	scan.Unit = mustValue(t, nodomain.NewHandlingUnitID, "SYN-UNIT-NRS04-SCAN")
	if result, err := reception.Handle(t.Context(), scan); err != nil || result.Outcome() != nodeopsapp.ReceptionUndecided {
		t.Fatalf("仅扫描 outcome = %v err = %v, want RECEPTION_UNDECIDED", result.Outcome(), err)
	}
	scanKey := noports.ReceptionKey{TenantID: scan.TenantID, SourceID: scan.SourceID}
	var before string
	if err := pool.QueryRow(t.Context(), `SELECT row_to_json(r)::text FROM node_operations.reception r WHERE tenant_id = $1 AND source_id = $2`,
		scan.TenantID.String(), scan.SourceID).Scan(&before); err != nil {
		t.Fatalf("读待确认原行：%v", err)
	}
	undecided, _, err := receptions.FindByKey(t.Context(), scanKey)
	if err != nil {
		t.Fatalf("读回待确认：%v", err)
	}

	received := sameUnitAgain(t, scan, "SYN-DELIVERY-NRS04-RECEIVED", nodeopsapp.ExplicitReception, time.Hour)
	result, err := reception.Handle(t.Context(), received)
	if err != nil || result.Outcome() != nodeopsapp.UnitPendingIdentification {
		t.Fatalf("明确接收 outcome = %v err = %v, want PENDING_IDENTIFICATION", result.Outcome(), err)
	}
	continued, _ := result.Record()
	if continued.Predecessor != undecided.JudgmentVersion || !continued.Control.Active() {
		t.Fatalf("新判断 前版 = %q 控制 = %v, want 以待确认 %q 为前版、建控制", continued.Predecessor, continued.Control.Active(), undecided.JudgmentVersion)
	}
	var after string
	if err := pool.QueryRow(t.Context(), `SELECT row_to_json(r)::text FROM node_operations.reception r WHERE tenant_id = $1 AND source_id = $2`,
		scan.TenantID.String(), scan.SourceID).Scan(&after); err != nil {
		t.Fatalf("再读待确认原行：%v", err)
	}
	if after != before {
		t.Fatalf("待确认原行被改了：\n前 %s\n后 %s", before, after)
	}

	replay, err := reception.Handle(t.Context(), scan)
	if err != nil || replay.Outcome() != nodeopsapp.ReceptionExistingResult {
		t.Fatalf("重投待确认 outcome = %v err = %v, want EXISTING_RESULT", replay.Outcome(), err)
	}
	if replay.RecordedOutcome() != nodeopsapp.ReceptionUndecided || replay.CurrentVersion() != continued.JudgmentVersion {
		t.Fatalf("重投待确认 原判断种类 %v 当前有效版本 %q, want RECEPTION_UNDECIDED %q", replay.RecordedOutcome(), replay.CurrentVersion(), continued.JudgmentVersion)
	}

	refused := sameUnitAgain(t, scan, "SYN-DELIVERY-NRS04-REFUSED", nodeopsapp.ExplicitRefusal, 2*time.Hour)
	merged, err := reception.Handle(t.Context(), refused)
	if err != nil || merged.Outcome() != nodeopsapp.ReceptionExistingResult || merged.MergedInto() != continued.JudgmentVersion {
		t.Fatalf("待识别之后再拒收 outcome = %v 并入 %q err = %v, want EXISTING_RESULT 并入 %q", merged.Outcome(), merged.MergedInto(), err, continued.JudgmentVersion)
	}
}
