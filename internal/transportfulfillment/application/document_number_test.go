package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// documentNumberIssuerDouble 是取号端口的替身，照编号组件的取号编排行事：按类型逐个出候选号，经调用方的「是否已用」判撞，
// 撞了取下一个，连撞 maxSkips 个答撞号。outcome 给了就直接答那一格；err 给了就交错误。
type documentNumberIssuerDouble struct {
	outcome  ports.DocumentNumberIssueOutcome
	err      error
	next     map[docnumber.TypeCode]int
	maxSkips int
	asked    []ports.DocumentNumberRequest
}

func newDocumentNumberIssuer() *documentNumberIssuerDouble {
	return &documentNumberIssuerDouble{next: map[docnumber.TypeCode]int{}, maxSkips: 20}
}

func (double *documentNumberIssuerDouble) IssueDocumentNumber(
	ctx context.Context,
	request ports.DocumentNumberRequest,
) (ports.DocumentNumberIssue, error) {
	double.asked = append(double.asked, request)
	if double.err != nil {
		return ports.DocumentNumberIssue{}, double.err
	}
	if double.outcome != ports.DocumentNumberIssueInvalid {
		return ports.DocumentNumberIssue{Outcome: double.outcome}, nil
	}
	for skipped := 0; skipped < double.maxSkips; skipped++ {
		double.next[request.Type]++
		number, err := domain.NewDocumentNumber(fmt.Sprintf("%s-%05d", request.Type, double.next[request.Type]))
		if err != nil {
			return ports.DocumentNumberIssue{}, err
		}
		used, err := request.IsUsed(ctx, number)
		if err != nil {
			return ports.DocumentNumberIssue{}, err
		}
		if !used {
			return ports.DocumentNumberIssue{Outcome: ports.DocumentNumberIssued, Number: number}, nil
		}
	}
	return ports.DocumentNumberIssue{Outcome: ports.DocumentNumberRuleCollision}, nil
}

// 委托与订舱两个仓储替身同时实现号表那一口：查号在已存记录上查，记号写回记录本身——重放读回的就是记下的那一格。

func (double *commissionStoreDouble) CommissionDocumentNumberUsed(
	_ context.Context,
	tenant domain.TenantID,
	number domain.DocumentNumber,
) (bool, error) {
	for _, record := range double.records {
		if stored, numbered := record.DocumentNumber.Number(); numbered && record.Key.TenantID == tenant && stored == number {
			return true, nil
		}
	}
	return false, nil
}

func (double *commissionStoreDouble) RecordCommissionDocumentNumber(
	_ context.Context,
	key ports.TransportCommissionKey,
	numbering domain.DocumentNumbering,
) error {
	record, found := double.records[commissionStoreKey(key)]
	if !found {
		return errors.New("commission not saved")
	}
	record.DocumentNumber = numbering
	double.records[commissionStoreKey(key)] = record
	return nil
}

func (double *bookingStoreDouble) BookingDocumentNumberUsed(
	_ context.Context,
	tenant domain.TenantID,
	number domain.DocumentNumber,
) (bool, error) {
	for _, record := range double.records {
		if stored, numbered := record.DocumentNumber.Number(); numbered && record.Key.TenantID == tenant && stored == number {
			return true, nil
		}
	}
	return false, nil
}

func (double *bookingStoreDouble) RecordBookingDocumentNumber(
	_ context.Context,
	key ports.BookingKey,
	numbering domain.DocumentNumbering,
) error {
	record, found := double.records[bookingStoreKey(key)]
	if !found {
		return errors.New("booking not saved")
	}
	record.DocumentNumber = numbering
	double.records[bookingStoreKey(key)] = record
	return nil
}

func numberOf(t *testing.T, numbering domain.DocumentNumbering) string {
	t.Helper()
	number, numbered := numbering.Number()
	if !numbered {
		absence, _ := numbering.Absence()
		t.Fatalf("没有单据号，原因 %s", absence)
	}
	return number.String()
}

