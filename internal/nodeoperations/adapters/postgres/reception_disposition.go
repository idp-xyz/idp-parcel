package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件是身份冲突处置要问收寄册的那几样（ADR-0210 决定二至四、八）：按实物读各行，以及 FindOtherUnitIntakes 与
// ParcelContainmentView 共用的「处置截断」谓词。都读收寄册本尊，挂在 Receptions 上，不另立适配器。

var _ ports.IdentityDispositionRegister = (*Receptions)(nil)

// FindUnitIntakes 按（租户，作业实物）取带收寄的各行，走索引 reception_intake_unit_idx（迁移 0005）。不在 SQL 里先筛链头：哪一版是
// 链头由领域在这一组里认，查询条件与索引因此不随链变（同 IntakeIdentities.FindIntakeIdentities 的取舍）。走 ReadExecutor：处置在
// 事务里调时读得到本事务刚追加的那一版。
func (repository *Receptions) FindUnitIntakes(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.HandlingUnitID,
) ([]ports.ReceptionRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find unit intakes: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+receptionColumns+`
		   FROM node_operations.reception
		  WHERE tenant_id = $1
		    AND intake IS NOT NULL
		    AND (intake ->> 'unit') = $2
		  ORDER BY source_id`,
		tenant.String(),
		unit.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find unit intakes: %w", err)
	}
	records, err := collectReceptions(rows, tenant)
	if err != nil {
		return nil, fmt.Errorf("find unit intakes: %w", err)
	}
	return records, nil
}

// succeededByDisposition 是「这一行被身份冲突处置版接续过」的 SQL 谓词（ADR-0210 决定八）：从这一行沿后继走，后面有一版带处置
// 文档。alias 是外层那一行的表名或别名，谓词只引它的租户与判断版本，不占外层的参数位。每一步按（租户，前版）找唯一的那一行，走
// 索引 reception_predecessor_unique；步数照 CurrentVersion 封顶，绕过编排写出的环在这里只是走不到处置版，不让查询不收敛。
//
// 截断取「沿后继走」而不是在处置版上记下它截断的各版：链是只追加的，处置版一落，它前面的每一版沿后继都走得到它，不必在写处置版时
// 把整条链抄一遍，也就不会有「抄漏了一版」这种只在某条长链上才露出来的错。
func succeededByDisposition(alias string) string {
	return fmt.Sprintf(`EXISTS (
		         WITH RECURSIVE later (version, disposed, depth) AS (
		             SELECT successor.judgment_version, successor.disposition_document IS NOT NULL, 1
		               FROM node_operations.reception AS successor
		              WHERE successor.tenant_id = %[1]s.tenant_id
		                AND successor.predecessor_version = %[1]s.judgment_version
		           UNION ALL
		             SELECT successor.judgment_version, successor.disposition_document IS NOT NULL, later.depth + 1
		               FROM later
		               JOIN node_operations.reception AS successor
		                 ON successor.tenant_id = %[1]s.tenant_id
		                AND successor.predecessor_version = later.version
		              WHERE later.depth < %[2]d)
		         SELECT 1 FROM later WHERE later.disposed)`, alias, maxJudgmentChain)
}
