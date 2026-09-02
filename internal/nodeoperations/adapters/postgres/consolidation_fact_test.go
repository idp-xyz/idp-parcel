package postgres_test

import (
	"context"
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

// TestAConsolidationFactRoundTripsAndKeepsTheTwoTimesApart 证 UC-NO-003 结果契约点名
// 要保存的那几项确实落库并读得回，且业务发生时间与记录时刻各占一列。
//
// 两个时间分立是本票的要害：它们若同源，一份补录或导入进来的事实就再也说不出现场究竟
// 什么时候发生过——而那正是 ADR-0023 判给设备、不许服务端代铸的东西。所以这里让两者
// 相隔九十分钟，读回后逐个比对，任一被对方顶替都会当场失配。
//
// 取封装格而不是开启格：六格里只有它带封签列，顺带把「在场的那一格真写进去了」证掉。
func TestAConsolidationFactRoundTripsAndKeepsTheTwoTimesApart(t *testing.T) {
	facts, units, transactor, _ := newConsolidationFacts(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	unit := openConsolidation(t, "bag-1", "asset-7")
	saveConsolidation(t, transactor, ctx, units, tenant, unit)

	source := workSource(t, "src-seal-1", consolidationAt)
	recordedAt := consolidationAt.Add(90 * time.Minute)
	record := ports.ConsolidationFactRecord{
		Key:           ports.ConsolidationFactKey{TenantID: tenant, SourceID: source.SourceID()},
		ContentDigest: "digest-seal-1",
		Action:        domain.SealUnitAction,
		Unit:          unit.ID(),
		Seal:          ref(t, domain.NewSealReference, "seal-1"),
		PerformedBy:   source.PerformedBy(),
		Evidence:      source.Evidence(),
		OccurredAt:    source.OccurredAt(),
		RecordedAt:    recordedAt,
	}
	saveConsolidationFact(t, transactor, ctx, facts, record)

	found, exists, err := facts.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if found.ContentDigest != record.ContentDigest ||
		found.Action != domain.SealUnitAction ||
		found.Unit != unit.ID() ||
		found.Seal.String() != record.Seal.String() {
		t.Errorf("作业事实往返变形：digest=%q action=%q unit=%q seal=%q",
			found.ContentDigest, found.Action, found.Unit, found.Seal)
	}
	if found.PerformedBy.String() != record.PerformedBy.String() ||
		found.Evidence.String() != record.Evidence.String() {
		t.Errorf("执行方或证据未落库：performedBy=%q evidence=%q", found.PerformedBy, found.Evidence)
	}
	if !found.OccurredAt.Equal(consolidationAt) {
		t.Errorf("业务发生时间 = %v，应为现场自带的 %v", found.OccurredAt, consolidationAt)
	}
	if !found.RecordedAt.Equal(recordedAt) {
		t.Errorf("记录时刻 = %v，应为 %v", found.RecordedAt, recordedAt)
	}
	if found.Member.String() != "" {
		t.Errorf("封装格读回了成员 %q——缺席在这一格是真话，不是漏填", found.Member)
	}

	tenantB := ref(t, domain.NewTenantID, "tenant-b")
	crossKey := ports.ConsolidationFactKey{TenantID: tenantB, SourceID: record.Key.SourceID}
	if _, leaked, err := facts.FindByKey(ctx, crossKey); err != nil || leaked {
		t.Errorf("另一个租户读到了作业事实：found=%v err=%v", leaked, err)
	}
}

// TestASecondConsolidationFactWriterKeepsTheFirst 守 AT-NO-043 落在库面上的那一半：同一
// 来源身份的第二份写入一律答`已有记录`，先到者一个字都不改。
//
// 重放与冲突在这一层长得一样是有意的——分辨两者要比内容指纹，那是编排受理闸的活；库面
// 只负责先到者不被顶替，所以两向都在这里各取一次证：同指纹的重放不重复落行，异指纹的
// 冲突也不覆盖，读回的始终是先到那一份的指纹。
func TestASecondConsolidationFactWriterKeepsTheFirst(t *testing.T) {
	facts, units, transactor, _ := newConsolidationFacts(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	unit := openConsolidation(t, "bag-1", "asset-7")
	saveConsolidation(t, transactor, ctx, units, tenant, unit)

	first := consolidationFactRecord(t, tenant, unit, "src-open-1")
	saveConsolidationFact(t, transactor, ctx, facts, first)

	replay := first
	conflicting := first
	conflicting.ContentDigest = "digest-another-content"

	for name, second := range map[string]ports.ConsolidationFactRecord{
		"重放同一内容": replay,
		"同身份异内容": conflicting,
	} {
		t.Run(name, func(t *testing.T) {
			var outcome ports.ConsolidationFactSaveOutcome
			var winner ports.ConsolidationFactRecord
			var winnerFound bool
			inTx(t, transactor, ctx, func(txCtx context.Context) error {
				saved, err := facts.Save(txCtx, second)
				if err != nil {
					return err
				}
				outcome = saved
				winner, winnerFound, err = facts.FindByKey(txCtx, first.Key)
				return err
			})
			if outcome != ports.ConsolidationFactAlreadyRecorded {
				t.Fatalf("第二份写入结果 = %d，应为 ALREADY_RECORDED", outcome)
			}
			if !winnerFound || winner.ContentDigest != first.ContentDigest {
				t.Fatalf("同事务读回赢家失败：found=%v digest=%q", winnerFound, winner.ContentDigest)
			}
		})
	}
}

// TestConsolidationFactPresenceConstraintsRejectImpossibleRows 守库面按动作判成员与封签
// 两列在场的那两条 CHECK。它们不是重复领域校验：绕过适配器直接写库的一行——迁移脚本、
// 运维改数、日后新写的另一个适配器——领域构造器一次都不会经过，这两列于是可以又缺又多。
func TestConsolidationFactPresenceConstraintsRejectImpossibleRows(t *testing.T) {
	_, units, transactor, pool := newConsolidationFacts(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	unit := openConsolidation(t, "bag-1", "asset-7")
	saveConsolidation(t, transactor, ctx, units, tenant, unit)

	insert := func(sourceID, action string, member, seal any) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO node_operations.consolidation_fact
				(tenant_id, source_id, content_digest, action, unit_id, member_id, seal_ref,
				 performed_by, evidence, occurred_at, recorded_at)
			 VALUES ('tenant-a', $1, 'digest-x', $2, 'bag-1', $3, $4,
			         'packer-1', 'WORK-EVIDENCE/x', now(), now())`,
			sourceID, action, member, seal)
		return err
	}

	if err := insert("bad-member-missing", "ADD_MEMBER", nil, nil); err == nil {
		t.Error("一行「移入却没有成员」溜进了作业事实登记")
	}
	if err := insert("bad-member-extra", "OPEN_UNIT", "unit-1", nil); err == nil {
		t.Error("一行「开启却带着成员」溜进了作业事实登记")
	}
	if err := insert("bad-seal-missing", "SEAL_UNIT", nil, nil); err == nil {
		t.Error("一行「封装却没有封签」溜进了作业事实登记")
	}
	if err := insert("bad-seal-extra", "REMOVE_MEMBER", "unit-1", "seal-1"); err == nil {
		t.Error("一行「移出却带着封签」溜进了作业事实登记")
	}
	if err := insert("bad-action", "REWEIGH", nil, nil); err == nil {
		t.Error("一个封闭词表外的动作溜进了作业事实登记")
	}
	if err := insert("good-open", "OPEN_UNIT", nil, nil); err != nil {
		t.Errorf("合法的一行被拒，上面五条拒绝因此说明不了是这几条 CHECK 在起作用：%v", err)
	}
}

func saveConsolidationFact(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	facts *adapter.ConsolidationFacts,
	record ports.ConsolidationFactRecord,
) {
	t.Helper()
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := facts.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.ConsolidationFactSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})
}
