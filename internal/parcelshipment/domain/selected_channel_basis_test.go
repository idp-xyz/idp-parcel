package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证「择优结果」对象（票 label-channel/29，裁决补强 ①）：它把择优选中的候选与建立面单交易要的七类
// 依据引用固定成一份值对象，就是 PC CONTEXT「再固定该次决定实际使用的映射和授权依据，并保留它们与委托
// 接受时快照的关系」那份快照——载体是七格引用、不复制任何 PC 正文。评价痕迹可缺席，七格一格不能缺。

func selectedBasisSpec(t *testing.T) domain.SelectedChannelBasisSpec {
	t.Helper()

	return domain.SelectedChannelBasisSpec{
		Candidate:              mustValue(t, domain.NewChannelCandidateID, "SYN-CHANNEL-A"),
		ChannelAccount:         mustValue(t, domain.NewChannelAccountReference, "SYN-ACCOUNT-1"),
		AccountHolder:          mustValue(t, domain.NewChannelAccountHolderReference, "SYN-PARTY-HOLDER"),
		ServiceProvider:        mustValue(t, domain.NewChannelServiceProviderReference, "SYN-PARTY-SUPPLIER"),
		SettlementCounterparty: mustValue(t, domain.NewSettlementCounterpartyReference, "SYN-PARTY-SUPPLIER"),
		Contract:               mustValue(t, domain.NewChannelContractReference, "SYN-AGREEMENT/v1"),
		Rate:                   mustValue(t, domain.NewChannelRateReference, "SYN-BUY-PLAN/v3"),
		ResponsibilityBasis:    mustValue(t, domain.NewResponsibilityBasisSnapshotReference, "SYN-RESOLUTION-1"),
		Evaluation:             mustValue(t, domain.NewChannelCostEvaluationReference, "SYN-EVAL-1"),
	}
}

// Covers: 票 29 判据 1——七格 + 候选逐格固定、逐格读回；评价痕迹在场时读得回。
func TestASelectedChannelBasisFixesTheCandidateAndAllSevenReferences(t *testing.T) {
	t.Parallel()

	spec := selectedBasisSpec(t)
	basis, err := domain.NewSelectedChannelBasis(spec)
	if err != nil {
		t.Fatalf("造择优结果：%v", err)
	}

	if basis.Candidate() != spec.Candidate ||
		basis.ChannelAccount() != spec.ChannelAccount ||
		basis.AccountHolder() != spec.AccountHolder ||
		basis.ServiceProvider() != spec.ServiceProvider ||
		basis.SettlementCounterparty() != spec.SettlementCounterparty ||
		basis.Contract() != spec.Contract ||
		basis.Rate() != spec.Rate ||
		basis.ResponsibilityBasis() != spec.ResponsibilityBasis {
		t.Fatalf("读回的格与写进去的不同：%#v", basis)
	}
	evaluation, present := basis.Evaluation()
	if !present || evaluation != spec.Evaluation {
		t.Fatalf("评价痕迹 = %v/%v，want %v 在场", evaluation, present, spec.Evaluation)
	}
}

// Covers: 票 29 判据 1「七格任一空白即拒」——候选与七格逐格挖空各红一次，错误是具名的。
func TestASelectedChannelBasisRefusesAnyBlankReference(t *testing.T) {
	t.Parallel()

	blankings := map[string]func(*domain.SelectedChannelBasisSpec){
		"候选":   func(spec *domain.SelectedChannelBasisSpec) { spec.Candidate = domain.ChannelCandidateID{} },
		"渠道账号": func(spec *domain.SelectedChannelBasisSpec) { spec.ChannelAccount = domain.ChannelAccountReference{} },
		"账号持有人": func(spec *domain.SelectedChannelBasisSpec) {
			spec.AccountHolder = domain.ChannelAccountHolderReference{}
		},
		"渠道服务方": func(spec *domain.SelectedChannelBasisSpec) {
			spec.ServiceProvider = domain.ChannelServiceProviderReference{}
		},
		"结算相对方": func(spec *domain.SelectedChannelBasisSpec) {
			spec.SettlementCounterparty = domain.SettlementCounterpartyReference{}
		},
		"合同": func(spec *domain.SelectedChannelBasisSpec) { spec.Contract = domain.ChannelContractReference{} },
		"费率": func(spec *domain.SelectedChannelBasisSpec) { spec.Rate = domain.ChannelRateReference{} },
		"责任依据": func(spec *domain.SelectedChannelBasisSpec) {
			spec.ResponsibilityBasis = domain.ResponsibilityBasisSnapshotReference{}
		},
	}
	for name, blank := range blankings {
		t.Run(name, func(t *testing.T) {
			spec := selectedBasisSpec(t)
			blank(&spec)
			if _, err := domain.NewSelectedChannelBasis(spec); !errors.Is(err, domain.ErrInvalidSelectedChannelBasis) {
				t.Fatalf("挖空%s后 err = %v，want ErrInvalidSelectedChannelBasis", name, err)
			}
		})
	}
}

// Covers: 票 29「评价痕迹引用可缺席」——缺席是合法状态，读回时如实报缺席而不是零值冒充在场。
func TestASelectedChannelBasisMayLackAnEvaluationTrace(t *testing.T) {
	t.Parallel()

	spec := selectedBasisSpec(t)
	spec.Evaluation = domain.ChannelCostEvaluationReference{}
	basis, err := domain.NewSelectedChannelBasis(spec)
	if err != nil {
		t.Fatalf("不带评价痕迹的择优结果被拒：%v", err)
	}
	if _, present := basis.Evaluation(); present {
		t.Fatal("没给评价痕迹却读回「在场」")
	}
}

