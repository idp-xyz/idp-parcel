package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件钉 ADR-0303 复核记录（票 demo-chain-gaps/180）在其余六个运营决定口与派送补拍、按册重问上的编排半边：信封里的提交操作者
// 随决定落在决定记录上；同一个决定换一位操作者重放，答复与首次重放同格、在册的仍是先到的那一位；命令不带操作者照样形成——
// 它只进审计，不进重放比对，也不进任何门。夹具全为合成引用（S 级）。

func tenantOne(t *testing.T) domain.TenantID {
	t.Helper()
	return value(t, domain.NewTenantID, "tenant-1")
}

func (fixture *commissionFixture) storedBooking(t *testing.T, booking string) ports.BookingRecord {
	t.Helper()
	key := ports.BookingKey{TenantID: tenantOne(t), Booking: value(t, domain.NewBookingReference, booking)}
	record, found := fixture.bookings.records[bookingStoreKey(key)]
	if !found {
		t.Fatalf("订舱册上没有 %s", booking)
	}
	return record
}

func (fixture *commissionFixture) storedCommissionRecord(t *testing.T, commission string) ports.TransportCommissionRecord {
	t.Helper()
	key := ports.TransportCommissionKey{TenantID: tenantOne(t), Commission: value(t, domain.NewTransportCommissionReference, commission)}
	record, found := fixture.commissions.records[commissionStoreKey(key)]
	if !found {
		t.Fatalf("委托册上没有 %s", commission)
	}
	return record
}

// Covers: `AT-TF-348` 编排半边——订舱提交记下提交人；另一位操作者原样再交答已有订舱、交回原订舱，提交人不被顶替，异内容照旧答
// 冲突；不带操作者的订舱照样形成、不记人；带 NUL 的操作者存不进登记册，在构造期答未受理、不动库。
func TestABookingSubmissionRecordsItsOperatorAndAReplayByAnotherKeepsIt(t *testing.T) {
	fixture := newCommissionFixture(t)
	if result, err := fixture.handler.SubmitCommission(t.Context(), submitCommissionCommand(t)); err != nil || result.Outcome() != application.CommissionSubmitted {
		t.Fatalf("submit commission: %q %v", result.Outcome(), err)
	}

	command := submitBookingCommand(t)
	command.SubmittedBy = firstOperator
	if result, err := fixture.handler.SubmitBooking(t.Context(), command); err != nil || result.Outcome() != application.BookingSubmitted {
		t.Fatalf("订舱：%q %v", result.Outcome(), err)
	}
	replay := command
	replay.SubmittedBy = secondOperator
	if result, err := fixture.handler.SubmitBooking(t.Context(), replay); err != nil || result.Outcome() != application.BookingExistingResult {
		t.Fatalf("另一位操作者原样再交：%q %v，want EXISTING_BOOKING", result.Outcome(), err)
	}
	conflicting := replay
	conflicting.Quantity++
	if result, err := fixture.handler.SubmitBooking(t.Context(), conflicting); err != nil || result.Outcome() != application.BookingConflict {
		t.Fatalf("另一位操作者异内容再交：%q %v，want BOOKING_CONFLICT", result.Outcome(), err)
	}
	if by := fixture.storedBooking(t, "booking-1").SubmittedBy; by.String() != firstOperator {
		t.Fatalf("在册提交人 = %q，want 先到的那一位 %q", by.String(), firstOperator)
	}

	plain := submitBookingCommand(t)
	plain.Booking = "booking-2"
	if result, err := fixture.handler.SubmitBooking(t.Context(), plain); err != nil || result.Outcome() != application.BookingSubmitted {
		t.Fatalf("不带操作者订舱：%q %v", result.Outcome(), err)
	}
	if by := fixture.storedBooking(t, "booking-2").SubmittedBy; by.Present() {
		t.Fatalf("不带操作者不该记人：%q", by.String())
	}

	unstorable := submitBookingCommand(t)
	unstorable.Booking = "booking-3"
	unstorable.SubmittedBy = "issuer#sub\x00"
	if result, err := fixture.handler.SubmitBooking(t.Context(), unstorable); err != nil || result.Outcome() != application.CommissionNotAccepted {
		t.Fatalf("带 NUL 的操作者：%q %v，want INPUT_NOT_ACCEPTED", result.Outcome(), err)
	}
	if _, stored := fixture.bookings.records[bookingStoreKey(ports.BookingKey{
		TenantID: tenantOne(t), Booking: value(t, domain.NewBookingReference, "booking-3"),
	})]; stored {
		t.Fatal("未受理的订舱不得落库")
	}
}

