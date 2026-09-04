package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件让 ChannelSelectionDecisions 同时实现 ports.ChannelSelectionDecisionRead（票 label-channel/23）：
// 写侧登记册与运营查阅读口是同一只适配器，两个契约一只实现（判据同 TF 的 ExternalTrackingFacts 兼
// ExternalTrackingFactReviewRead）。读口只读列面，SQL 里不解释四格之外的任何语义——「并列冲突」就是
// conclusion 列等于 TIED 那个字面，字面取自领域封闭集的 String()，与迁移 CHECK 同一集合。
//
// 读回照旧走 rehydrateChannelSelectionDecision：列面就是权威内容，跨行命题由重建门核（ADR-0028），
// 查阅面不因为「只是看看」就绕开那道门——绕开了，一条坏记录会在页面上长得像合法的。

var _ ports.ChannelSelectionDecisionRead = (*ChannelSelectionDecisions)(nil)

// ListTiedChannelSelectionDecisions 按租户列并列冲突，按决定时刻倒序、同刻按标识倒序，limit 截页。
// 收窄对象时多两个等值条件，走与 ListBySubject 同一个索引（tenant_id, scope_ref, mapping_ref, decided_at）。
// 不按时间隐式截断：留痕要求属实例半边，窄口由调用方显式给。
func (registry *ChannelSelectionDecisions) ListTiedChannelSelectionDecisions(
	ctx context.Context,
	tenant domain.TenantID,
	filter ports.TiedChannelSelectionFilter,
	limit int,
) ([]domain.ChannelSelectionDecision, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list tied channel selection decisions: limit must be positive, got %d", limit)
	}
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tied channel selection decisions: %w", err)
	}

	query := `SELECT decision_id, scope_ref, mapping_ref, assembled_as_of, rule, decided_at, conclusion
		   FROM parcel_shipment.channel_selection_decision
		  WHERE tenant_id = $1
		    AND conclusion = $2`
	args := []any{tenant.String(), domain.ChannelSelectionConcludedTied.String()}
	if subject, narrowed := filter.Subject(); narrowed {
		query += `
		    AND scope_ref = $3
		    AND mapping_ref = $4`
		args = append(args, subject.Scope().String(), subject.Mapping().String())
	}
	query += fmt.Sprintf(`
		  ORDER BY decided_at DESC, decision_id DESC
		  LIMIT $%d`, len(args)+1)
	args = append(args, limit)

	heads, err := scanChannelSelectionDecisionHeads(ctx, querier, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tied channel selection decisions: %w", err)
	}
	if len(heads) == 0 {
		return []domain.ChannelSelectionDecision{}, nil
	}
	decisions, err := rehydrateChannelSelectionDecisions(ctx, querier, tenant, heads)
	if err != nil {
		return nil, fmt.Errorf("list tied channel selection decisions: %w", err)
	}
	return decisions, nil
}

// FindChannelSelectionDecision 按（租户 + 决定标识）取一条。租户维在 SQL 条件上：别的租户拿同一个标识
// 得到的是「没有」，与真不存在同答（ADR-0029 探针纪律）。
func (registry *ChannelSelectionDecisions) FindChannelSelectionDecision(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.ChannelSelectionDecisionID,
) (domain.ChannelSelectionDecision, bool, error) {
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ChannelSelectionDecision{}, false, fmt.Errorf("find channel selection decision: %w", err)
	}
	heads, err := scanChannelSelectionDecisionHeads(ctx, querier,
		`SELECT decision_id, scope_ref, mapping_ref, assembled_as_of, rule, decided_at, conclusion
		   FROM parcel_shipment.channel_selection_decision
		  WHERE tenant_id = $1
		    AND decision_id = $2`,
		tenant.String(), id.String(),
	)
	if err != nil {
		return domain.ChannelSelectionDecision{}, false, fmt.Errorf("find channel selection decision: %w", err)
	}
	if len(heads) == 0 {
		return domain.ChannelSelectionDecision{}, false, nil
	}
	decisions, err := rehydrateChannelSelectionDecisions(ctx, querier, tenant, heads)
	if err != nil {
		return domain.ChannelSelectionDecision{}, false, fmt.Errorf("find channel selection decision: %w", err)
	}
	return decisions[0], true, nil
}

// scanChannelSelectionDecisionHeads 跑一条只选头行七列的查询并按查询给的顺序读回。
func scanChannelSelectionDecisionHeads(
	ctx context.Context,
	querier bentopg.Querier,
	query string,
	args ...any,
) ([]channelSelectionDecisionRow, error) {
	rows, err := querier.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var heads []channelSelectionDecisionRow
	for rows.Next() {
		var head channelSelectionDecisionRow
		if err := rows.Scan(&head.id, &head.scope, &head.mapping, &head.assembledAsOf,
			&head.rule, &head.decidedAt, &head.conclusion); err != nil {
			return nil, err
		}
		heads = append(heads, head)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return heads, nil
}

// rehydrateChannelSelectionDecisions 给一批头行取子行并逐条过重建门，保持头行的顺序。子行按标识集合一次
// 取回（ANY($2)）而不是逐条查：一页并列冲突的候选行一次读完，读口的往返次数不随页大小长。
func rehydrateChannelSelectionDecisions(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	heads []channelSelectionDecisionRow,
) ([]domain.ChannelSelectionDecision, error) {
	identifiers := make([]string, 0, len(heads))
	for _, head := range heads {
		identifiers = append(identifiers, head.id)
	}
	candidateRows, err := querier.Query(ctx,
		`SELECT decision_id, candidate_ref, evaluation_ref, outcome, exclusion
		   FROM parcel_shipment.channel_selection_candidate
		  WHERE tenant_id = $1
		    AND decision_id = ANY($2)
		  ORDER BY decision_id, position`,
		tenant.String(), identifiers,
	)
	if err != nil {
		return nil, fmt.Errorf("load channel selection candidates: %w", err)
	}
	defer candidateRows.Close()
	candidatesOf := make(map[string][]channelSelectionCandidateRow, len(heads))
	for candidateRows.Next() {
		var row channelSelectionCandidateRow
		if err := candidateRows.Scan(&row.decision, &row.candidate, &row.evaluation, &row.outcome, &row.exclusion); err != nil {
			return nil, fmt.Errorf("load channel selection candidates: %w", err)
		}
		candidatesOf[row.decision] = append(candidatesOf[row.decision], row)
	}
	if err := candidateRows.Err(); err != nil {
		return nil, fmt.Errorf("load channel selection candidates: %w", err)
	}

	decisions := make([]domain.ChannelSelectionDecision, 0, len(heads))
	for _, head := range heads {
		decision, err := rehydrateChannelSelectionDecision(tenant, head, candidatesOf[head.id])
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}
