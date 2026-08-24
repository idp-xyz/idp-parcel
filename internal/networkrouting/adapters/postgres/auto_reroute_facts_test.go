package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var factsCatalogRegisteredAt = time.Date(2026, 8, 24, 11, 0, 0, 0, time.UTC)

func newAutoRerouteFactsCatalog(t *testing.T) (*adapter.AutoRerouteFactsCatalog, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalog, err := adapter.NewAutoRerouteFactsCatalog(db)
	if err != nil {
		t.Fatalf("构造事实目录：%v", err)
	}
	return catalog, db.Transactor(), pool
}

func autoRerouteKey(t *testing.T, tenant, parcel string) domain.InitialRouteJudgmentKey {
	t.Helper()
	return domain.InitialRouteJudgmentKey{
		TenantID:           scalar(t, domain.NewTenantID, tenant),
		CustomerAccountID:  scalar(t, domain.NewCustomerAccountID, "customer-a"),
		ShipmentRequestID:  scalar(t, domain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: scalar(t, domain.NewAcceptanceBaselineReference, "submission-1"),
		DeclaredParcelID:   scalar(t, domain.NewDeclaredParcelID, parcel),
		ServicePurpose:     scalar(t, domain.NewServicePurpose, "LAST_MILE_DELIVERY"),
	}
}

func autoRerouteRecord(t *testing.T, key domain.InitialRouteJudgmentKey, version int) ports.AutoRerouteFactsRecord {
	t.Helper()
	return ports.AutoRerouteFactsRecord{
		Key:     key,
		Version: version,
		Facts: domain.AutoRerouteFacts{
			PolicyAllowsAutomatic:  true,
			AtControlledNode:       true,
			OnlyUnexecutedAffected: false,
			UnresolvedRestrictions: []domain.RestrictionReference{
				scalar(t, domain.NewRestrictionReference, "restriction/customs-hold-1"),
			},
			OutstandingResponsibilities: []domain.ResponsibilityReference{
				scalar(t, domain.NewResponsibilityReference, "responsibility/booking-42"),
			},
		},
		StrategyBasis: "route-strategy/syn-v1",
		RegisteredAt:  factsCatalogRegisteredAt,
	}
}

func registerFacts(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	catalog *adapter.AutoRerouteFactsCatalog,
	record ports.AutoRerouteFactsRecord,
) {
	t.Helper()
	within(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := catalog.RegisterAutoRerouteFacts(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.AutoRerouteFactsRegistered {
			t.Fatalf("register outcome = %q, want REGISTERED", outcome)
		}
		return nil
	})
}

// Covers: 票 05「目录未配置时保持现状」的存取半边——从未登记过的判断键答第二格
// （未配置），不是错误也不是一份空事实；复核编排据此整段不做改路评估。
func TestAutoRerouteFactsAnswerUnconfiguredForUnregisteredKey(t *testing.T) {
	catalog, _, _ := newAutoRerouteFactsCatalog(t)

	_, configured, err := catalog.LoadAutoRerouteFacts(t.Context(), autoRerouteKey(t, "tenant-1", "parcel-1"))
	if err != nil {
		t.Fatalf("读未登记键不该是错误，实得：%v", err)
	}
	if configured {
		t.Fatal("从未登记过事实的判断键被答成了已配置")
	}
}

// Covers: 装载口与写入方的真库往返——五件事实与折算依据逐字段等值读回,验证用合成
// 条件（隔离合成 S,不进任何生产默认）。
func TestAutoRerouteFactsRoundTripThroughRealDatabase(t *testing.T) {
	catalog, transactor, _ := newAutoRerouteFactsCatalog(t)
	ctx := t.Context()
	key := autoRerouteKey(t, "tenant-1", "parcel-1")
	registerFacts(t, transactor, ctx, catalog, autoRerouteRecord(t, key, 1))

	facts, configured, err := catalog.LoadAutoRerouteFacts(ctx, key)
	if err != nil {
		t.Fatalf("读事实：%v", err)
	}
	if !configured {
		t.Fatal("登记过的键被答成了未配置")
	}
	if !facts.PolicyAllowsAutomatic || !facts.AtControlledNode || facts.OnlyUnexecutedAffected {
		t.Fatalf("三个折算结论没有等值读回：%+v", facts)
	}
	if len(facts.UnresolvedRestrictions) != 1 ||
		facts.UnresolvedRestrictions[0].String() != "restriction/customs-hold-1" {
		t.Fatalf("限制清单没有等值读回：%+v", facts.UnresolvedRestrictions)
	}
	if len(facts.OutstandingResponsibilities) != 1 ||
		facts.OutstandingResponsibilities[0].String() != "responsibility/booking-42" {
		t.Fatalf("责任清单没有等值读回：%+v", facts.OutstandingResponsibilities)
	}
}

// Covers: 历史链的读侧——同键多版并存时当前陈述取最大版本（先例：availability_
// adjustment 的「当前陈述取最大版本」机制），历史版本原样保留供 FindAutoRerouteFacts
// 审计取证。
func TestAutoRerouteFactsCurrentStatementIsTheHighestVersion(t *testing.T) {
	catalog, transactor, _ := newAutoRerouteFactsCatalog(t)
	ctx := t.Context()
	key := autoRerouteKey(t, "tenant-1", "parcel-1")

	first := autoRerouteRecord(t, key, 1)
	registerFacts(t, transactor, ctx, catalog, first)

	// 第二版：限制解除了（清单转空是有效陈述,不是缺数据）。
	second := autoRerouteRecord(t, key, 2)
	second.Facts.UnresolvedRestrictions = nil
	second.StrategyBasis = "route-strategy/syn-v2"
	registerFacts(t, transactor, ctx, catalog, second)

	facts, configured, err := catalog.LoadAutoRerouteFacts(ctx, key)
	if err != nil {
		t.Fatalf("读事实：%v", err)
	}
	if !configured {
		t.Fatal("登记过的键被答成了未配置")
	}
	if len(facts.UnresolvedRestrictions) != 0 {
		t.Fatalf("当前陈述该是第 2 版（限制已解除），实得：%+v", facts.UnresolvedRestrictions)
	}

	kept, found, err := catalog.FindAutoRerouteFacts(ctx, key, 1)
	if err != nil || !found {
		t.Fatalf("第 1 版历史必须原样保留：found=%v err=%v", found, err)
	}
	if len(kept.Facts.UnresolvedRestrictions) != 1 {
		t.Fatalf("历史版本的清单被改动了：%+v", kept.Facts.UnresolvedRestrictions)
	}
	if kept.StrategyBasis != "route-strategy/syn-v1" {
		t.Fatalf("历史版本的折算依据被改动了：%q", kept.StrategyBasis)
	}
}

// Covers: 不可覆盖纪律的库面——同键同版本再来答`已登记`且事务保持可用（ON CONFLICT
// 而非中止态），同一事务里紧接着读回既有版比对，这正是登记编排的用法。
func TestAutoRerouteFactsDuplicateVersionKeepsTransactionUsable(t *testing.T) {
	catalog, transactor, _ := newAutoRerouteFactsCatalog(t)
	ctx := t.Context()
	key := autoRerouteKey(t, "tenant-1", "parcel-1")
	registerFacts(t, transactor, ctx, catalog, autoRerouteRecord(t, key, 1))

	changed := autoRerouteRecord(t, key, 1)
	changed.Facts.PolicyAllowsAutomatic = false

	var outcome ports.AutoRerouteFactsSaveOutcome
	var winner ports.AutoRerouteFactsRecord
	var winnerFound bool
	within(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = catalog.RegisterAutoRerouteFacts(txCtx, changed)
		if err != nil {
			return err
		}
		// `已登记`之后同一事务里读回赢家——捕 23505 的译法在这一步就会挂。
		winner, winnerFound, err = catalog.FindAutoRerouteFacts(txCtx, key, 1)
		return err
	})

	if outcome != ports.AutoRerouteFactsAlreadyRegistered {
		t.Fatalf("outcome = %q, want ALREADY_REGISTERED", outcome)
	}
	if !winnerFound {
		t.Fatal("同一事务里读不回赢家")
	}
	if !winner.Facts.PolicyAllowsAutomatic {
		t.Fatal("已在册的陈述被第二份覆盖了")
	}
}

