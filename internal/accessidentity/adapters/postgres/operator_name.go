package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

var _ accessidentity.OperatorNameBook = (*OperatorRegistry)(nil)

// SaveOperatorName 先查本租户册上有没有这个主体再写，理由同 RegisterGrant：让外键报错会把整个事务打成失败态，
// 调用方就拿不到一个「没有这个操作者」的答复。
func (registry *OperatorRegistry) SaveOperatorName(
	ctx context.Context,
	tenantID string,
	subject accessidentity.OperatorSubject,
	name string,
	by accessidentity.OperatorSubject,
) (accessidentity.OperatorNameOutcome, error) {
	const operation = "save operator name"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	var bound bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM access_identity.operator WHERE issuer = $1 AND subject = $2 AND tenant_id = $3)`,
		subject.Issuer(), subject.Subject(), tenantID).Scan(&bound); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !bound {
		return accessidentity.OperatorNameOperatorNotFound, nil
	}
	if name == "" {
		if _, err := executor.Exec(ctx,
			`DELETE FROM access_identity.operator_name WHERE issuer = $1 AND subject = $2`,
			subject.Issuer(), subject.Subject()); err != nil {
			return "", fmt.Errorf("%s: %w", operation, err)
		}
		return accessidentity.OperatorNameCleared, nil
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO access_identity.operator_name (issuer, subject, tenant_id, name, recorded_by)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (issuer, subject) DO UPDATE
		    SET name = EXCLUDED.name, recorded_by = EXCLUDED.recorded_by, recorded_at = now()`,
		subject.Issuer(), subject.Subject(), tenantID, name, by.Issuer()+"#"+by.Subject()); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	return accessidentity.OperatorNameRecorded, nil
}

func (registry *OperatorRegistry) ListOperatorNames(ctx context.Context, tenantID string) ([]accessidentity.OperatorNameRow, error) {
	const operation = "list operator names"
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT issuer, subject, name FROM access_identity.operator_name
		  WHERE tenant_id = $1
		  ORDER BY issuer, subject`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()
	var names []accessidentity.OperatorNameRow
	for rows.Next() {
		var issuer, subject, name string
		if err := rows.Scan(&issuer, &subject, &name); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		parsed, err := accessidentity.NewOperatorSubject(issuer, subject)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		names = append(names, accessidentity.OperatorNameRow{Subject: parsed, Name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return names, nil
}
