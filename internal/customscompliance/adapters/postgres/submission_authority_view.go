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

// SubmissionAuthorityView 实现 ports.SubmissionAuthorityView：读申报单元的提交授权。
//
// **这本册子不是接入认证。** CONTEXT 硬句 165 要求提交授权请求、批准、拒绝、撤销、
// 申报渠道账号使用和实际提交权限分别表达，并明写「能够登录系统或持有账号凭据均不能
// 自动取得其他权限」——接入渠道认证（`PAR-INT-01`）回答的是「这个请求来自哪个租户」，
// 本口回答的是「这个申报单元当前有没有有效的提交授权依据」。把后者接到前者上就等于
// 让登录直接换来报关权，正是该硬句要禁的合并。签名本身也证得了这一点：租户是入参，
// 而接入认证的产物才是租户。
//
// 与 ReadinessView 同形三态、分表存放：两条轨分别形成和失效（CONTEXT 244），就绪在场
// 顶替不了授权。
type SubmissionAuthorityView struct {
	db *bentopg.DB
}

func NewSubmissionAuthorityView(db *bentopg.DB) (*SubmissionAuthorityView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &SubmissionAuthorityView{db: db}, nil
}

var _ ports.SubmissionAuthorityView = (*SubmissionAuthorityView)(nil)

// LoadSubmissionAuthority 取回授权判断并整门重建：授予先立住，撤销再追加——原授予的
// 依据与时间原样保留，失效不是删除。
func (view *SubmissionAuthorityView) LoadSubmissionAuthority(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.DeclarationUnitID,
) (domain.SubmissionAuthorization, bool, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SubmissionAuthorization{}, false, fmt.Errorf("load submission authority: %w", err)
	}

	var (
		authorityRef string
		grantedAt    time.Time
		revokedBy    *string
		revokedAt    *time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT authority_ref, granted_at, revoked_by, revoked_at
		   FROM customs_compliance.submission_authority
		  WHERE tenant_id = $1 AND unit_id = $2`,
		tenant.String(), unit.String(),
	).Scan(&authorityRef, &grantedAt, &revokedBy, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SubmissionAuthorization{}, false, nil
	}
	if err != nil {
		return domain.SubmissionAuthorization{}, false, fmt.Errorf("load submission authority: %w", err)
	}

	authority, err := domain.NewSubmissionAuthorityReference(authorityRef)
	if err != nil {
		return domain.SubmissionAuthorization{}, false, fmt.Errorf("rebuild submission authority: %w", err)
	}
	authorization, err := domain.GrantSubmissionAuthority(unit, authority, grantedAt)
	if err != nil {
		return domain.SubmissionAuthorization{}, false, fmt.Errorf("rebuild submission authority: %w", err)
	}
	if revokedBy == nil {
		return authorization, true, nil
	}
	revoked, err := authorization.Revoke(*revokedBy, *revokedAt)
	if err != nil {
		return domain.SubmissionAuthorization{}, false, fmt.Errorf("rebuild submission authority: %w", err)
	}
	return revoked, true, nil
}
