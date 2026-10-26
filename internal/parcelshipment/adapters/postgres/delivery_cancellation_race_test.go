package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件在 cancellation_boundary_race_test.go 的夹具上补网络服务有效交付的终局采用（FormParcelFinalHandler 经
// AdoptOnEffectiveDeliveryAdapter 到达的那一路）：已成立的取消在册时，迟到的有效交付不形成非取消终局，不论取消是顺序在先、
// 并发在先，还是停在 CANCELLATION_ORDER_CONFLICT 上。并发两种先后各证一遍，办法同那份文件：屏障停在先手最后一道边界读之后，
// 后手要么已提交、要么经池外单独连接看 pg_locks 等在咨询锁上，才放先手走完。

var boundaryDeliveredAt = boundaryCancelledAt.Add(30 * time.Minute)

// deliver 装网络服务有效交付的终局采用；gated 为真时屏障装在它读已成立的取消（边界读）上。
func (rig *boundaryRig) deliver(t *testing.T, gated bool, into *application.FormParcelFinalResult) func(context.Context) error {
	t.Helper()
	var cancellations ports.ParcelCancellationView = rig.cancellations
	if gated {
		cancellations = gatedCancellations{ParcelCancellations: rig.cancellations, gate: rig.gate}
	}
	handler := application.NewFormParcelFinalHandler(application.FormParcelFinalDeps{
		Requests: rig.requests,
		Rules: boundaryFinalRules{judgment: ports.FinalRuleJudgment{
			Satisfied:   true,
			Kind:        mustBuild(t, domain.NewFinalKindReference, "NETWORK_SERVICE_DELIVERED"),
			RuleVersion: mustBuild(t, domain.NewFinalRuleVersionReference, "SYN-FINAL-RULE/1"),
		}},
		Finals:        rig.finals,
		Cancellations: cancellations,
		Identities:    boundaryFinalIDs{id: mustBuild(t, domain.NewFinalOutcomeVersionID, "FINAL-1")},
		Downstream:    boundaryHandoffs{},
		Clock:         boundaryClock{},
		BoundaryLock:  rig.locks,
	})
	command := application.FormParcelFinalCommand{
		Identity:          rig.identity,
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "request-1"),
		Outcome: domain.ResponsibilityOutcomeSpec{
			Kind:       domain.EffectiveDeliveryOutcome,
			Parcel:     rig.parcel,
			Decision:   mustBuild(t, domain.NewResponsibilityDecisionReference, "DELIVERY-JUDGMENT/parcel-1/attempt-1"),
			Execution:  mustBuild(t, domain.NewExecutionEvidenceReference, "POD/POD-1"),
			Version:    mustBuild(t, domain.NewResponsibilityOutcomeVersion, "delivery-result/v1"),
			OccurredAt: boundaryDeliveredAt,
		},
	}
	return func(txCtx context.Context) error {
		result, err := handler.Handle(txCtx, command)
		*into = result
		return err
	}
}

// inTransaction 在一笔环境事务里跑一路，顺序用例用它。
func (rig *boundaryRig) inTransaction(t *testing.T, run func(context.Context) error) {
	t.Helper()
	if err := rig.transactor.WithinTransaction(t.Context(), run); err != nil {
		t.Fatalf("事务失败：%v", err)
	}
}

// refusalOnParcel 读回这件包裹上有效交付来源那一份不采用记录。
func (rig *boundaryRig) refusalOnParcel(t *testing.T) (ports.FinalOutcomeRecord, bool) {
	t.Helper()
	record, found, err := rig.finals.FindByKey(t.Context(), ports.FinalAdoptionKey{
		TenantID: rig.tenant,
		Parcel:   rig.parcel,
		Kind:     domain.EffectiveDeliveryOutcome,
		Version:  mustBuild(t, domain.NewResponsibilityOutcomeVersion, "delivery-result/v1"),
	})
	if err != nil {
		t.Fatalf("读回有效交付的采用记录：%v", err)
	}
	return record, found
}

