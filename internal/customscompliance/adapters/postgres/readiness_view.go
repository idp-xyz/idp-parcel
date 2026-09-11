package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ReadinessView 实现 ports.ReadinessView：读申报单元的就绪判断。
//
// 三态逐格分开（CONTEXT「提交授权与就绪判断分别形成和失效」）：无行=就绪规则/资格目录未配置（实例半边，编排停在
// 未决）；有行未撤销=仍就绪；有行已撤销=`不再就绪`（业务负向，恢复动作是重新取得
// 就绪）。撤销那格是本适配器存在的理由——把失效读成未配置会把「重新取得就绪」错
// 指成「等实例参数」。
type ReadinessView struct {
	db *bentopg.DB
}

func NewReadinessView(db *bentopg.DB) (*ReadinessView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &ReadinessView{db: db}, nil
}

var _ ports.ReadinessView = (*ReadinessView)(nil)

// LoadReadiness 取回就绪判断并整门重建：原判断（依据与时间）先经 JudgeReady 立住，
// 撤销再经 Revoke 追加——失效不是删除，两段都得走领域的构造门，坏行在这里就拦下，
// 不留给编排去猜。
func (view *ReadinessView) LoadReadiness(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.DeclarationUnitID,
) (domain.ReadinessJudgment, bool, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ReadinessJudgment{}, false, fmt.Errorf("load readiness: %w", err)
	}

	var (
		basisRef  string
		judgedAt  time.Time
		revokedBy *string
		revokedAt *time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT basis_ref, judged_at, revoked_by, revoked_at
		   FROM customs_compliance.readiness_judgment
		  WHERE tenant_id = $1 AND unit_id = $2`,
		tenant.String(), unit.String(),
	).Scan(&basisRef, &judgedAt, &revokedBy, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReadinessJudgment{}, false, nil
	}
	if err != nil {
		return domain.ReadinessJudgment{}, false, fmt.Errorf("load readiness: %w", err)
	}

	basis, err := domain.NewReadinessBasisReference(basisRef)
	if err != nil {
		return domain.ReadinessJudgment{}, false, fmt.Errorf("rebuild readiness: %w", err)
	}
	judgment, err := domain.JudgeReady(unit, basis, judgedAt)
	if err != nil {
		return domain.ReadinessJudgment{}, false, fmt.Errorf("rebuild readiness: %w", err)
	}
	if revokedBy == nil {
		return judgment, true, nil
	}
	revoked, err := judgment.Revoke(*revokedBy, *revokedAt)
	if err != nil {
		return domain.ReadinessJudgment{}, false, fmt.Errorf("rebuild readiness: %w", err)
	}
	return revoked, true, nil
}
