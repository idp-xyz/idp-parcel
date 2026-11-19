package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// RegisterZoneResult 是库区登记用例的结果：与仓同一套代数（registration_result.go），Field 是库区修订上的格（见
// domain.ZoneField），回显的是在册的库区修订。
type RegisterZoneResult = RegistrationResult[domain.ZoneRevision, domain.ZoneField]

// RegisterZoneCommand 登记库区的一笔修订：修订 1 是首次登记，其后是改名称、停用与再启用，各占下一个修订号。Warehouse 是库区
// 所属的仓，库区身份的一段，不是内容里的一格。Tenant 与 Registrant 取自认证信封，不取自载荷。
type RegisterZoneCommand struct {
	Tenant     domain.TenantID
	Warehouse  domain.WarehouseCode
	Code       domain.ZoneCode
	Revision   int
	Content    domain.ZoneContent
	Registrant domain.Registrant
}

// RegisterZoneHandler 是库区登记册的写侧编排。事务边界在调用方：每条命令各起一个环境事务，登记册的保存在没有环境事务时拒绝。
// 并发提交同一修订号的两笔不靠这里加锁——登记册的主键让至多一笔落册，另一笔的保存按内容答重放或冲突，再由这里译成结果。
//
// 核「仓已登记」只读仓册，不写：仓册只追加，登记过的仓不会再变成「不在册」，所以这一读不需要锁，也不与库区的并发登记相干。
type RegisterZoneHandler struct {
	zones      ports.ZoneRegistry
	warehouses ports.WarehouseLookup
	clock      ports.Clock
}

func NewRegisterZoneHandler(
	zones ports.ZoneRegistry,
	warehouses ports.WarehouseLookup,
	clock ports.Clock,
) *RegisterZoneHandler {
	return &RegisterZoneHandler{zones: zones, warehouses: warehouses, clock: clock}
}

