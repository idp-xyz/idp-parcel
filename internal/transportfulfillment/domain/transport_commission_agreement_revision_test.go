package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

func revisionSpec(t *testing.T, version string, appliesFrom time.Time, basis string) domain.CommissionAgreementRevisionSpec {
	t.Helper()
	successor, err := domain.NewSupplierAgreementVersion("agreement-1", version)
	if err != nil {
		t.Fatal(err)
	}
	return domain.CommissionAgreementRevisionSpec{
		Successor:   successor,
		AppliesFrom: appliesFrom,
		Basis:       mustValue(t, domain.NewCommissionAgreementRevisionBasisReference, basis),
		DecidedBy:   mustValue(t, domain.NewRegistrantReference, "https://id.syn.example#SYN-OPERATOR-01"),
	}
}

func revise(t *testing.T, commission domain.TransportCommission, spec domain.CommissionAgreementRevisionSpec, snapshot string) domain.TransportCommission {
	t.Helper()
	draft, err := commission.DraftAgreementRevision(spec)
	if err != nil {
		t.Fatalf("换版草稿：%v", err)
	}
	revised, _, err := draft.Revise(
		mustValue(t, domain.NewAgreementSnapshotReference, snapshot),
		mustValue(t, domain.NewProcurementLegalEntityReference, "legal-entity-"+snapshot),
		commissionSubmittedAt.Add(30*24*time.Hour),
	)
	if err != nil {
		t.Fatalf("换版：%v", err)
	}
	return revised
}