// Covers: 写口的事务纪律——无环境事务即拒绝（RequireExecutor），事实版本行不得在
// 无事务边界下落库（先例：票 06 发现并钉住的同一格）。
func TestAutoRerouteFactsRegisterRequiresAmbientTransaction(t *testing.T) {
	catalog, _, _ := newAutoRerouteFactsCatalog(t)

	_, err := catalog.RegisterAutoRerouteFacts(
		t.Context(), autoRerouteRecord(t, autoRerouteKey(t, "tenant-1", "parcel-1"), 1))
	if err == nil {
		t.Fatal("无环境事务的登记必须被拒")
	}
}

// Covers: 「身份不成立时不读任何权威」——判断键不完整的读是错误,不是未配置：把一次
// 编程错误答成`未配置`会让复核静默跳过改路评估。
func TestAutoRerouteFactsIncompleteKeyIsAnErrorNotUnconfigured(t *testing.T) {
	catalog, _, _ := newAutoRerouteFactsCatalog(t)

	_, _, err := catalog.LoadAutoRerouteFacts(t.Context(), domain.InitialRouteJudgmentKey{})
	if err == nil {
		t.Fatal("键不完整的读必须报错")
	}
}

// Covers: 租户隔离（ADR-0003 最高数据隔离边界）——另一租户登记的事实不得越过租户
// 维答给本租户的同名判断范围。
func TestAutoRerouteFactsAreTenantIsolated(t *testing.T) {
	catalog, transactor, _ := newAutoRerouteFactsCatalog(t)
	ctx := t.Context()
	registerFacts(t, transactor, ctx, catalog, autoRerouteRecord(t, autoRerouteKey(t, "tenant-1", "parcel-1"), 1))

	_, configured, err := catalog.LoadAutoRerouteFacts(ctx, autoRerouteKey(t, "tenant-2", "parcel-1"))
	if err != nil {
		t.Fatalf("读另一租户：%v", err)
	}
	if configured {
		t.Fatal("事实越过了租户边界")
	}
}
