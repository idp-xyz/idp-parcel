package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 报价详情把租户规则和这一条自己是不是有效接受分开答，生效停在缺的那一格。

func TestQuotationEffectivenessSeparatesTheRuleFromThisAcceptance(t *testing.T) {
	book := newQuotationBook(t)
	fact := readyFact()
	fact.Scope = "SYN-SCOPE-SELL"
	book.prices.facts["PLAN"] = fact
	sentAt := time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
	sent, err := book.sender.Handle(context.Background(), book.command(t, sentAt, "digest-sell", ""))
	if err != nil || sent.Outcome != application.QuotationSent {
		t.Fatalf("发出 = %s %v", sent.Outcome, err)
	}
	view := &effectivenessView{digest: sent.Send.ContentDigest()}
	cards := &effectivenessCards{}
	reader, err := application.NewReadQuotationHandler(book.store, &fakeReplyStore{}, book.prices, view, cards)
	if err != nil {
		t.Fatal(err)
	}
	read := func() application.QuotationReadingDetail {
		t.Helper()
		detail, readErr := reader.Get(context.Background(), book.tenant, sent.Send.ID(), sentAt)
		if readErr != nil {
			t.Fatal(readErr)
		}
		return detail
	}

	view.accepted = true
	detail := read()
	if detail.Effectiveness.Requirement != application.QuotationPublicationRuleUnconfigured || !detail.Effectiveness.ValidAcceptance {
		t.Fatalf("未登记却已接受 = %s valid %v", detail.Effectiveness.Requirement, detail.Effectiveness.ValidAcceptance)
	}
	if detail.Effectiveness.Requirement.String() == "ACCEPTANCE_REQUIRED" {
		t.Fatal("规则状态不该是要求接受")
	}

	view.configured, view.requires = true, true
	detail = read()
	if detail.Effectiveness.Requirement != application.QuotationPublicationRuleRequired || !detail.Effectiveness.ValidAcceptance {
		t.Fatalf("要求且这条已接受 = %s valid %v", detail.Effectiveness.Requirement, detail.Effectiveness.ValidAcceptance)
	}

	view.accepted = false
	detail = read()
	if detail.Effectiveness.Requirement != application.QuotationPublicationRuleRequired || detail.Effectiveness.ValidAcceptance {
		t.Fatalf("要求但这条还不是接受 = %s valid %v", detail.Effectiveness.Requirement, detail.Effectiveness.ValidAcceptance)
	}

	view.requires = false
	detail = read()
	if detail.Effectiveness.Requirement != application.QuotationPublicationRuleNotRequired {
		t.Fatalf("不要求 = %s", detail.Effectiveness.Requirement)
	}

	cards.has, cards.basis = true, sent.Send.ID().String()
	detail = read()
	if !detail.Effectiveness.RecordedBasis {
		t.Fatal("记下的依据就是这条报价")
	}
	cards.basis = "QTS-OTHER"
	detail = read()
	if detail.Effectiveness.RecordedBasis {
		t.Fatal("记下的是别的引用")
	}
}

func TestQuotationEffectivenessNamesTheStepThatIsStillOpen(t *testing.T) {
	cases := []struct {
		name       string
		withdrawn  bool
		named      bool
		onRegister bool
		selects    bool
		next       string
	}{
		{name: "草稿已撤下", withdrawn: true, named: true, next: "CANNOT_PUBLISH"},
		{name: "合同还没指名", named: false, onRegister: true, selects: true, next: "SOURCE_LIST_FROZEN"},
		{name: "等发布价卡", named: true, next: "PUBLISH_CARD"},
		{name: "已在册但此刻选不中", named: true, onRegister: true, next: "NOT_SELECTABLE"},
		{name: "三格都成立", named: true, onRegister: true, selects: true, next: "ALREADY_EFFECTIVE"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			book := newQuotationBook(t)
			fact := readyFact()
			fact.Scope = "SYN-SCOPE-SELL"
			book.prices.facts["PLAN"] = fact
			sentAt := time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
			sent, err := book.sender.Handle(context.Background(), book.command(t, sentAt, "digest-sell", ""))
			if err != nil || sent.Outcome != application.QuotationSent {
				t.Fatalf("发出 = %s %v", sent.Outcome, err)
			}
			if item.withdrawn {
				withdrawn := fact
				withdrawn.Standing = ports.QuotedPriceWithdrawn
				book.prices.facts["PLAN"] = withdrawn
			}
			reader, err := application.NewReadQuotationHandler(book.store, &fakeReplyStore{}, book.prices, &effectivenessView{named: item.named}, &effectivenessCards{
				onRegister: item.onRegister, selects: item.selects,
			})
			if err != nil {
				t.Fatal(err)
			}
			detail, err := reader.Get(context.Background(), book.tenant, sent.Send.ID(), sentAt)
			if err != nil {
				t.Fatal(err)
			}
			got := detail.Effectiveness
			if got.NextStep != item.next || got.PriceOnRegister != item.onRegister || got.SelectsNow != item.selects || got.ScopeNamed != item.named {
				t.Fatalf("生效 = %+v，want %s", got, item.next)
			}
		})
	}
}

type effectivenessView struct {
	named      bool
	configured bool
	requires   bool
	accepted   bool
	digest     string
}

func (view *effectivenessView) LoadQuotationPublicationRequirement(context.Context, domain.TenantID) (bool, bool, error) {
	return view.requires, view.configured, nil
}

func (view *effectivenessView) SaveQuotationPublicationRequirement(context.Context, domain.QuotationPublicationRequirement) (bool, bool, error) {
	return false, false, nil
}

func (view *effectivenessView) ListQuotationSendsForPlan(context.Context, domain.TenantID, string, string) ([]ports.QuotationSendRef, error) {
	return nil, nil
}

func (view *effectivenessView) AcceptedQuotationReply(context.Context, domain.TenantID, string) (string, bool, bool, error) {
	if !view.accepted {
		return "", false, false, nil
	}
	return view.digest, true, true, nil
}

func (view *effectivenessView) ContractNamesPricingScope(context.Context, domain.TenantID, string, string, time.Time) (bool, error) {
	return view.named, nil
}

type effectivenessCards struct {
	onRegister bool
	selects    bool
	basis      string
	has        bool
}

func (cards *effectivenessCards) ReadRegisteredPrice(context.Context, domain.TenantID, string, string, time.Time) (ports.RegisteredPriceFact, error) {
	return ports.RegisteredPriceFact{
		OnRegister: cards.onRegister, SelectsNow: cards.selects,
		PublicationBasis: cards.basis, HasPublicationBasis: cards.has,
	}, nil
}
