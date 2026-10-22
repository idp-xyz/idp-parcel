package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidChargeAttribution 说明归属日形态、对账安排或所选时点不是这套判定能用的输入。
	ErrInvalidChargeAttribution = errors.New("settlement accounting: invalid charge attribution")
	// ErrInvalidStatementArrangement 说明对账安排的账户、业务时区或截单时刻不成形。
	ErrInvalidStatementArrangement = errors.New("settlement accounting: invalid statement arrangement")
)

// ChargeAttributionForm 是归属日的判定形态。两套都取一个已点名的时点，在对账安排的业务时区里落成公历日，
// 本地时刻达到截单时刻则归到下一日。包裹创建、收寄、签收不在这套词表里。
type ChargeAttributionForm uint8

const (
	ChargeAttributionFormInvalid ChargeAttributionForm = iota
	ChargeAttributionSourceOccurred
	ChargeAttributionChargeConfirmed
)

func (form ChargeAttributionForm) String() string {
	switch form {
	case ChargeAttributionSourceOccurred:
		return "SOURCE_OCCURRED"
	case ChargeAttributionChargeConfirmed:
		return "CHARGE_CONFIRMED"
	default:
		return ""
	}
}

func (form ChargeAttributionForm) judges() bool {
	return form == ChargeAttributionSourceOccurred || form == ChargeAttributionChargeConfirmed
}

// ChargeAttributionFormFromName 只认封闭两值。词表外拒，不夹成来源发生或费用确认。
func ChargeAttributionFormFromName(name string) (ChargeAttributionForm, error) {
	switch name {
	case "SOURCE_OCCURRED":
		return ChargeAttributionSourceOccurred, nil
	case "CHARGE_CONFIRMED":
		return ChargeAttributionChargeConfirmed, nil
	default:
		return ChargeAttributionFormInvalid, fmt.Errorf("%w: form", ErrInvalidChargeAttribution)
	}
}

// AttributionRegister 指名归属日判定读的两本册。判不出时答的是缺哪一本：两本的登记口不同，补登的人
// 要知道去登哪一本（ADR-0269 Consequences「缺哪一册」）。
type AttributionRegister uint8

const (
	AttributionRegisterInvalid AttributionRegister = iota
	StatementArrangementRegister
	ChargeAttributionFormRegister
)

func (register AttributionRegister) String() string {
	switch register {
	case StatementArrangementRegister:
		return "STATEMENT_ARRANGEMENT"
	case ChargeAttributionFormRegister:
		return "CHARGE_ATTRIBUTION_FORM"
	default:
		return ""
	}
}

// AttributionDate 是一个公历日。它不是某个时区的零点时刻。
type AttributionDate struct {
	Year  int
	Month time.Month
	Day   int
}

// ParseAttributionDate 只认 YYYY-MM-DD 且必须是真实存在的一天。不接带时刻或时区的写法：归属日是
// 已经在对账安排里落好的公历日，再带时区就成了第二次落日。
func ParseAttributionDate(value string) (AttributionDate, error) {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return AttributionDate{}, fmt.Errorf("%w: attribution date", ErrInvalidChargeAttribution)
	}
	year, month, day := parsed.Date()
	return AttributionDate{Year: year, Month: month, Day: day}, nil
}

func (date AttributionDate) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", date.Year, date.Month, date.Day)
}

// Valid 只认真实存在的一天：零值与 2 月 30 日这类写法都不是归属日。
func (date AttributionDate) Valid() bool {
	if date.Year < 1 || date.Month < time.January || date.Month > time.December || date.Day < 1 {
		return false
	}
	normalized := date.civil()
	return normalized.Day() == date.Day && normalized.Month() == date.Month
}

// After 按公历先后比。两边都取 UTC 的零点比，不涉任何业务时区。
func (date AttributionDate) After(other AttributionDate) bool {
	return date.civil().After(other.civil())
}

func (date AttributionDate) civil() time.Time {
	return time.Date(date.Year, date.Month, date.Day, 0, 0, 0, 0, time.UTC)
}

// ChargeAttributionRegistration 把一套判定形态挂到费用项目上。这一行只有形态：业务时区与截单时刻登在
// 结算账户的对账安排上，费用项目一册再存一份，同一账户下不同费用项目就会落进不同的日界（ADR-0269 决定二）。
type ChargeAttributionRegistration struct {
	feeItem FeeItemReference
	form    ChargeAttributionForm
}

func NewChargeAttributionRegistration(feeItem FeeItemReference, form ChargeAttributionForm) (ChargeAttributionRegistration, error) {
	if !feeItem.valid() || !form.judges() {
		return ChargeAttributionRegistration{}, fmt.Errorf("%w: charge attribution", ErrInvalidChargeAttribution)
	}
	return ChargeAttributionRegistration{feeItem: feeItem, form: form}, nil
}

