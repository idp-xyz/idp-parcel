package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidStatementCycle 说明对账周期的一格不成立。
var ErrInvalidStatementCycle = errors.New("settlement accounting: invalid statement cycle")

// StatementCycleKind 是对账周期按什么切。切法是产品给的几种（ADR-0339 决定三），某户用哪一种、哪一天截是租户取值。
type StatementCycleKind uint8

const (
	StatementCycleKindInvalid StatementCycleKind = iota
	// DailyCycle：每个归属日各成一期。
	DailyCycle
	// WeeklyCycle：每周在点名的那一个星期几截。
	WeeklyCycle
	// MonthlyCycle：每月在点名的那一天截；登 0 表示月末那一天。
	MonthlyCycle
)

func (kind StatementCycleKind) String() string {
	switch kind {
	case DailyCycle:
		return "DAILY"
	case WeeklyCycle:
		return "WEEKLY"
	case MonthlyCycle:
		return "MONTHLY"
	default:
		return ""
	}
}

// StatementCycleKindFromName 按字面取值认切法。
func StatementCycleKindFromName(name string) (StatementCycleKind, error) {
	for _, kind := range []StatementCycleKind{DailyCycle, WeeklyCycle, MonthlyCycle} {
		if kind.String() == name {
			return kind, nil
		}
	}
	return StatementCycleKindInvalid, fmt.Errorf("%w: kind %q", ErrInvalidStatementCycle, name)
}

// StatementCycle 是一个结算账户的对账周期：每一期的最后一个归属日落在哪一天。它是对账安排的一部分，登在对账安排之下的第二本册上
// ——业务时区与截单时刻那一本按版次追加、只作用于此后形成的归属日，周期一改要接上已截的各期，两者的变更规则不同，所以分册
// （ADR-0339 决定三）。
//
// Since 只定第一个截止日从哪天之后算起，不是费用的下限：起算日之后的第一期把此前没出账的已确认费用都收进来。
type StatementCycle struct {
	account    SettlementAccountID
	kind       StatementCycleKind
	weekday    time.Weekday
	monthDay   int
	since      AttributionDate
	hasWeekday bool
}

// NewStatementCycle 立一份周期。周截要点名星期几，月截要点名 0（月末）或 1–28：29 日以后不是每个月都有，登上去会有几个月不截。
func NewStatementCycle(
	account SettlementAccountID,
	kind StatementCycleKind,
	weekday *time.Weekday,
	monthDay *int,
	since AttributionDate,
) (StatementCycle, error) {
	if !account.valid() || !since.Valid() {
		return StatementCycle{}, fmt.Errorf("%w: account or since", ErrInvalidStatementCycle)
	}
	cycle := StatementCycle{account: account, kind: kind, since: since}
	switch kind {
	case DailyCycle:
		if weekday != nil || monthDay != nil {
			return StatementCycle{}, fmt.Errorf("%w: a daily cycle names no closing day", ErrInvalidStatementCycle)
		}
	case WeeklyCycle:
		if weekday == nil || monthDay != nil || *weekday < time.Sunday || *weekday > time.Saturday {
			return StatementCycle{}, fmt.Errorf("%w: a weekly cycle names one weekday", ErrInvalidStatementCycle)
		}
		cycle.weekday, cycle.hasWeekday = *weekday, true
	case MonthlyCycle:
		if monthDay == nil || weekday != nil || *monthDay < 0 || *monthDay > 28 {
			return StatementCycle{}, fmt.Errorf("%w: a monthly cycle names day 1-28 or 0 for the month end", ErrInvalidStatementCycle)
		}
		cycle.monthDay = *monthDay
	default:
		return StatementCycle{}, fmt.Errorf("%w: kind", ErrInvalidStatementCycle)
	}
	return cycle, nil
}

func (cycle StatementCycle) Account() SettlementAccountID { return cycle.account }

func (cycle StatementCycle) Kind() StatementCycleKind { return cycle.kind }

func (cycle StatementCycle) Since() AttributionDate { return cycle.since }

// Weekday 只在周截时有。
func (cycle StatementCycle) Weekday() (time.Weekday, bool) { return cycle.weekday, cycle.hasWeekday }

// MonthDay 只在月截时有意义，0 是月末。
func (cycle StatementCycle) MonthDay() int { return cycle.monthDay }

// SameCycle 判两份登记是不是同一份内容。
func (cycle StatementCycle) SameCycle(other StatementCycle) bool {
	return cycle == other
}

// ClosingsAfter 交回 after 之后（不含）、through 之前（含）的每一个截止日，按先后排。after 早于 Since 时从 Since 之后起。截止日就是
// 那一期的最后一个归属日。
func (cycle StatementCycle) ClosingsAfter(after, through AttributionDate) []AttributionDate {
	if !after.Valid() || after.civil().Before(cycle.since.civil()) {
		after = cycle.since
	}
	var closings []AttributionDate
	for day := after.civil().AddDate(0, 0, 1); !day.After(through.civil()); day = day.AddDate(0, 0, 1) {
		if cycle.closesOn(day) {
			year, month, date := day.Date()
			closings = append(closings, AttributionDate{Year: year, Month: month, Day: date})
		}
	}
	return closings
}

func (cycle StatementCycle) closesOn(day time.Time) bool {
	switch cycle.kind {
	case DailyCycle:
		return true
	case WeeklyCycle:
		return day.Weekday() == cycle.weekday
	case MonthlyCycle:
		if cycle.monthDay == 0 {
			return day.AddDate(0, 0, 1).Day() == 1
		}
		return day.Day() == cycle.monthDay
	default:
		return false
	}
}

// ClosingInstant 是截止日 closing 这一期能截的最早时刻：这一天在对账安排业务时区里的截单时刻。归属日在那一刻之后落到下一日
// （ADR-0168 决定一），这一期从此不会再长出新的一笔。
func (arrangement StatementArrangement) ClosingInstant(closing AttributionDate) (time.Time, error) {
	if arrangement.location == nil || !closing.Valid() {
		return time.Time{}, fmt.Errorf("%w: arrangement or closing", ErrInvalidChargeAttribution)
	}
	// 按墙钟取那一刻，与 dateOf 按墙钟比截单时刻同一口径：夏令时切换那一天零点加时长会差一小时。
	return time.Date(closing.Year, closing.Month, closing.Day,
		arrangement.cutoffMinute/60, arrangement.cutoffMinute%60, 0, 0, arrangement.location), nil
}

// LatestClosableDate 是此刻已经关上的最后一个归属日：此刻落成的归属日的前一天。
func (arrangement StatementArrangement) LatestClosableDate(now time.Time) (AttributionDate, error) {
	today, err := arrangement.dateOf(now)
	if err != nil {
		return AttributionDate{}, err
	}
	previous := today.civil().AddDate(0, 0, -1)
	year, month, day := previous.Date()
	return AttributionDate{Year: year, Month: month, Day: day}, nil
}
