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

// 本文件钉票 demo-chain-gaps/24 的编排面（ADR-0202 决定二、五与文末补记）：订舱引在册班次、挂在在册且未取消的委托下；
// 承运答复只由该订舱所属委托的服务提供方答，五值，候补之后可接一份定局。

func submittedCommissionFixture(t *testing.T) *commissionFixture {
	t.Helper()
	fixture := newCommissionFixture(t)
	if result, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t)); err != nil || result.Outcome() != application.CommissionSubmitted {
		t.Fatalf("submit commission: %q %v", result.Outcome(), err)
	}
	return fixture
}

// Covers: `AT-TF-148`（编排面）——订舱提交成立：在已提交委托下就在册班次订舱答已提交，班次随订舱保全；同一订舱原样重交
// 答原订舱，同标识换班次是冲突、保留原订舱；订舱不碰委托。
func TestABookingOnARegisteredScheduleIsSubmittedOnce(t *testing.T) {
	fixture := submittedCommissionFixture(t)
	command := submitBookingCommand(t)

	first, err := fixture.handler.SubmitBooking(context.Background(), command)
	if err != nil || first.Outcome() != application.BookingSubmitted {
		t.Fatalf("submit booking: %q %v", first.Outcome(), err)
	}
	record, present := first.Booking()
	if !present || record.Booking.Schedule().String() != "schedule-1" || record.Booking.Commission().String() != "commission-1" {
		t.Fatalf("订舱 = %+v", record)
	}

	replay, err := fixture.handler.SubmitBooking(context.Background(), command)
	if err != nil || replay.Outcome() != application.BookingExistingResult {
		t.Fatalf("原样重交 = %q %v", replay.Outcome(), err)
	}

	fixture.schedules.registered["tenant-1|schedule-2"] = true
	moved := submitBookingCommand(t)
	moved.Schedule = "schedule-2"
	conflict, err := fixture.handler.SubmitBooking(context.Background(), moved)
	if err != nil || conflict.Outcome() != application.BookingConflict {
		t.Fatalf("同标识换班次 = %q %v, want BOOKING_CONFLICT", conflict.Outcome(), err)
	}
	if kept := fixture.bookings.records["tenant-1|booking-1"]; kept.Booking.Schedule().String() != "schedule-1" {
		t.Fatalf("冲突改写了原订舱的班次：%q", kept.Booking.Schedule())
	}
}

// Covers: `AT-TF-146`、`AT-TF-147`（编排面）——订舱引用的班次不在本租户班次登记里答班次不在册（他租户的同名班次同落
// 这一格）；委托不在册答委托不在册；委托已取消答委托不可订舱；各格零写入、各自分开不并进未受理。重放判在两项核对之前：
// 班次事后撤不掉，但委托事后取消了，原样重放仍是原订舱。
func TestABookingNeedsARegisteredCommissionAndSchedule(t *testing.T) {
	cases := map[string]struct {
		arrange func(t *testing.T, fixture *commissionFixture, command *application.SubmitBookingCommand)
		want    application.CommissionOutcome
	}{
		"班次从未登记": {
			arrange: func(_ *testing.T, _ *commissionFixture, command *application.SubmitBookingCommand) {
				command.Schedule = "schedule-9"
			},
			want: application.BookingScheduleNotFound,
		},
		"班次只登在别的租户名下": {
			arrange: func(_ *testing.T, fixture *commissionFixture, command *application.SubmitBookingCommand) {
				fixture.schedules.registered["tenant-2|schedule-other"] = true
				command.Schedule = "schedule-other"
			},
			want: application.BookingScheduleNotFound,
		},
		"委托不在册": {
			arrange: func(_ *testing.T, _ *commissionFixture, command *application.SubmitBookingCommand) {
				command.Commission = "commission-9"
			},
			want: application.CommissionNotFound,
		},
		"委托已取消": {
			arrange: func(t *testing.T, fixture *commissionFixture, _ *application.SubmitBookingCommand) {
				tenant, _ := domain.NewTenantID("tenant-1")
				cancelled, err := fixture.handler.CancelCommission(context.Background(), application.CancelCommissionCommand{
					TenantID: tenant, Commission: "commission-1", CancelledAt: commissionSentAt.Add(10 * time.Minute),
				})
				if err != nil || cancelled.Outcome() != application.CommissionCancelled {
					t.Fatalf("cancel: %q %v", cancelled.Outcome(), err)
				}
			},
			want: application.BookingCommissionNotBookable,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := submittedCommissionFixture(t)
			command := submitBookingCommand(t)
			testCase.arrange(t, fixture, &command)
			result, err := fixture.handler.SubmitBooking(context.Background(), command)
			if err != nil || result.Outcome() != testCase.want {
				t.Fatalf("outcome = %q %v, want %q", result.Outcome(), err, testCase.want)
			}
			if len(fixture.bookings.records) != 0 {
				t.Fatalf("核不过的订舱落了 %d 行", len(fixture.bookings.records))
			}
		})
	}

	t.Run("重放判在委托核对之前", func(t *testing.T) {
		fixture := newBookedFixture(t)
		tenant, _ := domain.NewTenantID("tenant-1")
		if cancelled, err := fixture.handler.CancelCommission(context.Background(), application.CancelCommissionCommand{
			TenantID: tenant, Commission: "commission-1", CancelledAt: commissionSentAt.Add(time.Hour),
		}); err != nil || cancelled.Outcome() != application.CommissionCancelled {
			t.Fatalf("cancel: %q %v", cancelled.Outcome(), err)
		}
		replay, err := fixture.handler.SubmitBooking(context.Background(), submitBookingCommand(t))
		if err != nil || replay.Outcome() != application.BookingExistingResult {
			t.Fatalf("委托取消后原样重放 = %q %v, want EXISTING_BOOKING", replay.Outcome(), err)
		}
	})
}

