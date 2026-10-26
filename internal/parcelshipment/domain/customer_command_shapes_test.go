package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
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

// Covers: AT-PS-140——资料修订口的摘要走 PAM-1：前缀是 PAM-1；指名（原提交标识）、范围、基准、意图、原因、适用时间与内容任一不同摘要就不同；
// 要素内容由提交口同一个 AddressElementsOf 挑出，显式清空与缺席内容都不带内容。
func TestCustomerSourceDataAmendmentDigestIsItsOwnShapeAndCarriesEveryNaming(t *testing.T) {
	request := mustShipmentRequestID(t, "SYN-REQ-1")
	scope, err := NewShipmentScopedSourceData(request, DeliveryPlaceDataGroup())
	if err != nil {
		t.Fatal(err)
	}
	reason, err := NewAmendmentReasonReference("SYN-REASON-1")
	if err != nil {
		t.Fatal(err)
	}
	postal, err := NewCanonicalContentEntry(AddressElementEntryName(DeliveryPlaceDataGroup(), PostalCodeElement), "SYN-POSTAL-1")
	if err != nil {
		t.Fatal(err)
	}
	base := SourceDataAmendmentPayloadSpec{
		OriginalAccessRequestID: "SYN-ACCESS-1", Scope: scope, Basis: NewSupplementOnAcceptanceBaseline(),
		Intent: CorrectionIntent, Reason: reason, Entries: []CanonicalContentEntry{postal},
	}
	first, err := CanonicalizeCustomerSourceDataAmendment(base)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.Digest().String(), CustomerSourceDataAmendmentShapeVersion+":") {
		t.Fatalf("摘要 %q 不带 PAM-1 前缀", first.Digest())
	}
	if got, ok := first.Content().AddressElements(); !ok {
		t.Fatal("地址内容缺席")
	} else if value, declared := got.PostalCode(); !declared || value != "SYN-POSTAL-1" {
		t.Fatalf("邮编 = %q, %v", value, declared)
	}
	variant := func(change func(*SourceDataAmendmentPayloadSpec)) string {
		spec := base
		change(&spec)
		canonical, err := CanonicalizeCustomerSourceDataAmendment(spec)
		if err != nil {
			t.Fatal(err)
		}
		return canonical.Digest().String()
	}
	prior, _ := NewSourceDataVersionID("SYN-PRIOR-1")
	basisOf, _ := NewAmendmentOfVersion(prior)
	otherReason, _ := NewAmendmentReasonReference("SYN-REASON-2")
	otherScope, _ := NewShipmentScopedSourceData(request, SenderPlaceDataGroup())
	otherPostal, _ := NewCanonicalContentEntry(AddressElementEntryName(DeliveryPlaceDataGroup(), PostalCodeElement), "SYN-POSTAL-2")
	for name, changed := range map[string]string{
		"原提交标识": variant(func(s *SourceDataAmendmentPayloadSpec) { s.OriginalAccessRequestID = "SYN-ACCESS-2" }),
		"范围":    variant(func(s *SourceDataAmendmentPayloadSpec) { s.Scope = otherScope }),
		"基准":    variant(func(s *SourceDataAmendmentPayloadSpec) { s.Basis = basisOf }),
		"意图":    variant(func(s *SourceDataAmendmentPayloadSpec) { s.Intent = SupplementIntent }),
		"原因":    variant(func(s *SourceDataAmendmentPayloadSpec) { s.Reason = otherReason }),
		"适用时间":  variant(func(s *SourceDataAmendmentPayloadSpec) { s.EffectiveAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }),
		"内容":    variant(func(s *SourceDataAmendmentPayloadSpec) { s.Entries = []CanonicalContentEntry{otherPostal} }),
	} {
		if changed == first.Digest().String() {
			t.Errorf("改%s摘要不变", name)
		}
	}
	clear := base
	clear.Intent, clear.Entries = ExplicitClearIntent, nil
	cleared, err := CanonicalizeCustomerSourceDataAmendment(clear)
	if err != nil || !cleared.Content().Empty() {
		t.Fatalf("显式清空应不带内容：%v", err)
	}
	if _, err := CanonicalizeCustomerSourceDataAmendment(SourceDataAmendmentPayloadSpec{}); !errors.Is(err, ErrInvalidSubmissionPayload) {
		t.Fatalf("空规格 = %v, want ErrInvalidSubmissionPayload", err)
	}
}
