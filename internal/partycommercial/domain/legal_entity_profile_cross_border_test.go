package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证 ADR-0359 决定三、四在领域里的门：收款账户四格必填、币种只锁 ISO 4217 形状、同修订内（币种，账号）不重复、
// 次序即内容、读回不泄漏可变切片；多语言抬头挂在主抬头上、语言标签锁 BCP 47 形状、同一份开票资料里语言不重复（主抬头的
// 语言算在内）；两组明细缺席即这一版不带，不构成开票资料，也不改`资料不全`的判据。

func bankAccount(label, currency, number string) domain.LegalEntityBankAccount {
	return domain.LegalEntityBankAccount{Label: label, HolderName: "SYN 户名", BankName: "SYN 开户行", AccountNumber: number, Currency: currency}
}

// Covers: AT-PC-078——收款账户经构造门去首尾空白、必填四格缺一拒、币种非 ISO 4217 形状拒、同（币种，账号）拒；同账号不同币种
// 是两个账户；集合保留登记次序；交回的拷贝改不到内容。
func TestBankAccountsAreGuardedKeptInOrderAndCopiedOut(t *testing.T) {
	base := profileContent(t, "SYN 注册地址", nil)
	usd := bankAccount(" 美元收款 ", "USD", " SYN-USD-0001 ")
	cny := bankAccount("人民币对公", "CNY", "SYN-CNY-0001")
	sameNumberOtherCurrency := bankAccount("", "HKD", "SYN-USD-0001")
	content, err := domain.NewLegalEntityProfileContentWithSupplement(base.Address(), nil, nil, nil,
		domain.LegalEntityProfileSupplement{BankAccounts: []domain.LegalEntityBankAccount{usd, cny, sameNumberOtherCurrency}})
	if err != nil {
		t.Fatal(err)
	}
	accounts := content.Supplement().BankAccounts
	if len(accounts) != 3 || accounts[0].Label != "美元收款" || accounts[0].AccountNumber != "SYN-USD-0001" || accounts[1].Currency != "CNY" || accounts[2].Currency != "HKD" {
		t.Fatalf("收款账户次序或裁切不对：%+v", accounts)
	}
	if _, has := content.Invoicing(); has {
		t.Fatal("收款账户不构成开票资料")
	}
	accounts[0].AccountNumber = "改掉"
	if content.Supplement().BankAccounts[0].AccountNumber != "SYN-USD-0001" {
		t.Fatal("读回泄漏可变切片")
	}
	for name, invalid := range map[string][]domain.LegalEntityBankAccount{
		"缺户名":    {{BankName: "SYN 开户行", AccountNumber: "SYN-0001", Currency: "USD"}},
		"缺开户行":   {{HolderName: "SYN 户名", AccountNumber: "SYN-0001", Currency: "USD"}},
		"缺账号":    {{HolderName: "SYN 户名", BankName: "SYN 开户行", AccountNumber: " ", Currency: "USD"}},
		"缺币种":    {bankAccount("", "", "SYN-0001")},
		"小写币种":   {bankAccount("", "usd", "SYN-0001")},
		"四位币种":   {bankAccount("", "USDT", "SYN-0001")},
		"同币种同账号": {bankAccount("甲", "USD", "SYN-0001"), bankAccount("乙", "USD", " SYN-0001 ")},
	} {
		if _, err := domain.NewLegalEntityBankAccounts(invalid); err == nil {
			t.Fatalf("%s：接收了非法收款账户", name)
		}
		if _, err := domain.NewLegalEntityProfileContentWithSupplement(base.Address(), nil, nil, nil, domain.LegalEntityProfileSupplement{BankAccounts: invalid}); err == nil {
			t.Fatalf("%s：资料正文接收了非法收款账户", name)
		}
	}
	if accounts, err := domain.NewLegalEntityBankAccounts(nil); err != nil || accounts != nil {
		t.Fatalf("缺席应交回 nil：%v %v", accounts, err)
	}
	if accounts, err := domain.NewLegalEntityBankAccounts([]domain.LegalEntityBankAccount{}); err != nil || accounts != nil {
		t.Fatalf("空集合与缺席同义：%v %v", accounts, err)
	}
}