func (registration ChargeAttributionRegistration) FeeItem() FeeItemReference {
	return registration.feeItem
}

func (registration ChargeAttributionRegistration) Form() ChargeAttributionForm {
	return registration.form
}

func (registration ChargeAttributionRegistration) SameRegistration(other ChargeAttributionRegistration) bool {
	return registration.feeItem == other.feeItem &&
		registration.form == other.form &&
		registration.form.judges()
}

// Judge 按已登记的形态选出时点，在这件费用所属结算账户的对账安排里落成归属日。所选时点为零、形态不是
// 这两套或对账安排不成形，拒。这里没有包裹创建、收寄或签收可以拿来顶上。
func (registration ChargeAttributionRegistration) Judge(
	arrangement StatementArrangement,
	sourceOccurred, chargeConfirmed time.Time,
) (AttributionDate, error) {
	if !registration.form.judges() {
		return AttributionDate{}, fmt.Errorf("%w: registration", ErrInvalidChargeAttribution)
	}
	instant := sourceOccurred
	if registration.form == ChargeAttributionChargeConfirmed {
		instant = chargeConfirmed
	}
	if instant.IsZero() {
		return AttributionDate{}, fmt.Errorf("%w: instant", ErrInvalidChargeAttribution)
	}
	return arrangement.dateOf(instant)
}

// StatementArrangement 是一个结算账户对账安排的一版：业务时区与截单时刻。截单时刻是当天的分钟数，0 到 1439；
// 0 是午夜这一份登记，不是没登记（ADR-0168 决定二的写法，ADR-0269 决定一把它搬到这里）。对账周期的日历
// 不在这一版里，本期边界由截单命令点名。产品不预填任何一格。
type StatementArrangement struct {
	account      SettlementAccountID
	timeZone     string
	cutoffMinute int
	location     *time.Location
}

func NewStatementArrangement(account SettlementAccountID, timeZone string, cutoffMinute int) (StatementArrangement, error) {
	if !account.valid() || timeZone == "" || cutoffMinute < 0 || cutoffMinute > 23*60+59 {
		return StatementArrangement{}, fmt.Errorf("%w: statement arrangement", ErrInvalidStatementArrangement)
	}
	// time.LoadLocation 把 "Local" 解成部署机器的时区；收下它，登记的就是服务器时区，恰是 CONTEXT 禁的顶上。
	if timeZone == "Local" {
		return StatementArrangement{}, fmt.Errorf("%w: timezone", ErrInvalidStatementArrangement)
	}
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return StatementArrangement{}, fmt.Errorf("%w: timezone", ErrInvalidStatementArrangement)
	}
	return StatementArrangement{
		account:      account,
		timeZone:     timeZone,
		cutoffMinute: cutoffMinute,
		location:     location,
	}, nil
}

func (arrangement StatementArrangement) Account() SettlementAccountID { return arrangement.account }

func (arrangement StatementArrangement) TimeZone() string { return arrangement.timeZone }

func (arrangement StatementArrangement) CutoffMinute() int { return arrangement.cutoffMinute }

// SameArrangement 判两版是否同内容。登记时与最新一版同内容是重放，不另起一版。
func (arrangement StatementArrangement) SameArrangement(other StatementArrangement) bool {
	return arrangement.account == other.account &&
		arrangement.timeZone == other.timeZone &&
		arrangement.cutoffMinute == other.cutoffMinute &&
		arrangement.location != nil
}

// dateOf 把一个时点在业务时区里落成公历日。本地时刻达到截单时刻，归属日是下一日（ADR-0168 决定一）。
func (arrangement StatementArrangement) dateOf(instant time.Time) (AttributionDate, error) {
	if arrangement.location == nil {
		return AttributionDate{}, fmt.Errorf("%w: arrangement", ErrInvalidChargeAttribution)
	}
	local := instant.In(arrangement.location)
	year, month, day := local.Date()
	sinceMidnight := time.Duration(local.Hour())*time.Hour +
		time.Duration(local.Minute())*time.Minute +
		time.Duration(local.Second())*time.Second +
		time.Duration(local.Nanosecond())
	if sinceMidnight >= time.Duration(arrangement.cutoffMinute)*time.Minute {
		next := time.Date(year, month, day, 0, 0, 0, 0, arrangement.location).AddDate(0, 0, 1)
		year, month, day = next.Date()
	}
	return AttributionDate{Year: year, Month: month, Day: day}, nil
}
