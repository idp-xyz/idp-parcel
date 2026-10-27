package postgres_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件在 cancellation_boundary_race_test.go 的夹具上证冲突解开决定（ADR-0295 决定三）与同一包裹上并发到达的网络服务有效交付依次走：决定改的是「这件包裹有没有
// 有效取消」，有效交付的终局采用正是在包裹取消边界锁里读它。真库、真边界锁、真取消库、真终局库、真冲突册与真决定留痕；屏障与等锁的看法同 cancellation_boundary_race_test.go。
//
// 两个先后各证一遍：
//   - 决定先手：停在作废之后、提交之前；交付后手到达，要被锁挡住，等决定提交之后读到「取消已不成立」，形成终局，不留依据那份取消的不采用行。
//   - 交付先手：读完有效取消（还在册）后停住；决定后手到达，要被锁挡住，等交付提交之后作废它落下的不采用行。
//
// 没有锁时两个先后各有各的坏结局：前者交付读到仍在册的取消、落下一份依据已不成立的取消的不采用行，决定的作废早已跑完、碰不到它；后者决定在交付落记录之前
// 作废了零行，交付随后落下依据已不成立的取消的不采用行。两种都是永远没有人再作废、续办也无从重驱的卡死行。

// gatedVoider 在作废做完之后停在屏障上：此时决定留痕与作废都已写入、还没提交，锁还握着。
type gatedVoider struct {
	ports.CancellationRefusedFinalVoider
	gate *boundaryGate
}

func (voider gatedVoider) VoidCancellationRefusedFinals(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
	cancellation domain.ParcelCancellationID,
) (int, error) {
	voided, err := voider.CancellationRefusedFinalVoider.VoidCancellationRefusedFinals(ctx, tenant, parcel, cancellation)
	voider.gate.pass()
	return voided, err
}

// decide 装冲突决定编排，对册上这件包裹的唯一一份冲突认定取消依据不成立；gated 为真时屏障装在作废之后。
func (rig *boundaryRig) decide(t *testing.T, gated bool, into *application.DecideCancellationOrderConflictResult) func(context.Context) error {
	t.Helper()
	decisions, err := adapter.NewCancellationOrderConflictDecisions(rig.db)
	if err != nil {
		t.Fatalf("构造决定留痕：%v", err)
	}
	store, err := outbox.NewStore(rig.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxCancellationNotUpheldHandoff(rig.db, store, boundaryClock{})
	if err != nil {
		t.Fatalf("构造续办信封口：%v", err)
	}
	var voider ports.CancellationRefusedFinalVoider = rig.finals
	if gated {
		voider = gatedVoider{CancellationRefusedFinalVoider: rig.finals, gate: rig.gate}
	}
	handler := application.NewDecideCancellationOrderConflictHandler(application.DecideCancellationOrderConflictDeps{
		Conflicts:    rig.conflicts,
		Decisions:    decisions,
		Voider:       voider,
		Handoff:      handoff,
		BoundaryLock: rig.locks,
		Clock:        boundaryClock{},
	})
	entries := rig.conflictsOnParcel(t)
	if len(entries) != 1 {
		t.Fatalf("册上的冲突 %d 行, want 恰一行（前置：包裹先停在取消先后冲突上）", len(entries))
	}
	command := application.DecideCancellationOrderConflictCommand{
		Tenant:       entries[0].TenantID,
		Parcel:       entries[0].Parcel,
		BoundaryKind: entries[0].Boundary.Kind,
		BoundaryRef:  entries[0].Boundary.Reference,
		Cancellation: entries[0].Cancellation,
		Kind:         domain.CancellationNotUpheld,
		Decider:      mustBuild(t, domain.NewCancellationOrderConflictDeciderReference, "https://id.syn.example#alice"),
		Basis:        mustBuild(t, domain.NewCancellationOrderConflictDecisionBasis, "SYN-NODE-SCAN-CHECKED"),
	}
	return func(txCtx context.Context) error {
		result, err := handler.Handle(txCtx, command)
		*into = result
		return err
	}
}

// standByTheConflict 摆出一件被取消挡住的包裹：取消 PCXL-1 已成立，收寄发生在取消之前、后到，冲突入册；有效交付尚未到。
func (rig *boundaryRig) standByTheConflict(t *testing.T) {
	t.Helper()
	var cancelled application.CancelParcelResult
	var adopted application.AdoptNetworkIntakeResult
	rig.inTransaction(t, rig.cancel(t, false, &cancelled))
	rig.inTransaction(t, rig.adopt(t, false, &adopted))
	if cancelled.Outcome() != application.ParcelCancelled || adopted.UndecidedReason() != application.IntakeCancellationOrderConflict {
		t.Fatalf("前置：取消 = %s，采用 = %s/%s；包裹要先停在取消先后冲突上",
			cancelled.Outcome(), adopted.Outcome(), adopted.UndecidedReason())
	}
}

// Covers: AT-PS-169, AT-PS-173；ADR-0295 决定三——决定先手，作废之后、提交之前停住；有效交付的终局采用后手到达。失败形态：交付不排队，径直读到仍在册的
// 取消，落一份依据 PARCEL_CANCELLATION/PCXL-1 的不采用行并提交，而决定的作废早已跑完、碰不到它，这条记录日后永远没有人作废。要的是后手被包裹取消边界锁挡住，
// 等决定提交之后读到「取消已不成立」，形成终局，不留不采用行。
func TestAConflictDecisionAheadOfAnEffectiveDeliveryIsSeenByTheFinalAdoption(t *testing.T) {
	rig := newBoundaryRig(t)
	rig.standByTheConflict(t)

	var decided application.DecideCancellationOrderConflictResult
	var delivered application.FormParcelFinalResult
	firstErr, secondErr := rig.race(t, rig.decide(t, true, &decided), rig.deliver(t, false, &delivered))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("决定的事务 → %v；交付采用的事务 → %v；两路都该提交", firstErr, secondErr)
	}
	if decided.Outcome() != application.CancellationOrderConflictDecided || decided.VoidedRefusals() != 0 {
		t.Fatalf("决定 = %s 作废 %d 行, want DECIDED 作废 0 行（交付还没落记录）", decided.Outcome(), decided.VoidedRefusals())
	}
	if delivered.Outcome() != application.ParcelFinalFormed {
		t.Fatalf("交付采用 = %s（依据 %q），want FINAL_FORMED——后手没读到先手已提交的决定，仍把一份不成立的取消当成有效取消",
			delivered.Outcome(), delivered.Basis())
	}
	if isCancelled, _, finalized := rig.standing(t); isCancelled || !finalized {
		t.Fatalf("落库：有效取消 %t、非取消终局 %t，want 取消已不是有效取消、终局已形成", isCancelled, finalized)
	}
	if record, found := rig.refusalOnParcel(t); !found || !record.Finalized {
		t.Fatalf("交付来源的采用记录 found = %t finalized = %t, want 一份终局，不是不采用行", found, record.Finalized)
	}
}

