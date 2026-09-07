package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ParcelDeclarationFactsView 实现 ports.ParcelDeclarationFactsView：按正式包裹引用反查申报
// 单元、提交版本与案件关闭三本册子，各答一件事实（票 ps-port-remainder/05 的 CC 半边）。
//
// 三问一条 SQL、三个各自独立的 EXISTS：不在 SQL 里也不在 Go 里把它们折成一个阶段——哪一格
// 压过哪一格是消费方的判断，本上下文只说册子里有什么。三格的读法都写在端口头注里，这里只
// 记 SQL 上不显眼的两处：
//
//   - 「当前单元」排掉被替代的那些（另一单元的 replaces_unit_id 指向它）；替代关系今天还没
//     有写入方，这一句先按列的语义写好，写入方来了读面不必改。
//   - 提交版本按版本行自己的组成快照反查，不经单元——版本固定时快照的组成才是「交出去的
//     那一份」，且版本永久保留，不随单元被替代而消失。
//
// 只读，走 ReadExecutor：事务内读得到本事务刚写的行，事务外用注入的连接池。
type ParcelDeclarationFactsView struct {
	db *bentopg.DB
}

func NewParcelDeclarationFactsView(db *bentopg.DB) (*ParcelDeclarationFactsView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &ParcelDeclarationFactsView{db: db}, nil
}

var _ ports.ParcelDeclarationFactsView = (*ParcelDeclarationFactsView)(nil)

// LoadParcelDeclarationFacts 一次往返答三格。查询恒返回一行三列布尔，没有 ErrNoRows 那一支：
// 「查无此包裹」不是缺行，是三格皆否。
func (view *ParcelDeclarationFactsView) LoadParcelDeclarationFacts(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelReference,
) (ports.ParcelDeclarationFacts, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ParcelDeclarationFacts{}, fmt.Errorf("load parcel declaration facts: %w", err)
	}

	var facts ports.ParcelDeclarationFacts
	err = querier.QueryRow(ctx,
		`WITH current_unit AS (
		     SELECT u.unit_id, u.case_id
		       FROM customs_compliance.declaration_unit u
		      WHERE u.tenant_id = $1
		        AND u.members @> jsonb_build_array($2::text)
		        AND NOT EXISTS (
		                SELECT 1
		                  FROM customs_compliance.declaration_unit r
		                 WHERE r.tenant_id = u.tenant_id
		                   AND r.replaces_unit_id = u.unit_id
		            )
		 ),
		 parcel_case AS (
		     SELECT cu.case_id FROM current_unit cu
		     UNION
		     SELECT c.case_id
		       FROM customs_compliance.customs_case c
		      WHERE c.tenant_id = $1
		        AND c.parcels @> jsonb_build_array(jsonb_build_object('parcel', $2::text))
		 )
		 SELECT
		     EXISTS (
		         SELECT 1
		           FROM current_unit cu
		          WHERE NOT EXISTS (
		                    SELECT 1
		                      FROM customs_compliance.declaration_submission s
		                     WHERE s.tenant_id = $1
		                       AND s.unit_id = cu.unit_id
		                )
		     ) AS member_of_unsubmitted_unit,
		     EXISTS (
		         SELECT 1
		           FROM customs_compliance.declaration_submission s
		          WHERE s.tenant_id = $1
		            AND s.members @> jsonb_build_array($2::text)
		     ) AS in_fixed_submission_version,
		     EXISTS (
		         SELECT 1
		           FROM parcel_case pc
		           JOIN customs_compliance.case_closure cl
		             ON cl.tenant_id = $1
		            AND cl.case_ref = pc.case_id
		          WHERE jsonb_array_length(cl.reopenings) = 0
		     ) AS in_closed_case`,
		tenant.String(), parcel.String(),
	).Scan(&facts.MemberOfUnsubmittedUnit, &facts.InFixedSubmissionVersion, &facts.InClosedCase)
	if err != nil {
		return ports.ParcelDeclarationFacts{}, fmt.Errorf("load parcel declaration facts: %w", err)
	}
	return facts, nil
}
