package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件对真实 PostgreSQL 16 证 ParcelContainmentView 对未核对在袋件的答法（ADR-0199 决定五，`AT-NO-051`）：未核对的在袋件
// 可能是任何一件包裹，本上下文说不出是哪件，所以同租户里凡没有已关联实物在袋的包裹都答`不可归属`，不答`不在`。播种走写侧
// 适配器，与本包其余容纳用例同一做法。

// Covers: `AT-NO-051` 与 ADR-0199 决定五「同租户里有未核对的待识别实物此刻在未关闭单元里时，凡没有已关联实物在袋的包裹都答
// `不可归属`」——未核对那件没装袋时照常答`不在`；装袋之后，候选里没有、与它毫无关系的包裹也答`不可归属`。零候选的已核对待识别
// 装袋不改变答案：查过了、查无此号，那件不是任何一件包裹。
func TestAnUncheckedUnitInABagMakesEveryUnbaggedParcelUnattributable(t *testing.T) {
	fixture := newContainmentFixture(t)
	ctx := t.Context()
	saveReceptions(t, fixture.transactor, ctx, fixture.receptions,
		uncheckedPending(t, "tenant-a", "scan-unchecked", "SYN-INTAKEV-NRS05-UNCHECKED"))
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-9/link-v1"), ports.ParcelNotContained)

	fixture.bagWith(t, ctx, "tenant-a", "bag-1", "unit-2")
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-9/link-v1"), ports.ParcelContainmentUnattributable)
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelContainmentUnattributable)

	checked := newContainmentFixture(t)
	zero := recheckablePending(t, "tenant-a", "scan-checked", "SYN-INTAKEV-NRS05-CHECKED", "SYN-MARK-NRS05")
	zero.IdentityCheck = ports.IdentityChecked
	saveReceptions(t, checked.transactor, ctx, checked.receptions, zero)
	checked.bagWith(t, ctx, "tenant-a", "bag-1", "unit-2")
	assertContainment(t, checked.load(t, "tenant-a", "parcel-9/link-v1"), ports.ParcelNotContained)
}

// Covers: `AT-NO-051`「对已关联在袋的包裹仍答`在`」——已关联实物在袋压过未核对在袋件；已关联却没装袋的包裹，未核对那件可能就是
// 它，答`不可归属`。
func TestAnAssociatedUnitInABagStaysContainedBesideAnUncheckedUnit(t *testing.T) {
	fixture := newContainmentFixture(t)
	ctx := t.Context()
	saveReceptions(t, fixture.transactor, ctx, fixture.receptions,
		formedRecord(t, "tenant-a", "scan-formed"),
		uncheckedPending(t, "tenant-a", "scan-unchecked", "SYN-INTAKEV-NRS05-UNCHECKED"))
	fixture.bagWith(t, ctx, "tenant-a", "bag-1", "unit-2")
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelContainmentUnattributable)

	fixture.bagWith(t, ctx, "tenant-a", "bag-2", "unit-1")
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelContained)
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-9/link-v1"), ports.ParcelContainmentUnattributable)
}

// Covers: 未核对那一版已被重核接续（追加了「待识别 · 已核对」），它就不再是当前有效版本：在袋件此刻是查过了的零候选，包裹照常
// 答`不在`。另一租户的未核对在袋件不碰本租户的答案。
func TestARecheckedOrForeignUncheckedUnitNoLongerBlursContainment(t *testing.T) {
	fixture := newContainmentFixture(t)
	ctx := t.Context()
	unchecked := uncheckedPending(t, "tenant-a", "scan-unchecked", "SYN-INTAKEV-NRS05-UNCHECKED")
	checked := recheckablePending(t, "tenant-a", "IDENTITY-RECHECK/SYN-INTAKEV-NRS05-UNCHECKED", "SYN-INTAKEV-NRS05-RECHECKED", "SYN-MARK-NRS05")
	checked.IdentityCheck = ports.IdentityChecked
	checked.Predecessor = unchecked.JudgmentVersion
	foreign := uncheckedPending(t, "tenant-b", "scan-foreign", "SYN-INTAKEV-NRS05-FOREIGN")
	saveReceptions(t, fixture.transactor, ctx, fixture.receptions, unchecked, checked, foreign)
	fixture.bagWith(t, ctx, "tenant-a", "bag-1", "unit-2")
	fixture.bagWith(t, ctx, "tenant-b", "bag-b1", "unit-2")

	assertContainment(t, fixture.load(t, "tenant-a", "parcel-9/link-v1"), ports.ParcelNotContained)
	if containment := fixture.load(t, "tenant-b", "parcel-9/link-v1"); containment != ports.ParcelContainmentUnattributable {
		t.Fatalf("tenant-b 的未核对在袋件没起作用：%s", containment)
	}
}