// Covers: AT-PC-078——币种目录门只在写入边界：形状合法而目录里没有的码在领域构造门放行，经 BankAccountCurrenciesInCatalogue 才拒。
func TestBankAccountCurrencyCatalogueGateIsSeparateFromTheShapeGate(t *testing.T) {
	outside := []domain.LegalEntityBankAccount{bankAccount("", "XXX", "SYN-0001")}
	accounts, err := domain.NewLegalEntityBankAccounts(outside)
	if err != nil {
		t.Fatalf("形状合法的目录外币种应过构造门：%v", err)
	}
	if err := domain.BankAccountCurrenciesInCatalogue(accounts); err == nil {
		t.Fatal("目录外币种过了目录门")
	}
	inside, _ := domain.NewLegalEntityBankAccounts([]domain.LegalEntityBankAccount{bankAccount("", "USD", "SYN-0001"), bankAccount("", "CNY", "SYN-0002")})
	if err := domain.BankAccountCurrenciesInCatalogue(inside); err != nil {
		t.Fatalf("目录内币种被拒：%v", err)
	}
}

// Covers: AT-PC-078——多语言抬头：主抬头语言可空；语言标签锁 BCP 47 形状、去首尾空白；同一份开票资料里语言不重复（含主抬头
// 的语言，不区分大小写）；抬头空白拒；次序即内容；没有主抬头的开票资料造不出来。
func TestLocalizedInvoiceTitlesHangOnTheMainTitleAndKeepLanguagesUnique(t *testing.T) {
	title, err := domain.NewInvoiceTitle("合成运营方一号（演示）")
	if err != nil {
		t.Fatal(err)
	}
	details, err := domain.NewInvoicingDetailsWithLocalizedTitles(title, " zh-Hans ", []domain.LocalizedInvoiceTitle{
		{Language: "en", Title: "SYN Operator One (Demo)"}, {Language: "zh-Hant", Title: "合成營運方一號（演示）"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if details.TitleLanguage() != "zh-Hans" || len(details.LocalizedTitles()) != 2 || details.LocalizedTitles()[0].Language != "en" || details.LocalizedTitles()[1].Title != "合成營運方一號（演示）" {
		t.Fatalf("多语言抬头读回不对：%q %+v", details.TitleLanguage(), details.LocalizedTitles())
	}
	copied := details.LocalizedTitles()
	copied[0].Title = "改掉"
	if details.LocalizedTitles()[0].Title != "SYN Operator One (Demo)" {
		t.Fatal("读回泄漏可变切片")
	}
	plain, err := domain.NewInvoicingDetailsWithLocalizedTitles(title, "", nil)
	if err != nil || plain.TitleLanguage() != "" || plain.LocalizedTitles() != nil {
		t.Fatalf("只有主抬头的开票资料应与旧形状同义：%q %v %v", plain.TitleLanguage(), plain.LocalizedTitles(), err)
	}
	for name, invalid := range map[string]struct {
		language string
		titles   []domain.LocalizedInvoiceTitle
	}{
		"主抬头语言形状不对": {"zh_Hans", nil},
		"主抬头语言单字母":  {"z", nil},
		"翻译语言形状不对":  {"", []domain.LocalizedInvoiceTitle{{Language: "en-", Title: "SYN"}}},
		"翻译语言为空":    {"", []domain.LocalizedInvoiceTitle{{Language: " ", Title: "SYN"}}},
		"翻译抬头空白":    {"", []domain.LocalizedInvoiceTitle{{Language: "en", Title: " "}}},
		"翻译之间语言重复":  {"", []domain.LocalizedInvoiceTitle{{Language: "en", Title: "SYN A"}, {Language: "EN", Title: "SYN B"}}},
		"翻译撞主抬头语言":  {"zh-Hans", []domain.LocalizedInvoiceTitle{{Language: "zh-hans", Title: "SYN"}}},
	} {
		if _, err := domain.NewInvoicingDetailsWithLocalizedTitles(title, invalid.language, invalid.titles); err == nil {
			t.Fatalf("%s：接收了非法多语言抬头", name)
		}
	}
	if _, err := domain.NewInvoicingDetailsWithLocalizedTitles(domain.InvoiceTitle{}, "en", nil); err == nil {
		t.Fatal("没有主抬头的开票资料被造出来了")
	}
}

// Covers: AT-PC-078——两组明细随完整修订保存：带它们的修订在生效前不参与解析，生效后按时点切换；旧修订读作缺席，不回填；
// 带多语言抬头与收款账户的修订缺主抬头时照旧`资料不全`。
func TestCrossBorderDetailsFollowTheRevisionAndDoNotChangeTheInvoicingVerdict(t *testing.T) {
	base := profileContent(t, "SYN 注册地址", nil)
	title, _ := domain.NewInvoiceTitle("SYN 抬头")
	details, err := domain.NewInvoicingDetailsWithLocalizedTitles(title, "zh-Hans", []domain.LocalizedInvoiceTitle{{Language: "en", Title: "SYN Title"}})
	if err != nil {
		t.Fatal(err)
	}
	extended, err := domain.NewLegalEntityProfileContentWithSupplement(base.Address(), nil, &details, nil,
		domain.LegalEntityProfileSupplement{BankAccounts: []domain.LegalEntityBankAccount{bankAccount("人民币对公", "CNY", "SYN-CNY-0001")}})
	if err != nil {
		t.Fatal(err)
	}
	first := profileRevision(t, 1, profileJanuary, base)
	second := profileRevision(t, 2, profileMarch, extended)
	entity := profileEntity(t, profileJanuary, time.Time{})
	before, err := domain.ResolveLegalEntityProfile(entity, []domain.LegalEntityProfileRevision{first, second}, profileFebruary)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := before.EffectiveRevision()
	if old.Revision() != 1 || old.Content().Supplement().BankAccounts != nil {
		t.Fatal("未来资料的收款账户提前参与解析")
	}
	if _, has := old.Content().Invoicing(); has {
		t.Fatal("旧修订不该被回填开票资料")
	}
	after, err := domain.ResolveLegalEntityProfile(entity, []domain.LegalEntityProfileRevision{first, second}, profileMarch)
	if err != nil {
		t.Fatal(err)
	}
	current, ok := after.Resolved()
	if !ok || current.Revision() != 2 {
		t.Fatalf("生效边界未切到带收款账户的修订：%s", after.Outcome())
	}
	invoicing, _ := current.Content().Invoicing()
	if invoicing.TitleLanguage() != "zh-Hans" || len(invoicing.LocalizedTitles()) != 1 || len(current.Content().Supplement().BankAccounts) != 1 {
		t.Fatalf("新修订的跨境明细丢失：%+v %+v", invoicing, current.Content().Supplement())
	}

	bare, err := domain.NewLegalEntityProfileContentWithSupplement(base.Address(), nil, nil, nil,
		domain.LegalEntityProfileSupplement{BankAccounts: []domain.LegalEntityBankAccount{bankAccount("", "USD", "SYN-USD-0001")}})
	if err != nil {
		t.Fatal(err)
	}
	incomplete, err := domain.ResolveLegalEntityProfile(entity, []domain.LegalEntityProfileRevision{profileRevision(t, 1, profileJanuary, bare)}, profileMarch)
	if err != nil {
		t.Fatal(err)
	}
	if incomplete.Outcome() != domain.LegalEntityProfileIncomplete || incomplete.IncompleteCause() != domain.LegalEntityProfileNoInvoicingDetails {
		t.Fatalf("收款账户不替代开票抬头：%s / %s", incomplete.Outcome(), incomplete.IncompleteCause())
	}
}
