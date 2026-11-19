package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

const actualMeasurementRegister = "node_operations.actual_measurement"

// measurementVoidedText 与筛选词 true/false 同形。外层列按表名限定的缘由写在 measurementInvalidatedPredicate 上。
const measurementVoidedText = `(` + measurementInvalidatedPredicate + `)::text`

var actualMeasurementPages = ports.ActualMeasurementPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "recorded_at"},
	Identity: []string{"source_id"},
	Filters:  map[string]string{"voided": measurementVoidedText},
	Keyword:  []string{"source_id", "object_id", "location_ref", "performed_by"},
})

// ListActualMeasurements 上列实际测量册的一页。作废不改测量行，voided 另看有没有作废指过来。
func (catalogue *ReviewCatalogue) ListActualMeasurements(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ActualMeasurementCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.ActualMeasurementCatalogueRow], error) {
		return cataloguepage.Result[ports.ActualMeasurementCatalogueRow]{}, fmt.Errorf("list actual measurements: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := actualMeasurementPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(actualMeasurementRegister), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`source_id, object_kind, object_id, weight_value, weight_unit,
		 length_value, width_value, height_value, dimension_unit,
		 occurred_at, recorded_at, location_ref, basis_ref, performed_by,
		 COALESCE(entered_by, ''), COALESCE(original_document, ''), `+measurementVoidedText+` = 'true'`,
		actualMeasurementRegister), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]ports.ActualMeasurementCatalogueRow, 0, limit+1)
	for rows.Next() {
		var row ports.ActualMeasurementCatalogueRow
		if err := rows.Scan(
			&row.SourceID, &row.ObjectKind, &row.ObjectID,
			&row.WeightValue, &row.WeightUnit, &row.Length, &row.Width, &row.Height, &row.DimensionUnit,
			&row.OccurredAt, &row.RecordedAt, &row.Location, &row.Basis, &row.PerformedBy,
			&row.EnteredBy, &row.OriginalDocument, &row.Voided,
		); err != nil {
			return fail(err)
		}
		row.OccurredAt = row.OccurredAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.ActualMeasurementCatalogueRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{
				Value:    cataloguepage.FormatInstant(row.RecordedAt),
				Identity: []string{row.SourceID},
			}
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}
