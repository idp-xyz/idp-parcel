package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// resumeChain 是实测变化另起一代的两路编排（销售、采购）在代次链上走的同一套路：先沿后继走到链头，再沿前一代往回
// 找这次变化的来源是不是已经起过一代。规则只在这里有一份；两路只在请求记录的类型与读口上不同，由各自装配成本结构。
// 前一代该在而读不到是链断了，作错误交回，不当成没有前一代。
type resumeChain[R any, ID fmt.Stringer] struct {
	// label 冠在链断错误的前面，指明是哪一路的请求。
	label       string
	successorOf func(ctx context.Context, record R) (R, bool, error)
	basisOf     func(record R) domain.ResumeBasis
	previousOf  func(record R) (ID, bool)
	findByID    func(ctx context.Context, id ID) (R, bool, error)
}

// head 沿后继一路走到代次链的链头。
func (chain resumeChain[R, ID]) head(ctx context.Context, record R) (R, error) {
	for {
		successor, found, err := chain.successorOf(ctx, record)
		if err != nil {
			var zero R
			return zero, err
		}
		if !found {
			return record, nil
		}
		record = successor
	}
}

// basisAlreadyUsed 从给定的一代起沿前一代往回找：只要有一代的续办依据就是这次的，这次变化已经起过一代。
func (chain resumeChain[R, ID]) basisAlreadyUsed(ctx context.Context, record R, basis domain.ResumeBasis) (bool, error) {
	for {
		if chain.basisOf(record) == basis {
			return true, nil
		}
		previous, has := chain.previousOf(record)
		if !has {
			return false, nil
		}
		earlier, found, err := chain.findByID(ctx, previous)
		if err != nil {
			return false, err
		}
		if !found {
			return false, fmt.Errorf("%s: previous request %s is missing", chain.label, previous)
		}
		record = earlier
	}
}
