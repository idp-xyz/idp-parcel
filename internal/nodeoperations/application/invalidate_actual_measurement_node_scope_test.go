package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证作废的节点范围：作业端设备只能作废在自己节点上登记的测量，手工作废不限节点；判断顺序是来源身份冲突、目标不在册、
// 目标在别的节点，然后才是已作废与重放。编排用内存登记册与记下变化通知的替身；夹具全为合成（S 级）。

const (
	scopeNodeHere  = "node-origin"
	scopeNodeThere = "node-other"
)

func scopeAtNode(t *testing.T, node string) application.MeasurementInvalidationScope {
	t.Helper()
	reference, err := domain.NewNodeReference(node)
	if err != nil {
		t.Fatal(err)
	}
	return application.InvalidationAtNode(reference)
}

func voidAtNode(t *testing.T, node, sourceID, target string) application.InvalidateActualMeasurementCommand {
	t.Helper()
	command := voidCommand(sourceID, target, "称错件")
	command.Scope = scopeAtNode(t, node)
	return command
}

// newScopeRig 备两条测量：measure-here 在 node-origin，measure-there 在 node-other。
func newScopeRig(t *testing.T) (*application.RecordActualMeasurementHandler, *memoryMeasurements, *recordedMeasurementChanges) {
	t.Helper()
	handler, store, notices := newSharedIdentityHandler(t)
	mustRecordMeasurement(t, handler, "measure-here")
	elsewhere := measurementWithSource("measure-there")
	elsewhere.Node = scopeNodeThere
	if result, err := handler.Handle(context.Background(), elsewhere); err != nil || result.Outcome() != application.ActualMeasurementRecorded {
		t.Fatalf("登记别的节点的测量 = %s %v", result.Outcome(), err)
	}
	return handler, store, notices
}

// Covers: 作业端设备作废别的节点登记的测量，答 TARGET_AT_ANOTHER_NODE：什么都不写、不发变化事件，两条测量都仍有效。
func TestADeviceCannotVoidAMeasurementRegisteredAtAnotherNode(t *testing.T) {
	handler, store, notices := newScopeRig(t)
	before := len(notices.changes)
	store.calls = nil

	result, err := handler.Invalidate(context.Background(), voidAtNode(t, scopeNodeHere, "void-1", "measure-there"))
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	if result.Outcome() != application.MeasurementInvalidationTargetAtAnotherNode || result.Outcome().String() != "TARGET_AT_ANOTHER_NODE" {
		t.Fatalf("作废结果 %s，want TARGET_AT_ANOTHER_NODE", result.Outcome())
	}
	if _, present := result.Invalidation(); present {
		t.Fatal("被拒的作废不该带出作废事实")
	}
	for _, call := range store.calls {
		if call == "save-invalidation" {
			t.Fatalf("别的节点的作废碰了写口：%v", store.calls)
		}
	}
	if len(store.invalidations) != 0 || len(store.voided) != 0 {
		t.Fatalf("别的节点的作废落了册：%d 条作废，%d 条被指", len(store.invalidations), len(store.voided))
	}
	if len(notices.changes) != before {
		t.Fatalf("变化通知 %d 封，want %d（被拒的作废不发）", len(notices.changes), before)
	}
	if ids := stillValidSourceIDs(t, store); !ids["measure-here"] {
		t.Fatalf("仍有效 = %v", ids)
	}
}

// Covers: 作废在自己节点上登记的测量成功，同一条作废原样重投答原结果。
func TestADeviceCanVoidAMeasurementRegisteredAtItsOwnNode(t *testing.T) {
	handler, store, notices := newScopeRig(t)
	before := len(notices.changes)

	result, err := handler.Invalidate(context.Background(), voidAtNode(t, scopeNodeHere, "void-1", "measure-here"))
	if err != nil || result.Outcome() != application.MeasurementInvalidationRecorded {
		t.Fatalf("作废自己节点的测量 = %s %v，want INVALIDATION_RECORDED", result.Outcome(), err)
	}
	if len(notices.changes) != before+1 {
		t.Fatalf("变化通知 %d 封，want %d", len(notices.changes), before+1)
	}
	if ids := stillValidSourceIDs(t, store); ids["measure-here"] {
		t.Fatalf("仍有效 = %v，want 作废之后离开清单", ids)
	}
	again, err := handler.Invalidate(context.Background(), voidAtNode(t, scopeNodeHere, "void-1", "measure-here"))
	if err != nil || again.Outcome() != application.MeasurementInvalidationExisting {
		t.Fatalf("原样重投 = %s %v，want EXISTING_RESULT", again.Outcome(), err)
	}
}

