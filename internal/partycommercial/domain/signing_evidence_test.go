package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 签署证据挂在客户合同或供应商协议的版本旁：形状在领域里裁完，更正链也在这里裁——
// 自指、跨版本、同一旧证据被更正第二次，都还没碰到库。

var signingRecordedAt = time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)

func signingFile(t *testing.T) domain.SigningFile {
	t.Helper()
	file, err := domain.NewSigningFile("signed.pdf", strings.Repeat("ab", 32), "EVD-RESTRICTED-01")
	if err != nil {
		t.Fatalf("签署件：%v", err)
	}
	return file
}

func signingEvidence(t *testing.T, id, version string, corrects string) domain.SigningEvidence {
	t.Helper()
	var prior domain.SigningEvidenceID
	if corrects != "" {
		prior = commercialValue(t, domain.NewSigningEvidenceID, corrects)
	}
	evidence, err := domain.NewSigningEvidence(
		commercialValue(t, domain.NewSigningEvidenceID, id),
		commercialValue(t, domain.NewTenantID, "tenant-1"),
		domain.CustomerContractObject,
		commercialValue(t, domain.NewCommercialObjectID, "contract-1"),
		commercialValue(t, domain.NewCommercialVersionLabel, version),
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		"甲方签署人",
		"",
		signingFile(t),
		domain.SigningEvidenceOfflineSigned,
		prior,
		commercialValue(t, domain.NewOperatorSubjectReference, "op-recorder"),
		signingRecordedAt,
	)
	if err != nil {
		t.Fatalf("签署证据 %s：%v", id, err)
	}
	return evidence
}

func TestSigningEvidenceOnlyAcceptsAnOfflineFileBesideAContractOrAgreement(t *testing.T) {
	evidence := signingEvidence(t, "evidence-1", "v1", "")
	if evidence.Source().String() != "OFFLINE_SIGNED" || evidence.File().Name() != "signed.pdf" {
		t.Fatalf("evidence = %s %s", evidence.Source(), evidence.File().Name())
	}
	if _, corrects := evidence.Corrects(); corrects {
		t.Fatal("首登不该指名更正对象")
	}
	if evidence.CounterpartySignatory() != "甲方签署人" || evidence.OurSignatory() != "" {
		t.Fatalf("签署人 = %q / %q", evidence.CounterpartySignatory(), evidence.OurSignatory())
	}

	agreement, err := domain.NewSigningEvidence(
		evidence.ID(), evidence.Tenant(), domain.SupplierAgreementObject, evidence.ObjectID(), evidence.Version(),
		evidence.SignedOn(), "", "我方签署人", evidence.File(), evidence.Source(), domain.SigningEvidenceID{},
		evidence.RecordedBy(), evidence.RecordedAt(),
	)
	if err != nil || agreement.Kind() != domain.SupplierAgreementObject {
		t.Fatalf("供应商协议：%v %s", err, agreement.Kind())
	}

	_, err = domain.NewSigningEvidence(
		evidence.ID(), evidence.Tenant(), domain.CreditPolicyObject, evidence.ObjectID(), evidence.Version(),
		evidence.SignedOn(), "", "", evidence.File(), evidence.Source(), domain.SigningEvidenceID{},
		evidence.RecordedBy(), evidence.RecordedAt(),
	)
	if !errors.Is(err, domain.ErrInvalidSigningEvidence) {
		t.Fatalf("别的类别 err = %v", err)
	}
}

