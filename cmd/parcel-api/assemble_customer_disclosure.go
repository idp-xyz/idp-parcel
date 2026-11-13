package main

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	veps "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// customerDisclosureFaces 把披露判断与披露授权各包进一笔事务：披露决定册的写口按框架合同无事务即拒，事务边界归装配点。
// 壳复用关务各决定口那一只：未决整笔回滚、再交回编排那一份答复。这里不发通知——`披露`结论的通知是另一步，由通知
// 编排按已登记的决定另起一轮。
type customerDisclosureFaces struct {
	decision      customsDecisionTransaction[visibilityapp.DecideDisclosureCommand, visibilityapp.DecideDisclosureResult]
	authorization customsDecisionTransaction[visibilityapp.AuthorizeDisclosureCommand, visibilityapp.DecideDisclosureResult]
}

func (faces customerDisclosureFaces) Handle(ctx context.Context, command visibilityapp.DecideDisclosureCommand) (visibilityapp.DecideDisclosureResult, error) {
	return faces.decision.Handle(ctx, command)
}

func (faces customerDisclosureFaces) Authorize(ctx context.Context, command visibilityapp.AuthorizeDisclosureCommand) (visibilityapp.DecideDisclosureResult, error) {
	return faces.authorization.Handle(ctx, command)
}

// buildCustomerDisclosureFaces 装配两口背后的真编排：信号发作期、包裹的客户归属（parcel-shipment 的当前已接受委托）、
// 披露决定册（三态落库与人的那一半是同一只适配器的两面）与生产时钟。披露规则的读面在构造时钉住租户，而租户随每次
// 请求的认证结果到达，所以规则读面与编排逐请求构造；它们只持有连接池的引用，构造不触库。
func buildCustomerDisclosureFaces(db *bentopg.DB) (customerDisclosureFaces, error) {
	episodes, err := vepostgres.NewSignalEpisodes(db)
	if err != nil {
		return customerDisclosureFaces{}, fmt.Errorf("parcel-api: disclosure signal episodes: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return customerDisclosureFaces{}, fmt.Errorf("parcel-api: disclosure shipment requests: %w", err)
	}
	customers, err := veps.NewParcelCustomerAccountLookup(requests)
	if err != nil {
		return customerDisclosureFaces{}, fmt.Errorf("parcel-api: disclosure customer accounts: %w", err)
	}
	decisions, err := vepostgres.NewDisclosureDecisions(db)
	if err != nil {
		return customerDisclosureFaces{}, fmt.Errorf("parcel-api: disclosure decisions: %w", err)
	}
	handlerFor := func(tenant domain.TenantID) (*visibilityapp.DecideDisclosureHandler, error) {
		rules, err := vepostgres.NewExceptionDisclosureRules(db, tenant)
		if err != nil {
			return nil, fmt.Errorf("parcel-api: disclosure rules: %w", err)
		}
		return visibilityapp.NewDecideDisclosureHandler(visibilityapp.DecideDisclosureDeps{
			Episodes: episodes, EpisodeByID: episodes, Customers: customers, Rules: rules, Decisions: decisions, Ledger: decisions, Clock: systemClock{},
		}), nil
	}
	undecided := func(result visibilityapp.DecideDisclosureResult) bool {
		return result.Outcome() == visibilityapp.DecideDisclosureUndecided
	}
	transactor := db.Transactor()
	return customerDisclosureFaces{
		decision: customsDecisionTransaction[visibilityapp.DecideDisclosureCommand, visibilityapp.DecideDisclosureResult]{
			transactor: transactor, rollBack: undecided,
			handle: func(ctx context.Context, command visibilityapp.DecideDisclosureCommand) (visibilityapp.DecideDisclosureResult, error) {
				handler, err := handlerFor(command.TenantID)
				if err != nil {
					return visibilityapp.DecideDisclosureResult{}, err
				}
				return handler.Handle(ctx, command)
			},
		},
		authorization: customsDecisionTransaction[visibilityapp.AuthorizeDisclosureCommand, visibilityapp.DecideDisclosureResult]{
			transactor: transactor, rollBack: undecided,
			handle: func(ctx context.Context, command visibilityapp.AuthorizeDisclosureCommand) (visibilityapp.DecideDisclosureResult, error) {
				handler, err := handlerFor(command.TenantID)
				if err != nil {
					return visibilityapp.DecideDisclosureResult{}, err
				}
				return handler.Authorize(ctx, command)
			},
		},
	}, nil
}
