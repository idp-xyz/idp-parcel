package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: CONTEXT「商业价格规则必须声明含税、未税或税务不适用，并引用适用税务分类」。
//
// 三值与分类引用做成**充要耦合**而不是两个独立字段：含税与未税各自要求一份适用分类（不同分类
// 下同一个数是不同的钱），而「税务不适用」恰恰要求没有分类——给它挂一个分类就是在说它适用。
// 拆成两条独立校验会放过两种行：声明了含税却没有分类（下游不知道按哪套算），以及声明了不适用
// 却挂着分类（读起来像适用）。两种都不会报错，而它们改变的是报出去的价。
func TestTaxDispositionAndClassificationAreCoupledBothWays(t *testing.T) {
	classification := commercialValue(t, domain.NewTaxClassificationReference, "vat-standard")

	t.Run("tax inclusive requires a classification", func(t *testing.T) {
		if _, err := domain.NewTaxCaliber(
			domain.TaxInclusive, domain.TaxClassificationReference{},
		); !errors.Is(err, domain.ErrInvalidTaxCaliber) {
			t.Fatalf("error = %v；声明含税却没有适用分类的口径立住了", err)
		}
		caliber, err := domain.NewTaxCaliber(domain.TaxInclusive, classification)
		if err != nil {
			t.Fatalf("new tax caliber: %v", err)
		}
		if got, ok := caliber.Classification(); !ok || got != classification {
			t.Fatalf("classification = (%v, %v)", got, ok)
		}
	})

	t.Run("tax exclusive requires a classification too", func(t *testing.T) {
		if _, err := domain.NewTaxCaliber(
			domain.TaxExclusive, domain.TaxClassificationReference{},
		); !errors.Is(err, domain.ErrInvalidTaxCaliber) {
			t.Fatalf("error = %v；声明未税却没有适用分类的口径立住了", err)
		}
	})

	t.Run("tax not applicable refuses a classification", func(t *testing.T) {
		// 反向那一半：挂着分类的「不适用」读起来像适用，而没有任何东西会拦它。
		if _, err := domain.NewTaxCaliber(
			domain.TaxNotApplicable, classification,
		); !errors.Is(err, domain.ErrInvalidTaxCaliber) {
			t.Fatalf("error = %v；声明税务不适用却挂着分类的口径立住了", err)
		}
		caliber, err := domain.NewTaxCaliber(domain.TaxNotApplicable, domain.TaxClassificationReference{})
		if err != nil {
			t.Fatalf("new tax caliber: %v", err)
		}
		if _, ok := caliber.Classification(); ok {
			t.Fatal("税务不适用的口径交回了一份分类")
		}
	})

	t.Run("an unanswered disposition is not a caliber", func(t *testing.T) {
		// 零值不是「税务不适用」。缺席与「已判定为不适用」要人做的事不同：前者去补声明，
		// 后者不必。零值若落进不适用那一格，一次漏填就会静默变成一句商业声明。
		if _, err := domain.NewTaxCaliber(
			domain.TaxDispositionInvalid, domain.TaxClassificationReference{},
		); !errors.Is(err, domain.ErrInvalidTaxCaliber) {
			t.Fatalf("error = %v；未声明的税务处置被当成了不适用", err)
		}
	})
}
