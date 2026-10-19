package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件是收寄编排认同一作业实物的那一问（ADR-0199 决定四）：读收寄册本尊，与 Receptions 写同一张表，挂在它上面。

// FindCurrentJudgmentsAtNode 按（租户，作业实物，节点）交回各条判断链的当前有效版本（ADR-0199 决定四），走索引
// reception_judgments_by_unit_and_node（迁移 0009）：键与谓词与索引逐字同形。当前有效版本靠「没有一行指名它为前版」判，走
// reception_predecessor_unique。
//
// 查询之前按（租户，作业实物）取事务级咨询锁，把同一件实物上「认当前收寄 → 落册」串起来：没有它，同一件实物两个来源在两笔
// 并发事务里送站，READ COMMITTED 下各自读不到对方尚未提交的收寄，两条都形成控制起点。后到的一方等在锁上，先到的提交后才查，
// 这条查询另取快照，读得到刚提交的那一行。锁随事务释放，所以走 RequireExecutor（理由同 FindOtherUnitIntakes）。锁在实物上、不在
// 节点上：同一件实物不会同时在两个节点送站，多分一维只省下不存在的并发。锁键口径与包裹锁同：各维以 chr(31) 分隔后取
// hashtext，锁空间名不同，同一个串作实物与作包裹不互等。
func (repository *Receptions) FindCurrentJudgmentsAtNode(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.HandlingUnitID,
	node domain.NodeReference,
) ([]ports.ReceptionRecord, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find current judgments at node: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('node_operations.reception.handling_unit'), hashtext($1 || chr(31) || $2))`,
		tenant.String(), unit.String(),
	); err != nil {
		return nil, fmt.Errorf("find current judgments at node: 取实物锁：%w", err)
	}
	rows, err := executor.Query(ctx,
		`SELECT `+receptionColumns+`
		   FROM node_operations.reception AS r
		  WHERE r.tenant_id = $1
		    AND COALESCE(r.intake ->> 'unit', r.source_document ->> 'unit') = $2
		    AND COALESCE(r.intake ->> 'node', r.source_document ->> 'node') = $3
		    AND r.kind <> 'MERGED_INTO_EXISTING_INTAKE'
		    AND r.judgment_version IS NOT NULL
		    AND NOT EXISTS (
		        SELECT 1
		          FROM node_operations.reception AS successor
		         WHERE successor.tenant_id = r.tenant_id
		           AND successor.predecessor_version = r.judgment_version)
		  ORDER BY r.source_id`,
		tenant.String(),
		unit.String(),
		node.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find current judgments at node: %w", err)
	}
	records, err := collectReceptions(rows, tenant)
	if err != nil {
		return nil, fmt.Errorf("find current judgments at node: %w", err)
	}
	return records, nil
}
