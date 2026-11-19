package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// RegisterLocationTypeResult 是库位类型登记用例的结果：与仓、库区同一套代数（registration_result.go），Field 是库位类型修订上的格
// （见 domain.LocationTypeField），回显的是在册的库位类型修订。
type RegisterLocationTypeResult = RegistrationResult[domain.LocationTypeRevision, domain.LocationTypeField]

// RegisterLocationTypeCommand 登记库位类型的一笔修订：修订 1 是首次登记，其后是改名称、停用与再启用，各占下一个修订号。库位类型按租户登记、
// 各仓共用，身份里没有仓编码。Tenant 与 Registrant 取自认证信封，不取自载荷。
type RegisterLocationTypeCommand struct {
	Tenant     domain.TenantID
	Code       domain.LocationTypeCode
	Revision   int
	Content    domain.LocationTypeContent
	Registrant domain.Registrant
}

// RegisterLocationTypeHandler 是库位类型登记册的写侧编排。事务边界在调用方：每条命令各起一个环境事务，登记册的保存在没有环境事务时拒绝。
// 并发提交同一修订号的两笔不靠这里加锁——登记册的主键让至多一笔落册，另一笔的保存按内容答重放或冲突，再由这里译成结果。
//
// 「本租户有没有登记过仓」只读仓册，不写：仓册只追加，登记过仓的租户不会再回到「一个仓都没有」，所以这一读不需要锁。
type RegisterLocationTypeHandler struct {
	locationTypes ports.LocationTypeRegistry
	warehouses    ports.TenantWarehouseLookup
	clock         ports.Clock
}

func NewRegisterLocationTypeHandler(
	locationTypes ports.LocationTypeRegistry,
	warehouses ports.TenantWarehouseLookup,
	clock ports.Clock,
) *RegisterLocationTypeHandler {
	return &RegisterLocationTypeHandler{locationTypes: locationTypes, warehouses: warehouses, clock: clock}
}

