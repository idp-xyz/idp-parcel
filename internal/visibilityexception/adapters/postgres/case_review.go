package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// CaseReview 实现案件侧三张查阅页的伴生列表读端口（ports.TriageReviewRead、
// ports.CaseReviewRead、ports.ClaimsRecoveryReviewRead；ADR-0077，票
// admin-skeleton-closure-batch/06）。目录侧的列表读面在 catalogue_read.go，
// 本类型不触碰。
//
// 一个类型实现三组端口：三页共用同一读库与同一转写纪律，分设三个类型只会让装配点
// 看起来能只配一半。目录上列是照实转写，不经领域重建门：重建是写路与按键读回的
// 纪律（坏行要在那里响亮），检索列面把登记的字段原样透出。发作期行连同分诊结论
// 同键左联（0002 两表同笔提交）；索赔行按补充期限子表计版本数；追偿行按动作子表
// 各种类取最近节点——连、计、取末都是转写不是判断。
//
// 所有语句显式携带租户条件（ADR-0003）；读走 ReadExecutor，事务外用显式注入的
// 连接池。
type CaseReview struct {
	db *bentopg.DB
}

var _ ports.TriageReviewRead = (*CaseReview)(nil)
var _ ports.CaseReviewRead = (*CaseReview)(nil)
var _ ports.ClaimsRecoveryReviewRead = (*CaseReview)(nil)

func NewCaseReview(db *bentopg.DB) (*CaseReview, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &CaseReview{db: db}, nil
}

// ListSignalEpisodes 上列信号发作期册：新近首启在前，同刻按到达序（seq）再新近在前
// ——0002 立 seq 正是因为重开发作期的首命中允许与前期结束同刻，started_at 单列排
// 不出先后。
func (review *CaseReview) ListSignalEpisodes(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.SignalEpisodeCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list signal episodes: limit must be positive, got %d", limit)
	}
	querier, err := review.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list signal episodes: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT e.episode_id, e.parcel_ref, e.kind_ref, e.rule_ref, e.confidence_ref,
		        e.hits, e.started_at, e.last_hit_at,
		        COALESCE(e.release_basis, ''), e.ended_at,
		        COALESCE(e.prior_episode, ''),
		        COALESCE(c.outcome, ''), COALESCE(c.rule_ref, ''), c.triaged_at
		   FROM visibility_exception.signal_episode e
		   LEFT JOIN visibility_exception.triage_conclusion c
		     ON c.tenant_id = e.tenant_id
		    AND c.episode_id = e.episode_id
		  WHERE e.tenant_id = $1
		  ORDER BY e.started_at DESC, e.seq DESC
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list signal episodes: %w", err)
	}
	defer rows.Close()

	list := make([]ports.SignalEpisodeCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row       ports.SignalEpisodeCatalogueRow
			hits      int32
			endedAt   *time.Time
			triagedAt *time.Time
		)
		if err := rows.Scan(
			&row.EpisodeID, &row.Parcel, &row.Kind, &row.Rule, &row.Confidence,
			&hits, &row.StartedAt, &row.LastHitAt,
			&row.ReleaseBasis, &endedAt,
			&row.PriorEpisode,
			&row.Outcome, &row.OutcomeRule, &triagedAt,
		); err != nil {
			return nil, fmt.Errorf("list signal episodes: %w", err)
		}
		row.Hits = int64(hits)
		if endedAt != nil {
			utc := endedAt.UTC()
			row.EndedAt = &utc
		}
		if triagedAt != nil {
			utc := triagedAt.UTC()
			row.TriagedAt = &utc
		}
		row.StartedAt = row.StartedAt.UTC()
		row.LastHitAt = row.LastHitAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list signal episodes: %w", err)
	}
	return list, nil
}

