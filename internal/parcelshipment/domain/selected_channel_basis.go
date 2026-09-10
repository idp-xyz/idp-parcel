package domain

import "errors"

// 本文件是「择优结果」对象（票 label-channel/29，通道 1 裁决补强 ①：由 29 定义并产出、28 只接线）。
//
// 它把渠道择优选中的候选与建立面单交易要的七类依据引用固定成一份值对象。这份对象就是 PC CONTEXT
// 「后续发生渠道选择、面单交易……再固定该次决定实际使用的映射和授权依据，并保留它们与委托接受时快照
// 的关系」那份快照：前六格 + 候选是「实际使用的映射和授权依据」，责任依据那一格回指委托接受时的商业
// 解析——「与接受时快照的关系」由它保留。载体就是面单交易上的七格引用，不另起表、不复制任何 PC 正文。
//
// 系统择优作前置步产出它，日后运营端点人工择优也产同一个对象；面单交易的建立一步只收它、不问它从哪来
// （票 28 判据 1 的形状约束）。

var (
	// ErrInvalidSelectedChannelBasis 拒绝七格任一空白的择优结果：Establish 的聚合构造门对七格逐格都要，
	// 一份缺格的对象交过去只会在下游红，而红的地方离缺格的原因隔着一层翻译。
	ErrInvalidSelectedChannelBasis = errors.New("parcel shipment: invalid selected channel basis")
	// ErrInvalidSelectedChannelCandidate 拒绝立不住的选中候选（候选空白，或带上的评价 / 费率引用空白）。
	ErrInvalidSelectedChannelCandidate = errors.New("parcel shipment: invalid selected channel candidate")
)

// SelectedChannelCandidate 是择优步交出的「选中了谁」：候选，连同该候选按之出价的评价痕迹与费率引用。
//
// 两格各自可缺席而不是与候选同生：择优按成本单维只在已确立的成本之间比，赢家几乎总经过评价，但成本
// 取值的类型允许没有评价引用的已确立成本（PricedChannelCandidate 不要求它），这里如实沿用那个形状。
// 翻译适配器（票 29）要费率在场，缺席时由它具名停下，不在这里代填。
type SelectedChannelCandidate struct {
	candidate  ChannelCandidateID
	evaluation ChannelCostEvaluationReference
	rate       ChannelRateReference
}

func NewSelectedChannelCandidate(candidate ChannelCandidateID) (SelectedChannelCandidate, error) {
	if !candidate.valid() {
		return SelectedChannelCandidate{}, ErrInvalidSelectedChannelCandidate
	}
	return SelectedChannelCandidate{candidate: candidate}, nil
}

// WithEvaluation 带上该候选译自的那份 BUY 评价引用。
func (selected SelectedChannelCandidate) WithEvaluation(evaluation ChannelCostEvaluationReference) (SelectedChannelCandidate, error) {
	if !selected.candidate.valid() || !evaluation.valid() {
		return SelectedChannelCandidate{}, ErrInvalidSelectedChannelCandidate
	}
	selected.evaluation = evaluation
	return selected, nil
}

// WithRate 带上该候选按之出价的费率引用（评价所用的价卡版本）。
func (selected SelectedChannelCandidate) WithRate(rate ChannelRateReference) (SelectedChannelCandidate, error) {
	if !selected.candidate.valid() || !rate.valid() {
		return SelectedChannelCandidate{}, ErrInvalidSelectedChannelCandidate
	}
	selected.rate = rate
	return selected, nil
}

func (selected SelectedChannelCandidate) Candidate() ChannelCandidateID {
	return selected.candidate
}

// Evaluation 交出评价痕迹引用，第二个返回值为 false 即缺席。
func (selected SelectedChannelCandidate) Evaluation() (ChannelCostEvaluationReference, bool) {
	return selected.evaluation, selected.evaluation.valid()
}

// Rate 交出费率引用，第二个返回值为 false 即缺席。
func (selected SelectedChannelCandidate) Rate() (ChannelRateReference, bool) {
	return selected.rate, selected.rate.valid()
}

