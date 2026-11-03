package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件是 ADR-0359 决定三、四的真库用例：带收款账户与多语言抬头的资料修订经应用层用例落进 0036 + 0044，再经解析与历史
// 两个读口读回；旧修订读作缺席；重放答原修订；同修订改账户答内容冲突；同（币种，账号）与同语言拒收、一行不写；0044 在已有
// 数据的库上可施加，施加前的行照旧读回。

func crossBorderProfile(t *testing.T, revision int, effectiveFrom time.Time) application.RegisterLegalEntityProfileCommand {
	t.Helper()
	command := profileAt(t, revision, effectiveFrom, "CN", "SYN 注册地址", "SYN 抬头")
	details, err := domain.NewInvoicingDetailsWithLocalizedTitles(pcValue(t, domain.NewInvoiceTitle, "合成运营方一号（演示）"), "zh-Hans",
		[]domain.LocalizedInvoiceTitle{{Language: "en", Title: "SYN Operator One (Demo)"}})
	if err != nil {
		t.Fatal(err)
	}
	command.Invoicing = &details
	command.Supplement.BankAccounts = []domain.LegalEntityBankAccount{
		{Label: "人民币对公", HolderName: "SYN 户名", BankName: "SYN 开户行", AccountNumber: "SYN-CNY-0001", SwiftCode: "SYNBCNSH", Currency: "CNY"},
		{Label: "美元收款", HolderName: "SYN 户名", BankName: "SYN 开户行", AccountNumber: "SYN-USD-0001", Currency: "USD", Note: "SYN 备注"},
	}
	return command
}

// Covers: AT-PC-078——带收款账户与双语抬头的修订落库、重放答原修订、同修订改账户答内容冲突；同（币种，账号）与同语言的修订
// 答未受理且一行不写；旧修订按时点与历史读回都不带这两组明细，新修订两个读口都原样读回；0044 的镜像列随之落下。
func TestCrossBorderProfileDetailsPersistReplayConflictAndKeepOldRowsAbsent(t *testing.T) {
	fixture := newLegalEntityProfileFixture(t)
	legacy := profileAt(t, 1, day(time.March, 1), "CN", "SYN 注册地址", "SYN 抬头")
	if got := fixture.registerProfile(t, legacy); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("旧形状登记：%s %v", got.Outcome(), got.Cause())
	}
	command := crossBorderProfile(t, 2, day(time.June, 1))
	if got := fixture.registerProfile(t, command); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("跨境明细登记：%s %v", got.Outcome(), got.Cause())
	}
	if got := fixture.registerProfile(t, command); got.Outcome() != application.LegalEntityProfileAlreadyRegistered {
		t.Fatalf("重放：%s %v", got.Outcome(), got.Cause())
	}

	old, _ := fixture.resolveAt(t, day(time.May, 1)).Resolved()
	oldInvoicing, _ := old.Content().Invoicing()
	if old.Revision() != 1 || old.Content().Supplement().BankAccounts != nil || oldInvoicing.LocalizedTitles() != nil || oldInvoicing.TitleLanguage() != "" {
		t.Fatal("旧修订被回填了跨境明细")
	}
	current, _ := fixture.resolveAt(t, day(time.June, 1)).Resolved()
	invoicing, _ := current.Content().Invoicing()
	accounts := current.Content().Supplement().BankAccounts
	if current.Revision() != 2 || invoicing.TitleLanguage() != "zh-Hans" || len(invoicing.LocalizedTitles()) != 1 || invoicing.LocalizedTitles()[0].Title != "SYN Operator One (Demo)" ||
		len(accounts) != 2 || accounts[0].Label != "人民币对公" || accounts[0].SwiftCode != "SYNBCNSH" || accounts[1].Currency != "USD" || accounts[1].Note != "SYN 备注" {
		t.Fatalf("解析读口丢跨境明细：%+v %+v", invoicing, accounts)
	}
	history, err := fixture.profiles.ListLegalEntityProfileRevisions(t.Context(), command.Tenant, command.Entity)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].BankAccounts != nil || history[0].InvoiceTitles != nil || history[0].InvoiceTitleLanguage != "" ||
		len(history[1].BankAccounts) != 2 || history[1].InvoiceTitleLanguage != "zh-Hans" || len(history[1].InvoiceTitles) != 1 || history[1].BankAccounts[1].AccountNumber != "SYN-USD-0001" {
		t.Fatalf("历史读口的跨境明细不对：%+v", history)
	}
	var bankAccounts, invoiceTitles, language *string
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT bank_accounts::text, invoice_titles::text, invoice_title_language
		   FROM party_commercial.legal_entity_profile_revision
		  WHERE tenant_id = 'tenant-1' AND legal_entity_id = 'le-1' AND revision = 2`).Scan(&bankAccounts, &invoiceTitles, &language); err != nil {
		t.Fatal(err)
	}
	if bankAccounts == nil || invoiceTitles == nil || language == nil || *language != "zh-Hans" {
		t.Fatalf("0044 镜像列没落：%v %v %v", bankAccounts, invoiceTitles, language)
	}
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT bank_accounts::text, invoice_titles::text, invoice_title_language
		   FROM party_commercial.legal_entity_profile_revision
		  WHERE tenant_id = 'tenant-1' AND legal_entity_id = 'le-1' AND revision = 1`).Scan(&bankAccounts, &invoiceTitles, &language); err != nil {
		t.Fatal(err)
	}
	if bankAccounts != nil || invoiceTitles != nil || language != nil {
		t.Fatal("旧修订的镜像列不该有值")
	}

	changed := crossBorderProfile(t, 2, day(time.June, 1))
	changed.Supplement.BankAccounts = changed.Supplement.BankAccounts[:1]
	if got := fixture.registerProfile(t, changed); got.Outcome() != application.LegalEntityProfileContentConflict {
		t.Fatalf("同修订改收款账户应冲突：%s", got.Outcome())
	}
	changed = crossBorderProfile(t, 2, day(time.June, 1))
	plain, _ := domain.NewInvoicingDetails(pcValue(t, domain.NewInvoiceTitle, "合成运营方一号（演示）"))
	changed.Invoicing = &plain
	if got := fixture.registerProfile(t, changed); got.Outcome() != application.LegalEntityProfileContentConflict {
		t.Fatalf("同修订去掉多语言抬头应冲突：%s", got.Outcome())
	}

	// 同（币种，账号）两个账户：正文构造门在用例里拒，答未受理、一行不写。同语言两个抬头走不到用例——开票资料只能经
	// NewInvoicingDetailsWithLocalizedTitles 造出来，重复语言在那道门就被拒（在线口与受控 CLI 的译装用例各证一次）。
	duplicateAccount := crossBorderProfile(t, 3, day(time.July, 1))
	duplicateAccount.Supplement.BankAccounts = append(duplicateAccount.Supplement.BankAccounts, domain.LegalEntityBankAccount{HolderName: "SYN 另一户名", BankName: "SYN 另一开户行", AccountNumber: "SYN-CNY-0001", Currency: "CNY"})
	expectProfileRefused(t, fixture.registerProfile(t, duplicateAccount), "duplicate bank account")
	history, err = fixture.profiles.ListLegalEntityProfileRevisions(t.Context(), command.Tenant, command.Entity)
	if err != nil || len(history) != 2 {
		t.Fatalf("拒收或冲突改写历史：%d %v", len(history), err)
	}
}

