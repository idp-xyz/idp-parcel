package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ParcelContainmentView 实现 ports.ParcelContainmentView：经收寄判断里的版本化关联把「这个
// 包裹」搬成「这些作业实物」，再问当前容纳索引（票 ps-port-remainder/05 的 NO 半边）。
//
// 三值的分界写在端口头注里；这里只记 SQL 上的两处取舍：
//
//   - 已关联那一路按 intake ->> 'association' 等值匹配，不按 kind 字面量筛——关联在场本身
//     就是「识别成功」的表达（FormNodeIntake 只在恰一候选时带上它），多筛一次 kind 只是把同一
//     件事说两遍。候选那一路则要按 kind 筛：候选列只在待识别格上有意义，形成格上它是空数组，
//     筛 kind 是为了让语义落在结果契约的那一格上，而不是依赖「反正是空的」。
//   - 容纳索引 containment_current 只收未关闭单元的当前成员（0002 自注），所以 JOIN 到它就是
//     「此刻在未关闭单元里」，不必再看单元的 phase；关闭时适配器删行，这一路自然读不到。
//
// 只读，走 ReadExecutor。
type ParcelContainmentView struct {
	db *bentopg.DB
}

func NewParcelContainmentView(db *bentopg.DB) (*ParcelContainmentView, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &ParcelContainmentView{db: db}, nil
}

var _ ports.ParcelContainmentView = (*ParcelContainmentView)(nil)

// LoadParcelContainment 一次往返取两个布尔（已关联实物在袋里 / 候选实物在袋里），再按端口
// 头注的分界折成三值：已关联在袋里压过一切；只有候选在袋里答`不可归属`；其余`不在`。
func (view *ParcelContainmentView) LoadParcelContainment(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelAssociationReference,
) (ports.ParcelContainment, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ParcelContainmentInvalid, fmt.Errorf("load parcel containment: %w", err)
	}

	var identifiedContained, candidateContained bool
	err = querier.QueryRow(ctx,
		`SELECT
		     EXISTS (
		         SELECT 1
		           FROM node_operations.reception r
		           JOIN node_operations.containment_current c
		             ON c.tenant_id = r.tenant_id
		            AND c.member_id = (r.intake ->> 'unit')
		          WHERE r.tenant_id = $1
		            AND (r.intake ->> 'association') = $2
		     ) AS identified_contained,
		     EXISTS (
		         SELECT 1
		           FROM node_operations.reception r
		           JOIN node_operations.containment_current c
		             ON c.tenant_id = r.tenant_id
		            AND c.member_id = (r.intake ->> 'unit')
		          WHERE r.tenant_id = $1
		            AND r.kind = $3
		            AND r.candidates @> jsonb_build_array($2::text)
		     ) AS candidate_contained`,
		tenant.String(), parcel.String(), ports.RecordPendingIdentification.String(),
	).Scan(&identifiedContained, &candidateContained)
	if err != nil {
		return ports.ParcelContainmentInvalid, fmt.Errorf("load parcel containment: %w", err)
	}

	switch {
	case identifiedContained:
		return ports.ParcelContained, nil
	case candidateContained:
		return ports.ParcelContainmentUnattributable, nil
	default:
		return ports.ParcelNotContained, nil
	}
}