// Register 登记一笔库区修订。判定次序（UC-WF-001「应用流程」）：
//
//  1. 按（身份，修订号）找在册的那一笔——任何一笔，不只是最新那一笔：内容相同答重放，不同答冲突，后面的核都不再过。
//     册上早就有的修订不受后来的核对牵连。
//  2. 都不是，核修订号等于在册最新加一（首笔必须是 1）；
//  3. 核用途没有改（登记后不改的格，CONTEXT「主数据登记」）；
//  4. 核指名的仓在本租户已登记，停用的仓也算已登记。别的租户的仓编码按查不到答，与没登记过的仓同一个答案。
//
// 前面一格不成立就答前面那一格。仓放在最后核：它是对别的册的读，只拦将要落册的新修订，重放与冲突不经它。
func (handler *RegisterZoneHandler) Register(
	ctx context.Context,
	command RegisterZoneCommand,
) (RegisterZoneResult, error) {
	candidate, err := domain.NewZoneRevision(
		command.Tenant, command.Warehouse, command.Code, command.Revision, command.Content,
		command.Registrant, handler.clock.Now(),
	)
	if err != nil {
		var invalid *domain.ZoneContentFieldError
		switch {
		case errors.Is(err, domain.ErrInvalidRevisionNumber):
			// 小于 1 的修订号肯定不是「最新加一」，与跳号同答错位，登记方照回显的最新修订号重读册面。
			return handler.outOfSequence(ctx, command)
		case errors.As(err, &invalid):
			return zoneRefused(RegistrationRefusalContentInvalid, invalid.Field), nil
		default:
			// 租户、仓编码、库区编码与登记责任方都来自接入信封；没经构造器是装配错了，不是登记方能改的输入。
			return RegisterZoneResult{}, fmt.Errorf("register zone: %w", err)
		}
	}

	existing, found, err := handler.zones.LoadZoneRevision(ctx, command.Tenant, command.Warehouse, command.Code, command.Revision)
	if err != nil {
		return RegisterZoneResult{}, fmt.Errorf("register zone: %w", err)
	}
	var latest domain.ZoneRevision
	var hasLatest bool
	if !found {
		latest, hasLatest, err = handler.zones.LoadLatestZone(ctx, command.Tenant, command.Warehouse, command.Code)
		if err != nil {
			return RegisterZoneResult{}, fmt.Errorf("register zone: %w", err)
		}
		if hasLatest && command.Revision <= latest.Revision() {
			// 查册与读最新之间，别的事务把同一修订号落了册。回去再按在册的那一笔判：不然这一笔会被当成跳号答错位，
			// 而它该按内容答重放或冲突（UC-WF-001「一致性、幂等与并发」）。
			existing, found, err = handler.zones.LoadZoneRevision(ctx, command.Tenant, command.Warehouse, command.Code, command.Revision)
			if err != nil {
				return RegisterZoneResult{}, fmt.Errorf("register zone: %w", err)
			}
		}
	}
	if found {
		if existing.SameContentAs(candidate) {
			return zoneAlreadyRegistered(existing), nil
		}
		return RegisterZoneResult{outcome: RegistrationContentConflict}, nil
	}

	latestRevision := 0
	if hasLatest {
		latestRevision = latest.Revision()
	}
	if command.Revision != latestRevision+1 {
		return zoneOutOfSequenceAt(latestRevision), nil
	}
	if hasLatest && latest.Content().Usage() != candidate.Content().Usage() {
		return zoneRefused(RegistrationRefusalImmutableField, domain.ZoneFieldUsage), nil
	}
	_, warehouseRegistered, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return RegisterZoneResult{}, fmt.Errorf("register zone: %w", err)
	}
	if !warehouseRegistered {
		return zoneRefused(RegistrationRefusalReferenceNotFound, domain.ZoneFieldWarehouseCode), nil
	}

	outcome, err := handler.zones.SaveZoneRevision(ctx, candidate)
	if err != nil {
		return RegisterZoneResult{}, fmt.Errorf("register zone: %w", err)
	}
	switch outcome {
	case ports.RegistryRevisionSaved:
		return RegisterZoneResult{
			outcome: RegistrationRegistered, registered: candidate, hasRegistered: true,
		}, nil
	case ports.RegistryRevisionAlreadyRegistered:
		// 并发里输掉的一笔：别人先落了同一修订号、内容相同。回显在册的那一份，不回显自己这笔。
		landed, found, err := handler.zones.LoadZoneRevision(ctx, command.Tenant, command.Warehouse, command.Code, command.Revision)
		if err != nil {
			return RegisterZoneResult{}, fmt.Errorf("register zone: %w", err)
		}
		if !found {
			return RegisterZoneResult{}, errors.New("register zone: 登记册答已在册，却读不回那一笔")
		}
		return zoneAlreadyRegistered(landed), nil
	case ports.RegistryRevisionContentConflict:
		return RegisterZoneResult{outcome: RegistrationContentConflict}, nil
	default:
		return RegisterZoneResult{}, fmt.Errorf("register zone: 登记册答出了未知落点 %d", outcome)
	}
}

// outOfSequence 取在册最新修订号再答错位，给修订号本身就立不住的命令用。
func (handler *RegisterZoneHandler) outOfSequence(
	ctx context.Context,
	command RegisterZoneCommand,
) (RegisterZoneResult, error) {
	latest, hasLatest, err := handler.zones.LoadLatestZone(ctx, command.Tenant, command.Warehouse, command.Code)
	if err != nil {
		return RegisterZoneResult{}, fmt.Errorf("register zone: %w", err)
	}
	if !hasLatest {
		return zoneOutOfSequenceAt(0), nil
	}
	return zoneOutOfSequenceAt(latest.Revision()), nil
}

func zoneOutOfSequenceAt(latestRevision int) RegisterZoneResult {
	return RegisterZoneResult{
		outcome: RegistrationNotAccepted, refusal: RegistrationRefusalRevisionOutOfSequence, latestRevision: latestRevision,
	}
}

func zoneRefused(refusal RegistrationRefusal, field domain.ZoneField) RegisterZoneResult {
	return RegisterZoneResult{outcome: RegistrationNotAccepted, refusal: refusal, field: field}
}

func zoneAlreadyRegistered(registered domain.ZoneRevision) RegisterZoneResult {
	return RegisterZoneResult{outcome: RegistrationAlreadyRegistered, registered: registered, hasRegistered: true}
}
