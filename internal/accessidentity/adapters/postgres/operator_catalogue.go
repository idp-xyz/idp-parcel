package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

var _ accessidentity.OperatorCatalogue = (*OperatorRegistry)(nil)

// ListOperators 用一条查询取本租户的全部操作者连同授予与撤销（ADR-0363 决定二）：分两次查会让绑定与授予取自两个时点。
// 主体按登记时刻排、同刻按主体串排，授予按登记时刻排、同刻按授予标识排：管理台显的是「谁先登的」，不是生效先后。
// 此刻生不生效不在这里判，理由同 FindOperator。
func (registry *OperatorRegistry) ListOperators(
	ctx context.Context,
	tenantID string,
) ([]accessidentity.OperatorCatalogueRow, error) {
	const operation = "list operators"
	tenant := strings.TrimSpace(tenantID)
	if tenant == "" {
		return nil, fmt.Errorf("%s: %w", operation, accessidentity.ErrOperatorCatalogueTenantRequired)
	}
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT o.issuer, o.subject, o.basis_ref, o.recorded_at,
		        g.grant_id, g.capability_face, g.effective_starts_at, g.effective_ends_at, g.basis_ref,
		        g.decision_kind, g.work_scope_kind, g.work_scope_ref, g.recorded_at, r.revoked_at, r.basis_ref
		   FROM access_identity.operator o
		   LEFT JOIN access_identity.operator_grant g
		     ON g.issuer = o.issuer AND g.subject = o.subject AND g.tenant_id = o.tenant_id
		   LEFT JOIN access_identity.operator_grant_revocation r
		     ON r.tenant_id = g.tenant_id AND r.grant_id = g.grant_id
		  WHERE o.tenant_id = $1
		  ORDER BY o.recorded_at, o.subject, g.recorded_at, g.grant_id`,
		tenant,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	// 行按主体连续到达（ORDER BY 先按主体排），一个主体的行攒齐就建一行查阅行。
	listed := make([]accessidentity.OperatorCatalogueRow, 0)
	var (
		current    accessidentity.OperatorSubject
		binding    accessidentity.OperatorBinding
		recordedAt time.Time
		grants     []accessidentity.RecordedGrantRow
		open       bool
	)
	flush := func() error {
		if !open {
			return nil
		}
		row, err := accessidentity.NewOperatorCatalogueRow(binding, recordedAt, grants)
		if err != nil {
			return err
		}
		listed = append(listed, row)
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
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		subject, err := accessidentity.NewOperatorSubject(issuer, subjectID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		if !open || subject != current {
			if err := flush(); err != nil {
				return nil, fmt.Errorf("%s: %w", operation, err)
			}
			binding, err = accessidentity.NewOperatorBinding(subject, tenant, basis)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", operation, err)
			}
			current, recordedAt, grants, open = subject, operatorRecordedAt, nil, true
		}
		if grantID == nil {
			continue
		}
		recorded, err := recordedGrantFromRow(tenant, subject,
			*grantID, *face, decisionKind, scopeKind, scopeRef, *startsAt, endsAt, *grantBasis, revokedAt, revocationBasis)
		if err != nil {
			return nil, fmt.Errorf("%s: 授予 %s：%w", operation, *grantID, err)
		}
		row, err := accessidentity.NewRecordedGrantRow(recorded, *grantRecordedAt)
		if err != nil {
			return nil, fmt.Errorf("%s: 授予 %s：%w", operation, *grantID, err)
		}
		grants = append(grants, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	if err := flush(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return listed, nil
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
