package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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

// signalEpisodePages 把 ports.SignalEpisodePages 落到列上。outcome 落在左联进来的分诊结论上，尚未分诊的发作期该列为空、不落
// 在任何一个词上；q 在发作期标识、包裹与信号种类上做字面包含。
var signalEpisodePages = ports.SignalEpisodePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"startedAt": "e.started_at", "lastHitAt": "e.last_hit_at", "parcel": "e.parcel_ref"},
	Identity: []string{"e.episode_id"},
	Filters:  map[string]string{"outcome": "c.outcome"},
	Keyword:  []string{"e.episode_id", "e.parcel_ref", "e.kind_ref"},
})

// ListSignalEpisodes 上列信号发作期册，按查询的筛选、q 与排序取游标之后的一页；缺省是新近首启在前，同刻按发作期标识倒序。
// 0002 另立的到达序 seq 是 FindLatest 的并列裁决序，不是行标识，不进游标。
func (review *CaseReview) ListSignalEpisodes(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.SignalEpisodeCatalogueRow], error) {
	const source = `visibility_exception.signal_episode e
		LEFT JOIN visibility_exception.triage_conclusion c
		  ON c.tenant_id = e.tenant_id
		 AND c.episode_id = e.episode_id`
	page, err := readPage(ctx, review.db, signalEpisodePages, query, tenant, "e.tenant_id = $1", limit,
		pageSources{
			count: source,
			page:  source,
			selection: `e.episode_id, e.parcel_ref, e.kind_ref, e.causal_condition, e.fact_basis, e.rule_ref, e.confidence_ref,
				e.hits, e.started_at, e.last_hit_at,
				COALESCE(e.release_basis, ''), e.ended_at,
				COALESCE(e.prior_episode, ''),
				COALESCE(c.outcome, ''), COALESCE(c.rule_ref, ''), c.triaged_at`,
		},
		scanSignalEpisode, signalEpisodePosition)
	if err != nil {
		return cataloguepage.Result[ports.SignalEpisodeCatalogueRow]{}, fmt.Errorf("list signal episodes: %w", err)
	}
	return page, nil
}

func scanSignalEpisode(rows pgx.Rows) (ports.SignalEpisodeCatalogueRow, error) {
	var (
		row       ports.SignalEpisodeCatalogueRow
		basisJSON []byte
		hits      int32
		endedAt   *time.Time
		triagedAt *time.Time
	)
	if err := rows.Scan(
		&row.EpisodeID, &row.Parcel, &row.Kind, &row.CausalCondition, &basisJSON, &row.Rule, &row.Confidence,
		&hits, &row.StartedAt, &row.LastHitAt,
		&row.ReleaseBasis, &endedAt,
		&row.PriorEpisode,
		&row.Outcome, &row.OutcomeRule, &triagedAt,
	); err != nil {
		return ports.SignalEpisodeCatalogueRow{}, err
	}
	row.Hits = int64(hits)
	var basis []signalFactBasisRow
	if err := json.Unmarshal(basisJSON, &basis); err != nil {
		return ports.SignalEpisodeCatalogueRow{}, fmt.Errorf("fact basis: %w", err)
	}
	row.FactBasis = make([]ports.SignalEpisodeFactBasisRow, 0, len(basis))
	for _, item := range basis {
		row.FactBasis = append(row.FactBasis, ports.SignalEpisodeFactBasisRow(item))
	}
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
	return row, nil
}

func signalEpisodePosition(row ports.SignalEpisodeCatalogueRow, sortField string) cataloguepage.Position {
	value := row.Parcel
	switch sortField {
	case "startedAt":
		value = cataloguepage.FormatInstant(row.StartedAt)
	case "lastHitAt":
		value = cataloguepage.FormatInstant(row.LastHitAt)
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.EpisodeID}}
}