// Covers: 手工作废不限节点：持授予的操作者可以作废本租户任何节点上登记的测量。
func TestAManualInvalidationIsNotLimitedToANode(t *testing.T) {
	handler, store, _ := newScopeRig(t)
	for _, target := range []string{"measure-here", "measure-there"} {
		command := voidCommand("void-"+target, target, "称错件")
		command.Scope = application.InvalidationAcrossTenantNodes()
		result, err := handler.Invalidate(context.Background(), command)
		if err != nil || result.Outcome() != application.MeasurementInvalidationRecorded {
			t.Fatalf("手工作废 %s = %s %v，want INVALIDATION_RECORDED", target, result.Outcome(), err)
		}
	}
	if ids := stillValidSourceIDs(t, store); len(ids) != 0 {
		t.Fatalf("仍有效 = %v，want 空", ids)
	}
}

// Covers: 判断顺序：来源身份撞另一类 → SOURCE_CONFLICT；目标不在册 → TARGET_MISSING；目标在别的节点 → TARGET_AT_ANOTHER_NODE；
// 然后才是已作废与重放。别的节点的设备连「重放答原结果」也拿不到，自己节点的设备仍答原结果。
func TestTheNodeJudgmentComesAfterTheSourceAndTheTargetAndBeforeVoidAndReplay(t *testing.T) {
	handler, _, _ := newScopeRig(t)

	conflict, err := handler.Invalidate(context.Background(), voidAtNode(t, scopeNodeHere, "measure-here", "measure-there"))
	if err != nil || conflict.Outcome() != application.MeasurementInvalidationSourceConflict {
		t.Fatalf("借用测量的来源身份作废别的节点的测量 = %s %v，want SOURCE_CONFLICT", conflict.Outcome(), err)
	}
	missing, err := handler.Invalidate(context.Background(), voidAtNode(t, scopeNodeHere, "void-missing", "measure-nowhere"))
	if err != nil || missing.Outcome() != application.MeasurementInvalidationTargetMissing {
		t.Fatalf("作废不在册的测量 = %s %v，want TARGET_MISSING", missing.Outcome(), err)
	}

	manual := voidCommand("void-there", "measure-there", "称错件")
	manual.Scope = application.InvalidationAcrossTenantNodes()
	if result, err := handler.Invalidate(context.Background(), manual); err != nil || result.Outcome() != application.MeasurementInvalidationRecorded {
		t.Fatalf("手工作废 = %s %v", result.Outcome(), err)
	}

	otherNode := map[string]application.InvalidateActualMeasurementCommand{
		"别的节点的设备再作废一次（已作废）": voidAtNode(t, scopeNodeHere, "void-again", "measure-there"),
		"别的节点的设备重放那条作废":     voidAtNode(t, scopeNodeHere, "void-there", "measure-there"),
	}
	for name, command := range otherNode {
		result, err := handler.Invalidate(context.Background(), command)
		if err != nil || result.Outcome() != application.MeasurementInvalidationTargetAtAnotherNode {
			t.Fatalf("%s = %s %v，want TARGET_AT_ANOTHER_NODE", name, result.Outcome(), err)
		}
	}

	replay, err := handler.Invalidate(context.Background(), voidAtNode(t, scopeNodeThere, "void-there", "measure-there"))
	if err != nil || replay.Outcome() != application.MeasurementInvalidationExisting {
		t.Fatalf("自己节点的设备重放 = %s %v，want EXISTING_RESULT", replay.Outcome(), err)
	}
	again, err := handler.Invalidate(context.Background(), voidAtNode(t, scopeNodeThere, "void-again", "measure-there"))
	if err != nil || again.Outcome() != application.MeasurementInvalidationAlreadyVoid {
		t.Fatalf("自己节点的设备再作废一次 = %s %v，want ALREADY_VOID", again.Outcome(), err)
	}
}

// Covers: 漏传范围不会悄悄变成不限：命令没带范围（零值，或指名了一个空节点），编排以错误拒收，对册一次也不碰，也不答任何业务结果。
func TestAnInvalidationWithoutAScopeIsRefusedInsteadOfBeingUnrestricted(t *testing.T) {
	handler, store, notices := newScopeRig(t)
	before := len(notices.changes)
	store.calls = nil

	for name, scope := range map[string]application.MeasurementInvalidationScope{
		"没带范围":    {},
		"指名了空的节点": application.InvalidationAtNode(domain.NodeReference{}),
	} {
		command := voidCommand("void-unscoped", "measure-there", "称错件")
		command.Scope = scope
		result, err := handler.Invalidate(context.Background(), command)
		if !errors.Is(err, application.ErrMeasurementInvalidationScopeMissing) {
			t.Fatalf("%s：err = %v，want ErrMeasurementInvalidationScopeMissing", name, err)
		}
		if result.Outcome() != application.InvalidateActualMeasurementOutcomeInvalid {
			t.Fatalf("%s：答了业务结果 %s", name, result.Outcome())
		}
	}
	if len(store.calls) != 0 || len(store.invalidations) != 0 || len(notices.changes) != before {
		t.Fatalf("漏传范围的作废碰了册：calls %v，作废 %d，通知 %d 封（原 %d）", store.calls, len(store.invalidations), len(notices.changes), before)
	}
}
