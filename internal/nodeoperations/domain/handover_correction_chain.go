package domain

import "errors"

var ErrInvalidHandoverCorrectionChain = errors.New("node operations: invalid handover correction chain")

// HandoverCorrectionChain 是一版权威运输交接所在的那条更正链：这一版，与它沿 transport-fulfillment 的更正回指上溯的各前版，近者在前
// （ADR-0220）。
//
// 链的身份是 TF 的更正回指，不是（对象，范围）。TF 的首登口不问同一（对象，范围）下是否已有别的版本，换一个版本号就是另一次首登；
// 只有经更正口登记、回指前版的那一版才是前版的更正。本上下文若按（对象，范围）认链，同一交接范围下另起的一次交接就被当成前一次的
// 更正：前一次转出过，它零写入；前一次转入立起过控制，它不把那一份转出去——控制停在错的那次交接上，册上看不出来。
//
// TF 的更正只换版本、保留对象与范围，所以链上各版共有对象与范围。那是链的一条性质，构造时拿来验给的是不是一条链，不是认链的判据。
type HandoverCorrectionChain struct {
	head    AuthoritativeHandoverReference
	earlier []AuthoritativeHandoverReference
}

// NewHandoverCorrectionChain 由这一版与它沿更正回指上溯的各前版（近者在前；首登版一个也不给）立链。前版的对象或范围与这一版不同、
// 版本重复，都说明给的不是 TF 的更正链：TF 的更正只换版本，链线性，一版至多被更正一次。
func NewHandoverCorrectionChain(
	head AuthoritativeHandoverReference,
	earlier ...AuthoritativeHandoverReference,
) (HandoverCorrectionChain, error) {
	if !head.valid() {
		return HandoverCorrectionChain{}, ErrInvalidHandoverCorrectionChain
	}
	seen := map[string]bool{head.Version(): true}
	for _, predecessor := range earlier {
		if !predecessor.valid() || predecessor.object != head.object || predecessor.scope != head.scope ||
			seen[predecessor.Version()] {
			return HandoverCorrectionChain{}, ErrInvalidHandoverCorrectionChain
		}
		seen[predecessor.Version()] = true
	}
	return HandoverCorrectionChain{head: head, earlier: append([]AuthoritativeHandoverReference(nil), earlier...)}, nil
}

// Head 是立链时给的那一版：正在处理的这一版交接。
func (chain HandoverCorrectionChain) Head() AuthoritativeHandoverReference {
	return chain.head
}

// Members 交回链上各版：这一版在前，其后是各前版，近者在前。交回的是新切片。
func (chain HandoverCorrectionChain) Members() []AuthoritativeHandoverReference {
	members := make([]AuthoritativeHandoverReference, 0, len(chain.earlier)+1)
	return append(append(members, chain.head), chain.earlier...)
}

// Holds 报告 reference 是不是这条链上的一版，这一版自己也算。
func (chain HandoverCorrectionChain) Holds(reference AuthoritativeHandoverReference) bool {
	if reference == chain.head {
		return true
	}
	for _, predecessor := range chain.earlier {
		if reference == predecessor {
			return true
		}
	}
	return false
}

func (chain HandoverCorrectionChain) valid() bool {
	return chain.head.valid()
}
