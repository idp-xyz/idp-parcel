package registrationjson_test

import (
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件证译装两半：正——载荷每一格原样到命令（保真，不规范化、不补默认）；反——形状坏、词表外、
// 有构造门的标识空白，一律在入库前拒并指名差哪格。`AdoptFact` 的答案格与更正的链头判断在应用层已证（ADR-0137
// 决定四），这里不碰库。夹具币种取 ISO 4217 测试码 `XTS`，不用任何流通币的真码。

const adoptDocument = `{
	"tenantId": "SYN-T1",
	"factRef": "SYN-FACT-1",
	"sourceRef": "SYN-SOURCE-BANK-1",
	"payerRef": "SYN-PAYER-1",
	"kind": "RECEIPT_CONFIRMED",
	"currency": "XTS",
	"amountMinor": 8000,
	"version": "SYN-FACT-1/v1",
	"occurredAt": "2026-09-01T08:00:00Z"
}`

// Covers: 判据 (4) 正例——首版载荷逐格到 AdoptFundsFactCommand；kind 按封闭词表译回领域常量；付款人原样带过去
// （本上下文只保留不判断，是不是空白由用例 factFrom 那一格决定）。
func TestExternalFundsFactFromJSONCarriesEveryFieldWithFidelity(t *testing.T) {
	command, err := registrationjson.ExternalFundsFactFromJSON([]byte(adoptDocument))
	if err != nil {
		t.Fatalf("译装被拒：%v", err)
	}
	if command.TenantID.String() != "SYN-T1" {
		t.Fatalf("tenant = %q", command.TenantID)
	}
	if command.Fact != "SYN-FACT-1" || command.Source != "SYN-SOURCE-BANK-1" || command.Payer != "SYN-PAYER-1" {
		t.Fatalf("引用失真：fact=%q source=%q payer=%q", command.Fact, command.Source, command.Payer)
	}
	if command.Kind != domain.FundsReceiptConfirmed {
		t.Fatalf("kind = %v，要 RECEIPT_CONFIRMED 译回的领域常量", command.Kind)
	}
	if command.Currency != "XTS" || command.AmountMinor != 8000 || command.Version != "SYN-FACT-1/v1" {
		t.Fatalf("内容失真：currency=%q amount=%d version=%q", command.Currency, command.AmountMinor, command.Version)
	}
	if want := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC); !command.OccurredAt.Equal(want) {
		t.Fatalf("occurredAt = %s，要 %s", command.OccurredAt, want)
	}
}

// Covers: 付款人可缺席——来源未提供是事实的一个诚实状态，不是缺格（domain.FundsPayerReference 头注）；缺席时命令上是空串。
func TestExternalFundsFactFromJSONLetsThePayerBeAbsent(t *testing.T) {
	command, err := registrationjson.ExternalFundsFactFromJSON([]byte(`{
		"tenantId": "SYN-T1", "factRef": "SYN-FACT-1", "sourceRef": "SYN-SOURCE-BANK-1",
		"kind": "PAYMENT_FAILED", "currency": "XTS", "amountMinor": 8000,
		"version": "SYN-FACT-1/v1", "occurredAt": "2026-09-01T08:00:00Z"
	}`))
	if err != nil {
		t.Fatalf("无付款人的载荷被拒：%v", err)
	}
	if command.Payer != "" || command.Kind != domain.FundsPaymentFailed {
		t.Fatalf("payer=%q kind=%v", command.Payer, command.Kind)
	}
}

