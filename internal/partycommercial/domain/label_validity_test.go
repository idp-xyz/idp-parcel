package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func deliveryFinalization(t *testing.T) []domain.FinalizationDeclaration {
	t.Helper()
	return []domain.FinalizationDeclaration{{
		Outcome:   domain.DeclaredEffectiveDelivery,
		FinalKind: commercialValue(t, domain.NewRuleReference, "NETWORK_SERVICE_DELIVERED"),
	}}
}

// Covers: ADR-0119 Decision 二、三——面单有效期 = 起算时刻种类 × 正时长；种类集外与零/负时长都立不住，
// 「没有有效期」不走这条门。
func TestLabelValidityDeclarationNeedsAnAnchorKindAndAPositiveDuration(t *testing.T) {
	declaration, err := domain.NewLabelValidityDeclaration(domain.ChannelResultObservedAnchor, 72*time.Hour)
	if err != nil {
		t.Fatalf("new label validity: %v", err)
	}
	if declaration.Anchor() != domain.ChannelResultObservedAnchor || declaration.Duration() != 72*time.Hour {
		t.Fatalf("声明变形：%+v", declaration)
	}
	if declaration.Anchor().String() != "CHANNEL_RESULT_OBSERVED" {
		t.Fatalf("起算时刻种类的名字 = %q", declaration.Anchor())
	}

	for name, tc := range map[string]struct {
		anchor   domain.ValidityAnchorKind
		duration time.Duration
	}{
		"种类集外": {domain.ValidityAnchorKindInvalid, 72 * time.Hour},
		"零时长":  {domain.ChannelResultObservedAnchor, 0},
		"负时长":  {domain.ChannelResultObservedAnchor, -time.Hour},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewLabelValidityDeclaration(tc.anchor, tc.duration); !errors.Is(err, domain.ErrInvalidLabelValidity) {
				t.Fatalf("error = %v, want ErrInvalidLabelValidity", err)
			}
		})
	}
}

// Covers: ADR-0119 Decision 一、三——有效期是终局规则声明上的一格，一版至多一条；缺席由两个构造门分立
// 表达：NewFinalRuleContent 就是「没有这一格」，NewFinalRuleContentWithValidity 必须带一条立得住的声明；
// 终局规则行的既有判据两条门都守。
func TestFinalRuleContentCarriesAtMostOneLabelValidity(t *testing.T) {
	owner := rulePackage(t)
	validity, err := domain.NewLabelValidityDeclaration(domain.ChannelResultObservedAnchor, 72*time.Hour)
	if err != nil {
		t.Fatalf("label validity: %v", err)
	}

	t.Run("没有这一格就是没有", func(t *testing.T) {
		content, err := domain.NewFinalRuleContent(owner, deliveryFinalization(t))
		if err != nil {
			t.Fatalf("new final rule content: %v", err)
		}
		if _, declared := content.Validity(); declared {
			t.Fatal("未声明有效期的终局规则答成了已声明——成功面单会被按墙钟判失效")
		}
	})

	t.Run("带有效期的声明原样读回，终局规则行不变", func(t *testing.T) {
		content, err := domain.NewFinalRuleContentWithValidity(owner, deliveryFinalization(t), validity)
		if err != nil {
			t.Fatalf("new final rule content with validity: %v", err)
		}
		declared, present := content.Validity()
		if !present || declared != validity {
			t.Fatalf("validity = %+v present = %v", declared, present)
		}
		if _, ok := content.FinalKindFor(domain.DeclaredEffectiveDelivery); !ok {
			t.Fatal("加了有效期之后终局规则行丢了")
		}
		if !content.Owner().SameVersionAs(owner) {
			t.Fatal("声明没有钉住它所属的规则包")
		}
	})

	t.Run("有效期立不住整份拒", func(t *testing.T) {
		if _, err := domain.NewFinalRuleContentWithValidity(owner, deliveryFinalization(t), domain.LabelValidityDeclaration{}); !errors.Is(err, domain.ErrInvalidLabelValidity) {
			t.Fatalf("error = %v, want ErrInvalidLabelValidity", err)
		}
	})

	t.Run("既有判据在带有效期的门上照守", func(t *testing.T) {
		if _, err := domain.NewFinalRuleContentWithValidity(owner, nil, validity); !errors.Is(err, domain.ErrFinalContentNotConfigured) {
			t.Fatalf("error = %v; 零行声明被收下了", err)
		}
		if _, err := domain.NewFinalRuleContentWithValidity(effectiveServiceProduct(t), deliveryFinalization(t), validity); !errors.Is(err, domain.ErrUnusableRulePackage) {
			t.Fatalf("error = %v, want ErrUnusableRulePackage", err)
		}
	})
}