// Covers: AT-TF-296（ADR-0231 决定四）——事实按业务时间取委托那一刻采购所凭的那一版：起用时刻不晚于它的换版里最后登记的那一版，
// 没有就是提交那一版；起用时刻那一刻本身已归后继。后一次换版起用得更早时连同前一次覆盖的那一段一并改掉。
func TestAgreementTermsAreChosenByTheFactsBusinessTime(t *testing.T) {
	commission, err := submitCommission(t, commissionSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	cut := commissionSubmittedAt.Add(48 * time.Hour)
	revised := revise(t, commission, revisionSpec(t, "v2", cut, "SYN-DECISION-REV-1"), "agreement-snapshot-2")

	before := revised.AgreementTermsAt(cut.Add(-time.Second))
	if version, recorded := before.Version(); before.Revision() != 1 || !recorded || version.Version() != "v1" ||
		before.Snapshot().String() != "agreement-snapshot-1" || before.LegalEntity().String() != "legal-entity-1" {
		t.Fatalf("起用之前 = %+v, want 提交那一版", before)
	}
	at := revised.AgreementTermsAt(cut)
	if version, _ := at.Version(); at.Revision() != 2 || version.Version() != "v2" ||
		at.Snapshot().String() != "agreement-snapshot-2" || at.LegalEntity().String() != "legal-entity-agreement-snapshot-2" {
		t.Fatalf("起用那一刻 = %+v, want 第 2 版", at)
	}
	if commission.AgreementTermsAt(cut).Revision() != 1 || len(commission.AgreementRevisions()) != 0 {
		t.Fatal("换版改写了原来那一份委托值")
	}
	if revised.Agreement().String() != "agreement-snapshot-1" {
		t.Fatalf("提交那一版的快照被改写成 %s", revised.Agreement())
	}

	earlier := commissionSubmittedAt.Add(time.Hour)
	again := revise(t, revised, revisionSpec(t, "v3", earlier, "SYN-DECISION-REV-2"), "agreement-snapshot-3")
	for _, instant := range []time.Time{earlier, cut, cut.Add(time.Hour)} {
		if terms := again.AgreementTermsAt(instant); terms.Revision() != 3 {
			t.Fatalf("%s 取第 %d 版, want 后登记且起用更早的第 3 版", instant, terms.Revision())
		}
	}
	if terms := again.AgreementTermsAt(earlier.Add(-time.Second)); terms.Revision() != 1 {
		t.Fatalf("两次换版都没起用时取第 %d 版, want 提交那一版", terms.Revision())
	}
	if revisions := again.AgreementRevisions(); len(revisions) != 2 || revisions[0].Number() != 2 || revisions[1].Number() != 3 {
		t.Fatalf("换版串 = %+v", revisions)
	}
}

// Covers: AT-TF-297（ADR-0231 决定三）——换版声明那一半的门：起用早于委托提交、后继换了协议对象、各格缺席各答一格；同一份依据的重放
// 认得出、换了后继或起用时刻就不是同一个决定，换人重报仍是同一个。
func TestARevisionDeclarationIsCheckedBeforeTheAgreementIsAsked(t *testing.T) {
	commission, err := submitCommission(t, commissionSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commission.DraftAgreementRevision(revisionSpec(t, "v2", commissionSubmittedAt.Add(-time.Second), "B")); !errors.Is(err, domain.ErrRevisionAppliesBeforeSubmission) {
		t.Fatalf("起用早于提交 = %v", err)
	}
	other := revisionSpec(t, "v2", commissionSubmittedAt, "B")
	other.Successor, _ = domain.NewSupplierAgreementVersion("agreement-2", "v1")
	if _, err := commission.DraftAgreementRevision(other); !errors.Is(err, domain.ErrRevisionAgreementObjectMismatch) {
		t.Fatalf("换了协议对象 = %v", err)
	}
	blank := revisionSpec(t, "v2", commissionSubmittedAt, "B")
	blank.DecidedBy = domain.RegistrantReference{}
	if _, err := commission.DraftAgreementRevision(blank); !errors.Is(err, domain.ErrInvalidCommissionAgreementRevision) {
		t.Fatalf("缺决定人 = %v", err)
	}

	spec := revisionSpec(t, "v2", commissionSubmittedAt.Add(time.Hour), "SYN-DECISION-REV-1")
	revised := revise(t, commission, spec, "agreement-snapshot-2")
	found, ok := revised.AgreementRevisionBy(spec.Basis)
	if !ok || !found.SameDecisionAs(spec) {
		t.Fatalf("按依据认不出这一次换版：%+v %v", found, ok)
	}
	otherPerson := spec
	otherPerson.DecidedBy = mustValue(t, domain.NewRegistrantReference, "https://id.syn.example#SYN-OPERATOR-02")
	if !found.SameDecisionAs(otherPerson) {
		t.Fatal("换人重报被读成另一个决定")
	}
	moved := spec
	moved.AppliesFrom = spec.AppliesFrom.Add(time.Minute)
	if found.SameDecisionAs(moved) || found.SameDecisionAs(revisionSpec(t, "v3", spec.AppliesFrom, "SYN-DECISION-REV-1")) {
		t.Fatal("换了起用时刻或后继仍被读成同一个决定")
	}
}

// Covers: AT-TF-297（ADR-0231 决定三）——委托提交时没记下协议版本（迁移 TF 0038 之前）时，第一次换版定下协议对象，此后的换版都要同一个对象；
// 读回时整串一起验：号不连续、依据重复、起用早于提交、对象不一的换版串装不回。
func TestARevisionChainIsRehydratedOnlyWhenItHangsTogether(t *testing.T) {
	base := domain.RehydrateTransportCommissionSpec{
		TenantID:       mustValue(t, domain.NewTenantID, "tenant-1"),
		Commission:     mustValue(t, domain.NewTransportCommissionReference, "commission-1"),
		Provider:       mustValue(t, domain.NewServiceProviderReference, "partner-1"),
		Agreement:      mustValue(t, domain.NewAgreementSnapshotReference, "agreement-snapshot-1"),
		LegalEntity:    mustValue(t, domain.NewProcurementLegalEntityReference, "legal-entity-1"),
		Conditions:     mustValue(t, domain.NewConditionsSnapshotReference, "conditions-snapshot-1"),
		Role:           mustValue(t, domain.NewRoleSnapshotReference, "role-snapshot-1"),
		Responsibility: mustValue(t, domain.NewResponsibilitySnapshotReference, "responsibility-snapshot-1"),
		Members:        []domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, "parcel-1")},
		SubmittedAt:    commissionSubmittedAt,
	}
	legacy, err := domain.RehydrateTransportCommission(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, recorded := legacy.AgreementTermsAt(commissionSubmittedAt).Version(); recorded {
		t.Fatal("没记下版本的委托答出了版本")
	}
	first := revise(t, legacy, revisionSpec(t, "v2", commissionSubmittedAt, "B1"), "agreement-snapshot-2")
	other := revisionSpec(t, "v3", commissionSubmittedAt, "B2")
	other.Successor, _ = domain.NewSupplierAgreementVersion("agreement-2", "v3")
	if _, err := first.DraftAgreementRevision(other); !errors.Is(err, domain.ErrRevisionAgreementObjectMismatch) {
		t.Fatalf("第一次换版定下对象之后换对象 = %v", err)
	}

	row := func(number int, object, basis string, appliesFrom time.Time) domain.CommissionAgreementRevision {
		successor, _ := domain.NewSupplierAgreementVersion(object, "v9")
		revision, err := domain.RehydrateCommissionAgreementRevision(domain.RehydrateCommissionAgreementRevisionSpec{
			Number: number, Successor: successor,
			Snapshot:     mustValue(t, domain.NewAgreementSnapshotReference, object+"/v9"),
			LegalEntity:  mustValue(t, domain.NewProcurementLegalEntityReference, "legal-entity-9"),
			AppliesFrom:  appliesFrom,
			Basis:        mustValue(t, domain.NewCommissionAgreementRevisionBasisReference, basis),
			DecidedBy:    mustValue(t, domain.NewRegistrantReference, "issuer#sub"),
			RegisteredAt: commissionSubmittedAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("换版行：%v", err)
		}
		return revision
	}
	good := base
	good.AgreementRevisions = []domain.CommissionAgreementRevision{
		row(2, "agreement-1", "B1", commissionSubmittedAt), row(3, "agreement-1", "B2", commissionSubmittedAt.Add(time.Hour)),
	}
	if rebuilt, err := domain.RehydrateTransportCommission(good); err != nil || rebuilt.AgreementTermsAt(commissionSubmittedAt.Add(time.Hour)).Revision() != 3 {
		t.Fatalf("整串装回 = %v", err)
	}
	for name, chain := range map[string][]domain.CommissionAgreementRevision{
		"号不连续":   {row(3, "agreement-1", "B1", commissionSubmittedAt)},
		"依据重复":   {row(2, "agreement-1", "B1", commissionSubmittedAt), row(3, "agreement-1", "B1", commissionSubmittedAt)},
		"起用早于提交": {row(2, "agreement-1", "B1", commissionSubmittedAt.Add(-time.Second))},
		"对象不一":   {row(2, "agreement-1", "B1", commissionSubmittedAt), row(3, "agreement-2", "B2", commissionSubmittedAt)},
	} {
		bad := base
		bad.AgreementRevisions = chain
		if _, err := domain.RehydrateTransportCommission(bad); !errors.Is(err, domain.ErrInvalidRehydratedCommission) {
			t.Fatalf("%s 的换版串装回了：%v", name, err)
		}
	}
	if _, err := domain.RehydrateCommissionAgreementRevision(domain.RehydrateCommissionAgreementRevisionSpec{Number: 1}); !errors.Is(err, domain.ErrInvalidRehydratedCommission) {
		t.Fatalf("第 1 版当换版行装回：%v", err)
	}
}
