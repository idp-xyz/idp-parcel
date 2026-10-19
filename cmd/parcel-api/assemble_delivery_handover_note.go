package main

import (
	"context"
	"fmt"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	noaccess "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopc "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/partycommercial"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
)

// deliveryHandoverNoteFaces 是送站交接单五口在本装配点的件（ADR-0212 决定三；票 ps-external-mark-relations/12）：作业事实登记面
// 的三口共用一只 Intake，客户面一口一只 Intake，五口共用一只编排；运营决定确认口的 Intake 是 operatorDecisions.nodeOperations。
// Intake 缺席即那几行是字面量未配置。
type deliveryHandoverNoteFaces struct {
	workFacts  *nodeopshttp.WorkFactHandoverNoteIntake
	conclusion *nodeopshttp.CustomerHandoverConclusionIntake
	handler    nodeopshttp.DeliveryHandoverNoteHandler
}

// 以下四个在 nil 时交回字面量未配置：不把一个带类型的空指针塞进接口——那样的 Intake 不等于 nil，第一次请求才会崩。
func (faces deliveryHandoverNoteFaces) openingIntake() nodeopshttp.DeliveryHandoverNoteOpeningIntake {
	if faces.workFacts == nil {
		return nodeopshttp.UnconfiguredIntake{}
	}
	return faces.workFacts
}

func (faces deliveryHandoverNoteFaces) assignmentIntake() nodeopshttp.DeliveryHandoverAssignmentIntake {
	if faces.workFacts == nil {
		return nodeopshttp.UnconfiguredIntake{}
	}
	return faces.workFacts
}

func (faces deliveryHandoverNoteFaces) closureIntake() nodeopshttp.DeliveryHandoverClosureIntake {
	if faces.workFacts == nil {
		return nodeopshttp.UnconfiguredIntake{}
	}
	return faces.workFacts
}

func (faces deliveryHandoverNoteFaces) conclusionIntake() nodeopshttp.DeliveryHandoverConclusionIntake {
	if faces.conclusion == nil {
		return nodeopshttp.UnconfiguredIntake{}
	}
	return faces.conclusion
}

// buildDeliveryHandoverNoteFaces 装五口的件。作业事实三口另起一只作业事实铸造器：核验方、操作者册、设备册与准入范围与收寄口那一只
// 是同一组，铸造器无状态，分开建与共用一只答的是同一组格。客户结论口用提交口那一只首方铸造器（首方族一个进程一只）。
func buildDeliveryHandoverNoteFaces(
	db *bentopg.DB,
	verifier accessidentity.OperatorCredentialVerifier,
	operators accessidentity.OperatorRegistry,
	devices accessidentity.DeviceRegistry,
	admission accessidentity.AdmissionScope,
	customers *accessidentity.CustomerClientMinter,
) (deliveryHandoverNoteFaces, error) {
	workFacts, err := buildDeliveryHandoverNoteWorkFactIntake(verifier, operators, devices, admission)
	if err != nil {
		return deliveryHandoverNoteFaces{}, err
	}
	conclusion, err := buildCustomerHandoverConclusionIntake(customers)
	if err != nil {
		return deliveryHandoverNoteFaces{}, err
	}
	handler, err := buildDeliveryHandoverNoteOrchestration(db)
	if err != nil {
		return deliveryHandoverNoteFaces{}, err
	}
	return deliveryHandoverNoteFaces{workFacts: workFacts, conclusion: conclusion, handler: handler}, nil
}

// buildDeliveryHandoverNoteWorkFactIntake 装作业事实登记面在交接单三口的 Intake：认证方按开立、归入、关闭各判自己那一种事实的准入
// 范围（ADR-0149 决定四第二条）。准入范围是操作者族、集成客户端族、首方族共用的那一只。
func buildDeliveryHandoverNoteWorkFactIntake(
	verifier accessidentity.OperatorCredentialVerifier,
	operators accessidentity.OperatorRegistry,
	devices accessidentity.DeviceRegistry,
	admission accessidentity.AdmissionScope,
) (*nodeopshttp.WorkFactHandoverNoteIntake, error) {
	minter, err := accessidentity.NewWorkFactMinter(verifier, operators, devices, admission, time.Now)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: handover note work fact minter: %w", err)
	}
	authenticator, err := noaccess.NewWorkFactAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return nodeopshttp.NewWorkFactHandoverNoteIntake(authenticator)
}

