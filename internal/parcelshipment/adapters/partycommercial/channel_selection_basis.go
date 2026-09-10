package partycommercial

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件是择优结果 → 面单交易七类依据引用的翻译（票 `label-channel/29`）。落点由 ADR-0025 决定：候选标识
// 的字面就是 party-commercial 的渠道产品引用（channel_candidate_assembly.go 造候选时取 ChannelProductReference
// 的字面），七格里六格要从那一侧的账号使用授权与供应商协议上取，两套词汇之间的翻译只许在这一层发生。
//
// **翻译不判候选该不该赢，也不复制任何一条 PC 规则。** 授权此刻允不允许新的使用、协议此刻支不支持新的
// 采购决定，都问 PC 自己的谓词（AllowsUseAt / SupportsProcurementAt）；本包只把「不允许」分成调用方续办
// 不同的几格（撤销了去拿新授权、过期了去续期、盖错渠道去修配置），不重写判据。
//
// 「这次该用哪一条授权、哪一版协议、哪一次接受时解析」三问是消费方自己的实例半边（裁决 (1)，同
// ResolutionKeySource 留在本包的那条理由）：今天没有租户，谁也说不出候选 channel-a 该走哪条授权；三个源
// 未配置时各自具名停下，不代拟、不拿「最新一条」顶替「适用的那条」。

var (
	// ErrSelectedCandidateRateAbsent 说择优步交出的赢家没带费率引用。Rate 是 Establish 必填的一格，缺席
	// 只能是成本取值没把评价所用的卡带过来（装配缺件），不是「这笔交易没有费率」；这里不代填也不去问
	// parcel-pricing——费率由择优步带出是裁决 (3)。
	ErrSelectedCandidateRateAbsent = errors.New("parcel shipment: selected channel candidate carries no rate reference")

	// ErrChannelAccountUseNotConfigured 说「这个候选该用哪条账号使用授权」此刻答不上来（源未装或答未配置）。
	ErrChannelAccountUseNotConfigured = errors.New("parcel shipment: channel account use source not configured")
	// ErrChannelAccountUseNotRegistered 说源点名的那条授权从未登记——续办是去登记，不是换一条。
	ErrChannelAccountUseNotRegistered = errors.New("parcel shipment: channel account use authorization not registered")
	// ErrChannelAccountUseChannelMismatch 说那条授权盖的是另一个渠道产品：拿它给这个候选发起请求，账号持有人
	// 从没允许过。它与「授权不在有效期」分开——修的是配置，不是等时间。
	ErrChannelAccountUseChannelMismatch = errors.New("parcel shipment: channel account use authorization covers another channel product")
	// ErrChannelAccountUseGranteeMismatch 说那条授权授给的不是运营企业。
	ErrChannelAccountUseGranteeMismatch = errors.New("parcel shipment: channel account use authorization is granted to another party")
	// ErrChannelAccountUseScopeMismatch 说那条授权盖的是另一个商业范围。
	ErrChannelAccountUseScopeMismatch = errors.New("parcel shipment: channel account use authorization covers another commercial scope")
	// ErrChannelAccountUseRevoked 说那条授权在择优时点前已被持有人撤销——续办是取得新授权（ADR-0039）。
	ErrChannelAccountUseRevoked = errors.New("parcel shipment: channel account use authorization is revoked at the selection time")
	// ErrChannelAccountUseNotEffective 说那条授权的有效区间没盖住择优时点（未起效或已到期）。
	ErrChannelAccountUseNotEffective = errors.New("parcel shipment: channel account use authorization is not effective at the selection time")

	// ErrSupplierAgreementNotConfigured 说「这个候选该按哪一版供应商协议」此刻答不上来。
	ErrSupplierAgreementNotConfigured = errors.New("parcel shipment: supplier agreement source not configured")
	// ErrSupplierAgreementNotRegistered 说源点名的那一版协议在正文口取不到。
	ErrSupplierAgreementNotRegistered = errors.New("parcel shipment: supplier agreement not registered")
	// ErrSupplierAgreementTerminated 说那一版协议在择优时点前已终止——它不再支持新的采购决定。
	ErrSupplierAgreementTerminated = errors.New("parcel shipment: supplier agreement is terminated at the selection time")
	// ErrSupplierAgreementNotEffective 说那一版协议未生效或有效区间没盖住择优时点。
	ErrSupplierAgreementNotEffective = errors.New("parcel shipment: supplier agreement is not effective at the selection time")

	// ErrAcceptanceResolutionNotConfigured 说「这次择优对应哪一次委托接受时的商业解析」答不上来。择优查询今天
	// 不带委托或包裹的引用（ChannelSelectionSubject 头注原句），回指只能由消费方的实例半边给。
	ErrAcceptanceResolutionNotConfigured = errors.New("parcel shipment: acceptance resolution source not configured")
)

