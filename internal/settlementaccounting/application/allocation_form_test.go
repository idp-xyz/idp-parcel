package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

type allocationFormViewDouble struct {
	form  domain.AllocationForm
	found bool
	err   error
}

func (double allocationFormViewDouble) LoadAllocationForm(
	context.Context,
	domain.TenantID,
	domain.AllocationRuleVersionReference,
) (domain.AllocationForm, bool, error) {
	return double.form, double.found, double.err
}

func TestApportionCostsAnswersUnconfiguredWhenTheFormIsNotRegistered(t *testing.T) {
	handler := newApportionHandler(t, allocationFormViewDouble{})
	result, err := handler.Apportion(t.Context(), apportionCommand())
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.AllocationFormUnconfigured {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if _, found := result.Apportionment(); found {
		t.Fatal("没登记仍交出了份额")
	}
}

func TestApportionCostsDoesNotSplitWhenTheRuleIsNotApplicable(t *testing.T) {
	handler := newApportionHandler(t, allocationFormViewDouble{form: domain.AllocationNotApplicable, found: true})
	result, err := handler.Apportion(t.Context(), apportionCommand())
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.AllocationFormNotApplicable {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if _, found := result.Apportionment(); found {
		t.Fatal("不适用仍交出了份额")
	}
}

func TestApportionCostsUsesTheRegisteredWeightForm(t *testing.T) {
	handler := newApportionHandler(t, allocationFormViewDouble{form: domain.AllocationByWeight, found: true})
	result, err := handler.Apportion(t.Context(), apportionCommand())
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.AllocationFormApportioned {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	apportionment, found := result.Apportionment()
	if !found || apportionment.Form() != domain.AllocationByWeight {
		t.Fatal("已登记按重却没有展开")
	}
	portions := apportionment.Portions()
	if len(portions) != 2 || portions[0].AmountMinor != 3 || portions[1].AmountMinor != 2 {
		t.Fatalf("份额 = %+v", portions)
	}
}

func TestApportionCostsStopsWhenTheFormViewFails(t *testing.T) {
	handler := newApportionHandler(t, allocationFormViewDouble{err: errors.New("form view down")})
	result, err := handler.Apportion(t.Context(), apportionCommand())
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.AllocationFormViewUnavailable {
		t.Fatalf("outcome = %s", result.Outcome())
	}
}

func newApportionHandler(t *testing.T, view ports.AllocationFormView) *application.ApportionCostsHandler {
	t.Helper()
	handler, err := application.NewApportionCostsHandler(view)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func apportionCommand() application.ApportionCostsCommand {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		panic(err)
	}
	return application.ApportionCostsCommand{
		TenantID:    tenant,
		RuleVersion: "rule-1/v1",
		SourceMinor: 5,
		Bases: []application.AllocationBasisInput{
			{Target: "parcel-b", Basis: 1},
			{Target: "parcel-a", Basis: 1},
		},
	}
}
