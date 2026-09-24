package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// RegistrationNumberTypeOutcome 是注册号类型目录登记与停用用例的结果代数，判据同 PartyRegistryOutcome：
// `已登记`与`已停用`是本次落库；`重复`按同键同内容重放返回原结果；`内容冲突`要实施方对着册面修正、
// 绝不覆盖（ADR-0031）；`未受理`是输入被领域门或连续性检查拒绝（含修订错位与改层），一个字节没写；
// `未找到`只出现在停用——要停用的类型从未登记。
type RegistrationNumberTypeOutcome uint8

const (
	RegistrationNumberTypeOutcomeInvalid RegistrationNumberTypeOutcome = iota
	RegistrationNumberTypeRegistered
	RegistrationNumberTypeDeactivated
	RegistrationNumberTypeAlreadyRegistered
	RegistrationNumberTypeContentConflict
	RegistrationNumberTypeNotAccepted
	RegistrationNumberTypeNotFound
)

func (outcome RegistrationNumberTypeOutcome) String() string {
	switch outcome {
	case RegistrationNumberTypeRegistered:
		return "REGISTERED"
	case RegistrationNumberTypeDeactivated:
		return "DEACTIVATED"
	case RegistrationNumberTypeAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case RegistrationNumberTypeContentConflict:
		return "CONTENT_CONFLICT"
	case RegistrationNumberTypeNotAccepted:
		return "NOT_ACCEPTED"
	case RegistrationNumberTypeNotFound:
		return "NOT_FOUND"
	default:
		return ""
	}
}

// RegistrationNumberTypeResult 携带落点与`未受理`的原因（判据同 PartyRegistryResult：输入被拒不是
// 技术失败，事务保持可用，批内后项照常推进）。
type RegistrationNumberTypeResult struct {
	outcome RegistrationNumberTypeOutcome
	cause   error
}

func (result RegistrationNumberTypeResult) Outcome() RegistrationNumberTypeOutcome {
	return result.outcome
}

// Cause 只在`未受理`时非空。
func (result RegistrationNumberTypeResult) Cause() error {
	return result.cause
}

func registrationNumberTypeNotAccepted(cause error) RegistrationNumberTypeResult {
	return RegistrationNumberTypeResult{outcome: RegistrationNumberTypeNotAccepted, cause: cause}
}

// RegisterRegistrationNumberTypeCommand 登记注册号类型的一笔修订：修订 1 是登记，其后是内容更正。
type RegisterRegistrationNumberTypeCommand struct {
	Tenant        domain.TenantID
	Country       domain.RegistrationCountryCode
	Code          domain.RegistrationNumberTypeCode
	Revision      int
	Spec          domain.RegistrationNumberTypeSpec
	EffectiveFrom time.Time
}

// DeactivateRegistrationNumberTypeCommand 停用一个类型。Revision 是停用落点的修订号（= 册上最新
// 修订 + 1），判据同 DeactivatePartyIdentityCommand：操作者声明自己看到的册面，错位即拒。
type DeactivateRegistrationNumberTypeCommand struct {
	Tenant   domain.TenantID
	Country  domain.RegistrationCountryCode
	Code     domain.RegistrationNumberTypeCode
	Revision int
	Basis    domain.RegistrationNumberTypeBasisReference
	At       time.Time
}

// RegisterRegistrationNumberTypeHandler 是注册号类型目录的写侧编排：领域门 → 修订连续性与层不改
// 检查 → 登记册落库。调用方逐命令各起事务（批不是聚合，AT-PC-011）。
type RegisterRegistrationNumberTypeHandler struct {
	registry ports.RegistrationNumberTypeRegistry
}

func NewRegisterRegistrationNumberTypeHandler(
	registry ports.RegistrationNumberTypeRegistry,
) *RegisterRegistrationNumberTypeHandler {
	return &RegisterRegistrationNumberTypeHandler{registry: registry}
}

