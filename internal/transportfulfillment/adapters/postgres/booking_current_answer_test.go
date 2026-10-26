package postgres_test

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: `AT-TF-151` 真库面「当前答复是定局」、`AT-TF-332`「行上带当前那一份答复（有定局取定局，否则取候补）」，票
// demo-chain-gaps/177 完成判据——同一组答复，答复写口的 BookingAnswers.FindByKey（收费发生项经它读订舱的当前答复）与查阅口
// 订舱册 ReviewCatalogue.ListBookingRequests 选出同一份：两面都经 domain.CurrentCarrierAcceptance，不各选一遍。能经答复编排
// 落库的四种形照常选。凑不成一段答复史的三种形绕过编排直接落库——定局早于候补（库上约束拦不住），以及拆掉两条部分唯一索引
// 才落得进的两份定局、两份候补——两面同样答包着 domain.ErrBookingAnswersInconsistent 的错误，不挑一份。
func TestTheWriteAndReadFacesChooseTheSameCurrentAnswer(t *testing.T) {
	stores := newBookingCatalogueStores(t)
	ctx := t.Context()
	// 只在这只一次性库上拆：要证的是选法本身不挑，而这两种形索引在时根本落不进来。
	for _, index := range []string{"booking_answer_one_settling", "booking_answer_one_waitlist"} {
		if _, err := stores.pool.Exec(ctx, `DROP INDEX transport_fulfillment.`+index); err != nil {
			t.Fatalf("拆索引 %s：%v", index, err)
		}
	}

	type answer struct {
		id       string
		outcome  domain.CarrierAcceptanceOutcome
		quantity int64
		after    time.Duration
	}
	const inconsistent = "!"
	for index, shape := range []struct {
		name    string
		answers []answer
		want    string
	}{
		{"无答复", nil, ""},
		{"只有候补", []answer{{"W", domain.BookingWaitlisted, 0, time.Minute}}, "W"},
		{"候补加定局", []answer{{"W", domain.BookingWaitlisted, 0, time.Minute}, {"A", domain.BookingAccepted, 120, 2 * time.Minute}}, "A"},
		{"只有定局", []answer{{"R", domain.BookingRefused, 0, time.Minute}}, "R"},
		{"两份定局", []answer{{"A", domain.BookingAccepted, 120, time.Minute}, {"R", domain.BookingRefused, 0, 2 * time.Minute}}, inconsistent},
		{"两份候补", []answer{{"W", domain.BookingWaitlisted, 0, time.Minute}, {"W2", domain.BookingWaitlisted, 0, 2 * time.Minute}}, inconsistent},
		{"定局早于候补", []answer{{"A", domain.BookingAccepted, 120, time.Minute}, {"W", domain.BookingWaitlisted, 0, 2 * time.Minute}}, inconsistent},
	} {
		t.Run(shape.name, func(t *testing.T) {
			// 一种形一个租户：查阅口一页里有一行读不出状态词就整页答错误，各形分开读才看得见各自的答案。
			booking := stores.book(t, "SYN-TENANT-177-"+strconv.Itoa(index), "SYN-BOOKING-177", 0)
			for _, given := range shape.answers {
				stores.answer(t, booking, given.id, given.outcome, given.quantity, given.after)
			}

			written, found, writeErr := stores.answers.FindByKey(ctx, ports.BookingKey{TenantID: booking.TenantID(), Booking: booking.Booking()})
			rows, readErr := stores.catalogue.ListBookingRequests(ctx, booking.TenantID(), 10)
			if shape.want == inconsistent {
				if !errors.Is(writeErr, domain.ErrBookingAnswersInconsistent) {
					t.Errorf("写面 FindByKey = %+v found=%v err=%v，want ErrBookingAnswersInconsistent", written.Acceptance, found, writeErr)
				}
				if !errors.Is(readErr, domain.ErrBookingAnswersInconsistent) {
					t.Errorf("读面订舱册 = %+v err=%v，want ErrBookingAnswersInconsistent", rows, readErr)
				}
				return
			}

			writeChoice := ""
			if writeErr != nil {
				t.Errorf("写面 FindByKey：%v", writeErr)
			} else if found {
				writeChoice = written.Acceptance.Acceptance().String()
			}
			readChoice := ""
			switch {
			case readErr != nil:
				t.Errorf("读面订舱册：%v", readErr)
			case len(rows) != 1:
				t.Errorf("读面订舱册 %d 行，want 1", len(rows))
			case rows[0].Current != nil:
				readChoice = rows[0].Current.Answer
			}
			if writeChoice != shape.want {
				t.Errorf("写面选出 %q，want %q", writeChoice, shape.want)
			}
			if readChoice != shape.want {
				t.Errorf("读面选出 %q，want %q", readChoice, shape.want)
			}
		})
	}
}