// ChannelAccountUseSelection 是「这个候选该用哪条授权」的答复：登记标识，连同被授权人应当是谁（运营企业在 PC
// 里的参与方标识）。后者也放在答复里而不是另开一格配置：PC 没有「租户的运营企业是哪个 party」这个概念，
// 它同样是实例半边，与授权的选法出自同一处登记。
type ChannelAccountUseSelection struct {
	Authorization pcdomain.ChannelAccountUseAuthorizationID
	Grantee       pcdomain.PartyID
}

// ChannelAccountUseSource 回答某次择优的某个候选该用哪条账号使用授权。第二个返回值为 false 即「显式未配置」。
type ChannelAccountUseSource interface {
	AuthorizationFor(
		ctx context.Context,
		query psports.ChannelSelectionQuery,
		candidate psdomain.ChannelCandidateID,
	) (ChannelAccountUseSelection, bool, error)
}

// SupplierAgreementSource 回答某次择优的某个候选该按哪一版供应商协议。第二个返回值为 false 即「显式未配置」。
type SupplierAgreementSource interface {
	AgreementFor(
		ctx context.Context,
		query psports.ChannelSelectionQuery,
		candidate psdomain.ChannelCandidateID,
	) (pcdomain.CommercialVersion, bool, error)
}

// AcceptanceResolutionSource 回答某次择优对应哪一次委托接受时的商业解析。第二个返回值为 false 即「显式未配置」。
type AcceptanceResolutionSource interface {
	ResolutionFor(
		ctx context.Context,
		query psports.ChannelSelectionQuery,
	) (psdomain.CommercialResolutionID, bool, error)
}

// ChannelAccountUseReader 是账号使用授权登记册的只读半边。不直接依赖 pcports.ChannelAccountUseAuthorizationRegistry：
// 那个口带着 Save，而翻译是纯读路径（同 ProductChannelMappingReader 的理由）。
type ChannelAccountUseReader interface {
	LoadLatest(
		ctx context.Context,
		tenant pcdomain.TenantID,
		authorization pcdomain.ChannelAccountUseAuthorizationID,
	) (pcdomain.ChannelAccountUseAuthorizationRegistration, bool, error)
}

// ChannelSelectionBasisTranslatorDeps 收拢三个实例半边源与两个 PC 读口；≥5 个输入按本仓分界用结构体。
type ChannelSelectionBasisTranslatorDeps struct {
	Accounts       ChannelAccountUseSource
	Agreements     SupplierAgreementSource
	Resolutions    AcceptanceResolutionSource
	Authorizations ChannelAccountUseReader
	Contents       pcports.SupplierAgreementContentView
}

// ChannelSelectionBasisTranslator 把择优步选中的候选译成建立面单交易所需的「择优结果」。
type ChannelSelectionBasisTranslator struct {
	deps ChannelSelectionBasisTranslatorDeps
}

// NewChannelSelectionBasisTranslator 装配翻译器。三个源允许为 nil：那是「显式未配置」的诚实表达，届时翻译停在
// 各自具名的那一格——那正是首发要停下的地方，不是要绕过的地方。
func NewChannelSelectionBasisTranslator(deps ChannelSelectionBasisTranslatorDeps) *ChannelSelectionBasisTranslator {
	return &ChannelSelectionBasisTranslator{deps: deps}
}

var _ psports.ChannelSelectionBasisTranslator = (*ChannelSelectionBasisTranslator)(nil)

// TranslateSelectedCandidate 交回七格齐备的择优结果，或在第一个答不出的格具名停下。
//
// 次序：先核手上这份选中候选译得出去（费率在场、候选译成渠道产品引用），再问三个源，再去两个读口取回并核，
// 最后折成对象。源先问、读口后取：源答不上来时读口取回的东西没处用，而读口可能是一次远程读。
func (translator *ChannelSelectionBasisTranslator) TranslateSelectedCandidate(
	ctx context.Context,
	query psports.ChannelSelectionQuery,
	selected psdomain.SelectedChannelCandidate,
) (psdomain.SelectedChannelBasis, error) {
	keys, err := providerKeysOf(query)
	if err != nil {
		return psdomain.SelectedChannelBasis{}, err
	}
	channel, err := pcdomain.NewChannelProductReference(selected.Candidate().String())
	if err != nil {
		return psdomain.SelectedChannelBasis{}, fmt.Errorf("%w: channel candidate: %v", ErrUntranslatableQuery, err)
	}
	rate, present := selected.Rate()
	if !present {
		return psdomain.SelectedChannelBasis{}, ErrSelectedCandidateRateAbsent
	}

	selection, err := translator.accountUseFor(ctx, query, selected.Candidate())
	if err != nil {
		return psdomain.SelectedChannelBasis{}, err
	}
	agreementVersion, err := translator.agreementVersionFor(ctx, query, selected.Candidate())
	if err != nil {
		return psdomain.SelectedChannelBasis{}, err
	}
	resolution, err := translator.resolutionFor(ctx, query)
	if err != nil {
		return psdomain.SelectedChannelBasis{}, err
	}

	authorization, err := translator.authorizationOf(ctx, query, keys, channel, selection)
	if err != nil {
		return psdomain.SelectedChannelBasis{}, err
	}
	agreement, err := translator.agreementOf(ctx, query, keys, agreementVersion)
	if err != nil {
		return psdomain.SelectedChannelBasis{}, err
	}

	return selectedChannelBasisOf(selected, rate, authorization, agreement, resolution)
}