// buildCustomerHandoverConclusionIntake 装客户结论口的首方 Intake：准入范围在铸造侧判（ADR-0139 决定八）。
func buildCustomerHandoverConclusionIntake(minter *accessidentity.CustomerClientMinter) (*nodeopshttp.CustomerHandoverConclusionIntake, error) {
	if minter == nil {
		return nil, fmt.Errorf("customer handover conclusion intake needs a minter")
	}
	authenticator, err := noaccess.NewCustomerHandoverConclusionAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return nodeopshttp.NewCustomerHandoverConclusionIntake(authenticator)
}

// buildDeliveryHandoverNoteOrchestration 装五口背后的真编排（UC-NO-006 应用流程步骤 3 至 6）：交接单册、收寄库（归入取实物锁那一
// 问）、授权交付判断经本上下文的防腐适配器问 party-commercial 的真库读口（票 10 立的那一对，装配在这里）、「交付范围成立」门铃写
// outbox。
func buildDeliveryHandoverNoteOrchestration(db *bentopg.DB) (nodeopshttp.DeliveryHandoverNoteHandler, error) {
	notes, err := nopostgres.NewDeliveryHandoverNotes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery handover notes: %w", err)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery handover receptions: %w", err)
	}
	judgments, err := pcpostgres.NewAuthorizedDeliveryJudgments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: authorized delivery judgments: %w", err)
	}
	authorized, err := nopc.NewAuthorizedDeliveryView(judgments)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: authorized delivery view: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery scope doorbell outbox store: %w", err)
	}
	clock := systemClock{}
	doorbell, err := nopostgres.NewOutboxDeliveryScopeEstablishedDoorbell(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery scope doorbell: %w", err)
	}
	handler, err := nodeopsapp.NewDeliveryHandoverNoteHandler(nodeopsapp.DeliveryHandoverDeps{
		Notes:              notes,
		Receptions:         receptions,
		AuthorizedDelivery: authorized,
		Doorbell:           doorbell,
		Clock:              clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery handover orchestration: %w", err)
	}
	return transactionalDeliveryHandoverNotes{transactor: db.Transactor(), inner: handler}, nil
}

// transactionalDeliveryHandoverNotes 把五口的每次调用包进一笔事务：交接单册的写口无事务即拒；归入的实物锁、结论的交接单锁、落下的
// 成员或结论与它的门铃必须同生共死。
type transactionalDeliveryHandoverNotes struct {
	transactor bentoapp.Transactor
	inner      *nodeopsapp.DeliveryHandoverNoteHandler
}

var _ nodeopshttp.DeliveryHandoverNoteHandler = transactionalDeliveryHandoverNotes{}

func withinHandoverTransaction[Command any](
	ctx context.Context,
	transactor bentoapp.Transactor,
	handle func(context.Context, Command) (nodeopsapp.DeliveryHandoverResult, error),
	command Command,
) (nodeopsapp.DeliveryHandoverResult, error) {
	var result nodeopsapp.DeliveryHandoverResult
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return nodeopsapp.DeliveryHandoverResult{}, err
	}
	return result, nil
}

func (handler transactionalDeliveryHandoverNotes) Open(ctx context.Context, command nodeopsapp.OpenDeliveryHandoverNoteCommand) (nodeopsapp.DeliveryHandoverResult, error) {
	return withinHandoverTransaction(ctx, handler.transactor, handler.inner.Open, command)
}

func (handler transactionalDeliveryHandoverNotes) Assign(ctx context.Context, command nodeopsapp.AssignToDeliveryHandoverNoteCommand) (nodeopsapp.DeliveryHandoverResult, error) {
	return withinHandoverTransaction(ctx, handler.transactor, handler.inner.Assign, command)
}

func (handler transactionalDeliveryHandoverNotes) Close(ctx context.Context, command nodeopsapp.CloseDeliveryHandoverNoteCommand) (nodeopsapp.DeliveryHandoverResult, error) {
	return withinHandoverTransaction(ctx, handler.transactor, handler.inner.Close, command)
}

func (handler transactionalDeliveryHandoverNotes) ConfirmByOperationDecision(ctx context.Context, command nodeopsapp.ConfirmDeliveryHandoverCommand) (nodeopsapp.DeliveryHandoverResult, error) {
	return withinHandoverTransaction(ctx, handler.transactor, handler.inner.ConfirmByOperationDecision, command)
}

func (handler transactionalDeliveryHandoverNotes) ConcludeByCustomer(ctx context.Context, command nodeopsapp.ConcludeDeliveryHandoverCommand) (nodeopsapp.DeliveryHandoverResult, error) {
	return withinHandoverTransaction(ctx, handler.transactor, handler.inner.ConcludeByCustomer, command)
}
