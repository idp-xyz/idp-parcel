package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// checkBusinessPartyIdentification 是往来单位新登记与新修订的识别信息诸门（ADR-0367 决定一至三），按登记方最该先知道的
// 次序判：接续（改号要更正依据）、我方公司不在往来单位上登号（号对不对都不该登在这里，先说这一句）、注册号目录、号与外部
// 编码有没有别人持有。非空的第一个返回值是拒绝理由；error 只留给读册与读目录的技术失败。
func (handler *RegisterPartyIdentityHandler) checkBusinessPartyIdentification(
	ctx context.Context,
	registration domain.BusinessPartyRegistration,
	latest domain.BusinessPartyRegistration,
	found bool,
) (string, error) {
	party := registration.Party()
	tenant := party.Tenant()
	if found {
		if err := domain.CheckBusinessPartyIdentitySuccession(latest, registration); err != nil {
			return businessPartyIdentificationRefusal(err).Error(), nil
		}
	}

	if identity, has := registration.IdentityLayer(); has {
		// 取钉住这个参与方的锁（与公司主体那条路径互斥，理由在端口注释），再看它是不是我方公司。
		entities, err := handler.registry.FindLegalEntitiesPinning(ctx, tenant, party.ID())
		if err != nil {
			return "", err
		}
		if len(entities) > 0 {
			return fmt.Sprintf(
				"往来单位 %s 是公司主体 %s 的参与方：我方公司的注册号只登在公司主体上，往来单位这里不登",
				party.ID(), strings.Join(entities, "、"),
			), nil
		}
		refusal, err := handler.checkLifetimeNumbers(ctx, tenant, identity, registration.Lifecycle().EffectiveFrom(), "往来单位生效时点")
		if err != nil || refusal != "" {
			return refusal, err
		}
		holders, err := handler.registry.FindRegistrationNumberHolders(ctx, tenant, identity)
		if err != nil {
			return "", err
		}
		if refusal := registrationNumberHolderRefusal(holders, func(holder ports.RegistrationNumberHolder) bool {
			return holder.Kind == ports.IdentificationHeldByBusinessParty && holder.ID == party.ID().String()
		}); refusal != "" {
			return refusal, nil
		}
	}

	if references := registration.ExternalReferences(); len(references) > 0 {
		holders, err := handler.registry.FindExternalReferenceHolders(ctx, tenant, references)
		if err != nil {
			return "", err
		}
		for _, holder := range holders {
			if holder.Party == party.ID().String() {
				continue
			}
			return fmt.Sprintf(
				"外部系统 %s 的%s %s 已登在往来单位 %s 上：同一个编码只指一家单位；是同一家就更正那一家，不再登第二家",
				holder.System, externalPurposeLabel(holder.Purpose), holder.Code, holder.Party,
			), nil
		}
	}
	return "", nil
}

// registrationNumberHolderRefusal 把查重读口交回的持有方译成拒绝理由；self 认出登记方自己（同一身份的前后修订不算撞号）。
func registrationNumberHolderRefusal(holders []ports.RegistrationNumberHolder, self func(ports.RegistrationNumberHolder) bool) string {
	for _, holder := range holders {
		if self(holder) {
			continue
		}
		switch holder.Kind {
		case ports.IdentificationHeldByLegalEntity:
			return fmt.Sprintf(
				"注册号 %s %s 已登在公司主体 %s（往来单位 %s）上：同一个号只指一个在用身份；是我方公司就不在这里另登",
				holder.TypeCode, holder.Number, holder.ID, holder.Party,
			)
		default:
			return fmt.Sprintf(
				"注册号 %s %s 已登在往来单位 %s 上：同一个号只指一个在用身份；是同一家就更正那一家，不再登第二家",
				holder.TypeCode, holder.Number, holder.ID,
			)
		}
	}
	return ""
}

func externalPurposeLabel(purpose string) string {
	switch purpose {
	case "CUSTOMER":
		return "客户编码"
	case "SUPPLIER":
		return "供应商编码"
	case "PARTY":
		return "往来单位编码"
	default:
		return purpose
	}
}

// businessPartyIdentificationRefusal 把识别信息的领域拒绝译成登记方看得懂的续办；别的错误原样交回。
func businessPartyIdentificationRefusal(err error) error {
	switch {
	case errors.Is(err, domain.ErrBusinessPartyIdentityChangedWithoutCorrection):
		return errors.New(
			"已登的注册国家 / 地区或终身注册号不作变更：号真的变了就停用这家单位、登记新单位；" +
				"是当初录错了，就带身份更正依据登记更正修订（ADR-0367 决定一）",
		)
	case errors.Is(err, domain.ErrIdentityCorrectionWithoutChange):
		return errors.New(
			"身份更正依据只随改了或去掉已登的号的修订出现：首笔登记、第一次补登号与没改号的修订都无可更正，不带更正依据",
		)
	case errors.Is(err, domain.ErrInvalidExternalReference):
		return fmt.Errorf("外部系统编码不成立（同一系统、同一用途只收一个编码）：%w", err)
	default:
		return err
	}
}