// dispositionRequestPages 把 ports.DispositionRequestPages 落到列上。judgment 落在请求行自己的答复列上；q 里的案件单据号取自
// 左联进来的案件行，案件没有号或不在案件册上时那一列为空、不命中。
var dispositionRequestPages = ports.DispositionRequestPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"sentAt": "r.sent_at"},
	Identity: []string{"r.request_id"},
	Filters:  map[string]string{"judgment": "r.judgment"},
	Keyword:  []string{"r.request_id", "r.case_id", "c.document_number", "r.action_ref"},
})

// ListDispositionRequests 上列处置请求册，按查询的筛选、q 与排序取游标之后的一页；缺省是新近发送在前，同刻按请求标识倒序。
// 被替代的行照列——替代不是删除，版本链在册面上完整可见。所属案件的单据号按（租户，案件标识）左联带出。
func (review *CaseReview) ListDispositionRequests(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.DispositionRequestCatalogueRow], error) {
	const source = `visibility_exception.disposition_request r
		LEFT JOIN visibility_exception.exception_case c
		  ON c.tenant_id = r.tenant_id
		 AND c.case_id = r.case_id`
	page, err := readPage(ctx, review.db, dispositionRequestPages, query, tenant, "r.tenant_id = $1", limit,
		pageSources{
			count: source,
			page:  source,
			selection: `r.request_id, r.case_id, COALESCE(c.document_number, ''),
				r.target_context, r.action_ref, r.scope_ref,
				r.reason, r.evidence_ref, r.intent_version, r.sent_at, r.acceptance_window,
				COALESCE(r.judgment, ''), r.judged_at,
				COALESCE(r.cancellation, ''), COALESCE(r.superseded_by, '')`,
		},
		scanDispositionRequest, dispositionRequestPosition)
	if err != nil {
		return cataloguepage.Result[ports.DispositionRequestCatalogueRow]{}, fmt.Errorf("list disposition requests: %w", err)
	}
	return page, nil
}

func scanDispositionRequest(rows pgx.Rows) (ports.DispositionRequestCatalogueRow, error) {
	var (
		row              ports.DispositionRequestCatalogueRow
		intentVersion    int32
		acceptanceWindow *time.Time
		judgedAt         *time.Time
	)
	if err := rows.Scan(
		&row.RequestID, &row.CaseID, &row.CaseDocumentNumber, &row.TargetContext, &row.Action, &row.Scope,
		&row.Reason, &row.Evidence, &intentVersion, &row.SentAt, &acceptanceWindow,
		&row.Judgment, &judgedAt,
		&row.Cancellation, &row.SupersededBy,
	); err != nil {
		return ports.DispositionRequestCatalogueRow{}, err
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
	return row, nil
}

func dispositionRequestPosition(row ports.DispositionRequestCatalogueRow, _ string) cataloguepage.Position {
	return cataloguepage.Position{Value: cataloguepage.FormatInstant(row.SentAt), Identity: []string{row.RequestID}}
}

// exceptionCasePages 把 ports.ExceptionCasePages 落到列上；phase 落在存词的 phase 列上，q 的单据号列可空，没有号的案件不命中它。
var exceptionCasePages = ports.ExceptionCasePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"establishedAt": "established_at", "rootParcel": "root_parcel"},
	Identity: []string{"case_id"},
	Filters:  map[string]string{"phase": "phase"},
	Keyword:  []string{"case_id", "document_number", "root_parcel", "responsible_team"},
})

