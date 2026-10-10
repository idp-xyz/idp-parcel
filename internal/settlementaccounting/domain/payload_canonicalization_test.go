package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func sacInstant() time.Time {
	return time.Date(2026, time.March, 5, 10, 11, 12, 345678901, time.FixedZone("CST", 8*3600))
}

func TestCommandPayloadsRoundTripAsSAC1(t *testing.T) {
	at := sacInstant()
	cases := []struct {
		name     string
		document string
		digest   string
		call     func() ([]byte, string, error)
	}{
		{
			name:     "expected cost",
			document: `{"canonicalization":"SAC-1","face":"FORM_SUPPLIER_EXPECTED_COST","version":"V1","occurrence_id":"OCC","occurrence_reason":"REASON","occurrence_version":"OV","occurred_at":"2026-03-05T02:11:12.345678901Z","fee_item":"FEE","rule_version":"RULE","agreement":"AGR","evaluation":"EV","original_currency":"CNY","original_minor":10,"settlement_currency":"USD","settlement_minor":2,"conversion":"FX"}`,
			digest:   "SAC-1:adfef025f4f193fdc639b4a3a0aede3a02155742268ffc1628dc258fa3bd0314",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeSupplierExpectedCostPayload("V1", "OCC", "REASON", "OV", at, "FEE", "RULE", "AGR", "EV", "CNY", 10, "USD", 2, "FX")
			},
		},
		{
			name:     "publish",
			document: `{"canonicalization":"SAC-1","face":"PUBLISH_STATEMENT","account":"ACC","period":"P","version":"V1","currency":"CNY","declared_total_minor":9,"cut_off_at":"2026-03-05T02:11:12.345678901Z","charges":["C-A","C-B"]}`,
			digest:   "SAC-1:40fe8be8cc6210f15de59e9c0b4a2c48d2911d4419a50b84b14be43beaaabbb9",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizePublishStatementPayload("ACC", "P", "V1", "CNY", 9, at, []string{"C-B", "C-A"})
			},
		},
		{
			name:     "late charge",
			document: `{"canonicalization":"SAC-1","face":"INCLUDE_LATE_CHARGE","number":"N","charge_id":"C","subsequent_period":"P2","included_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:6d638f6d4f66c393bb55c085eeacb23357c8555a588030b0704820e76042c23d",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeLateChargeInclusionPayload("N", "C", "P2", at)
			},
		},
		{
			name:     "adjustment inclusion",
			document: `{"canonicalization":"SAC-1","face":"INCLUDE_ADJUSTMENT","number":"N","adjustment_id":"A","subsequent_period":"P2","included_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:ad90b95b4e61b1f3fb0e80ad5bc9b2582c7c34bce496a7c4bcef342655a51366",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeAdjustmentInclusionPayload("N", "A", "P2", at)
			},
		},
		{
			name:     "dispute",
			document: `{"canonicalization":"SAC-1","face":"OPEN_DISPUTE","number":"N","charge_id":"C","disputed_minor":4,"reason":"WHY","opened_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:7f3e9e2903ccc4aaf51a9b6c0f88117f0af9a8fa5ec7b120354992b676bd072e",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeOpenDisputePayload("N", "C", 4, "WHY", at)
			},
		},
		{
			name:     "allocate",
			document: `{"canonicalization":"SAC-1","face":"ALLOCATE_COST","source":"SRC","source_minor":8,"currency":"CNY","version":"V1","allocated_at":"2026-03-05T02:11:12.345678901Z","portions":[{"target":"T-A","amount_minor":1},{"target":"T-B","amount_minor":2}]}`,
			digest:   "SAC-1:1e07a847fb2f5a9ecf527552ea32a0f5a5908dd28fa029667ac252356ca54dad",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeAllocateCostPayload("SRC", 8, "CNY", "V1", at, []domain.CostPortionContent{{Target: "T-B", AmountMinor: 2}, {Target: "T-A", AmountMinor: 1}})
			},
		},
		{
			name:     "reallocate",
			document: `{"canonicalization":"SAC-1","face":"REALLOCATE_COST","new_version":"V2","allocated_at":"2026-03-05T02:11:12.345678901Z","portions":[{"target":"T-A","amount_minor":1}]}`,
			digest:   "SAC-1:14c6911d86a8e7d88fca899c46b43b7244b9bb1531862f57e4ca2c177e3e00e2",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeReallocateCostPayload("V2", at, []domain.CostPortionContent{{Target: "T-A", AmountMinor: 1}})
			},
		},
		{
			name:     "derive",
			document: `{"canonicalization":"SAC-1","face":"DERIVE_RESULT","currency":"CNY","version":"V1","as_of":"2026-03-05T02:11:12.345678901Z","components":[{"source":"S-A","effect":1,"amount_minor":3}]}`,
			digest:   "SAC-1:6ffd4df32cde37f8dc2ba46f31c0aadec866e61d449ac216d6d2fca5eb1e107a",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeDeriveResultPayload("CNY", "V1", at, []domain.CostComponentContent{{Source: "S-A", Effect: 1, AmountMinor: 3}})
			},
		},
		{
			name:     "rederive",
			document: `{"canonicalization":"SAC-1","face":"REDERIVE_RESULT","new_version":"V2","as_of":"2026-03-05T02:11:12.345678901Z","components":[{"source":"S-A","effect":1,"amount_minor":3}]}`,
			digest:   "SAC-1:3c380920a938a83eb060f350947f7f1cb4b4f3c5099cb8318b0cdca14bff1c26",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeRederiveResultPayload("V2", at, []domain.CostComponentContent{{Source: "S-A", Effect: 1, AmountMinor: 3}})
			},
		},
		{
			name:     "audit",
			document: `{"canonicalization":"SAC-1","face":"AUDIT_BILL_LINE","claim":"CL","version":"V1","line":"L1"}`,
			digest:   "SAC-1:fe1e4daa6caa428c8bf9b965ae54b147504400825e3faf02322c85c767b9a950",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeAuditBillLinePayload("CL", "V1", "L1")
			},
		},
		{
			name:     "credit note",
			document: `{"canonicalization":"SAC-1","face":"FORM_SUPPLIER_CREDIT_NOTE","payable":"PAY","amount_minor":5,"reason":"WHY","issued_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:af4a99cf2abdaa1a7d2878d2b6b0ce34eeedbd89f8f679887e2e504f7c1b209c",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeSupplierCreditNotePayload("PAY", 5, "WHY", at)
			},
		},
		{
			name:     "claim",
			document: `{"canonicalization":"SAC-1","face":"FORM_CLAIM_AMOUNT","kind":1,"claim_item":"ITEM","responsibility":"RESP","legal_entity":"LE","original_charge":"CHG","currency":"CNY","amount_minor":6,"period":"P","formed_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:cfe9d1111977f898ee7f6edae093d36d73499f98f0911c8d1444a249d8d4a23c",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeClaimAmountPayload(1, "ITEM", "RESP", "LE", "CHG", "CNY", 6, "P", at)
			},
		},
		{
			name:     "receivable",
			document: `{"canonicalization":"SAC-1","face":"FORM_RECEIVABLE","matter":"M","responsibility":"RESP","counterparty":"CP","legal_entity":"LE","currency":"CNY","amount_minor":7,"formed_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:0b04f6a1207b9c300351ca1c4fd8270f3ecf1380d5ac3d6c397d1006494bd69a",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeReceivablePayload("M", "RESP", "CP", "LE", "CNY", 7, at)
			},
		},
		{
			name:     "acknowledge",
			document: `{"canonicalization":"SAC-1","face":"ACKNOWLEDGE_RECEIVABLE","receivable":"R","response":"YES","standing":1,"acknowledged_minor":7,"acknowledged_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:b460bc7f981c0d0420152d8317ceed16a12d28a1de840e5e7823ffa3f7ca99a3",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeAcknowledgeReceivablePayload("R", "YES", 1, 7, at)
			},
		},
		{
			name:     "claim adjustment",
			document: `{"canonicalization":"SAC-1","face":"ADJUST_CLAIM_AMOUNT","target_kind":1,"target":"T","reason":2,"basis":"B","direction":1,"currency":"CNY","amount_minor":3,"period":"P","formed_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:6f3230343e3ae97f53dfd1a3a7a0519003386e6b672094a1db179c8903e96efd",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeAdjustClaimAmountPayload(1, "T", 2, "B", 1, "CNY", 3, "P", at)
			},
		},
		{
			name:     "bill",
			document: `{"canonicalization":"SAC-1","face":"RECEIVE_SUPPLIER_BILL","supplier":"SUP","period":"P","currency":"CNY","received_at":"2026-03-05T02:11:12.345678901Z","lines":[{"line":"L-A","fee_item":"FEE","claimed_minor":1},{"line":"L-B","fee_item":"FEE","claimed_minor":2}],"directives":[{"line":"L-A","classification":1,"expected_version":"EV","basis":"B"}]}`,
			digest:   "SAC-1:ce945a0aa27725793ef7a7179df487aa119ab9e58b649f74762082ea79ed6826",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeSupplierBillPayload("SUP", "P", "CNY", at, []domain.SupplierBillLineContent{
					{Line: "L-B", FeeItem: "FEE", ClaimedMinor: 2},
					{Line: "L-A", FeeItem: "FEE", ClaimedMinor: 1},
				}, []domain.SupplierBillDirectiveContent{{Line: "L-A", Classification: 1, ExpectedVersion: "EV", Basis: "B"}})
			},
		},
		{
			name:     "assess",
			document: `{"canonicalization":"SAC-1","face":"ASSESS_ADVANCE","verdict":1,"obligation":"OB","funds_fact":"FF","payer":"PAY","responsibility":"RESP","basis":"B","currency":"CNY","amount_minor":4,"version":"V1","judged_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:03c80c488fdded2600e981fbceb779cad49ad4a6b30dcc582dfbbe5ec9f4b0a6",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeAssessAdvancePayload(1, "OB", "FF", "PAY", "RESP", "B", "CNY", 4, "V1", at)
			},
		},
		{
			name:     "recovery",
			document: `{"canonicalization":"SAC-1","face":"FORM_RECOVERY","assessment":"AS","customer":"CU","account":"ACC","amount_minor":4,"formed_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:fc097a7ae420a979f6b271996d7f6ecdcafd542d1a5c8078a6b93e8767f6f482",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeFormRecoveryPayload("AS", "CU", "ACC", 4, at)
			},
		},
		{
			name:     "recovery adjustment",
			document: `{"canonicalization":"SAC-1","face":"ADJUST_RECOVERY","recovery":"REC","reason":1,"new_basis":"NB","direction":2,"currency":"CNY","amount_minor":4,"period":"P","formed_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:7f5ba6086e2a765839a970fec36a442e400ff6220317a33adb43a991e666385f",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeAdjustRecoveryPayload("REC", 1, "NB", 2, "CNY", 4, "P", at)
			},
		},
		{
			name:     "adopt",
			document: `{"canonicalization":"SAC-1","face":"ADOPT_FUNDS_FACT","source":"SRC","payer":"PAYER","kind":1,"currency":"CNY","amount_minor":9,"version":"V1","occurred_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:9af4959057e39fb93bbf15192300300e757b3f99dfa6936e224465062ef154b0",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeAdoptFundsFactPayload("SRC", " PAYER ", 1, "CNY", 9, "V1", at)
			},
		},
		{
			name:     "correct funds",
			document: `{"canonicalization":"SAC-1","face":"CORRECT_FUNDS_FACT","corrects":"V0","version":"V1","amount_minor":9,"corrected_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:268ccb3d44c27b6af6564afc7caa631e325579b2d584d5832dac67f97080edda",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeCorrectFundsFactPayload("V0", "V1", 9, at)
			},
		},
		{
			name:     "map",
			document: `{"canonicalization":"SAC-1","face":"MAP_FUNDS","fact":"F","target_kind":1,"target":"T","basis":"B","mapped_at":"2026-03-05T02:11:12.345678901Z"}`,
			digest:   "SAC-1:9c6cddd2bcc30fd75fd5bdd917e98ef4f68562a1f0993c931d5b9f6857156d9c",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeMapFundsPayload("F", 1, "T", "B", at)
			},
		},
		{
			name:     "apply",
			document: `{"canonicalization":"SAC-1","face":"APPLY_SETTLEMENT","fact":"F","target_currency":"CNY","basis":"B","applied_at":"2026-03-05T02:11:12.345678901Z","mappings":["M-A","M-B"],"allocations":[{"mapping":"M-B","target_kind":1,"target":"T","direction":2,"amount_minor":3},{"mapping":"M-A","target_kind":1,"target":"T","direction":1,"amount_minor":1}]}`,
			digest:   "SAC-1:249808523a633b20ed08e1160e0ddba56fd52518415f3c5ff67dd58807332907",
			call: func() ([]byte, string, error) {
				return domain.CanonicalizeApplySettlementPayload("F", "CNY", "B", at, []string{"M-B", "M-A"}, []domain.SettlementAllocationContent{
					{Mapping: "M-B", TargetKind: 1, Target: "T", Direction: 2, AmountMinor: 3},
					{Mapping: "M-A", TargetKind: 1, Target: "T", Direction: 1, AmountMinor: 1},
				})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			document, digest, err := tc.call()
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			if string(document) != tc.document {
				t.Fatalf("document\n got %s\nwant %s", document, tc.document)
			}
			if digest != tc.digest {
				t.Fatalf("digest %s, want %s", digest, tc.digest)
			}
			var roundTrip map[string]any
			if err := json.Unmarshal(document, &roundTrip); err != nil {
				t.Fatalf("round trip: %v", err)
			}
		})
	}
}

// Covers: ADR-0014「摘要只在同一规范化版本内可比」——带 SAC-1 前缀的与本次 SAC-1 摘要比，不带前缀的
// 与无版本那一版比，认不出的前缀既不是重放也不是冲突。
func TestStoredDigestIsComparedWithinItsOwnShape(t *testing.T) {
	const current, unversioned = "SAC-1:aa", "bb"
	cases := []struct {
		name   string
		stored string
		want   domain.StoredDigestComparison
	}{
		{"SAC-1 same content", "SAC-1:aa", domain.SamePayload},
		{"SAC-1 different content", "SAC-1:cc", domain.DifferentPayload},
		{"unversioned same content", "bb", domain.SamePayload},
		{"unversioned different content", "cc", domain.DifferentPayload},
		{"an unversioned digest is not read as SAC-1", "aa", domain.DifferentPayload},
		{"a later shape", "SAC-2:aa", domain.UnknownPayloadShape},
		{"another context's shape", "CCC-1:bb", domain.UnknownPayloadShape},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := domain.CompareStoredDigest(testCase.stored, current, unversioned); got != testCase.want {
				t.Fatalf("CompareStoredDigest(%q) = %d, want %d", testCase.stored, got, testCase.want)
			}
		})
	}
}
