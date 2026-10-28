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
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
)

// 本文件是节点监管协作承接与执行事实登记两口的组合根（票 launch-wiring/04，UC-NO-001，ADR-0338）。

// transactionalCollaboration 把承接与执行事实登记各包进一笔事务：承接册、事实册的写口与发布意图的 Outbox 入队按框架合同无事务即拒，
// 事务边界归装配点；记录与意图同笔落地，意图入不了队整笔回滚、答 5xx（ADR-0043）。业务负向格（冲突、未受理、拒接、越出范围）本就没有写入，
// 提交与回滚无别。
type transactionalCollaboration struct {
	transactor bentoapp.Transactor
	inner      *nodeopsapp.AcceptCollaborationHandler
}

var _ nodeopshttp.CollaborationHandler = transactionalCollaboration{}

func (collaboration transactionalCollaboration) Accept(
	ctx context.Context,
	command nodeopsapp.AcceptCollaborationCommand,
) (nodeopsapp.CollaborationResult, error) {
	var result nodeopsapp.CollaborationResult
	err := collaboration.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		accepted, acceptErr := collaboration.inner.Accept(txCtx, command)
		if acceptErr != nil {
			return acceptErr
		}
		result = accepted
		return nil
	})
	if err != nil {
		return nodeopsapp.CollaborationResult{}, err
	}
	return result, nil
}

func (collaboration transactionalCollaboration) RecordExecution(
	ctx context.Context,
	command nodeopsapp.RecordExecutionCommand,
) (nodeopsapp.CollaborationResult, error) {
	var result nodeopsapp.CollaborationResult
	err := collaboration.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		recorded, recordErr := collaboration.inner.RecordExecution(txCtx, command)
		if recordErr != nil {
			return recordErr
		}
		result = recorded
		return nil
	})
	if err != nil {
		return nodeopsapp.CollaborationResult{}, err
	}
	return result, nil
}

// buildNodeCollaboration 装配两口背后的真编排。缝全接真：承接册、事实册、两份发布意图的 Outbox 交付与时钟。协作事项本体不经读口
// （关务一侧今天没有按引用读回的提供方），事项引用、节点与承接范围由作出决定的责任角色填进载荷，范围校验归领域。
func buildNodeCollaboration(db *bentopg.DB) (nodeopshttp.CollaborationHandler, error) {
	acceptances, err := nopostgres.NewCollaborationAcceptances(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: collaboration acceptances: %w", err)
	}
	facts, err := nopostgres.NewExecutionFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: execution facts: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	acknowledge, err := nopostgres.NewOutboxCollaborationAcceptanceHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: collaboration acceptance handoff: %w", err)
	}
	executions, err := nopostgres.NewOutboxExecutionFactHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: execution fact handoff: %w", err)
	}
	handler, err := nodeopsapp.NewAcceptCollaborationHandler(nodeopsapp.AcceptCollaborationDeps{
		Acceptances: acceptances,
		Facts:       facts,
		Acknowledge: acknowledge,
		Executions:  executions,
		Clock:       clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: node collaboration orchestration: %w", err)
	}
	return transactionalCollaboration{transactor: db.Transactor(), inner: handler}, nil
}

// buildNodeCollaborationExecutionIntake 装执行事实口的作业事实 Intake：核验方、操作者册、设备册与准入范围与收寄口、送站交接单三口是
// 同一组，铸造器无状态，分开建与共用一只答的是同一组格。
func buildNodeCollaborationExecutionIntake(
	verifier accessidentity.OperatorCredentialVerifier,
	operators accessidentity.OperatorRegistry,
	devices accessidentity.DeviceRegistry,
	admission accessidentity.AdmissionScope,
) (*nodeopshttp.WorkFactCollaborationIntake, error) {
	minter, err := accessidentity.NewWorkFactMinter(verifier, operators, devices, admission, time.Now)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: collaboration execution work fact minter: %w", err)
	}
	authenticator, err := noaccess.NewWorkFactAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return nodeopshttp.NewWorkFactCollaborationIntake(authenticator)
}
