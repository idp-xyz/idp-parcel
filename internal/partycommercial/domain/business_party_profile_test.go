package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func partyProfileAddress(t *testing.T, country string, lines ...string) *domain.RegisteredAddress {
	t.Helper()
	address, err := domain.NewRegisteredAddress(commercialValue(t, domain.NewRegistrationCountryCode, country), lines)
	if err != nil {
		t.Fatalf("new address: %v", err)
	}
	return &address
}

func partyProfileRevision(t *testing.T, revision int, effectiveFrom time.Time) domain.BusinessPartyProfileRevision {
	t.Helper()
	content, err := domain.NewBusinessPartyProfileContent(domain.BusinessPartyProfileParts{})
	if err != nil {
		t.Fatal(err)
	}
	built, err := domain.NewBusinessPartyProfileRevision(
		commercialValue(t, domain.NewTenantID, "tenant-1"), commercialValue(t, domain.NewPartyID, "party-1"), revision,
		commercialValue(t, domain.NewBusinessPartyProfileBasisReference, "basis"), effectiveFrom, content,
	)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: 各项都可缺，空正文也成立；其他语言名称的语言标签与重复、空名拒；地址不成立拒；监管登记号与联系人沿用法人资料的规则。
func TestBusinessPartyProfileContentRules(t *testing.T) {
	if _, err := domain.NewBusinessPartyProfileContent(domain.BusinessPartyProfileParts{}); err != nil {
		t.Fatalf("空正文应成立：%v", err)
	}
	for name, names := range map[string][]domain.LocalizedPartyName{
		"语言标签不成立": {{Language: "english!", Name: "Acme"}},
		"语言重复":    {{Language: "en", Name: "Acme"}, {Language: "EN", Name: "ACME"}},
		"名称空白":    {{Language: "en", Name: "  "}},
	} {
		if _, err := domain.NewBusinessPartyProfileContent(domain.BusinessPartyProfileParts{Names: names}); err == nil {
			t.Errorf("%s 应拒", name)
		}
	}
	if _, err := domain.NewBusinessPartyProfileContent(domain.BusinessPartyProfileParts{OfficeAddress: &domain.RegisteredAddress{}}); !errors.Is(err, domain.ErrInvalidLegalEntityProfile) {
		t.Errorf("空地址应拒：%v", err)
	}
	tax, err := domain.NewTaxRegistrationNumber(commercialValue(t, domain.NewRegistrationNumberTypeCode, "SYN-TAX"), commercialValue(t, domain.NewRegistrationNumber, "SYN-TAX-0001"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.NewBusinessPartyProfileContent(domain.BusinessPartyProfileParts{TaxNumbers: []domain.TaxRegistrationNumber{tax, tax}}); !errors.Is(err, domain.ErrInvalidLegalEntityProfile) {
		t.Errorf("同一个号两次应拒：%v", err)
	}
	primary := domain.LegalEntityContactDetails{Purposes: []domain.ContactPurpose{domain.ContactFinance}, PrimaryFor: []domain.ContactPurpose{domain.ContactFinance}}
	first, err := domain.NewLegalEntityContactWithDetails("甲", "", "", primary)
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.NewLegalEntityContactWithDetails("乙", "", "", primary)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.NewBusinessPartyProfileContent(domain.BusinessPartyProfileParts{Contacts: []domain.LegalEntityContact{first, second}}); !errors.Is(err, domain.ErrInvalidLegalEntityProfile) {
		t.Errorf("同一用途两位主要联系人应拒：%v", err)
	}

	content, err := domain.NewBusinessPartyProfileContent(domain.BusinessPartyProfileParts{
		Names:             []domain.LocalizedPartyName{{Language: "en", Name: "Acme Logistics"}},
		RegisteredAddress: partyProfileAddress(t, "CN", "上海市 SYN 路 1 号"),
		Contacts:          []domain.LegalEntityContact{first},
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := content.Parts()
	parts.Names[0].Name = "改过"
	if content.Parts().Names[0].Name != "Acme Logistics" {
		t.Fatal("Parts 交回的应是拷贝")
	}
}

// Covers: 此刻有效的是生效时点不晚于它的修订里修订号最大的一笔；全在未来答没有；同号两次答错误。
func TestEffectiveBusinessPartyProfileRevision(t *testing.T) {
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	dec := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	chain := []domain.BusinessPartyProfileRevision{partyProfileRevision(t, 1, mar), partyProfileRevision(t, 2, jan), partyProfileRevision(t, 3, dec)}
	if revision, found, err := domain.EffectiveBusinessPartyProfileRevision(chain, at); err != nil || !found || revision != 2 {
		t.Fatalf("effective = %d %v %v, want 2", revision, found, err)
	}
	if _, found, err := domain.EffectiveBusinessPartyProfileRevision(chain[2:], at); err != nil || found {
		t.Fatalf("全在未来应答没有：%v %v", found, err)
	}
	if _, _, err := domain.EffectiveBusinessPartyProfileRevision([]domain.BusinessPartyProfileRevision{chain[0], chain[0]}, at); err == nil {
		t.Fatal("同号两次应答错误")
	}
}