// Register 登记一笔库位类型修订。判定次序（UC-WF-001「应用流程」）：
//
//  1. 按（身份，修订号）找在册的那一笔——任何一笔，不只是最新那一笔：内容相同答重放，不同答冲突，后面的核都不再过。
//     册上早就有的修订不受后来仓册的变化牵连；
//  2. 都不是，看本租户有没有登记过仓（停用的仓也算）：一个都没有答 NOT_CONFIGURED——租户取值还没到，不是拒绝，不落册；
//  3. 核修订号等于在册最新加一（首笔必须是 1）；
//  4. 落册。
//
// 库位类型没有登记后不改的格，也没有指名别的对象的格，所以没有「不可改的格」与「查无所指」两道核。前面一格不成立就答前面那一格。
func (handler *RegisterLocationTypeHandler) Register(
	ctx context.Context,
	command RegisterLocationTypeCommand,
) (RegisterLocationTypeResult, error) {
	candidate, err := domain.NewLocationTypeRevision(
		command.Tenant, command.Code, command.Revision, command.Content, command.Registrant, handler.clock.Now(),
	)
	if err != nil {
		var invalid *domain.LocationTypeContentFieldError
		switch {
		case errors.Is(err, domain.ErrInvalidRevisionNumber):
			// 小于 1 的修订号肯定不是「最新加一」，与跳号同答错位；但「一个仓都没登记」排在核修订连续之前，所以先过那一道。
			return handler.beforeSequence(ctx, command)
		case errors.As(err, &invalid):
			return locationTypeRefused(RegistrationRefusalContentInvalid, invalid.Field), nil
		default:
			// 租户、库位类型编码与登记责任方都来自接入信封；没经构造器是装配错了，不是登记方能改的输入。
			return RegisterLocationTypeResult{}, fmt.Errorf("register location type: %w", err)
		}
	}

	existing, found, err := handler.locationTypes.LoadLocationTypeRevision(ctx, command.Tenant, command.Code, command.Revision)
	if err != nil {
		return RegisterLocationTypeResult{}, fmt.Errorf("register location type: %w", err)
	}
	var latest domain.LocationTypeRevision
	var hasLatest bool
	if !found {
		latest, hasLatest, err = handler.locationTypes.LoadLatestLocationType(ctx, command.Tenant, command.Code)
		if err != nil {
			return RegisterLocationTypeResult{}, fmt.Errorf("register location type: %w", err)
		}
		if hasLatest && command.Revision <= latest.Revision() {
			// 查册与读最新之间，别的事务把同一修订号落了册。回去再按在册的那一笔判：不然这一笔会被当成跳号答错位，
			// 而它该按内容答重放或冲突（UC-WF-001「一致性、幂等与并发」）。
			existing, found, err = handler.locationTypes.LoadLocationTypeRevision(ctx, command.Tenant, command.Code, command.Revision)
			if err != nil {
				return RegisterLocationTypeResult{}, fmt.Errorf("register location type: %w", err)
			}
		}
	}
	if found {
		if existing.SameContentAs(candidate) {
			return locationTypeAlreadyRegistered(existing), nil
		}
		return RegisterLocationTypeResult{outcome: RegistrationContentConflict}, nil
	}

	configured, err := handler.warehouses.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return RegisterLocationTypeResult{}, fmt.Errorf("register location type: %w", err)
	}
	if !configured {
		return locationTypeNotConfigured(), nil
	}

	latestRevision := 0
	if hasLatest {
		latestRevision = latest.Revision()
	}
	if command.Revision != latestRevision+1 {
		return locationTypeOutOfSequenceAt(latestRevision), nil
	}

	outcome, err := handler.locationTypes.SaveLocationTypeRevision(ctx, candidate)
	if err != nil {
		return RegisterLocationTypeResult{}, fmt.Errorf("register location type: %w", err)
	}
	switch outcome {
	case ports.RegistryRevisionSaved:
		return RegisterLocationTypeResult{
			outcome: RegistrationRegistered, registered: candidate, hasRegistered: true,
		}, nil
	case ports.RegistryRevisionAlreadyRegistered:
		// 并发里输掉的一笔：别人先落了同一修订号、内容相同。回显在册的那一份，不回显自己这笔。
		landed, found, err := handler.locationTypes.LoadLocationTypeRevision(ctx, command.Tenant, command.Code, command.Revision)
		if err != nil {
			return RegisterLocationTypeResult{}, fmt.Errorf("register location type: %w", err)
		}
		if !found {
			return RegisterLocationTypeResult{}, errors.New("register location type: 登记册答已在册，却读不回那一笔")
		}
		return locationTypeAlreadyRegistered(landed), nil
	case ports.RegistryRevisionContentConflict:
		return RegisterLocationTypeResult{outcome: RegistrationContentConflict}, nil
	default:
		return RegisterLocationTypeResult{}, fmt.Errorf("register location type: 登记册答出了未知落点 %d", outcome)
	}
}

// beforeSequence 给修订号本身就立不住的命令用：这样的修订号在册上不可能有对应的修订，没有重放与冲突要判；先过「一个仓都没登记」，
// 再取在册最新修订号答错位。
func (handler *RegisterLocationTypeHandler) beforeSequence(
	ctx context.Context,
	command RegisterLocationTypeCommand,
) (RegisterLocationTypeResult, error) {
	configured, err := handler.warehouses.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return RegisterLocationTypeResult{}, fmt.Errorf("register location type: %w", err)
	}
	if !configured {
		return locationTypeNotConfigured(), nil
	}
	latest, hasLatest, err := handler.locationTypes.LoadLatestLocationType(ctx, command.Tenant, command.Code)
	if err != nil {
		return RegisterLocationTypeResult{}, fmt.Errorf("register location type: %w", err)
	}
	if !hasLatest {
		return locationTypeOutOfSequenceAt(0), nil
	}
	return locationTypeOutOfSequenceAt(latest.Revision()), nil
}

func locationTypeNotConfigured() RegisterLocationTypeResult {
	return RegisterLocationTypeResult{outcome: RegistrationNotConfigured}
}

func locationTypeOutOfSequenceAt(latestRevision int) RegisterLocationTypeResult {
	return RegisterLocationTypeResult{
		outcome: RegistrationNotAccepted, refusal: RegistrationRefusalRevisionOutOfSequence, latestRevision: latestRevision,
	}
}

func locationTypeRefused(refusal RegistrationRefusal, field domain.LocationTypeField) RegisterLocationTypeResult {
	return RegisterLocationTypeResult{outcome: RegistrationNotAccepted, refusal: refusal, field: field}
}

func locationTypeAlreadyRegistered(registered domain.LocationTypeRevision) RegisterLocationTypeResult {
	return RegisterLocationTypeResult{outcome: RegistrationAlreadyRegistered, registered: registered, hasRegistered: true}
}