// Covers: `AT-TF-146` 未决那一支——班次登记或委托册读不动答未决，各自指名等谁，零写入。
func TestABookingWhoseReferencesCannotBeReadIsUndecided(t *testing.T) {
	scheduleDown := submittedCommissionFixture(t)
	scheduleDown.schedules.findErr = errors.New("schedule register down")
	result, err := scheduleDown.handler.SubmitBooking(context.Background(), submitBookingCommand(t))
	if err != nil || result.Outcome() != application.CommissionUndecided || result.UndecidedReason() != application.ScheduleStoreUnavailable {
		t.Fatalf("班次读不动 = %q / %q %v", result.Outcome(), result.UndecidedReason(), err)
	}

	commissionDown := submittedCommissionFixture(t)
	commissionDown.commissions.findErr = errors.New("commission store down")
	result, err = commissionDown.handler.SubmitBooking(context.Background(), submitBookingCommand(t))
	if err != nil || result.Outcome() != application.CommissionUndecided || result.UndecidedReason() != application.CommissionStoreUnavailable {
		t.Fatalf("委托读不动 = %q / %q %v", result.Outcome(), result.UndecidedReason(), err)
	}
	if len(scheduleDown.bookings.records)+len(commissionDown.bookings.records) != 0 {
		t.Fatal("读不动的订舱落了行")
	}
}

// racingBookingStore 在本次 Save 那一刻让另一份先到的订舱占位。
type racingBookingStore struct {
	*bookingStoreDouble
	winner ports.BookingRecord
}

func (store *racingBookingStore) Save(context.Context, ports.BookingRecord) (ports.BookingSaveOutcome, error) {
	store.records[bookingStoreKey(store.winner.Key)] = store.winner
	return ports.BookingAlreadyRegistered, nil
}

// Covers: `AT-TF-148` 并发那一支——并发首交的输家按摘要分已有与冲突，不把冲突吞成「已有」。
func TestAConcurrentBookingLoserIsJudgedByContent(t *testing.T) {
	for name, testCase := range map[string]struct {
		winnerSchedule string
		want           application.CommissionOutcome
	}{
		"先到者同内容":  {winnerSchedule: "schedule-1", want: application.BookingExistingResult},
		"先到者换了班次": {winnerSchedule: "schedule-2", want: application.BookingConflict},
	} {
		t.Run(name, func(t *testing.T) {
			seed := submittedCommissionFixture(t)
			seed.schedules.registered["tenant-1|schedule-2"] = true
			winnerCommand := submitBookingCommand(t)
			winnerCommand.Schedule = testCase.winnerSchedule
			if result, err := seed.handler.SubmitBooking(context.Background(), winnerCommand); err != nil || result.Outcome() != application.BookingSubmitted {
				t.Fatalf("seed: %q %v", result.Outcome(), err)
			}
			winner := seed.bookings.records["tenant-1|booking-1"]

			fixture := submittedCommissionFixture(t)
			racing := &racingBookingStore{bookingStoreDouble: newBookingStore(), winner: winner}
			handler := application.NewCommissionTransportHandler(application.CommissionTransportDeps{
				Commissions: fixture.commissions,
				Bookings:    racing,
				Answers:     fixture.answers,
				Schedules:   fixture.schedules,
				Agreements:  fixture.agreements,
				Downstream:  fixture.handoff,
				Clock:       commissionClock{at: commissionNowAt},
			})
			result, err := handler.SubmitBooking(context.Background(), submitBookingCommand(t))
			if err != nil || result.Outcome() != testCase.want {
				t.Fatalf("outcome = %q %v, want %q", result.Outcome(), err, testCase.want)
			}
		})
	}
}

