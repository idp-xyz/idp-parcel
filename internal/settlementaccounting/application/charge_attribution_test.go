package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// attributionFormsDouble 是费用项目一册：按费用项目登形态。
type attributionFormsDouble struct {
	forms map[string]domain.ChargeAttributionRegistration
	err   error
}

func (double *attributionFormsDouble) LoadChargeAttribution(
	_ context.Context,
	_ domain.TenantID,
	feeItem domain.FeeItemReference,
) (domain.ChargeAttributionRegistration, bool, error) {
	if double.err != nil {
		return domain.ChargeAttributionRegistration{}, false, double.err
	}
	registration, found := double.forms[feeItem.String()]
	return registration, found, nil
}

// arrangementsDouble 是对账安排登记册：按结算账户取最新一版。
type arrangementsDouble struct {
	arrangements map[string]domain.StatementArrangement
	err          error
}

func (double *arrangementsDouble) LoadStatementArrangement(
	_ context.Context,
	_ domain.TenantID,
	account domain.SettlementAccountID,
) (domain.StatementArrangement, bool, error) {
	if double.err != nil {
		return domain.StatementArrangement{}, false, double.err
	}
	arrangement, found := double.arrangements[account.String()]
	return arrangement, found, nil
}

func newAttributionFormsDouble() *attributionFormsDouble {
	return &attributionFormsDouble{forms: map[string]domain.ChargeAttributionRegistration{}}
}

func newArrangementsDouble() *arrangementsDouble {
	return &arrangementsDouble{arrangements: map[string]domain.StatementArrangement{}}
}

func (double *attributionFormsDouble) register(t *testing.T, feeItem string, form domain.ChargeAttributionForm) {
	t.Helper()
	registration, err := domain.NewChargeAttributionRegistration(billValue(t, domain.NewFeeItemReference, feeItem), form)
	if err != nil {
		t.Fatal(err)
	}
	double.forms[feeItem] = registration
}

func (double *arrangementsDouble) register(t *testing.T, account, zone string, cutoff int) {
	t.Helper()
	arrangement, err := domain.NewStatementArrangement(billValue(t, domain.NewSettlementAccountID, account), zone, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	double.arrangements[account] = arrangement
}

func newAttributionJudge(t *testing.T, forms *attributionFormsDouble, arrangements *arrangementsDouble) *application.JudgeChargeAttributionHandler {
	t.Helper()
	handler, err := application.NewJudgeChargeAttributionHandler(forms, arrangements)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// Covers: ADR-0269 决定二「形态或对账安排任一没有相符行，答未配置，并指名缺的是哪一册」：两本都读完再答，
// 两本都没登就两本都列。
func TestJudgeChargeAttributionNamesEveryMissingRegister(t *testing.T) {
	cases := []struct {
		name        string
		form        bool
		arrangement bool
		want        []domain.AttributionRegister
	}{
		{"neither", false, false, []domain.AttributionRegister{domain.StatementArrangementRegister, domain.ChargeAttributionFormRegister}},
		{"no arrangement", true, false, []domain.AttributionRegister{domain.StatementArrangementRegister}},
		{"no form", false, true, []domain.AttributionRegister{domain.ChargeAttributionFormRegister}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			forms, arrangements := newAttributionFormsDouble(), newArrangementsDouble()
			if testCase.form {
				forms.register(t, "BASE", domain.ChargeAttributionSourceOccurred)
			}
			if testCase.arrangement {
				arrangements.register(t, "ACCOUNT/syn-1", "UTC", 18*60)
			}
			result, err := newAttributionJudge(t, forms, arrangements).Judge(t.Context(), attributionCommand())
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome() != application.ChargeAttributionUnconfigured {
				t.Fatalf("outcome = %s", result.Outcome())
			}
			if _, judged := result.Date(); judged {
				t.Fatal("没登记仍交出了归属日")
			}
			if !reflect.DeepEqual(result.Missing(), testCase.want) {
				t.Fatalf("missing = %v, want %v", result.Missing(), testCase.want)
			}
		})
	}
}

func TestJudgeChargeAttributionUsesTheRegisteredFormInTheAccountsArrangement(t *testing.T) {
	forms, arrangements := newAttributionFormsDouble(), newArrangementsDouble()
	forms.register(t, "BASE", domain.ChargeAttributionSourceOccurred)
	arrangements.register(t, "ACCOUNT/syn-1", "UTC", 18*60)
	// 另一户的对账安排换了时区，与这件费用无关。
	arrangements.register(t, "ACCOUNT/syn-2", "Asia/Shanghai", 18*60)
	result, err := newAttributionJudge(t, forms, arrangements).Judge(t.Context(), attributionCommand())
	if err != nil {
		t.Fatal(err)
	}
	date, judged := result.Date()
	if result.Outcome() != application.ChargeAttributionJudged || !judged || date.String() != "2026-09-30" || len(result.Missing()) != 0 {
		t.Fatalf("outcome = %s date = %s judged=%v missing=%v", result.Outcome(), date, judged, result.Missing())
	}
}

func TestJudgeChargeAttributionStopsWhenAViewFails(t *testing.T) {
	for _, broken := range []string{"forms", "arrangements"} {
		t.Run(broken, func(t *testing.T) {
			forms, arrangements := newAttributionFormsDouble(), newArrangementsDouble()
			forms.register(t, "BASE", domain.ChargeAttributionSourceOccurred)
			arrangements.register(t, "ACCOUNT/syn-1", "UTC", 18*60)
			if broken == "forms" {
				forms.err = errors.New("attribution view down")
			} else {
				arrangements.err = errors.New("arrangement view down")
			}
			result, err := newAttributionJudge(t, forms, arrangements).Judge(t.Context(), attributionCommand())
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome() != application.ChargeAttributionViewUnavailable {
				t.Fatalf("outcome = %s", result.Outcome())
			}
		})
	}
}

func attributionCommand() application.JudgeChargeAttributionCommand {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		panic(err)
	}
	return application.JudgeChargeAttributionCommand{
		TenantID:       tenant,
		FeeItem:        "BASE",
		Account:        "ACCOUNT/syn-1",
		SourceOccurred: time.Date(2026, 9, 30, 17, 59, 0, 0, time.UTC),
	}
}
