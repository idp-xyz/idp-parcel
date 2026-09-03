package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: CONTEXT「信用政策……按责任法人、业务角色、费用类型、金额或比例形成版本」。
//
// 「金额**或**比例」是两格封闭而不是「一个数加一个标记」（票 party-commercial-context-gaps/03
// 的裁决）：额度 100 作金额与作比例在同一个数字里长得一模一样，标记设错时没有任何东西能分辨，
// 两种读法都产出一个合法的额度、只是差几个数量级。两格各自构造，值落在哪一格本身就是判别式，
// 它不可能与自己不一致。
func TestCreditLimitIsEitherAnAmountOrARatioNeverBoth(t *testing.T) {
	t.Run("amount", func(t *testing.T) {
		limit, err := domain.NewCreditAmountLimit(500000)
		if err != nil {
			t.Fatalf("new amount limit: %v", err)
		}
		if minor, ok := limit.AmountMinor(); !ok || minor != 500000 {
			t.Fatalf("amount = (%d, %v)", minor, ok)
		}
		if _, ok := limit.RatioBasisPoints(); ok {
			t.Fatal("一份金额额度交回了比例")
		}
	})

	t.Run("ratio", func(t *testing.T) {
		limit, err := domain.NewCreditRatioLimit(1500)
		if err != nil {
			t.Fatalf("new ratio limit: %v", err)
		}
		if bps, ok := limit.RatioBasisPoints(); !ok || bps != 1500 {
			t.Fatalf("ratio = (%d, %v)", bps, ok)
		}
		if _, ok := limit.AmountMinor(); ok {
			t.Fatal("一份比例额度交回了金额")
		}
	})

	// 零额度是「授予了零信用」，与「没有政策」不同（CreditBasis.Applicable 分它们），所以
	// 零在两格都立得住；负值在哪一格都无意义。
	t.Run("zero is a granted limit, negative is not a limit", func(t *testing.T) {
		if _, err := domain.NewCreditAmountLimit(0); err != nil {
			t.Fatalf("零金额额度立不住：%v", err)
		}
		if _, err := domain.NewCreditRatioLimit(0); err != nil {
			t.Fatalf("零比例额度立不住：%v", err)
		}
		if _, err := domain.NewCreditAmountLimit(-1); !errors.Is(err, domain.ErrInvalidCreditLimit) {
			t.Fatalf("error = %v；负金额额度立住了", err)
		}
		if _, err := domain.NewCreditRatioLimit(-1); !errors.Is(err, domain.ErrInvalidCreditLimit) {
			t.Fatalf("error = %v；负比例额度立住了", err)
		}
	})

	// 零值 CreditLimit 是「忘了填」，不是任何一格；构造门要把它挡在政策之外。
	t.Run("the zero value is neither", func(t *testing.T) {
		var limit domain.CreditLimit
		if _, ok := limit.AmountMinor(); ok {
			t.Fatal("零值额度读成了金额")
		}
		if _, ok := limit.RatioBasisPoints(); ok {
			t.Fatal("零值额度读成了比例")
		}
	})
}
