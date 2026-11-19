package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/refdata"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 仓这一册沿用的结果名字：结果代数是各册共用的一套（registration_result.go），这里的名字是同一套类型与取值的别名，仓一侧的
// 编排、译装与用例不必跟着换名。对外的字面量、答复体与状态码都由那一套给出，别名不另立。
type (
	RegisterWarehouseOutcome = RegistrationOutcome
	RegisterWarehouseRefusal = RegistrationRefusal
	// RegisterWarehouseResult 的 Field 是仓修订上的格（见 domain.WarehouseField），回显的是在册的仓修订。
	RegisterWarehouseResult = RegistrationResult[domain.WarehouseRevision, domain.WarehouseField]
)

const (
	RegisterWarehouseOutcomeInvalid = RegistrationOutcomeInvalid
	WarehouseRegistered             = RegistrationRegistered
	WarehouseAlreadyRegistered      = RegistrationAlreadyRegistered
	WarehouseContentConflict        = RegistrationContentConflict
	WarehouseNotAccepted            = RegistrationNotAccepted

	WarehouseRefusalNone                  = RegistrationRefusalNone
	WarehouseRefusalRevisionOutOfSequence = RegistrationRefusalRevisionOutOfSequence
	WarehouseRefusalImmutableField        = RegistrationRefusalImmutableField
	WarehouseRefusalContentInvalid        = RegistrationRefusalContentInvalid
)

// RegisterWarehouseCommand 登记仓的一笔修订：修订 1 是首次登记，其后是改资料、停用与再启用，各占下一个修订号。
// Tenant 与 Registrant 取自认证信封，不取自载荷。
type RegisterWarehouseCommand struct {
	Tenant     domain.TenantID
	Code       domain.WarehouseCode
	Revision   int
	Content    domain.WarehouseContent
	Registrant domain.Registrant
}

// RegisterWarehouseHandler 是仓登记册的写侧编排。事务边界在调用方：每条命令各起一个环境事务，登记册的保存在没有环境事务时拒绝。
// 并发提交同一修订号的两笔不靠这里加锁——登记册的主键让至多一笔落册，另一笔的保存按内容答重放或冲突，再由这里译成结果。
type RegisterWarehouseHandler struct {
	registry ports.WarehouseRegistry
	clock    ports.Clock
}

func NewRegisterWarehouseHandler(registry ports.WarehouseRegistry, clock ports.Clock) *RegisterWarehouseHandler {
	return &RegisterWarehouseHandler{registry: registry, clock: clock}
}

