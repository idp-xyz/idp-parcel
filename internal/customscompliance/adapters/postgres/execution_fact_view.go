package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ExecutionFactView 实现 ports.ExecutionFactView：按监管决定读回执行方已形成的物理
// 执行事实引用。
//
// 读的是本上下文自己的引用册，不是节点或运输的表。事实本体归执行方所有
// （CONTEXT-MAP `NO → CC` 提供实测、查验协作与处置执行事实；node-operations CONTEXT
// 「关务只引用节点结果，双方不得共同编辑」），本上下文只登记已经收到的引用并据以
// 核对。跨模块直读对方的表会同时破掉这条所有权边界和包布局那条「不能直接读取其他
// 模块拥有的表」。
//
// 空清单是如实答案：决定推导不出执行，没有事实就是证据不足（领域折出`证据不足`）。
// 依赖调不通照原样上抛——空清单与读不回若合并，一次故障就会被记成一次「执行方什么
// 也没做」的核对判断，而那份判断会带着指纹落库成版。
type ExecutionFactView struct {
	db *bentopg.DB
}

func NewExecutionFactView(db *bentopg.DB) (*ExecutionFactView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &ExecutionFactView{db: db}, nil
}

var _ ports.ExecutionFactView = (*ExecutionFactView)(nil)

// LoadExecutionFacts 按发生时间取回该决定名下的全部事实引用。部分执行与再次执行各是
// 一份，迟到与重复各自成行——不在这里合并或去重：核对要看的正是「逐范围、逐数量」的
// 明细，合并会把差异抹平成一个看起来刚好覆盖的总数。
func (view *ExecutionFactView) LoadExecutionFacts(
	ctx context.Context,
	tenant domain.TenantID,
	decision domain.RegulatoryDecisionID,
) ([]domain.ExecutionFact, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load execution facts: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT executor_ref, fact_ref, scope_ref, units, occurred_at
		   FROM customs_compliance.execution_fact
		  WHERE tenant_id = $1 AND decision_id = $2
		  ORDER BY occurred_at, fact_ref`,
		tenant.String(), decision.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("load execution facts: %w", err)
	}
	defer rows.Close()

	facts := make([]domain.ExecutionFact, 0)
	for rows.Next() {
		var (
			executorRef, factRef, scopeRef string
			units                          int
			occurredAt                     time.Time
		)
		if err := rows.Scan(&executorRef, &factRef, &scopeRef, &units, &occurredAt); err != nil {
			return nil, fmt.Errorf("load execution facts: %w", err)
		}
		executor, err := domain.NewExecutorReference(executorRef)
		if err != nil {
			return nil, fmt.Errorf("load execution facts: %w", err)
		}
		fact, err := domain.NewExecutionFactReference(factRef)
		if err != nil {
			return nil, fmt.Errorf("load execution facts: %w", err)
		}
		scope, err := domain.NewDecisionScopeReference(scopeRef)
		if err != nil {
			return nil, fmt.Errorf("load execution facts: %w", err)
		}
		built, err := domain.NewExecutionFact(domain.ExecutionFactSpec{
			Executor:   executor,
			Fact:       fact,
			Scope:      scope,
			Units:      units,
			OccurredAt: occurredAt,
		})
		if err != nil {
			return nil, fmt.Errorf("load execution facts: %w", err)
		}
		facts = append(facts, built)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load execution facts: %w", err)
	}
	return facts, nil
}
