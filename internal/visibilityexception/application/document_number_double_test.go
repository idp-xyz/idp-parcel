package application_test

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// documentNumberDouble 替编号组件的取号编排作答：缺省按调用次序出号，outcome 改成别的格就照那一格答。它同时站在两个
// 端口上——分诊编排只看得见 IssueCaseNumber，索赔编排只看得见 IssueClaimNumber，各自的用例数各自的调用。
type documentNumberDouble struct {
	prefix  string
	outcome ports.DocumentNumberIssueOutcome
	number  string
	err     error
	calls   int
	tenants []domain.TenantID
}

func newDocumentNumbers(prefix string) *documentNumberDouble {
	return &documentNumberDouble{prefix: prefix, outcome: ports.DocumentNumberAssigned}
}

func (double *documentNumberDouble) issue(tenant domain.TenantID) (ports.DocumentNumberIssue, error) {
	double.calls++
	double.tenants = append(double.tenants, tenant)
	if double.err != nil {
		return ports.DocumentNumberIssue{}, double.err
	}
	if double.outcome != ports.DocumentNumberAssigned {
		return ports.DocumentNumberIssue{Outcome: double.outcome, Number: double.number}, nil
	}
	number := double.number
	if number == "" {
		number = fmt.Sprintf("%s-%05d", double.prefix, double.calls)
	}
	return ports.DocumentNumberIssue{Outcome: ports.DocumentNumberAssigned, Number: number}, nil
}

func (double *documentNumberDouble) IssueCaseNumber(_ context.Context, tenant domain.TenantID) (ports.DocumentNumberIssue, error) {
	return double.issue(tenant)
}

func (double *documentNumberDouble) IssueClaimNumber(_ context.Context, tenant domain.TenantID) (ports.DocumentNumberIssue, error) {
	return double.issue(tenant)
}

func claimDocumentNumber(t interface {
	Helper()
	Fatalf(string, ...any)
}, raw string) domain.DocumentNumber {
	t.Helper()
	number, err := domain.NewDocumentNumber(raw)
	if err != nil {
		t.Fatalf("document number %q: %v", raw, err)
	}
	return number
}
