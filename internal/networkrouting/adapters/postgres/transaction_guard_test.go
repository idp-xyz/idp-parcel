package postgres_test

import (
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// PBC-08 行为面负向证据（bento-gate-reeval 票 02）：目录登记口在无事务上下文必须被
// RequireExecutor 拒绝。拒绝先于任何入参解读，所以传零值就够；若有人把入参校验挪到
// 守卫之前，断言会以「错误不是 ErrTransactionRequired」如实变红。
func TestCatalogRegistrationRefusesToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	catalog, err := adapter.NewNetworkCatalog(db)
	if err != nil {
		t.Fatalf("构造网络目录：%v", err)
	}
	if err := catalog.RegisterNodeVersion(ctx, domain.TenantID{}, ports.NodeDefinitionVersion{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记节点版本应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := catalog.RegisterConnectionVersion(ctx, domain.TenantID{}, ports.ConnectionDefinitionVersion{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记连接版本应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := catalog.RegisterLineVersion(ctx, domain.TenantID{}, ports.LineDefinitionVersion{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记线路版本应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := catalog.RegisterServiceAreaVersion(ctx, domain.TenantID{}, ports.ServiceAreaDefinitionVersion{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记服务区域版本应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := catalog.RegisterServiceCalendarVersion(ctx, domain.TenantID{}, ports.ServiceCalendarDefinitionVersion{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记服务日历版本应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := catalog.RegisterRouteStrategyVersion(ctx, domain.TenantID{}, ports.RouteStrategyDefinitionVersion{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记路由策略版本应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := catalog.RegisterAvailabilityAdjustment(ctx, domain.TenantID{}, ports.AvailabilityAdjustmentStatement{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记可用性调整应返回 ErrTransactionRequired，实得：%v", err)
	}

	facts, err := adapter.NewAutoRerouteFactsCatalog(db)
	if err != nil {
		t.Fatalf("构造自动改路事实目录：%v", err)
	}
	// 行完整性门在守卫之前（键完整、版本为正、依据与时刻非空），键复用本包
	// auto_reroute_facts_test.go 的夹具让请求走到守卫。
	record := ports.AutoRerouteFactsRecord{
		Key:           autoRerouteKey(t, "tenant-ntx", "parcel-ntx"),
		Version:       1,
		StrategyBasis: "strategy/ntx",
		RegisteredAt:  time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC),
	}
	if _, err := facts.RegisterAutoRerouteFacts(ctx, record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记自动改路事实应返回 ErrTransactionRequired，实得：%v", err)
	}
}
