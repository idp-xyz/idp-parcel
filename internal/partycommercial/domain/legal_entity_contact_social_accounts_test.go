package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func TestContactSocialAccountsValidatePairsAndKeepImmutableCopies(t *testing.T) {
	accounts := []domain.ContactSocialAccount{{Platform: " 微信 ", Account: " SYN-WX-01 "}, {Platform: "Custom", Account: "SYN-02"}, {Platform: "微信", Account: "SYN-WX-02"}}
	contact, err := domain.NewLegalEntityContactWithDetails("SYN 联系人", "", "", domain.LegalEntityContactDetails{SocialAccounts: accounts})
	if err != nil {
		t.Fatal(err)
	}
	accounts[0].Account = "SYN-ALTERED"
	got := contact.Details().SocialAccounts
	if len(got) != 3 || got[0].Platform != "微信" || got[0].Account != "SYN-WX-01" {
		t.Fatalf("账号未保全：%+v", got)
	}
	got[0].Platform = "SYN-ALTERED"
	if contact.Details().SocialAccounts[0].Platform != "微信" {
		t.Fatal("返回集合可改写原资料")
	}
	for _, invalid := range [][]domain.ContactSocialAccount{
		{{Platform: "微信"}}, {{Account: "SYN-01"}}, {{Platform: " ", Account: " "}},
		{{Platform: "Telegram", Account: "SYN-01"}, {Platform: " telegram ", Account: " SYN-01 "}},
	} {
		if _, err := domain.NewLegalEntityContactWithDetails("SYN", "", "", domain.LegalEntityContactDetails{SocialAccounts: invalid}); err == nil {
			t.Fatalf("接收不成对或重复账号：%+v", invalid)
		}
	}
}
