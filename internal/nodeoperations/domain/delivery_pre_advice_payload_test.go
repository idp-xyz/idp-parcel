package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证预报两口载荷的规范化形状（ADR-0139 决定八；票 ps-external-mark-relations/09 做什么「载荷形状」）：NOC-1 下各立一面，
// 摘要带形状版本前缀；号码是集合、提交顺序不是内容；时刻折成册上的样子；交付方线索在场与缺席是两份内容；接入请求标识根本不在
// 形状里。夹具全为合成（S 级）。

// 字面量独立于实现：摘要是这串 JSON 的 SHA-256，前缀带形状版本。
const (
	preAdviceSubmissionDocument = `{"canonicalization":"NOC-1","face":"SUBMIT_DELIVERY_PRE_ADVICE","node":"SYN-NODE-SHA-HUB","expected_arrival_at":"2026-10-20T02:00:00.123456Z","numbers":["SYN-PCL-PA-0001","SYN-PCL-PA-0002"],"delivering_party":"SYN-DRIVER-01"}`
	preAdviceWithdrawalDocument = `{"canonicalization":"NOC-1","face":"WITHDRAW_DELIVERY_PRE_ADVICE","pre_advice":"PREADV-SYN-0001"}`
)

func versionedDigestOf(document string) string {
	sum := sha256.Sum256([]byte(document))
	return "NOC-1:" + hex.EncodeToString(sum[:])
}

func preAdvisedNumbersOf(t *testing.T, values ...string) []domain.PreAdvisedParcelNumber {
	t.Helper()
	numbers := make([]domain.PreAdvisedParcelNumber, 0, len(values))
	for _, value := range values {
		number, err := domain.NewPreAdvisedParcelNumber(value)
		if err != nil {
			t.Fatal(err)
		}
		numbers = append(numbers, number)
	}
	return numbers
}

func submissionContent(t *testing.T, deliveringParty string, numbers ...string) ([]byte, string, error) {
	t.Helper()
	node, err := domain.NewNodeReference("SYN-NODE-SHA-HUB")
	if err != nil {
		t.Fatal(err)
	}
	var party domain.DeliveringPartyReference
	if deliveringParty != "" {
		if party, err = domain.NewDeliveringPartyReference(deliveringParty); err != nil {
			t.Fatal(err)
		}
	}
	expectedAt := time.Date(2026, 10, 20, 10, 0, 0, 123456789, time.FixedZone("UTC+8", 8*3600))
	return domain.CanonicalizeDeliveryPreAdviceSubmissionPayload(node, expectedAt, preAdvisedNumbersOf(t, numbers...), party)
}

// Covers: AT-NO-069（载荷半边）——提交口的 NOC-1 文档逐字如上：时刻按 UTC、截到微秒（册的精度），号码排序后编码；摘要是文档的
// SHA-256 带 NOC-1 前缀。号码换个次序交来是同一份内容。
func TestPreAdviceSubmissionCanonicalizesAsNOC1(t *testing.T) {
	document, digest, err := submissionContent(t, "SYN-DRIVER-01", "SYN-PCL-PA-0002", "SYN-PCL-PA-0001")
	if err != nil {
		t.Fatal(err)
	}
	if string(document) != preAdviceSubmissionDocument {
		t.Fatalf("文档 =\n%s\nwant\n%s", document, preAdviceSubmissionDocument)
	}
	if digest != versionedDigestOf(preAdviceSubmissionDocument) {
		t.Fatalf("摘要 = %s", digest)
	}
	_, reordered, err := submissionContent(t, "SYN-DRIVER-01", "SYN-PCL-PA-0001", "SYN-PCL-PA-0002")
	if err != nil || reordered != digest {
		t.Fatalf("换个次序：%s, %v；want 同一摘要", reordered, err)
	}
}

