package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// LegalEntityProfileOutcome 是法人资料登记用例的结果代数，判据同 PartyRegistryOutcome：`已登记`是本次落库；
// `重复`按同键同内容重放返回原结果；`内容冲突`要登记方对着册面修正、绝不覆盖（ADR-0031）；`未受理`是输入
// 被领域门、修订连续性或资料门拒绝，一个字节没写。资料没有停用这一步——法人停用后资料随之不再参与新的
// 解析（CONTEXT Lifecycles「法人资料」），所以没有`已停用`与`未找到`。
type LegalEntityProfileOutcome uint8

const (
	LegalEntityProfileOutcomeInvalid LegalEntityProfileOutcome = iota
	LegalEntityProfileRegistered
	LegalEntityProfileAlreadyRegistered
	LegalEntityProfileContentConflict
	LegalEntityProfileNotAccepted
)

func (outcome LegalEntityProfileOutcome) String() string {
	switch outcome {
	case LegalEntityProfileRegistered:
		return "REGISTERED"
	case LegalEntityProfileAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case LegalEntityProfileContentConflict:
		return "CONTENT_CONFLICT"
	case LegalEntityProfileNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// LegalEntityProfileResult 携带落点与`未受理`的原因（判据同 PartyRegistryResult：输入被拒不是技术失败，事务
// 保持可用，批内后项照常推进）。
type LegalEntityProfileResult struct {
	outcome LegalEntityProfileOutcome
	cause   error
}

func (result LegalEntityProfileResult) Outcome() LegalEntityProfileOutcome {
	return result.outcome
}

// Cause 只在`未受理`时非空。
func (result LegalEntityProfileResult) Cause() error {
	return result.cause
}

func legalEntityProfileNotAccepted(cause error) LegalEntityProfileResult {
	return LegalEntityProfileResult{outcome: LegalEntityProfileNotAccepted, cause: cause}
}

// RegisterLegalEntityProfileCommand 登记法人资料的一笔修订。Invoicing 为 nil 即这笔修订不带开票资料——登记时
// 不拦，开立时才答`资料不全`（ADR-0145 决定六）。
type RegisterLegalEntityProfileCommand struct {
	Tenant        domain.TenantID
	Entity        domain.LegalEntityReference
	Revision      int
	Basis         domain.LegalEntityProfileBasisReference
	EffectiveFrom time.Time
	Address       domain.RegisteredAddress
	TaxNumbers    []domain.TaxRegistrationNumber
	Invoicing     *domain.InvoicingDetails
	Contacts      []domain.LegalEntityContact
}

// RegisterLegalEntityProfileHandler 是法人资料的写侧编排：领域门 → 修订连续性 → 资料门（法人在册且未停用、
// 地址国家对得上身份、税务登记号按目录判）→ 登记册落库。调用方逐命令各起事务（批不是聚合，AT-PC-011）。
type RegisterLegalEntityProfileHandler struct {
	profiles    ports.LegalEntityProfileRegistry
	entities    ports.LegalEntityRegistrationLookup
	numberTypes ports.RegistrationNumberTypeLookup
}

func NewRegisterLegalEntityProfileHandler(
	profiles ports.LegalEntityProfileRegistry,
	entities ports.LegalEntityRegistrationLookup,
	numberTypes ports.RegistrationNumberTypeLookup,
) *RegisterLegalEntityProfileHandler {
	return &RegisterLegalEntityProfileHandler{profiles: profiles, entities: entities, numberTypes: numberTypes}
}

// Register 登记一笔资料修订。修订从 1 起连续递增；新修订可以未来生效，也可以追溯生效——哪一笔在某个时点有效
// 由解析回答，登记不按生效时点排队。
func (handler *RegisterLegalEntityProfileHandler) Register(
	ctx context.Context,
	command RegisterLegalEntityProfileCommand,
) (LegalEntityProfileResult, error) {
	content, err := domain.NewLegalEntityProfileContent(command.Address, command.TaxNumbers, command.Invoicing, command.Contacts)
	if err != nil {
		return legalEntityProfileNotAccepted(err), nil
	}
	revision, err := domain.NewLegalEntityProfileRevision(
		command.Tenant, command.Entity, command.Revision, command.Basis, command.EffectiveFrom, content,
	)
	if err != nil {
		return legalEntityProfileNotAccepted(err), nil
	}

	latest, found, err := handler.profiles.LoadLatestLegalEntityProfile(ctx, command.Tenant, command.Entity)
	if err != nil {
		return LegalEntityProfileResult{}, fmt.Errorf("register legal entity profile: %w", err)
	}
	switch {
	case !found && command.Revision != 1:
		return legalEntityProfileNotAccepted(fmt.Errorf("首笔资料修订必须是 1，收到 %d", command.Revision)), nil
	case found && command.Revision > latest.Revision()+1:
		return legalEntityProfileNotAccepted(fmt.Errorf(
			"修订必须连续：册上最新为 %d，收到 %d", latest.Revision(), command.Revision,
		)), nil
	}

	// 资料门只拦将要落册的新修订。修订号落在已有修订上的是重放或冲突，照册面原样比对：法人此后停用了、
	// 目录修订了，拿今天的门去拦一次重放只会把`已登记`答错。
	if !found || command.Revision > latest.Revision() {
		refusal, err := handler.checkSuccessor(ctx, revision)
		if err != nil {
			return LegalEntityProfileResult{}, fmt.Errorf("register legal entity profile: %w", err)
		}
		if refusal != "" {
			return legalEntityProfileNotAccepted(errors.New(refusal)), nil
		}
	}

	outcome, err := handler.profiles.SaveLegalEntityProfile(ctx, revision)
	if err != nil {
		return LegalEntityProfileResult{}, fmt.Errorf("register legal entity profile: %w", err)
	}
	switch outcome {
	case ports.LegalEntityProfileRegistrySaved:
		return LegalEntityProfileResult{outcome: LegalEntityProfileRegistered}, nil
	case ports.LegalEntityProfileRegistryAlreadyRegistered:
		return LegalEntityProfileResult{outcome: LegalEntityProfileAlreadyRegistered}, nil
	case ports.LegalEntityProfileRegistryContentConflict:
		return LegalEntityProfileResult{outcome: LegalEntityProfileContentConflict}, nil
	default:
		return LegalEntityProfileResult{}, fmt.Errorf("register legal entity profile: 登记册答出了未知落点 %d", outcome)
	}
}

// checkSuccessor 是资料门。非空的第一个返回值是给登记方看的拒绝理由；error 只留给读册的技术失败。
//
// 法人停用按「册上已登记停用」判，不问停用时点到没到——与身份修订连续性门同一口径（checkRevisionSlot）：停用
// 不是中场休息，登记在册的停用就是这个法人不再往下走。法人生效时点未到则照收：法人可以先登记、后补资料
// （ADR-0145 决定六）。
func (handler *RegisterLegalEntityProfileHandler) checkSuccessor(
	ctx context.Context,
	revision domain.LegalEntityProfileRevision,
) (string, error) {
	id := revision.LegalEntity()
	entity, found, err := handler.entities.LoadLatestLegalEntity(ctx, revision.Tenant(), id)
	if err != nil {
		return "", err
	}
	if !found {
		return fmt.Sprintf("责任法人 %s 未登记：法人资料引用责任法人身份，先登记法人", id), nil
	}
	if _, _, deactivated := entity.Lifecycle().Deactivation(); deactivated {
		return fmt.Sprintf("责任法人 %s 已停用，不再收新的资料修订；历史修订与已开单据的引用照旧保留", id), nil
	}
	identity, hasIdentity := entity.IdentityLayer()
	if !hasIdentity {
		return fmt.Sprintf(
			"责任法人 %s 的身份层（注册国家 / 地区与终身注册号）未登记：注册地址的国家 / 地区要对着它判，先登记带身份层的法人修订", id,
		), nil
	}
	content := revision.Content()
	country := identity.Country()
	if addressCountry := content.Address().Country(); addressCountry != country {
		return fmt.Sprintf(
			"注册地址的国家 / 地区 %s 与责任法人身份上的注册国家 / 地区 %s 不一致（ADR-0145 决定三）", addressCountry, country,
		), nil
	}

	numbers := content.TaxNumbers()
	if len(numbers) == 0 {
		return "", nil
	}
	if handler.numberTypes == nil {
		return "", errors.New("注册号类型目录未装配，判不了税务登记号")
	}
	catalogue, err := handler.numberTypes.LoadRegistrationNumberTypeCatalogue(ctx, revision.Tenant(), country)
	if err != nil {
		return "", err
	}
	// 判号时点取这笔资料修订的生效时点：号自那一刻起随资料对外使用，类型那一刻就得在用。
	at := revision.EffectiveFrom()
	for _, number := range numbers {
		check, err := catalogue.Check(number.TypeCode(), domain.RegistrationNumberProfileLayer, number.Number(), at)
		if err != nil {
			return "", err
		}
		refusal, err := registrationNumberRefusal(
			check, country, number.TypeCode(), number.Number(), domain.RegistrationNumberProfileLayer, "资料生效时点", at,
		)
		if err != nil || refusal != "" {
			return refusal, err
		}
	}
	return "", nil
}

// ResolveLegalEntityProfileHandler 是「按时点解析法人资料」的用例（ADR-0145 决定五、六）：给开立方用。开立方
// 拿到`已解析`后固定的是修订引用（domain.LegalEntityProfileRevision.Reference），不是这一刻读到的内容——此后
// 资料再改、包括追溯生效的修订，已开单据仍指向那一笔。其余各格都是明确非成功，开立方据此拒绝开立。
type ResolveLegalEntityProfileHandler struct {
	chains   ports.LegalEntityProfileChainRead
	entities ports.LegalEntityRegistrationLookup
}

func NewResolveLegalEntityProfileHandler(
	chains ports.LegalEntityProfileChainRead,
	entities ports.LegalEntityRegistrationLookup,
) *ResolveLegalEntityProfileHandler {
	return &ResolveLegalEntityProfileHandler{chains: chains, entities: entities}
}

// Resolve 读法人最新修订与整条资料修订链，交领域判。读册失败走 error：一次该重试的故障不能答成`资料不全`，
// 那会让开立方去催登记方补一份本来就在的资料。
func (handler *ResolveLegalEntityProfileHandler) Resolve(
	ctx context.Context,
	tenant domain.TenantID,
	entity domain.LegalEntityReference,
	at time.Time,
) (domain.LegalEntityProfileResolution, error) {
	registration, found, err := handler.entities.LoadLatestLegalEntity(ctx, tenant, entity)
	if err != nil {
		return domain.LegalEntityProfileResolution{}, fmt.Errorf("resolve legal entity profile: %w", err)
	}
	if !found {
		return domain.NotRegisteredLegalEntityProfileResolution(), nil
	}
	chain, err := handler.chains.LoadLegalEntityProfileChain(ctx, tenant, entity)
	if err != nil {
		return domain.LegalEntityProfileResolution{}, fmt.Errorf("resolve legal entity profile: %w", err)
	}
	resolution, err := domain.ResolveLegalEntityProfile(registration, chain, at)
	if err != nil {
		return domain.LegalEntityProfileResolution{}, fmt.Errorf("resolve legal entity profile: %w", err)
	}
	return resolution, nil
}
