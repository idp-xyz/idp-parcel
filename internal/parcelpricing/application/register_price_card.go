package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// ErrUnexpectedPriceCardOutcome 说明价卡仓储交回了结果代数以外的写入结果。
var ErrUnexpectedPriceCardOutcome = errors.New("parcel pricing: unexpected price card registration outcome")

// RegisterPriceCardOutcome 是价卡登记请求的应用处理结果。
type RegisterPriceCardOutcome uint8

const (
	RegisterPriceCardOutcomeInvalid RegisterPriceCardOutcome = iota
	// PriceCardRecorded：新版本已入册。
	PriceCardRecorded
	// PriceCardAlreadyOnRegister：同版本同内容已在册，重复登记是幂等重放。
	PriceCardAlreadyOnRegister
	// PriceCardRegistrationConflict：版本内容冲突（同版本引用、同规范化版本、异
	// 内容摘要）。原行不被顶替，续办属治理裁决。
	PriceCardRegistrationConflict
	// PriceCardRegistrationIncomparable：同版本引用已按另一套规范化版本在册，摘要
	// 不可比（ADR-0014）——不是冲突，也不是幂等重放。
	PriceCardRegistrationIncomparable
	// PriceCardRegistrationNotAccepted：请求不合法（零值登记），未到达仓储。
	PriceCardRegistrationNotAccepted
	// PriceCardRegistrationUndecided：依赖故障，登记与否未知，原因随错误交回。
	PriceCardRegistrationUndecided
	// PriceCardRegistrationNewerCanonicalizationNotEnabled：方案记的规范化版本不是基础版，而本部署没开登记较新版本
	// 的开关；未到达仓储，什么都没写。续办是升级全部读价卡的进程后由运维开开关再重试，不是改卡。
	PriceCardRegistrationNewerCanonicalizationNotEnabled
	// PriceCardRegistrationDirectionAuthorizationNotPublished：引用的授权不在册，或还没发布。未入册。
	PriceCardRegistrationDirectionAuthorizationNotPublished
	// PriceCardRegistrationDirectionAuthorizationNotInForce：授权已到期、已收尾，或登记时刻不在它的有效区间内。未入册。
	PriceCardRegistrationDirectionAuthorizationNotInForce
	// PriceCardRegistrationDirectionAuthorizationDirectionMismatch：授权的方向与价卡方向不同。未入册。
	PriceCardRegistrationDirectionAuthorizationDirectionMismatch
)

func (outcome RegisterPriceCardOutcome) String() string {
	switch outcome {
	case PriceCardRecorded:
		return "RECORDED"
	case PriceCardAlreadyOnRegister:
		return "ALREADY_REGISTERED"
	case PriceCardRegistrationConflict:
		return "CONTENT_CONFLICT"
	case PriceCardRegistrationIncomparable:
		return "CANONICALIZATION_DIFFERS"
	case PriceCardRegistrationNotAccepted:
		return "NOT_ACCEPTED"
	case PriceCardRegistrationUndecided:
		return "UNDECIDED"
	case PriceCardRegistrationNewerCanonicalizationNotEnabled:
		return "NEWER_CANONICALIZATION_NOT_ENABLED"
	case PriceCardRegistrationDirectionAuthorizationNotPublished:
		return "DIRECTION_AUTHORIZATION_NOT_PUBLISHED"
	case PriceCardRegistrationDirectionAuthorizationNotInForce:
		return "DIRECTION_AUTHORIZATION_EXPIRED_OR_CLOSED"
	case PriceCardRegistrationDirectionAuthorizationDirectionMismatch:
		return "DIRECTION_AUTHORIZATION_DIRECTION_MISMATCH"
	default:
		return ""
	}
}

// RegisterPriceCardCommand 携带一次价卡登记请求。登记本体（方案版本、源文件身份、
// 方向授权引用、发布批准责任方）全部在领域对象内，编排不拆解它。
type RegisterPriceCardCommand struct {
	Registration domain.PriceCardRegistration
}

