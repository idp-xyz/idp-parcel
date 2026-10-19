package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证供应商协议版本点读口（票 demo-chain-gaps/22）：按（租户、对象、版本标签）
// 交壳与正文；没有这一版、壳在正文缺、壳未生效、壳已收尾各落各的在场组合；正文挂在未生效壳上是坏数据；
// 租户与对象类别都是身份的一部分。

func loadVersion(
	t *testing.T,
	view ports.SupplierAgreementVersionView,
	tenant, object, label string,
) (ports.SupplierAgreementVersionRead, error) {
	t.Helper()
	return view.LoadSupplierAgreementVersion(t.Context(),
		pcTenant(t, tenant),
		pcValue(t, domain.NewCommercialObjectID, object),
		pcValue(t, domain.NewCommercialVersionLabel, label))
}

// plannedSupplierAgreement 是一份已发布而未取效的协议壳：发布用例在生效边界未开时就这样入册（已计划生效）。
func plannedSupplierAgreement(t *testing.T, objectID, label, digest string) domain.CommercialVersion {
	t.Helper()
	effective := effectiveVersionOfKind(t, domain.SupplierAgreementObject, objectID, label, digest)
	approval, _ := effective.ApprovalBasis()
	publishedAt, _ := effective.PublishedAt()
	planned, err := domain.RehydrateCommercialVersion(domain.RehydrateCommercialVersionSpec{
		TenantID:      effective.Tenant(),
		Kind:          effective.Kind(),
		ObjectID:      effective.ObjectID(),
		Version:       effective.Version(),
		Scope:         effective.Scope(),
		ContentDigest: effective.ContentDigest(),
		Effective:     effective.Effective(),
		Status:        domain.CommercialVersionPublished,
		Approval:      approval,
		PublishedAt:   publishedAt,
	})
	if err != nil {
		t.Fatalf("重建已计划生效的协议壳：%v", err)
	}
	return planned
}

// saveBodyRow 只写正文行，不过发布用例：用来造发布用例本身写不出的那几种库面状态。
func saveBodyRow(
	t *testing.T,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	transactor bentoapp.Transactor,
	agreement domain.SupplierAgreement,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.SaveSupplierAgreement(txCtx, agreement)
		return err
	})
}

// Covers: 票 demo-chain-gaps/22 完成判据「按（租户、对象、版本标签）答版本壳与正文」——已生效的壳带正文交回，
// 正文挂回同一版本，供应商与责任法人原样在场。
func TestAnEffectiveSupplierAgreementIsReadBackByObjectAndLabelWithItsBody(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-1", "v1", "digest-a1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSaveSupplierAgreement(t, transactor, ctx, repository,
		supplierAgreementOn(t, version, "supplier-1", "plan-buy-1"))

	read, err := loadVersion(t, contents, "tenant-1", "agreement-1", "v1")
	if err != nil {
		t.Fatalf("点读：%v", err)
	}
	if !read.HasVersion || !read.HasAgreement {
		t.Fatalf("在场 = 壳 %v 正文 %v，want 都在", read.HasVersion, read.HasAgreement)
	}
	if !read.Version.SameVersionAs(version) || read.Version.Status() != domain.CommercialVersionEffective {
		t.Fatalf("交回的壳不是登记的那一版：%#v", read.Version)
	}
	if !read.Agreement.Version().SameVersionAs(version) {
		t.Fatal("正文挂回了另一个版本")
	}
	if read.Agreement.Supplier().String() != "supplier-1" || read.Agreement.LegalEntity().String() != "legal-1" {
		t.Fatalf("正文被改动了：%#v", read.Agreement)
	}
}

// Covers: 从未发布的那一版答「没有」而不是 error；同号的别类对象、他租户的同号协议都不算这一版（ADR-0003）。
func TestAVersionNeverPublishedInThisTenantAsThisKindIsAbsent(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	// 同对象号同版本标签的客户合同：类别是身份的一部分，点读供应商协议不得命中它。
	mustSaveVersion(t, transactor, ctx, repository,
		effectiveVersionOfKind(t, domain.CustomerContractObject, "agreement-1", "v1", "digest-c1"))
	theirs := policyVersionInTenant(t, "tenant-2", domain.SupplierAgreementObject, "agreement-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveSupplierAgreement(t, transactor, ctx, repository,
		supplierAgreementOn(t, theirs, "supplier-2", "plan-buy-9"))

	for _, label := range []string{"v1", "v2"} {
		read, err := loadVersion(t, contents, "tenant-1", "agreement-1", label)
		if err != nil {
			t.Fatalf("%s：从未发布应答没有而不是 error：%v", label, err)
		}
		if read.HasVersion || read.HasAgreement {
			t.Fatalf("%s：在场 = 壳 %v 正文 %v，want 都不在", label, read.HasVersion, read.HasAgreement)
		}
	}
}