// Covers: AT-TF-371（编排半边）——委托进入已提交时在同一编排里取 TRANSPORT_COMMISSION 的号并记到委托上；原样重放答原委托
// 连同原号，不再取号；取消不换号。
func TestASubmittedCommissionCarriesItsDocumentNumberAndAReplayKeepsIt(t *testing.T) {
	fixture := newCommissionFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.SubmitCommission(ctx, submitCommissionCommand(t))
	if err != nil || first.Outcome() != application.CommissionSubmitted {
		t.Fatalf("首交 = %q %v", first.Outcome(), err)
	}
	record, _ := first.Commission()
	if got := numberOf(t, record.DocumentNumber); got != "TRANSPORT_COMMISSION-00001" {
		t.Fatalf("委托单据号 = %q", got)
	}
	if len(fixture.numbers.asked) != 1 || fixture.numbers.asked[0].Type != domain.TransportCommissionDocumentNumberType.Code {
		t.Fatalf("取号请求 = %+v", fixture.numbers.asked)
	}
	stored := fixture.commissions.records["tenant-1|commission-1"]
	if numberOf(t, stored.DocumentNumber) != "TRANSPORT_COMMISSION-00001" {
		t.Fatal("号没有记到在册的委托上")
	}

	replay, err := fixture.handler.SubmitCommission(ctx, submitCommissionCommand(t))
	if err != nil || replay.Outcome() != application.CommissionExistingResult {
		t.Fatalf("重放 = %q %v", replay.Outcome(), err)
	}
	replayed, _ := replay.Commission()
	if numberOf(t, replayed.DocumentNumber) != "TRANSPORT_COMMISSION-00001" || len(fixture.numbers.asked) != 1 {
		t.Fatalf("重放应答原号且不再取号：%q，取号 %d 次", numberOf(t, replayed.DocumentNumber), len(fixture.numbers.asked))
	}

	cancel, err := fixture.handler.CancelCommission(ctx, application.CancelCommissionCommand{
		TenantID: record.Key.TenantID, Commission: "commission-1", CancelledAt: commissionNowAt,
	})
	if err != nil || cancel.Outcome() != application.CommissionCancelled {
		t.Fatalf("取消 = %q %v", cancel.Outcome(), err)
	}
	if numberOf(t, fixture.commissions.records["tenant-1|commission-1"].DocumentNumber) != "TRANSPORT_COMMISSION-00001" ||
		len(fixture.numbers.asked) != 1 {
		t.Fatal("取消换了号或又取了号")
	}
}

// Covers: AT-TF-372（编排半边）——订舱申请登记时取 BOOKING 的号；重放答原号、不再取号；号与订舱标识两回事。
func TestASubmittedBookingCarriesItsDocumentNumberAndAReplayKeepsIt(t *testing.T) {
	fixture := newCommissionFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.SubmitCommission(ctx, submitCommissionCommand(t)); err != nil {
		t.Fatal(err)
	}

	first, err := fixture.handler.SubmitBooking(ctx, submitBookingCommand(t))
	if err != nil || first.Outcome() != application.BookingSubmitted {
		t.Fatalf("首交 = %q %v", first.Outcome(), err)
	}
	record, _ := first.Booking()
	if numberOf(t, record.DocumentNumber) != "BOOKING-00001" || record.Key.Booking.String() != "booking-1" {
		t.Fatalf("订舱单据号 %q、订舱标识 %q", numberOf(t, record.DocumentNumber), record.Key.Booking)
	}
	last := fixture.numbers.asked[len(fixture.numbers.asked)-1]
	if last.Type != domain.BookingDocumentNumberType.Code {
		t.Fatalf("订舱取号类型 = %s", last.Type)
	}

	asked := len(fixture.numbers.asked)
	replay, err := fixture.handler.SubmitBooking(ctx, submitBookingCommand(t))
	if err != nil || replay.Outcome() != application.BookingExistingResult {
		t.Fatalf("重放 = %q %v", replay.Outcome(), err)
	}
	replayed, _ := replay.Booking()
	if numberOf(t, replayed.DocumentNumber) != "BOOKING-00001" || len(fixture.numbers.asked) != asked {
		t.Fatal("订舱重放应答原号且不再取号")
	}
}

