package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: CONTEXT「它还声明计价所需的商业口径：汇率的牌价类型、取值时点规则……」，与 parcel-pricing
// 那侧「不接受未声明口径的裸汇率」。
//
// 三格全是引用：牌价类型（中行现汇卖出 / 央行中间价……）是租户与其银行之间的约定，开发方不是
// 当事人，按红线只能引用不能枚举；取值时点复用本上下文既有的时点语义引用与政策版本，不为汇率
// 另开一套时点口径。**缺任一格都立不住**——一个只有牌价类型没有时点的口径，与一个只有时点没有
// 牌价类型的口径，各自都能让同一份原清单在两个时刻算出两个数。
func TestFxCaliberRequiresQuoteTypeAndAsOfAnchorTogether(t *testing.T) {
	quoteType := commercialValue(t, domain.NewFxQuoteTypeReference, "boc-cash-selling")
	semantics := commercialValue(t, domain.NewAsOfSemanticsReference, "AT_ORDER_DATE")
	policyVersion := commercialValue(t, domain.NewAsOfPolicyVersion, "asof-policy/v3")

	t.Run("all three present", func(t *testing.T) {
		caliber, err := domain.NewFxCaliber(quoteType, semantics, policyVersion)
		if err != nil {
			t.Fatalf("new fx caliber: %v", err)
		}
		if got := caliber.QuoteType(); got != quoteType {
			t.Fatalf("quote type = %v", got)
		}
		if got := caliber.AsOfSemantics(); got != semantics {
			t.Fatalf("as-of semantics = %v", got)
		}
		if got := caliber.AsOfPolicyVersion(); got != policyVersion {
			t.Fatalf("as-of policy version = %v", got)
		}
	})

	t.Run("quote type missing", func(t *testing.T) {
		if _, err := domain.NewFxCaliber(
			domain.FxQuoteTypeReference{}, semantics, policyVersion,
		); !errors.Is(err, domain.ErrInvalidFxCaliber) {
			t.Fatalf("error = %v；没有牌价类型的汇率口径也立住了", err)
		}
	})

	t.Run("as-of semantics missing", func(t *testing.T) {
		if _, err := domain.NewFxCaliber(
			quoteType, domain.AsOfSemanticsReference{}, policyVersion,
		); !errors.Is(err, domain.ErrInvalidFxCaliber) {
			t.Fatalf("error = %v；没有取值时点语义的汇率口径也立住了", err)
		}
	})

	t.Run("as-of policy version missing", func(t *testing.T) {
		if _, err := domain.NewFxCaliber(
			quoteType, semantics, domain.AsOfPolicyVersion{},
		); !errors.Is(err, domain.ErrInvalidFxCaliber) {
			t.Fatalf("error = %v；时点语义没有钉住政策版本也立住了", err)
		}
	})
}

// 牌价类型引用与其它引用一样拒绝空白：一个空的牌价类型不是「用默认牌价」，是没声明。
func TestFxQuoteTypeReferenceRejectsBlank(t *testing.T) {
	if _, err := domain.NewFxQuoteTypeReference("  "); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("error = %v", err)
	}
}
