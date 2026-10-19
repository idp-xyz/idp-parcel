package partycommercial

import (
	"context"
	"fmt"
	"time"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件是供应商协议核对缝的消费侧适配器（票 demo-chain-gaps/22）：把 party-commercial 的版本点读口答回的壳与
// 正文译成 tfports.SupplierAgreementStanding 的各格。与同包其余适配器并列、各答各的，不合成「PC 门面」——
// 所有者虽同，问的事、键与失败形状都不同。
//
// **判据一条不复制。** 「这一版此刻支不支持新的采购决定」问 PC 自己的 SupportsProcurementAt，本包只把「不支持」
// 按事实分成已终止与未生效两格（有没有终止时刻、终止时刻在不在决定时点之前），判据同 parcel-shipment 那一侧的
// agreementOf。正文缺席时那个谓词无从可问，就按壳的生命周期位置分格：
//
//   - 已计划生效（PUBLISHED）、已到期、已被替代 → 未生效。三者对这一版的续办相同：换一版此刻生效的。
//   - 已退役 → 已终止。PC CONTEXT 写供应商协议版本「到期、终止或被替代」，版本生命周期上与「终止」
//     对应的是按明确决定收尾的退役（Retire 要一份决定引用），不是按区间到点的到期。
//   - 已生效而正文没登 → 未在册。供应商与责任法人都在正文里，没有正文就核不了服务方、也快照不了法人；续办
//     是回 PC 带正文重放同一版，与「从未发布」同属去 PC 补登。
//
// 服务方比的是同一串字面：TF 的服务提供方引用与 PC 的参与方身份在两个上下文里是同一串（同包其余适配器的
// 约定）。不比名称、不做归一——差一个字符就是另一个参与方。

// SupplierAgreementSource 实现 tfports.SupplierAgreementSource。
type SupplierAgreementSource struct {
	versions pcports.SupplierAgreementVersionView
}

// NewSupplierAgreementSource 点读口必须在：缺了它每一次核对都只能答 error，而 error 在调用方那里是`未决`，
// 一个装配漏接会长成一串永远等不来的未决。
func NewSupplierAgreementSource(versions pcports.SupplierAgreementVersionView) (*SupplierAgreementSource, error) {
	if versions == nil {
		return nil, fmt.Errorf("transport fulfillment partycommercial adapter: supplier agreement version view is nil")
	}
	return &SupplierAgreementSource{versions: versions}, nil
}

var _ tfports.SupplierAgreementSource = (*SupplierAgreementSource)(nil)

// CheckSupplierAgreement 按（租户、协议版本）向 PC 点读，再按 tfports.SupplierAgreementStanding 所列检查的次序
// 译成一格。
func (source *SupplierAgreementSource) CheckSupplierAgreement(
	ctx context.Context,
	tenant tfdomain.TenantID,
	agreement tfdomain.SupplierAgreementVersion,
	provider tfdomain.ServiceProviderReference,
	at time.Time,
) (tfports.SupplierAgreementCheck, error) {
	none := tfports.SupplierAgreementCheck{}
	if at.IsZero() {
		// 零时点不是「没有时点」的业务答案：SupportsProcurementAt 对它恒答否，会被译成一格看似真实的未生效。
		return none, fmt.Errorf("check supplier agreement: the decision instant is required")
	}
	pcTenant, err := pcdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	object, err := pcdomain.NewCommercialObjectID(agreement.Object())
	if err != nil {
		return none, fmt.Errorf("%w: agreement object: %v", ErrUntranslatableAnswer, err)
	}
	label, err := pcdomain.NewCommercialVersionLabel(agreement.Version())
	if err != nil {
		return none, fmt.Errorf("%w: agreement version label: %v", ErrUntranslatableAnswer, err)
	}

	read, err := source.versions.LoadSupplierAgreementVersion(ctx, pcTenant, object, label)
	if err != nil {
		return none, fmt.Errorf("load supplier agreement version: party commercial: %w", err)
	}
	if !read.HasVersion {
		if read.HasAgreement {
			return none, fmt.Errorf("%w: party commercial answered a body without its version", ErrUntranslatableAnswer)
		}
		return tfports.SupplierAgreementCheck{Standing: tfports.SupplierAgreementNotRegistered}, nil
	}
	version := read.Version
	if version.Tenant() != pcTenant || version.Kind() != pcdomain.SupplierAgreementObject ||
		version.ObjectID() != object || version.Version() != label {
		return none, fmt.Errorf("%w: party commercial answered %s %s/%s for %s/%s",
			ErrUntranslatableAnswer, version.Kind(), version.ObjectID(), version.Version(), object, label)
	}

	switch version.Status() {
	case pcdomain.CommercialVersionEffective:
	case pcdomain.CommercialVersionPublished, pcdomain.CommercialVersionExpired, pcdomain.CommercialVersionSuperseded:
		return shellOnly(read, tfports.SupplierAgreementNotEffective)
	case pcdomain.CommercialVersionRetired:
		return shellOnly(read, tfports.SupplierAgreementTerminated)
	default:
		return none, fmt.Errorf("%w: party commercial answered a version in position %q", ErrUntranslatableAnswer, version.Status())
	}

	if !read.HasAgreement {
		return tfports.SupplierAgreementCheck{Standing: tfports.SupplierAgreementNotRegistered}, nil
	}
	content := read.Agreement
	if !content.Version().SameVersionAs(version) {
		return none, fmt.Errorf("%w: party commercial answered a body of another version for %s/%s",
			ErrUntranslatableAnswer, object, label)
	}
	if !content.SupportsProcurementAt(at) {
		if terminatedAt, terminated := content.TerminatedAt(); terminated && !at.Before(terminatedAt) {
			return tfports.SupplierAgreementCheck{Standing: tfports.SupplierAgreementTerminated}, nil
		}
		return tfports.SupplierAgreementCheck{Standing: tfports.SupplierAgreementNotEffective}, nil
	}

	supplier, err := tfdomain.NewServiceProviderReference(content.Supplier().String())
	if err != nil {
		return none, fmt.Errorf("%w: supplier: %v", ErrUntranslatableAnswer, err)
	}
	if supplier != provider {
		return tfports.SupplierAgreementCheck{Standing: tfports.SupplierAgreementProviderMismatch, Supplier: supplier}, nil
	}
	legalEntity, err := tfdomain.NewProcurementLegalEntityReference(content.LegalEntity().String())
	if err != nil {
		return none, fmt.Errorf("%w: legal entity: %v", ErrUntranslatableAnswer, err)
	}
	// 快照引用取 PC 一处拼出的「对象/版本」（ADR-0080 决定七），本包不自己拼。
	snapshot, err := tfdomain.NewAgreementSnapshotReference(version.QualifiedLabel().String())
	if err != nil {
		return none, fmt.Errorf("%w: agreement snapshot: %v", ErrUntranslatableAnswer, err)
	}
	return tfports.SupplierAgreementCheck{
		Standing:    tfports.SupplierAgreementRegistered,
		Snapshot:    snapshot,
		LegalEntity: legalEntity,
		Supplier:    supplier,
	}, nil
}

// shellOnly 答不在已生效位置的壳。这样的壳带着正文是 PC 读口违约：领域只在已生效版本上立正文。
func shellOnly(
	read pcports.SupplierAgreementVersionRead,
	standing tfports.SupplierAgreementStanding,
) (tfports.SupplierAgreementCheck, error) {
	if read.HasAgreement {
		return tfports.SupplierAgreementCheck{}, fmt.Errorf("%w: party commercial answered a body on a version in position %q",
			ErrUntranslatableAnswer, read.Version.Status())
	}
	return tfports.SupplierAgreementCheck{Standing: standing}, nil
}
