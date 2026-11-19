package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/refdata"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// RegisterPackagingMaterialResult 是包材登记用例的结果：与仓、库区、库位类型同一套代数（registration_result.go），Field 是包材修订上的格
// （见 domain.PackagingMaterialField），回显的是在册的包材修订。
type RegisterPackagingMaterialResult = RegistrationResult[domain.PackagingMaterialRevision, domain.PackagingMaterialField]

// RegisterPackagingMaterialCommand 登记包材的一笔修订：修订 1 是首次登记，其后是改名称、改尺寸与重量、停用与再启用，各占下一个修订号。
// 包材按租户登记、各仓共用，身份里没有仓编码。Tenant 与 Registrant 取自认证信封，不取自载荷。
type RegisterPackagingMaterialCommand struct {
	Tenant     domain.TenantID
	Code       domain.PackagingMaterialCode
	Revision   int
	Content    domain.PackagingMaterialContent
	Registrant domain.Registrant
}

// RegisterPackagingMaterialHandler 是包材登记册的写侧编排。事务边界在调用方：每条命令各起一个环境事务，登记册的保存在没有环境事务时拒绝。
// 并发提交同一修订号的两笔不靠这里加锁——登记册的主键让至多一笔落册，另一笔的保存按内容答重放或冲突，再由这里译成结果。
//
// 「本租户有没有登记过仓」只读仓册，不写：仓册只追加，登记过仓的租户不会再回到「一个仓都没有」，所以这一读不需要锁。
type RegisterPackagingMaterialHandler struct {
	packagingMaterials ports.PackagingMaterialRegistry
	warehouses         ports.TenantWarehouseLookup
	clock              ports.Clock
}

func NewRegisterPackagingMaterialHandler(
	packagingMaterials ports.PackagingMaterialRegistry,
	warehouses ports.TenantWarehouseLookup,
	clock ports.Clock,
) *RegisterPackagingMaterialHandler {
	return &RegisterPackagingMaterialHandler{packagingMaterials: packagingMaterials, warehouses: warehouses, clock: clock}
}

// Register 登记一笔包材修订。判定次序（UC-WF-001「应用流程」）：
//
//  1. 按（身份，修订号）找在册的那一笔——任何一笔，不只是最新那一笔：内容相同答重放，不同答冲突，后面的核都不再过。
//     册上早就有的修订不受后来的目录改版牵连，也不会因为一个零值被判成别的答案；
//  2. 都不是，看本租户有没有登记过仓（停用的仓也算）：一个都没有答 NOT_CONFIGURED——租户取值还没到，不是拒绝，不落册；
//  3. 核修订号等于在册最新加一（首笔必须是 1）；
//  4. 核内容：四个数值不为零、尺寸单位在产品内置计量单位目录里且是长度类、重量单位在目录里且是重量类（ADR-0335）；
//  5. 落册。
//
// 前面一格不成立就答前面那一格。核内容的两类门放在这里，不放进领域构造器：它们只拦将要落册的新修订，读回与重放不经它；构造器只管
// 各格给齐、数值写法合规。零值数值因此一路构造得出来，要到核内容这一格才被拒——在册的修订不可能带零值（库上另有 CHECK 兜住），
// 重放与冲突在找在册修订那一格就判完，不会被它误答。
func (handler *RegisterPackagingMaterialHandler) Register(
	ctx context.Context,
	command RegisterPackagingMaterialCommand,
) (RegisterPackagingMaterialResult, error) {
	candidate, err := domain.NewPackagingMaterialRevision(
		command.Tenant, command.Code, command.Revision, command.Content, command.Registrant, handler.clock.Now(),
	)
	if err != nil {
		var invalid *domain.PackagingMaterialContentFieldError
		switch {
		case errors.Is(err, domain.ErrInvalidRevisionNumber):
			// 小于 1 的修订号肯定不是「最新加一」，与跳号同答错位；但「一个仓都没登记」排在核修订连续之前，所以先过那一道。
			return handler.beforeSequence(ctx, command)
		case errors.As(err, &invalid):
			return packagingMaterialRefused(RegistrationRefusalContentInvalid, invalid.Field), nil
		default:
			// 租户、包材编码与登记责任方都来自接入信封；没经构造器是装配错了，不是登记方能改的输入。
			return RegisterPackagingMaterialResult{}, fmt.Errorf("register packaging material: %w", err)
		}
	}

	existing, found, err := handler.packagingMaterials.LoadPackagingMaterialRevision(ctx, command.Tenant, command.Code, command.Revision)
	if err != nil {
		return RegisterPackagingMaterialResult{}, fmt.Errorf("register packaging material: %w", err)
	}
	var latest domain.PackagingMaterialRevision
	var hasLatest bool
	if !found {
		latest, hasLatest, err = handler.packagingMaterials.LoadLatestPackagingMaterial(ctx, command.Tenant, command.Code)
		if err != nil {
			return RegisterPackagingMaterialResult{}, fmt.Errorf("register packaging material: %w", err)
		}
		if hasLatest && command.Revision <= latest.Revision() {
			// 查册与读最新之间，别的事务把同一修订号落了册。回去再按在册的那一笔判：不然这一笔会被当成跳号答错位，
			// 而它该按内容答重放或冲突（UC-WF-001「一致性、幂等与并发」）。
			existing, found, err = handler.packagingMaterials.LoadPackagingMaterialRevision(ctx, command.Tenant, command.Code, command.Revision)
			if err != nil {
				return RegisterPackagingMaterialResult{}, fmt.Errorf("register packaging material: %w", err)
			}
		}
	}
	if found {
		if existing.SameContentAs(candidate) {
			return packagingMaterialAlreadyRegistered(existing), nil
		}
		return RegisterPackagingMaterialResult{outcome: RegistrationContentConflict}, nil
	}

	configured, err := handler.warehouses.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return RegisterPackagingMaterialResult{}, fmt.Errorf("register packaging material: %w", err)
	}
	if !configured {
		return packagingMaterialNotConfigured(), nil
	}

	latestRevision := 0
	if hasLatest {
		latestRevision = latest.Revision()
	}
	if command.Revision != latestRevision+1 {
		return packagingMaterialOutOfSequenceAt(latestRevision), nil
	}
	if field := inadmissibleCell(candidate.Content()); field != domain.PackagingMaterialFieldInvalid {
		return packagingMaterialRefused(RegistrationRefusalContentInvalid, field), nil
	}

	outcome, err := handler.packagingMaterials.SavePackagingMaterialRevision(ctx, candidate)
	if err != nil {
		return RegisterPackagingMaterialResult{}, fmt.Errorf("register packaging material: %w", err)
	}
	switch outcome {
	case ports.RegistryRevisionSaved:
		return RegisterPackagingMaterialResult{
			outcome: RegistrationRegistered, registered: candidate, hasRegistered: true,
		}, nil
	case ports.RegistryRevisionAlreadyRegistered:
		// 并发里输掉的一笔：别人先落了同一修订号、内容相同。回显在册的那一份，不回显自己这笔。
		landed, found, err := handler.packagingMaterials.LoadPackagingMaterialRevision(ctx, command.Tenant, command.Code, command.Revision)
		if err != nil {
			return RegisterPackagingMaterialResult{}, fmt.Errorf("register packaging material: %w", err)
		}
		if !found {
			return RegisterPackagingMaterialResult{}, errors.New("register packaging material: 登记册答已在册，却读不回那一笔")
		}
		return packagingMaterialAlreadyRegistered(landed), nil
	case ports.RegistryRevisionContentConflict:
		return RegisterPackagingMaterialResult{outcome: RegistrationContentConflict}, nil
	default:
		return RegisterPackagingMaterialResult{}, fmt.Errorf("register packaging material: 登记册答出了未知落点 %d", outcome)
	}
}

