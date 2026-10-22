package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func TestAttributionDateUsesSourceOccurredBeforeCutoff(t *testing.T) {
	registration := attributionRegistration(t, domain.ChargeAttributionSourceOccurred)
	// 2026-09-30 17:59 UTC，截单 18:00，归属日仍是当天。
	date, err := registration.Judge(
		statementArrangement(t, "UTC", 18*60),
		time.Date(2026, 9, 30, 17, 59, 0, 0, time.UTC),
		time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if date.String() != "2026-09-30" {
		t.Fatalf("归属日 = %s", date)
	}
}

func TestAttributionDateRollsForwardAtCutoff(t *testing.T) {
	registration := attributionRegistration(t, domain.ChargeAttributionSourceOccurred)
	date, err := registration.Judge(
		statementArrangement(t, "UTC", 18*60),
		time.Date(2026, 9, 30, 18, 0, 0, 0, time.UTC),
		time.Time{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if date.String() != "2026-10-01" {
		t.Fatalf("归属日 = %s", date)
	}
}

func TestAttributionDateUsesChargeConfirmedWhenThatFormIsRegistered(t *testing.T) {
	registration := attributionRegistration(t, domain.ChargeAttributionChargeConfirmed)
	date, err := registration.Judge(
		statementArrangement(t, "UTC", 18*60),
		time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if date.String() != "2026-10-02" {
		t.Fatalf("归属日 = %s", date)
	}
}

// Covers: `AT-SA-062`「账户时区跨日 → 按账户时区和截单时刻形成范围，不按服务器时区猜测」的判定面：同一时点在
// UTC 还是 8 月 31 日，在对账安排的 Asia/Shanghai 里已是 9 月 1 日。
func TestAttributionDateFallsInTheArrangementTimeZone(t *testing.T) {
	registration := attributionRegistration(t, domain.ChargeAttributionSourceOccurred)
	instant := time.Date(2026, 8, 31, 17, 30, 0, 0, time.UTC)
	shanghai, err := registration.Judge(statementArrangement(t, "Asia/Shanghai", 23*60+59), instant, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if shanghai.String() != "2026-09-01" {
		t.Fatalf("Asia/Shanghai 归属日 = %s，want 2026-09-01", shanghai)
	}
	utc, err := registration.Judge(statementArrangement(t, "UTC", 23*60+59), instant, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if utc.String() != "2026-08-31" {
		t.Fatalf("UTC 归属日 = %s，want 2026-08-31", utc)
	}
}

func TestAttributionDateRejectsAMissingInstantAndAnUnknownForm(t *testing.T) {
	registration := attributionRegistration(t, domain.ChargeAttributionChargeConfirmed)
	if _, err := registration.Judge(statementArrangement(t, "UTC", 18*60), time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC), time.Time{}); !errors.Is(err, domain.ErrInvalidChargeAttribution) {
		t.Fatalf("缺确认时点 err = %v", err)
	}
	if _, err := domain.ChargeAttributionFormFromName("DELIVERED"); !errors.Is(err, domain.ErrInvalidChargeAttribution) {
		t.Fatalf("签收被收成归属日形态：%v", err)
	}
	if _, err := registration.Judge(domain.StatementArrangement{}, time.Time{}, time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)); !errors.Is(err, domain.ErrInvalidChargeAttribution) {
		t.Fatalf("零值对账安排被拿来落日：%v", err)
	}
}

// Covers: ADR-0269 决定一「没登记答未配置，不预填时区或截单时刻，不拿服务器时区顶上」的登记形：空时区、坏时区、
// 服务器时区的别名与越界的截单时刻都不成一版；0 是一份登记。
func TestAStatementArrangementRefusesAServerTimeZoneAndAnOutOfDayCutoff(t *testing.T) {
	account := settlementValue(t, domain.NewSettlementAccountID, "ACCOUNT/syn-1")
	for _, zone := range []string{"", "Local", "Not/AZone"} {
		if _, err := domain.NewStatementArrangement(account, zone, 18*60); !errors.Is(err, domain.ErrInvalidStatementArrangement) {
			t.Fatalf("时区 %q 收成了对账安排：%v", zone, err)
		}
	}
	for _, minute := range []int{-1, 24 * 60} {
		if _, err := domain.NewStatementArrangement(account, "UTC", minute); !errors.Is(err, domain.ErrInvalidStatementArrangement) {
			t.Fatalf("截单时刻 %d 收成了对账安排：%v", minute, err)
		}
	}
	if _, err := domain.NewStatementArrangement(account, "UTC", 0); err != nil {
		t.Fatalf("0 是午夜这一份登记：%v", err)
	}
}

func TestAnAttributionDateIsARealCalendarDay(t *testing.T) {
	for _, raw := range []string{"2026-02-30", "2026-8-31", "2026-08-31T00:00:00Z", ""} {
		if _, err := domain.ParseAttributionDate(raw); err == nil {
			t.Fatalf("%q 被收成归属日", raw)
		}
	}
	date, err := domain.ParseAttributionDate("2026-08-31")
	if err != nil || !date.Valid() || date.String() != "2026-08-31" {
		t.Fatalf("date = %s valid=%v err=%v", date, date.Valid(), err)
	}
	if (domain.AttributionDate{}).Valid() {
		t.Fatal("零值是一个归属日")
	}
}

func attributionRegistration(t *testing.T, form domain.ChargeAttributionForm) domain.ChargeAttributionRegistration {
	t.Helper()
	fee, err := domain.NewFeeItemReference("BASE")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.NewChargeAttributionRegistration(fee, form)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func statementArrangement(t *testing.T, zone string, cutoff int) domain.StatementArrangement {
	t.Helper()
	arrangement, err := domain.NewStatementArrangement(settlementValue(t, domain.NewSettlementAccountID, "ACCOUNT/syn-1"), zone, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	return arrangement
}