func (translator *ChannelSelectionBasisTranslator) accountUseFor(
	ctx context.Context,
	query psports.ChannelSelectionQuery,
	candidate psdomain.ChannelCandidateID,
) (ChannelAccountUseSelection, error) {
	if translator.deps.Accounts == nil {
		return ChannelAccountUseSelection{}, ErrChannelAccountUseNotConfigured
	}
	selection, configured, err := translator.deps.Accounts.AuthorizationFor(ctx, query, candidate)
	if err != nil {
		return ChannelAccountUseSelection{}, fmt.Errorf("select channel account use authorization: %w", err)
	}
	if !configured {
		return ChannelAccountUseSelection{}, ErrChannelAccountUseNotConfigured
	}
	return selection, nil
}

func (translator *ChannelSelectionBasisTranslator) agreementVersionFor(
	ctx context.Context,
	query psports.ChannelSelectionQuery,
	candidate psdomain.ChannelCandidateID,
) (pcdomain.CommercialVersion, error) {
	if translator.deps.Agreements == nil {
		return pcdomain.CommercialVersion{}, ErrSupplierAgreementNotConfigured
	}
	version, configured, err := translator.deps.Agreements.AgreementFor(ctx, query, candidate)
	if err != nil {
		return pcdomain.CommercialVersion{}, fmt.Errorf("select supplier agreement: %w", err)
	}
	if !configured {
		return pcdomain.CommercialVersion{}, ErrSupplierAgreementNotConfigured
	}
	return version, nil
}

func (translator *ChannelSelectionBasisTranslator) resolutionFor(
	ctx context.Context,
	query psports.ChannelSelectionQuery,
) (psdomain.CommercialResolutionID, error) {
	if translator.deps.Resolutions == nil {
		return psdomain.CommercialResolutionID{}, ErrAcceptanceResolutionNotConfigured
	}
	resolution, configured, err := translator.deps.Resolutions.ResolutionFor(ctx, query)
	if err != nil {
		return psdomain.CommercialResolutionID{}, fmt.Errorf("select acceptance resolution: %w", err)
	}
	if !configured {
		return psdomain.CommercialResolutionID{}, ErrAcceptanceResolutionNotConfigured
	}
	return resolution, nil
}

// authorizationOf 取回源点名的那条授权并核它确实盖住这次使用：盖的渠道产品就是候选、授给的是运营企业、盖的
// 范围就是这次的范围、时点上允许新的使用。四条核完才交回——任一不满足即停，不换一条。
//
// 「允不允许」问 PC 的 AllowsUseAt；本包只把「不允许」分成撤销与不在有效期两格（续办不同），分格看的是
// 事实（有没有撤销时刻、撤销时刻在不在时点之前），不重写它的判据。
func (translator *ChannelSelectionBasisTranslator) authorizationOf(
	ctx context.Context,
	query psports.ChannelSelectionQuery,
	keys providerKeys,
	channel pcdomain.ChannelProductReference,
	selection ChannelAccountUseSelection,
) (pcdomain.ChannelAccountUseAuthorization, error) {
	registration, found, err := translator.deps.Authorizations.LoadLatest(ctx, keys.tenant, selection.Authorization)
	if err != nil {
		return pcdomain.ChannelAccountUseAuthorization{}, fmt.Errorf("load channel account use authorization: %w", err)
	}
	if !found {
		return pcdomain.ChannelAccountUseAuthorization{}, ErrChannelAccountUseNotRegistered
	}
	authorization := registration.Authorization()
	if authorization.Channel() != channel {
		return pcdomain.ChannelAccountUseAuthorization{}, ErrChannelAccountUseChannelMismatch
	}
	if authorization.Grantee() != selection.Grantee {
		return pcdomain.ChannelAccountUseAuthorization{}, ErrChannelAccountUseGranteeMismatch
	}
	if authorization.Scope() != keys.scope {
		return pcdomain.ChannelAccountUseAuthorization{}, ErrChannelAccountUseScopeMismatch
	}
	if !authorization.AllowsUseAt(query.At) {
		if revokedAt, revoked := authorization.RevokedAt(); revoked && !query.At.Before(revokedAt) {
			return pcdomain.ChannelAccountUseAuthorization{}, ErrChannelAccountUseRevoked
		}
		return pcdomain.ChannelAccountUseAuthorization{}, ErrChannelAccountUseNotEffective
	}
	return authorization, nil
}

