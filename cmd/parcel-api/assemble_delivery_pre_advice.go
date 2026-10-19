package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	noaccess "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	noidentity "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
)

// buildCustomerPreAdviceIntake 装配预报两口的首方 Intake（ADR-0212 决定二；ADR-0139 决定四；票 ps-external-mark-relations/09）。铸造器
// 与提交口同一只（buildCustomerClientMinter）：首方族一个进程一只铸造器，准入范围是操作者族、集成客户端族共用的那一只——预报两口在
// 铸造侧判它（ADR-0139 决定八）。号码形状门问的是 PS 的入册门，与收寄那一侧同一个防腐适配器。
func buildCustomerPreAdviceIntake(minter *accessidentity.CustomerClientMinter) (*nodeopshttp.CustomerPreAdviceIntake, error) {
	if minter == nil {
		return nil, fmt.Errorf("customer pre-advice intake needs a minter")
	}
	authenticator, err := noaccess.NewCustomerPreAdviceAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	intake, err := nodeopshttp.NewCustomerPreAdviceIntake(authenticator, noparcel.PreAdvisedNumberGate{})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: customer pre-advice intake: %w", err)
	}
	return intake, nil
}

// customerPreAdviceIntakes 交回端点表两行用的 Intake：首方 Intake 缺席时两行都是字面量未配置即拒。在这里判 nil，是为了不把一个
// 带类型的空指针塞进接口——那样的 Intake 不等于 nil，第一次请求才会崩。
func customerPreAdviceIntakes(intake *nodeopshttp.CustomerPreAdviceIntake) (nodeopshttp.DeliveryPreAdviceSubmissionIntake, nodeopshttp.DeliveryPreAdviceWithdrawalIntake) {
	if intake == nil {
		return nodeopshttp.UnconfiguredIntake{}, nodeopshttp.UnconfiguredIntake{}
	}
	return intake, intake
}

// transactionalDeliveryPreAdvices 把预报的每次提交与撤回包进一笔事务，理由随 transactionalPreAdviceWindowRegistrar：预报册的写口按
// 框架合同无事务即拒，事务边界归装配点。
type transactionalDeliveryPreAdvices struct {
	transactor bentoapp.Transactor
	inner      *nodeopsapp.DeliveryPreAdviceHandler
}

var _ nodeopshttp.DeliveryPreAdviceHandler = transactionalDeliveryPreAdvices{}

func (handler transactionalDeliveryPreAdvices) Submit(
	ctx context.Context,
	command nodeopsapp.SubmitDeliveryPreAdviceCommand,
) (nodeopsapp.DeliveryPreAdviceSubmissionResult, error) {
	var result nodeopsapp.DeliveryPreAdviceSubmissionResult
	err := handler.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := handler.inner.Submit(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return nodeopsapp.DeliveryPreAdviceSubmissionResult{}, err
	}
	return result, nil
}

func (handler transactionalDeliveryPreAdvices) Withdraw(
	ctx context.Context,
	command nodeopsapp.WithdrawDeliveryPreAdviceCommand,
) (nodeopsapp.DeliveryPreAdviceWithdrawalResult, error) {
	var result nodeopsapp.DeliveryPreAdviceWithdrawalResult
	err := handler.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := handler.inner.Withdraw(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return nodeopsapp.DeliveryPreAdviceWithdrawalResult{}, err
	}
	return result, nil
}

// buildDeliveryPreAdviceOrchestration 装配预报两口背后的真编排（UC-NO-006 应用流程步骤 2）。缝——预报册、预报标识签发与时钟——全接
// 真；没有意图交付：预报是本上下文判交付范围时自己读的证据，没有下游要通知（ADR-0212 决定六「预报没有触发」）。
func buildDeliveryPreAdviceOrchestration(db *bentopg.DB) (nodeopshttp.DeliveryPreAdviceHandler, error) {
	register, err := nopostgres.NewDeliveryPreAdvices(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery pre-advices: %w", err)
	}
	ids, err := noidentity.NewDeliveryPreAdviceIDs()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery pre-advice IDs: %w", err)
	}
	handler, err := nodeopsapp.NewDeliveryPreAdviceHandler(nodeopsapp.DeliveryPreAdviceDeps{
		Register: register,
		IDs:      ids,
		Clock:    systemClock{},
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery pre-advice orchestration: %w", err)
	}
	return transactionalDeliveryPreAdvices{transactor: db.Transactor(), inner: handler}, nil
}