func TestSigningEvidenceRejectsAFileThatIsNotAnExternalReference(t *testing.T) {
	base := signingEvidence(t, "evidence-1", "v1", "")
	cases := []struct {
		name, digest, index string
	}{
		{name: "dir/signed.pdf", digest: strings.Repeat("ab", 32), index: "EVD-1"},
		{name: `dir\signed.pdf`, digest: strings.Repeat("ab", 32), index: "EVD-1"},
		{name: "signed.pdf", digest: strings.Repeat("AB", 32), index: "EVD-1"},
		{name: "signed.pdf", digest: strings.Repeat("ab", 31), index: "EVD-1"},
		{name: "signed.pdf", digest: strings.Repeat("zz", 32), index: "EVD-1"},
		{name: "signed.pdf", digest: strings.Repeat("ab", 32), index: "  "},
		{name: "  ", digest: strings.Repeat("ab", 32), index: "EVD-1"},
	}
	for _, testCase := range cases {
		_, err := domain.NewSigningFile(testCase.name, testCase.digest, testCase.index)
		if !errors.Is(err, domain.ErrInvalidSigningEvidence) {
			t.Fatalf("%q / %q / %q err = %v", testCase.name, testCase.digest, testCase.index, err)
		}
	}
	if _, known := domain.SigningEvidenceSourceNamed("PLATFORM_SIGNED"); known {
		t.Fatal("本票不收电子签章来源")
	}
	_, err := domain.NewSigningEvidence(
		base.ID(), base.Tenant(), base.Kind(), base.ObjectID(), base.Version(),
		base.SignedOn().Add(time.Hour), "", "", base.File(), base.Source(), domain.SigningEvidenceID{},
		base.RecordedBy(), base.RecordedAt(),
	)
	if !errors.Is(err, domain.ErrInvalidSigningEvidence) {
		t.Fatalf("带时刻的签署日 err = %v", err)
	}
}

func TestSigningEvidenceCorrectionChain(t *testing.T) {
	original := signingEvidence(t, "evidence-1", "v1", "")
	_, err := domain.NewSigningEvidence(
		original.ID(), original.Tenant(), original.Kind(), original.ObjectID(), original.Version(),
		original.SignedOn(), "", "", original.File(), original.Source(), original.ID(),
		original.RecordedBy(), original.RecordedAt(),
	)
	if !errors.Is(err, domain.ErrSigningEvidenceCorrectsSelf) {
		t.Fatalf("自指 err = %v", err)
	}

	correction := signingEvidence(t, "evidence-2", "v1", "evidence-1")
	if err := correction.AcceptCorrection([]domain.SigningEvidence{original}); err != nil {
		t.Fatalf("更正同版本：%v", err)
	}
	if err := correction.AcceptCorrection(nil); !errors.Is(err, domain.ErrSigningEvidenceCorrectsUnknown) {
		t.Fatalf("找不到被更正的那条 err = %v", err)
	}

	otherVersion := signingEvidence(t, "evidence-1", "v2", "")
	if err := correction.AcceptCorrection([]domain.SigningEvidence{otherVersion}); !errors.Is(err, domain.ErrSigningEvidenceCrossVersion) {
		t.Fatalf("跨版本 err = %v", err)
	}

	second := signingEvidence(t, "evidence-3", "v1", "evidence-1")
	if err := second.AcceptCorrection([]domain.SigningEvidence{original, correction}); !errors.Is(err, domain.ErrSigningEvidenceAlreadyCorrected) {
		t.Fatalf("再更正 err = %v", err)
	}
	// 更正链可以继续：新的一条更正的是上一条更正，不是已经被更正过的那条。
	follow := signingEvidence(t, "evidence-4", "v1", "evidence-2")
	if err := follow.AcceptCorrection([]domain.SigningEvidence{original, correction}); err != nil {
		t.Fatalf("更正链下一环：%v", err)
	}
}

func TestSigningEvidenceRequirementNamesEachKindSeparately(t *testing.T) {
	rule, err := domain.NewSigningEvidenceRequirement(
		commercialValue(t, domain.NewTenantID, "tenant-1"), true, false,
	)
	if err != nil {
		t.Fatalf("要求：%v", err)
	}
	if !rule.Requires(domain.CustomerContractObject) || rule.Requires(domain.SupplierAgreementObject) {
		t.Fatalf("requires = %v %v", rule.RequiresCustomerContract(), rule.RequiresSupplierAgreement())
	}
	if rule.Requires(domain.PriceRuleObject) {
		t.Fatal("别的类别不该被这道门拦住")
	}
	if _, err := domain.NewSigningEvidenceRequirement(domain.TenantID{}, false, false); !errors.Is(err, domain.ErrInvalidSigningEvidenceRequirement) {
		t.Fatalf("空租户 err = %v", err)
	}
	again, err := domain.NewSigningEvidenceRequirement(rule.Tenant(), true, false)
	if err != nil || !rule.SameContent(again) {
		t.Fatalf("同内容：%v %v", err, again)
	}
	changed, err := domain.NewSigningEvidenceRequirement(rule.Tenant(), true, true)
	if err != nil || rule.SameContent(changed) {
		t.Fatal("两个布尔有一个不同就不是同一份内容")
	}
}