// Covers: `AT-TF-349` 编排半边——取消订舱记下取消人，与提交人各记各的；另一位操作者再取消答原取消，取消人不被顶替；不带操作者
// 的取消照样取消、不记人；带 NUL 的操作者答未受理、订舱仍未取消。
func TestABookingCancellationRecordsItsOperatorAndAReplayByAnotherKeepsIt(t *testing.T) {
	fixture := newBookedFixture(t)
	other := submitBookingCommand(t)
	other.Booking = "booking-2"
	if result, err := fixture.handler.SubmitBooking(t.Context(), other); err != nil || result.Outcome() != application.BookingSubmitted {
		t.Fatalf("second booking: %q %v", result.Outcome(), err)
	}

	unstorable := application.CancelBookingCommand{
		TenantID: tenantOne(t), Booking: "booking-1", CancelledAt: commissionNowAt, CancelledBy: "issuer#sub\x00",
	}
	if result, err := fixture.handler.CancelBooking(t.Context(), unstorable); err != nil || result.Outcome() != application.CommissionNotAccepted {
		t.Fatalf("带 NUL 的操作者：%q %v，want INPUT_NOT_ACCEPTED", result.Outcome(), err)
	}
	if _, cancelled := fixture.storedBooking(t, "booking-1").Booking.Cancelled(); cancelled {
		t.Fatal("未受理的取消不得落库")
	}

	command := application.CancelBookingCommand{TenantID: tenantOne(t), Booking: "booking-1", CancelledAt: commissionNowAt, CancelledBy: firstOperator}
	if result, err := fixture.handler.CancelBooking(t.Context(), command); err != nil || result.Outcome() != application.BookingCancelled {
		t.Fatalf("取消订舱：%q %v", result.Outcome(), err)
	}
	replay := command
	replay.CancelledBy = secondOperator
	replay.CancelledAt = commissionNowAt.Add(time.Hour)
	if result, err := fixture.handler.CancelBooking(t.Context(), replay); err != nil || result.Outcome() != application.CancellationExistingResult {
		t.Fatalf("另一位操作者再取消：%q %v，want EXISTING_CANCELLATION", result.Outcome(), err)
	}
	stored := fixture.storedBooking(t, "booking-1")
	if stored.CancelledBy.String() != firstOperator || stored.SubmittedBy.Present() {
		t.Fatalf("在册取消人 = %q、提交人 = %q，want 先取消的那一位且提交人各记各的（这份订舱没带提交人）", stored.CancelledBy.String(), stored.SubmittedBy.String())
	}

	plain := application.CancelBookingCommand{TenantID: tenantOne(t), Booking: "booking-2", CancelledAt: commissionNowAt}
	if result, err := fixture.handler.CancelBooking(t.Context(), plain); err != nil || result.Outcome() != application.BookingCancelled {
		t.Fatalf("不带操作者取消：%q %v", result.Outcome(), err)
	}
	if by := fixture.storedBooking(t, "booking-2").CancelledBy; by.Present() {
		t.Fatalf("不带操作者不该记人：%q", by.String())
	}
}

