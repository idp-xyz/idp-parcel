package domain

import (
	"errors"
	"strings"
	"testing"
)

// Covers: AT-PS-124——首方客户命令四口各有自己的形状版本名，名互不相同、也不同于提交口的 PSC-1，带版本号后缀（ADR-0139 决定八）。
func TestCustomerCommandShapeVersionsAreDistinct(t *testing.T) {
	seen := map[string]bool{payloadCanonicalizationVersion: true}
	for _, version := range []string{
		CustomerWithdrawalShapeVersion, CustomerParcelCancellationShapeVersion,
		CustomerSourceDataAmendmentShapeVersion, CustomerSupplementShapeVersion,
	} {
		if len(version) < 3 || version[len(version)-2:] != "-1" {
			t.Fatalf("形状版本 %q 不带 -1 后缀", version)
		}
		if seen[version] {
			t.Fatalf("形状版本 %q 与别的口重名", version)
		}
		seen[version] = true
	}
}

// Covers: AT-PS-131——补充口的摘要走 PSU-1：前缀是 PSU-1、不同于同一内容的 PSC-1 摘要；基准不同摘要不同；没带基准即拒。
func TestCustomerSupplementDigestIsItsOwnShapeAndCarriesTheBasis(t *testing.T) {
	reference := mustShipmentRequestID(t, "SYN-CUSTREF-1")
	parcel, err := NewDeclaredParcelID("SYN-PCLREF-1")
	if err != nil {
		t.Fatal(err)
	}
	basisA, err := NewSubmissionVersionID("SYN-VERSION-A")
	if err != nil {
		t.Fatal(err)
	}
	basisB, err := NewSubmissionVersionID("SYN-VERSION-B")
	if err != nil {
		t.Fatal(err)
	}
	spec := SubmissionPayloadSpec{RequestReference: reference, BasisVersion: basisA, DeclaredParcelIDs: []DeclaredParcelID{parcel}}

	supplement, err := CanonicalizeCustomerSupplement(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := supplement.Digest().String(); !strings.HasPrefix(got, CustomerSupplementShapeVersion+":") {
		t.Fatalf("补充摘要 %q 不带 %s 前缀", got, CustomerSupplementShapeVersion)
	}
	submission, err := CanonicalizeSubmission(spec)
	if err != nil {
		t.Fatal(err)
	}
	if supplement.Digest() == submission.Digest() {
		t.Fatal("同一内容的补充摘要与提交摘要相同：两个判重空间会互相当成重放")
	}
	spec.BasisVersion = basisB
	otherBasis, err := CanonicalizeCustomerSupplement(spec)
	if err != nil {
		t.Fatal(err)
	}
	if otherBasis.Digest() == supplement.Digest() {
		t.Fatal("基准不同摘要却相同")
	}
	spec.BasisVersion = SubmissionVersionID{}
	if _, err := CanonicalizeCustomerSupplement(spec); !errors.Is(err, ErrInvalidSubmissionPayload) {
		t.Fatalf("没带基准：err = %v, want ErrInvalidSubmissionPayload", err)
	}
}

func mustShipmentRequestID(t *testing.T, value string) ShipmentRequestID {
	t.Helper()
	id, err := NewShipmentRequestID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Covers: AT-PS-135——撤回口的摘要走 PWD-1：前缀是 PWD-1；原提交标识、委托号（带与不带）、原因任一不同摘要就不同；空白不入摘要；缺原提交标识或原因即拒。
func TestCustomerWithdrawalDigestIsItsOwnShapeAndCarriesTheNames(t *testing.T) {
	reason, err := NewWithdrawalReasonReference("SYN-REASON-1")
	if err != nil {
		t.Fatal(err)
	}
	otherReason, err := NewWithdrawalReasonReference("SYN-REASON-2")
	if err != nil {
		t.Fatal(err)
	}
	base := CustomerWithdrawalSpec{OriginalAccessRequestID: "SYN-ORIG-1", Reason: reason}
	digest, err := CanonicalizeCustomerWithdrawal(base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(digest.String(), CustomerWithdrawalShapeVersion+":") {
		t.Fatalf("摘要 %q 不带 %s 前缀", digest, CustomerWithdrawalShapeVersion)
	}
	padded := base
	padded.OriginalAccessRequestID = " SYN-ORIG-1 "
	if same, _ := CanonicalizeCustomerWithdrawal(padded); same != digest {
		t.Fatal("两端空白进了摘要")
	}
	for name, mutate := range map[string]func(*CustomerWithdrawalSpec){
		"改指原提交": func(spec *CustomerWithdrawalSpec) { spec.OriginalAccessRequestID = "SYN-ORIG-2" },
		"带委托号":  func(spec *CustomerWithdrawalSpec) { spec.ShipmentRequestID = "SHR-1" },
		"改原因":   func(spec *CustomerWithdrawalSpec) { spec.Reason = otherReason },
	} {
		spec := base
		mutate(&spec)
		changed, err := CanonicalizeCustomerWithdrawal(spec)
		if err != nil || changed == digest {
			t.Errorf("%s：摘要未变（err=%v）", name, err)
		}
	}
	if _, err := CanonicalizeCustomerWithdrawal(CustomerWithdrawalSpec{Reason: reason}); !errors.Is(err, ErrInvalidSubmissionPayload) {
		t.Errorf("缺原提交标识：err = %v", err)
	}
	if _, err := CanonicalizeCustomerWithdrawal(CustomerWithdrawalSpec{OriginalAccessRequestID: "X"}); !errors.Is(err, ErrInvalidSubmissionPayload) {
		t.Errorf("缺原因：err = %v", err)
	}
}
