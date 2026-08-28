package postgres_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 代收分户账列表读面的用例(ADR-0077 Decision 四/五那三句在本册的样子)。播种走
// pool.Exec 的显式 SQL,不借道写适配器:读口用例必须能独立于写口播种,否则写口一有
// bug 就会同时染红两侧,再也分不出是谁错(判据同 customscompliance viewFixture 注释)。
//
// 助手一律带 catalogue 前缀:本包与写侧登记册测试共享 postgres_test 包名,前缀防撞
// 已在频道向 MCP-2 声明。

var catalogueBaseAt = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)

type catalogueFixture struct {
	pool *pgxpool.Pool
	db   *bentopg.DB
}

func newCatalogueFixture(t *testing.T) *catalogueFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB:%v", err)
	}
	return &catalogueFixture{pool: pool, db: db}
}

// seed 播种一行登记内容,失败即测试失败——播不进去的夹具证不了任何读口行为。
func (fixture *catalogueFixture) seed(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("播种登记内容:%v", err)
	}
}

func catalogueValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q:%v", raw, err)
	}
	return built
}

func newCodSubledgerCatalogue(t *testing.T) (*adapter.CodSubledgerCatalogue, *catalogueFixture) {
	t.Helper()
	fixture := newCatalogueFixture(t)
	catalogue, err := adapter.NewCodSubledgerCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造分户账列表读口:%v", err)
	}
	return catalogue, fixture
}