// Covers: AT-PS-169, AT-PS-173；ADR-0295 决定三——有效交付的终局采用先手，读完有效取消（还在册）后停住；冲突解开决定后手到达。失败形态：决定不排队，
// 在交付落记录之前就提交，作废了零行；交付随后照它读到的取消落下不采用行并提交，这条依据已不成立的取消的记录永远留在账上。要的是后手被锁挡住，等交付提交之后
// 作废它落下的那一行。
func TestAnEffectiveDeliveryAheadOfAConflictDecisionIsVoidedByTheDecision(t *testing.T) {
	rig := newBoundaryRig(t)
	rig.standByTheConflict(t)

	var delivered application.FormParcelFinalResult
	var decided application.DecideCancellationOrderConflictResult
	firstErr, secondErr := rig.race(t, rig.deliver(t, true, &delivered), rig.decide(t, false, &decided))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("交付采用的事务 → %v；决定的事务 → %v；两路都该提交", firstErr, secondErr)
	}
	if delivered.Outcome() != application.FinalSourceNotAdopted || delivered.Basis().String() != "PARCEL_CANCELLATION/PCXL-1" {
		t.Fatalf("交付采用 = %s（依据 %q），want SOURCE_NOT_ADOPTED 指名 PARCEL_CANCELLATION/PCXL-1（先手读到时取消仍在册）",
			delivered.Outcome(), delivered.Basis())
	}
	if decided.Outcome() != application.CancellationOrderConflictDecided || decided.VoidedRefusals() != 1 {
		t.Fatalf("决定 = %s 作废 %d 行, want DECIDED 作废 1 行——后手没读到先手已提交的不采用行", decided.Outcome(), decided.VoidedRefusals())
	}
	if _, found := rig.refusalOnParcel(t); found {
		t.Fatal("决定之后依据已不成立的取消的不采用行仍在账上")
	}
	if isCancelled, started, finalized := rig.standing(t); isCancelled || started || finalized {
		t.Fatalf("落库：有效取消 %t、责任起点 %t、非取消终局 %t，want 都没有（取消已不是有效取消，被挡住的来源等续办）", isCancelled, started, finalized)
	}
}