// Covers: AT-NO-069（载荷半边「换内容答来源冲突」的判据）——号码多一个、少一个、换一个，交付方线索在场与缺席，各是另一份内容。
func TestPreAdviceSubmissionDigestSeparatesEveryContentChange(t *testing.T) {
	_, base, err := submissionContent(t, "SYN-DRIVER-01", "SYN-PCL-PA-0001", "SYN-PCL-PA-0002")
	if err != nil {
		t.Fatal(err)
	}
	for name, build := range map[string]func() (string, error){
		"one more number": func() (string, error) {
			_, digest, err := submissionContent(t, "SYN-DRIVER-01", "SYN-PCL-PA-0001", "SYN-PCL-PA-0002", "SYN-PCL-PA-0003")
			return digest, err
		},
		"another number": func() (string, error) {
			_, digest, err := submissionContent(t, "SYN-DRIVER-01", "SYN-PCL-PA-0001", "SYN-PCL-PA-0009")
			return digest, err
		},
		"clue absent": func() (string, error) {
			_, digest, err := submissionContent(t, "", "SYN-PCL-PA-0001", "SYN-PCL-PA-0002")
			return digest, err
		},
		"number with a trailing space": func() (string, error) {
			_, digest, err := submissionContent(t, "SYN-DRIVER-01", "SYN-PCL-PA-0001 ", "SYN-PCL-PA-0002")
			return digest, err
		},
	} {
		digest, err := build()
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if digest == base {
			t.Fatalf("%s：摘要没变", name)
		}
	}
}

// Covers: 形状只收立得起来的内容——节点、时刻缺席或号码为空，规范化即拒，不给一份残缺内容算摘要。
func TestPreAdviceSubmissionCanonicalizationRefusesIncompleteContent(t *testing.T) {
	node, err := domain.NewNodeReference("SYN-NODE-SHA-HUB")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC)
	numbers := preAdvisedNumbersOf(t, "SYN-PCL-PA-0001")
	for name, build := range map[string]func() error{
		"no node": func() error {
			_, _, err := domain.CanonicalizeDeliveryPreAdviceSubmissionPayload(domain.NodeReference{}, at, numbers, domain.DeliveringPartyReference{})
			return err
		},
		"no expected arrival": func() error {
			_, _, err := domain.CanonicalizeDeliveryPreAdviceSubmissionPayload(node, time.Time{}, numbers, domain.DeliveringPartyReference{})
			return err
		},
		"no numbers": func() error {
			_, _, err := domain.CanonicalizeDeliveryPreAdviceSubmissionPayload(node, at, nil, domain.DeliveringPartyReference{})
			return err
		},
		"withdrawal names no pre-advice": func() error {
			_, _, err := domain.CanonicalizeDeliveryPreAdviceWithdrawalPayload(domain.DeliveryPreAdviceID{})
			return err
		},
	} {
		if err := build(); !errors.Is(err, domain.ErrInvalidDeliveryPreAdvice) {
			t.Fatalf("%s：err = %v, want ErrInvalidDeliveryPreAdvice", name, err)
		}
	}
}

// Covers: AT-NO-070（载荷半边）——撤回口的 NOC-1 文档只有它指名的那份预报。
func TestPreAdviceWithdrawalCanonicalizesAsNOC1(t *testing.T) {
	id, err := domain.NewDeliveryPreAdviceID("PREADV-SYN-0001")
	if err != nil {
		t.Fatal(err)
	}
	document, digest, err := domain.CanonicalizeDeliveryPreAdviceWithdrawalPayload(id)
	if err != nil {
		t.Fatal(err)
	}
	if string(document) != preAdviceWithdrawalDocument || digest != versionedDigestOf(preAdviceWithdrawalDocument) {
		t.Fatalf("文档 = %s，摘要 = %s", document, digest)
	}
}

// Covers: ADR-0014「只在同一形状内可比」——预报两口自立起就只写带版本的摘要：同串是同一份，同形状异串是另一份；不带前缀或前缀
// 认不出的已存摘要无从比，答形状认不出，不当冲突、也不当重放。
func TestVersionedDigestComparisonOnlyComparesWithinItsShape(t *testing.T) {
	current := versionedDigestOf(preAdviceWithdrawalDocument)
	other := versionedDigestOf(preAdviceSubmissionDocument)
	for name, testCase := range map[string]struct {
		stored string
		want   domain.StoredDigestComparison
	}{
		"same":            {current, domain.SamePayload},
		"different":       {other, domain.DifferentPayload},
		"unversioned":     {current[len("NOC-1:"):], domain.UnknownPayloadShape},
		"unknown version": {"NOC-9:" + current[len("NOC-1:"):], domain.UnknownPayloadShape},
	} {
		if got := domain.CompareVersionedDigest(testCase.stored, current); got != testCase.want {
			t.Fatalf("%s：%v, want %v", name, got, testCase.want)
		}
	}
}