// Covers: `AT-TF-350` 编排半边——取消运输委托记下取消人，与提交人各记各的；另一位操作者再取消答原取消，取消人不被顶替；不带
// 操作者的取消照样取消；委托开始（段采用）不动取消人；带 NUL 的操作者答未受理、委托仍未取消。
func TestACommissionCancellationRecordsItsOperatorAndAReplayByAnotherKeepsIt(t *testing.T) {
	fixture := newCommissionFixture(t)
	first := submitCommissionCommand(t)
	first.SubmittedBy = secondOperator
	second := submitCommissionCommand(t)
	second.Commission = "commission-2"
	for _, command := range []application.SubmitCommissionCommand{first, second} {
		if result, err := fixture.handler.SubmitCommission(t.Context(), command); err != nil || result.Outcome() != application.CommissionSubmitted {
			t.Fatalf("submit %s: %q %v", command.Commission, result.Outcome(), err)
		}
	}

	unstorable := application.CancelCommissionCommand{
		TenantID: tenantOne(t), Commission: "commission-1", CancelledAt: commissionNowAt, CancelledBy: "issuer#sub\x00",
	}
	if result, err := fixture.handler.CancelCommission(t.Context(), unstorable); err != nil || result.Outcome() != application.CommissionNotAccepted {
		t.Fatalf("带 NUL 的操作者：%q %v，want INPUT_NOT_ACCEPTED", result.Outcome(), err)
	}
	if _, cancelled := fixture.storedCommissionRecord(t, "commission-1").Commission.Cancelled(); cancelled {
		t.Fatal("未受理的取消不得落库")
	}

	command := application.CancelCommissionCommand{TenantID: tenantOne(t), Commission: "commission-1", CancelledAt: commissionNowAt, CancelledBy: firstOperator}
	if result, err := fixture.handler.CancelCommission(t.Context(), command); err != nil || result.Outcome() != application.CommissionCancelled {
		t.Fatalf("取消委托：%q %v", result.Outcome(), err)
	}
	replay := command
	replay.CancelledBy = secondOperator
	if result, err := fixture.handler.CancelCommission(t.Context(), replay); err != nil || result.Outcome() != application.CancellationExistingResult {
		t.Fatalf("另一位操作者再取消：%q %v，want EXISTING_CANCELLATION", result.Outcome(), err)
	}
	stored := fixture.storedCommissionRecord(t, "commission-1")
	if stored.CancelledBy.String() != firstOperator || stored.SubmittedBy.String() != secondOperator {
		t.Fatalf("取消人 = %q、提交人 = %q，want 先取消的那一位与各记各的提交人", stored.CancelledBy.String(), stored.SubmittedBy.String())
	}

	plain := application.CancelCommissionCommand{TenantID: tenantOne(t), Commission: "commission-2", CancelledAt: commissionNowAt}
	if result, err := fixture.handler.CancelCommission(t.Context(), plain); err != nil || result.Outcome() != application.CommissionCancelled {
		t.Fatalf("不带操作者取消：%q %v", result.Outcome(), err)
	}
	if by := fixture.storedCommissionRecord(t, "commission-2").CancelledBy; by.Present() {
		t.Fatalf("不带操作者不该记人：%q", by.String())
	}
}

