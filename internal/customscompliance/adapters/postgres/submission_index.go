package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// SubmissionIndex 实现 ports.SubmissionIndex：按版本回答「本系统有没有这份提交」。
//
// 它读的是提交链自己的 declaration_submission 表——版本标识在那里有唯一约束
// （declaration_submission_version_unique），因此这一问不需要第二本索引册。另立一本
// 就会有两处说法，而它们不一致的那一刻，归属判断会按错的那本走。
type SubmissionIndex struct {
	db *bentopg.DB
}

func NewSubmissionIndex(db *bentopg.DB) (*SubmissionIndex, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &SubmissionIndex{db: db}, nil
}

var _ ports.SubmissionIndex = (*SubmissionIndex)(nil)

// FindSubmission 报告该租户下是否存在此提交版本。found=false 是「归属不上」的如实
// 答案，接收编排据以留存不猜（CONTEXT「不得据此猜测提交、补造缺失层次」）——因此这里绝不能把读取失败折成
// false：那会让一次依赖故障被读成「监管机构回的是一份我们没提交过的申报」。
func (index *SubmissionIndex) FindSubmission(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.SubmissionVersionID,
) (bool, error) {
	querier, err := index.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("find submission: %w", err)
	}

	var exists bool
	err = querier.QueryRow(ctx,
		`SELECT true
		   FROM customs_compliance.declaration_submission
		  WHERE tenant_id = $1 AND version_id = $2`,
		tenant.String(), version.String(),
	).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find submission: %w", err)
	}
	return exists, nil
}
