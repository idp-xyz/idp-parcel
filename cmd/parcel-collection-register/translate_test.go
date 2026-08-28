package main

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

// 打错字段名不得静默变成「没给」：未知字段一律拒收，七种输入无一例外。
func TestEveryInputRefusesUnknownFields(t *testing.T) {
	cases := []struct {
		what  string
		raw   string
		parse func([]byte) error
	}{
		{"代收指令", `{"tenantId":"SYN-T1","instructionRef":"SYN-INSTR-1","parcelRef":"SYN-P1",
			"requirementRef":"SYN-REQ-1","customerRef":"SYN-C1","legalEntityRef":"SYN-E1",
			"currency":"EUR","channelRef":"SYN-CH1","amountMinor":12000,
			"instructedAt":"2026-08-24T00:30:00Z","feeRate":"0.02"}`,
			func(raw []byte) error { _, err := instructionCommandFromJSON(raw); return err }},
		{"分户账开立", `{"tenantId":"SYN-T1","customerRef":"SYN-C1","legalEntityRef":"SYN-E1",
			"currency":"EUR","channelRef":"SYN-CH1","custodyBasisRef":"SYN-COD-V1",
			"openedAt":"2026-08-24T00:00:00Z","balanceMinor":0}`,
			func(raw []byte) error { _, err := subledgerCommandFromJSON(raw); return err }},
		{"代收事实", `{"tenantId":"SYN-T1","factRef":"SYN-F1","instructionRef":"SYN-INSTR-1",
			"sourceLayer":"RECIPIENT_PAYMENT","evidenceRef":"SYN-EV1","currency":"EUR",
			"amountMinor":12000,"occurredAt":"2026-08-24T01:00:00Z","settled":true}`,
			func(raw []byte) error { _, err := factCommandFromJSON(raw); return err }},
		{"差异事项", `{"tenantId":"SYN-T1","discrepancyRef":"SYN-D1","instructionRef":"SYN-INSTR-1",
			"kind":"SHORTFALL","currency":"EUR","amountMinor":2000,"basisRef":"SYN-B1",
			"observedAt":"2026-08-25T00:00:00Z","writeOff":true}`,
			func(raw []byte) error { _, err := discrepancyCommandFromJSON(raw); return err }},
		{"回汇批次", `{"tenantId":"SYN-T1","batchRef":"SYN-BATCH-1","customerRef":"SYN-C1",
			"legalEntityRef":"SYN-E1","currency":"EUR","channelRef":"SYN-CH1",
			"collectedThrough":"2026-08-31T00:00:00Z","formedAt":"2026-08-31T01:00:00Z",
			"remittanceCycle":"MONTHLY"}`,
			func(raw []byte) error { _, err := batchCommandFromJSON(raw); return err }},
		{"汇付主张交出", `{"tenantId":"SYN-T1","batchRef":"SYN-BATCH-1","paidAt":"2026-09-01T00:00:00Z"}`,
			func(raw []byte) error { _, err := handOverCommandFromJSON(raw); return err }},
		{"分户账记账", `{"tenantId":"SYN-T1","postingRef":"SYN-POST-1",
			"fromPosition":"EXTERNAL_SOURCE","toPosition":"IN_TRANSIT_AT_CHANNEL",
			"amountMinor":12000,"basisKind":"COLLECTION_FACT","basisRef":"SYN-F1",
			"postedAt":"2026-08-24T01:10:00Z","currency":"EUR"}`,
			func(raw []byte) error { _, err := postingCommandFromJSON(raw); return err }},
	}
	for _, testCase := range cases {
		if err := testCase.parse([]byte(testCase.raw)); err == nil {
			t.Fatalf("%s：未知字段被静默吃掉", testCase.what)
		}
	}
}

// 记账输入里没有分户账键与币种：键由依据推出，币种取自那本账。多给就是拒。
func TestAPostingInputCarriesNeitherLedgerKeyNorCurrency(t *testing.T) {
	withKey := `{"tenantId":"SYN-T1","postingRef":"SYN-POST-1",
		"fromPosition":"EXTERNAL_SOURCE","toPosition":"IN_TRANSIT_AT_CHANNEL",
		"amountMinor":12000,"basisKind":"COLLECTION_FACT","basisRef":"SYN-F1",
		"postedAt":"2026-08-24T01:10:00Z","customerRef":"SYN-C1"}`
	if _, err := postingCommandFromJSON([]byte(withKey)); err == nil {
		t.Fatal("记账输入带分户账键被接受——记进别人的账就有了一个说得通的入口")
	}
}

