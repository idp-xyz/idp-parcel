package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 生产装配要插进来的就是 PC 的正文读口；编译期钉住，形状不合当场编不过。
var (
	_ pcports.SupplierAgreementVersionView = (*pcpostgres.SupplierAgreementContents)(nil)
	_ tfports.SupplierAgreementSource      = (*adapter.SupplierAgreementSource)(nil)
)

var (
	agreementStartsAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	agreementEndsAt   = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	insideAgreement   = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
)

type versionViewStub struct {
	read  pcports.SupplierAgreementVersionRead
	err   error
	asked []string
}

func (stub *versionViewStub) LoadSupplierAgreementVersion(
	_ context.Context,
	tenant pcdomain.TenantID,
	agreement pcdomain.CommercialObjectID,
	version pcdomain.CommercialVersionLabel,
) (pcports.SupplierAgreementVersionRead, error) {
	stub.asked = append(stub.asked, tenant.String()+" "+agreement.String()+" "+version.String())
	return stub.read, stub.err
}

// effectiveAgreementShell 经重建门造一份已生效的协议壳：tenant-1 / agreement-1 / v1，有界区间。
func effectiveAgreementShell(t *testing.T, object, label string) pcdomain.CommercialVersion {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(agreementStartsAt, agreementEndsAt)
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustBuild(t, pcdomain.NewApprovalReference, "approval-1"),
		mustBuild(t, pcdomain.NewCommercialSourceReference, "source-1"),
		agreementStartsAt.Add(-48*time.Hour))
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustBuild(t, pcdomain.NewTenantID, "tenant-1"),
		Kind:          pcdomain.SupplierAgreementObject,
		ObjectID:      mustBuild(t, pcdomain.NewCommercialObjectID, object),
		Version:       mustBuild(t, pcdomain.NewCommercialVersionLabel, label),
		Scope:         mustBuild(t, pcdomain.NewCommercialScopeReference, "scope-1"),
		ContentDigest: mustBuild(t, pcdomain.NewCommercialContentDigest, "digest-"+object+"-"+label),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   agreementStartsAt.Add(-24 * time.Hour),
		EffectiveAt:   agreementStartsAt,
	})
	if err != nil {
		t.Fatalf("重建协议壳：%v", err)
	}
	return version
}

func agreementBody(t *testing.T, version pcdomain.CommercialVersion, supplier string) pcdomain.SupplierAgreement {
	t.Helper()
	body, err := pcdomain.NewSupplierAgreement(
		version,
		mustBuild(t, pcdomain.NewPartyID, supplier),
		mustBuild(t, pcdomain.NewLegalEntityReference, "LE-1"),
		mustBuild(t, pcdomain.NewCommercialScopeReference, "scope-procurement"),
		mustBuild(t, pcdomain.NewPricingPlanReference, "plan-buy-1"),
		version.Effective(),
	)
	if err != nil {
		t.Fatalf("协议正文：%v", err)
	}
	return body
}

func withBody(t *testing.T, supplier string) pcports.SupplierAgreementVersionRead {
	t.Helper()
	shell := effectiveAgreementShell(t, "agreement-1", "v1")
	return pcports.SupplierAgreementVersionRead{
		Version: shell, HasVersion: true, Agreement: agreementBody(t, shell, supplier), HasAgreement: true,
	}
}

func shellOnlyRead(version pcdomain.CommercialVersion) pcports.SupplierAgreementVersionRead {
	return pcports.SupplierAgreementVersionRead{Version: version, HasVersion: true}
}

func checkThrough(
	t *testing.T,
	stub *versionViewStub,
	provider string,
	at time.Time,
) (tfports.SupplierAgreementCheck, error) {
	t.Helper()
	source, err := adapter.NewSupplierAgreementSource(stub)
	if err != nil {
		t.Fatalf("build source: %v", err)
	}
	agreement, err := tfdomain.NewSupplierAgreementVersion("agreement-1", "v1")
	if err != nil {
		t.Fatalf("agreement version: %v", err)
	}
	return source.CheckSupplierAgreement(t.Context(), tenant(t),
		agreement, mustBuild(t, tfdomain.NewServiceProviderReference, provider), at)
}

func mustCheck(t *testing.T, stub *versionViewStub, provider string, at time.Time) tfports.SupplierAgreementCheck {
	t.Helper()
	check, err := checkThrough(t, stub, provider, at)
	if err != nil {
		t.Fatalf("核对：%v", err)
	}
	return check
}

