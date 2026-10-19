package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件是重核用的两问（ADR-0199 决定三）：按判断版本取一行、按号码找待重核的待识别。都读收寄册本尊，与 Receptions 写同一张
// 表，所以挂在它上面，不另立适配器。

var _ ports.PendingIdentityLookup = (*Receptions)(nil)

// FindByJudgmentVersion 按（租户，判断版本）取一行，走索引 reception_by_judgment_version（迁移 0008）。走 ReadExecutor：重核在
// 事务里调时读得到本事务刚追加的那一版。判断版本由本上下文的同一个签发器签出，一个租户里不该有两行同版本；读到两行就报错，
// 不任取一行——取错了就是重核了另一件实物。
func (repository *Receptions) FindByJudgmentVersion(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.IntakeResultVersion,
) (ports.ReceptionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReceptionRecord{}, false, fmt.Errorf("find reception by judgment version: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+receptionColumns+`
		   FROM node_operations.reception
		  WHERE tenant_id = $1
		    AND judgment_version = $2
		  LIMIT 2`,
		tenant.String(),
		version.String(),
	)
	if err != nil {
		return ports.ReceptionRecord{}, false, fmt.Errorf("find reception by judgment version: %w", err)
	}
	records, err := collectReceptions(rows, tenant)
	if err != nil {
		return ports.ReceptionRecord{}, false, fmt.Errorf("find reception by judgment version: %w", err)
	}
	switch len(records) {
	case 0:
		return ports.ReceptionRecord{}, false, nil
	case 1:
		return records[0], true, nil
	default:
		return ports.ReceptionRecord{}, false, fmt.Errorf("find reception by judgment version: %q names more than one judgment", version)
	}
}

// FindRecheckableByMark 按（租户，号码）找待重核的待识别，走索引 reception_recheckable_by_mark（迁移 0012 按 ADR-0211 决定二重建，
// 放进来由为只因歧义信号的冲突行）：谓词里那三格与索引的部分条件逐字同形——规划器只认得出同形的部分条件，改一边不改另一边就
// 悄悄退成全表扫。号码先比 md5 再比原文。当前有效版本这一格靠「没有一行指名它为前版」判，走 reception_predecessor_unique。
func (repository *Receptions) FindRecheckableByMark(
	ctx context.Context,
	tenant domain.TenantID,
	mark string,
) ([]ports.ReceptionRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find recheckable receptions by mark: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+receptionColumns+`
		   FROM node_operations.reception AS r
		  WHERE r.tenant_id = $1
		    AND r.kind = 'PENDING_IDENTIFICATION'
		    AND (NOT r.identity_conflict OR r.identity_conflict_cause = 'AMBIGUITY_ONLY')
		    AND r.source_document IS NOT NULL
		    AND md5(r.source_document ->> 'mark') = md5($2::text)
		    AND r.source_document ->> 'mark' = $2::text
		    AND r.intake ->> 'association' IS NULL
		    AND r.judgment_version IS NOT NULL
		    AND NOT EXISTS (
		        SELECT 1
		          FROM node_operations.reception AS successor
		         WHERE successor.tenant_id = r.tenant_id
		           AND successor.predecessor_version = r.judgment_version)
		  ORDER BY r.source_id`,
		tenant.String(),
		mark,
	)
	if err != nil {
		return nil, fmt.Errorf("find recheckable receptions by mark: %w", err)
	}
	records, err := collectReceptions(rows, tenant)
	if err != nil {
		return nil, fmt.Errorf("find recheckable receptions by mark: %w", err)
	}
	return records, nil
}

// collectReceptions 把一组行逐行经 scanReception 读回；没有行交回空切片。
func collectReceptions(rows pgx.Rows, tenant domain.TenantID) ([]ports.ReceptionRecord, error) {
	defer rows.Close()
	records := []ports.ReceptionRecord{}
	for rows.Next() {
		record, err := scanReception(rows, tenant)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