// Covers: `AT-TF-149`（编排面）——答复方的来源身份不是该订舱所属委托的服务提供方：答来源不是服务方，零写入；按同一串
// 字面比，大小写不同就是另一个来源；来源核对排在重放之前，拿别人的答复标识原样重放也读不回别人的答复。
func TestOnlyTheCommissionedProviderAnswersItsBooking(t *testing.T) {
	fixture := newBookedFixture(t)
	foreign := answerBookingCommand(t, domain.BookingAccepted)
	foreign.Source = "partner-2"
	result, err := fixture.handler.AnswerBooking(context.Background(), foreign)
	if err != nil || result.Outcome() != application.BookingSourceNotProvider {
		t.Fatalf("别的承运商答 = %q %v, want SOURCE_NOT_SERVICE_PROVIDER", result.Outcome(), err)
	}
	if fixture.answers.answersOf("booking-1") != 0 {
		t.Fatal("来源不符的答复落了行")
	}

	cased := answerBookingCommand(t, domain.BookingAccepted)
	cased.Source = "PARTNER-1"
	if result, err := fixture.handler.AnswerBooking(context.Background(), cased); err != nil || result.Outcome() != application.BookingSourceNotProvider {
		t.Fatalf("大小写不同的来源 = %q %v——来源被归一了", result.Outcome(), err)
	}

	own := answerBookingCommand(t, domain.BookingAccepted)
	if result, err := fixture.handler.AnswerBooking(context.Background(), own); err != nil || result.Outcome() != application.BookingAnswered {
		t.Fatalf("服务方自己答 = %q %v", result.Outcome(), err)
	}
	replayByForeign, err := fixture.handler.AnswerBooking(context.Background(), foreign)
	if err != nil || replayByForeign.Outcome() != application.BookingSourceNotProvider {
		t.Fatalf("别的承运商拿同一答复标识原样重放 = %q %v——读回了别人的答复", replayByForeign.Outcome(), err)
	}
	if _, present := replayByForeign.Answer(); present {
		t.Fatal("来源不符的答复带回了在册答复")
	}
}

// Covers: `AT-TF-150`（编排面）——答复五值：接受成约带接受量；拒绝、失效、撤回、候补各带依据、不成约。`待确认`不是答复值：
// 没有任何答复时订舱当前读不到答复。
func TestACarrierAnswersInFiveValues(t *testing.T) {
	fresh := newBookedFixture(t)
	if fresh.answers.answersOf("booking-1") != 0 {
		t.Fatal("订舱一提交就有了答复——`待确认`被写成了答复值")
	}
	for _, outcome := range []domain.CarrierAcceptanceOutcome{
		domain.BookingAccepted, domain.BookingRefused, domain.BookingExpired, domain.BookingWithdrawn, domain.BookingWaitlisted,
	} {
		t.Run(outcome.String(), func(t *testing.T) {
			fixture := newBookedFixture(t)
			result, err := fixture.handler.AnswerBooking(context.Background(), answerBookingCommand(t, outcome))
			if err != nil || result.Outcome() != application.BookingAnswered {
				t.Fatalf("answer %s = %q %v", outcome, result.Outcome(), err)
			}
			answer, _ := result.Answer()
			if answer.Acceptance.Outcome() != outcome || answer.Acceptance.Binds() != (outcome == domain.BookingAccepted) {
				t.Fatalf("答复 = %+v", answer.Acceptance)
			}
		})
	}
}