// Covers: `AT-TF-351` 编排半边——履约段采用运输委托记下采用人；另一位操作者原样再交答已有采用、交回原连线，采用人不被顶替，
// 同段换委托照旧答冲突；不带操作者的采用照样成立、不记人，委托的开始不记人；带 NUL 的操作者答未受理、零写入。
func TestAnAdoptionRecordsItsOperatorAndAReplayByAnotherKeepsIt(t *testing.T) {
	fixture := newCommissionAdoptionFixture(t)

	unstorable := adoptCommand("SEG-1", "commission-1", "partner-1")
	unstorable.AdoptedBy = "issuer#sub\x00"
	if result, err := fixture.adopter.Adopt(t.Context(), unstorable); err != nil || result.Outcome() != application.CommissionAdoptionNotAccepted {
		t.Fatalf("带 NUL 的操作者：%q %v，want INPUT_NOT_ACCEPTED", result.Outcome(), err)
	}
	if fixture.adoptions.saves != 0 {
		t.Fatal("未受理的采用不得落库")
	}

	command := adoptCommand("SEG-1", "commission-1", "partner-1")
	command.AdoptedBy = firstOperator
	if result, err := fixture.adopter.Adopt(t.Context(), command); err != nil || result.Outcome() != application.CommissionAdopted {
		t.Fatalf("采用：%q %v", result.Outcome(), err)
	}
	replay := command
	replay.AdoptedBy = secondOperator
	replayed, err := fixture.adopter.Adopt(t.Context(), replay)
	if err != nil || replayed.Outcome() != application.CommissionAdoptionExisting {
		t.Fatalf("另一位操作者原样再交：%q %v，want EXISTING_ADOPTION", replayed.Outcome(), err)
	}
	if adoption, _ := replayed.Adoption(); adoption.AdoptedBy.String() != firstOperator {
		t.Fatalf("交回的连线采用人 = %q，want 先到的那一位", adoption.AdoptedBy.String())
	}
	conflicting := replay
	conflicting.Commission = "commission-2"
	if result, err := fixture.adopter.Adopt(t.Context(), conflicting); err != nil || result.Outcome() != application.CommissionAdoptionConflict {
		t.Fatalf("另一位操作者同段换委托：%q %v，want ADOPTION_CONFLICT", result.Outcome(), err)
	}
	if by := fixture.adoptions.records["tenant-1|SEG-1"].AdoptedBy; by.String() != firstOperator {
		t.Fatalf("在册采用人 = %q，want 先到的那一位 %q", by.String(), firstOperator)
	}

	plain, err := fixture.adopter.Adopt(t.Context(), adoptCommand("SEG-2", "commission-1", "partner-1"))
	if err != nil || plain.Outcome() != application.CommissionAdopted {
		t.Fatalf("不带操作者采用：%q %v", plain.Outcome(), err)
	}
	if by := fixture.adoptions.records["tenant-1|SEG-2"].AdoptedBy; by.Present() {
		t.Fatalf("不带操作者不该记人：%q", by.String())
	}
	if stored := fixture.storedCommission(t); stored.CancelledBy.Present() || stored.SubmittedBy.Present() {
		t.Fatalf("采用带出的开始不是一位操作者的决定，不该给委托记人：%+v", stored)
	}
}

// Covers: `AT-TF-352` 编排半边——容量预占记下预占人；另一位操作者原样重交答已有预占，预占人不被顶替，异内容照旧答冲突；之后
// 别的预占与释放换存池，先到的预占人原样带着；不带操作者的预占照样形成、不记人；带 NUL 的操作者答未受理、池不动。
func TestAReservationRecordsItsOperatorAndLaterReplacementsKeepIt(t *testing.T) {
	fixture := newOpportunityFixture(t)
	establishedPool(t, fixture)
	pool := func() ports.CapacityPoolRecord {
		return fixture.pools.records[poolKey(ports.CapacityPoolKey{TenantID: opportunityTenant(t), Pool: value(t, domain.NewCapacityPoolReference, fixturePool)})]
	}

	unstorable := reserveCommand(t, 10)
	unstorable.ReservedBy = "issuer#sub\x00"
	if result, err := fixture.handler.Reserve(t.Context(), unstorable); err != nil || result.Outcome() != application.OpportunityNotAccepted {
		t.Fatalf("带 NUL 的操作者：%q %v，want INPUT_NOT_ACCEPTED", result.Outcome(), err)
	}
	if fixture.pools.replaces != 0 {
		t.Fatal("未受理的预占不得落库")
	}

	command := reserveCommand(t, 60)
	command.ReservedBy = firstOperator
	if result, err := fixture.handler.Reserve(t.Context(), command); err != nil || result.Outcome() != application.CapacityReserved {
		t.Fatalf("预占：%q %v", result.Outcome(), err)
	}
	replay := command
	replay.ReservedBy = secondOperator
	if result, err := fixture.handler.Reserve(t.Context(), replay); err != nil || result.Outcome() != application.ReservationExistingResult {
		t.Fatalf("另一位操作者原样重交：%q %v，want EXISTING_RESERVATION", result.Outcome(), err)
	}
	conflicting := replay
	conflicting.Quantity = 59
	if result, err := fixture.handler.Reserve(t.Context(), conflicting); err != nil || result.Outcome() != application.ReservationConflict {
		t.Fatalf("另一位操作者异内容重交：%q %v，want RESERVATION_CONFLICT", result.Outcome(), err)
	}
	reservation := value(t, domain.NewCapacityReservationReference, "reservation-1")
	if by := pool().ReservedBy[reservation]; by.String() != firstOperator {
		t.Fatalf("在册预占人 = %q，want 先到的那一位 %q", by.String(), firstOperator)
	}

	plain := reserveCommand(t, 20)
	plain.Reservation = "reservation-2"
	if result, err := fixture.handler.Reserve(t.Context(), plain); err != nil || result.Outcome() != application.CapacityReserved {
		t.Fatalf("不带操作者预占：%q %v", result.Outcome(), err)
	}
	record := pool()
	if by, recorded := record.ReservedBy[value(t, domain.NewCapacityReservationReference, "reservation-2")]; recorded || by.Present() {
		t.Fatalf("不带操作者不该记人：%q", by.String())
	}
	if by := record.ReservedBy[reservation]; by.String() != firstOperator {
		t.Fatalf("第二份预占换存池之后，第一份的预占人 = %q，want 原样带着", by.String())
	}
}