// SelectedChannelBasisSpec 是形成一份择优结果所需的全部输入；≥5 个输入按本包分界用 Spec 结构体
// （理由见 CommercialBasisSnapshotSpec 头注，此处不复述）。Evaluation 可缺席，其余各格必填。
type SelectedChannelBasisSpec struct {
	Candidate              ChannelCandidateID
	ChannelAccount         ChannelAccountReference
	AccountHolder          ChannelAccountHolderReference
	ServiceProvider        ChannelServiceProviderReference
	SettlementCounterparty SettlementCounterpartyReference
	Contract               ChannelContractReference
	Rate                   ChannelRateReference
	// ResponsibilityBasis 回指委托接受时的商业解析（CommercialResolutionID 的字面）——本对象作为快照与
	// 接受时快照的关系由这一格保留，不是拿接受时快照顶替本对象。
	ResponsibilityBasis ResponsibilityBasisSnapshotReference
	// Evaluation 是该候选译自的 BUY 评价引用，可缺席。
	Evaluation ChannelCostEvaluationReference
}

// SelectedChannelBasis 是一份已固定的择优结果，见文件头注。
type SelectedChannelBasis struct {
	candidate              ChannelCandidateID
	channelAccount         ChannelAccountReference
	accountHolder          ChannelAccountHolderReference
	serviceProvider        ChannelServiceProviderReference
	settlementCounterparty SettlementCounterpartyReference
	contract               ChannelContractReference
	rate                   ChannelRateReference
	responsibilityBasis    ResponsibilityBasisSnapshotReference
	evaluation             ChannelCostEvaluationReference
}

func NewSelectedChannelBasis(spec SelectedChannelBasisSpec) (SelectedChannelBasis, error) {
	if !spec.Candidate.valid() ||
		!spec.ChannelAccount.valid() ||
		!spec.AccountHolder.valid() ||
		!spec.ServiceProvider.valid() ||
		!spec.SettlementCounterparty.valid() ||
		!spec.Contract.valid() ||
		!spec.Rate.valid() ||
		!spec.ResponsibilityBasis.valid() {
		return SelectedChannelBasis{}, ErrInvalidSelectedChannelBasis
	}
	return SelectedChannelBasis{
		candidate:              spec.Candidate,
		channelAccount:         spec.ChannelAccount,
		accountHolder:          spec.AccountHolder,
		serviceProvider:        spec.ServiceProvider,
		settlementCounterparty: spec.SettlementCounterparty,
		contract:               spec.Contract,
		rate:                   spec.Rate,
		responsibilityBasis:    spec.ResponsibilityBasis,
		evaluation:             spec.Evaluation,
	}, nil
}

func (basis SelectedChannelBasis) Candidate() ChannelCandidateID {
	return basis.candidate
}

func (basis SelectedChannelBasis) ChannelAccount() ChannelAccountReference {
	return basis.channelAccount
}

func (basis SelectedChannelBasis) AccountHolder() ChannelAccountHolderReference {
	return basis.accountHolder
}

func (basis SelectedChannelBasis) ServiceProvider() ChannelServiceProviderReference {
	return basis.serviceProvider
}

func (basis SelectedChannelBasis) SettlementCounterparty() SettlementCounterpartyReference {
	return basis.settlementCounterparty
}

func (basis SelectedChannelBasis) Contract() ChannelContractReference {
	return basis.contract
}

func (basis SelectedChannelBasis) Rate() ChannelRateReference {
	return basis.rate
}

func (basis SelectedChannelBasis) ResponsibilityBasis() ResponsibilityBasisSnapshotReference {
	return basis.responsibilityBasis
}

// Evaluation 交出评价痕迹引用，第二个返回值为 false 即缺席。
func (basis SelectedChannelBasis) Evaluation() (ChannelCostEvaluationReference, bool) {
	return basis.evaluation, basis.evaluation.valid()
}
