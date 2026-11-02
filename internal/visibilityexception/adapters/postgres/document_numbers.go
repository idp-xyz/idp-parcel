package postgres

import (
	"context"
	"errors"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// DocumentNumberAssigner 是编号组件的取号编排（docnumber.Assigner）在本适配器这一侧看得见的那一面。
type DocumentNumberAssigner interface {
	Assign(ctx context.Context, request docnumber.IssueRequest) (docnumber.Assignment, error)
}

// DocumentNumbers 实现 ports.CaseNumberIssuer 与 ports.ClaimNumberIssuer：把本上下文的异常案件与客户索赔项接到编号组件的取号编排上
// （ADR-0355 决定四），「这个号是否已用」查本上下文自己的单据表。
//
// 取号包在一个保存点里，撞号或出错就退回保存点：编排撞号时已经从序列取过跳号上限那么多个值，提交就全成空号（决定三只许
// 随单落定的跳号成为空号）。退回放在这里而不是交给调用方回滚整笔事务——异常案件的开立事务里同时落着投影派生，分诊
// 那一侧答未决时派生照常提交（UC-VE-002 的冲突信号搁置），不能为一件案件撞号让整笔回滚，也不能让撞号的流水随派生提交。
// 编排与保存点必须在同一个 *bentopg.DB 的同一笔事务里：序列存储按 DB 认事务归属，装配处用同一个 DB 构造两者。
type DocumentNumbers struct {
	db       *bentopg.DB
	assigner DocumentNumberAssigner
}

var (
	_ ports.CaseNumberIssuer  = (*DocumentNumbers)(nil)
	_ ports.ClaimNumberIssuer = (*DocumentNumbers)(nil)
)

func NewDocumentNumbers(db *bentopg.DB, assigner DocumentNumberAssigner) (*DocumentNumbers, error) {
	if db == nil || assigner == nil {
		return nil, errors.New("visibility exception postgres: document numbers need a db and an assigner")
	}
	return &DocumentNumbers{db: db, assigner: assigner}, nil
}

// documentNumberSavepoint 是取号那一段的保存点名。同一笔事务里先后几次取号各自建、各自释放，
// 不嵌套，用同一个名字不会互相遮住。
const documentNumberSavepoint = "visibility_exception_document_number"

func (numbers *DocumentNumbers) IssueCaseNumber(ctx context.Context, tenant domain.TenantID) (ports.DocumentNumberIssue, error) {
	return numbers.issue(ctx, tenant, domain.ExceptionCaseDocumentNumberType.Code,
		`SELECT EXISTS (
		        SELECT 1 FROM visibility_exception.exception_case
		         WHERE tenant_id = $1 AND document_number = $2)`)
}

func (numbers *DocumentNumbers) IssueClaimNumber(ctx context.Context, tenant domain.TenantID) (ports.DocumentNumberIssue, error) {
	return numbers.issue(ctx, tenant, domain.ClaimDocumentNumberType.Code,
		`SELECT EXISTS (
		        SELECT 1 FROM visibility_exception.claim_item
		         WHERE tenant_id = $1 AND document_number = $2)`)
}

func (numbers *DocumentNumbers) issue(
	ctx context.Context,
	tenant domain.TenantID,
	code docnumber.TypeCode,
	usedQuery string,
) (ports.DocumentNumberIssue, error) {
	if tenant.String() == "" {
		return ports.DocumentNumberIssue{}, errors.New("issue document number: tenant is empty")
	}
	executor, err := numbers.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DocumentNumberIssue{}, fmt.Errorf("issue %s number: %w", code, err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT "+documentNumberSavepoint); err != nil {
		return ports.DocumentNumberIssue{}, fmt.Errorf("issue %s number: %w", code, err)
	}
	assignment, err := numbers.assigner.Assign(ctx, docnumber.IssueRequest{
		Tenant: tenant.String(),
		Type:   code,
		IsUsed: func(ctx context.Context, number string) (bool, error) {
			var used bool
			err := executor.QueryRow(ctx, usedQuery, tenant.String(), number).Scan(&used)
			return used, err
		},
	})
	if err != nil {
		// 出错同样退回保存点：编排可能已取过值；退回之后事务仍可用，回不回滚整笔由调用方按端口契约办。
		if _, rollbackErr := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+documentNumberSavepoint); rollbackErr != nil {
			return ports.DocumentNumberIssue{}, fmt.Errorf("issue %s number: %w (and the savepoint did not roll back: %v)", code, err, rollbackErr)
		}
		return ports.DocumentNumberIssue{}, fmt.Errorf("issue %s number: %w", code, err)
	}

	var issue ports.DocumentNumberIssue
	switch assignment.Outcome {
	case docnumber.NumberAssigned:
		issue = ports.DocumentNumberIssue{Outcome: ports.DocumentNumberAssigned, Number: assignment.Number}
	case docnumber.NumberingTimeZoneNotConfigured:
		issue = ports.DocumentNumberIssue{Outcome: ports.DocumentNumberingTimeZoneNotConfigured}
	case docnumber.NumberingRuleCollision:
		if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+documentNumberSavepoint); err != nil {
			return ports.DocumentNumberIssue{}, fmt.Errorf("issue %s number: release the collided sequence values: %w", code, err)
		}
		issue = ports.DocumentNumberIssue{Outcome: ports.DocumentNumberingRuleCollision, Template: assignment.Template}
	default:
		_, _ = executor.Exec(ctx, "ROLLBACK TO SAVEPOINT "+documentNumberSavepoint)
		return ports.DocumentNumberIssue{}, fmt.Errorf("issue %s number: unexpected assignment outcome %q", code, assignment.Outcome)
	}
	if _, err := executor.Exec(ctx, "RELEASE SAVEPOINT "+documentNumberSavepoint); err != nil {
		return ports.DocumentNumberIssue{}, fmt.Errorf("issue %s number: %w", code, err)
	}
	return issue, nil
}
