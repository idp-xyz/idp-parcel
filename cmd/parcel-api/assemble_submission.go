package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	pspilot "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/pilotgovernance"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// envDatabaseDSN 与 parcel-dispatch 同名：两个进程读同一个部署形态参数，分开命名只会
// 让同一台机器上的同一个库看起来像两份配置。
const envDatabaseDSN = "IDP_PARCEL_POSTGRES_DSN"

// submissionOwnershipAnswerValidity 是归属桥的答复视界，值由审计票 13 裁定
// （`.scratch/syn-wall-door-audit/issues/13-production-ownership-bridge-has-no-assembly-point.md`）：
// 语义是「单次提交判断的处理视界」，答复不跨提交复用——上界要短到暂停与接管在下一次
// 提交判断时必然生效，不存在一份仍在窗内的旧答复替新提交作数；下界要长过单次 Handle
// 内从形成到使用的间隙。登记册里有终点的区间仍会更早截断（适配器取声明视界与登记终点
// 中更早者）。若日后要跨请求复用答复，那是新决定，按 AGENTS 走 ADR，不改这个数了事。
const submissionOwnershipAnswerValidity = time.Minute

// systemClock 是 Clock 端口的生产实现，与 parcel-dispatch 同形：要的是真实时钟，
// `time.Now()` 就是它，不是替身。
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// openDatabase 读部署形态并开真实连接池。步骤与理由随 parcel-dispatch 的装配点：
// 建池是惰性的，库可达性由这一次 Ping 认定；`CheckSchema` 排在 Ping 之后，让「缺框架
// schema」与「连不上」在日志里分成两格；装配中途失败就地关池——半开的池会在下一次
// 尝试时耗掉连接数，而那种耗尽看起来像数据库出了问题。
//
// DSN 缺席让进程带原因退出，不静默降级回占位编排：提交编排接真之后，库就是本进程的
// 部署形态之一，缺它是部署坏了，不是一种可运行的形态——静默降级只会把部署错误伪装成
// 运行时故障（与 parcel-dispatch「装配点失败让进程带着原因退出」同一条理由）。
func openDatabase(ctx context.Context, getenv func(string) string) (*bentopg.DB, func(), error) {
	dsn := getenv(envDatabaseDSN)
	if dsn == "" {
		return nil, nil, fmt.Errorf("parcel-api: %s is required", envDatabaseDSN)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("parcel-api: ping: %w", err)
	}
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("parcel-api: framework db: %w", err)
	}
	if err := db.CheckSchema(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("parcel-api: schema check: %w", err)
	}
	return db, pool.Close, nil
}

// transactionalSubmission 把提交编排包进一笔事务。PS 的写口按框架合同走
// RequireExecutor，无事务即拒，事务边界因此归装配点，编排自己不开也不提交。
//
// 编排交回业务答案（含被门禁拦下的那几格）时事务提交——被拦下的提交也已保全来源，
// 重放同一份输入答`已有结果`正是靠这笔提交；编排返回错误时整笔回滚，半截写入不落库。
type transactionalSubmission struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.SubmitShipmentRequestHandler
}

var _ shipmenthttp.SubmissionHandler = transactionalSubmission{}

func (submission transactionalSubmission) Handle(
	ctx context.Context,
	command shipmentapp.SubmitShipmentRequestCommand,
) (shipmentapp.SubmitShipmentRequestResult, error) {
	var result shipmentapp.SubmitShipmentRequestResult
	err := submission.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := submission.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return shipmentapp.SubmitShipmentRequestResult{}, err
	}
	return result, nil
}

// buildSubmissionOrchestration 装配 `/shipment-requests` 的真编排（UC-PS-001；审计票 13）。
//
// 治理桥的三个读口（权威区间、暂停、接管）接真库；GovernanceScopeDirectory 与
// SelfAuthority 是范围缝的实例半边——没有租户就没有目录，缺席即适配器的「显式未配置」，
// 归属如实答`权威未确定`，提交停在 OWNERSHIP_UNRESOLVED。
//
// 与接线前的区别不在结果，在结果的来源，但这句话只对那三个读口成立：它们的恢复动作
// 已从「写代码」变成「登记」（ADR-0063 的「显式未配置」讲的就是这个差别）。**下面留空
// 的两样不在其内**——`governanceScope` 在目录缺席或自身权威串为空时先于读登记册就早退，
// 因此往 authority_interval 里登记多少行都不会改变答案，那两格的恢复动作仍是「写一个
// 目录实现并说出自己的权威串」。把它们读成「登记一条区间就好」会让人登完仍撞
// OWNERSHIP_UNRESOLVED 而不知道为什么。
func buildSubmissionOrchestration(db *bentopg.DB) (shipmenthttp.SubmissionHandler, error) {
	sources, err := pspostgres.NewSourceSubmissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: source submissions: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	intervals, err := pgpostgres.NewAuthorityIntervals(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: authority intervals: %w", err)
	}
	suspensions, err := pgpostgres.NewSuspensions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: suspensions: %w", err)
	}
	takeovers, err := pgpostgres.NewTakeovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: takeovers: %w", err)
	}
	clock := systemClock{}
	ownership, err := pspilot.NewProductionOwnershipAdapter(pspilot.ProductionOwnershipAdapterDeps{
		Intervals:   intervals,
		Suspensions: suspensions,
		Handoffs:    takeovers,
		// Directory 与 SelfAuthority 留空：实例半边，租户随试点登记后在此填上；
		// 在那之前适配器对缺席的回答就是`权威未确定`，不代拟任何坐标。
		Clock:          clock,
		AnswerValidity: submissionOwnershipAnswerValidity,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: production ownership adapter: %w", err)
	}
	identities, err := psidentity.NewSubmissionIdentities()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: submission identities: %w", err)
	}
	handler := shipmentapp.NewSubmitShipmentRequestHandler(sources, requests, ownership, identities, clock)
	return transactionalSubmission{transactor: db.Transactor(), inner: handler}, nil
}