// inadmissibleCell 按报文里各格的次序（外廓的长、宽、高与单位，再到重量的数值与单位）找第一个过不了登记门的格：数值为零，或单位不在目录里、
// 类别不对。都过得了交回零值。次序固定，同一份内容有几处不合时答的格才是确定的。
func inadmissibleCell(content domain.PackagingMaterialContent) domain.PackagingMaterialField {
	switch {
	case content.Length().IsZero():
		return domain.PackagingMaterialFieldDimensionsLength
	case content.Width().IsZero():
		return domain.PackagingMaterialFieldDimensionsWidth
	case content.Height().IsZero():
		return domain.PackagingMaterialFieldDimensionsHeight
	case !isUnitOfCategory(content.DimensionsUnit().String(), refdata.UnitCategoryLength):
		return domain.PackagingMaterialFieldDimensionsUnit
	case content.Weight().IsZero():
		return domain.PackagingMaterialFieldWeightValue
	case !isUnitOfCategory(content.WeightUnit().String(), refdata.UnitCategoryWeight):
		return domain.PackagingMaterialFieldWeightUnit
	default:
		return domain.PackagingMaterialFieldInvalid
	}
}

// isUnitOfCategory 只收目录里的码、按码逐字比，且类别要对：`cm` 不是 `CM`，`KG` 不是长度单位。
func isUnitOfCategory(code string, category refdata.UnitCategory) bool {
	unit, found := refdata.Unit(code)
	return found && unit.Category == category
}

// beforeSequence 给修订号本身就立不住的命令用：这样的修订号在册上不可能有对应的修订，没有重放与冲突要判；先过「一个仓都没登记」，
// 再取在册最新修订号答错位。
func (handler *RegisterPackagingMaterialHandler) beforeSequence(
	ctx context.Context,
	command RegisterPackagingMaterialCommand,
) (RegisterPackagingMaterialResult, error) {
	configured, err := handler.warehouses.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return RegisterPackagingMaterialResult{}, fmt.Errorf("register packaging material: %w", err)
	}
	if !configured {
		return packagingMaterialNotConfigured(), nil
	}
	latest, hasLatest, err := handler.packagingMaterials.LoadLatestPackagingMaterial(ctx, command.Tenant, command.Code)
	if err != nil {
		return RegisterPackagingMaterialResult{}, fmt.Errorf("register packaging material: %w", err)
	}
	if !hasLatest {
		return packagingMaterialOutOfSequenceAt(0), nil
	}
	return packagingMaterialOutOfSequenceAt(latest.Revision()), nil
}

func packagingMaterialNotConfigured() RegisterPackagingMaterialResult {
	return RegisterPackagingMaterialResult{outcome: RegistrationNotConfigured}
}

func packagingMaterialOutOfSequenceAt(latestRevision int) RegisterPackagingMaterialResult {
	return RegisterPackagingMaterialResult{
		outcome: RegistrationNotAccepted, refusal: RegistrationRefusalRevisionOutOfSequence, latestRevision: latestRevision,
	}
}

func packagingMaterialRefused(refusal RegistrationRefusal, field domain.PackagingMaterialField) RegisterPackagingMaterialResult {
	return RegisterPackagingMaterialResult{outcome: RegistrationNotAccepted, refusal: refusal, field: field}
}

func packagingMaterialAlreadyRegistered(registered domain.PackagingMaterialRevision) RegisterPackagingMaterialResult {
	return RegisterPackagingMaterialResult{outcome: RegistrationAlreadyRegistered, registered: registered, hasRegistered: true}
}