// 批次输入不收状态：收了就等于允许输入直接造一个「已交出主张」的批次。
func TestABatchInputCannotDeclareItsOwnState(t *testing.T) {
	withState := `{"tenantId":"SYN-T1","batchRef":"SYN-BATCH-1","customerRef":"SYN-C1",
		"legalEntityRef":"SYN-E1","currency":"EUR","channelRef":"SYN-CH1",
		"collectedThrough":"2026-08-31T00:00:00Z","formedAt":"2026-08-31T01:00:00Z",
		"state":"HANDED_FOR_PAYMENT"}`
	if _, err := batchCommandFromJSON([]byte(withState)); err == nil {
		t.Fatal("批次输入自带状态被接受")
	}

	clean := strings.ReplaceAll(withState, `,
		"state":"HANDED_FOR_PAYMENT"`, "")
	command, err := batchCommandFromJSON([]byte(clean))
	if err != nil {
		t.Fatalf("译装批次：%v", err)
	}
	if command.Batch.State() != domain.BatchCollected {
		t.Fatalf("新批次状态 = %s，要 COLLECTED", command.Batch.State())
	}
}

// 陌生封闭词在译装处就拒，不折成任何一个成员。
func TestUnknownClosedWordsAreRefusedAtTranslation(t *testing.T) {
	badLayer := `{"tenantId":"SYN-T1","factRef":"SYN-F1","instructionRef":"SYN-INSTR-1",
		"sourceLayer":"BANK_TRANSFER","evidenceRef":"SYN-EV1","currency":"EUR",
		"amountMinor":12000,"occurredAt":"2026-08-24T01:00:00Z"}`
	if _, err := factCommandFromJSON([]byte(badLayer)); err == nil {
		t.Fatal("陌生来源层级被折成了某一层——四层不得互相推导这条就断在这里")
	}

	badPosition := `{"tenantId":"SYN-T1","postingRef":"SYN-POST-1",
		"fromPosition":"HELD","toPosition":"IN_TRANSIT_AT_CHANNEL",
		"amountMinor":12000,"basisKind":"COLLECTION_FACT","basisRef":"SYN-F1",
		"postedAt":"2026-08-24T01:10:00Z"}`
	if _, err := postingCommandFromJSON([]byte(badPosition)); err == nil {
		t.Fatal("陌生资金位置被接受")
	}

	badKind := `{"tenantId":"SYN-T1","postingRef":"SYN-POST-1",
		"fromPosition":"EXTERNAL_SOURCE","toPosition":"IN_TRANSIT_AT_CHANNEL",
		"amountMinor":12000,"basisKind":"MANUAL","basisRef":"SYN-F1",
		"postedAt":"2026-08-24T01:10:00Z"}`
	if _, err := postingCommandFromJSON([]byte(badKind)); err == nil {
		t.Fatal("陌生依据种类被接受")
	}

	badDiscrepancy := `{"tenantId":"SYN-T1","discrepancyRef":"SYN-D1",
		"instructionRef":"SYN-INSTR-1","kind":"ROUNDING","currency":"EUR",
		"amountMinor":2000,"basisRef":"SYN-B1","observedAt":"2026-08-25T00:00:00Z"}`
	if _, err := discrepancyCommandFromJSON([]byte(badDiscrepancy)); err == nil {
		t.Fatal("陌生差异方向被接受")
	}
}

// 有构造门的标识与金额在译装处就拒，不代填。
func TestTranslationFillsNothingIn(t *testing.T) {
	blankTenant := `{"tenantId":"","instructionRef":"SYN-INSTR-1","parcelRef":"SYN-P1",
		"requirementRef":"SYN-REQ-1","customerRef":"SYN-C1","legalEntityRef":"SYN-E1",
		"currency":"EUR","channelRef":"SYN-CH1","amountMinor":12000,
		"instructedAt":"2026-08-24T00:30:00Z"}`
	if _, err := instructionCommandFromJSON([]byte(blankTenant)); err == nil {
		t.Fatal("空租户被接受")
	}

	zeroAmount := strings.Replace(blankTenant, `"tenantId":""`, `"tenantId":"SYN-T1"`, 1)
	zeroAmount = strings.Replace(zeroAmount, `"amountMinor":12000`, `"amountMinor":0`, 1)
	if _, err := instructionCommandFromJSON([]byte(zeroAmount)); err == nil {
		t.Fatal("零金额指令被接受")
	}

	noRequirement := strings.Replace(blankTenant, `"tenantId":""`, `"tenantId":"SYN-T1"`, 1)
	noRequirement = strings.Replace(noRequirement, `"requirementRef":"SYN-REQ-1"`, `"requirementRef":""`, 1)
	if _, err := instructionCommandFromJSON([]byte(noRequirement)); err == nil {
		t.Fatal("无服务要求依据的指令被接受")
	}
}