// ListDispositionRequests 上列处置请求册：新近发送在前，同刻按请求标识稳定排序。
// 被替代的行照列——替代不是删除，版本链在册面上完整可见。
func (review *CaseReview) ListDispositionRequests(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.DispositionRequestCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list disposition requests: limit must be positive, got %d", limit)
	}
	querier, err := review.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list disposition requests: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT request_id, case_id, target_context, action_ref, scope_ref,
		        reason, evidence_ref, intent_version, sent_at, acceptance_window,
		        COALESCE(judgment, ''), judged_at,
		        COALESCE(cancellation, ''), COALESCE(superseded_by, '')
		   FROM visibility_exception.disposition_request
		  WHERE tenant_id = $1
		  ORDER BY sent_at DESC, request_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list disposition requests: %w", err)
	}
	defer rows.Close()

	list := make([]ports.DispositionRequestCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row              ports.DispositionRequestCatalogueRow
			intentVersion    int32
			acceptanceWindow *time.Time
			judgedAt         *time.Time
		)
		if err := rows.Scan(
			&row.RequestID, &row.CaseID, &row.TargetContext, &row.Action, &row.Scope,
			&row.Reason, &row.Evidence, &intentVersion, &row.SentAt, &acceptanceWindow,
			&row.Judgment, &judgedAt,
			&row.Cancellation, &row.SupersededBy,
		); err != nil {
			return nil, fmt.Errorf("list disposition requests: %w", err)
		}
		row.IntentVersion = int64(intentVersion)
		if acceptanceWindow != nil {
			utc := acceptanceWindow.UTC()
			row.AcceptanceWindow = &utc
		}
		if judgedAt != nil {
			utc := judgedAt.UTC()
			row.JudgedAt = &utc
		}
		row.SentAt = row.SentAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list disposition requests: %w", err)
	}
	return list, nil
}

// ListExceptionCases 上列异常案件册：新近建立在前，同刻按案件标识稳定排序。
func (review *CaseReview) ListExceptionCases(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ExceptionCaseCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list exception cases: limit must be positive, got %d", limit)
	}
	querier, err := review.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list exception cases: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT case_id, root_parcel, impact_scope, responsible_team, phase,
		        established_at, first_response, closed_at,
		        COALESCE(conclusion, ''), COALESCE(merged_into, '')
		   FROM visibility_exception.exception_case
		  WHERE tenant_id = $1
		  ORDER BY established_at DESC, case_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list exception cases: %w", err)
	}
	defer rows.Close()

	list := make([]ports.ExceptionCaseCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row           ports.ExceptionCaseCatalogueRow
			firstResponse *time.Time
			closedAt      *time.Time
		)
		if err := rows.Scan(
			&row.CaseID, &row.RootParcel, &row.ImpactScope, &row.ResponsibleTeam, &row.Phase,
			&row.EstablishedAt, &firstResponse, &closedAt,
			&row.Conclusion, &row.MergedInto,
		); err != nil {
			return nil, fmt.Errorf("list exception cases: %w", err)
		}
		if firstResponse != nil {
			utc := firstResponse.UTC()
			row.FirstResponse = &utc
		}
		if closedAt != nil {
			utc := closedAt.UTC()
			row.ClosedAt = &utc
		}
		row.EstablishedAt = row.EstablishedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list exception cases: %w", err)
	}
	return list, nil
}