// ListExceptionCases 上列异常案件册，按查询的筛选、q 与排序取游标之后的一页；缺省是新近建立在前，同刻按案件标识倒序。
func (review *CaseReview) ListExceptionCases(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ExceptionCaseCatalogueRow], error) {
	const source = "visibility_exception.exception_case"
	page, err := readPage(ctx, review.db, exceptionCasePages, query, tenant, "tenant_id = $1", limit,
		pageSources{
			count: source,
			page:  source,
			selection: `case_id, COALESCE(document_number, ''), COALESCE(document_number_absence, ''),
				root_parcel, impact_scope, responsible_team, phase,
				established_at, first_response, closed_at,
				COALESCE(conclusion, ''), COALESCE(merged_into, '')`,
		},
		scanExceptionCase, exceptionCasePosition)
	if err != nil {
		return cataloguepage.Result[ports.ExceptionCaseCatalogueRow]{}, fmt.Errorf("list exception cases: %w", err)
	}
	return page, nil
}

func scanExceptionCase(rows pgx.Rows) (ports.ExceptionCaseCatalogueRow, error) {
	var (
		row                           ports.ExceptionCaseCatalogueRow
		number, documentNumberAbsence string
		firstResponse                 *time.Time
		closedAt                      *time.Time
	)
	if err := rows.Scan(
		&row.CaseID, &number, &documentNumberAbsence,
		&row.RootParcel, &row.ImpactScope, &row.ResponsibleTeam, &row.Phase,
		&row.EstablishedAt, &firstResponse, &closedAt,
		&row.Conclusion, &row.MergedInto,
	); err != nil {
		return ports.ExceptionCaseCatalogueRow{}, err
	}
	var err error
	if row.DocumentNumber, row.DocumentNumberAbsence, err = documentNumberColumns(number, documentNumberAbsence); err != nil {
		return ports.ExceptionCaseCatalogueRow{}, fmt.Errorf("case %s: %w", row.CaseID, err)
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
	return row, nil
}

func exceptionCasePosition(row ports.ExceptionCaseCatalogueRow, sortField string) cataloguepage.Position {
	value := row.RootParcel
	if sortField == "establishedAt" {
		value = cataloguepage.FormatInstant(row.EstablishedAt)
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.CaseID}}
}

// customerNotificationPages 把 ports.CustomerNotificationPages 落到列上；q 在通知标识、客户、发作期与渠道上做字面包含。
var customerNotificationPages = ports.CustomerNotificationPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"decidedAt": "decided_at"},
	Identity: []string{"notification_id"},
	Keyword:  []string{"notification_id", "customer_ref", "episode_id", "channel_ref"},
})

// ListCustomerNotifications 上列客户异常通知册，按查询的 q 与排序取游标之后的一页；缺省是新近决定在前，同刻按通知标识倒序。
// 过程节点整列照 milestones jsonb 转写（写侧 notificationMilestoneRow 的键形状，两侧共一份）。
func (review *CaseReview) ListCustomerNotifications(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerNotificationCatalogueRow], error) {
	const source = "visibility_exception.customer_notification"
	page, err := readPage(ctx, review.db, customerNotificationPages, query, tenant, "tenant_id = $1", limit,
		pageSources{
			count: source,
			page:  source,
			selection: `notification_id, customer_ref, episode_id, decided_at,
				disclosure_policy_ref, content_ref, deadline,
				channel_ref, obligation_ref, milestones`,
		},
		scanCustomerNotification, customerNotificationPosition)
	if err != nil {
		return cataloguepage.Result[ports.CustomerNotificationCatalogueRow]{}, fmt.Errorf("list customer notifications: %w", err)
	}
	return page, nil
}