// Covers: `AT-TF-151`（编排面）——候补之后接一份定局，两份都留；定局之后再答（换标识）是冲突；候补之后再候补是冲突；
// 早于候补的定局是冲突。
func TestAWaitlistedBookingSettlesOnce(t *testing.T) {
	fixture := newBookedFixture(t)
	waitlist := answerBookingCommand(t, domain.BookingWaitlisted)
	waitlist.Acceptance = "answer-waitlist"
	if result, err := fixture.handler.AnswerBooking(context.Background(), waitlist); err != nil || result.Outcome() != application.BookingAnswered {
		t.Fatalf("候补 = %q %v", result.Outcome(), err)
	}

	again := answerBookingCommand(t, domain.BookingWaitlisted)
	again.Acceptance = "answer-waitlist-2"
	again.DecidedAt = waitlist.DecidedAt.Add(time.Minute)
	if result, err := fixture.handler.AnswerBooking(context.Background(), again); err != nil || result.Outcome() != application.BookingAnswerConflict {
		t.Fatalf("候补之后再候补 = %q %v", result.Outcome(), err)
	}
	early := answerBookingCommand(t, domain.BookingAccepted)
	early.Acceptance = "answer-early"
	early.DecidedAt = waitlist.DecidedAt.Add(-time.Minute)
	if result, err := fixture.handler.AnswerBooking(context.Background(), early); err != nil || result.Outcome() != application.BookingAnswerConflict {
		t.Fatalf("早于候补的定局 = %q %v", result.Outcome(), err)
	}

	confirmed := answerBookingCommand(t, domain.BookingAccepted)
	confirmed.Acceptance = "answer-confirmed"
	confirmed.DecidedAt = waitlist.DecidedAt.Add(time.Hour)
	result, err := fixture.handler.AnswerBooking(context.Background(), confirmed)
	if err != nil || result.Outcome() != application.BookingAnswered {
		t.Fatalf("候补之后接受 = %q %v", result.Outcome(), err)
	}
	if fixture.answers.answersOf("booking-1") != 2 {
		t.Fatalf("候补与接受应各留一份，册上 %d 份", fixture.answers.answersOf("booking-1"))
	}

	refused := answerBookingCommand(t, domain.BookingRefused)
	refused.Acceptance = "answer-refused"
	refused.DecidedAt = waitlist.DecidedAt.Add(2 * time.Hour)
	if result, err := fixture.handler.AnswerBooking(context.Background(), refused); err != nil || result.Outcome() != application.BookingAnswerConflict {
		t.Fatalf("定局之后另一份 = %q %v, want ANSWER_CONFLICT", result.Outcome(), err)
	}
	if replay, err := fixture.handler.AnswerBooking(context.Background(), waitlist); err != nil || replay.Outcome() != application.BookingAnswerExists {
		t.Fatalf("候补原样重放 = %q %v, want EXISTING_ANSWER（后继不改写候补）", replay.Outcome(), err)
	}
}

// Covers: `AT-TF-153` 与恢复面——答复命令缺来源身份是装配违约（error）；订舱指名的委托读不回是数据违约（error），不答
// 「订舱不存在」；订舱、委托、答复各册读不动答未决、指名等谁。
func TestAnAnswerRecoveryDiscipline(t *testing.T) {
	fixture := newBookedFixture(t)
	missing := answerBookingCommand(t, domain.BookingAccepted)
	missing.Source = " "
	if _, err := fixture.handler.AnswerBooking(context.Background(), missing); !errors.Is(err, application.ErrAnswerSourceMissing) {
		t.Fatalf("缺来源：err = %v", err)
	}

	orphan := newBookedFixture(t)
	orphan.commissions.records = map[string]ports.TransportCommissionRecord{}
	if _, err := orphan.handler.AnswerBooking(context.Background(), answerBookingCommand(t, domain.BookingAccepted)); !errors.Is(err, application.ErrBookingWithoutCommission) {
		t.Fatalf("订舱的委托读不回：err = %v", err)
	}

	for name, testCase := range map[string]struct {
		breakIt func(fixture *commissionFixture)
		reason  application.CommissionUndecidedReason
	}{
		"订舱册": {func(fixture *commissionFixture) { fixture.bookings.findErr = errors.New("down") }, application.BookingStoreUnavailable},
		"委托册": {func(fixture *commissionFixture) { fixture.commissions.findErr = errors.New("down") }, application.CommissionStoreUnavailable},
		"答复册": {func(fixture *commissionFixture) { fixture.answers.findErr = errors.New("down") }, application.AnswerStoreUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newBookedFixture(t)
			testCase.breakIt(fixture)
			result, err := fixture.handler.AnswerBooking(context.Background(), answerBookingCommand(t, domain.BookingAccepted))
			if err != nil || result.Outcome() != application.CommissionUndecided || result.UndecidedReason() != testCase.reason {
				t.Fatalf("%s 读不动 = %q / %q %v", name, result.Outcome(), result.UndecidedReason(), err)
			}
		})
	}
}