// Covers: `AT-TF-353` 编排半边——容量释放记下释放人，与预占人各记各的；另一位操作者原样重交答已有释放，释放人不被顶替、不再还
// 一份；不带操作者的释放照样形成、不记人；带 NUL 的操作者答未受理、池不动。
func TestAReleaseRecordsItsOperatorAndAReplayByAnotherKeepsIt(t *testing.T) {
	fixture := newOpportunityFixture(t)
	establishedPool(t, fixture)
	reserve := reserveCommand(t, 60)
	reserve.ReservedBy = secondOperator
	if result, err := fixture.handler.Reserve(t.Context(), reserve); err != nil || result.Outcome() != application.CapacityReserved {
		t.Fatalf("预占：%q %v", result.Outcome(), err)
	}
	pool := func() ports.CapacityPoolRecord {
		return fixture.pools.records[poolKey(ports.CapacityPoolKey{TenantID: opportunityTenant(t), Pool: value(t, domain.NewCapacityPoolReference, fixturePool)})]
	}
	replaces := fixture.pools.replaces

	unstorable := releaseCommand(t, "release-1", 10, reserveAt.Add(time.Hour))
	unstorable.ReleasedBy = "issuer#sub\x00"
	if result, err := fixture.handler.Release(t.Context(), unstorable); err != nil || result.Outcome() != application.OpportunityNotAccepted {
		t.Fatalf("带 NUL 的操作者：%q %v，want INPUT_NOT_ACCEPTED", result.Outcome(), err)
	}
	if fixture.pools.replaces != replaces {
		t.Fatal("未受理的释放不得落库")
	}

	command := releaseCommand(t, "release-1", 10, reserveAt.Add(time.Hour))
	command.ReleasedBy = firstOperator
	if result, err := fixture.handler.Release(t.Context(), command); err != nil || result.Outcome() != application.CapacityReleased {
		t.Fatalf("释放：%q %v", result.Outcome(), err)
	}
	replay := command
	replay.ReleasedBy = secondOperator
	if result, err := fixture.handler.Release(t.Context(), replay); err != nil || result.Outcome() != application.ReleaseExistingResult {
		t.Fatalf("另一位操作者原样重交：%q %v，want EXISTING_RELEASE", result.Outcome(), err)
	}
	first := value(t, domain.NewCapacityReleaseReference, "release-1")
	if by := pool().ReleasedBy[first]; by.String() != firstOperator {
		t.Fatalf("在册释放人 = %q，want 先到的那一位 %q", by.String(), firstOperator)
	}
	if by := pool().ReservedBy[value(t, domain.NewCapacityReservationReference, "reservation-1")]; by.String() != secondOperator {
		t.Fatalf("释放换存池之后预占人 = %q，want 原样带着", by.String())
	}

	if result, err := fixture.handler.Release(t.Context(), releaseCommand(t, "release-2", 5, reserveAt.Add(time.Hour))); err != nil || result.Outcome() != application.CapacityReleased {
		t.Fatalf("不带操作者释放：%q %v", result.Outcome(), err)
	}
	if by, recorded := pool().ReleasedBy[value(t, domain.NewCapacityReleaseReference, "release-2")]; recorded || by.Present() {
		t.Fatalf("不带操作者不该记人：%q", by.String())
	}
}

