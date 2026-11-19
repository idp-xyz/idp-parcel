package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// RegisterLocationResult 是库位登记用例的结果：与仓、库区同一套代数（registration_result.go），Field 是库位修订上的格（见
// domain.LocationField），回显的是在册的库位修订。
type RegisterLocationResult = RegistrationResult[domain.LocationRevision, domain.LocationField]

// RegisterLocationCommand 登记库位的一笔修订：修订 1 是首次登记，其后是改库位类型、改优先级、停用与再启用，各占下一个修订号。
// Warehouse 是库位所属的仓，库位身份的一段，不是内容里的一格；所属库区在内容里，登记后不改。Tenant 与 Registrant 取自认证信封，
// 不取自载荷。
type RegisterLocationCommand struct {
	Tenant     domain.TenantID
	Warehouse  domain.WarehouseCode
	Code       domain.LocationCode
	Revision   int
	Content    domain.LocationContent
	Registrant domain.Registrant
}

// RegisterLocationHandler 是库位登记册的写侧编排。事务边界在调用方：每条命令各起一个环境事务，登记册的保存在没有环境事务时拒绝。
// 并发提交同一修订号的两笔不靠这里加锁——登记册的主键让至多一笔落册，另一笔的保存按内容答重放或冲突，再由这里译成结果。
//
// 对仓册、库区册与库位类型册只读不写，所以它们都只拿窄读口：各册只追加，登记过的对象不会再变成「不在册」，对它们的读不需要锁，
// 也不与库位的并发登记相干。
type RegisterLocationHandler struct {
	locations     ports.LocationRegistry
	warehouses    ports.WarehouseLookup
	zones         ports.ZoneLookup
	locationTypes ports.LocationTypeLookup
	clock         ports.Clock
}

func NewRegisterLocationHandler(
	locations ports.LocationRegistry,
	warehouses ports.WarehouseLookup,
	zones ports.ZoneLookup,
	locationTypes ports.LocationTypeLookup,
	clock ports.Clock,
) *RegisterLocationHandler {
	return &RegisterLocationHandler{
		locations: locations, warehouses: warehouses, zones: zones, locationTypes: locationTypes, clock: clock,
	}
}

