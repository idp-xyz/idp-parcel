package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: UC-SA-002 主流程「确认同笔按费用项目的归属日形态与所属结算账户的对账安排形成结算归属日」
// （ADR-0269 决定三）：确认时刻 2026-08-13T11:00Z 在 Asia/Shanghai 是 19:00，过了 18:00 的截单时刻，归属日是
// 8 月 14 日；落库的那一份费用上就带着它，不是事后另补。
func TestConfirmationFormsTheAttributionDateInTheSameSave(t *testing.T) {
	fixture := newConfirmFixture(t)
	fixture.forms.register(t, "fee-freight", domain.ChargeAttributionChargeConfirmed)
	fixture.arrangements.register(t, "ACCOUNT/syn-1", "Asia/Shanghai", 18*60)

	result, err := fixture.handler.Handle(context.Background(), confirmCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.ChargeConfirmedOutcome {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	stored := fixture.store.charges["charge-1"]
	date, attributed := stored.AttributionDate()
	if !attributed || date.String() != "2026-08-14" {
		t.Fatalf("落库的归属日 = %s attributed=%v, want 2026-08-14", date, attributed)
	}
	if fixture.store.saves != 1 || fixture.store.records != 0 {
		t.Fatalf("saves = %d records = %d：归属日要与确认同笔落库", fixture.store.saves, fixture.store.records)
	}
}

// Covers: ADR-0269 决定三「确认时判不出不挡确认：确认条件与归属日是两件事，归属日留空」。
func TestAnUnjudgeableAttributionDoesNotHoldTheConfirmation(t *testing.T) {
	cases := map[string]func(*confirmFixture){
		"no arrangement": func(fixture *confirmFixture) {
			fixture.forms.register(t, "fee-freight", domain.ChargeAttributionChargeConfirmed)
		},
		"no form": func(fixture *confirmFixture) {
			fixture.arrangements.register(t, "ACCOUNT/syn-1", "UTC", 18*60)
		},
		// 形态要来源发生时点，这件费用形成时没带上：判不出，同样不挡确认。
		"no source instant": func(fixture *confirmFixture) {
			fixture.forms.register(t, "fee-freight", domain.ChargeAttributionSourceOccurred)
			fixture.arrangements.register(t, "ACCOUNT/syn-1", "UTC", 18*60)
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newConfirmFixture(t)
			arrange(fixture)
			result, err := fixture.handler.Handle(context.Background(), confirmCommand(t))
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome() != application.ChargeConfirmedOutcome {
				t.Fatalf("outcome = %s：判不出归属日挡住了确认", result.Outcome())
			}
			if _, attributed := fixture.store.charges["charge-1"].AttributionDate(); attributed {
				t.Fatal("判不出却落了归属日")
			}
		})
	}
}

// 读不出册时判得出判不出此刻不知道：留空会让截单改用那时在册的一版，所以答未决等重投，不落确认。
func TestAnUnreadableAttributionRegisterLeavesTheConfirmationUndecided(t *testing.T) {
	fixture := newConfirmFixture(t)
	fixture.forms.err = errors.New("forms down")
	result, err := fixture.handler.Handle(context.Background(), confirmCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.ConfirmUndecided || result.UndecidedReason() != application.ConfirmAttributionViewUnavailable {
		t.Fatalf("outcome = %s reason = %s", result.Outcome(), result.UndecidedReason())
	}
	if fixture.store.saves != 0 {
		t.Fatalf("saves = %d：未决时不落确认", fixture.store.saves)
	}
}