// Covers: 判据 (4) 反例——未知字段拒（打错的键静默丢弃会让操作员以为登进去的比实际多）；kind 词表外或缺席拒（没有
// 「缺席即某种意思」的格，与 CC 协作事项那格不同）；有构造门的标识空白在这里就拒并点名字段。
func TestExternalFundsFactFromJSONRejectsBadShapeVocabularyAndBlankIdentifiers(t *testing.T) {
	cases := map[string]struct {
		raw   string
		names string
	}{
		"未知字段":     {strings.Replace(adoptDocument, `"payerRef"`, `"payer"`, 1), "payer"},
		"kind 词表外": {strings.Replace(adoptDocument, `"RECEIPT_CONFIRMED"`, `"RECEIVED"`, 1), "kind"},
		"kind 缺席":  {strings.Replace(adoptDocument, `"kind": "RECEIPT_CONFIRMED",`, ``, 1), "kind"},
		"租户空白":     {strings.Replace(adoptDocument, `"SYN-T1"`, `"  "`, 1), "tenant"},
		"事实引用空白":   {strings.Replace(adoptDocument, `"factRef": "SYN-FACT-1"`, `"factRef": ""`, 1), "fact"},
		"来源引用空白":   {strings.Replace(adoptDocument, `"SYN-SOURCE-BANK-1"`, `" "`, 1), "source"},
		"版本空白":     {strings.Replace(adoptDocument, `"version": "SYN-FACT-1/v1"`, `"version": ""`, 1), "version"},
		"币种空白":     {strings.Replace(adoptDocument, `"XTS"`, `""`, 1), "currency"},
		"付款人给了但空白": {strings.Replace(adoptDocument, `"SYN-PAYER-1"`, `"  "`, 1), "payer"},
	}
	for name, test := range cases {
		_, err := registrationjson.ExternalFundsFactFromJSON([]byte(test.raw))
		if err == nil {
			t.Fatalf("%s：译装没拒", name)
		}
		if !strings.Contains(strings.ToLower(err.Error()), test.names) {
			t.Fatalf("%s：错误文本 %q 没点名 %q", name, err.Error(), test.names)
		}
	}
}

const correctionDocument = `{
	"tenantId": "SYN-T1",
	"factRef": "SYN-FACT-1",
	"corrects": "SYN-FACT-1/v1",
	"version": "SYN-FACT-1/v2",
	"amountMinor": 9000,
	"correctedAt": "2026-09-02T08:00:00Z"
}`

// Covers: 判据 (4) 正例——更正载荷逐格到 CorrectFundsFactCommand；来源 / 付款人 / 种类 / 币种 / 发生时刻不在载荷上
// （用例从链头照抄，命令上不接——CorrectFundsFactCommand 头注），载荷若带它们按未知字段拒（下一例）。
func TestExternalFundsFactCorrectionFromJSONCarriesEveryFieldWithFidelity(t *testing.T) {
	command, err := registrationjson.ExternalFundsFactCorrectionFromJSON([]byte(correctionDocument))
	if err != nil {
		t.Fatalf("译装被拒：%v", err)
	}
	if command.TenantID.String() != "SYN-T1" || command.Fact != "SYN-FACT-1" {
		t.Fatalf("身份失真：tenant=%q fact=%q", command.TenantID, command.Fact)
	}
	if command.Corrects != "SYN-FACT-1/v1" || command.Version != "SYN-FACT-1/v2" || command.AmountMinor != 9000 {
		t.Fatalf("内容失真：corrects=%q version=%q amount=%d", command.Corrects, command.Version, command.AmountMinor)
	}
	if want := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC); !command.CorrectedAt.Equal(want) {
		t.Fatalf("correctedAt = %s，要 %s", command.CorrectedAt, want)
	}
}

// Covers: 判据 (4) 反例——更正载荷带首版才有的格（来源、币种）按未知字段拒：外部更正改的是金额，其余若也变了那是另一条
// 事实，不是更正；回指与新版本空白在这里就拒。「回指非链头」「回指自己」「非正金额」不在此拒——那是用例的`未受理`格。
func TestExternalFundsFactCorrectionFromJSONRejectsForeignFieldsAndBlankVersions(t *testing.T) {
	cases := map[string]struct {
		raw   string
		names string
	}{
		"带来源引用":  {strings.Replace(correctionDocument, `"corrects"`, `"sourceRef": "SYN-SOURCE-BANK-1", "corrects"`, 1), "sourceref"},
		"带币种":    {strings.Replace(correctionDocument, `"corrects"`, `"currency": "XTS", "corrects"`, 1), "currency"},
		"回指空白":   {strings.Replace(correctionDocument, `"corrects": "SYN-FACT-1/v1"`, `"corrects": " "`, 1), "corrects"},
		"新版本空白":  {strings.Replace(correctionDocument, `"version": "SYN-FACT-1/v2"`, `"version": ""`, 1), "version"},
		"事实引用空白": {strings.Replace(correctionDocument, `"factRef": "SYN-FACT-1"`, `"factRef": ""`, 1), "fact"},
	}
	for name, test := range cases {
		_, err := registrationjson.ExternalFundsFactCorrectionFromJSON([]byte(test.raw))
		if err == nil {
			t.Fatalf("%s：译装没拒", name)
		}
		if !strings.Contains(strings.ToLower(err.Error()), test.names) {
			t.Fatalf("%s：错误文本 %q 没点名 %q", name, err.Error(), test.names)
		}
	}
}
