package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func cycleDate(t *testing.T, value string) domain.AttributionDate {
	t.Helper()
	date, err := domain.ParseAttributionDate(value)
	if err != nil {
		t.Fatal(err)
	}
	return date
}

func closingsOf(t *testing.T, cycle domain.StatementCycle, after, through string) []string {
	t.Helper()
	var got []string
	for _, closing := range cycle.ClosingsAfter(cycleDate(t, after), cycleDate(t, through)) {
		got = append(got, closing.String())
	}
	return got
}

// Covers: AT-SA-224（ADR-0339 决定三）——三种切法各自推出截止日：日截每天、周截点名的星期几、月截点名的那一天或月末；不早于登记的
// 起算日，不回扫之前。
func TestAStatementCycleNamesItsClosings(t *testing.T) {
	account, err := domain.NewSettlementAccountID("SYN-ACCT-1")
	if err != nil {
		t.Fatal(err)
	}
	since := cycleDate(t, "2026-10-01")
	friday, monthEnd, twentyFifth := time.Friday, 0, 25
	daily, err := domain.NewStatementCycle(account, domain.DailyCycle, nil, nil, since)
	if err != nil {
		t.Fatal(err)
	}
	weekly, err := domain.NewStatementCycle(account, domain.WeeklyCycle, &friday, nil, since)
	if err != nil {
		t.Fatal(err)
	}
	endOfMonth, err := domain.NewStatementCycle(account, domain.MonthlyCycle, nil, &monthEnd, since)
	if err != nil {
		t.Fatal(err)
	}
	onThe25th, err := domain.NewStatementCycle(account, domain.MonthlyCycle, nil, &twentyFifth, since)
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		cycle          domain.StatementCycle
		after, through string
		want           []string
	}{
		"日截":     {daily, "2026-10-27", "2026-10-29", []string{"2026-10-28", "2026-10-29"}},
		"日截不回扫":  {daily, "2026-09-01", "2026-10-02", []string{"2026-10-02"}},
		"周五截":    {weekly, "2026-10-01", "2026-10-20", []string{"2026-10-02", "2026-10-09", "2026-10-16"}},
		"月末截":    {endOfMonth, "2026-10-01", "2027-01-15", []string{"2026-10-31", "2026-11-30", "2026-12-31"}},
		"月末截跨二月": {endOfMonth, "2027-02-01", "2027-03-01", []string{"2027-02-28"}},
		"25 号截":  {onThe25th, "2026-10-25", "2026-12-24", []string{"2026-11-25"}},
	} {
		got := closingsOf(t, tc.cycle, tc.after, tc.through)
		if len(got) != len(tc.want) {
			t.Fatalf("%s = %v，want %v", name, got, tc.want)
		}
		for index := range got {
			if got[index] != tc.want[index] {
				t.Fatalf("%s = %v，want %v", name, got, tc.want)
			}
		}
	}

	twentyNinth, sunday := 29, time.Sunday
	for name, build := range map[string]func() error{
		"月截 29 号": func() error {
			_, err := domain.NewStatementCycle(account, domain.MonthlyCycle, nil, &twentyNinth, since)
			return err
		},
		"周截不点名": func() error {
			_, err := domain.NewStatementCycle(account, domain.WeeklyCycle, nil, nil, since)
			return err
		},
		"日截点了星期几": func() error {
			_, err := domain.NewStatementCycle(account, domain.DailyCycle, &sunday, nil, since)
			return err
		},
		"月截点了星期几": func() error {
			_, err := domain.NewStatementCycle(account, domain.MonthlyCycle, &sunday, &monthEnd, since)
			return err
		},
	} {
		if err := build(); !errors.Is(err, domain.ErrInvalidStatementCycle) {
			t.Fatalf("%s err = %v", name, err)
		}
	}
}

// Covers: AT-SA-224（ADR-0339 决定三）——截止日那一期在对账安排业务时区里那一天的截单时刻关上；此刻已关上的最后一个归属日是此刻落成的
// 归属日的前一天。
func TestAClosingClosesAtTheArrangementCutoff(t *testing.T) {
	account, err := domain.NewSettlementAccountID("SYN-ACCT-1")
	if err != nil {
		t.Fatal(err)
	}
	arrangement, err := domain.NewStatementArrangement(account, "Asia/Shanghai", 18*60)
	if err != nil {
		t.Fatal(err)
	}
	instant, err := arrangement.ClosingInstant(cycleDate(t, "2026-10-31"))
	if err != nil || !instant.Equal(time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("截单时刻 = %v err = %v，want 上海 18:00 即 10:00Z", instant, err)
	}
	for now, want := range map[time.Time]string{
		time.Date(2026, 10, 31, 9, 59, 0, 0, time.UTC): "2026-10-30",
		time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC): "2026-10-31",
	} {
		latest, err := arrangement.LatestClosableDate(now)
		if err != nil || latest.String() != want {
			t.Fatalf("%v 已关上的 = %s err = %v，want %s", now, latest, err, want)
		}
	}
}