// RegisterPriceCardDeps 是价卡登记的依赖。Registered 交「参数已登记」：结算那一侧停在没卡、卡上没声明取整或评价采用
// 不了的请求，补登一版卡之后由它续办（ADR-0229 决定五）。缺了它登记照样成功，停着的请求却再也等不来续办，所以必备。
//
// NewerCanonicalizationEnabled 是部署开关「允许登记基础版之外的规范化版本」，由装配从环境读一次后交进来，零值即关。
// 只认基础版的旧进程读到较新版本的卡会停在「不认这一版」，租户级的序列与目录登记 / 复核还会整笔失败；所以第一张
// 较新版本的卡入册之前，所有读价卡的进程必须已是新构建——这件事只有部署方知道，产品不替它猜（ADR-0352）。
//
// DirectionAuthorizations 在入册前核卡上引用的价格方向授权。缺了它，新登记答未决：不能把没核过的引用写成已登记。
// RegisteredContent 用来认出同一份已入册内容的幂等重放；重放答已登记，不再用授权这道门把重放打成拒绝。生产上它就是
// 价卡册自己。Clock 是登记时刻；没装时用当前时刻。
type RegisterPriceCardDeps struct {
	Catalog                      ports.PriceCardCatalog
	Registered                   ports.PricingParameterRegisteredHandoff
	NewerCanonicalizationEnabled bool
	DirectionAuthorizations      ports.DirectionAuthorizationReader
	RegisteredContent            ports.PriceCardVersionLoader
	Clock                        ports.Clock
}

// ErrPriceCardRegistrationHandoffMissing 说明装配没给价卡登记接「参数已登记」交接口。
var ErrPriceCardRegistrationHandoffMissing = errors.New("parcel pricing: price card registration has no pricing parameter registered handoff")

// ErrPriceCardDirectionAuthorizationUnreadable 说明授权读口没装，或交回了结果代数以外的答复。登记与否未知。
var ErrPriceCardDirectionAuthorizationUnreadable = errors.New("parcel pricing: price card direction authorization could not be read")

// RegisterPriceCardHandler 编排价卡登记用例（票 07 件③）。治理判断全在领域与仓储
// （登记门在构造器，冲突判定在仓储的结果代数），这里只做受理与答案翻译。
type RegisterPriceCardHandler struct {
	deps RegisterPriceCardDeps
}

func NewRegisterPriceCardHandler(deps RegisterPriceCardDeps) *RegisterPriceCardHandler {
	return &RegisterPriceCardHandler{deps: deps}
}

// Handle 把一次登记请求推进到登记册答案。与评价用例不同，依赖故障不吞：登记是治理
// 动作，操作者必须拿到失败原因才能续办，答一个没有成因的`未决`等于没答。
func (handler *RegisterPriceCardHandler) Handle(
	ctx context.Context,
	command RegisterPriceCardCommand,
) (RegisterPriceCardOutcome, error) {
	// 零值登记连租户都指不出来。构造器保证非零登记整体立得住，所以受理门只需分辨
	// 「根本没构造过」。
	if command.Registration.Tenant().String() == "" {
		return PriceCardRegistrationNotAccepted, nil
	}
	if handler.deps.Registered == nil {
		return PriceCardRegistrationUndecided, ErrPriceCardRegistrationHandoffMissing
	}
	// 门在仓储之前、只看卡本身：开关关着时，同一版已在册的重放也拒。代价是重放拿不到`已在册`；换来的是门不读库，
	// 关着的部署里任何较新版本的登记都不碰登记册、不发「参数已登记」交接（ADR-0352）。写成「不是基础版」而不是
	// 「是 PPC-6」：再有新版时同此办，不必回来改门。
	if command.Registration.Plan().CanonicalizationVersion() != domain.CurrentCanonicalizationVersion() &&
		!handler.deps.NewerCanonicalizationEnabled {
		return PriceCardRegistrationNewerCanonicalizationNotEnabled, nil
	}
	// 同一份已入册内容的重放不进授权这道门：旧卡引用后来悬空或被收尾，重放仍答已登记，不把它们赶出选择。
	// 内容不同的尝试（含同版本异文）和新登记仍要核。规范化版本那道门在前面，不因这一段而放行较新版本的重放。
	same, err := handler.sameRegisteredContent(ctx, command.Registration)
	if err != nil {
		return PriceCardRegistrationUndecided, fmt.Errorf("register price card: %w", err)
	}
	if !same {
		refused, refuseErr := assessCitedDirectionAuthorization(ctx, handler.deps.DirectionAuthorizations, handler.deps.Clock,
			command.Registration.Tenant(), command.Registration.DirectionAuthorization(), command.Registration.Plan().Direction())
		if refuseErr != nil {
			return refused, fmt.Errorf("register price card: %w", refuseErr)
		}
		if refused != 0 {
			return refused, nil
		}
	}

	saved, err := handler.deps.Catalog.Register(ctx, command.Registration)
	if err != nil {
		return PriceCardRegistrationUndecided, fmt.Errorf("register price card: %w", err)
	}
	switch saved {
	case ports.PriceCardRegistered:
		return handler.announce(ctx, command.Registration, PriceCardRecorded)
	case ports.PriceCardAlreadyRegistered:
		return handler.announce(ctx, command.Registration, PriceCardAlreadyOnRegister)
	case ports.PriceCardContentConflict:
		return PriceCardRegistrationConflict, nil
	case ports.PriceCardCanonicalizationDiffers:
		return PriceCardRegistrationIncomparable, nil
	default:
		return RegisterPriceCardOutcomeInvalid, fmt.Errorf("%w: %d", ErrUnexpectedPriceCardOutcome, saved)
	}
}