// Register 登记一笔库位修订。判定次序（UC-WF-001「应用流程」）：
//
//  1. 按（身份，修订号）找在册的那一笔——任何一笔，不只是最新那一笔：内容相同答重放，不同答冲突，后面的核都不再过。
//     册上早就有的修订不受后来的核对牵连；
//  2. 都不是，核修订号等于在册最新加一（首笔必须是 1）；
//  3. 核所属库区没有改（登记后不改的格，CONTEXT「主数据登记」）；
//  4. 核指名的仓在本租户已登记，停用的仓也算已登记；
//  5. 核所属库区在本租户已登记且属于同一个仓：库区编码在另一个仓下存在，按查不到答；
//  6. 带库位类型的，核那个类型在本租户已登记；
//  7. 「新指名」的库位类型此刻已停用，答所指已停用。新指名即首笔修订、或在册最新那一笔没带库位类型、或带的是另一个类型编码；
//     沿用在册最新那一笔里的同一个类型编码、而它后来被停用了，照常受理；
//  8. 落册。
//
// 前面一格不成立就答前面那一格。仓、库区与库位类型放在最后核：它们是对别的册的读，只拦将要落册的新修订，重放与冲突不经它们。
// 别的租户的仓、库区与库位类型编码按查不到答，与没登记过的同一个答案：各册都按本租户读，答复里没有别的租户有没有它们的痕迹。
//
// 不核仓与库区是否停用：停用只挡此后的移入目标，不挡登记（CONTEXT「主数据登记」），停用的仓、停用的库区下面照常登记库位。
func (handler *RegisterLocationHandler) Register(
	ctx context.Context,
	command RegisterLocationCommand,
) (RegisterLocationResult, error) {
	candidate, err := domain.NewLocationRevision(
		command.Tenant, command.Warehouse, command.Code, command.Revision, command.Content,
		command.Registrant, handler.clock.Now(),
	)
	if err != nil {
		var invalid *domain.LocationContentFieldError
		switch {
		case errors.Is(err, domain.ErrInvalidRevisionNumber):
			// 小于 1 的修订号肯定不是「最新加一」，与跳号同答错位，登记方照回显的最新修订号重读册面。
			return handler.outOfSequence(ctx, command)
		case errors.As(err, &invalid):
			return locationRefused(RegistrationRefusalContentInvalid, invalid.Field), nil
		default:
			// 租户、仓编码、库位编码与登记责任方都来自接入信封；没经构造器是装配错了，不是登记方能改的输入。
			return RegisterLocationResult{}, fmt.Errorf("register location: %w", err)
		}
	}

	existing, found, err := handler.locations.LoadLocationRevision(ctx, command.Tenant, command.Warehouse, command.Code, command.Revision)
	if err != nil {
		return RegisterLocationResult{}, fmt.Errorf("register location: %w", err)
	}
	var latest domain.LocationRevision
	var hasLatest bool
	if !found {
		latest, hasLatest, err = handler.locations.LoadLatestLocation(ctx, command.Tenant, command.Warehouse, command.Code)
		if err != nil {
			return RegisterLocationResult{}, fmt.Errorf("register location: %w", err)
		}
		if hasLatest && command.Revision <= latest.Revision() {
			// 查册与读最新之间，别的事务把同一修订号落了册。回去再按在册的那一笔判：不然这一笔会被当成跳号答错位，
			// 而它该按内容答重放或冲突（UC-WF-001「一致性、幂等与并发」）。
			existing, found, err = handler.locations.LoadLocationRevision(ctx, command.Tenant, command.Warehouse, command.Code, command.Revision)
			if err != nil {
				return RegisterLocationResult{}, fmt.Errorf("register location: %w", err)
			}
		}
	}
	if found {
		if existing.SameContentAs(candidate) {
			return locationAlreadyRegistered(existing), nil
		}
		return RegisterLocationResult{outcome: RegistrationContentConflict}, nil
	}

	latestRevision := 0
	if hasLatest {
		latestRevision = latest.Revision()
	}
	if command.Revision != latestRevision+1 {
		return locationOutOfSequenceAt(latestRevision), nil
	}
	if hasLatest && latest.Content().ZoneCode() != candidate.Content().ZoneCode() {
		return locationRefused(RegistrationRefusalImmutableField, domain.LocationFieldZoneCode), nil
	}

	_, warehouseRegistered, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return RegisterLocationResult{}, fmt.Errorf("register location: %w", err)
	}
	if !warehouseRegistered {
		return locationRefused(RegistrationRefusalReferenceNotFound, domain.LocationFieldWarehouseCode), nil
	}
	// 键里带仓编码：读到就同时是库区在册与库区属于这个仓。
	_, zoneRegistered, err := handler.zones.LoadLatestZone(ctx, command.Tenant, command.Warehouse, candidate.Content().ZoneCode())
	if err != nil {
		return RegisterLocationResult{}, fmt.Errorf("register location: %w", err)
	}
	if !zoneRegistered {
		return locationRefused(RegistrationRefusalReferenceNotFound, domain.LocationFieldZoneCode), nil
	}
	if locationTypeCode, named := candidate.Content().LocationType(); named {
		locationType, typeRegistered, err := handler.locationTypes.LoadLatestLocationType(ctx, command.Tenant, locationTypeCode)
		if err != nil {
			return RegisterLocationResult{}, fmt.Errorf("register location: %w", err)
		}
		if !typeRegistered {
			return locationRefused(RegistrationRefusalReferenceNotFound, domain.LocationFieldLocationTypeCode), nil
		}
		// 停用只作用于此后的新指名：沿用在册最新那一笔里的同一个类型，它后来被停用了也照常受理。这一读与下面的落册不在同一把锁里，
		// 类型若恰在两者之间被别人停用，这一笔照常落册——那等同于这一笔先于那次停用登记，可以接受，不为它给类型册加锁。
		if locationType.Content().Deactivated() && candidate.Content().NamesLocationTypeNewly(latest.Content(), hasLatest) {
			return locationRefused(RegistrationRefusalReferenceDeactivated, domain.LocationFieldLocationTypeCode), nil
		}
	}

	outcome, err := handler.locations.SaveLocationRevision(ctx, candidate)
	if err != nil {
		return RegisterLocationResult{}, fmt.Errorf("register location: %w", err)
	}
	switch outcome {
	case ports.RegistryRevisionSaved:
		return RegisterLocationResult{
			outcome: RegistrationRegistered, registered: candidate, hasRegistered: true,
		}, nil
	case ports.RegistryRevisionAlreadyRegistered:
		// 并发里输掉的一笔：别人先落了同一修订号、内容相同。回显在册的那一份，不回显自己这笔。
		landed, found, err := handler.locations.LoadLocationRevision(ctx, command.Tenant, command.Warehouse, command.Code, command.Revision)
		if err != nil {
			return RegisterLocationResult{}, fmt.Errorf("register location: %w", err)
		}
		if !found {
			return RegisterLocationResult{}, errors.New("register location: 登记册答已在册，却读不回那一笔")
		}
		return locationAlreadyRegistered(landed), nil
	case ports.RegistryRevisionContentConflict:
		return RegisterLocationResult{outcome: RegistrationContentConflict}, nil
	default:
		return RegisterLocationResult{}, fmt.Errorf("register location: 登记册答出了未知落点 %d", outcome)
	}
}

// outOfSequence 取在册最新修订号再答错位，给修订号本身就立不住的命令用。
func (handler *RegisterLocationHandler) outOfSequence(
	ctx context.Context,
	command RegisterLocationCommand,
) (RegisterLocationResult, error) {
	latest, hasLatest, err := handler.locations.LoadLatestLocation(ctx, command.Tenant, command.Warehouse, command.Code)
	if err != nil {
		return RegisterLocationResult{}, fmt.Errorf("register location: %w", err)
	}
	if !hasLatest {
		return locationOutOfSequenceAt(0), nil
	}
	return locationOutOfSequenceAt(latest.Revision()), nil
}

func locationOutOfSequenceAt(latestRevision int) RegisterLocationResult {
	return RegisterLocationResult{
		outcome: RegistrationNotAccepted, refusal: RegistrationRefusalRevisionOutOfSequence, latestRevision: latestRevision,
	}
}

func locationRefused(refusal RegistrationRefusal, field domain.LocationField) RegisterLocationResult {
	return RegisterLocationResult{outcome: RegistrationNotAccepted, refusal: refusal, field: field}
}

func locationAlreadyRegistered(registered domain.LocationRevision) RegisterLocationResult {
	return RegisterLocationResult{outcome: RegistrationAlreadyRegistered, registered: registered, hasRegistered: true}
}
