package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证集运作业的来源事实层（票 no-consolidation-fact-provenance/01）。
// 单元行与事实行一并构造：事实行外键指向单元行，没有单元的事实登记不了。

func newConsolidationFacts(t *testing.T) (
	*adapter.ConsolidationFacts,
	*adapter.ConsolidationUnits,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	facts, err := adapter.NewConsolidationFacts(db)
	if err != nil {
		t.Fatalf("构造来源事实登记：%v", err)
	}
	units, err := adapter.NewConsolidationUnits(db)
	if err != nil {
		t.Fatalf("构造合箱库：%v", err)
	}
	return facts, units, db.Transactor(), pool
}

// consolidationFactRecord 造一份可登记的作业事实。动作取开启格，因此成员与封签两列
// 缺席——迁移的在场 CHECK 正是按动作判这两格该不该有。
func consolidationFactRecord(
	t *testing.T,
	tenant domain.TenantID,
	unit *domain.ConsolidationUnit,
	sourceID string,
) ports.ConsolidationFactRecord {
	t.Helper()
	source := workSource(t, sourceID, consolidationAt)
	return ports.ConsolidationFactRecord{
		Key:           ports.ConsolidationFactKey{TenantID: tenant, SourceID: sourceID},
		ContentDigest: "digest-" + sourceID,
		Action:        domain.OpenUnitAction,
		Unit:          unit.ID(),
		PerformedBy:   source.PerformedBy(),
		Evidence:      source.Evidence(),
		OccurredAt:    source.OccurredAt(),
		RecordedAt:    consolidationAt.Add(time.Minute),
	}
}

// TestConsolidationFactWritesRefuseToRunOutsideATransaction 守 PBC-08 在本适配器上的
// 那一格：来源事实与它推动的那一步单元状态必须同生共死——只落一半时，要么单元变了却
// 说不出是谁变的，要么登记声称做过一件没做的事。所以写口走 RequireExecutor，无事务
// 上下文一律拒。这条性质随适配器逐个成立，别的包证过不算这里证过。
//
// 单元行先在事务里落好，事实行的外键因此是满足的：这一份记录放进事务就会被收下，
// 拒绝它的只可能是缺事务这一件事。
func TestConsolidationFactWritesRefuseToRunOutsideATransaction(t *testing.T) {
	facts, units, transactor, _ := newConsolidationFacts(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	unit := openConsolidation(t, "bag-1", "asset-7")
	saveConsolidation(t, transactor, ctx, units, tenant, unit)

	record := consolidationFactRecord(t, tenant, unit, "src-outside-tx")
	if _, err := facts.Save(ctx, record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, found, err := facts.FindByKey(ctx, record.Key); err != nil || found {
		t.Errorf("被拒绝的写入仍然落库：found=%v err=%v", found, err)
	}
}
