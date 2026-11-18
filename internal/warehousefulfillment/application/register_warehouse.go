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

// RegisterWarehouseOutcome 是仓登记用例的结果代数（ADR-0031）：`REGISTERED` 是本次落册；`ALREADY_REGISTERED` 是同一修订号、
// 内容相同的重放，回显在册的那一份；`CONTENT_CONFLICT` 是同一修订号、内容不同，不覆盖在册的；`NOT_ACCEPTED` 是新修订没通过
// 核对，一个字节没写，并且一定带一个封闭的拒绝理由。
//
// 结果的形状（落点 + 拒绝理由 + 指出的格）是为了往后再加：别的册会有指名对象查不到、对象已停用这类拒绝理由，也会有
// 租户取值没到时的另一种落点。它们落在这两个枚举里再添一格，Field 已经能指名所指的那一格，结果的形状不用动。
type RegisterWarehouseOutcome uint8

const (
	RegisterWarehouseOutcomeInvalid RegisterWarehouseOutcome = iota
	WarehouseRegistered
	WarehouseAlreadyRegistered
	WarehouseContentConflict
	WarehouseNotAccepted
)

func (outcome RegisterWarehouseOutcome) String() string {
	switch outcome {
	case WarehouseRegistered:
		return "REGISTERED"
	case WarehouseAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case WarehouseContentConflict:
		return "CONTENT_CONFLICT"
	case WarehouseNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// RegisterWarehouseRefusal 是 `NOT_ACCEPTED` 的原因。逐格分开是因为登记方的续办各不相同：修订错位要重读册面，不可改的格
// 要换成原来的值，内容不合要改那一格的取值。
type RegisterWarehouseRefusal uint8

const (
	WarehouseRefusalNone RegisterWarehouseRefusal = iota
	WarehouseRefusalRevisionOutOfSequence
	WarehouseRefusalImmutableField
	WarehouseRefusalContentInvalid
)

func (refusal RegisterWarehouseRefusal) String() string {
	switch refusal {
	case WarehouseRefusalRevisionOutOfSequence:
		return "REVISION_OUT_OF_SEQUENCE"
	case WarehouseRefusalImmutableField:
		return "IMMUTABLE_FIELD"
	case WarehouseRefusalContentInvalid:
		return "CONTENT_INVALID"
	default:
		return ""
	}
}

// RegisterWarehouseResult 携带落点、`NOT_ACCEPTED` 的原因与细节。输入被拒是已形成的答案，不是技术失败；只有册读写不动才
// 另走 error。
type RegisterWarehouseResult struct {
	outcome        RegisterWarehouseOutcome
	refusal        RegisterWarehouseRefusal
	field          domain.WarehouseField
	latestRevision int
	registered     domain.WarehouseRevision
	hasRegistered  bool
}

func (result RegisterWarehouseResult) Outcome() RegisterWarehouseOutcome {
	return result.outcome
}

// Refusal 只在 NOT_ACCEPTED 时有值。
func (result RegisterWarehouseResult) Refusal() RegisterWarehouseRefusal {
	return result.refusal
}

// Field 指出 IMMUTABLE_FIELD 与 CONTENT_INVALID 差在哪一格；别的结果上是零值。
func (result RegisterWarehouseResult) Field() domain.WarehouseField {
	return result.field
}

// LatestRevision 只在 REVISION_OUT_OF_SEQUENCE 时有值：在册的最新修订号，册上没有这个仓时是 0。
func (result RegisterWarehouseResult) LatestRevision() int {
	return result.latestRevision
}

// Revision 只在 REGISTERED 与 ALREADY_REGISTERED 时有：落册或在册的那一笔。重放回显的是在册那一份，登记责任方与登记时刻
// 因此是当初落册时的，不是这次重投的。
func (result RegisterWarehouseResult) Revision() (domain.WarehouseRevision, bool) {
	return result.registered, result.hasRegistered
}

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
