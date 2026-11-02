package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func TestProfileContactPurposesGuardPrimaryAndKeepImmutableCopies(t *testing.T) {
	details := domain.LegalEntityContactDetails{Department: "SYN 财务部", Position: "SYN 经理", Purposes: []domain.ContactPurpose{domain.ContactOperations, domain.ContactFinance}, PrimaryFor: []domain.ContactPurpose{domain.ContactFinance}}
	contact, err := domain.NewLegalEntityContactWithDetails("SYN 联系人", "syn@example.invalid", "", details)
	if err != nil {
		t.Fatal(err)
	}
	details.Purposes[0] = domain.ContactGeneral
	if got := contact.Details(); got.Purposes[0] != domain.ContactFinance || got.Purposes[1] != domain.ContactOperations {
		t.Fatalf("用途排序或输入隔离失效：%+v", got)
	}
	returned := contact.Details()
	returned.PrimaryFor[0] = domain.ContactGeneral
	if contact.Details().PrimaryFor[0] != domain.ContactFinance {
		t.Fatal("读回泄漏可变切片")
	}
	for _, invalid := range []domain.LegalEntityContactDetails{
		{Purposes: []domain.ContactPurpose{"UNKNOWN"}},
		{Purposes: []domain.ContactPurpose{domain.ContactFinance, domain.ContactFinance}},
		{PrimaryFor: []domain.ContactPurpose{domain.ContactFinance}},
	} {
		if _, err := domain.NewLegalEntityContactWithDetails("SYN", "", "", invalid); err == nil {
			t.Fatal("接收了非法联系用途")
		}
	}
	base := profileContent(t, "SYN 注册地址", nil)
	if _, err := domain.NewLegalEntityProfileContent(base.Address(), nil, nil, []domain.LegalEntityContact{contact, contact}); err == nil {
		t.Fatal("同一用途接收了多个主要联系人")
	}
	secondary, _ := domain.NewLegalEntityContactWithDetails("SYN 备选联系人", "", "", domain.LegalEntityContactDetails{Purposes: []domain.ContactPurpose{domain.ContactFinance}})
	if _, err := domain.NewLegalEntityProfileContent(base.Address(), nil, nil, []domain.LegalEntityContact{contact, secondary}); err != nil {
		t.Fatal(err)
	}
}

func TestProfileSupplementDoesNotImplyInvoicingOrOverwriteAnEffectiveRevision(t *testing.T) {
	base := profileContent(t, "SYN 注册地址", nil)
	representative, err := domain.NewLegalEntityRepresentative("LEGAL_REPRESENTATIVE", "SYN 代表人")
	if err != nil {
		t.Fatal(err)
	}
	office := base.Address()
	supplement := domain.LegalEntityProfileSupplement{Representative: &representative, OfficeAddress: &office}
	content, err := domain.NewLegalEntityProfileContentWithSupplement(base.Address(), nil, nil, nil, supplement)
	if err != nil {
		t.Fatal(err)
	}
	if _, has := content.Invoicing(); has {
		t.Fatal("扩展资料不构成开票资料")
	}
	*supplement.Representative = domain.LegalEntityRepresentative{}
	copy := content.Supplement()
	*copy.OfficeAddress = domain.RegisteredAddress{}
	if content.Supplement().Representative.Name() != "SYN 代表人" || content.Supplement().OfficeAddress.Country().String() == "" {
		t.Fatal("扩展资料泄漏可变指针")
	}
	for _, kind := range []string{"", "BUSINESS_OWNER"} {
		if _, err := domain.NewLegalEntityRepresentative(kind, "SYN"); err == nil {
			t.Fatal("接收了无效代表人类型")
		}
	}
	if _, err := domain.NewLegalEntityRepresentative("RESPONSIBLE_PERSON", " "); err == nil {
		t.Fatal("接收了空姓名")
	}
	if _, err := domain.NewLegalEntityProfileContentWithSupplement(base.Address(), nil, nil, nil, domain.LegalEntityProfileSupplement{OfficeAddress: &domain.RegisteredAddress{}}); err == nil {
		t.Fatal("接收了零值地址")
	}
	first := profileRevision(t, 1, profileJanuary, base)
	future := profileRevision(t, 2, profileMarch, content)
	entity := profileEntity(t, profileJanuary, time.Time{})
	before, err := domain.ResolveLegalEntityProfile(entity, []domain.LegalEntityProfileRevision{first, future}, profileFebruary)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := before.EffectiveRevision()
	if old.Revision() != 1 || old.Content().Supplement().Representative != nil {
		t.Fatal("未来资料提前参与解析")
	}
	after, err := domain.ResolveLegalEntityProfile(entity, []domain.LegalEntityProfileRevision{first, future}, profileMarch)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := after.EffectiveRevision()
	if current.Revision() != 2 || current.Content().Supplement().Representative.Name() != "SYN 代表人" {
		t.Fatal("生效边界未切到新资料")
	}
}
