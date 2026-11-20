package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// RegisterDisposalRuleResult 是待认领处置规则登记的结果，复用登记册结果代数。
type RegisterDisposalRuleResult = RegistrationResult[domain.DisposalRuleRevision, domain.DisposalRuleField]

// RegisterDisposalRuleCommand 登记一个仓的待认领处置规则修订。Tenant 与 Registrant 取自信封。
type RegisterDisposalRuleCommand struct {
	Tenant     domain.TenantID
	Warehouse  domain.WarehouseCode
	Revision   int
	Content    domain.DisposalRuleContent
	Registrant domain.Registrant
}

// RegisterDisposalRuleHandler 是待认领处置规则修订册的写侧编排（登记册配置写面，不判准入范围）。
type RegisterDisposalRuleHandler struct {
	rules      ports.DisposalRuleRegistry
	warehouses ports.WarehouseLookup
	clock      ports.Clock
}

func NewRegisterDisposalRuleHandler(
	rules ports.DisposalRuleRegistry,
	warehouses ports.WarehouseLookup,
	clock ports.Clock,
) *RegisterDisposalRuleHandler {
	return &RegisterDisposalRuleHandler{rules: rules, warehouses: warehouses, clock: clock}
}

// Register 登记一笔待认领处置规则修订。判定次序（UC-WF-005、AT-WF-062）：
//  1. 按（租户，仓，修订号）认重放与冲突；
//  2. 核指名的仓在本租户已登记（停用也算；无仓或仓查不到答 NOT_ACCEPTED/REFERENCE_NOT_FOUND）；
//  3. 核修订号等于在册最新加一（首笔为 1）；
//  4. 核认领期限为正、不超过 time.Duration 能表示的最大整秒，且处置方式非空、非空白、不重复（否则 NOT_ACCEPTED/CONTENT_INVALID）；
//  5. 落册。
func (handler *RegisterDisposalRuleHandler) Register(
	ctx context.Context,
	command RegisterDisposalRuleCommand,
) (RegisterDisposalRuleResult, error) {
	candidate, err := domain.NewDisposalRuleRevision(
		command.Tenant, command.Warehouse, command.Revision, command.Content, command.Registrant, handler.clock.Now(),
	)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidRevisionNumber) {
			return handler.beforeSequence(ctx, command)
		}
		return RegisterDisposalRuleResult{}, fmt.Errorf("register unclaimed disposal rule: %w", err)
	}

	existing, found, err := handler.rules.LoadDisposalRuleRevision(ctx, command.Tenant, command.Warehouse, command.Revision)
	if err != nil {
		return RegisterDisposalRuleResult{}, fmt.Errorf("register unclaimed disposal rule: %w", err)
	}
	var latest domain.DisposalRuleRevision
	var hasLatest bool
	if !found {
		latest, hasLatest, err = handler.rules.LoadLatestDisposalRule(ctx, command.Tenant, command.Warehouse)
		if err != nil {
			return RegisterDisposalRuleResult{}, fmt.Errorf("register unclaimed disposal rule: %w", err)
		}
		if hasLatest && command.Revision <= latest.Revision() {
			existing, found, err = handler.rules.LoadDisposalRuleRevision(ctx, command.Tenant, command.Warehouse, command.Revision)
			if err != nil {
				return RegisterDisposalRuleResult{}, fmt.Errorf("register unclaimed disposal rule: %w", err)
			}
		}
	}
	if found {
		if existing.SameContentAs(candidate) {
			return disposalRuleAlreadyRegistered(existing), nil
		}
		return RegisterDisposalRuleResult{outcome: RegistrationContentConflict}, nil
	}

	_, warehouseRegistered, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return RegisterDisposalRuleResult{}, fmt.Errorf("register unclaimed disposal rule: %w", err)
	}
	if !warehouseRegistered {
		return disposalRuleRefused(RegistrationRefusalReferenceNotFound, domain.DisposalRuleFieldWarehouseCode), nil
	}

	latestRevision := 0
	if hasLatest {
		latestRevision = latest.Revision()
	}
	if command.Revision != latestRevision+1 {
		return disposalRuleOutOfSequenceAt(latestRevision), nil
	}
	if invalid := candidate.Content().InvalidField(); invalid != domain.DisposalRuleFieldInvalid {
		return disposalRuleRefused(RegistrationRefusalContentInvalid, invalid), nil
	}

	outcome, err := handler.rules.SaveDisposalRuleRevision(ctx, candidate)
	if err != nil {
		return RegisterDisposalRuleResult{}, fmt.Errorf("register unclaimed disposal rule: %w", err)
	}
	switch outcome {
	case ports.RegistryRevisionSaved:
		return RegisterDisposalRuleResult{outcome: RegistrationRegistered, registered: candidate, hasRegistered: true}, nil
	case ports.RegistryRevisionAlreadyRegistered:
		saved, exists, loadErr := handler.rules.LoadDisposalRuleRevision(ctx, command.Tenant, command.Warehouse, command.Revision)
		if loadErr != nil {
			return RegisterDisposalRuleResult{}, fmt.Errorf("register unclaimed disposal rule: %w", loadErr)
		}
		if !exists {
			return RegisterDisposalRuleResult{}, errors.New("register unclaimed disposal rule: 撞键后读不回既有行")
		}
		return disposalRuleAlreadyRegistered(saved), nil
	case ports.RegistryRevisionContentConflict:
		return RegisterDisposalRuleResult{outcome: RegistrationContentConflict}, nil
	default:
		return RegisterDisposalRuleResult{}, fmt.Errorf("register unclaimed disposal rule: unexpected save outcome %s", outcome)
	}
}