// askLedgerDouble 是问询册的内存替身：只增，err 非空时写口答它，扮演册写不动。
type askLedgerDouble struct {
	records []ports.DeliveryDispatchAskRecord
	err     error
}

func (double *askLedgerDouble) RecordAsk(_ context.Context, record ports.DeliveryDispatchAskRecord) error {
	if double.err != nil {
		return double.err
	}
	double.records = append(double.records, record)
	return nil
}

func (double *askLedgerDouble) ListAsks(context.Context, domain.TenantID, int) ([]ports.DeliveryDispatchAskRecord, error) {
	return append([]ports.DeliveryDispatchAskRecord(nil), double.records...), nil
}

// Covers: `AT-TF-354` 编排半边——补拍带着操作者时在问询册记下一次按下（谁、什么时候、问的哪个段与对象）；另一位操作者对同一件再补拍，
// 答复与第一次相同、各记一次；执行器形成的任务不记建任务人；不带操作者的补拍（派送发起执行器随交接信封发起的那一路）不进册；
// 带 NUL 的操作者答不受理、不进册；操作者在场而册没接，或册写不下，这一拍不答。
func TestADeliveryDispatchTriggerRecordsWhoAskedAndNeverTheTask(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	asks := &askLedgerDouble{}
	fixture.deps.Asks = asks

	command := triggerCommand(t, "parcel-1")
	command.TriggeredBy = firstOperator
	formed, err := fixture.handler().Trigger(t.Context(), command)
	if err != nil || formed.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("补拍：%s %v", formed.Outcome(), err)
	}
	replay := command
	replay.TriggeredBy = secondOperator
	again, err := fixture.handler().Trigger(t.Context(), replay)
	if err != nil || again.Outcome() != application.DeliveryDispatchTaskAlreadyFormed {
		t.Fatalf("另一位操作者再补拍：%s %v，want DISPATCH_TASK_ALREADY_FORMED", again.Outcome(), err)
	}
	if len(asks.records) != 2 || asks.records[0].AskedBy.String() != firstOperator || asks.records[1].AskedBy.String() != secondOperator ||
		asks.records[0].Kind != ports.DeliveryDispatchAskTrigger || asks.records[0].Segment.String() != "segment-1" ||
		asks.records[0].Object.String() != "parcel-1" || !asks.records[0].AskedAt.Equal(deliveryTriggeredAt) {
		t.Fatalf("问询册 = %+v，want 两次按下、各记各的人，带段与对象与业务时间", asks.records)
	}
	for _, task := range fixture.tasks.records {
		if task.OpenedBy.Present() {
			t.Fatalf("执行器形成的任务不记建任务人：%q", task.OpenedBy.String())
		}
	}

	plain := triggerCommand(t, "parcel-1")
	if result, err := fixture.handler().Trigger(t.Context(), plain); err != nil || result.Outcome() != application.DeliveryDispatchTaskAlreadyFormed {
		t.Fatalf("不带操作者补拍：%s %v", result.Outcome(), err)
	}
	unstorable := triggerCommand(t, "parcel-1")
	unstorable.TriggeredBy = "issuer#sub\x00"
	if result, err := fixture.handler().Trigger(t.Context(), unstorable); err != nil || result.Outcome() != application.DeliveryDispatchNotAccepted {
		t.Fatalf("带 NUL 的操作者：%s %v，want INPUT_NOT_ACCEPTED", result.Outcome(), err)
	}
	if len(asks.records) != 2 {
		t.Fatalf("不带操作者与未受理的补拍不该进册：%d 行", len(asks.records))
	}

	asks.err = errors.New("ledger down")
	if _, err := fixture.handler().Trigger(t.Context(), command); !errors.Is(err, application.ErrDeliveryDispatchAskUnrecorded) {
		t.Fatalf("册写不下：err = %v，want ErrDeliveryDispatchAskUnrecorded", err)
	}
	fixture.deps.Asks = nil
	if _, err := fixture.handler().Trigger(t.Context(), command); !errors.Is(err, application.ErrDeliveryDispatchAskLedgerNotWired) {
		t.Fatalf("册没接：err = %v，want ErrDeliveryDispatchAskLedgerNotWired", err)
	}
}

