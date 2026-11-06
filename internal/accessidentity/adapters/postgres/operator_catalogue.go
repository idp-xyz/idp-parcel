package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

var _ accessidentity.OperatorCatalogue = (*OperatorRegistry)(nil)

// operatorCataloguePages 把 accessidentity.OperatorCataloguePages 落到操作者表 o 的列上。取页语句里 o 先是操作者表、再是
// 取出的本页（子查询同名），两处列名同形，同一份落列在两处都成立。
var operatorCataloguePages = accessidentity.OperatorCataloguePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "o.recorded_at"},
	Identity: []string{"o.issuer", "o.subject"},
	Keyword:  []string{"o.subject", "o.basis_ref"},
})

// scannedOperator 是一个操作者扫描回来的排序键列与建好的查阅行：游标位置取列上原样读回的值，不取领域对象里的副本。
type scannedOperator struct {
	issuer, subject string
	recordedAt      time.Time
	row             accessidentity.OperatorCatalogueRow
}

// ListOperators 用一条查询取本页操作者连同各自的授予与撤销（ADR-0363 决定二）：分两次查会让绑定与授予取自两个时点。本页先
// 在操作者表上按 accessidentity.OperatorCataloguePages 取出（带 LIMIT 的子查询），授予与撤销只随本页的操作者连接，不为翻
// 不到的操作者扫授予。授予在操作者名下按登记时刻排、同刻按授予标识排：管理台显的是「谁先登的」，不是生效先后。总数另起
// 一句：与本页同一组条件，不保证同一快照——要同快照的是一个操作者的绑定与授予，不是页与总数。
// 此刻生不生效不在这里判，理由同 FindOperator。limit 非正是调用方编程错误。
func (registry *OperatorRegistry) ListOperators(
	ctx context.Context,
	tenantID string,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[accessidentity.OperatorCatalogueRow], error) {
	const operation = "list operators"
	fail := func(err error) (cataloguepage.Result[accessidentity.OperatorCatalogueRow], error) {
		return cataloguepage.Result[accessidentity.OperatorCatalogueRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	tenant := strings.TrimSpace(tenantID)
	if tenant == "" {
		return fail(accessidentity.ErrOperatorCatalogueTenantRequired)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := operatorCataloguePages.Plan(query, "o.tenant_id = $1", []any{tenant}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "access_identity.operator AS o"
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	pageSQL := plan.PageSQL("o.issuer, o.subject, o.tenant_id, o.basis_ref, o.recorded_at", source)
	rows, err := querier.Query(ctx,
		`SELECT o.issuer, o.subject, o.basis_ref, o.recorded_at,
		        g.grant_id, g.capability_face, g.effective_starts_at, g.effective_ends_at, g.basis_ref,
		        g.decision_kind, g.work_scope_kind, g.work_scope_ref, g.recorded_at, r.revoked_at, r.basis_ref
		   FROM (`+pageSQL+`) AS o
		   LEFT JOIN access_identity.operator_grant g
		     ON g.issuer = o.issuer AND g.subject = o.subject AND g.tenant_id = o.tenant_id
		   LEFT JOIN access_identity.operator_grant_revocation r
		     ON r.tenant_id = g.tenant_id AND r.grant_id = g.grant_id
		  ORDER BY `+plan.OrderBy+`, g.recorded_at, g.grant_id`,
		plan.PageArgs...,
	)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	// 行按操作者连续到达（ORDER BY 先按操作者的排序键排，键含主键），一个操作者的行攒齐就建一行查阅行。
	var (
		fetched []scannedOperator
		current scannedOperator
		binding accessidentity.OperatorBinding
		grants  []accessidentity.RecordedGrantRow
		open    bool
	)
	flush := func() error {
		if !open {
			return nil
		}
		row, err := accessidentity.NewOperatorCatalogueRow(binding, current.recordedAt, grants)
		if err != nil {
			return err
		}
		current.row = row
		fetched = append(fetched, current)
		return nil
	}
	for rows.Next() {
		var (
			issuer, subjectID, basis          string
			operatorRecordedAt                time.Time
			grantID, face, grantBasis         *string
			startsAt, endsAt, grantRecordedAt *time.Time
			decisionKind, scopeKind, scopeRef *string
			revokedAt                         *time.Time
			revocationBasis                   *string
		)
		if err := rows.Scan(&issuer, &subjectID, &basis, &operatorRecordedAt,
			&grantID, &face, &startsAt, &endsAt, &grantBasis,
			&decisionKind, &scopeKind, &scopeRef, &grantRecordedAt, &revokedAt, &revocationBasis,
		); err != nil {
			return fail(err)
		}
		if !open || issuer != current.issuer || subjectID != current.subject {
			if err := flush(); err != nil {
				return fail(err)
			}
			subject, err := accessidentity.NewOperatorSubject(issuer, subjectID)
			if err != nil {
				return fail(err)
			}
			binding, err = accessidentity.NewOperatorBinding(subject, tenant, basis)
			if err != nil {
				return fail(err)
			}
			current = scannedOperator{issuer: issuer, subject: subjectID, recordedAt: operatorRecordedAt}
			grants, open = nil, true
		}
		if grantID == nil {
			continue
		}
		recorded, err := recordedGrantFromRow(tenant, binding.Subject(),
			*grantID, *face, decisionKind, scopeKind, scopeRef, *startsAt, endsAt, *grantBasis, revokedAt, revocationBasis)
		if err != nil {
			return fail(fmt.Errorf("授予 %s：%w", *grantID, err))
		}
		row, err := accessidentity.NewRecordedGrantRow(recorded, *grantRecordedAt)
		if err != nil {
			return fail(fmt.Errorf("授予 %s：%w", *grantID, err))
		}
		grants = append(grants, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	if err := flush(); err != nil {
		return fail(err)
	}

	page, err := cataloguepage.Finish(query, fetched, limit, total, operatorPosition)
	if err != nil {
		return fail(err)
	}
	listed := make([]accessidentity.OperatorCatalogueRow, 0, len(page.Rows))
	for _, scanned := range page.Rows {
		listed = append(listed, scanned.row)
	}
	return cataloguepage.Result[accessidentity.OperatorCatalogueRow]{Rows: listed, Next: page.Next, Total: page.Total}, nil
}

// operatorPosition 取本页末个操作者扫描回来的列值：登记时刻与（发行方、主体）都从 o 的列上原样读回，与 ORDER BY 用的是
// 同一份。本册只有登记时刻一个可排维。
func operatorPosition(row scannedOperator, _ string) cataloguepage.Position {
	return cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(row.recordedAt),
		Identity: []string{row.issuer, row.subject},
	}
}

// FindGrant 按（租户、授予标识）取一笔授予连同撤销，给在线撤销口在写入前判防锁死（ADR-0363 决定三）。别的租户拿同一个
// 授予标识读不到这一笔。
func (registry *OperatorRegistry) FindGrant(
	ctx context.Context,
	tenantID, grantID string,
) (accessidentity.RecordedGrant, bool, error) {
	const operation = "find operator grant"
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return accessidentity.RecordedGrant{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var (
		issuer, subjectID, face, basis string
		startsAt                       time.Time
		endsAt, revokedAt              *time.Time
		decisionKind                   *string
		scopeKind, scopeRef            *string
		revocationBasis                *string
	)
	err = querier.QueryRow(ctx,
		`SELECT g.issuer, g.subject, g.capability_face, g.effective_starts_at, g.effective_ends_at, g.basis_ref,
		        g.decision_kind, g.work_scope_kind, g.work_scope_ref, r.revoked_at, r.basis_ref
		   FROM access_identity.operator_grant g
		   LEFT JOIN access_identity.operator_grant_revocation r
		     ON r.tenant_id = g.tenant_id AND r.grant_id = g.grant_id
		  WHERE g.tenant_id = $1 AND g.grant_id = $2`,
		strings.TrimSpace(tenantID), strings.TrimSpace(grantID),
	).Scan(&issuer, &subjectID, &face, &startsAt, &endsAt, &basis,
		&decisionKind, &scopeKind, &scopeRef, &revokedAt, &revocationBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return accessidentity.RecordedGrant{}, false, nil
	}
	if err != nil {
		return accessidentity.RecordedGrant{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	subject, err := accessidentity.NewOperatorSubject(issuer, subjectID)
	if err != nil {
		return accessidentity.RecordedGrant{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	recorded, err := recordedGrantFromRow(strings.TrimSpace(tenantID), subject,
		strings.TrimSpace(grantID), face, decisionKind, scopeKind, scopeRef, startsAt, endsAt, basis, revokedAt, revocationBasis)
	if err != nil {
		return accessidentity.RecordedGrant{}, false, fmt.Errorf("%s: 授予 %s：%w", operation, grantID, err)
	}
	return recorded, true, nil
}
