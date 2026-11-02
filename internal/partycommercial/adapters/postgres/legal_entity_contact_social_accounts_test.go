package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func TestContactSocialAccountsPersistReplayAndKeepHistoricalValues(t *testing.T) {
	fixture := newLegalEntityProfileFixture(t)
	legacy := profileAt(t, 1, day(time.March, 1), "CN", "SYN 地址", "SYN 抬头")
	if got := fixture.registerProfile(t, legacy); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatal(got.Outcome())
	}
	command := profileAt(t, 2, day(time.June, 1), "CN", "SYN 地址", "SYN 抬头")
	contact, err := domain.NewLegalEntityContactWithDetails("SYN 联络人", "", "", domain.LegalEntityContactDetails{SocialAccounts: []domain.ContactSocialAccount{{Platform: "微信", Account: "SYN-WX-01"}, {Platform: "WhatsApp", Account: "SYN-WA-01"}}})
	if err != nil {
		t.Fatal(err)
	}
	command.Contacts = []domain.LegalEntityContact{contact}
	if got := fixture.registerProfile(t, command); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatal(got.Outcome())
	}
	if got := fixture.registerProfile(t, command); got.Outcome() != application.LegalEntityProfileAlreadyRegistered {
		t.Fatal(got.Outcome())
	}
	current, _ := fixture.resolveAt(t, day(time.June, 1)).Resolved()
	accounts := current.Content().Contacts()[0].Details().SocialAccounts
	if len(accounts) != 2 || accounts[0].Account != "SYN-WX-01" || accounts[1].Platform != "WhatsApp" {
		t.Fatalf("解析丢账号：%+v", accounts)
	}
	history, err := fixture.profiles.ListLegalEntityProfileRevisions(t.Context(), command.Tenant, command.Entity)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || len(history[1].Contacts[0].SocialAccounts) != 2 {
		t.Fatalf("历史丢账号：%+v", history)
	}
	old, _ := fixture.resolveAt(t, day(time.May, 1)).Resolved()
	if old.Revision() != 1 {
		t.Fatal("未来账号改写旧时点")
	}
	changed := command
	other, _ := domain.NewLegalEntityContactWithDetails("SYN 联络人", "", "", domain.LegalEntityContactDetails{SocialAccounts: []domain.ContactSocialAccount{{Platform: "微信", Account: "SYN-CHANGED"}}})
	changed.Contacts = []domain.LegalEntityContact{other}
	if got := fixture.registerProfile(t, changed); got.Outcome() != application.LegalEntityProfileContentConflict {
		t.Fatal("改账号未触发内容冲突", got.Outcome())
	}
	if got := fixture.registerProfile(t, legacy); got.Outcome() != application.LegalEntityProfileAlreadyRegistered {
		t.Fatal("旧版重放冲突", got.Outcome())
	}
}