// Covers: `AT-TF-355` 编排半边——按册重问带着操作者时先于逐件去问记一次按下（一次一行，带问法与件数上限，不是逐件一行）；册上有
// 待形成的那几件也不各记一行，逐件交执行器的命令不带操作者，形成的任务不记建任务人；册写不下一件也不问；不带操作者的重问
// 不进册；操作者在场而册没接答错；重问的结果与不带操作者时相同。
func TestAReaskRecordsWhoPressedItOnceAndAsksNothingWhenItCannot(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.enterByHandover(t, "parcel-2", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.places.resolution = ports.RequirementMissing
	fixture.triggerAt(t, "parcel-1", deliveryTriggeredAt)
	fixture.triggerAt(t, "parcel-2", deliveryTriggeredAt.Add(time.Minute))
	fixture.places.resolution = ports.RequirementResolved

	asks := &askLedgerDouble{}
	reaskWith := func(ledger ports.DeliveryDispatchAskLedger, by string) (application.ReaskPendingDeliveryDispatchesResult, error) {
		return application.NewReaskPendingDeliveryDispatchesHandler(application.ReaskPendingDeliveryDispatchesDeps{
			Pending: fixture.pending, Cadences: cadenceReaderStub{}, Trigger: fixture.trigger(), Failed: fixture.pending,
			Clock: handoverClock{at: reaskAt}, Asks: ledger,
		}).Reask(t.Context(), application.ReaskPendingDeliveryDispatchesCommand{
			TenantID: tenantOne(t), Scope: application.ReaskAllPending, Limit: 10, ReaskedBy: by,
		})
	}

	broken := &askLedgerDouble{err: errors.New("ledger down")}
	calls := fixture.askedCalls()
	if _, err := reaskWith(broken, firstOperator); !errors.Is(err, application.ErrDeliveryDispatchAskUnrecorded) {
		t.Fatalf("册写不下：err = %v，want ErrDeliveryDispatchAskUnrecorded", err)
	}
	if fixture.askedCalls() != calls {
		t.Fatal("册写不下，一件也不该问")
	}
	if _, err := reaskWith(nil, firstOperator); !errors.Is(err, application.ErrDeliveryDispatchAskLedgerNotWired) {
		t.Fatalf("册没接：err = %v，want ErrDeliveryDispatchAskLedgerNotWired", err)
	}
	if _, err := reaskWith(asks, "issuer#sub\x00"); err != nil {
		t.Fatalf("带 NUL 的操作者：%v", err)
	} else if len(asks.records) != 0 || fixture.askedCalls() != calls {
		t.Fatal("带 NUL 的操作者答不受理：不进册、不问")
	}

	result, err := reaskWith(asks, firstOperator)
	if err != nil || result.Outcome() != application.ReaskAsked || len(result.Items()) != 2 {
		t.Fatalf("重问：%s %d 件 %v，want 问过两件", result.Outcome(), len(result.Items()), err)
	}
	if len(asks.records) != 1 || asks.records[0].Kind != ports.DeliveryDispatchAskReask || asks.records[0].AskedBy.String() != firstOperator ||
		asks.records[0].Scope != "ALL" || asks.records[0].Limit != 10 || !asks.records[0].AskedAt.Equal(reaskAt) ||
		asks.records[0].Segment.String() != "" || asks.records[0].Object.String() != "" {
		t.Fatalf("问询册 = %+v，want 一次按下一行、带问法与上限与重问时刻、不带段与对象", asks.records)
	}
	for _, task := range fixture.tasks.records {
		if task.OpenedBy.Present() {
			t.Fatalf("重问形成的任务不记建任务人：%q", task.OpenedBy.String())
		}
	}

	if _, err := reaskWith(asks, ""); err != nil || len(asks.records) != 1 {
		t.Fatalf("不带操作者的重问不进册：%v %d 行", err, len(asks.records))
	}
}
