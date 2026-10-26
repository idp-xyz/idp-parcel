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
