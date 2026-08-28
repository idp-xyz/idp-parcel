package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ProductChannelOutcome 是服务形态与产品—渠道映射登记用例的结果代数，判据同
// PartyRegistryOutcome：`已登记`是本次落库；`重复`按同键同内容重放返回原结果；
// `内容冲突`要商业责任方对着册面修正、绝不覆盖（ADR-0031）；`未受理`是输入被领域门
// 或引用检查拒绝（含修订错位与悬空产品引用），一个字节没写。
type ProductChannelOutcome uint8

const (
	ProductChannelOutcomeInvalid ProductChannelOutcome = iota
	ProductChannelRegistered
	ProductChannelAlreadyRegistered
	ProductChannelContentConflict
	ProductChannelNotAccepted
)

func (outcome ProductChannelOutcome) String() string {
	switch outcome {
	case ProductChannelRegistered:
		return "REGISTERED"
	case ProductChannelAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case ProductChannelContentConflict:
		return "CONTENT_CONFLICT"
	case ProductChannelNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// ProductChannelResult 携带落点与`未受理`的原因（判据同 PartyRegistryResult：输入
// 被拒不是技术失败，事务保持可用，批内后项照常推进）。
type ProductChannelResult struct {
	outcome ProductChannelOutcome
	cause   error
}

func (result ProductChannelResult) Outcome() ProductChannelOutcome {
	return result.outcome
}

func (result ProductChannelResult) Cause() error {
	return result.cause
}

func productChannelNotAccepted(cause error) ProductChannelResult {
	return ProductChannelResult{outcome: ProductChannelNotAccepted, cause: cause}
}

// RegisterServiceProductFormCommand 登记一份已发布服务产品版本的服务形态
// （ADR-0050 写侧的进程入口，补 SaveServiceProduct 的调用方）。
type RegisterServiceProductFormCommand struct {
	Tenant   domain.TenantID
	Scope    domain.CommercialScopeReference
	ObjectID domain.CommercialObjectID
	Version  domain.CommercialVersionLabel
	Form     domain.ServiceProductForm
}

// RegisterProductChannelMappingCommand 登记一笔产品—渠道映射修订。Scope 用于装载
// 版本册核对产品引用——产品在哪个范围发布由批文声明，登记面不替它猜。
type RegisterProductChannelMappingCommand struct {
	Tenant   domain.TenantID
	Scope    domain.CommercialScopeReference
	ID       domain.ProductChannelMappingID
	Revision int
	Spec     domain.ProductChannelMappingSpec
}

// RegisterProductChannelHandler 是服务形态与产品—渠道映射登记的写侧编排：领域门 →
// 产品引用与修订连续性检查 → 登记册落库。调用方逐命令各起事务（批不是聚合，
// AT-PC-011）。
//
// 形态册挂在发布登记册的形态切面上（ADR-0050，ports.ServiceProductFormRegistry——
// 本用例不持有 SaveVersion 等发布写口），映射册是自己的登记面（0016）——两个依赖
// 各是各的，不为省一个字段并成一口。
type RegisterProductChannelHandler struct {
	products ports.ServiceProductFormRegistry
	mappings ports.ProductChannelMappingRegistry
}

func NewRegisterProductChannelHandler(
	products ports.ServiceProductFormRegistry,
	mappings ports.ProductChannelMappingRegistry,
) *RegisterProductChannelHandler {
	return &RegisterProductChannelHandler{products: products, mappings: mappings}
}

// RegisterServiceProductForm 登记一份服务产品版本的服务形态。
//
// 版本从整册装载后按键取回：不在册的版本拒绝，登记面不替发布面造版本。形态构造门
// （NewServiceProduct）要求版本已生效——形态是解析采用的内容，挂在还没生效或已收尾
// 的版本上装进登记册也永远选不中（service_product_form.go 装载侧同一条裁决）。
func (handler *RegisterProductChannelHandler) RegisterServiceProductForm(
	ctx context.Context,
	command RegisterServiceProductFormCommand,
) (ProductChannelResult, error) {
	registry, err := handler.products.LoadForScope(ctx, command.Tenant, command.Scope)
	if err != nil {
		// 登记是写权威的动作——整册读不回时不得闭眼登记，照原样上抛等重试
		// （判据同发布用例）。
		return ProductChannelResult{}, fmt.Errorf("register service product form: %w", err)
	}
	version, exists := registry.Lookup(
		command.Tenant, domain.ServiceProductObject, command.ObjectID, command.Version,
	)
	if !exists {
		return productChannelNotAccepted(fmt.Errorf(
			"服务产品版本 %s/%s 不在范围 %s 的版本册上，形态不钉悬空引用",
			command.ObjectID, command.Version, command.Scope,
		)), nil
	}
	product, err := domain.NewServiceProduct(version, command.Form)
	if err != nil {
		return productChannelNotAccepted(fmt.Errorf(
			"服务产品版本 %s/%s（状态 %s）过不了形态构造门：%w",
			command.ObjectID, command.Version, version.Status(), err,
		)), nil
	}

	outcome, err := handler.products.SaveServiceProduct(ctx, product)
	if err != nil {
		return ProductChannelResult{}, fmt.Errorf("register service product form: %w", err)
	}
	switch outcome {
	case ports.ServiceProductSaved:
		return ProductChannelResult{outcome: ProductChannelRegistered}, nil
	case ports.ServiceProductAlreadyRegistered:
		return ProductChannelResult{outcome: ProductChannelAlreadyRegistered}, nil
	case ports.ServiceProductContentConflict:
		return ProductChannelResult{outcome: ProductChannelContentConflict}, nil
	default:
		return ProductChannelResult{}, fmt.Errorf(
			"register service product form: unexpected save outcome %d", outcome)
	}
}

// RegisterMapping 登记一笔产品—渠道映射修订。
//
// 产品引用必须指着册上的服务产品版本，且该版本未收尾：CONTEXT「服务产品版本退役只
// 停止其参与新的商业选择」，登记新映射正是一次新的商业选择；已计划生效（PUBLISHED）
// 的版本可以先配映射——渠道候选在产品开卖前备好是常态，不是边缘。形态是否已登记
// 刻意不做门（ADR-0050：形态缺席不使解析退化，映射与形态两册各自为政）。
func (handler *RegisterProductChannelHandler) RegisterMapping(
	ctx context.Context,
	command RegisterProductChannelMappingCommand,
) (ProductChannelResult, error) {
	registration, err := domain.NewProductChannelMappingRegistration(
		command.Tenant, command.ID, command.Revision, command.Spec,
	)
	if err != nil {
		return productChannelNotAccepted(err), nil
	}

	registry, err := handler.products.LoadForScope(ctx, command.Tenant, command.Scope)
	if err != nil {
		return ProductChannelResult{}, fmt.Errorf("register product channel mapping: %w", err)
	}
	version, exists := registry.Lookup(
		command.Tenant, domain.ServiceProductObject, command.Spec.Product, command.Spec.ProductVersion,
	)
	if !exists {
		return productChannelNotAccepted(fmt.Errorf(
			"服务产品版本 %s/%s 不在范围 %s 的版本册上，映射不钉悬空引用",
			command.Spec.Product, command.Spec.ProductVersion, command.Scope,
		)), nil
	}
	switch version.Status() {
	case domain.CommercialVersionPublished, domain.CommercialVersionEffective:
	default:
		return productChannelNotAccepted(fmt.Errorf(
			"服务产品版本 %s/%s 已收尾（状态 %s），不再支持新的映射登记",
			command.Spec.Product, command.Spec.ProductVersion, version.Status(),
		)), nil
	}

	latest, found, err := handler.mappings.LoadLatestMapping(ctx, command.Tenant, command.ID)
	if err != nil {
		return ProductChannelResult{}, fmt.Errorf("register product channel mapping: %w", err)
	}
	if found {
		// 映射标识钉着它的产品版本：改指产品是另一笔映射，不是本映射的新修订——
		// 放行会让「已经固定到委托或交易的映射依据」横跨两个产品而无从回答。
		if latest.Product() != command.Spec.Product ||
			latest.ProductVersion() != command.Spec.ProductVersion {
			return productChannelNotAccepted(fmt.Errorf(
				"映射 %s 钉着产品版本 %s/%s，不改指 %s/%s——那是另一笔映射，登记新映射标识",
				command.ID, latest.Product(), latest.ProductVersion(),
				command.Spec.Product, command.Spec.ProductVersion,
			)), nil
		}
		if command.Revision > latest.Revision()+1 {
			return productChannelNotAccepted(fmt.Errorf(
				"修订必须连续：册上最新为 %d，收到 %d", latest.Revision(), command.Revision,
			)), nil
		}
	} else if command.Revision != 1 {
		return productChannelNotAccepted(fmt.Errorf(
			"首笔登记修订必须是 1，收到 %d", command.Revision,
		)), nil
	}

	outcome, err := handler.mappings.SaveMapping(ctx, registration)
	if err != nil {
		return ProductChannelResult{}, fmt.Errorf("register product channel mapping: %w", err)
	}
	switch outcome {
	case ports.MappingSaved:
		return ProductChannelResult{outcome: ProductChannelRegistered}, nil
	case ports.MappingAlreadyRegistered:
		return ProductChannelResult{outcome: ProductChannelAlreadyRegistered}, nil
	case ports.MappingContentConflict:
		return ProductChannelResult{outcome: ProductChannelContentConflict}, nil
	default:
		return ProductChannelResult{}, fmt.Errorf(
			"register product channel mapping: unexpected save outcome %d", outcome)
	}
}