// Register 登记一笔类型修订。
//
// 所属层钉在类型上，新修订改层不受理：一个类型换了层，此前按它在身份层登记过的号就说不清还算
// 不算终身注册号（ADR-0145 决定一「两层各只收自己那一类」）。改层是另一个类型，登记新的类型代码。
func (handler *RegisterRegistrationNumberTypeHandler) Register(
	ctx context.Context,
	command RegisterRegistrationNumberTypeCommand,
) (RegistrationNumberTypeResult, error) {
	lifecycle, err := domain.NewRegistrationNumberTypeLifecycle(command.EffectiveFrom)
	if err != nil {
		return registrationNumberTypeNotAccepted(err), nil
	}
	registration, err := domain.NewRegistrationNumberTypeRegistration(
		command.Tenant, command.Country, command.Code, command.Revision, command.Spec, lifecycle,
	)
	if err != nil {
		return registrationNumberTypeNotAccepted(err), nil
	}

	latest, found, err := handler.registry.LoadLatestRegistrationNumberType(
		ctx, command.Tenant, command.Country, command.Code,
	)
	if err != nil {
		return RegistrationNumberTypeResult{}, fmt.Errorf("register registration number type: %w", err)
	}
	if found {
		_, _, deactivated := latest.Lifecycle().Deactivation()
		if command.Revision > latest.Revision() {
			if deactivated {
				return registrationNumberTypeNotAccepted(errors.New("类型已停用，不再收新修订")), nil
			}
			if command.Revision != latest.Revision()+1 {
				return registrationNumberTypeNotAccepted(fmt.Errorf(
					"修订必须连续：册上最新为 %d，收到 %d", latest.Revision(), command.Revision,
				)), nil
			}
			if latest.Layer() != command.Spec.Layer {
				return registrationNumberTypeNotAccepted(fmt.Errorf(
					"类型 %s/%s 钉在 %s 层，新修订不改层——改层是另一个类型，登记新的类型代码",
					command.Country, command.Code, latest.Layer(),
				)), nil
			}
		}
	} else if command.Revision != 1 {
		return registrationNumberTypeNotAccepted(fmt.Errorf(
			"首笔登记修订必须是 1，收到 %d", command.Revision,
		)), nil
	}

	outcome, err := handler.registry.SaveRegistrationNumberType(ctx, registration)
	if err != nil {
		return RegistrationNumberTypeResult{}, fmt.Errorf("register registration number type: %w", err)
	}
	return registrationNumberTypeSaveResult(outcome, RegistrationNumberTypeRegistered)
}

// Deactivate 停用一个类型：取最新修订，经领域转换生成下一笔修订落库。
func (handler *RegisterRegistrationNumberTypeHandler) Deactivate(
	ctx context.Context,
	command DeactivateRegistrationNumberTypeCommand,
) (RegistrationNumberTypeResult, error) {
	if command.Tenant.String() == "" || command.Country.String() == "" || command.Code.String() == "" {
		return registrationNumberTypeNotAccepted(errors.New("租户、注册国家 / 地区与类型代码缺一不可")), nil
	}
	latest, found, err := handler.registry.LoadLatestRegistrationNumberType(
		ctx, command.Tenant, command.Country, command.Code,
	)
	if err != nil {
		return RegistrationNumberTypeResult{}, fmt.Errorf("deactivate registration number type: %w", err)
	}
	if !found {
		return RegistrationNumberTypeResult{outcome: RegistrationNumberTypeNotFound}, nil
	}
	if basis, at, has := latest.Lifecycle().Deactivation(); has {
		// 撞上已停用的册面：同修订同内容是重放，同修订异内容是冲突，其余不再落笔。
		switch {
		case command.Revision == latest.Revision() && basis == command.Basis && at.Equal(command.At.UTC()):
			return RegistrationNumberTypeResult{outcome: RegistrationNumberTypeAlreadyRegistered}, nil
		case command.Revision == latest.Revision():
			return RegistrationNumberTypeResult{outcome: RegistrationNumberTypeContentConflict}, nil
		default:
			return registrationNumberTypeNotAccepted(errors.New("类型已停用，停用不重复落笔")), nil
		}
	}
	next, err := latest.Deactivate(command.Basis, command.At)
	if err != nil {
		return registrationNumberTypeNotAccepted(err), nil
	}
	if next.Revision() != command.Revision {
		return registrationNumberTypeNotAccepted(fmt.Errorf(
			"停用落点修订错位：命令声明 %d，册面推进到 %d——册面已被并发推进或意图已陈旧",
			command.Revision, next.Revision(),
		)), nil
	}
	outcome, err := handler.registry.SaveRegistrationNumberType(ctx, next)
	if err != nil {
		return RegistrationNumberTypeResult{}, fmt.Errorf("deactivate registration number type: %w", err)
	}
	return registrationNumberTypeSaveResult(outcome, RegistrationNumberTypeDeactivated)
}

func registrationNumberTypeSaveResult(
	outcome ports.RegistrationNumberTypeSaveOutcome,
	landed RegistrationNumberTypeOutcome,
) (RegistrationNumberTypeResult, error) {
	switch outcome {
	case ports.RegistrationNumberTypeRegistrySaved:
		return RegistrationNumberTypeResult{outcome: landed}, nil
	case ports.RegistrationNumberTypeRegistryAlreadyRegistered:
		return RegistrationNumberTypeResult{outcome: RegistrationNumberTypeAlreadyRegistered}, nil
	case ports.RegistrationNumberTypeRegistryContentConflict:
		return RegistrationNumberTypeResult{outcome: RegistrationNumberTypeContentConflict}, nil
	default:
		return RegistrationNumberTypeResult{}, fmt.Errorf(
			"registration number type registry: unexpected save outcome %d", outcome)
	}
}