// Covers: AT-TF-371、AT-TF-372（编排半边）——租户没登单据编号时区：委托与订舱照常开立，不带号，记无号原因「单据编号时区
// 未配置」。
func TestWithoutANumberingTimeZoneTheDocumentsAreIssuedWithoutANumberAndSayWhy(t *testing.T) {
	fixture := newCommissionFixture(t)
	fixture.numbers.outcome = ports.DocumentNumberTimeZoneNotConfigured
	ctx := context.Background()

	commission, err := fixture.handler.SubmitCommission(ctx, submitCommissionCommand(t))
	if err != nil || commission.Outcome() != application.CommissionSubmitted {
		t.Fatalf("委托 = %q %v", commission.Outcome(), err)
	}
	booking, err := fixture.handler.SubmitBooking(ctx, submitBookingCommand(t))
	if err != nil || booking.Outcome() != application.BookingSubmitted {
		t.Fatalf("订舱 = %q %v", booking.Outcome(), err)
	}
	commissionRecord, _ := commission.Commission()
	bookingRecord, _ := booking.Booking()
	for name, numbering := range map[string]domain.DocumentNumbering{
		"委托":   commissionRecord.DocumentNumber,
		"在册委托": fixture.commissions.records["tenant-1|commission-1"].DocumentNumber,
		"订舱":   bookingRecord.DocumentNumber,
		"在册订舱": fixture.bookings.records["tenant-1|booking-1"].DocumentNumber,
	} {
		if _, numbered := numbering.Number(); numbered {
			t.Fatalf("%s 不该有号", name)
		}
		if absence, _ := numbering.Absence(); absence != domain.DocumentNumberingTimeZoneNotConfigured {
			t.Fatalf("%s 无号原因 = %s", name, absence)
		}
	}
}

// Covers: AT-TF-373（编排半边）——已用的号跳过；跳到上限仍撞答编号规则撞号，指名撞的是哪一类的规则，不回显委托或订舱
// （装配点据此回滚整笔开立）。
func TestUsedNumbersAreSkippedAndAFullCollisionIsAnsweredAsANumberingRuleCollision(t *testing.T) {
	fixture := newCommissionFixture(t)
	ctx := context.Background()
	tenant, _ := domain.NewTenantID("tenant-1")
	used, _ := domain.NewDocumentNumber("TRANSPORT_COMMISSION-00001")
	fixture.commissions.records["tenant-1|older"] = ports.TransportCommissionRecord{
		Key:            ports.TransportCommissionKey{TenantID: tenant},
		DocumentNumber: domain.NumberedDocument(used),
	}

	skipped, err := fixture.handler.SubmitCommission(ctx, submitCommissionCommand(t))
	if err != nil || skipped.Outcome() != application.CommissionSubmitted {
		t.Fatalf("跳号 = %q %v", skipped.Outcome(), err)
	}
	record, _ := skipped.Commission()
	if numberOf(t, record.DocumentNumber) != "TRANSPORT_COMMISSION-00002" {
		t.Fatalf("跳过已用号之后应取下一个：%q", numberOf(t, record.DocumentNumber))
	}

	fixture.numbers.outcome = ports.DocumentNumberRuleCollision
	collided, err := fixture.handler.SubmitBooking(ctx, submitBookingCommand(t))
	if err != nil || collided.Outcome() != application.CommissionNumberingRuleCollision {
		t.Fatalf("撞号 = %q %v", collided.Outcome(), err)
	}
	if collided.Outcome().String() != "NUMBERING_RULE_COLLISION" {
		t.Fatalf("撞号码 = %q", collided.Outcome())
	}
	if rule, named := collided.NumberingRule(); !named || rule != domain.BookingDocumentNumberType.Code {
		t.Fatalf("撞号应指名订舱的编号规则：%q %v", rule, named)
	}
	if _, present := collided.Booking(); present {
		t.Fatal("撞号不回显订舱")
	}
}

// Covers: AT-TF-373（编排半边）——取号端口交错误：开立交错误（装配点回滚整笔），不当成没号开立。
func TestAnIssuerFailureFailsTheIssueInsteadOfIssuingWithoutANumber(t *testing.T) {
	fixture := newCommissionFixture(t)
	fixture.numbers.err = errors.New("sequence unavailable")
	result, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t))
	if err == nil {
		t.Fatalf("取号失败应交错误，得到 %q", result.Outcome())
	}
}
