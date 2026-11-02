package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func TestLegalEntityProfileDetailsPersistReplayConflictAndKeepHistory(t *testing.T) {
	fixture := newLegalEntityProfileFixture(t)
	legacy := profileAt(t, 1, day(time.March, 1), "CN", "SYN 注册地址", "SYN 开票抬头")
	if got := fixture.registerProfile(t, legacy); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("旧形状登记：%+v", got)
	}
	if got := fixture.registerProfile(t, legacy); got.Outcome() != application.LegalEntityProfileAlreadyRegistered {
		t.Fatalf("旧形状重放：%+v", got)
	}
	command := profileAt(t, 2, day(time.June, 1), "CN", "SYN 注册地址", "SYN 开票抬头")
	representative, err := domain.NewLegalEntityRepresentative("RESPONSIBLE_PERSON", "SYN 登记负责人")
	if err != nil {
		t.Fatal(err)
	}
	office, err := domain.NewRegisteredAddress(pcValue(t, domain.NewRegistrationCountryCode, "SG"), []string{"SYN 办公地址"})
	if err != nil {
		t.Fatal(err)
	}
	mail, err := domain.NewRegisteredAddress(pcValue(t, domain.NewRegistrationCountryCode, "CN"), []string{"SYN 通信地址"})
	if err != nil {
		t.Fatal(err)
	}
	command.Supplement = domain.LegalEntityProfileSupplement{Representative: &representative, OfficeAddress: &office, CorrespondenceAddress: &mail}
	contact, err := domain.NewLegalEntityContactWithDetails("SYN 财务联系人", "finance@example.invalid", "SYN-PHONE-01", domain.LegalEntityContactDetails{Department: "SYN 财务部", Position: "SYN 财务经理", Purposes: []domain.ContactPurpose{domain.ContactFinance, domain.ContactBusiness}, PrimaryFor: []domain.ContactPurpose{domain.ContactFinance}})
	if err != nil {
		t.Fatal(err)
	}
	command.Contacts = []domain.LegalEntityContact{contact}
	if got := fixture.registerProfile(t, command); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("扩展登记：%s %v", got.Outcome(), got.Cause())
	}
	if got := fixture.registerProfile(t, command); got.Outcome() != application.LegalEntityProfileAlreadyRegistered {
		t.Fatalf("扩展重放：%s %v", got.Outcome(), got.Cause())
	}
	old, _ := fixture.resolveAt(t, day(time.May, 1)).Resolved()
	if old.Revision() != 1 || old.Content().Supplement().Representative != nil {
		t.Fatal("未来资料改写旧时点")
	}
	current, _ := fixture.resolveAt(t, day(time.June, 1)).Resolved()
	supplement := current.Content().Supplement()
	if current.Revision() != 2 || supplement.Representative == nil || supplement.Representative.Name() != "SYN 登记负责人" || supplement.OfficeAddress.Country().String() != "SG" || supplement.CorrespondenceAddress.Lines()[0] != "SYN 通信地址" {
		t.Fatalf("读回扩展资料：%+v", supplement)
	}
	details := current.Content().Contacts()[0].Details()
	if details.Department != "SYN 财务部" || details.Position != "SYN 财务经理" || len(details.Purposes) != 2 || len(details.PrimaryFor) != 1 || details.PrimaryFor[0] != domain.ContactFinance {
		t.Fatalf("联系人字段丢失：%+v", details)
	}
	history, err := fixture.profiles.ListLegalEntityProfileRevisions(t.Context(), command.Tenant, command.Entity)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Representative != nil || history[1].Representative.Name != "SYN 登记负责人" || history[1].OfficeAddress.Country != "SG" || history[1].Contacts[0].Department != "SYN 财务部" || len(history[1].Contacts[0].PrimaryFor) != 1 {
		t.Fatalf("历史读回字段丢失：%+v", history)
	}
	changed := command
	changed.Contacts = legacy.Contacts
	if got := fixture.registerProfile(t, changed); got.Outcome() != application.LegalEntityProfileContentConflict {
		t.Fatalf("同修订改联系人应冲突：%s", got.Outcome())
	}
	changed = command
	changed.Supplement = domain.LegalEntityProfileSupplement{}
	if got := fixture.registerProfile(t, changed); got.Outcome() != application.LegalEntityProfileContentConflict {
		t.Fatalf("同修订改扩展资料应冲突：%s", got.Outcome())
	}
	changed = command
	changed.Revision = 3
	changed.Contacts = []domain.LegalEntityContact{contact, contact}
	if got := fixture.registerProfile(t, changed); got.Outcome() != application.LegalEntityProfileNotAccepted {
		t.Fatalf("主要联系人冲突应不受理：%s", got.Outcome())
	}
	history, err = fixture.profiles.ListLegalEntityProfileRevisions(t.Context(), command.Tenant, command.Entity)
	if err != nil || len(history) != 2 {
		t.Fatalf("拒收或冲突改写历史：%d %v", len(history), err)
	}
}
