package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ExceptionCases 实现 ports.ActiveCaseView：按案件标识回答在场与未关闭。
//
// 只读不写。案件本体的读写归案件编排，本类型刻意不带 Save——处置请求侧要的只是
// 一个在场判据（「案件范围缩小、改派、归并或关闭前必须盘点全部未完成处置请求」，
// 关闭后再挂新请求就是绕过那次盘点），给它一个能写案件的对象是多给的权力。
//
// 租户在装配期固定：ActiveCaseView.CaseActive 的签名里没有租户，而案件标识只在
// 租户内唯一（ADR-0003）。把租户放进构造器，跨租户就仍然是装配期看得见的一次选择，
// 而不是查询期一个可以忘掉的参数。
type ExceptionCases struct {
	db     *bentopg.DB
	tenant domain.TenantID
}

// NewExceptionCases 装配在场视图。
//
// 允许零值租户，且那不是错误：本产品今天还没有租户，零值即「租户未登记」，此时
// 任何案件都不在场。构造期拒绝零值会让整个 VE 装配不起来，而 CaseActive 交回
// found=false 恰好是安全方向——处置请求挂不上去，不会挂到一个想象出来的案件上。
func NewExceptionCases(db *bentopg.DB, tenant domain.TenantID) (*ExceptionCases, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &ExceptionCases{db: db, tenant: tenant}, nil
}

var _ ports.ActiveCaseView = (*ExceptionCases)(nil)

func (view *ExceptionCases) CaseActive(
	ctx context.Context,
	caseID domain.CaseID,
) (bool, bool, error) {
	if view.tenant.String() == "" || caseID.String() == "" {
		return false, false, nil
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return false, false, fmt.Errorf("case active: %w", err)
	}

	var phase string
	err = querier.QueryRow(ctx,
		`SELECT phase
		   FROM visibility_exception.exception_case
		  WHERE tenant_id = $1 AND case_id = $2`,
		view.tenant.String(), caseID.String(),
	).Scan(&phase)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("case active: %w", err)
	}

	// 活 = 未关闭。待响应与处理中都在处置中，两者都接得住新的处置请求；已归并的
	// 案件走的是关闭那一格（结论 `MERGED`），因此这里自然把它排除——归并后应当挂到
	// 主案件上，而不是继续往被归并的案件里塞请求。
	return phase != casePhaseClosed, true, nil
}

const casePhaseClosed = "CLOSED"
