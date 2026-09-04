package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ChannelSelectionDecisions 实现 ports.ChannelSelectionDecisionRegistry：「渠道择优决定」记录的只追加
// 登记册（票 label-channel/14）。头行落 channel_selection_decision，逐候选结果落
// channel_selection_candidate，同一事务写入。
//
// 与 LabelTransactions / ContinuedAttemptRegisters 的「快照 + 列面」纹样不同属：本记录没有需要整份
// 重验的内部结构，权威内容就在列上，读回逐字段过领域构造函数再进 RehydrateChannelSelectionDecision
// ——单行形状由库内 CHECK 钉，跨行命题（选中至多一个且与并列互斥、结论与结果互证）由重建门核
// （ADR-0028）。两层各自有牙。
//
// 本适配器今天没有生产装配点：择优编排 SelectChannelCandidateHandler 自身尚无组合根（票 12 收口
// Comment「生产可达仍差两步」），留痕随那条接线一起可达。机制先立起来，接线那天对着的不是一张裸表。
type ChannelSelectionDecisions struct {
	db *bentopg.DB
}

func NewChannelSelectionDecisions(db *bentopg.DB) (*ChannelSelectionDecisions, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &ChannelSelectionDecisions{db: db}, nil
}

var _ ports.ChannelSelectionDecisionRegistry = (*ChannelSelectionDecisions)(nil)