// Register 登记一笔仓修订。判定次序（UC-WF-001「应用流程」）：
//
//  1. 按（身份，修订号）找在册的那一笔——任何一笔，不只是最新那一笔：内容相同答重放，不同答冲突，后面的核都不再过。
//     册上早就有的修订不受后来的目录改版与时区库更新牵连。
//  2. 都不是，核修订号等于在册最新加一（首笔必须是 1）；
//  3. 核种类没有改（登记后不改的格，CONTEXT「主数据登记」）；
//  4. 核国家码在产品内置目录里（ADR-0345 决定五）；
//  5. 核业务时区认得出——没给就是没登记，不取任何默认。
//
// 前面一格不成立就答前面那一格。目录与时区这两道成员资格门放在这里，不放进领域构造器：它们只拦将要落册的新修订，
// 读回与重放不经它；时区库还要读运行环境，领域包不读环境。
func (handler *RegisterWarehouseHandler) Register(
	ctx context.Context,
	command RegisterWarehouseCommand,
) (RegisterWarehouseResult, error) {
	candidate, err := domain.NewWarehouseRevision(
		command.Tenant, command.Code, command.Revision, command.Content, command.Registrant, handler.clock.Now(),
	)
	if err != nil {
		var invalid *domain.ContentFieldError
		switch {
		case errors.Is(err, domain.ErrInvalidRevisionNumber):
			// 小于 1 的修订号肯定不是「最新加一」，与跳号同答错位，登记方照回显的最新修订号重读册面。
			return handler.outOfSequence(ctx, command)
		case errors.As(err, &invalid):
			return RegisterWarehouseResult{
				outcome: WarehouseNotAccepted, refusal: WarehouseRefusalContentInvalid, field: invalid.Field,
			}, nil
		default:
			// 租户、仓编码与登记责任方都来自接入信封；没经构造器是装配错了，不是登记方能改的输入。
			return RegisterWarehouseResult{}, fmt.Errorf("register warehouse: %w", err)
		}
	}

	existing, found, err := handler.registry.LoadWarehouseRevision(ctx, command.Tenant, command.Code, command.Revision)
	if err != nil {
		return RegisterWarehouseResult{}, fmt.Errorf("register warehouse: %w", err)
	}
	var latest domain.WarehouseRevision
	var hasLatest bool
	if !found {
		latest, hasLatest, err = handler.registry.LoadLatestWarehouse(ctx, command.Tenant, command.Code)
		if err != nil {
			return RegisterWarehouseResult{}, fmt.Errorf("register warehouse: %w", err)
		}
		if hasLatest && command.Revision <= latest.Revision() {
			// 查册与读最新之间，别的事务把同一修订号落了册。回去再按在册的那一笔判：不然这一笔会被当成跳号答错位，
			// 而它该按内容答重放或冲突（UC-WF-001「一致性、幂等与并发」）。
			existing, found, err = handler.registry.LoadWarehouseRevision(ctx, command.Tenant, command.Code, command.Revision)
			if err != nil {
				return RegisterWarehouseResult{}, fmt.Errorf("register warehouse: %w", err)
			}
		}
	}
	if found {
		if existing.SameContentAs(candidate) {
			return alreadyRegistered(existing), nil
		}
		return RegisterWarehouseResult{outcome: WarehouseContentConflict}, nil
	}

	latestRevision := 0
	if hasLatest {
		latestRevision = latest.Revision()
	}
	if command.Revision != latestRevision+1 {
		return outOfSequenceAt(latestRevision), nil
	}
	if hasLatest && latest.Content().Kind() != candidate.Content().Kind() {
		return RegisterWarehouseResult{
			outcome: WarehouseNotAccepted, refusal: WarehouseRefusalImmutableField, field: domain.WarehouseFieldKind,
		}, nil
	}
	if err := refdata.CheckCountry(candidate.Content().Address().Country().String()); err != nil {
		return contentInvalid(domain.WarehouseFieldAddressCountry), nil
	}
	if zone := candidate.Content().BusinessTimeZone(); zone.Registered() && !businessTimeZoneRecognized(zone.String()) {
		return contentInvalid(domain.WarehouseFieldBusinessTimeZone), nil
	}

	outcome, err := handler.registry.SaveWarehouseRevision(ctx, candidate)
	if err != nil {
		return RegisterWarehouseResult{}, fmt.Errorf("register warehouse: %w", err)
	}
	switch outcome {
	case ports.WarehouseRevisionSaved:
		return RegisterWarehouseResult{
			outcome: WarehouseRegistered, registered: candidate, hasRegistered: true,
		}, nil
	case ports.WarehouseRevisionAlreadyRegistered:
		// 并发里输掉的一笔：别人先落了同一修订号、内容相同。回显在册的那一份，不回显自己这笔。
		landed, found, err := handler.registry.LoadWarehouseRevision(ctx, command.Tenant, command.Code, command.Revision)
		if err != nil {
			return RegisterWarehouseResult{}, fmt.Errorf("register warehouse: %w", err)
		}
		if !found {
			return RegisterWarehouseResult{}, errors.New("register warehouse: 登记册答已在册，却读不回那一笔")
		}
		return alreadyRegistered(landed), nil
	case ports.WarehouseRevisionContentConflict:
		return RegisterWarehouseResult{outcome: WarehouseContentConflict}, nil
	default:
		return RegisterWarehouseResult{}, fmt.Errorf("register warehouse: 登记册答出了未知落点 %d", outcome)
	}
}

// outOfSequence 取在册最新修订号再答错位，给修订号本身就立不住的命令用。
func (handler *RegisterWarehouseHandler) outOfSequence(
	ctx context.Context,
	command RegisterWarehouseCommand,
) (RegisterWarehouseResult, error) {
	latest, hasLatest, err := handler.registry.LoadLatestWarehouse(ctx, command.Tenant, command.Code)
	if err != nil {
		return RegisterWarehouseResult{}, fmt.Errorf("register warehouse: %w", err)
	}
	if !hasLatest {
		return outOfSequenceAt(0), nil
	}
	return outOfSequenceAt(latest.Revision()), nil
}

func outOfSequenceAt(latestRevision int) RegisterWarehouseResult {
	return RegisterWarehouseResult{
		outcome: WarehouseNotAccepted, refusal: WarehouseRefusalRevisionOutOfSequence, latestRevision: latestRevision,
	}
}

func contentInvalid(field domain.WarehouseField) RegisterWarehouseResult {
	return RegisterWarehouseResult{outcome: WarehouseNotAccepted, refusal: WarehouseRefusalContentInvalid, field: field}
}

func alreadyRegistered(registered domain.WarehouseRevision) RegisterWarehouseResult {
	return RegisterWarehouseResult{outcome: WarehouseAlreadyRegistered, registered: registered, hasRegistered: true}
}

// businessTimeZoneRecognized 只收时区库里的名字。「Local」会被 time.LoadLocation 读成部署机器所在的时区——那是替仓挑了一个
// 时区，正是「不取任何默认」要防的；首尾带空白的串不是时区库里的名，不替登记方去掉空白再认。
func businessTimeZoneRecognized(name string) bool {
	if name == "Local" || name != strings.TrimSpace(name) {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}
