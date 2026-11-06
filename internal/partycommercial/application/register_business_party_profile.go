package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/refdata"
)

// BusinessPartyProfileOutcome 是参与方资料登记用例的结果代数（ADR-0369），判据与 LegalEntityProfileOutcome 一致：资料没有
// 停用这一步，往来单位停用后不再收新修订，历史修订照旧读回。
type BusinessPartyProfileOutcome uint8

const (
	BusinessPartyProfileOutcomeInvalid BusinessPartyProfileOutcome = iota
	BusinessPartyProfileRegistered
	BusinessPartyProfileAlreadyRegistered
	BusinessPartyProfileContentConflict
	BusinessPartyProfileNotAccepted
)

func (outcome BusinessPartyProfileOutcome) String() string {
	switch outcome {
	case BusinessPartyProfileRegistered:
		return "REGISTERED"
	case BusinessPartyProfileAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case BusinessPartyProfileContentConflict:
		return "CONTENT_CONFLICT"
	case BusinessPartyProfileNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// BusinessPartyProfileResult 携带落点与`未受理`的原因；原因随结果交回而不折进 error，判据同 LegalEntityProfileResult。
type BusinessPartyProfileResult struct {
	outcome BusinessPartyProfileOutcome
	cause   error
}

func (result BusinessPartyProfileResult) Outcome() BusinessPartyProfileOutcome { return result.outcome }

// Cause 只在`未受理`时非空。
func (result BusinessPartyProfileResult) Cause() error { return result.cause }

func businessPartyProfileNotAccepted(cause error) BusinessPartyProfileResult {
	return BusinessPartyProfileResult{outcome: BusinessPartyProfileNotAccepted, cause: cause}
}

// RegisterBusinessPartyProfileCommand 登记参与方资料的一笔修订；Parts 的各项都可缺。
type RegisterBusinessPartyProfileCommand struct {
	Tenant        domain.TenantID
	Party         domain.PartyID
	Revision      int
	Basis         domain.BusinessPartyProfileBasisReference
	EffectiveFrom time.Time
	Parts         domain.BusinessPartyProfileParts
}

// RegisterBusinessPartyProfileHandler 是参与方资料的写侧编排：领域门 → 修订连续性 → 登记门 → 落册。调用方逐命令各起事务。
type RegisterBusinessPartyProfileHandler struct {
	profiles    ports.BusinessPartyProfileRegistry
	identities  ports.PartyIdentityRegistry
	numberTypes ports.RegistrationNumberTypeLookup
}

func NewRegisterBusinessPartyProfileHandler(
	profiles ports.BusinessPartyProfileRegistry,
	identities ports.PartyIdentityRegistry,
	numberTypes ports.RegistrationNumberTypeLookup,
) *RegisterBusinessPartyProfileHandler {
	return &RegisterBusinessPartyProfileHandler{profiles: profiles, identities: identities, numberTypes: numberTypes}
}

// Register 登记一笔资料修订。修订从 1 起连续递增；可以未来生效，也可以追溯生效——哪一笔此刻有效由读口按
// domain.EffectiveBusinessPartyProfileRevision 回答，登记不按生效时点排队。
func (handler *RegisterBusinessPartyProfileHandler) Register(
	ctx context.Context,
	command RegisterBusinessPartyProfileCommand,
) (BusinessPartyProfileResult, error) {
	content, err := domain.NewBusinessPartyProfileContent(command.Parts)
	if err != nil {
		return businessPartyProfileNotAccepted(err), nil
	}
	revision, err := domain.NewBusinessPartyProfileRevision(
		command.Tenant, command.Party, command.Revision, command.Basis, command.EffectiveFrom, content,
	)
	if err != nil {
		return businessPartyProfileNotAccepted(err), nil
	}
	latest, found, err := handler.profiles.LoadLatestBusinessPartyProfile(ctx, command.Tenant, command.Party)
	if err != nil {
		return BusinessPartyProfileResult{}, fmt.Errorf("register business party profile: %w", err)
	}
	switch {
	case !found && command.Revision != 1:
		return businessPartyProfileNotAccepted(fmt.Errorf("首笔资料修订必须是 1，收到 %d", command.Revision)), nil
	case found && command.Revision > latest.Revision()+1:
		return businessPartyProfileNotAccepted(fmt.Errorf("修订必须连续：册上最新为 %d，收到 %d", latest.Revision(), command.Revision)), nil
	}
	// 登记门只拦将要落册的新修订；修订号落在已有修订上的是重放或冲突，照册面原样比对（判据同法人资料）。
	if !found || command.Revision > latest.Revision() {
		refusal, err := handler.checkSuccessor(ctx, revision)
		if err != nil {
			return BusinessPartyProfileResult{}, fmt.Errorf("register business party profile: %w", err)
		}
		if refusal != "" {
			return businessPartyProfileNotAccepted(errors.New(refusal)), nil
		}
	}
	outcome, err := handler.profiles.SaveBusinessPartyProfile(ctx, revision)
	if err != nil {
		return BusinessPartyProfileResult{}, fmt.Errorf("register business party profile: %w", err)
	}
	switch outcome {
	case ports.BusinessPartyProfileSaved:
		return BusinessPartyProfileResult{outcome: BusinessPartyProfileRegistered}, nil
	case ports.BusinessPartyProfileAlreadyRegistered:
		return BusinessPartyProfileResult{outcome: BusinessPartyProfileAlreadyRegistered}, nil
	case ports.BusinessPartyProfileContentConflict:
		return BusinessPartyProfileResult{outcome: BusinessPartyProfileContentConflict}, nil
	default:
		return BusinessPartyProfileResult{}, fmt.Errorf("register business party profile: 登记册答出了未知落点 %d", outcome)
	}
}

// checkSuccessor 是登记门（ADR-0369 决定二）。非空的第一个返回值是给登记方看的拒绝理由；error 只留给读册与读目录的技术失败。
func (handler *RegisterBusinessPartyProfileHandler) checkSuccessor(
	ctx context.Context,
	revision domain.BusinessPartyProfileRevision,
) (string, error) {
	tenant, partyID := revision.Tenant(), revision.Party()
	party, found, err := handler.identities.LoadLatestBusinessParty(ctx, tenant, partyID)
	if err != nil {
		return "", err
	}
	if !found {
		return fmt.Sprintf("往来单位 %s 未登记：参与方资料引用往来单位身份，先登记往来单位", partyID), nil
	}
	if _, _, deactivated := party.Lifecycle().Deactivation(); deactivated {
		return fmt.Sprintf("往来单位 %s 已停用，不再收新的资料修订；历史修订照旧保留", partyID), nil
	}
	entities, err := handler.identities.FindLegalEntitiesPinning(ctx, tenant, partyID)
	if err != nil {
		return "", err
	}
	if len(entities) > 0 {
		return fmt.Sprintf(
			"往来单位 %s 是公司主体 %s 的参与方：我方公司的资料只登在公司主体的法人资料上，往来单位这里不登",
			partyID, strings.Join(entities, "、"),
		), nil
	}

	parts := revision.Content().Parts()
	// 新登记入口的国家码与币种只收产品内置目录（ADR-0345 决定五）；读回与重放不经这道门。
	for _, entry := range []struct {
		label   string
		address *domain.RegisteredAddress
	}{{"注册地址", parts.RegisteredAddress}, {"办公地址", parts.OfficeAddress}, {"通信地址", parts.CorrespondenceAddress}} {
		if entry.address == nil {
			continue
		}
		if err := refdata.CheckCountry(entry.address.Country().String()); err != nil {
			return fmt.Sprintf("%s的国家 / 地区 %s 不在产品内置的国家 / 地区目录里（ISO 3166-1）", entry.label, entry.address.Country()), nil
		}
	}
	if err := domain.BankAccountCurrenciesInCatalogue(parts.BankAccounts); err != nil {
		return fmt.Sprintf("收款账户的币种不在产品内置的币种目录里：%v", err), nil
	}

	layer, hasIdentity := party.IdentityLayer()
	if hasIdentity && parts.RegisteredAddress != nil && parts.RegisteredAddress.Country() != layer.Country() {
		return fmt.Sprintf(
			"注册地址的国家 / 地区 %s 与往来单位登记的注册国家 / 地区 %s 不一致", parts.RegisteredAddress.Country(), layer.Country(),
		), nil
	}
	if len(parts.TaxNumbers) == 0 {
		return "", nil
	}
	// 监管登记号按往来单位的注册国家判，没有则按注册地址的国家；两者都没有就无从判类型。
	var country domain.RegistrationCountryCode
	switch {
	case hasIdentity:
		country = layer.Country()
	case parts.RegisteredAddress != nil:
		country = parts.RegisteredAddress.Country()
	default:
		return "带了监管登记号却没有可判的国家 / 地区：先在往来单位上登注册地与注册号，或在这一版资料里填注册地址", nil
	}
	if handler.numberTypes == nil {
		return "", errors.New("注册号类型目录未装配，判不了监管登记号")
	}
	catalogue, err := handler.numberTypes.LoadRegistrationNumberTypeCatalogue(ctx, tenant, country)
	if err != nil {
		return "", err
	}
	at := revision.EffectiveFrom()
	for _, number := range parts.TaxNumbers {
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
