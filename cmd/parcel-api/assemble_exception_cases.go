package main

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// exceptionCaseFaces 把接单、关闭与归并各包进一笔事务：案件册的写口按框架合同无事务即拒，事务边界归装配点。壳复用关务
// 各决定口那一只（它只认「答复要不要回滚」，不认上下文）：未决整笔回滚、再交回编排那一份答复。不发布意图——案件的推进
// 今天没有订阅者。
type exceptionCaseFaces struct {
	takeUp  customsDecisionTransaction[visibilityapp.TakeUpCaseCommand, visibilityapp.ManageCaseResult]
	closure customsDecisionTransaction[visibilityapp.CloseCaseCommand, visibilityapp.ManageCaseResult]
	merge   customsDecisionTransaction[visibilityapp.MergeCaseCommand, visibilityapp.ManageCaseResult]
}

func (faces exceptionCaseFaces) TakeUp(ctx context.Context, command visibilityapp.TakeUpCaseCommand) (visibilityapp.ManageCaseResult, error) {
	return faces.takeUp.Handle(ctx, command)
}

func (faces exceptionCaseFaces) Close(ctx context.Context, command visibilityapp.CloseCaseCommand) (visibilityapp.ManageCaseResult, error) {
	return faces.closure.Handle(ctx, command)
}

func (faces exceptionCaseFaces) Merge(ctx context.Context, command visibilityapp.MergeCaseCommand) (visibilityapp.ManageCaseResult, error) {
	return faces.merge.Handle(ctx, command)
}

// buildExceptionCaseFaces 装配异常案件三口背后的真编排：缝只有案件册一只与生产时钟。案件由分诊规则命中建立；册上没有
// 案件时三口都如实答不在册。
func buildExceptionCaseFaces(db *bentopg.DB) (exceptionCaseFaces, error) {
	cases, err := vepostgres.NewExceptionCaseStore(db)
	if err != nil {
		return exceptionCaseFaces{}, fmt.Errorf("parcel-api: exception cases: %w", err)
	}
	handler, err := visibilityapp.NewManageCaseHandler(visibilityapp.ManageCaseDeps{Cases: cases, Clock: systemClock{}})
	if err != nil {
		return exceptionCaseFaces{}, fmt.Errorf("parcel-api: manage exception case: %w", err)
	}
	undecided := func(result visibilityapp.ManageCaseResult) bool {
		return result.Outcome() == visibilityapp.CaseUndecided
	}
	transactor := db.Transactor()
	return exceptionCaseFaces{
		takeUp: customsDecisionTransaction[visibilityapp.TakeUpCaseCommand, visibilityapp.ManageCaseResult]{
			transactor: transactor, handle: handler.TakeUp, rollBack: undecided,
		},
		closure: customsDecisionTransaction[visibilityapp.CloseCaseCommand, visibilityapp.ManageCaseResult]{
			transactor: transactor, handle: handler.Close, rollBack: undecided,
		},
		merge: customsDecisionTransaction[visibilityapp.MergeCaseCommand, visibilityapp.ManageCaseResult]{
			transactor: transactor, handle: handler.Merge, rollBack: undecided,
		},
	}, nil
}
