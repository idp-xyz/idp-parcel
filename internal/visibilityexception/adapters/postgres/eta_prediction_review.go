package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// etaPredictionPages 把 ports.ETAPredictionPages 落到列上；q 在包裹、里程碑、来源与版本上做字面包含。
var etaPredictionPages = ports.ETAPredictionPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"predictedAt": "v.predicted_at"},
	Identity: []string{"v.version_id"},
	Keyword:  []string{"v.parcel_ref", "v.milestone_ref", "v.source", "v.version_id"},
})

// ListCurrentETAPredictions 上列各预测线的当前版。「当前」照当前版指针表取，不按时刻另算；列面照实转写、不经领域重建门——
// 重建是写路与按键读回的纪律，检索列面把登记的字段原样透出。留痕按版本标识对：一版至多一条，新版本出来后旧版的留痕不跟过去。
func (repository *ETAs) ListCurrentETAPredictions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ETAPredictionReviewRow], error) {
	const source = `visibility_exception.eta_prediction_current AS c
		   JOIN visibility_exception.eta_prediction_version AS v
		     ON v.tenant_id = c.tenant_id AND v.version_id = c.version_id
		   LEFT JOIN visibility_exception.eta_manual_entry AS entry
		     ON entry.tenant_id = v.tenant_id AND entry.version_id = v.version_id`
	page, err := readPage(ctx, repository.db, etaPredictionPages, query, tenant, "c.tenant_id = $1", limit,
		pageSources{
			count: source,
			page:  source,
			selection: `v.parcel_ref, v.milestone_ref, v.source, v.version_id, v.inputs_ref, v.model_ref,
				v.range_from, v.range_to, v.confidence_ref, v.predicted_at, COALESCE(v.prior_version, ''),
				entry.entered_by, entry.original_ref, entry.entered_at`,
		},
		scanETAPredictionReviewRow,
		func(row ports.ETAPredictionReviewRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(row.PredictedAt), Identity: []string{row.Version}}
		})
	if err != nil {
		return cataloguepage.Result[ports.ETAPredictionReviewRow]{}, fmt.Errorf("list current eta predictions: %w", err)
	}
	return page, nil
}

func scanETAPredictionReviewRow(rows pgx.Rows) (ports.ETAPredictionReviewRow, error) {
	var (
		row                 ports.ETAPredictionReviewRow
		enteredBy, original *string
		enteredAt           *time.Time
	)
	if err := rows.Scan(
		&row.Parcel, &row.Milestone, &row.Source, &row.Version, &row.Inputs, &row.Model,
		&row.RangeFrom, &row.RangeTo, &row.Confidence, &row.PredictedAt, &row.PriorVersion,
		&enteredBy, &original, &enteredAt,
	); err != nil {
		return ports.ETAPredictionReviewRow{}, err
	}
	row.RangeFrom, row.RangeTo, row.PredictedAt = row.RangeFrom.UTC(), row.RangeTo.UTC(), row.PredictedAt.UTC()
	if enteredBy != nil && original != nil && enteredAt != nil {
		row.ManualEntry = &ports.ManualETAEntryNote{EnteredBy: *enteredBy, Original: *original, EnteredAt: enteredAt.UTC()}
	}
	return row, nil
}

var _ ports.ETAPredictionReviewRead = (*ETAs)(nil)
