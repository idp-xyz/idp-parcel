package docnumbering_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/docnumbering"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
)

type assignerDouble struct {
	assignment docnumber.Assignment
	err        error
	request    docnumber.IssueRequest
}

func (double *assignerDouble) Assign(_ context.Context, request docnumber.IssueRequest) (docnumber.Assignment, error) {
	double.request = request
	return double.assignment, double.err
}

type usedDouble struct {
	tenant domain.TenantID
	number string
}

func (double *usedDouble) ShipmentDocumentNumberUsed(_ context.Context, tenant domain.TenantID, number string) (bool, error) {
	double.tenant, double.number = tenant, number
	return number == "P20261102000001", nil
}

func tenant(t *testing.T) domain.TenantID {
	t.Helper()
	value, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// Covers: ADR-0025「翻译必须是全函数」——取号编排的三格各有落点；请求按委托的类型声明取`SHIPMENT`、不交开立方业务时区，
// 判撞查询按同一租户查本上下文的表。
func TestTheIssuerTranslatesEveryAssignmentOutcome(t *testing.T) {
	cases := []struct {
		name       string
		assignment docnumber.Assignment
		want       ports.ShipmentDocumentNumberIssueOutcome
	}{
		{"assigned", docnumber.Assignment{Outcome: docnumber.NumberAssigned, Number: "P20261102000002"}, ports.ShipmentDocumentNumberAssigned},
		{"time zone not configured", docnumber.Assignment{Outcome: docnumber.NumberingTimeZoneNotConfigured}, ports.ShipmentDocumentNumberingTimeZoneNotConfigured},
		{"collision", docnumber.Assignment{Outcome: docnumber.NumberingRuleCollision, Skipped: docnumber.MaxSkippedNumbers}, ports.ShipmentDocumentNumberRuleCollision},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assigner := &assignerDouble{assignment: testCase.assignment}
			used := &usedDouble{}
			issuer, err := docnumbering.NewIssuer(assigner, used)
			if err != nil {
				t.Fatal(err)
			}
			issue, err := issuer.IssueShipmentDocumentNumber(t.Context(), tenant(t))
			if err != nil {
				t.Fatalf("issue: %v", err)
			}
			if issue.Outcome != testCase.want {
				t.Fatalf("outcome = %d, want %d", issue.Outcome, testCase.want)
			}
			if testCase.want == ports.ShipmentDocumentNumberAssigned && issue.Number.String() != testCase.assignment.Number {
				t.Fatalf("number = %q", issue.Number)
			}
			if testCase.want != ports.ShipmentDocumentNumberAssigned && issue.Number.String() != "" {
				t.Fatalf("a number came with %s", testCase.name)
			}
			if assigner.request.Type != domain.ShipmentDocumentNumberType.Code || assigner.request.Tenant != "SYN-TENANT-01" || assigner.request.IssuerTimeZone != "" {
				t.Fatalf("request = %+v", assigner.request)
			}
			taken, err := assigner.request.IsUsed(t.Context(), "P20261102000001")
			if err != nil || !taken || used.tenant != tenant(t) || used.number != "P20261102000001" {
				t.Fatalf("used query: %v %v %+v", taken, err, used)
			}
		})
	}
}

// Covers: 适配器不判断——编排的错误原样上抛（编排答撞号之外的失败，提交编排据以回滚），编排交来集合外的答案报错，不落进任何一格。
func TestTheIssuerSurfacesFailuresAndUnknownOutcomes(t *testing.T) {
	failure := errors.New("sequence unavailable")
	issuer, err := docnumbering.NewIssuer(&assignerDouble{err: failure}, &usedDouble{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.IssueShipmentDocumentNumber(t.Context(), tenant(t)); !errors.Is(err, failure) {
		t.Fatalf("error = %v", err)
	}

	unknown, err := docnumbering.NewIssuer(&assignerDouble{assignment: docnumber.Assignment{Outcome: "SOMETHING_NEW"}}, &usedDouble{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unknown.IssueShipmentDocumentNumber(t.Context(), tenant(t)); !errors.Is(err, docnumbering.ErrUntranslatableAssignment) {
		t.Fatalf("unknown outcome: %v", err)
	}

	if _, err := docnumbering.NewIssuer(nil, &usedDouble{}); err == nil {
		t.Fatal("a missing assigner was accepted")
	}
}
