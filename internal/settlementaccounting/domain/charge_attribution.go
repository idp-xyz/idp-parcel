package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidChargeAttribution 说明归属日形态、时区、截单时刻或所选时点不是这套判定能用的输入。
	ErrInvalidChargeAttribution = errors.New("settlement accounting: invalid charge attribution")
)

// ChargeAttributionForm 是归属日的判定形态。两套都取一个已点名的时点，在业务时区里落成公历日，
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

// AttributionDate 是一个公历日。它不是某个时区的零点时刻。
type AttributionDate struct {
	Year  int
	Month time.Month
	Day   int
}

func (date AttributionDate) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", date.Year, date.Month, date.Day)
}

// ChargeAttributionRegistration 把一套判定挂到费用项目上。时区与截单时刻是这一行的租户取值，
// 不在产品里预填。
type ChargeAttributionRegistration struct {
	feeItem      FeeItemReference
	form         ChargeAttributionForm
	timeZone     string
	cutoffMinute int
	location     *time.Location
}

func NewChargeAttributionRegistration(
	feeItem FeeItemReference,
	form ChargeAttributionForm,
	timeZone string,
	cutoffMinute int,
) (ChargeAttributionRegistration, error) {
	if !feeItem.valid() || !form.judges() || timeZone == "" || cutoffMinute < 0 || cutoffMinute > 23*60+59 {
		return ChargeAttributionRegistration{}, fmt.Errorf("%w: charge attribution", ErrInvalidChargeAttribution)
	}
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return ChargeAttributionRegistration{}, fmt.Errorf("%w: timezone", ErrInvalidChargeAttribution)
	}
	return ChargeAttributionRegistration{
		feeItem:      feeItem,
		form:         form,
		timeZone:     timeZone,
		cutoffMinute: cutoffMinute,
		location:     location,
	}, nil
}

func (registration ChargeAttributionRegistration) FeeItem() FeeItemReference {
	return registration.feeItem
}

func (registration ChargeAttributionRegistration) Form() ChargeAttributionForm {
	return registration.form
}

func (registration ChargeAttributionRegistration) TimeZone() string { return registration.timeZone }

func (registration ChargeAttributionRegistration) CutoffMinute() int {
	return registration.cutoffMinute
}

func (registration ChargeAttributionRegistration) SameRegistration(other ChargeAttributionRegistration) bool {
	return registration.feeItem == other.feeItem &&
		registration.form == other.form &&
		registration.timeZone == other.timeZone &&
		registration.cutoffMinute == other.cutoffMinute &&
		registration.form.judges()
}

// Judge 按已登记的形态把一个时点收成归属日。本地时刻达到截单时刻，归属日是下一日。
// 所选时点为零、或形态不是这两套，拒。这里没有包裹创建、收寄或签收可以拿来顶上。
func (registration ChargeAttributionRegistration) Judge(sourceOccurred, chargeConfirmed time.Time) (AttributionDate, error) {
	if registration.location == nil || !registration.form.judges() {
		return AttributionDate{}, fmt.Errorf("%w: registration", ErrInvalidChargeAttribution)
	}
	instant := sourceOccurred
	if registration.form == ChargeAttributionChargeConfirmed {
		instant = chargeConfirmed
	}
	if instant.IsZero() {
		return AttributionDate{}, fmt.Errorf("%w: instant", ErrInvalidChargeAttribution)
	}
	local := instant.In(registration.location)
	year, month, day := local.Date()
	sinceMidnight := time.Duration(local.Hour())*time.Hour +
		time.Duration(local.Minute())*time.Minute +
		time.Duration(local.Second())*time.Second +
		time.Duration(local.Nanosecond())
	if sinceMidnight >= time.Duration(registration.cutoffMinute)*time.Minute {
		next := time.Date(year, month, day, 0, 0, 0, 0, registration.location).AddDate(0, 0, 1)
		year, month, day = next.Date()
	}
	return AttributionDate{Year: year, Month: month, Day: day}, nil
}