// Covers: 方案 D02-4 的检查都过——在册格带采购责任法人（取自协议，不由调用方给）与 PC 一处拼出的「对象/版本」快照引用；
// 租户、对象、版本标签原样交给 PC。
func TestARegisteredAgreementHandsOverItsLegalEntityAndSnapshot(t *testing.T) {
	stub := &versionViewStub{read: withBody(t, "supplier-1")}

	check := mustCheck(t, stub, "supplier-1", insideAgreement)

	if check.Standing != tfports.SupplierAgreementRegistered {
		t.Fatalf("standing = %s，want AGREEMENT_REGISTERED", check.Standing)
	}
	if check.LegalEntity.String() != "LE-1" || check.Supplier.String() != "supplier-1" {
		t.Fatalf("法人或供应商 = %q / %q", check.LegalEntity, check.Supplier)
	}
	if want := stub.read.Version.QualifiedLabel().String(); check.Snapshot.String() != want {
		t.Fatalf("快照 = %q，want PC 拼出的 %q", check.Snapshot, want)
	}
	if len(stub.asked) != 1 || stub.asked[0] != "tenant-1 agreement-1 v1" {
		t.Fatalf("问 PC 的键 = %v", stub.asked)
	}
}

// Covers: 方案 D02-4「协议供应商等于服务提供方」——协议在有效期内而供应商不是服务方，答服务方不符并说出协议约定的是谁；不交法人与快照。
func TestAnAgreementWithAnotherSupplierIsAProviderMismatch(t *testing.T) {
	check := mustCheck(t, &versionViewStub{read: withBody(t, "supplier-1")}, "supplier-2", insideAgreement)

	if check.Standing != tfports.SupplierAgreementProviderMismatch {
		t.Fatalf("standing = %s，want PROVIDER_MISMATCH", check.Standing)
	}
	if check.Supplier.String() != "supplier-1" {
		t.Fatalf("协议约定的供应商 = %q", check.Supplier)
	}
	if check.LegalEntity.String() != "" || check.Snapshot.String() != "" {
		t.Fatal("服务方不符却交出了法人或快照")
	}
}

// Covers: 方案 D02-4「在提交时点 `SupportsProcurementAt`」——决定时点落在协议区间之外答未生效；次序上先于服务方：区间外的协议即便供应商也不对，
// 答的仍是未生效。
func TestAnAgreementOutsideItsIntervalIsNotEffectiveBeforeTheProviderIsCompared(t *testing.T) {
	for name, at := range map[string]time.Time{
		"区间未开": agreementStartsAt.Add(-time.Second),
		"区间已过": agreementEndsAt,
	} {
		for _, provider := range []string{"supplier-1", "supplier-2"} {
			check := mustCheck(t, &versionViewStub{read: withBody(t, "supplier-1")}, provider, at)
			if check.Standing != tfports.SupplierAgreementNotEffective {
				t.Fatalf("%s / %s：standing = %s，want AGREEMENT_NOT_EFFECTIVE", name, provider, check.Standing)
			}
		}
	}
}

// Covers: 终止自其时点起关闭后续使用（SupplierAgreement.Terminate）——终止前的决定照常在册，终止及之后答已终止。
func TestATerminatedAgreementIsTerminatedFromItsTerminationOn(t *testing.T) {
	read := withBody(t, "supplier-1")
	terminatedAt := insideAgreement
	terminated, err := read.Agreement.Terminate(
		mustBuild(t, pcdomain.NewRelationshipBasisReference, "termination-1"), terminatedAt)
	if err != nil {
		t.Fatalf("终止：%v", err)
	}
	read.Agreement = terminated

	if check := mustCheck(t, &versionViewStub{read: read}, "supplier-1", terminatedAt.Add(-time.Second)); check.Standing != tfports.SupplierAgreementRegistered {
		t.Fatalf("终止前：standing = %s，want AGREEMENT_REGISTERED", check.Standing)
	}
	if check := mustCheck(t, &versionViewStub{read: read}, "supplier-1", terminatedAt); check.Standing != tfports.SupplierAgreementTerminated {
		t.Fatalf("终止时点：standing = %s，want AGREEMENT_TERMINATED", check.Standing)
	}
}

// Covers: 没有这一版、已生效的壳没登正文，都答未在册——供应商与法人都在正文里，没有正文就无从核起。
func TestAMissingVersionOrBodyIsNotRegistered(t *testing.T) {
	for name, read := range map[string]pcports.SupplierAgreementVersionRead{
		"没有这一版": {},
		"壳在正文缺": shellOnlyRead(effectiveAgreementShell(t, "agreement-1", "v1")),
	} {
		check := mustCheck(t, &versionViewStub{read: read}, "supplier-1", insideAgreement)
		if check.Standing != tfports.SupplierAgreementNotRegistered {
			t.Fatalf("%s：standing = %s，want AGREEMENT_NOT_REGISTERED", name, check.Standing)
		}
	}
}

