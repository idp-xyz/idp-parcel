package application_test

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// documentNumberDouble 替编号组件的取号编排作答：缺省按调用次序出号，outcome 改成别的格就照那一格答。它同时站在两个
// 端口上——分诊编排只看得见 IssueCaseNumber，索赔编排只看得见 IssueClaimNumber，各自的用例数各自的调用。
type documentNumberDouble struct {
	prefix   string
	outcome  ports.DocumentNumberIssueOutcome
	number   string
	template string
	err      error
	calls    int
	tenants  []domain.TenantID
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
		template := double.template
		if template == "" && double.outcome == ports.DocumentNumberingRuleCollision {
			template = "EX-{yyyy}{mm}{dd}-{seq:4}"
		}
		return ports.DocumentNumberIssue{Outcome: double.outcome, Number: double.number, Template: template}, nil
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

// collisionBookDouble 是案件撞号册的替身：记下的、推后的、销掉的各记一份，到期按 next 判。
type collisionBookDouble struct {
	recorded  []ports.CaseNumberingCollision
	next      map[ports.CaseNumberingCollisionKey]time.Time
	rows      map[ports.CaseNumberingCollisionKey]ports.CaseNumberingCollision
	resolved  []ports.CaseNumberingCollisionKey
	recordErr error
}

func newCollisionBook() *collisionBookDouble {
	return &collisionBookDouble{
		next: map[ports.CaseNumberingCollisionKey]time.Time{},
		rows: map[ports.CaseNumberingCollisionKey]ports.CaseNumberingCollision{},
	}
}

func (double *collisionBookDouble) RecordCaseNumberingCollision(_ context.Context, collision ports.CaseNumberingCollision) error {
	if double.recordErr != nil {
		return double.recordErr
	}
	double.recorded = append(double.recorded, collision)
	double.rows[collision.Key()] = collision
	double.next[collision.Key()] = collision.At
	return nil
}

func (double *collisionBookDouble) DueCaseNumberingCollisions(_ context.Context, at time.Time, limit int) ([]ports.CaseNumberingCollision, error) {
	var due []ports.CaseNumberingCollision
	for key, row := range double.rows {
		if !double.next[key].After(at) && len(due) < limit {
			due = append(due, row)
		}
	}
	return due, nil
}

func (double *collisionBookDouble) PostponeCaseNumberingCollision(_ context.Context, key ports.CaseNumberingCollisionKey, until time.Time) error {
	if _, present := double.rows[key]; present {
		double.next[key] = until
	}
	return nil
}

func (double *collisionBookDouble) ResolveCaseNumberingCollision(_ context.Context, key ports.CaseNumberingCollisionKey) error {
	double.resolved = append(double.resolved, key)
	delete(double.rows, key)
	delete(double.next, key)
	return nil
}