// Covers: `AT-PS-080`「取消不被覆盖」在有效交付一路上的对偶与 `AT-PS-107`（ADR-0254 决定二第 3 格）——取消先手，读完提交边界上的
// 事实后停住；有效交付的终局采用后手到达。失败形态：交付径直读到「没有取消」、形成非取消终局并提交，先手随后照它读到的提交取消，
// 一件包裹同时带着取消终局与非取消终局。要的是后手排在取消之后、读到已提交的取消：不采用，依据指名那份取消，不形成终局。
func TestACancellationAheadOfAnEffectiveDeliveryIsSeenByTheFinalAdoption(t *testing.T) {
	rig := newBoundaryRig(t)
	var cancelled application.CancelParcelResult
	var delivered application.FormParcelFinalResult
	firstErr, secondErr := rig.race(t, rig.cancel(t, true, &cancelled), rig.deliver(t, false, &delivered))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("取消的事务 → %v；交付采用的事务 → %v；两路都该提交", firstErr, secondErr)
	}
	if cancelled.Outcome() != application.ParcelCancelled {
		t.Fatalf("取消 = %s/%s，want PARCEL_CANCELLED", cancelled.Outcome(), cancelled.UndecidedReason())
	}
	if delivered.Outcome() != application.FinalSourceNotAdopted ||
		delivered.Basis().String() != "PARCEL_CANCELLATION/PCXL-1" {
		t.Fatalf("交付采用 = %s（依据 %q），want SOURCE_NOT_ADOPTED 指名 PARCEL_CANCELLATION/PCXL-1——后手没读到先手的取消",
			delivered.Outcome(), delivered.Basis())
	}
	if isCancelled, _, finalized := rig.standing(t); !isCancelled || finalized {
		t.Fatalf("落库：取消 %t、非取消终局 %t，want 只有取消", isCancelled, finalized)
	}
	if refusal, found := rig.refusalOnParcel(t); !found || refusal.Finalized {
		t.Fatalf("不采用记录 found = %t finalized = %t，want 来源保全在一份不采用记录上", found, refusal.Finalized)
	}
}

// Covers: `AT-PS-086` 的并发半边在有效交付一路上的对偶——有效交付的终局采用先手，读完已成立的取消（不在册）后停住；取消后手到达。
// 失败形态：取消径直读到「没有终局」、提交取消，先手随后照它读到的形成非取消终局。要的是后手读到先手已提交的终局、落第 1 格：
// 答既有终局并指名那一份，不落取消记录。
func TestAnEffectiveDeliveryFinalAheadOfACancellationIsReturnedAsTheStandingFinal(t *testing.T) {
	rig := newBoundaryRig(t)
	var delivered application.FormParcelFinalResult
	var cancelled application.CancelParcelResult
	firstErr, secondErr := rig.race(t, rig.deliver(t, true, &delivered), rig.cancel(t, false, &cancelled))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("交付采用的事务 → %v；取消的事务 → %v；两路都该提交", firstErr, secondErr)
	}
	if delivered.Outcome() != application.ParcelFinalFormed {
		t.Fatalf("交付采用 = %s/%s，want FINAL_FORMED", delivered.Outcome(), delivered.UndecidedReason())
	}
	if cancelled.Outcome() != application.CancellationFinalOutcomeStands ||
		cancelled.Basis().String() != "FINAL_OUTCOME/FINAL-1" {
		t.Fatalf("取消 = %s（依据 %q），want FINAL_OUTCOME_STANDS 指名 FINAL-1——后手没读到先手的终局",
			cancelled.Outcome(), cancelled.Basis())
	}
	if isCancelled, _, finalized := rig.standing(t); isCancelled || !finalized {
		t.Fatalf("落库：取消 %t、非取消终局 %t，want 只有终局", isCancelled, finalized)
	}
}

