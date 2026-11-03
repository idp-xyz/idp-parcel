package domain

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.idp.xyz/idp-parcel/internal/platform/refdata"
)

// LegalEntityBankAccount 是法人资料上的一个收款账户（ADR-0359 决定三；CONTEXT「法人资料」）。它属 party-commercial：
// 对账单、发票要印的是法人自己的银行账户，与 settlement-accounting 的结算账户（固定责任法人、相对方、方向、币种与
// 结算政策的业务账户）不是一个概念。Currency 是账户自身的属性——这个账户收哪种钱——不是结算币种的决定：结算币种
// 仍由结算政策按责任法人、相对方、合同与费用范围解析（ADR-0145 决定四）。
//
// 字段导出并带 JSON 标签，照 ContactSocialAccount 的模式：它是随完整资料修订保存的多值明细，三个边界（在线口、
// 受控 CLI、快照）都原样搬它，不各抄一份字段清单。
type LegalEntityBankAccount struct {
	Label         string `json:"label,omitempty"`
	HolderName    string `json:"holderName"`
	BankName      string `json:"bankName"`
	AccountNumber string `json:"accountNumber"`
	SwiftCode     string `json:"swiftCode,omitempty"`
	BankAddress   string `json:"bankAddress,omitempty"`
	Currency      string `json:"currency"`
	Note          string `json:"note,omitempty"`
}

// 币种只锁 ISO 4217 的形状：构造门同时是快照重建门，目录随发版变，重建不能因目录更新读不出已在册的行
// （ADR-0345 决定六）。是否在产品内置目录里由写入边界经 BankAccountCurrenciesInCatalogue 另判。
var bankAccountCurrencyShape = regexp.MustCompile(`^[A-Z]{3}$`)

// NewLegalEntityBankAccounts 是收款账户集合的构造门：各格去首尾空白，户名、开户行、账号与币种必填，币种须是
// ISO 4217 形状；同一修订内（币种，账号）不得重复——同一个账号登两种币种是两个账户，登两次同币种才是重复。
// 集合保留登记次序，次序即内容；交回 nil 表示这一版不带收款账户。SWIFT / BIC 与账号不校验形状：各国账号形态
// 各异，拿一条形状去拒会把合法的账号挡在外面。
func NewLegalEntityBankAccounts(values []LegalEntityBankAccount) ([]LegalEntityBankAccount, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := slices.Clone(values)
	for i := range result {
		account := &result[i]
		account.Label = strings.TrimSpace(account.Label)
		account.HolderName = strings.TrimSpace(account.HolderName)
		account.BankName = strings.TrimSpace(account.BankName)
		account.AccountNumber = strings.TrimSpace(account.AccountNumber)
		account.SwiftCode = strings.TrimSpace(account.SwiftCode)
		account.BankAddress = strings.TrimSpace(account.BankAddress)
		account.Currency = strings.TrimSpace(account.Currency)
		account.Note = strings.TrimSpace(account.Note)
		if account.HolderName == "" || account.BankName == "" || account.AccountNumber == "" {
			return nil, fmt.Errorf("%w: bank account needs a holder name, a bank name and an account number", ErrInvalidLegalEntityProfile)
		}
		if !bankAccountCurrencyShape.MatchString(account.Currency) {
			return nil, fmt.Errorf("%w: bank account currency %q is not an ISO 4217 code", ErrInvalidLegalEntityProfile, account.Currency)
		}
		for _, previous := range result[:i] {
			if previous.Currency == account.Currency && previous.AccountNumber == account.AccountNumber {
				return nil, fmt.Errorf("%w: duplicate bank account %s %s", ErrInvalidLegalEntityProfile, account.Currency, account.AccountNumber)
			}
		}
	}
	return result, nil
}

// BankAccountCurrenciesInCatalogue 是新登记入口的币种目录门（ADR-0345 决定五）：收款账户的币种只收产品内置目录里的
// 码。与 NewCatalogueCurrencyCode 同一取舍——门在写入边界，不在构造函数，读回与重放不经此门。
func BankAccountCurrenciesInCatalogue(accounts []LegalEntityBankAccount) error {
	for _, account := range accounts {
		if err := refdata.CheckCurrency(strings.TrimSpace(account.Currency)); err != nil {
			return err
		}
	}
	return nil
}

