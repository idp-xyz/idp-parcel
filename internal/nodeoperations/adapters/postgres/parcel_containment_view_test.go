package postgres_test

import (
	"context"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证按正式包裹的版本化关联答「此刻在不在未关闭集运单元里」的读面
// （票 ps-port-remainder/05 的 NO 半边）。播种走写侧适配器（收寄库与合箱库），与查阅目录那
// 组用例同一做法：读面沿的是写口真实落下的那两条关联路，用写口造出来的行证它才证得到。
// 三值各证一格，再证已关联压过候选、关闭与另一租户读不到。

type containmentFixture struct {
	view           *adapter.ParcelContainmentView
	receptions     *adapter.Receptions
	consolidations *adapter.ConsolidationUnits
	transactor     bentoapp.Transactor
}

func newContainmentFixture(t *testing.T) *containmentFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	view, err := adapter.NewParcelContainmentView(db)
	if err != nil {
		t.Fatalf("构造包裹容纳读口：%v", err)
	}
	receptions, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	consolidations, err := adapter.NewConsolidationUnits(db)
	if err != nil {
		t.Fatalf("构造合箱库：%v", err)
	}
	return &containmentFixture{
		view:           view,
		receptions:     receptions,
		consolidations: consolidations,
		transactor:     db.Transactor(),
	}
}

// bagWith 开一个集运单元并把给定实物移入，返回单元供后续封装/关闭。
func (fixture *containmentFixture) bagWith(t *testing.T, ctx context.Context, tenant, bagID string, members ...string) *domain.ConsolidationUnit {
	t.Helper()
	tenantID := ref(t, domain.NewTenantID, tenant)
	unit := openConsolidation(t, bagID, "asset-"+bagID)
	saveConsolidation(t, fixture.transactor, ctx, fixture.consolidations, tenantID, unit)
	for _, member := range members {
		if err := unit.AddMember(ref(t, domain.NewHandlingUnitID, member)); err != nil {
			t.Fatalf("移入 %s：%v", member, err)
		}
	}
	updateConsolidation(t, fixture.transactor, ctx, fixture.consolidations, tenantID, unit)
	return unit
}

func (fixture *containmentFixture) load(t *testing.T, tenant, parcel string) ports.ParcelContainment {
	t.Helper()
	containment, err := fixture.view.LoadParcelContainment(t.Context(),
		ref(t, domain.NewTenantID, tenant),
		ref(t, domain.NewParcelAssociationReference, parcel))
	if err != nil {
		t.Fatalf("读包裹容纳：%v", err)
	}
	return containment
}

func assertContainment(t *testing.T, got, want ports.ParcelContainment) {
	t.Helper()
	if got != want {
		t.Fatalf("containment = %s, want %s", got, want)
	}
}

// 从未把任何实物关联到该包裹：`不在`，不是`不知道`——本上下文册子里没有可归属于它的装袋
// 事实，这是如实回答；答成不知道会让每一个尚未到站的包裹都判不出阶段。
func TestAParcelNoNodeEverAssociatedIsNotContained(t *testing.T) {
	fixture := newContainmentFixture(t)
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelNotContained)
}

// 已关联的实物收寄了但没进任何单元：`不在`；移入未关闭单元后：`在`；封装不改变在场——
// 封装冻结成员快照，成员仍在袋里。
func TestAnAssociatedUnitIsContainedOnceItIsBaggedAndStaysSoWhenSealed(t *testing.T) {
	fixture := newContainmentFixture(t)
	ctx := t.Context()
	saveReceptions(t, fixture.transactor, ctx, fixture.receptions, formedRecord(t, "tenant-a", "scan-1"))
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelNotContained)

	bag := fixture.bagWith(t, ctx, "tenant-a", "bag-1", "unit-1")
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelContained)

	if err := bag.Seal(
		ref(t, domain.NewSealReference, "seal-1"),
		ref(t, domain.NewWorkBasisReference, "PACK/1"),
		workSource(t, "src-seal-1", consolidationAt),
	); err != nil {
		t.Fatalf("seal：%v", err)
	}
	updateConsolidation(t, fixture.transactor, ctx, fixture.consolidations, ref(t, domain.NewTenantID, "tenant-a"), bag)
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelContained)
}

