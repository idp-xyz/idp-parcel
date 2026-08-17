package postgres_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证网络定义登记册读口的三格（ADR-0053）：查无登记即`未配置`、
// 租户之间互不可见、登记在场而本进程产不出事实时响亮报错——最后一格是全文件的重点，
// 它挡住「有定义就答已配置带空事实」这条会编出业务结论的路。

// TestAnUnregisteredScopeIsUnconfigured 证首发唯一走得到的真实分支：没有租户就没有网络
// 定义，读口如实答`未配置`且不报错——`未配置`是一个答案，不是一次失败。
func TestAnUnregisteredScopeIsUnconfigured(t *testing.T) {
	definitions, _ := newNetworkDefinitions(t)

	evidence, configured, err := definitions.LoadInitialRouteEvidence(
		t.Context(), initialRouteKeyFor(t, "tenant-a", "NETWORK_SERVICE"))
	if err != nil {
		t.Fatalf("未登记被当成错误：%v", err)
	}
	if configured {
		t.Fatal("查无登记却答已配置")
	}
	if evidence.ViewRevision.Valid() {
		t.Fatal("未配置的答复带了修订标识——那会让判断留下一个没有出处的比对锚")
	}

	reachability, configured, err := definitions.LoadNetworkEvidence(
		t.Context(), reachabilityKeyFor(t, "tenant-a", "NETWORK_SERVICE"))
	if err != nil || configured || reachability.ViewRevision.Valid() {
		t.Fatalf("可达性侧不同形：configured=%v err=%v", configured, err)
	}
}

// TestARegisteredDefinitionWithoutAResolverIsLoud 是本切片最要紧的一条。
//
// 登记在场而解析层不在时，读口既不能答`未配置`（会让人去催租户登记一份已经登记过的东西），
// 也不能答已配置带空事实（领域会照常评估并得出`无当前有效路由`，把「这个构建缺解析层」
// 讲成「这个网络里什么都没有」）。它必须响亮报错——非空答复必须有定义来源。
func TestARegisteredDefinitionWithoutAResolverIsLoud(t *testing.T) {
	definitions, pool := newNetworkDefinitions(t)
	registerDefinition(t, pool, "tenant-a", "NETWORK_SERVICE", "net-view-rev-1")

	_, configured, err := definitions.LoadInitialRouteEvidence(
		t.Context(), initialRouteKeyFor(t, "tenant-a", "NETWORK_SERVICE"))
	if !errors.Is(err, adapter.ErrNetworkDefinitionUnresolvable) {
		t.Fatalf("err = %v, want ErrNetworkDefinitionUnresolvable", err)
	}
	if configured {
		t.Fatal("产不出事实却声称已配置——这正是会编出业务结论的那一格")
	}

	if _, configured, err := definitions.LoadNetworkEvidence(
		t.Context(), reachabilityKeyFor(t, "tenant-a", "NETWORK_SERVICE"),
	); !errors.Is(err, adapter.ErrNetworkDefinitionUnresolvable) || configured {
		t.Fatalf("可达性侧不同形：configured=%v err=%v", configured, err)
	}
}

// TestDefinitionsAreScopedByTenantAndPurpose 证登记按（租户+服务目的）圈定：别的租户或
// 别的服务目的登记过，与本范围无关，仍是`未配置`。
func TestDefinitionsAreScopedByTenantAndPurpose(t *testing.T) {
	definitions, pool := newNetworkDefinitions(t)
	registerDefinition(t, pool, "tenant-a", "NETWORK_SERVICE", "net-view-rev-1")

	for name, key := range map[string]domain.InitialRouteJudgmentKey{
		"他租户":   initialRouteKeyFor(t, "tenant-b", "NETWORK_SERVICE"),
		"他服务目的": initialRouteKeyFor(t, "tenant-a", "LAST_MILE_DELIVERY"),
	} {
		t.Run(name, func(t *testing.T) {
			_, configured, err := definitions.LoadInitialRouteEvidence(t.Context(), key)
			if err != nil || configured {
				t.Fatalf("configured = %v err = %v；别处的登记不该覆盖本范围", configured, err)
			}
		})
	}
}

// TestAnIncompleteKeyIsRefusedBeforeQuerying 证最小范围不成立时不去查权威：一次已经发出
// 的查询收不回来，而这里连该查哪个范围都说不出。
func TestAnIncompleteKeyIsRefusedBeforeQuerying(t *testing.T) {
	definitions, _ := newNetworkDefinitions(t)

	if _, configured, err := definitions.LoadInitialRouteEvidence(
		t.Context(), domain.InitialRouteJudgmentKey{},
	); err == nil || configured {
		t.Fatalf("空键被放行：configured = %v err = %v", configured, err)
	}
}

func newNetworkDefinitions(t *testing.T) (*adapter.NetworkDefinitions, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	definitions, err := adapter.NewNetworkDefinitions(db)
	if err != nil {
		t.Fatalf("构造网络定义读口：%v", err)
	}
	return definitions, pool
}

// registerDefinition 直插一行登记。今天没有写入方（原语与解析层都不在），所以这一步只在
// 测试里发生——它演的是「日后真有定义可登之后」的那一刻。
func registerDefinition(t *testing.T, pool *pgxpool.Pool, tenant, purpose, revision string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO network_routing.network_definition (tenant_id, service_purpose, view_revision)
		 VALUES ($1, $2, $3)`,
		tenant, purpose, revision); err != nil {
		t.Fatalf("登记网络定义：%v", err)
	}
}

func initialRouteKeyFor(t *testing.T, tenant, purpose string) domain.InitialRouteJudgmentKey {
	t.Helper()
	return domain.InitialRouteJudgmentKey{
		TenantID:           scalar(t, domain.NewTenantID, tenant),
		CustomerAccountID:  scalar(t, domain.NewCustomerAccountID, "customer-a"),
		ShipmentRequestID:  scalar(t, domain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: scalar(t, domain.NewAcceptanceBaselineReference, "submission-1"),
		DeclaredParcelID:   scalar(t, domain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:     scalar(t, domain.NewServicePurpose, purpose),
	}
}

func reachabilityKeyFor(t *testing.T, tenant, purpose string) domain.ReachabilityJudgmentKey {
	t.Helper()
	return domain.ReachabilityJudgmentKey{
		TenantID:          scalar(t, domain.NewTenantID, tenant),
		CustomerAccountID: scalar(t, domain.NewCustomerAccountID, "customer-a"),
		ShipmentRequestID: scalar(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: scalar(t, domain.NewSubmissionVersionID, "submission-1"),
		DeclaredParcelID:  scalar(t, domain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:    scalar(t, domain.NewServicePurpose, purpose),
	}
}