// announce 在这一版卡落库（或重放命中同一版）之后交「参数已登记」，带卡的计价范围与方向。重放重发同一封，由交接口按
// 同一个标识去重；本口落地之前登进册的卡也借重放补上它那一封。冲突与形状不同不交：那一版没写进册。交接失败上抛，
// 信封与登记行同生共死。
func (handler *RegisterPriceCardHandler) announce(
	ctx context.Context,
	registration domain.PriceCardRegistration,
	outcome RegisterPriceCardOutcome,
) (RegisterPriceCardOutcome, error) {
	plan := registration.Plan()
	if err := announcePricingParameter(ctx, handler.deps.Registered, registration.Tenant(),
		ports.PricingParameterPriceCard, plan.Reference(),
		[]ports.PricedScope{{Scope: plan.Scope(), Direction: plan.Direction()}}); err != nil {
		return PriceCardRegistrationUndecided, fmt.Errorf("register price card: %w", err)
	}
	return outcome, nil
}

// sameRegisteredContent 回答册上是不是已经有同一方案版本、同一规范化版本、同一内容摘要。读不回是未决。
// 没装读口时答否：认不出重放，于是新登记照常走授权门，不会把没核过的卡放进去。
func (handler *RegisterPriceCardHandler) sameRegisteredContent(
	ctx context.Context,
	registration domain.PriceCardRegistration,
) (bool, error) {
	if handler.deps.RegisteredContent == nil {
		return false, nil
	}
	stored, found, err := handler.deps.RegisteredContent.FindByReference(ctx, registration.Tenant(), registration.Plan().Reference())
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	incoming := registration.Plan()
	return stored.CanonicalizationVersion() == incoming.CanonicalizationVersion() &&
		stored.ContentDigest() == incoming.ContentDigest(), nil
}

// assessCitedDirectionAuthorization 是登记与预览共用的那一次核对。通过时结果为零。读口没装、读失败、
// 答复不在封闭集里都是未决，不是拒绝。
func assessCitedDirectionAuthorization(
	ctx context.Context,
	reader ports.DirectionAuthorizationReader,
	clock ports.Clock,
	tenant domain.TenantID,
	authorization domain.VersionReference,
	direction domain.PricingDirection,
) (RegisterPriceCardOutcome, error) {
	if reader == nil {
		return PriceCardRegistrationUndecided, ErrPriceCardDirectionAuthorizationUnreadable
	}
	at := time.Now().UTC()
	if clock != nil {
		at = clock.Now().UTC()
	}
	if at.IsZero() {
		return PriceCardRegistrationUndecided, ErrPriceCardDirectionAuthorizationUnreadable
	}
	assessment, err := reader.ReadDirectionAuthorization(ctx, ports.DirectionAuthorizationQuery{
		Tenant: tenant, Authorization: authorization, Direction: direction, At: at,
	})
	if err != nil {
		return PriceCardRegistrationUndecided, err
	}
	switch assessment {
	case ports.DirectionAuthorizationMatches:
		return 0, nil
	case ports.DirectionAuthorizationNotPublished:
		return PriceCardRegistrationDirectionAuthorizationNotPublished, nil
	case ports.DirectionAuthorizationNotInForce:
		return PriceCardRegistrationDirectionAuthorizationNotInForce, nil
	case ports.DirectionAuthorizationDirectionMismatch:
		return PriceCardRegistrationDirectionAuthorizationDirectionMismatch, nil
	default:
		return PriceCardRegistrationUndecided, fmt.Errorf("%w: %d", ErrPriceCardDirectionAuthorizationUnreadable, assessment)
	}
}
