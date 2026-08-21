package postgres_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var viewBaseAt = time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)

// viewFixture 是九个只读视图共用的夹具。播种走 pool.Exec 的显式 SQL，不借道任何
// 写适配器。
//
// 这条原先的理由是「视图这一侧根本没有写口」，自 case_config_registry.go 落地后不再
// 成立：五本案件配置登记册已有生产写口。但做法不变，理由换成两条——读口用例必须能
// 独立于写口播种，否则写口一有 bug 就会同时染红两侧，再也分不出是谁错；而 rejects
// 那一族要的正是绕开一切 Go 侧校验、直接撞库里的 CHECK 与外键。
//
// 写口自己的往返用例在 case_config_registry_test.go，那边一律穿读口取回。
type viewFixture struct {
	pool *pgxpool.Pool
	db   *bentopg.DB
}

func newViewFixture(t *testing.T) *viewFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return &viewFixture{pool: pool, db: db}
}

// seed 播种一行登记内容，失败即测试失败——播不进去的夹具证不了任何读口行为。
func (fixture *viewFixture) seed(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("播种登记内容：%v", err)
	}
}

// rejects 断言库把一行挡在门外。用于逐条钉住迁移里的 CHECK 与外键：领域不变量在
// 库内再守一遍，旁路写入也进不来。
func (fixture *viewFixture) rejects(t *testing.T, reason string, sql string, args ...any) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(), sql, args...); err == nil {
		t.Fatalf("库接受了本该拒绝的行：%s", reason)
	}
}

func viewValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}
