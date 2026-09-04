package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var _ ports.ExternalTrackingFactReviewRead = (*ExternalTrackingFacts)(nil)

// ListCurrentExternalTrackingFacts 上列某源的当前版（label-channel/21）。「当前」照 FindCurrent 的派生法
// ——没有任何行回指它——不另存标记；待判断过滤落在 effective_basis 上，库面 CHECK 已保证它与
// effective_at 的缺席同真（迁移 0011）。
//
// 列面照实转写、不经领域重建门（判据同 ReviewCatalogue）：重建是写路与按键读回的纪律，检索列面把登记
// 的字段原样透出。按发生时间先后排——判断人按事发顺序判；同刻按事实与版本稳定排序。
func (repository *ExternalTrackingFacts) ListCurrentExternalTrackingFacts(
	ctx context.Context,
	tenant domain.TenantID,
	source domain.TrackingSourceReference,
	filter ports.EffectiveTimeReviewFilter,
	limit int,
) ([]ports.ExternalTrackingFactReviewRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list current external tracking facts: limit must be positive, got %d", limit)
	}
	var pendingClause string
	switch filter {
	case ports.PendingEffectiveTimeOnly:
		pendingClause = `AND current.effective_basis = '` + domain.EffectiveTimePending.String() + `'`
	case ports.EveryCurrentVersion:
		pendingClause = ""
	default:
		return nil, fmt.Errorf("list current external tracking facts: filter %d is not one of pending-only, every-current-version", filter)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list current external tracking facts: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT current.fact_ref, current.version, current.source_ref, current.credential_ref, current.object_ref,
		        current.source_event, current.status_ref, current.occurred_at, current.received_at,
		        current.effective_basis, current.effective_at, current.effective_rule, current.effective_rule_version,
		        current.supersedes_version, current.version_origin, current.recorded_at
		   FROM transport_fulfillment.external_carrier_tracking_fact AS current
		  WHERE current.tenant_id = $1 AND current.source_ref = $2
		    AND NOT EXISTS (
		        SELECT 1 FROM transport_fulfillment.external_carrier_tracking_fact AS successor
		         WHERE successor.tenant_id = current.tenant_id
		           AND successor.fact_ref = current.fact_ref
		           AND successor.supersedes_version = current.version)
		    `+pendingClause+`
		  ORDER BY current.occurred_at, current.fact_ref, current.version
		  LIMIT $3`,
		tenant.String(), source.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("list current external tracking facts: %w", err)
	}
	defer rows.Close()

	list := make([]ports.ExternalTrackingFactReviewRow, 0, limit)
	for rows.Next() {
		var row ports.ExternalTrackingFactReviewRow
		var sourceEvent, effectiveRule, effectiveRuleVersion, supersedes *string
		var effectiveAt *time.Time
		if err := rows.Scan(
			&row.Fact, &row.Version, &row.Source, &row.Credential, &row.Object,
			&sourceEvent, &row.Status, &row.OccurredAt, &row.ReceivedAt,
			&row.EffectiveBasis, &effectiveAt, &effectiveRule, &effectiveRuleVersion,
			&supersedes, &row.Origin, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list current external tracking facts: %w", err)
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
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list current external tracking facts: %w", err)
	}
	return list, nil
}