func scanCustomerNotification(rows pgx.Rows) (ports.CustomerNotificationCatalogueRow, error) {
	var (
		row           ports.CustomerNotificationCatalogueRow
		milestonesRaw []byte
	)
	if err := rows.Scan(
		&row.NotificationID, &row.Customer, &row.Episode, &row.DecidedAt,
		&row.Policy, &row.Content, &row.Deadline,
		&row.Channel, &row.Obligation, &milestonesRaw,
	); err != nil {
		return ports.CustomerNotificationCatalogueRow{}, err
	}
	var nodes []notificationMilestoneRow
	if err := json.Unmarshal(milestonesRaw, &nodes); err != nil {
		return ports.CustomerNotificationCatalogueRow{}, err
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
	return row, nil
}

func customerNotificationPosition(row ports.CustomerNotificationCatalogueRow, _ string) cataloguepage.Position {
	return cataloguepage.Position{Value: cataloguepage.FormatInstant(row.DecidedAt), Identity: []string{row.NotificationID}}
}

// claimItemPages 把 ports.ClaimItemPages 落到列上。screen 与 conclusion 落在索赔项行自己的两判列上，尚未判的项该列为空、不落在
// 任何一个词上；q 的单据号列可空，没有号的项不命中它。
var claimItemPages = ports.ClaimItemPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"submittedAt": "i.submitted_at"},
	Identity: []string{"i.batch_ref", "i.item_id"},
	Filters:  map[string]string{"screen": "i.screen", "conclusion": "i.conclusion"},
	Keyword:  []string{"i.batch_ref", "i.item_id", "i.document_number", "i.customer_ref", "i.target_ref", "i.kind_ref"},
})

// ListClaimItems 上列客户索赔项册，按查询的筛选、q 与排序取游标之后的一页；缺省是新近提交在前，同刻按（批次，项）倒序。
// 补充期限版本数按期限历史子表计——获批延期形成新版本原期限保留（0013），列面只计数不展开。
func (review *CaseReview) ListClaimItems(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ClaimItemCatalogueRow], error) {
	page, err := readPage(ctx, review.db, claimItemPages, query, tenant, "i.tenant_id = $1", limit,
		pageSources{
			count: "visibility_exception.claim_item i",
			page: `visibility_exception.claim_item i
				LEFT JOIN (
					SELECT tenant_id, batch_ref, item_id, COUNT(*)::bigint AS versions
					  FROM visibility_exception.claim_supplement_deadline
					 GROUP BY tenant_id, batch_ref, item_id
				) d
				  ON d.tenant_id = i.tenant_id
				 AND d.batch_ref = i.batch_ref
				 AND d.item_id = i.item_id`,
			selection: `i.batch_ref, i.item_id,
				COALESCE(i.document_number, ''), COALESCE(i.document_number_absence, ''),
				i.customer_ref,
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
				i.withdrawn, i.withdrawn_at`,
		},
		scanClaimItem, claimItemPosition)
	if err != nil {
		return cataloguepage.Result[ports.ClaimItemCatalogueRow]{}, fmt.Errorf("list claim items: %w", err)
	}
	return page, nil
}

func scanClaimItem(rows pgx.Rows) (ports.ClaimItemCatalogueRow, error) {
	var (
		row                           ports.ClaimItemCatalogueRow
		number, documentNumberAbsence string
		supplementDeadline            *time.Time
		concludedAt                   *time.Time
		reviewBy                      *time.Time
		withdrawnAt                   *time.Time
	)
	if err := rows.Scan(
		&row.Batch, &row.ItemID, &number, &documentNumberAbsence,
		&row.Customer, &row.Applicant, &row.Contract,
		&row.Target, &row.Kind, &row.SubmittedAt, &row.Revision,
		&row.Screen, &row.ScreenBasis,
		&row.MissingMaterials, &row.SupplementScope, &row.SupplementNotice,
		&supplementDeadline,
		&row.DeadlineVersions,
		&row.Conclusion, &concludedAt, &reviewBy,
		&row.PriorConclusion,
		&row.Withdrawn, &withdrawnAt,
	); err != nil {
		return ports.ClaimItemCatalogueRow{}, err
	}
	var err error
	if row.DocumentNumber, row.DocumentNumberAbsence, err = documentNumberColumns(number, documentNumberAbsence); err != nil {
		return ports.ClaimItemCatalogueRow{}, fmt.Errorf("%s/%s: %w", row.Batch, row.ItemID, err)
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
	return row, nil
}

func claimItemPosition(row ports.ClaimItemCatalogueRow, _ string) cataloguepage.Position {
	return cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(row.SubmittedAt),
		Identity: []string{row.Batch, row.ItemID},
	}
}