// Covers: `AT-PS-107`（ADR-0254 决定二第 3 格）「不形成责任起点也不形成非取消终局」——包裹停在 CANCELLATION_ORDER_CONFLICT 上
// （取消已提交，早于取消业务时间的收寄后到，冲突记进册）；此后到的有效交付不得绕过冲突形成终局：不采用，依据指名那份取消，
// 冲突册上那一行与已提交的取消原样不动。这是顺序用例，与并发无关。
func TestAnEffectiveDeliveryDoesNotGetPastACancellationOrderConflict(t *testing.T) {
	rig := newBoundaryRig(t)
	var cancelled application.CancelParcelResult
	var adopted application.AdoptNetworkIntakeResult
	var delivered application.FormParcelFinalResult
	rig.inTransaction(t, rig.cancel(t, false, &cancelled))
	rig.inTransaction(t, rig.adopt(t, false, &adopted))
	if cancelled.Outcome() != application.ParcelCancelled ||
		adopted.UndecidedReason() != application.IntakeCancellationOrderConflict {
		t.Fatalf("前置：取消 = %s，采用 = %s/%s；包裹要先停在取消先后冲突上",
			cancelled.Outcome(), adopted.Outcome(), adopted.UndecidedReason())
	}

	rig.inTransaction(t, rig.deliver(t, false, &delivered))

	if delivered.Outcome() != application.FinalSourceNotAdopted ||
		delivered.Basis().String() != "PARCEL_CANCELLATION/PCXL-1" {
		t.Fatalf("交付采用 = %s（依据 %q），want SOURCE_NOT_ADOPTED 指名 PARCEL_CANCELLATION/PCXL-1——交付绕过了停着的冲突",
			delivered.Outcome(), delivered.Basis())
	}
	if isCancelled, started, finalized := rig.standing(t); !isCancelled || started || finalized {
		t.Fatalf("落库：取消 %t、责任起点 %t、非取消终局 %t，want 只有取消", isCancelled, started, finalized)
	}
	if entries := rig.conflictsOnParcel(t); len(entries) != 1 {
		t.Fatalf("冲突册 = %+v，want 原来那一行原样在册", entries)
	}
}

// Covers: 不回归——没有取消的包裹，有效交付照常形成终局，重投答已有结果；取消随后到达答既有终局（第 1 格）。
func TestAnEffectiveDeliveryOnAParcelWithoutACancellationFormsTheFinalAsBefore(t *testing.T) {
	rig := newBoundaryRig(t)
	var delivered, replayed application.FormParcelFinalResult
	rig.inTransaction(t, rig.deliver(t, false, &delivered))
	rig.inTransaction(t, rig.deliver(t, false, &replayed))

	if delivered.Outcome() != application.ParcelFinalFormed {
		t.Fatalf("交付采用 = %s/%s，want FINAL_FORMED", delivered.Outcome(), delivered.UndecidedReason())
	}
	if replayed.Outcome() != application.ParcelFinalExistingResult {
		t.Fatalf("重投 = %s，want EXISTING_RESULT", replayed.Outcome())
	}
	if isCancelled, _, finalized := rig.standing(t); isCancelled || !finalized {
		t.Fatalf("落库：取消 %t、非取消终局 %t，want 只有终局", isCancelled, finalized)
	}

	var cancelled application.CancelParcelResult
	rig.inTransaction(t, rig.cancel(t, false, &cancelled))
	if cancelled.Outcome() != application.CancellationFinalOutcomeStands {
		t.Fatalf("终局之后的取消 = %s，want FINAL_OUTCOME_STANDS", cancelled.Outcome())
	}
}

// Covers: UC-PS-004「同一履约结果重复到达——返回原终局判断」（`AT-PS-061`）在排队之下仍成立——同一份有效交付并发重投，先手读完
// 已成立的取消后停住。排队若在幂等读之后，后手越过幂等读时先手还没落记录，进锁后读到先手已形成的终局，当成同源新版本去重派生
// 同一版本。要的是后手在幂等读之前就排上队，读到先手落下的记录，答已有结果，不走重派生。
func TestAConcurrentReplayOfOneEffectiveDeliveryAnswersTheExistingResult(t *testing.T) {
	rig := newBoundaryRig(t)
	var first, second application.FormParcelFinalResult
	firstErr, secondErr := rig.race(t, rig.deliver(t, true, &first), rig.deliver(t, false, &second))
	if firstErr != nil || secondErr != nil {
		t.Fatalf("先手的事务 → %v；后手的事务 → %v；两路都该提交", firstErr, secondErr)
	}
	if first.Outcome() != application.ParcelFinalFormed {
		t.Fatalf("先手 = %s/%s，want FINAL_FORMED", first.Outcome(), first.UndecidedReason())
	}
	if second.Outcome() != application.ParcelFinalExistingResult {
		t.Fatalf("后手 = %s，want EXISTING_RESULT——重投没读到先手落下的记录", second.Outcome())
	}
	if _, _, finalized := rig.standing(t); !finalized {
		t.Fatal("落库后没有终局")
	}
}