// LocalizedInvoiceTitle 是开票抬头的一个语言版本（ADR-0359 决定四）：Language 是 BCP 47 语言标签（zh-Hans、en、
// zh-Hant），Title 是该语言的抬头。哪种语言印到单据上由开单方按收件方决定，本上下文只保存。
type LocalizedInvoiceTitle struct {
	Language string `json:"language"`
	Title    string `json:"title"`
}

// 语言标签只锁 BCP 47 的大体形状（主语言子标签加任意多个子标签）：标签是否在 IANA 注册表里不在本上下文判，
// 判了也会随注册表更新而让已在册的行读不回。
var languageTagShape = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$`)

func newLanguageTag(value string) (string, error) {
	tag := strings.TrimSpace(value)
	if !languageTagShape.MatchString(tag) {
		return "", fmt.Errorf("%w: language tag %q is not a BCP 47 tag", ErrInvalidLegalEntityProfile, value)
	}
	return tag, nil
}

// NewInvoicingDetailsWithLocalizedTitles 造带多语言抬头的开票资料。title 仍是主抬头——`资料不全`的判据只看它有没有
// （ADR-0145 决定六，本记录不改）；titleLanguage 标主抬头的语言，可空；titles 是其余语言版本。同一份开票资料里
// 语言不得重复，主抬头的语言也算在内：同一种语言两个抬头，开单方无从选。语言标签按 BCP 47 不区分大小写比对，
// 原样保存。多语言抬头保留登记次序。
func NewInvoicingDetailsWithLocalizedTitles(
	title InvoiceTitle,
	titleLanguage string,
	titles []LocalizedInvoiceTitle,
) (InvoicingDetails, error) {
	details, err := NewInvoicingDetails(title)
	if err != nil {
		return InvoicingDetails{}, err
	}
	seen := make([]string, 0, len(titles)+1)
	if strings.TrimSpace(titleLanguage) != "" {
		tag, err := newLanguageTag(titleLanguage)
		if err != nil {
			return InvoicingDetails{}, err
		}
		details.titleLanguage = tag
		seen = append(seen, tag)
	}
	if len(titles) > 0 {
		details.localizedTitles = make([]LocalizedInvoiceTitle, 0, len(titles))
	}
	for _, localized := range titles {
		tag, err := newLanguageTag(localized.Language)
		if err != nil {
			return InvoicingDetails{}, err
		}
		if strings.TrimSpace(localized.Title) == "" {
			return InvoicingDetails{}, fmt.Errorf("%w: invoice title for %s is blank", ErrInvalidLegalEntityProfile, tag)
		}
		for _, previous := range seen {
			if strings.EqualFold(previous, tag) {
				return InvoicingDetails{}, fmt.Errorf("%w: duplicate invoice title language %s", ErrInvalidLegalEntityProfile, tag)
			}
		}
		seen = append(seen, tag)
		details.localizedTitles = append(details.localizedTitles, LocalizedInvoiceTitle{Language: tag, Title: localized.Title})
	}
	return details, nil
}

// TitleLanguage 交回主抬头的语言标签；空串即未标。
func (details InvoicingDetails) TitleLanguage() string {
	return details.titleLanguage
}

// LocalizedTitles 交回一份拷贝；nil 即这笔修订只有主抬头。
func (details InvoicingDetails) LocalizedTitles() []LocalizedInvoiceTitle {
	return slices.Clone(details.localizedTitles)
}

// checked 把一份外来的开票资料重新过一遍构造门：content 的构造函数收的是指针，调用方可以绕过 New* 直接拼结构体。
func (details InvoicingDetails) checked() (InvoicingDetails, error) {
	return NewInvoicingDetailsWithLocalizedTitles(details.title, details.titleLanguage, details.localizedTitles)
}
