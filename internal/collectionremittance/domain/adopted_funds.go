package domain

import (
	"errors"
	"fmt"
	"math"
)

var (
	// ErrAdoptedFundsCurrency：真实到账的币种与它引用的已采用事实不同。请求本身错了，改请求。
	ErrAdoptedFundsCurrency = errors.New("collection remittance: arrival currency differs from the adopted funds fact")
	// ErrAdoptedFundsExceeded：引用同一条已采用事实的各条真实到账，金额之和超过该事实此刻的金额。多出的部分没有来源，
	// 要人回头核对——到账登错了，或结算一侧把那条事实更正小了。
	ErrAdoptedFundsExceeded = errors.New("collection remittance: arrivals exceed the adopted funds fact")
	// ErrAdoptedFundsInstructionTaken：这条已采用事实已经支撑着同一条指令的另一条真实到账。一笔银行回款对一条指令
	// 只有一次到账；同一条指令的第二笔钱来自另一次回款，引用另一条事实。
	ErrAdoptedFundsInstructionTaken = errors.New("collection remittance: the adopted funds fact already backs an arrival of this instruction")
)

// AdoptedFundsReference 指名 settlement-accounting 已采用的一条外部资金事实。事实的来源身份与采用归结算，本上下文
// 只引用：同一笔银行回款只在结算里铸一次，真实到账以它为证据，不自铸一份。
type AdoptedFundsReference struct{ requiredValue }

func NewAdoptedFundsReference(value string) (AdoptedFundsReference, error) {
	required, err := newRequiredValue("adopted funds reference", value)
	return AdoptedFundsReference{required}, err
}

// AdoptedFundsVersion 是登记真实到账时读到的、那条已采用事实的链头版本。只为留痕：登记之后结算一侧的外部更正会换链头，
// 这个版本说的是「当时读到的是哪一版」，不参与重放比对，也不是之后判金额的依据——判金额永远读此刻的链头。
type AdoptedFundsVersion struct{ requiredValue }

func NewAdoptedFundsVersion(value string) (AdoptedFundsVersion, error) {
	required, err := newRequiredValue("adopted funds version", value)
	return AdoptedFundsVersion{required}, err
}

// AdoptedFunds 是按引用读回的一条已采用事实：此刻的链头版本与金额（币种在金额上），并且在结算一侧已被整条认作代收本金
// ——读口只把这样的事实交回，别的（没采用、采用了但没认作代收本金）一律读不到。
type AdoptedFunds struct {
	version AdoptedFundsVersion
	amount  Money
}

func NewAdoptedFunds(version AdoptedFundsVersion, amount Money) (AdoptedFunds, error) {
	if !version.valid() || !amount.valid() {
		return AdoptedFunds{}, fmt.Errorf("%w: adopted funds", ErrBlankValue)
	}
	return AdoptedFunds{version: version, amount: amount}, nil
}

func (funds AdoptedFunds) Version() AdoptedFundsVersion { return funds.version }

func (funds AdoptedFunds) Amount() Money { return funds.amount }

// AdmitAdoptedFundsReference 判一条新的（或重放的）真实到账能不能加入这条已采用事实名下已有的到账：币种相同；同一条
// 指令至多一条；金额之和（含它自己，不含它重放时已在册的自己）不超过事实此刻的金额。referenced 是现有全部引用这条
// 事实的真实到账。判在事实此刻的链头金额上，不在登记时钉的版本上：被结算更正小了的事实，容不下的新到账进不来。
func AdmitAdoptedFundsReference(funds AdoptedFunds, referenced []CollectionFact, candidate CollectionFact) error {
	if !candidate.layer.SettlesToOperator() {
		return fmt.Errorf("%w: only an operator bank credit references adopted funds", ErrInvalidCollectionFact)
	}
	if candidate.amount.currency != funds.amount.currency {
		return ErrAdoptedFundsCurrency
	}
	sum := int64(0)
	for _, existing := range referenced {
		if existing.id == candidate.id {
			continue
		}
		if existing.instruction == candidate.instruction {
			return ErrAdoptedFundsInstructionTaken
		}
		if sum > math.MaxInt64-existing.amount.amountMinor {
			return ErrAdoptedFundsExceeded
		}
		sum += existing.amount.amountMinor
	}
	if sum > math.MaxInt64-candidate.amount.amountMinor || sum+candidate.amount.amountMinor > funds.amount.amountMinor {
		return ErrAdoptedFundsExceeded
	}
	return nil
}

// AdoptedFundsCover 判现有全部引用这条已采用事实的真实到账，此刻是否仍被事实撑住：金额之和不超过链头金额。
// 移位记账之前问它——登记时撑得住的到账，之后结算把事实更正小了，就不能再把`渠道在途`移成`待清分`。
func AdoptedFundsCover(funds AdoptedFunds, referenced []CollectionFact) error {
	sum := int64(0)
	for _, existing := range referenced {
		if existing.amount.currency != funds.amount.currency {
			return ErrAdoptedFundsCurrency
		}
		if sum > math.MaxInt64-existing.amount.amountMinor {
			return ErrAdoptedFundsExceeded
		}
		sum += existing.amount.amountMinor
	}
	if sum > funds.amount.amountMinor {
		return ErrAdoptedFundsExceeded
	}
	return nil
}