// Covers: 票 29 判据 3 正路的后半——七格照抄进 EstablishLabelTransactionSpec 就能过聚合构造门，且交易上
// 读回的七格与择优结果逐格同：对象与 Establish 的七格同型不是口头约定，是这条测试钉的。
func TestASelectedChannelBasisFeedsTheLabelTransactionEstablishGate(t *testing.T) {
	t.Parallel()

	basis, err := domain.NewSelectedChannelBasis(selectedBasisSpec(t))
	if err != nil {
		t.Fatalf("造择优结果：%v", err)
	}
	transaction, err := domain.EstablishLabelTransaction(domain.EstablishLabelTransactionSpec{
		Tenant:                 mustValue(t, domain.NewTenantID, "tenant-1"),
		ID:                     mustValue(t, domain.NewLabelTransactionID, "LT-1"),
		CoveredParcels:         []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "P-1")},
		ChannelAccount:         basis.ChannelAccount(),
		AccountHolder:          basis.AccountHolder(),
		ServiceProvider:        basis.ServiceProvider(),
		SettlementCounterparty: basis.SettlementCounterparty(),
		Contract:               basis.Contract(),
		Rate:                   basis.Rate(),
		ResponsibilityBasis:    basis.ResponsibilityBasis(),
		EstablishedAt:          time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("七格照抄进 Establish 被拒：%v", err)
	}
	if transaction.ChannelAccount() != basis.ChannelAccount() ||
		transaction.AccountHolder() != basis.AccountHolder() ||
		transaction.ServiceProvider() != basis.ServiceProvider() ||
		transaction.SettlementCounterparty() != basis.SettlementCounterparty() ||
		transaction.Contract() != basis.Contract() ||
		transaction.Rate() != basis.Rate() ||
		transaction.ResponsibilityBasis() != basis.ResponsibilityBasis() {
		t.Fatal("交易上读回的七格与择优结果不同")
	}
}

// Covers: 票 29 判据 2 的领域半边——择优步交出的「选中了谁」可以带上该候选的评价痕迹与费率，两格各自可缺
// （没登记价卡的候选没经过评价），带上时空白即拒。
func TestASelectedChannelCandidateCarriesItsEvaluationAndRateWhenPriced(t *testing.T) {
	t.Parallel()

	candidate := mustValue(t, domain.NewChannelCandidateID, "SYN-CHANNEL-A")
	bare, err := domain.NewSelectedChannelCandidate(candidate)
	if err != nil {
		t.Fatalf("造选中候选：%v", err)
	}
	if bare.Candidate() != candidate {
		t.Fatalf("候选 = %v，want %v", bare.Candidate(), candidate)
	}
	if _, present := bare.Evaluation(); present {
		t.Fatal("没带评价却报在场")
	}
	if _, present := bare.Rate(); present {
		t.Fatal("没带费率却报在场")
	}

	evaluation := mustValue(t, domain.NewChannelCostEvaluationReference, "SYN-EVAL-1")
	rate := mustValue(t, domain.NewChannelRateReference, "SYN-BUY-PLAN/v3")
	evaluated, err := bare.WithEvaluation(evaluation)
	if err != nil {
		t.Fatalf("带评价：%v", err)
	}
	priced, err := evaluated.WithRate(rate)
	if err != nil {
		t.Fatalf("带费率：%v", err)
	}
	if got, present := priced.Evaluation(); !present || got != evaluation {
		t.Fatalf("评价 = %v/%v", got, present)
	}
	if got, present := priced.Rate(); !present || got != rate {
		t.Fatalf("费率 = %v/%v", got, present)
	}

	if _, err := bare.WithEvaluation(domain.ChannelCostEvaluationReference{}); !errors.Is(err, domain.ErrInvalidSelectedChannelCandidate) {
		t.Fatalf("空白评价 err = %v", err)
	}
	if _, err := bare.WithRate(domain.ChannelRateReference{}); !errors.Is(err, domain.ErrInvalidSelectedChannelCandidate) {
		t.Fatalf("空白费率 err = %v", err)
	}
	if _, err := domain.NewSelectedChannelCandidate(domain.ChannelCandidateID{}); !errors.Is(err, domain.ErrInvalidSelectedChannelCandidate) {
		t.Fatalf("空白候选 err = %v", err)
	}
}

// Covers: 票 29 判据 2 的成本半边——已确立的成本取值能带上它按之出价的费率引用；出局的候选没有「适用的
// 费率」可言，带费率即拒（评价痕迹两格都能带，费率只有已确立那格能带）。
func TestAnEstablishedChannelCandidateCostCarriesTheRateItWasPricedAgainst(t *testing.T) {
	t.Parallel()

	rate := mustValue(t, domain.NewChannelRateReference, "SYN-BUY-PLAN/v3")
	priced, err := pricedCost(t, "SYN-CHANNEL-A", "12.50").WithRate(rate)
	if err != nil {
		t.Fatalf("已确立成本带费率：%v", err)
	}
	if got, present := priced.Rate(); !present || got != rate {
		t.Fatalf("费率 = %v/%v，want %v 在场", got, present, rate)
	}
	if _, present := pricedCost(t, "SYN-CHANNEL-B", "9.00").Rate(); present {
		t.Fatal("没带费率的成本报了在场")
	}
	if _, err := unpriceableCost(t, "SYN-CHANNEL-C", domain.ChannelCostRatecardExclusion).WithRate(rate); !errors.Is(err, domain.ErrInvalidChannelCandidateCost) {
		t.Fatalf("出局候选带费率 err = %v，want ErrInvalidChannelCandidateCost", err)
	}
}
