package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

var sealedAt = time.Date(2026, 8, 9, 15, 0, 0, 0, time.UTC)

func openUnit(t *testing.T) *domain.ConsolidationUnit {
	t.Helper()
	unit, err := domain.OpenConsolidationUnit(
		mustValue(t, domain.NewConsolidationUnitID, "bag-instance-1"),
		mustValue(t, domain.NewCarrierAssetReference, "bag-asset-7"),
	)
	if err != nil {
		t.Fatalf("open consolidation unit: %v", err)
	}
	return unit
}

func addMember(t *testing.T, unit *domain.ConsolidationUnit, id string) {
	t.Helper()
	if err := unit.AddMember(mustValue(t, domain.NewHandlingUnitID, id)); err != nil {
		t.Fatalf("add member %q: %v", id, err)
	}
}

// Covers: NO CONTEXT「封装冻结当时成员快照。封装后的成员变化必须依次保留开封、成员
// 变化和重新封装事实，并形成新封签记录；任何历史快照和封签都不得被新版本覆盖」——
// 封装态加减成员拒（必须先开封）；开封-变更-重封产生第二份快照，第一份原样在列；
// 空单元封不出快照。
func TestSealingFreezesSnapshotsAndNeverOverwritesThem(t *testing.T) {
	unit := openUnit(t)
	addMember(t, unit, "unit-1")
	addMember(t, unit, "unit-2")

	if err := unit.Seal(
		mustValue(t, domain.NewSealReference, "seal-1"),
		mustValue(t, domain.NewWorkBasisReference, "PACK-TASK/7"),
		sealedAt,
	); err != nil {
		t.Fatalf("seal: %v", err)
	}

	if err := unit.AddMember(mustValue(t, domain.NewHandlingUnitID, "unit-3")); !errors.Is(err, domain.ErrUnitSealed) {
		t.Fatalf("err = %v; 封装后改变成员必须先开封", err)
	}

	if err := unit.Unseal(mustValue(t, domain.NewWorkBasisReference, "UNPACK-TASK/8"), sealedAt.Add(time.Hour)); err != nil {
		t.Fatalf("unseal: %v", err)
	}
	if err := unit.RemoveMember(mustValue(t, domain.NewHandlingUnitID, "unit-2")); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if err := unit.Seal(
		mustValue(t, domain.NewSealReference, "seal-2"),
		mustValue(t, domain.NewWorkBasisReference, "REPACK-TASK/9"),
		sealedAt.Add(2*time.Hour),
	); err != nil {
		t.Fatalf("reseal: %v", err)
	}

	snapshots := unit.Snapshots()
	if len(snapshots) != 2 {
		t.Fatalf("snapshots = %d, want 2；重新封装创建新快照不覆盖原快照", len(snapshots))
	}
	if len(snapshots[0].Members()) != 2 || snapshots[0].Seal().String() != "seal-1" {
		t.Fatalf("first snapshot = %#v; 原快照被改写了", snapshots[0])
	}
	if len(snapshots[1].Members()) != 1 || snapshots[1].Seal().String() != "seal-2" {
		t.Fatalf("second snapshot = %#v", snapshots[1])
	}

	empty := openUnit(t)
	if err := empty.Seal(
		mustValue(t, domain.NewSealReference, "seal-9"),
		mustValue(t, domain.NewWorkBasisReference, "PACK-TASK/9"),
		sealedAt,
	); !errors.Is(err, domain.ErrInvalidConsolidation) {
		t.Fatalf("err = %v; 空单元冻结不出任何关系", err)
	}
}

// Covers: NO CONTEXT「全部成员已经移出，或剩余成员已通过明确处置转移后，实例才能
// 显式终局关闭。已关闭集运单元实例永久终局，不得重开、清空后复用或承载新成员」——
// 带成员且无处置转移关不了；关闭后一切操作拒；封装态先开封再关。
func TestClosureIsExplicitAndPermanent(t *testing.T) {
	unit := openUnit(t)
	addMember(t, unit, "unit-1")

	if err := unit.Close(domain.WorkBasisReference{}, sealedAt); !errors.Is(err, domain.ErrMembersStillContained) {
		t.Fatalf("err = %v; 带成员且无处置转移关掉了", err)
	}
	if err := unit.Close(
		mustValue(t, domain.NewWorkBasisReference, "DISPOSITION-TRANSFER/3"),
		sealedAt,
	); err != nil {
		t.Fatalf("close with disposition: %v", err)
	}
	if !unit.Closed() || unit.ClosedAt().IsZero() {
		t.Fatal("关闭没有落到实例上")
	}

	if err := unit.AddMember(mustValue(t, domain.NewHandlingUnitID, "unit-9")); !errors.Is(err, domain.ErrUnitClosed) {
		t.Fatalf("err = %v; 已关闭实例承载了新成员", err)
	}
	if err := unit.Seal(
		mustValue(t, domain.NewSealReference, "seal-9"),
		mustValue(t, domain.NewWorkBasisReference, "PACK/9"),
		sealedAt.Add(time.Hour),
	); !errors.Is(err, domain.ErrUnitClosed) {
		t.Fatalf("err = %v; 已关闭实例被重新封装了", err)
	}
	if err := unit.Close(
		mustValue(t, domain.NewWorkBasisReference, "DISPOSITION/again"),
		sealedAt.Add(time.Hour),
	); !errors.Is(err, domain.ErrUnitClosed) {
		t.Fatalf("err = %v; 终局关了两次", err)
	}

	emptied := openUnit(t)
	addMember(t, emptied, "unit-1")
	if err := emptied.RemoveMember(mustValue(t, domain.NewHandlingUnitID, "unit-1")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := emptied.Close(domain.WorkBasisReference{}, sealedAt); err != nil {
		t.Fatalf("close emptied: %v", err)
	}
}

// Covers: 成员纪律——重复移入拒（本单元侧的单父级防线；跨单元唯一性由收纳编排凭
// 仓储视野核对）；移出不在册的成员拒；封装态移出拒。
func TestMembershipStaysDisciplined(t *testing.T) {
	unit := openUnit(t)
	addMember(t, unit, "unit-1")

	if err := unit.AddMember(mustValue(t, domain.NewHandlingUnitID, "unit-1")); !errors.Is(err, domain.ErrInvalidConsolidation) {
		t.Fatalf("err = %v; 同一实物移入了两次", err)
	}
	if err := unit.RemoveMember(mustValue(t, domain.NewHandlingUnitID, "unit-9")); !errors.Is(err, domain.ErrInvalidConsolidation) {
		t.Fatalf("err = %v; 移出了不在册的成员", err)
	}

	if err := unit.Seal(
		mustValue(t, domain.NewSealReference, "seal-1"),
		mustValue(t, domain.NewWorkBasisReference, "PACK/1"),
		sealedAt,
	); err != nil {
		t.Fatalf("seal: %v", err)
	}
	if err := unit.RemoveMember(mustValue(t, domain.NewHandlingUnitID, "unit-1")); !errors.Is(err, domain.ErrUnitSealed) {
		t.Fatalf("err = %v; 封装态移出了成员", err)
	}
	if err := unit.Unseal(mustValue(t, domain.NewWorkBasisReference, "UNPACK/2"), sealedAt.Add(time.Hour)); err != nil {
		t.Fatalf("unseal: %v", err)
	}
	unsealed := openUnit(t)
	if err := unsealed.Unseal(mustValue(t, domain.NewWorkBasisReference, "UNPACK/3"), sealedAt); !errors.Is(err, domain.ErrUnitNotSealed) {
		t.Fatalf("err = %v; 未封装开了封", err)
	}
}
