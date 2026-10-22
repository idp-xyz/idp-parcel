package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证候选成本取数侧的 savepoint 隔离：内层在事务里报 SQL 级错误时，判断那一笔事务照样能写、能提交。
// 替身取数侧与生产的仓储读口一样经 ReadExecutor 在调用方的事务里发 SQL。随后的写借交接登记册，它与复核、初始路由的落册同在
// network_routing 一笔事务里。断言一律在事务闭包外。

// sqlCostSource 在调用方的事务里发一条 SQL；query 出错时把错误原样交回，像一张读不动的价卡表那样。
type sqlCostSource struct {
	db    *bentopg.DB
	query string
	asked int
}

func (source *sqlCostSource) LoadCandidateCosts(
	ctx context.Context, _ domain.InitialRouteJudgmentKey, _ ports.InitialRouteEvidence, _ ports.RequestCarriedContent,
) (ports.RouteCandidateCosts, error) {
	source.asked++
	querier, err := source.db.ReadExecutor(ctx)
	if err != nil {
		return ports.RouteCandidateCosts{}, err
	}
	var amount int64
	if err := querier.QueryRow(ctx, source.query).Scan(&amount); err != nil {
		return ports.RouteCandidateCosts{}, err
	}
	candidate, err := domain.NewCandidateID("SYN-CANDIDATE-01")
	if err != nil {
		return ports.RouteCandidateCosts{}, err
	}
	fact, err := domain.NewPricedCandidateCost(candidate, amount, "CNY")
	if err != nil {
		return ports.RouteCandidateCosts{}, err
	}
	return ports.RouteCandidateCosts{Facts: []domain.CandidateCostFact{fact}}, nil
}

func candidateCostsSavepointFixture(
	t *testing.T, query string,
) (*bentopg.DB, *adapter.RouteHandoffLogs, *sqlCostSource, *adapter.SavepointedRouteCandidateCosts) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	log, err := adapter.NewRouteHandoffLogs(db)
	if err != nil {
		t.Fatalf("构造交接登记册：%v", err)
	}
	inner := &sqlCostSource{db: db, query: query}
	costs, err := adapter.NewSavepointedRouteCandidateCosts(db, inner)
	if err != nil {
		t.Fatalf("构造装饰器：%v", err)
	}
	return db, log, inner, costs
}

func appendAfterCosts(
	ctx context.Context, t *testing.T, log *adapter.RouteHandoffLogs, correlation string,
) error {
	return log.Append(ctx,
		logValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		logValue(t, domain.NewRequestCorrelationID, correlation),
		"digest-after-costs")
}

func digestLogged(t *testing.T, log *adapter.RouteHandoffLogs, correlation string) bool {
	t.Helper()
	_, found, err := log.FindDigest(t.Context(),
		logValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		logValue(t, domain.NewRequestCorrelationID, correlation))
	if err != nil {
		t.Fatalf("读回指纹：%v", err)
	}
	return found
}

// Covers: ports.RouteCandidateCostSource「读失败不得连累调用方的事务」——内层在事务里执行 SELECT 1/0，装饰器把错误原样交回；
// 同一事务随后的写照样提交、读得回。没有这层隔离，事务被打成中止态，随后的写答 25P02，提交只拿到 rollback。
func TestASavepointKeepsAFailedCostReadFromAbortingTheJudgment(t *testing.T) {
	db, log, inner, costs := candidateCostsSavepointFixture(t, `SELECT (1/0)::bigint`)

	var readErr error
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, readErr = costs.LoadCandidateCosts(txCtx, domain.InitialRouteJudgmentKey{}, ports.InitialRouteEvidence{},
			ports.RequestCarriedContent{})
		return appendAfterCosts(txCtx, t, log, "corr-after-failed-costs")
	})
	if readErr == nil || inner.asked != 1 {
		t.Fatalf("内层问了 %d 次、错误 %v, want 问一次并交回 SELECT 1/0 的错误", inner.asked, readErr)
	}
	if err != nil {
		t.Fatalf("取数侧读失败连带了随后的写：%v", err)
	}
	if !digestLogged(t, log, "corr-after-failed-costs") {
		t.Fatal("提交之后读不回随后写的那一行")
	}
}

// Covers: 装饰器的可读一支——RELEASE 之后成本事实原样交回，同一事务随后的写照常提交。
func TestASavepointReleasesReadableCostsUnchanged(t *testing.T) {
	db, log, _, costs := candidateCostsSavepointFixture(t, `SELECT 4200::bigint`)

	var loaded ports.RouteCandidateCosts
	var readErr error
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		loaded, readErr = costs.LoadCandidateCosts(txCtx, domain.InitialRouteJudgmentKey{}, ports.InitialRouteEvidence{},
			ports.RequestCarriedContent{})
		return appendAfterCosts(txCtx, t, log, "corr-after-readable-costs")
	})
	if readErr != nil || len(loaded.Facts) != 1 || loaded.Facts[0].AmountMinor() != 4200 {
		t.Fatalf("读回 %+v err=%v, want 一条 4200", loaded.Facts, readErr)
	}
	if err != nil {
		t.Fatalf("提交：%v", err)
	}
	if !digestLogged(t, log, "corr-after-readable-costs") {
		t.Fatal("提交之后读不回随后写的那一行")
	}
}

// Covers: 装饰器的无事务一支——ctx 不在事务里时直通内层，不发 SAVEPOINT（事务块外发它本身就报错），成本与错误原样交回；
// 取数侧答没接时哨兵穿得过这一层，编排照旧认得出。
func TestASavepointPassesCostsThroughOutsideATransaction(t *testing.T) {
	db, _, inner, costs := candidateCostsSavepointFixture(t, `SELECT 3100::bigint`)
	if _, err := db.RequireExecutor(t.Context()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("前提不成立：ctx 应在事务外，RequireExecutor 却答 %v", err)
	}
	loaded, err := costs.LoadCandidateCosts(t.Context(), domain.InitialRouteJudgmentKey{}, ports.InitialRouteEvidence{},
		ports.RequestCarriedContent{})
	if err != nil || len(loaded.Facts) != 1 || loaded.Facts[0].AmountMinor() != 3100 || inner.asked != 1 {
		t.Fatalf("读回 %+v err=%v 问内层 %d 次, want 直通一次", loaded.Facts, err, inner.asked)
	}

	unconfigured, err := adapter.NewSavepointedRouteCandidateCosts(db, unconfiguredCosts{})
	if err != nil {
		t.Fatalf("构造装饰器：%v", err)
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := unconfigured.LoadCandidateCosts(txCtx, domain.InitialRouteJudgmentKey{}, ports.InitialRouteEvidence{},
			ports.RequestCarriedContent{})
		if !errors.Is(err, ports.ErrRouteCostSourceNotConfigured) {
			t.Errorf("事务里 err = %v, want 没接的哨兵原样交回", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("提交：%v", err)
	}
}

type unconfiguredCosts struct{}

func (unconfiguredCosts) LoadCandidateCosts(
	context.Context, domain.InitialRouteJudgmentKey, ports.InitialRouteEvidence, ports.RequestCarriedContent,
) (ports.RouteCandidateCosts, error) {
	return ports.RouteCandidateCosts{}, ports.ErrRouteCostSourceNotConfigured
}
