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

// viewFixture 是九个只读视图共用的夹具。视图这一侧没有写口——登记册的内容属实例
// 半边，由配置或上游上下文在带外落入——所以播种直接走 pool.Exec 的显式 SQL，而不
// 造一个只有测试用得上的写适配器。那种写口会成为生产代码里第二条能改登记册的路。
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