// 单元关闭后成员不再占据当前父级：读面随容纳索引一起退回`不在`。「已装袋」是此刻的事实，
// 不是历史。
func TestAClosedUnitNoLongerContainsTheParcel(t *testing.T) {
	fixture := newContainmentFixture(t)
	ctx := t.Context()
	saveReceptions(t, fixture.transactor, ctx, fixture.receptions, formedRecord(t, "tenant-a", "scan-1"))
	bag := fixture.bagWith(t, ctx, "tenant-a", "bag-1", "unit-1")
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelContained)

	if err := bag.Close(ref(t, domain.NewWorkBasisReference, "DISPOSITION/1"), consolidationAt); err != nil {
		t.Fatalf("close：%v", err)
	}
	updateConsolidation(t, fixture.transactor, ctx, fixture.consolidations, ref(t, domain.NewTenantID, "tenant-a"), bag)
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelNotContained)
}

// 待识别实物的候选里列着该包裹、且那件实物此刻在袋里：`不可归属`。候选不是归属——既不能
// 答`在`（拿候选冒充关联），也不能答`不在`（身份冲突尚未处置的包裹被读成没装袋）。那件实物
// 不在袋里时，所有可能是它的东西都没装袋，如实答`不在`。
func TestACandidateOnlyUnitInABagIsUnattributable(t *testing.T) {
	fixture := newContainmentFixture(t)
	ctx := t.Context()
	// pendingRecord：unit-2 待识别，候选 parcel-7 与 parcel-8，身份冲突。
	saveReceptions(t, fixture.transactor, ctx, fixture.receptions, pendingRecord(t, "tenant-a", "scan-2"))
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-7/link-v1"), ports.ParcelNotContained)

	fixture.bagWith(t, ctx, "tenant-a", "bag-1", "unit-2")
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-7/link-v1"), ports.ParcelContainmentUnattributable)
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-8/link-v1"), ports.ParcelContainmentUnattributable)
	// 候选里没有的包裹与这件实物无关。
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelNotContained)
}

// 已关联实物在袋里压过候选：一个包裹既有已识别实物在袋里、又被另一件待识别实物列为候选时，
// 答`在`——已关联的那件已经足以成立事实，候选那件说不出的东西不再改变答案。
func TestAnAssociatedUnitInABagOverridesACandidateOnlyUnit(t *testing.T) {
	fixture := newContainmentFixture(t)
	ctx := t.Context()
	formed := formedRecord(t, "tenant-a", "scan-1")
	pending := pendingRecord(t, "tenant-a", "scan-2")
	pending.Candidates = []domain.ParcelAssociationReference{
		ref(t, domain.NewParcelAssociationReference, "parcel-1/link-v1"),
		ref(t, domain.NewParcelAssociationReference, "parcel-8/link-v1"),
	}
	saveReceptions(t, fixture.transactor, ctx, fixture.receptions, formed, pending)
	fixture.bagWith(t, ctx, "tenant-a", "bag-1", "unit-1", "unit-2")

	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelContained)
	assertContainment(t, fixture.load(t, "tenant-a", "parcel-8/link-v1"), ports.ParcelContainmentUnattributable)
}

func TestParcelContainmentOfAnotherTenantIsInvisible(t *testing.T) {
	fixture := newContainmentFixture(t)
	ctx := t.Context()
	saveReceptions(t, fixture.transactor, ctx, fixture.receptions, formedRecord(t, "tenant-a", "scan-1"))
	fixture.bagWith(t, ctx, "tenant-a", "bag-1", "unit-1")

	assertContainment(t, fixture.load(t, "tenant-a", "parcel-1/link-v1"), ports.ParcelContained)
	assertContainment(t, fixture.load(t, "tenant-b", "parcel-1/link-v1"), ports.ParcelNotContained)
}