// seedSubledger 开立一本分户账(四维键+保管依据),开立时间取 catalogueBaseAt。
func (fixture *catalogueFixture) seedSubledger(t *testing.T, tenant, customer, entity, currency, channel string) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO collection_remittance.subledger
			(tenant_id, customer_ref, legal_entity_ref, currency, channel_ref, custody_basis_ref, opened_at)
		 VALUES ($1, $2, $3, $4, $5, 'SYN-CUSTODY-01', $6)`,
		tenant, customer, entity, currency, channel, catalogueBaseAt)
}

// seedPosting 追加一笔记账;依据种类按库上四条依据门取合法值(入账凭代收事实、进
// 应付客户不得凭代收事实、进已汇付凭批次、进短溢款凭差异事项),读口用例不关心写口
// 编排,只要行进得了库。
func (fixture *catalogueFixture) seedPosting(
	t *testing.T,
	tenant, posting, customer, entity, currency, channel,
	fromPosition, toPosition string,
	amountMinor int64,
	basisKind, basisRef string,
) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO collection_remittance.subledger_posting
			(tenant_id, posting_ref, customer_ref, legal_entity_ref, currency, channel_ref,
			 from_position, to_position, amount_minor, basis_kind, basis_ref, posted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		tenant, posting, customer, entity, currency, channel,
		fromPosition, toPosition, amountMinor, basisKind, basisRef, catalogueBaseAt)
}

// Covers: ADR-0077 Decision 四 — 空册如实答空列表,不是错误也不折成未配置。
func TestEmptyCodSubledgerRegisterAnswersAnEmptyList(t *testing.T) {
	catalogue, _ := newCodSubledgerCatalogue(t)
	entries, err := catalogue.ListCodSubledgers(t.Context(),
		catalogueValue(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil || len(entries) != 0 {
		t.Fatalf("空册上列走样:err=%v entries=%+v", err, entries)
	}
}

// Covers: 六位置余额按记账派生(去向侧加、来源侧减)、记账笔数照实、批次按标识升序
// 附于所属账、开立面字段照登转写;跨租户不可见。
func TestCodSubledgerCatalogueDerivesBalancesAndAttachesBatches(t *testing.T) {
	catalogue, fixture := newCodSubledgerCatalogue(t)
	fixture.seedSubledger(t, "tenant-a", "SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")
	fixture.seedSubledger(t, "tenant-b", "SYN-CUST-09", "SYN-LE-09", "SYN-CUR-09", "SYN-CHAN-09")

	// 入账 150000 → 全额回款 → 100000 清分到应付客户:在途归零、待清分余 50000、
	// 应付 100000。
	fixture.seedPosting(t, "tenant-a", "SYN-POST-01",
		"SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01",
		"EXTERNAL_SOURCE", "IN_TRANSIT_AT_CHANNEL", 150000, "COLLECTION_FACT", "SYN-FACT-01")
	fixture.seedPosting(t, "tenant-a", "SYN-POST-02",
		"SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01",
		"IN_TRANSIT_AT_CHANNEL", "AWAITING_ALLOCATION", 150000, "COLLECTION_FACT", "SYN-FACT-02")
	// 清分凭代收指令(ALLOCATION)——库上 CHECK 拒绝凭一层来源事实直接进应付客户。
	fixture.seedPosting(t, "tenant-a", "SYN-POST-03",
		"SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01",
		"AWAITING_ALLOCATION", "PAYABLE_TO_CUSTOMER", 100000, "ALLOCATION", "SYN-INSTR-01")

	successionAt := catalogueBaseAt.Add(72 * time.Hour)
	fixture.seed(t,
		`INSERT INTO collection_remittance.remittance_batch
			(tenant_id, batch_ref, customer_ref, legal_entity_ref, currency, channel_ref,
			 collected_through, state, formed_at)
		 VALUES
			('tenant-a', 'SYN-BATCH-02', 'SYN-CUST-01', 'SYN-LE-01', 'SYN-CUR-01', 'SYN-CHAN-01',
			 $1, 'HANDED_FOR_PAYMENT', $2),
			('tenant-a', 'SYN-BATCH-01', 'SYN-CUST-01', 'SYN-LE-01', 'SYN-CUR-01', 'SYN-CHAN-01',
			 $1, 'COLLECTED', $2)`,
		successionAt, catalogueBaseAt)

	entries, err := catalogue.ListCodSubledgers(t.Context(),
		catalogueValue(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil {
		t.Fatalf("上列分户账:%v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("上列了 %d 行,要 1 行:%+v", len(entries), entries)
	}
	row := entries[0]
	if row.Customer != "SYN-CUST-01" || row.LegalEntity != "SYN-LE-01" ||
		row.Currency != "SYN-CUR-01" || row.Channel != "SYN-CHAN-01" ||
		row.CustodyBasis != "SYN-CUSTODY-01" || !row.OpenedAt.Equal(catalogueBaseAt) {
		t.Fatalf("开立面字段转写走样:%+v", row)
	}
	balances := row.Balances
	if balances.InTransitAtChannelMinor != 0 ||
		balances.AwaitingAllocationMinor != 50000 ||
		balances.PayableToCustomerMinor != 100000 ||
		balances.RemittedMinor != 0 || balances.ShortfallMinor != 0 || balances.SurplusMinor != 0 {
		t.Fatalf("派生余额走样:%+v", balances)
	}
	if row.PostingCount != 3 {
		t.Fatalf("记账笔数 = %d,要 3", row.PostingCount)
	}
	if len(row.Batches) != 2 ||
		row.Batches[0].Batch != "SYN-BATCH-01" || row.Batches[0].State != "COLLECTED" ||
		!row.Batches[0].CollectedThrough.Equal(successionAt) ||
		!row.Batches[0].FormedAt.Equal(catalogueBaseAt) ||
		row.Batches[1].Batch != "SYN-BATCH-02" || row.Batches[1].State != "HANDED_FOR_PAYMENT" {
		t.Fatalf("批次转写走样(要按标识升序两个):%+v", row.Batches)
	}
}

// Covers: 「已开立但从未记账」以全零余额、零笔数、空批次在场——与「未开立」(整行
// 不在列)是两个相反的答案(迁移 0001 开立面自注);排序按分户账键升序。
func TestCodSubledgerCatalogueKeepsOpenedButUnpostedLedgersInPlace(t *testing.T) {
	catalogue, fixture := newCodSubledgerCatalogue(t)
	fixture.seedSubledger(t, "tenant-a", "SYN-CUST-02", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")
	fixture.seedSubledger(t, "tenant-a", "SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")
	fixture.seedPosting(t, "tenant-a", "SYN-POST-01",
		"SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01",
		"EXTERNAL_SOURCE", "IN_TRANSIT_AT_CHANNEL", 88000, "COLLECTION_FACT", "SYN-FACT-01")

	entries, err := catalogue.ListCodSubledgers(t.Context(),
		catalogueValue(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil {
		t.Fatalf("上列分户账:%v", err)
	}
	if len(entries) != 2 || entries[0].Customer != "SYN-CUST-01" || entries[1].Customer != "SYN-CUST-02" {
		t.Fatalf("键序或行数走样:%+v", entries)
	}
	unposted := entries[1]
	if unposted.PostingCount != 0 ||
		unposted.Balances != (ports.CodSubledgerPositionBalances{}) ||
		len(unposted.Batches) != 0 {
		t.Fatalf("零记账账走样(要全零余额、零笔数、空批次):%+v", unposted)
	}
}

// Covers: 读口照实转写,不钳位——旁路写入造出的负余额原样透出:钳掉的那截正是要人
// 去查的证据(库不守余额非负,那道门在写口编排,读口不代守)。
func TestCodSubledgerCatalogueTranscribesNegativeBalancesFaithfully(t *testing.T) {
	catalogue, fixture := newCodSubledgerCatalogue(t)
	fixture.seedSubledger(t, "tenant-a", "SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")
	// 无入账直接清分:待清分被扣成负——这行在库上合法(依据门不核余额),只有写口
	// 编排的余额守卫会拦。
	fixture.seedPosting(t, "tenant-a", "SYN-POST-01",
		"SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01",
		"AWAITING_ALLOCATION", "PAYABLE_TO_CUSTOMER", 500, "ALLOCATION", "SYN-INSTR-01")

	entries, err := catalogue.ListCodSubledgers(t.Context(),
		catalogueValue(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("上列分户账走样:err=%v entries=%+v", err, entries)
	}
	balances := entries[0].Balances
	if balances.AwaitingAllocationMinor != -500 || balances.PayableToCustomerMinor != 500 {
		t.Fatalf("负余额没有照实透出:%+v", balances)
	}
}

// Covers: ADR-0077 Decision 五 — limit 非正拒;limit 截断行数而不是静默全量。
func TestCodSubledgerCatalogueGuardsItsLimit(t *testing.T) {
	catalogue, fixture := newCodSubledgerCatalogue(t)
	tenant := catalogueValue(t, domain.NewTenantID, "tenant-a")
	if _, err := catalogue.ListCodSubledgers(t.Context(), tenant, 0); err == nil {
		t.Fatal("limit=0 的上列被接受了")
	}
	if _, err := catalogue.ListCodSubledgers(t.Context(), tenant, -1); err == nil {
		t.Fatal("limit=-1 的上列被接受了")
	}

	fixture.seedSubledger(t, "tenant-a", "SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")
	fixture.seedSubledger(t, "tenant-a", "SYN-CUST-02", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")
	fixture.seedSubledger(t, "tenant-a", "SYN-CUST-03", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")
	entries, err := catalogue.ListCodSubledgers(t.Context(), tenant, 2)
	if err != nil || len(entries) != 2 {
		t.Fatalf("limit=2 却上列了 %d 行:err=%v", len(entries), err)
	}
}
