package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// identityReviewUnitPages 把 ports.IdentityReviewUnitPages 落到导出表 u 的两列上：作业实物与它待识别各版里最近一次的登记时刻。
// 这一册一行是一件实物而不是一行登记，排序、键集、q 与总数都只能落在聚合导出的列上；q 只在作业实物上做字面包含。
var identityReviewUnitPages = ports.IdentityReviewUnitPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"latestRecordedAt": "u.latest"},
	Identity: []string{"u.unit"},
	Keyword:  []string{"u.unit"},
})

// identityReviewUnits 把「有过待识别那一格的作业实物」做成导出表 u，计数与取页都从它上面走。
//
// 作业实物是 intake 里的 jsonb 键：库里 reception_intake_presence 只保证 intake 本身非空，键在不在靠收寄写口（intakeRow 的 unit
// 不省略）。分组键因此先导出成非空列（cataloguepage.Columns 头注：可排维与行标识的列必须非空）——留着 NULL 的话，正序翻页时与游标
// 同刻、排在它后面的那一组会被键集比较悄悄跳过。缺键的行归到空串那一组，它落进交回的那一页时由 ListIdentityReviewUnits 响亮报错，
// 不把坏行演成一件看起来正常的实物。后面按实物取链头那一读仍按 intake ->> 'unit' 取，与 reception_intake_unit_idx 的表达式同形
// 才走得上它。
const identityReviewUnits = `WITH u AS (
		SELECT COALESCE(intake ->> 'unit', '') AS unit, max(recorded_at) AS latest
		  FROM node_operations.reception
		 WHERE tenant_id = $1
		   AND kind = 'PENDING_IDENTIFICATION'
		 GROUP BY COALESCE(intake ->> 'unit', '')
	) `

// ListIdentityReviewUnits 按实物上列待识别实物页（ports.IdentityReviewUnitRow）的一页：有过待识别那一格的作业实物，缺省按待识别各版
// 里最近一次的登记时刻倒序、同刻按实物倒序，每件交回它各条判断链没有后继的那几版（形成格也在——一件实物在另一个节点关联成立的
// 那条链同样左右它的身份）。链头按「没有一行指名它为前版」认，走 reception_predecessor_unique；带收寄的两格的后继只会是这两格，
// 所以在这件实物带收寄的行里就认得全。
//
// 先取实物这一页，再只为本页的实物取链头：limit 与 total 数的都是实物，链头多少不影响翻页。处置文档经领域读回门重建（理由在
// ports.IdentityReviewHead 的头注）；其余各格照行转写。
func (catalogue *ReviewCatalogue) ListIdentityReviewUnits(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.IdentityReviewUnitRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.IdentityReviewUnitRow], error) {
		return cataloguepage.Result[ports.IdentityReviewUnitRow]{}, fmt.Errorf("list identity review units: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := identityReviewUnitPages.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, identityReviewUnits+plan.CountSQL("u"), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}

	rows, err := querier.Query(ctx, identityReviewUnits+plan.PageSQL("u.unit, u.latest", "u"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	fetched := make([]ports.IdentityReviewUnitRow, 0, limit+1)
	for rows.Next() {
		var (
			unit   string
			latest time.Time
		)
		if err := rows.Scan(&unit, &latest); err != nil {
			rows.Close()
			return fail(err)
		}
		fetched = append(fetched, ports.IdentityReviewUnitRow{Unit: unit, LatestRecordedAt: latest.UTC()})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	// 实物与最近登记时刻都是从 u 的两列原样扫回的值，与 ORDER BY 用的是同一份。
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row ports.IdentityReviewUnitRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(row.LatestRecordedAt), Identity: []string{row.Unit}}
		})
	if err != nil {
		return fail(err)
	}
	// 缺键那一组切页之后才判：多取来判下一页的那一行是它时，不含它的本页照常答。
	for _, row := range page.Rows {
		if row.Unit == "" {
			return fail(fmt.Errorf("pending identification rows without an intake unit, latest recorded at %s",
				row.LatestRecordedAt.Format(time.RFC3339Nano)))
		}
	}
	if len(page.Rows) == 0 {
		return page, nil
	}

	// 游标关了再取链头：同一个执行器上不套第二个游标。
	units := make([]string, len(page.Rows))
	at := make(map[string]int, len(page.Rows))
	for index, row := range page.Rows {
		units[index] = row.Unit
		at[row.Unit] = index
		page.Rows[index].Heads = []ports.IdentityReviewHead{}
	}
	heads, err := querier.Query(ctx,
		`SELECT r.intake ->> 'unit', r.source_id, r.kind,
		        r.intake ->> 'node',
		        COALESCE(r.judgment_version, ''),
		        COALESCE(r.intake ->> 'association', ''),
		        r.candidates,
		        r.identity_conflict,
		        (r.intake ->> 'receivedAt')::timestamptz,
		        r.recorded_at,
		        r.disposition_document
		   FROM node_operations.reception AS r
		  WHERE r.tenant_id = $1
		    AND r.intake IS NOT NULL
		    AND (r.intake ->> 'unit') = ANY($2)
		    AND NOT EXISTS (
		        SELECT 1
		          FROM node_operations.reception AS successor
		         WHERE successor.tenant_id = r.tenant_id
		           AND successor.predecessor_version = r.judgment_version)
		  ORDER BY r.intake ->> 'unit', r.source_id`,
		tenant.String(),
		units,
	)
	if err != nil {
		return fail(err)
	}
	defer heads.Close()
	for heads.Next() {
		var (
			unit            string
			head            ports.IdentityReviewHead
			candidatesJSON  []byte
			dispositionJSON []byte
		)
		if err := heads.Scan(
			&unit, &head.SourceID, &head.Kind, &head.Node, &head.JudgmentVersion, &head.Association,
			&candidatesJSON, &head.IdentityConflict, &head.ReceivedAt, &head.RecordedAt, &dispositionJSON,
		); err != nil {
			return fail(err)
		}
		if err := json.Unmarshal(candidatesJSON, &head.Candidates); err != nil {
			return fail(err)
		}
		if len(dispositionJSON) > 0 {
			if head.Disposition, err = domain.ParseIdentityDisposition(dispositionJSON); err != nil {
				return fail(fmt.Errorf("source %q: %w", head.SourceID, err))
			}
		}
		head.ReceivedAt = head.ReceivedAt.UTC()
		head.RecordedAt = head.RecordedAt.UTC()
		index := at[unit]
		page.Rows[index].Heads = append(page.Rows[index].Heads, head)
	}
	if err := heads.Err(); err != nil {
		return fail(err)
	}
	return page, nil
}