// Covers: AT-PC-078——0044 在有数据的库上可施加：先撤掉它的三列、按 0036 的形状落一行，再施加 0044 原文；旧行三列为 NULL、
// 按历史读口读作缺席，此后带收款账户的新修订照常落库。
func TestMigration0044AppliesOnARegisterThatAlreadyHasRows(t *testing.T) {
	fixture := newLegalEntityProfileFixture(t)
	ctx := t.Context()
	exec := func(step, statement string) {
		t.Helper()
		if _, err := fixture.pool.Exec(ctx, statement); err != nil {
			t.Fatalf("%s：%v", step, err)
		}
	}
	exec("撤 0044 的三列", `ALTER TABLE party_commercial.legal_entity_profile_revision
		DROP COLUMN bank_accounts, DROP COLUMN invoice_titles, DROP COLUMN invoice_title_language`)
	exec("按 0036 的形状落一行", `INSERT INTO party_commercial.legal_entity_profile_revision
		(tenant_id, legal_entity_id, revision, basis_ref, effective_from, address_country, address_lines,
		 tax_registration_numbers, invoice_title, contacts, content_digest, snapshot)
		VALUES ('tenant-1', 'le-1', 1, 'SYN-BASIS', '2026-01-01T00:00:00Z', 'CN', '["SYN 地址"]', '[]', 'SYN 抬头', '[]',
		 'legacy-digest', '{"tenant":"tenant-1","legalEntity":"le-1","revision":1,"basis":"SYN-BASIS","effectiveFrom":"2026-01-01T00:00:00Z","address":{"country":"CN","lines":["SYN 地址"]},"taxRegistrationNumbers":[],"invoiceTitle":"SYN 抬头","contacts":[]}')`)

	assets, err := migrations.PartyCommercial()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0044_legal_entity_profile_bank_accounts_and_invoice_titles"
	applied := false
	for _, asset := range assets {
		if asset.Name == target {
			exec("施加 "+target, asset.SQL)
			applied = true
		}
	}
	if !applied {
		t.Fatalf("没有找到迁移 %s", target)
	}

	history, err := fixture.profiles.ListLegalEntityProfileRevisions(ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].BankAccounts != nil || history[0].InvoiceTitles != nil || history[0].InvoiceTitleLanguage != "" || !history[0].HasInvoicing {
		t.Fatalf("施加前的行应读作缺席：%+v", history)
	}
	if got := fixture.registerProfile(t, crossBorderProfile(t, 2, day(time.June, 1))); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("施加后带收款账户的修订：%s %v", got.Outcome(), got.Cause())
	}
	var count int
	if err := fixture.pool.QueryRow(ctx, `SELECT count(*) FROM party_commercial.legal_entity_profile_revision WHERE bank_accounts IS NOT NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("镜像列：%d %v", count, err)
	}
	// 多语言抬头挂在主抬头上：列上的 CHECK 把没主抬头却带翻译的行挡在外面，与快照门同一口径。
	if _, err := fixture.pool.Exec(ctx, `INSERT INTO party_commercial.legal_entity_profile_revision
		(tenant_id, legal_entity_id, revision, basis_ref, effective_from, address_country, address_lines,
		 tax_registration_numbers, invoice_title, contacts, content_digest, snapshot, invoice_titles)
		VALUES ('tenant-1', 'le-1', 9, 'SYN-BASIS', '2026-01-01T00:00:00Z', 'CN', '["SYN 地址"]', '[]', NULL, '[]',
		 'bad-digest', '{}', '[{"language":"en","title":"SYN"}]')`); err == nil {
		t.Fatal("没有主抬头却带多语言抬头的行进了册")
	}
}