// ListCustomerNotifications 上列客户异常通知册：新近决定在前，同刻按通知标识稳定
// 排序。过程节点整列照 milestones jsonb 转写（写侧 notificationMilestoneRow 的键
// 形状，两侧共一份）。
func (review *CaseReview) ListCustomerNotifications(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CustomerNotificationCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list customer notifications: limit must be positive, got %d", limit)
	}
	querier, err := review.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customer notifications: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT notification_id, customer_ref, episode_id, decided_at,
		        disclosure_policy_ref, content_ref, deadline,
		        channel_ref, obligation_ref, milestones
		   FROM visibility_exception.customer_notification
		  WHERE tenant_id = $1
		  ORDER BY decided_at DESC, notification_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list customer notifications: %w", err)
	}
	defer rows.Close()

	list := make([]ports.CustomerNotificationCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row           ports.CustomerNotificationCatalogueRow
			milestonesRaw []byte
		)
		if err := rows.Scan(
			&row.NotificationID, &row.Customer, &row.Episode, &row.DecidedAt,
			&row.Policy, &row.Content, &row.Deadline,
			&row.Channel, &row.Obligation, &milestonesRaw,
		); err != nil {
			return nil, fmt.Errorf("list customer notifications: %w", err)
		}
		var nodes []notificationMilestoneRow
		if err := json.Unmarshal(milestonesRaw, &nodes); err != nil {
			return nil, fmt.Errorf("list customer notifications: %w", err)
		}
		row.Milestones = make([]ports.NotificationMilestoneNode, 0, len(nodes))
		for _, node := range nodes {
			row.Milestones = append(row.Milestones, ports.NotificationMilestoneNode{
				Milestone:  node.Milestone,
				RecordedAt: node.RecordedAt.UTC(),
			})
		}
		row.DecidedAt = row.DecidedAt.UTC()
		row.Deadline = row.Deadline.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list customer notifications: %w", err)
	}
	return list, nil
}

// ListClaimItems 上列客户索赔项册：新近提交在前，同刻按（批次，项）稳定排序。
// 补充期限版本数按期限历史子表计——获批延期形成新版本原期限保留（0013），列面
// 只计数不展开。
func (review *CaseReview) ListClaimItems(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ClaimItemCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list claim items: limit must be positive, got %d", limit)
	}
	querier, err := review.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list claim items: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT i.batch_ref, i.item_id, i.customer_ref,
		        COALESCE(i.applicant_ref, ''), i.contract_ref,
		        i.target_ref, i.kind_ref, i.submitted_at, i.revision,
		        COALESCE(i.screen, ''), COALESCE(i.screen_basis, ''),
		        COALESCE(i.missing_materials_ref, ''),
		        COALESCE(i.supplement_scope_ref, ''),
		        COALESCE(i.supplement_notice_ref, ''),
		        i.supplement_deadline,
		        COALESCE(d.versions, 0),
		        COALESCE(i.conclusion, ''), i.concluded_at, i.review_by,
		        COALESCE(i.prior_conclusion, ''),
		        i.withdrawn, i.withdrawn_at
		   FROM visibility_exception.claim_item i
		   LEFT JOIN (
		        SELECT tenant_id, batch_ref, item_id, COUNT(*)::bigint AS versions
		          FROM visibility_exception.claim_supplement_deadline
		         GROUP BY tenant_id, batch_ref, item_id
		   ) d
		     ON d.tenant_id = i.tenant_id
		    AND d.batch_ref = i.batch_ref
		    AND d.item_id = i.item_id
		  WHERE i.tenant_id = $1
		  ORDER BY i.submitted_at DESC, i.batch_ref, i.item_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list claim items: %w", err)
	}
	defer rows.Close()

	list := make([]ports.ClaimItemCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row                ports.ClaimItemCatalogueRow
			supplementDeadline *time.Time
			concludedAt        *time.Time
			reviewBy           *time.Time
			withdrawnAt        *time.Time
		)
		if err := rows.Scan(
			&row.Batch, &row.ItemID, &row.Customer, &row.Applicant, &row.Contract,
			&row.Target, &row.Kind, &row.SubmittedAt, &row.Revision,
			&row.Screen, &row.ScreenBasis,
			&row.MissingMaterials, &row.SupplementScope, &row.SupplementNotice,
			&supplementDeadline,
			&row.DeadlineVersions,
			&row.Conclusion, &concludedAt, &reviewBy,
			&row.PriorConclusion,
			&row.Withdrawn, &withdrawnAt,
		); err != nil {
			return nil, fmt.Errorf("list claim items: %w", err)
		}
		if supplementDeadline != nil {
			utc := supplementDeadline.UTC()
			row.SupplementDeadline = &utc
		}
		if concludedAt != nil {
			utc := concludedAt.UTC()
			row.ConcludedAt = &utc
		}
		if reviewBy != nil {
			utc := reviewBy.UTC()
			row.ReviewBy = &utc
		}
		if withdrawnAt != nil {
			utc := withdrawnAt.UTC()
			row.WithdrawnAt = &utc
		}
		row.SubmittedAt = row.SubmittedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list claim items: %w", err)
	}
	return list, nil
}