func (handler *RegisterDisposalRuleHandler) beforeSequence(
	ctx context.Context,
	command RegisterDisposalRuleCommand,
) (RegisterDisposalRuleResult, error) {
	if command.Tenant.String() == "" || command.Warehouse.String() == "" || command.Registrant.String() == "" {
		return RegisterDisposalRuleResult{}, errors.New("register unclaimed disposal rule: tenant, warehouse and registrant are required")
	}
	_, warehouseRegistered, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return RegisterDisposalRuleResult{}, fmt.Errorf("register unclaimed disposal rule: %w", err)
	}
	if !warehouseRegistered {
		return disposalRuleRefused(RegistrationRefusalReferenceNotFound, domain.DisposalRuleFieldWarehouseCode), nil
	}
	latest, hasLatest, err := handler.rules.LoadLatestDisposalRule(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return RegisterDisposalRuleResult{}, fmt.Errorf("register unclaimed disposal rule: %w", err)
	}
	if !hasLatest {
		return disposalRuleOutOfSequenceAt(0), nil
	}
	return disposalRuleOutOfSequenceAt(latest.Revision()), nil
}

func disposalRuleAlreadyRegistered(revision domain.DisposalRuleRevision) RegisterDisposalRuleResult {
	return RegisterDisposalRuleResult{outcome: RegistrationAlreadyRegistered, registered: revision, hasRegistered: true}
}

func disposalRuleOutOfSequenceAt(latest int) RegisterDisposalRuleResult {
	return RegisterDisposalRuleResult{
		outcome:        RegistrationNotAccepted,
		refusal:        RegistrationRefusalRevisionOutOfSequence,
		latestRevision: latest,
	}
}

func disposalRuleRefused(refusal RegistrationRefusal, field domain.DisposalRuleField) RegisterDisposalRuleResult {
	return RegisterDisposalRuleResult{outcome: RegistrationNotAccepted, refusal: refusal, field: field}
}

// QueryDisposalRulesHandler 是待认领处置规则的运营查阅用例：各仓当前规则分页列表与单个仓的修订史。
type QueryDisposalRulesHandler struct {
	catalogue ports.DisposalRuleCatalogueRead
	history   ports.DisposalRuleHistoryRead
}

func NewQueryDisposalRulesHandler(
	catalogue ports.DisposalRuleCatalogueRead,
	history ports.DisposalRuleHistoryRead,
) *QueryDisposalRulesHandler {
	return &QueryDisposalRulesHandler{catalogue: catalogue, history: history}
}

func (handler *QueryDisposalRulesHandler) ListDisposalRules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.DisposalRuleRow], error) {
	page, err := handler.catalogue.ListDisposalRules(ctx, tenant, limit, query)
	if err != nil {
		return cataloguepage.Result[ports.DisposalRuleRow]{}, fmt.Errorf("list unclaimed disposal rules: %w", err)
	}
	return page, nil
}

func (handler *QueryDisposalRulesHandler) DisposalRuleRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
) ([]ports.DisposalRuleRow, error) {
	rows, err := handler.history.ListDisposalRuleRevisions(ctx, tenant, warehouse)
	if err != nil {
		return nil, fmt.Errorf("list unclaimed disposal rule revisions: %w", err)
	}
	return rows, nil
}
