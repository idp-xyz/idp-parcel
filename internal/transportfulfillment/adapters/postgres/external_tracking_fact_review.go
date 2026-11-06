package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var _ ports.ExternalTrackingFactReviewRead = (*ExternalTrackingFacts)(nil)

// externalTrackingFactPages 把 ports.ExternalTrackingFactPages 落到列上。q 在事实、对象、源事件与状态引用上做字面包含。列都带
// 别名 current：「当前」由回指同表的反连接派生，两份表要分得开。
var externalTrackingFactPages = ports.ExternalTrackingFactPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"occurredAt": "current.occurred_at", "receivedAt": "current.received_at"},
	Identity: []string{"current.fact_ref", "current.version"},
	Filters:  map[string]string{"effectiveBasis": "current.effective_basis"},
	Keyword:  []string{"current.fact_ref", "current.object_ref", "current.source_event", "current.status_ref"},
})

// ListCurrentExternalTrackingFacts 上列某源的当前版（label-channel/21）。「当前」照 FindCurrent 的派生法
// ——没有任何行回指它——不另存标记；待判断过滤落在 effective_basis 上，库面 CHECK 已保证它与
// effective_at 的缺席同真（迁移 0011）。
//
// 列面照实转写、不经领域重建门（判据同 ReviewCatalogue）：重建是写路与按键读回的纪律，检索列面把登记
// 的字段原样透出。缺省按发生时间先后排——判断人按事发顺序判；同刻按事实与版本升序。待判断过滤是选择器 view 的
// 原义（filter），与查询对象上的筛选维 effectiveBasis 并列取交。
func (repository *ExternalTrackingFacts) ListCurrentExternalTrackingFacts(
	ctx context.Context,
	tenant domain.TenantID,
	source domain.TrackingSourceReference,
	filter ports.EffectiveTimeReviewFilter,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ExternalTrackingFactReviewRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.ExternalTrackingFactReviewRow], error) {
		return cataloguepage.Result[ports.ExternalTrackingFactReviewRow]{}, fmt.Errorf("list current external tracking facts: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	var pendingClause string
	switch filter {
	case ports.PendingEffectiveTimeOnly:
		pendingClause = ` AND current.effective_basis = '` + domain.EffectiveTimePending.String() + `'`
	case ports.EveryCurrentVersion:
		pendingClause = ""
	default:
		return fail(fmt.Errorf("filter %d is not one of pending-only, every-current-version", filter))
	}
	plan, err := externalTrackingFactPages.Plan(query,
		`current.tenant_id = $1 AND current.source_ref = $2
		    AND NOT EXISTS (
		        SELECT 1 FROM transport_fulfillment.external_carrier_tracking_fact AS successor
		         WHERE successor.tenant_id = current.tenant_id
		           AND successor.fact_ref = current.fact_ref
		           AND successor.supersedes_version = current.version)`+pendingClause,
		[]any{tenant.String(), source.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const facts = "transport_fulfillment.external_carrier_tracking_fact AS current"
	page, err := readCataloguePage(ctx, querier, query, plan, limit, plan.CountSQL(facts), plan.PageSQL(
		`current.fact_ref, current.version, current.source_ref, current.credential_ref, current.object_ref,
		 current.source_event, current.status_ref, current.occurred_at, current.received_at,
		 current.effective_basis, current.effective_at, current.effective_rule, current.effective_rule_version,
		 current.supersedes_version, current.version_origin, current.recorded_at, current.effective_judged_by`, facts),
		scanExternalTrackingFactReviewRow, externalTrackingFactPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func scanExternalTrackingFactReviewRow(scanned pgx.Row) (ports.ExternalTrackingFactReviewRow, error) {
	var row ports.ExternalTrackingFactReviewRow
	var sourceEvent, effectiveRule, effectiveRuleVersion, supersedes, judgedBy *string
	var effectiveAt *time.Time
	if err := scanned.Scan(
		&row.Fact, &row.Version, &row.Source, &row.Credential, &row.Object,
		&sourceEvent, &row.Status, &row.OccurredAt, &row.ReceivedAt,
		&row.EffectiveBasis, &effectiveAt, &effectiveRule, &effectiveRuleVersion,
		&supersedes, &row.Origin, &row.RecordedAt, &judgedBy,
	); err != nil {
		return ports.ExternalTrackingFactReviewRow{}, err
	}
	row.OccurredAt = row.OccurredAt.UTC()
	row.ReceivedAt = row.ReceivedAt.UTC()
	row.RecordedAt = row.RecordedAt.UTC()
	if sourceEvent != nil {
		row.SourceEvent = *sourceEvent
	}
	if effectiveAt != nil {
		at := effectiveAt.UTC()
		row.EffectiveAt = &at
	}
	if effectiveRule != nil {
		row.EffectiveRule = *effectiveRule
	}
	if effectiveRuleVersion != nil {
		row.EffectiveRuleVersion = *effectiveRuleVersion
	}
	if supersedes != nil {
		row.Supersedes = *supersedes
	}
	if judgedBy != nil {
		row.EffectiveJudgedBy = *judgedBy
	}
	return row, nil
}

func externalTrackingFactPosition(row ports.ExternalTrackingFactReviewRow, sortField string) cataloguepage.Position {
	at := row.OccurredAt
	if sortField == "receivedAt" {
		at = row.ReceivedAt
	}
	return cataloguepage.Position{Value: cataloguepage.FormatInstant(at), Identity: []string{row.Fact, row.Version}}
}