// ListRecoveryMatters 上列追偿事项册：新近发起在前，同刻按事项标识稳定排序。两个
// 动作种类各取最近一个过程节点（LATERAL 按到达序取末行）——预先通知与正式主张
// 不合并，尚无动作的种类成对缺席。
func (review *CaseReview) ListRecoveryMatters(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.RecoveryMatterCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list recovery matters: limit must be positive, got %d", limit)
	}
	querier, err := review.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list recovery matters: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT m.matter_id, m.case_id, m.counterparty_ref, m.scope_ref,
		        m.basis_ref, m.legal_entity_ref, m.evidence_ref,
		        m.deadline, m.opened_at,
		        pre.milestone, pre.attempt, pre.occurred_at,
		        formal.milestone, formal.attempt, formal.occurred_at
		   FROM visibility_exception.recovery_matter m
		   LEFT JOIN LATERAL (
		        SELECT a.milestone, a.attempt, a.occurred_at
		          FROM visibility_exception.recovery_action a
		         WHERE a.tenant_id = m.tenant_id
		           AND a.matter_id = m.matter_id
		           AND a.kind = 'PRELIMINARY_NOTICE'
		         ORDER BY a.seq DESC
		         LIMIT 1
		   ) pre ON TRUE
		   LEFT JOIN LATERAL (
		        SELECT a.milestone, a.attempt, a.occurred_at
		          FROM visibility_exception.recovery_action a
		         WHERE a.tenant_id = m.tenant_id
		           AND a.matter_id = m.matter_id
		           AND a.kind = 'FORMAL_ASSERTION'
		         ORDER BY a.seq DESC
		         LIMIT 1
		   ) formal ON TRUE
		  WHERE m.tenant_id = $1
		  ORDER BY m.opened_at DESC, m.matter_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list recovery matters: %w", err)
	}
	defer rows.Close()

	list := make([]ports.RecoveryMatterCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row            ports.RecoveryMatterCatalogueRow
			preMilestone   *string
			preAttempt     *int32
			preOccurredAt  *time.Time
			formMilestone  *string
			formAttempt    *int32
			formOccurredAt *time.Time
		)
		if err := rows.Scan(
			&row.MatterID, &row.CaseID, &row.Counterparty, &row.Scope,
			&row.Basis, &row.LegalEntity, &row.Evidence,
			&row.Deadline, &row.OpenedAt,
			&preMilestone, &preAttempt, &preOccurredAt,
			&formMilestone, &formAttempt, &formOccurredAt,
		); err != nil {
			return nil, fmt.Errorf("list recovery matters: %w", err)
		}
		row.PreliminaryNotice = recoveryCell(preMilestone, preAttempt, preOccurredAt)
		row.FormalAssertion = recoveryCell(formMilestone, formAttempt, formOccurredAt)
		row.Deadline = row.Deadline.UTC()
		row.OpenedAt = row.OpenedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recovery matters: %w", err)
	}
	return list, nil
}

// recoveryCell 把 LATERAL 取回的三件拼成一格：三件同出同缺（同一行取的），任一在场
// 即整格在场。
func recoveryCell(milestone *string, attempt *int32, occurredAt *time.Time) *ports.RecoveryActionCell {
	if milestone == nil {
		return nil
	}
	return &ports.RecoveryActionCell{
		Milestone:  *milestone,
		Attempt:    int64(*attempt),
		OccurredAt: occurredAt.UTC(),
	}
}