// recoveryMatterPages 把 ports.RecoveryMatterPages 落到列上；q 里的案件单据号取自左联进来的案件行，没有号时不命中。
var recoveryMatterPages = ports.RecoveryMatterPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"openedAt": "m.opened_at"},
	Identity: []string{"m.matter_id"},
	Keyword:  []string{"m.matter_id", "m.case_id", "c.document_number", "m.counterparty_ref"},
})

// ListRecoveryMatters 上列追偿事项册，按查询的 q 与排序取游标之后的一页；缺省是新近发起在前，同刻按事项标识倒序。两个动作
// 种类各取最近一个过程节点（LATERAL 按到达序取末行）——预先通知与正式主张不合并，尚无动作的种类成对缺席。关联案件的单据号
// 按（租户，案件标识）左联带出。
func (review *CaseReview) ListRecoveryMatters(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.RecoveryMatterCatalogueRow], error) {
	const withCase = `visibility_exception.recovery_matter m
		LEFT JOIN visibility_exception.exception_case c
		  ON c.tenant_id = m.tenant_id
		 AND c.case_id = m.case_id`
	page, err := readPage(ctx, review.db, recoveryMatterPages, query, tenant, "m.tenant_id = $1", limit,
		pageSources{
			count: withCase,
			page: withCase + `
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
				) formal ON TRUE`,
			selection: `m.matter_id, m.case_id, COALESCE(c.document_number, ''), m.counterparty_ref, m.scope_ref,
				m.basis_ref, m.legal_entity_ref, m.evidence_ref,
				m.deadline, m.opened_at,
				pre.milestone, pre.attempt, pre.occurred_at,
				formal.milestone, formal.attempt, formal.occurred_at`,
		},
		scanRecoveryMatter, recoveryMatterPosition)
	if err != nil {
		return cataloguepage.Result[ports.RecoveryMatterCatalogueRow]{}, fmt.Errorf("list recovery matters: %w", err)
	}
	return page, nil
}

func scanRecoveryMatter(rows pgx.Rows) (ports.RecoveryMatterCatalogueRow, error) {
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
		&row.MatterID, &row.CaseID, &row.CaseDocumentNumber, &row.Counterparty, &row.Scope,
		&row.Basis, &row.LegalEntity, &row.Evidence,
		&row.Deadline, &row.OpenedAt,
		&preMilestone, &preAttempt, &preOccurredAt,
		&formMilestone, &formAttempt, &formOccurredAt,
	); err != nil {
		return ports.RecoveryMatterCatalogueRow{}, err
	}
	row.PreliminaryNotice = recoveryCell(preMilestone, preAttempt, preOccurredAt)
	row.FormalAssertion = recoveryCell(formMilestone, formAttempt, formOccurredAt)
	row.Deadline = row.Deadline.UTC()
	row.OpenedAt = row.OpenedAt.UTC()
	return row, nil
}

func recoveryMatterPosition(row ports.RecoveryMatterCatalogueRow, _ string) cataloguepage.Position {
	return cataloguepage.Position{Value: cataloguepage.FormatInstant(row.OpenedAt), Identity: []string{row.MatterID}}
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

// documentNumberColumns 把库里单据号两格转写成读行的两格：有号交号；没号交无号原因——两格都空的旧行说成「上线前开立」，
// 这一层约定只在领域的 RehydrateDocumentNumber 一处，读面经它转写，不另写一份。
func documentNumberColumns(number, documentNumberAbsence string) (string, string, error) {
	documentNumber, err := domain.RehydrateDocumentNumber(number, documentNumberAbsence)
	if err != nil {
		return "", "", err
	}
	assigned, _ := documentNumber.Number()
	return assigned, string(documentNumber.Absence()), nil
}