// Covers: 正文缺席时按壳的生命周期位置分格——已计划生效、已到期、已被替代答未生效；已退役答已终止。
func TestAShellOutsideTheEffectivePositionIsTranslatedByItsPosition(t *testing.T) {
	effective := effectiveAgreementShell(t, "agreement-1", "v1")
	approval, _ := effective.ApprovalBasis()
	publishedAt, _ := effective.PublishedAt()
	planned, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID: effective.Tenant(), Kind: effective.Kind(), ObjectID: effective.ObjectID(),
		Version: effective.Version(), Scope: effective.Scope(), ContentDigest: effective.ContentDigest(),
		Effective: effective.Effective(), Status: pcdomain.CommercialVersionPublished,
		Approval: approval, PublishedAt: publishedAt,
	})
	if err != nil {
		t.Fatalf("已计划生效：%v", err)
	}
	expired, err := effective.Expire(agreementEndsAt)
	if err != nil {
		t.Fatalf("到期：%v", err)
	}
	superseded, err := effective.SupersededBy(effectiveAgreementShell(t, "agreement-1", "v2"), insideAgreement)
	if err != nil {
		t.Fatalf("替代：%v", err)
	}
	retired, err := effective.Retire(mustBuild(t, pcdomain.NewRetirementReference, "retirement-1"), insideAgreement)
	if err != nil {
		t.Fatalf("退役：%v", err)
	}

	for name, tc := range map[string]struct {
		version pcdomain.CommercialVersion
		want    tfports.SupplierAgreementStanding
	}{
		"已计划生效": {planned, tfports.SupplierAgreementNotEffective},
		"已到期":   {expired, tfports.SupplierAgreementNotEffective},
		"已被替代":  {superseded, tfports.SupplierAgreementNotEffective},
		"已退役":   {retired, tfports.SupplierAgreementTerminated},
	} {
		check := mustCheck(t, &versionViewStub{read: shellOnlyRead(tc.version)}, "supplier-1", insideAgreement)
		if check.Standing != tc.want {
			t.Fatalf("%s：standing = %s，want %s", name, check.Standing, tc.want)
		}
	}
}

// Covers: X6 / ADR-0029——PC 读不动走 error 原样上抛，不折成任何一格否定结论，也不冒充词汇表外的答复。
func TestAPartyCommercialFailureIsNeverFoldedIntoAStanding(t *testing.T) {
	unavailable := errors.New("party commercial unavailable")

	check, err := checkThrough(t, &versionViewStub{err: unavailable}, "supplier-1", insideAgreement)

	if !errors.Is(err, unavailable) || errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("err = %v，want 原样上抛的读取失败", err)
	}
	if check.Standing != tfports.SupplierAgreementStandingInvalid {
		t.Fatalf("读不动却答了 %s", check.Standing)
	}
}

// Covers: ADR-0025 翻译是全函数——PC 读口交回自相矛盾的东西是编程错误，答 ErrUntranslatableAnswer 不答业务格。
func TestAContradictoryPartyCommercialAnswerIsUntranslatable(t *testing.T) {
	effective := effectiveAgreementShell(t, "agreement-1", "v1")
	retired, err := effective.Retire(mustBuild(t, pcdomain.NewRetirementReference, "retirement-1"), insideAgreement)
	if err != nil {
		t.Fatalf("退役：%v", err)
	}
	other := effectiveAgreementShell(t, "agreement-2", "v1")

	for name, read := range map[string]pcports.SupplierAgreementVersionRead{
		"有正文没有壳":   {Agreement: agreementBody(t, effective, "supplier-1"), HasAgreement: true},
		"交回了另一份协议": shellOnlyRead(other),
		"退役壳上带正文": {
			Version: retired, HasVersion: true,
			Agreement: agreementBody(t, effective, "supplier-1"), HasAgreement: true,
		},
		"正文挂在另一版上": {
			Version: effective, HasVersion: true,
			Agreement: agreementBody(t, other, "supplier-1"), HasAgreement: true,
		},
	} {
		if _, err := checkThrough(t, &versionViewStub{read: read}, "supplier-1", insideAgreement); !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("%s：err = %v，want ErrUntranslatableAnswer", name, err)
		}
	}
}

// Covers: 零决定时点会被 SupportsProcurementAt 恒答否、伪装成一格未生效——在问 PC 之前就拒。
func TestAZeroDecisionInstantIsRefusedBeforePartyCommercialIsAsked(t *testing.T) {
	stub := &versionViewStub{read: withBody(t, "supplier-1")}

	if _, err := checkThrough(t, stub, "supplier-1", time.Time{}); err == nil {
		t.Fatal("零时点被当成了一次核对")
	}
	if len(stub.asked) != 0 {
		t.Fatalf("零时点仍去问了 PC：%v", stub.asked)
	}
}

func TestTheSourceRefusesToBuildWithoutTheVersionView(t *testing.T) {
	if _, err := adapter.NewSupplierAgreementSource(nil); err == nil {
		t.Fatal("没有点读口也造出了核对源")
	}
}
