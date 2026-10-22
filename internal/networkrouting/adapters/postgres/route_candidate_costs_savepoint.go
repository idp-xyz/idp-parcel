package postgres

import (
	"context"
	"errors"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// candidateCostsSavepoint 是装饰器在调用方事务里开的 savepoint 名。同一笔事务里逐件包裹、逐次复核先后开、各自收，名字重用无妨；
// 取数侧内层不开同名的点，退回时不会退到别人的点上。
const candidateCostsSavepoint = "network_routing_candidate_costs"

// SavepointedRouteCandidateCosts 把候选成本取数侧隔在一个 savepoint 里，兑现 ports.RouteCandidateCostSource「读失败不连累调用方
// 的事务」那一条。
//
// 取数侧逐段读网络目录、商业发布、价卡、参考序列与包裹事实，这些仓储读口在事务里走调用方那一笔事务。初始路由与复核都跑在
// 消费门的 inbox 事务里，一条 SQL 级读错误（表被挪走、权限、语句超时）会把整笔事务打进中止态：复核失效线上失效与复核记录随即
// 写不进，仍适用线上已判的仍适用丢掉，两格都被报成写库故障；初始路由那一件随后的提交前重校读不动，原因被报成网络证据读不动。
// 编排写定的落法（候选评估未决、保留原计划、成本读不回）只有在读失败不碰事务时才在库里成立。
//
// 内层失败就退回 savepoint、把错误原样交回，各条线据此落各自的格；成功就 RELEASE。ctx 不在事务里时直通：没有可保护的写，
// 事务块外也开不了 savepoint。
//
// 落在适配器里、不经 bento 的事务口：bento 的 WithinTransaction 嵌套时只沿用外层事务，内层失败即把整笔标成只回滚，没有
// savepoint 语义；savepoint 只经它的写执行器发 SQL，领域与应用层不碰 pgx。做法同节点作业的 SavepointedServiceResultView
// （ADR-0186 决定七）。
type SavepointedRouteCandidateCosts struct {
	db    *bentopg.DB
	inner ports.RouteCandidateCostSource
}

var _ ports.RouteCandidateCostSource = (*SavepointedRouteCandidateCosts)(nil)

func NewSavepointedRouteCandidateCosts(
	db *bentopg.DB,
	inner ports.RouteCandidateCostSource,
) (*SavepointedRouteCandidateCosts, error) {
	if db == nil {
		return nil, errors.New("network routing postgres: db is nil")
	}
	if inner == nil {
		return nil, errors.New("network routing postgres: route candidate cost source is nil")
	}
	return &SavepointedRouteCandidateCosts{db: db, inner: inner}, nil
}

func (costs *SavepointedRouteCandidateCosts) LoadCandidateCosts(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
	evidence ports.InitialRouteEvidence,
	carried ports.RequestCarriedContent,
) (ports.RouteCandidateCosts, error) {
	executor, err := costs.db.RequireExecutor(ctx)
	if errors.Is(err, bentopg.ErrTransactionRequired) {
		return costs.inner.LoadCandidateCosts(ctx, key, evidence, carried)
	}
	if err != nil {
		return ports.RouteCandidateCosts{}, fmt.Errorf("candidate costs savepoint: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT "+candidateCostsSavepoint); err != nil {
		return ports.RouteCandidateCosts{}, fmt.Errorf("candidate costs savepoint: %w", err)
	}
	loaded, loadErr := costs.inner.LoadCandidateCosts(ctx, key, evidence, carried)
	if loadErr != nil {
		if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+candidateCostsSavepoint); err != nil {
			// 退不回 savepoint，事务就还在中止态，随后的写会自己失败、整笔回滚；两条错误都交回，免得只剩一半原因。
			return ports.RouteCandidateCosts{}, errors.Join(loadErr, fmt.Errorf("candidate costs savepoint rollback: %w", err))
		}
		return ports.RouteCandidateCosts{}, loadErr
	}
	if _, err := executor.Exec(ctx, "RELEASE SAVEPOINT "+candidateCostsSavepoint); err != nil {
		return ports.RouteCandidateCosts{}, fmt.Errorf("candidate costs savepoint release: %w", err)
	}
	return loaded, nil
}