// Append 落一条决定：头行 ON CONFLICT DO NOTHING，零行命中即`已登记`（同一标识重放到位，不是两次
// 择优——标识由签发口铸），子行随头行同事务写入。撞键用 DO NOTHING 而不是捕 23505：撞键的 INSERT
// 会把整个事务打进中止态，而编排还要在同一事务里继续。
func (registry *ChannelSelectionDecisions) Append(
	ctx context.Context,
	decision domain.ChannelSelectionDecision,
) (ports.ChannelSelectionDecisionAppendOutcome, error) {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ChannelSelectionDecisionAppendOutcomeInvalid, fmt.Errorf("append channel selection decision: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.channel_selection_decision
			(tenant_id, decision_id, scope_ref, mapping_ref, assembled_as_of, rule, decided_at, conclusion)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		decision.Tenant().String(),
		decision.ID().String(),
		decision.Subject().Scope().String(),
		decision.Subject().Mapping().String(),
		decision.AssembledAsOf(),
		decision.Rule().String(),
		decision.DecidedAt(),
		decision.Conclusion().String(),
	)
	if err != nil {
		return ports.ChannelSelectionDecisionAppendOutcomeInvalid, fmt.Errorf("append channel selection decision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ChannelSelectionDecisionAlreadyRecorded, nil
	}

	for position, result := range decision.Results() {
		var evaluation, exclusion sql.NullString
		if reference, present := result.Evaluation(); present {
			evaluation = sql.NullString{String: reference.String(), Valid: true}
		}
		if grade, excluded := result.Exclusion(); excluded {
			exclusion = sql.NullString{String: grade.String(), Valid: true}
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO parcel_shipment.channel_selection_candidate
				(tenant_id, decision_id, position, candidate_ref, evaluation_ref, outcome, exclusion)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			decision.Tenant().String(),
			decision.ID().String(),
			position+1,
			result.Candidate().String(),
			evaluation,
			result.Outcome().String(),
			exclusion,
		); err != nil {
			return ports.ChannelSelectionDecisionAppendOutcomeInvalid, fmt.Errorf("append channel selection candidate: %w", err)
		}
	}
	return ports.ChannelSelectionDecisionAppended, nil
}

// channelSelectionDecisionRow 是头行读回的一行。
type channelSelectionDecisionRow struct {
	id            string
	scope         string
	mapping       string
	assembledAsOf time.Time
	rule          string
	decidedAt     time.Time
	conclusion    string
}

// channelSelectionCandidateRow 是子行读回的一行；评价引用与出局因由可空。
type channelSelectionCandidateRow struct {
	decision   string
	candidate  string
	evaluation sql.NullString
	outcome    string
	exclusion  sql.NullString
}

// ListBySubject 按（租户 + 被择优对象）取回全部决定，按决定时刻升序、同刻按标识升序。子行按
// position 升序，读回的逐候选次序与形成时一致。两次查询走同一个读执行器；结果在内存里按决定
// 标识归并后逐条过重建门。
func (registry *ChannelSelectionDecisions) ListBySubject(
	ctx context.Context,
	tenant domain.TenantID,
	subject domain.ChannelSelectionSubject,
) ([]domain.ChannelSelectionDecision, error) {
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list channel selection decisions: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT decision_id, scope_ref, mapping_ref, assembled_as_of, rule, decided_at, conclusion
		   FROM parcel_shipment.channel_selection_decision
		  WHERE tenant_id = $1
		    AND scope_ref = $2
		    AND mapping_ref = $3
		  ORDER BY decided_at, decision_id`,
		tenant.String(), subject.Scope().String(), subject.Mapping().String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list channel selection decisions: %w", err)
	}
	var heads []channelSelectionDecisionRow
	for rows.Next() {
		var head channelSelectionDecisionRow
		if err := rows.Scan(&head.id, &head.scope, &head.mapping, &head.assembledAsOf,
			&head.rule, &head.decidedAt, &head.conclusion); err != nil {
			rows.Close()
			return nil, fmt.Errorf("list channel selection decisions: %w", err)
		}
		heads = append(heads, head)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list channel selection decisions: %w", err)
	}
	if len(heads) == 0 {
		return []domain.ChannelSelectionDecision{}, nil
	}

	candidateRows, err := querier.Query(ctx,
		`SELECT c.decision_id, c.candidate_ref, c.evaluation_ref, c.outcome, c.exclusion
		   FROM parcel_shipment.channel_selection_candidate c
		   JOIN parcel_shipment.channel_selection_decision d
		     ON d.tenant_id = c.tenant_id AND d.decision_id = c.decision_id
		  WHERE c.tenant_id = $1
		    AND d.scope_ref = $2
		    AND d.mapping_ref = $3
		  ORDER BY c.decision_id, c.position`,
		tenant.String(), subject.Scope().String(), subject.Mapping().String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list channel selection candidates: %w", err)
	}
	candidatesOf := make(map[string][]channelSelectionCandidateRow, len(heads))
	for candidateRows.Next() {
		var row channelSelectionCandidateRow
		if err := candidateRows.Scan(&row.decision, &row.candidate, &row.evaluation, &row.outcome, &row.exclusion); err != nil {
			candidateRows.Close()
			return nil, fmt.Errorf("list channel selection candidates: %w", err)
		}
		candidatesOf[row.decision] = append(candidatesOf[row.decision], row)
	}
	candidateRows.Close()
	if err := candidateRows.Err(); err != nil {
		return nil, fmt.Errorf("list channel selection candidates: %w", err)
	}

	decisions := make([]domain.ChannelSelectionDecision, 0, len(heads))
	for _, head := range heads {
		decision, err := rehydrateChannelSelectionDecision(tenant, head, candidatesOf[head.id])
		if err != nil {
			return nil, fmt.Errorf("list channel selection decisions: %w", err)
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

// rehydrateChannelSelectionDecision 把头行与子行逐字段过领域构造函数，再进重建门。列上的封闭集
// 取值经各自的译码函数进领域类型——认不出的取值在这里响，不落进某一格。
func rehydrateChannelSelectionDecision(
	tenant domain.TenantID,
	head channelSelectionDecisionRow,
	candidates []channelSelectionCandidateRow,
) (domain.ChannelSelectionDecision, error) {
	id, err := domain.NewChannelSelectionDecisionID(head.id)
	if err != nil {
		return domain.ChannelSelectionDecision{}, err
	}
	scope, err := domain.NewCommercialScopeReference(head.scope)
	if err != nil {
		return domain.ChannelSelectionDecision{}, err
	}
	mapping, err := domain.NewProductChannelMappingReference(head.mapping)
	if err != nil {
		return domain.ChannelSelectionDecision{}, err
	}
	subject, err := domain.NewChannelSelectionSubject(scope, mapping)
	if err != nil {
		return domain.ChannelSelectionDecision{}, err
	}
	conclusion, err := channelSelectionConclusionFrom(head.conclusion)
	if err != nil {
		return domain.ChannelSelectionDecision{}, err
	}

	results := make([]domain.ChannelCandidateResultSpec, 0, len(candidates))
	for _, row := range candidates {
		candidate, err := domain.NewChannelCandidateID(row.candidate)
		if err != nil {
			return domain.ChannelSelectionDecision{}, err
		}
		outcome, err := channelCandidateOutcomeFrom(row.outcome)
		if err != nil {
			return domain.ChannelSelectionDecision{}, err
		}
		result := domain.ChannelCandidateResultSpec{Candidate: candidate, Outcome: outcome}
		if row.evaluation.Valid {
			evaluation, err := domain.NewChannelCostEvaluationReference(row.evaluation.String)
			if err != nil {
				return domain.ChannelSelectionDecision{}, err
			}
			result.Evaluation = evaluation
		}
		if row.exclusion.Valid {
			grade, err := channelCostUnavailabilityFrom(row.exclusion.String)
			if err != nil {
				return domain.ChannelSelectionDecision{}, err
			}
			result.Exclusion = grade
		}
		results = append(results, result)
	}

	return domain.RehydrateChannelSelectionDecision(domain.RehydrateChannelSelectionDecisionSpec{
		ID:            id,
		Tenant:        tenant,
		Subject:       subject,
		AssembledAsOf: head.assembledAsOf,
		Rule:          domain.ChannelSelectionRule(head.rule),
		DecidedAt:     head.decidedAt,
		Conclusion:    conclusion,
		Results:       results,
	})
}

func channelSelectionConclusionFrom(value string) (domain.ChannelSelectionConclusion, error) {
	for _, candidate := range []domain.ChannelSelectionConclusion{
		domain.ChannelSelectionConcludedSelected,
		domain.ChannelSelectionConcludedTied,
		domain.ChannelSelectionConcludedNoneQualified,
	} {
		if candidate.String() == value {
			return candidate, nil
		}
	}
	return domain.ChannelSelectionConclusionInvalid, fmt.Errorf("parcel shipment postgres: unknown channel selection conclusion %q", value)
}

func channelCandidateOutcomeFrom(value string) (domain.ChannelCandidateOutcome, error) {
	for _, candidate := range []domain.ChannelCandidateOutcome{
		domain.ChannelCandidateSelected,
		domain.ChannelCandidateNotSelected,
		domain.ChannelCandidateExcluded,
		domain.ChannelCandidateTied,
	} {
		if candidate.String() == value {
			return candidate, nil
		}
	}
	return domain.ChannelCandidateOutcomeInvalid, fmt.Errorf("parcel shipment postgres: unknown channel candidate outcome %q", value)
}

func channelCostUnavailabilityFrom(value string) (domain.ChannelCostUnavailability, error) {
	for _, candidate := range []domain.ChannelCostUnavailability{
		domain.ChannelCostPendingEvidence,
		domain.ChannelCostRatecardExclusion,
		domain.ChannelCostConflict,
		domain.ChannelCostNotFormed,
	} {
		if candidate.String() == value {
			return candidate, nil
		}
	}
	return domain.ChannelCostUnavailabilityInvalid, fmt.Errorf("parcel shipment postgres: unknown channel cost unavailability %q", value)
}
