package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// firstOccurrence 是一份首版 V1 的发生项：修订门只认本处长出的 V 加正整数那一串。
func firstOccurrence(t *testing.T) domain.TransportChargeOccurrence {
	t.Helper()
	spec := occurrenceSpec(t, "TCO-REVISION-1", "journey-original")
	spec.Validity = domain.FirstOccurrenceValidity()
	occurrence, err := domain.FormTransportChargeOccurrence(spec)
	if err != nil {
		t.Fatal(err)
	}
	return occurrence
}

// Covers: AT-TF-311 的领域半边（ADR-0232 决定四）——失效从链头长出下一版：版本由前一版派生、回指它、标失效，依据与修订者是这一次
// 决定的；内容原样带过去，原版一字不动。已失效的链头再失效答已失效。
func TestInvalidatingTheHeadGrowsTheNextVersionAndLeavesTheOriginal(t *testing.T) {
	head := firstOccurrence(t)
	basis := mustValue(t, domain.NewOccurrenceBasisReference, "SYN-INVALIDATION-EVIDENCE-01")
	by := mustValue(t, domain.NewRegistrantReference, "https://id.syn.example/dex#SYN-OPERATOR-01")
	at := occurredAt.Add(48 * time.Hour)

	invalidated, err := domain.InvalidateChargeOccurrence(head, basis, at, by)
	if err != nil {
		t.Fatal(err)
	}
	corrects, revised := invalidated.Corrects()
	kind, revisionBasis, revisedAt, _ := invalidated.Revision()
	reviser, present := invalidated.RevisedBy()
	if invalidated.Validity().String() != "V2" || !revised || corrects != head.Validity() || kind != domain.OccurrenceInvalidated ||
		revisionBasis != basis || !revisedAt.Equal(at) || !present || reviser != by || !invalidated.Invalidated() {
		t.Fatalf("失效版本 = %+v", invalidated)
	}
	if !invalidated.SameFactContentAs(head) || head.Invalidated() || head.Validity().String() != "V1" {
		t.Fatal("失效改了内容或动了原版")
	}
	if _, err := domain.InvalidateChargeOccurrence(invalidated, basis, at, by); !errors.Is(err, domain.ErrChargeOccurrenceAlreadyInvalidated) {
		t.Fatalf("已失效再失效 err = %v", err)
	}
	if _, err := domain.InvalidateChargeOccurrence(head, basis, at, domain.RegistrantReference{}); err == nil {
		t.Fatal("没有修订者的失效被放行")
	}
}

// Covers: AT-TF-312 的领域半边（ADR-0232 决定五）——失效之后凭同一件事实重新成立：从失效链头长出替代版本，内容取这一次的判断
// （采购上下文可以不同），回指失效版、修订者是这一次的登记者；链头不是失效版不能这么长，身份维不同也不能。
func TestAnInvalidatedOccurrenceIsReestablishedFromItsHeadWithTheNewJudgment(t *testing.T) {
	head := firstOccurrence(t)
	by := mustValue(t, domain.NewRegistrantReference, "https://id.syn.example/dex#SYN-OPERATOR-01")
	at := occurredAt.Add(48 * time.Hour)
	invalidated, err := domain.InvalidateChargeOccurrence(head, mustValue(t, domain.NewOccurrenceBasisReference, "SYN-EVIDENCE"), at, by)
	if err != nil {
		t.Fatal(err)
	}

	spec := occurrenceSpec(t, "TCO-REVISION-1", "journey-original")
	spec.Validity = domain.FirstOccurrenceValidity()
	spec.Provider = mustValue(t, domain.NewServiceProviderReference, "partner-2")
	spec.Agreement = mustValue(t, domain.NewAgreementSnapshotReference, "agreement-snapshot-2")
	formed, err := domain.FormTransportChargeOccurrence(spec)
	if err != nil {
		t.Fatal(err)
	}
	later := at.Add(time.Hour)
	reestablished, err := domain.ReestablishChargeOccurrence(invalidated, formed, later)
	if err != nil {
		t.Fatal(err)
	}
	corrects, _ := reestablished.Corrects()
	kind, basis, revisedAt, _ := reestablished.Revision()
	reviser, _ := reestablished.RevisedBy()
	if reestablished.Validity().String() != "V3" || corrects != invalidated.Validity() || kind != domain.OccurrenceSuperseded ||
		basis != formed.FactBasis() || !revisedAt.Equal(later) || reviser != formed.Condition().RegisteredBy() ||
		reestablished.Invalidated() || reestablished.Provider().String() != "partner-2" || !reestablished.SameFactContentAs(formed) {
		t.Fatalf("替代版本 = %+v", reestablished)
	}
	if _, err := domain.ReestablishChargeOccurrence(head, formed, later); !errors.Is(err, domain.ErrChargeOccurrenceNotInvalidated) {
		t.Fatalf("从仍有效的链头长替代版本 err = %v", err)
	}
	other := occurrenceSpec(t, "TCO-REVISION-2", "journey-original")
	other.Validity = domain.FirstOccurrenceValidity()
	stranger, err := domain.FormTransportChargeOccurrence(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.ReestablishChargeOccurrence(invalidated, stranger, later); err == nil {
		t.Fatal("另一个身份的发生项从这条链上长了出来")
	}
}

func TestTheNextValidityIsDerivedOnlyFromTheVersionsGrownHere(t *testing.T) {
	next, err := domain.NextOccurrenceValidity(domain.FirstOccurrenceValidity())
	if err != nil || next.String() != "V2" {
		t.Fatalf("V1 之后 = %q, %v", next, err)
	}
	for _, foreign := range []string{"occurrence/v1", "V0", "V01", "Vx", "v2"} {
		if _, err := domain.NextOccurrenceValidity(mustValue(t, domain.NewOccurrenceValidityVersion, foreign)); err == nil {
			t.Fatalf("%q 也派生出了下一版", foreign)
		}
	}
}

// 重建门：修订者只随修订版本出现。
func TestTheRevisedByRidesOnlyOnARevisedVersion(t *testing.T) {
	head := firstOccurrence(t)
	spec := domain.RehydrateChargeOccurrenceSpec{
		TenantID: head.TenantID(), Occurrence: head.Occurrence(), Journey: head.Journey(), LegalEntity: head.LegalEntity(),
		Provider: head.Provider(), Agreement: head.Agreement(), Reason: head.Reason(), FactBasis: head.FactBasis(), Scope: head.Scope(),
		Members: head.Members(), OccurredAt: head.OccurredAt(), Validity: head.Validity(), Condition: head.Condition(),
		RevisedBy: mustValue(t, domain.NewRegistrantReference, "https://id.syn.example/dex#SYN-OPERATOR-01"),
	}
	spec.Quantity, spec.Unit = head.Quantity()
	if _, err := domain.RehydrateChargeOccurrence(spec); err == nil {
		t.Fatal("首版带着修订者重建成功")
	}
}