// agreementOf 取回源点名的那一版协议并核它此刻支持新的采购决定。判据问 PC 的 SupportsProcurementAt；本包只把
// 「不支持」分成已终止与不在有效期两格。
func (translator *ChannelSelectionBasisTranslator) agreementOf(
	ctx context.Context,
	query psports.ChannelSelectionQuery,
	keys providerKeys,
	version pcdomain.CommercialVersion,
) (pcdomain.SupplierAgreement, error) {
	agreement, found, err := translator.deps.Contents.LoadSupplierAgreement(ctx, keys.tenant, version)
	if err != nil {
		return pcdomain.SupplierAgreement{}, fmt.Errorf("load supplier agreement: %w", err)
	}
	if !found {
		return pcdomain.SupplierAgreement{}, ErrSupplierAgreementNotRegistered
	}
	if !agreement.SupportsProcurementAt(query.At) {
		if terminatedAt, terminated := agreement.TerminatedAt(); terminated && !query.At.Before(terminatedAt) {
			return pcdomain.SupplierAgreement{}, ErrSupplierAgreementTerminated
		}
		return pcdomain.SupplierAgreement{}, ErrSupplierAgreementNotEffective
	}
	return agreement, nil
}

// selectedChannelBasisOf 把取回的东西折成七格。服务方与结算相对方同取协议的 Supplier()：同源是首发的实例事实、
// 不是类型上的同义——PC CONTEXT 要求两者分别表达，代理 / 分包场景下会不同，届时 PC 立第二格、这里改取处
// （裁决 (2)）。合同引用照「对象/版本」两段写，与 commercial_basis.go 对 PC 版本引用的写法同形。
func selectedChannelBasisOf(
	selected psdomain.SelectedChannelCandidate,
	rate psdomain.ChannelRateReference,
	authorization pcdomain.ChannelAccountUseAuthorization,
	agreement pcdomain.SupplierAgreement,
	resolution psdomain.CommercialResolutionID,
) (psdomain.SelectedChannelBasis, error) {
	spec := psdomain.SelectedChannelBasisSpec{Candidate: selected.Candidate(), Rate: rate}
	var err error
	if spec.ChannelAccount, err = psdomain.NewChannelAccountReference(authorization.Account().String()); err != nil {
		return psdomain.SelectedChannelBasis{}, fmt.Errorf("%w: channel account: %v", ErrUntranslatableAnswer, err)
	}
	if spec.AccountHolder, err = psdomain.NewChannelAccountHolderReference(authorization.Grantor().String()); err != nil {
		return psdomain.SelectedChannelBasis{}, fmt.Errorf("%w: account holder: %v", ErrUntranslatableAnswer, err)
	}
	supplier := agreement.Supplier().String()
	if spec.ServiceProvider, err = psdomain.NewChannelServiceProviderReference(supplier); err != nil {
		return psdomain.SelectedChannelBasis{}, fmt.Errorf("%w: service provider: %v", ErrUntranslatableAnswer, err)
	}
	if spec.SettlementCounterparty, err = psdomain.NewSettlementCounterpartyReference(supplier); err != nil {
		return psdomain.SelectedChannelBasis{}, fmt.Errorf("%w: settlement counterparty: %v", ErrUntranslatableAnswer, err)
	}
	version := agreement.Version()
	if spec.Contract, err = psdomain.NewChannelContractReference(version.ObjectID().String() + "/" + version.Version().String()); err != nil {
		return psdomain.SelectedChannelBasis{}, fmt.Errorf("%w: contract: %v", ErrUntranslatableAnswer, err)
	}
	if spec.ResponsibilityBasis, err = psdomain.NewResponsibilityBasisSnapshotReference(resolution.String()); err != nil {
		return psdomain.SelectedChannelBasis{}, fmt.Errorf("%w: responsibility basis: %v", ErrUntranslatableAnswer, err)
	}
	if evaluation, present := selected.Evaluation(); present {
		spec.Evaluation = evaluation
	}
	basis, err := psdomain.NewSelectedChannelBasis(spec)
	if err != nil {
		return psdomain.SelectedChannelBasis{}, fmt.Errorf("%w: selected channel basis: %v", ErrUntranslatableAnswer, err)
	}
	return basis, nil
}