// Covers: 壳可先入册、正文随发布登记——已生效的壳没有正文时只交壳，不是 error，也不是「没有这一版」。
func TestAnEffectiveShellWithoutItsBodyIsHandedOverAlone(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-bare", "v1", "digest-ab")
	mustSaveVersion(t, transactor, ctx, repository, version)

	read, err := loadVersion(t, contents, "tenant-1", "agreement-bare", "v1")
	if err != nil {
		t.Fatalf("点读：%v", err)
	}
	if !read.HasVersion || read.HasAgreement {
		t.Fatalf("在场 = 壳 %v 正文 %v，want 只有壳", read.HasVersion, read.HasAgreement)
	}
	if read.Version.Status() != domain.CommercialVersionEffective {
		t.Fatalf("壳的位置 = %s", read.Version.Status())
	}
}

// Covers: 本口不判可用与否——已计划生效的壳原样交出它的位置，由消费方去问；不折成「没有」。
func TestAPlannedShellIsHandedOverWithItsLifecyclePosition(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	mustSaveVersion(t, transactor, ctx, repository, plannedSupplierAgreement(t, "agreement-later", "v1", "digest-al"))

	read, err := loadVersion(t, contents, "tenant-1", "agreement-later", "v1")
	if err != nil {
		t.Fatalf("点读：%v", err)
	}
	if !read.HasVersion || read.HasAgreement {
		t.Fatalf("在场 = 壳 %v 正文 %v，want 只有壳", read.HasVersion, read.HasAgreement)
	}
	if read.Version.Status() != domain.CommercialVersionPublished {
		t.Fatalf("壳的位置 = %s，want PUBLISHED", read.Version.Status())
	}
}

// Covers: 已收尾的壳上生效期间登下的正文是历史——只交壳，位置照实；领域不在收尾版本上立正文。
func TestARetiredShellIsHandedOverWithoutItsHistoricalBody(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	effective := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-old", "v1", "digest-ao")
	retired, err := effective.Retire(
		pcValue(t, domain.NewRetirementReference, "retirement-1"), effectiveAtRow.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("退役：%v", err)
	}
	mustSaveVersion(t, transactor, ctx, repository, retired)
	saveBodyRow(t, ctx, repository, transactor, supplierAgreementOn(t, effective, "supplier-1", "plan-buy-1"))

	read, err := loadVersion(t, contents, "tenant-1", "agreement-old", "v1")
	if err != nil {
		t.Fatalf("点读：%v", err)
	}
	if !read.HasVersion || read.HasAgreement {
		t.Fatalf("在场 = 壳 %v 正文 %v，want 只有壳", read.HasVersion, read.HasAgreement)
	}
	if read.Version.Status() != domain.CommercialVersionRetired {
		t.Fatalf("壳的位置 = %s，want RETIRED", read.Version.Status())
	}
}

// Covers: 正文挂在已计划生效的壳上是坏数据（发布用例写不出这一行）——走 error，不折成任何一格在场组合。
func TestABodyOnAPlannedShellIsBadDataNotAnAnswer(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	mustSaveVersion(t, transactor, ctx, repository, plannedSupplierAgreement(t, "agreement-odd", "v1", "digest-ao"))
	effective := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-odd", "v1", "digest-ao")
	saveBodyRow(t, ctx, repository, transactor, supplierAgreementOn(t, effective, "supplier-1", "plan-buy-1"))

	read, err := loadVersion(t, contents, "tenant-1", "agreement-odd", "v1")
	if err == nil {
		t.Fatalf("坏数据答成了在场组合：壳 %v 正文 %v", read.HasVersion, read.HasAgreement)
	}
	if read.HasVersion || read.HasAgreement {
		t.Fatal("报错时仍交出了内容")
	}
}
