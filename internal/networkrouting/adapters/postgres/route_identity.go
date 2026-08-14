package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// routePlanVersionPrefix 让签发出来的标识一眼看得出是什么东西的版本号。它只是前缀，
// 不承载语义——版本号不得可解析出租户、包裹或时点，那些维度已经在判断键上。
const routePlanVersionPrefix = "RPV-"

// RouteIdentities 实现 ports.RouteIdentityFactory：用 network_routing 自己的序列签发
// 计划版本标识。序列而不是取自调用方，正是端口那句「交接关联不得变成计划版本号」的
// 落点——两者一旦同源，重放同一次交接就会撞出同一个版本号，而改路要求新旧两代分得开。
type RouteIdentities struct {
	db *bentopg.DB
}

func NewRouteIdentities(db *bentopg.DB) (*RouteIdentities, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &RouteIdentities{db: db}, nil
}

var _ ports.RouteIdentityFactory = (*RouteIdentities)(nil)

// NextRoutePlanVersionID 取序列的下一个值。nextval 不随事务回滚：一次未能提交的判断
// 会烧掉一个号，但绝不会把这个号让给下一次判断——号段连续没有业务含义，两个计划共用
// 一个版本号却会让 plan_applicability 的主键把它们压成一行。
//
// 走 RequireExecutor 而不是 ReadExecutor：nextval 推进序列状态，是写不是读，发到只读
// 连接上会失败，发到读副本上更糟——那里推进的是另一份序列状态。
func (factory *RouteIdentities) NextRoutePlanVersionID(
	ctx context.Context,
) (domain.RoutePlanVersionID, error) {
	executor, err := factory.db.RequireExecutor(ctx)
	if err != nil {
		return domain.RoutePlanVersionID{}, fmt.Errorf("next route plan version: %w", err)
	}

	var sequence int64
	err = executor.QueryRow(ctx,
		`SELECT nextval('network_routing.route_plan_version_seq')`,
	).Scan(&sequence)
	if err != nil {
		return domain.RoutePlanVersionID{}, fmt.Errorf("next route plan version: %w", err)
	}

	// 零填充到 12 位只为可读与可排序；序列超过 12 位时 %012d 自然变长，不截断。
	return domain.NewRoutePlanVersionID(fmt.Sprintf("%s%012d", routePlanVersionPrefix, sequence))
}
