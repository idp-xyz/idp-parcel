package application

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// RegisterGoodsOutcome 是商品登记用例的结果字面量，与 UC-WF-002「结果语义契约」逐行对应。它们是平铺的结果，不套仓那几册的 `NOT_ACCEPTED` 加原因：
// 这个口是客户命令与运营决定，不是登记册的配置写。输入被拒是已形成的答案，不是技术失败；只有册读写不动才另走 error。
type RegisterGoodsOutcome uint8

const (
	RegisterGoodsOutcomeInvalid RegisterGoodsOutcome = iota
	// GoodsOutcomeRegistered 是新修订落册，自登记那一刻生效。
	GoodsOutcomeRegistered
	// GoodsOutcomeExistingResult 是同一个键、内容相同的重投：回显在册的那一笔。
	GoodsOutcomeExistingResult
	// GoodsOutcomeSourceConflict 是同一个键、内容不同：在册的不变。
	GoodsOutcomeSourceConflict
	// GoodsOutcomeBasisStale 是基准修订不是当前修订：别人已先落了新修订，没带基准而商品已登记，或带了基准而商品还没登记。
	GoodsOutcomeBasisStale
	// GoodsOutcomeImmutableField 是改了登记后不改的格，或商品有在库库存时改了是否跟踪批号、效期。
	GoodsOutcomeImmutableField
	// GoodsOutcomeIdentifierConflict 是商品编码或别名在同一货主内同一时刻已指向别的商品，或同一份资料里一个码出现两次。
	GoodsOutcomeIdentifierConflict
	// GoodsOutcomeContentInvalid 是包装层级、组成、目录码、数值或有效区间不成立。
	GoodsOutcomeContentInvalid
	// GoodsOutcomeNotConfigured 是本租户一个仓都没登记：租户取值还没到，不是拒绝，不落册。
	GoodsOutcomeNotConfigured
)

// ErrOwnerAccountNotInTenant 是代录指名的货主客户账户在本租户没有登记。这是已形成的答案，与越权探针同答，
// 不是八个业务结果之一，也不是技术失败：调用方不得把它写成 REGISTERED 或 NOT_CONFIGURED，也不得落册。
var ErrOwnerAccountNotInTenant = errors.New("register goods: owner account is not registered in this tenant")

var errOwnerLookupMissing = errors.New("owner account lookup is not wired")

func (outcome RegisterGoodsOutcome) String() string {
	switch outcome {
	case GoodsOutcomeRegistered:
		return "REGISTERED"
	case GoodsOutcomeExistingResult:
		return "EXISTING_RESULT"
	case GoodsOutcomeSourceConflict:
		return "SOURCE_CONFLICT"
	case GoodsOutcomeBasisStale:
		return "BASIS_STALE"
	case GoodsOutcomeImmutableField:
		return "IMMUTABLE_FIELD"
	case GoodsOutcomeIdentifierConflict:
		return "IDENTIFIER_CONFLICT"
	case GoodsOutcomeContentInvalid:
		return "CONTENT_INVALID"
	case GoodsOutcomeNotConfigured:
		return "NOT_CONFIGURED"
	default:
		return ""
	}
}

// CurrentGoodsBasis 是「基准过期」回显的当前修订：商品已登记时带当前修订号，没登记时如实说没登记。没登记不用 0 兼表达，免得登记方把「没登记」
// 读成「当前修订是 0」。
type CurrentGoodsBasis struct {
	registered bool
	revision   int
}

// Registered 回答商品有没有登记。
func (basis CurrentGoodsBasis) Registered() bool {
	return basis.registered
}

// Revision 是当前修订号；商品没登记时是 0，调用方须先看 Registered。
func (basis CurrentGoodsBasis) Revision() int {
	return basis.revision
}

// RegisterGoodsResult 携带落点与各落点的细节，细节只在对应的落点上有值，别的落点上读它们得零值。
type RegisterGoodsResult struct {
	outcome     RegisterGoodsOutcome
	revision    domain.GoodsRevision
	hasRevision bool
	current     CurrentGoodsBasis
	immutable   domain.GoodsImmutableCell
	conflicting domain.IdentifierCode
	cells       []domain.GoodsCell
}

func (result RegisterGoodsResult) Outcome() RegisterGoodsOutcome {
	return result.outcome
}

// Revision 只在 REGISTERED 与 EXISTING_RESULT 时有：落册或在册的那一笔。重放回显的是在册那一份，登记责任方与登记时刻因此是当初落册时的，
// 不是这次重投的。
func (result RegisterGoodsResult) Revision() (domain.GoodsRevision, bool) {
	return result.revision, result.hasRevision
}

// CurrentBasis 只在 BASIS_STALE 时有值。
func (result RegisterGoodsResult) CurrentBasis() CurrentGoodsBasis {
	return result.current
}

// ImmutableCell 只在 IMMUTABLE_FIELD 时有值：被改的那一格。
func (result RegisterGoodsResult) ImmutableCell() domain.GoodsImmutableCell {
	return result.immutable
}

// ConflictingCode 只在 IDENTIFIER_CONFLICT 时有值：撞上的那个码。
func (result RegisterGoodsResult) ConflictingCode() (domain.IdentifierCode, bool) {
	return result.conflicting, result.outcome == GoodsOutcomeIdentifierConflict
}

// InvalidCells 只在 CONTENT_INVALID 时有值：全部不合的格，逐格列出，次序确定（见 RegisterGoodsHandler.Register）。
func (result RegisterGoodsResult) InvalidCells() []domain.GoodsCell {
	return slices.Clone(result.cells)
}

// RegisterGoodsCommand 登记商品的一笔修订。Tenant 与 Registrant 取自认证信封，不取自载荷；货主客户账户在客户面取自信封、代录时是载荷里的业务内容。
// Basis 是所基于的修订，首次登记不带。Submission 带提交途径、来源与键，代录另带依据引用。
type RegisterGoodsCommand struct {
	Tenant     domain.TenantID
	Owner      domain.OwnerAccountID
	Code       domain.GoodsCode
	Basis      domain.BasisRevision
	Content    domain.GoodsContent
	Submission domain.GoodsSubmission
	Registrant domain.Registrant
}

func (command RegisterGoodsCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.Owner.String() == "":
		return errors.New("缺货主客户账户")
	case command.Code.String() == "":
		return errors.New("缺商品编码")
	case command.Registrant.String() == "":
		return errors.New("缺登记责任方")
	case command.Submission.Path() == domain.SubmissionPathInvalid || command.Submission.Key().String() == "":
		return errors.New("缺提交")
	default:
		return nil
	}
}

func (command RegisterGoodsCommand) request() domain.GoodsRequest {
	return domain.NewGoodsRequest(command.Owner, command.Code, command.Basis, command.Content, command.Submission)
}

// RegisterGoodsHandler 是商品登记册的写侧编排，货主口与代录口走同一条。事务边界在调用方：每条命令各起一个环境事务，登记册的保存在没有环境事务时拒绝。
//
// 并发靠登记册的保存兜底，这里不加锁：保存把判重、修订号接得上与识别码不撞串在落册里（见 ports.GoodsRegistry），输给并发的一笔由这里译成对应的结果。
// 「本租户有没有登记过仓」只读仓册，不写：仓册只追加，登记过仓的租户不会再回到「一个仓都没有」，所以这一读不需要锁。
type RegisterGoodsHandler struct {
	goods      ports.GoodsRegistry
	warehouses ports.TenantWarehouseLookup
	stock      ports.GoodsStockLookup
	owners     ports.OwnerAccountLookup
	clock      ports.Clock
}

func NewRegisterGoodsHandler(
	goods ports.GoodsRegistry,
	warehouses ports.TenantWarehouseLookup,
	stock ports.GoodsStockLookup,
	owners ports.OwnerAccountLookup,
	clock ports.Clock,
) *RegisterGoodsHandler {
	return &RegisterGoodsHandler{goods: goods, warehouses: warehouses, stock: stock, owners: owners, clock: clock}
}

// Register 登记一笔商品修订。代录先核货主客户账户在本租户有没有登记，这一步排在下面的次序之前：
// 查不到答 ErrOwnerAccountNotInTenant，不读仓册、不落册；读不动走 error，不形成答案。货主口不问这一口。
// 其后的判定次序（UC-WF-002「应用流程」），前面一格不成立就答前面那一格：
//
//  1. 本租户一个仓都没登记：NOT_CONFIGURED，不落册；
//  2. 按（提交途径，来源，键）认重放与冲突：判重要比的内容相同答 EXISTING_RESULT，不同答 SOURCE_CONFLICT；
//  3. 基准修订须是当前修订：不是答 BASIS_STALE，回显当前修订号，商品没登记的如实说没登记；
//  4. 不可改的格：基本单位；是否组合商品；已声明的包装层级不删、折合数不改；商品有在库库存时改是否跟踪批号、效期。答 IMMUTABLE_FIELD 并指出那一格；
//  5. 识别码：同一份资料里一个码出现两次，或码在同一货主内与别的商品的当前修订相撞。答 IDENTIFIER_CONFLICT 并指出那个码；
//  6. 资料内容：逐格指出全部不合的格，答 CONTENT_INVALID；
//  7. 落册，答 REGISTERED。
//
// 重放排在基准之前：重放的是已经形成的答案，不因为此后别人又落了新修订就变成基准过期。查有没有在库库存只在改了跟踪属性时才问，别的修订不为它多读一次库存账。
// 资料内容里「组件是已登记的非组合商品」读的是别的商品的当前修订，这一格只会随时间变得更确定——非组合商品不能改成组合，登记过的商品不会消失。
func (handler *RegisterGoodsHandler) Register(ctx context.Context, command RegisterGoodsCommand) (RegisterGoodsResult, error) {
	if err := command.assemblyError(); err != nil {
		// 租户、货主、商品编码、提交与登记责任方没经构造器，是装配错了，不是登记方能改的输入。
		return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", err)
	}
	if command.Submission.Path() == domain.SubmissionOnBehalf {
		if handler.owners == nil {
			return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", errOwnerLookupMissing)
		}
		registered, err := handler.owners.OwnerAccountRegistered(ctx, command.Tenant, command.Owner)
		if err != nil {
			return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", err)
		}
		if !registered {
			return RegisterGoodsResult{}, ErrOwnerAccountNotInTenant
		}
	}

	configured, err := handler.warehouses.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", err)
	}
	if !configured {
		return RegisterGoodsResult{outcome: GoodsOutcomeNotConfigured}, nil
	}

	existing, found, err := handler.goods.LoadGoodsRevisionBySubmission(ctx, command.Tenant, command.Submission)
	if err != nil {
		return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", err)
	}
	if found {
		return replayResult(existing, command), nil
	}

	latest, hasLatest, err := handler.goods.LoadLatestGoods(ctx, command.Tenant, command.Owner, command.Code)
	if err != nil {
		return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", err)
	}
	if current, stale := staleBasis(command.Basis, latest, hasLatest); stale {
		// 查过键之后、核基准之前，别人可能刚用同一个键落了册。重放排在基准之前：回去再认一次键，
		// 认到了就按内容答，不把已经形成的重放或来源冲突答成基准过期。
		existing, found, err = handler.goods.LoadGoodsRevisionBySubmission(ctx, command.Tenant, command.Submission)
		if err != nil {
			return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", err)
		}
		if found {
			return replayResult(existing, command), nil
		}
		return RegisterGoodsResult{outcome: GoodsOutcomeBasisStale, current: current}, nil
	}

	if hasLatest {
		cell, immutable, err := handler.immutableChange(ctx, command, latest)
		if err != nil {
			return RegisterGoodsResult{}, err
		}
		if immutable {
			return RegisterGoodsResult{outcome: GoodsOutcomeImmutableField, immutable: cell}, nil
		}
	}

	if repeated, repeats := command.Content.RepeatedIdentifier(command.Code); repeats {
		return RegisterGoodsResult{outcome: GoodsOutcomeIdentifierConflict, conflicting: repeated}, nil
	}
	taken, isTaken, err := handler.goods.FindGoodsIdentifierConflict(
		ctx, command.Tenant, command.Owner, command.Code, command.Content.IdentifierClaims(command.Code))
	if err != nil {
		return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", err)
	}
	if isTaken {
		return RegisterGoodsResult{outcome: GoodsOutcomeIdentifierConflict, conflicting: taken}, nil
	}

	cells, err := handler.contentDefects(ctx, command)
	if err != nil {
		return RegisterGoodsResult{}, err
	}
	if len(cells) > 0 {
		return RegisterGoodsResult{outcome: GoodsOutcomeContentInvalid, cells: cells}, nil
	}

	number := 1
	if hasLatest {
		number = latest.Revision() + 1
	}
	candidate, err := domain.NewGoodsRevision(
		command.Tenant, command.Owner, command.Code, number, command.Content, command.Submission,
		command.Registrant, handler.clock.Now(),
	)
	if err != nil {
		return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", err)
	}
	return handler.save(ctx, command, candidate)
}

// replayResult 按在册那一笔判重放：判重要比的内容相同答 EXISTING_RESULT 并回显在册的，不同答 SOURCE_CONFLICT。
func replayResult(existing domain.GoodsRevision, command RegisterGoodsCommand) RegisterGoodsResult {
	if existing.Request().SameAs(command.request()) {
		return RegisterGoodsResult{outcome: GoodsOutcomeExistingResult, revision: existing, hasRevision: true}
	}
	return RegisterGoodsResult{outcome: GoodsOutcomeSourceConflict}
}

// staleBasis 判基准是不是当前修订：首次登记（没带基准、商品没登记）不过期；没带基准而商品已登记、带了基准而商品没登记、基准不是当前修订都过期。
func staleBasis(basis domain.BasisRevision, latest domain.GoodsRevision, hasLatest bool) (CurrentGoodsBasis, bool) {
	switch {
	case !hasLatest && !basis.Given():
		return CurrentGoodsBasis{}, false
	case !hasLatest:
		return CurrentGoodsBasis{}, true
	case basis.Given() && basis.Number() == latest.Revision():
		return CurrentGoodsBasis{}, false
	default:
		return CurrentGoodsBasis{registered: true, revision: latest.Revision()}, true
	}
}

// immutableChange 找新资料相对在册最新那一笔改了哪个登记后不改的格。结构格先判；是否跟踪批号与效期只在商品有在库库存时才不可改，要问库存账。
func (handler *RegisterGoodsHandler) immutableChange(
	ctx context.Context,
	command RegisterGoodsCommand,
	latest domain.GoodsRevision,
) (domain.GoodsImmutableCell, bool, error) {
	if cell, changed := command.Content.StructuralChangeFrom(latest.Content()); changed {
		return cell, true, nil
	}
	cell, changed := command.Content.TrackingChangeFrom(latest.Content())
	if !changed {
		return domain.GoodsImmutableCell{}, false, nil
	}
	onHand, err := handler.stock.HasOnHandStock(ctx, command.Tenant, command.Owner, command.Code)
	if err != nil {
		return domain.GoodsImmutableCell{}, false, fmt.Errorf("register goods: %w", err)
	}
	return cell, onHand, nil
}

// save 落册，并把保存在并发下的定论译成结果：重放与冲突按保存时在册的那一笔答，基准过期回显此刻的当前修订，识别码冲突指出撞上的码。
func (handler *RegisterGoodsHandler) save(
	ctx context.Context,
	command RegisterGoodsCommand,
	candidate domain.GoodsRevision,
) (RegisterGoodsResult, error) {
	saved, err := handler.goods.SaveGoodsRevision(ctx, candidate)
	if err != nil {
		return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", err)
	}
	switch saved.Outcome {
	case ports.GoodsRevisionSaved:
		return RegisterGoodsResult{outcome: GoodsOutcomeRegistered, revision: candidate, hasRevision: true}, nil
	case ports.GoodsRevisionAlreadyRegistered:
		// 并发里输掉的一笔：别人先落了同一个键、内容相同。回显在册的那一份，不回显自己这笔。
		landed, found, err := handler.goods.LoadGoodsRevisionBySubmission(ctx, command.Tenant, command.Submission)
		if err != nil {
			return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", err)
		}
		if !found {
			return RegisterGoodsResult{}, errors.New("register goods: 登记册答已在册，却读不回那一笔")
		}
		return replayResult(landed, command), nil
	case ports.GoodsSourceConflict:
		return RegisterGoodsResult{outcome: GoodsOutcomeSourceConflict}, nil
	case ports.GoodsBasisStale:
		latest, hasLatest, err := handler.goods.LoadLatestGoods(ctx, command.Tenant, command.Owner, command.Code)
		if err != nil {
			return RegisterGoodsResult{}, fmt.Errorf("register goods: %w", err)
		}
		current := CurrentGoodsBasis{}
		if hasLatest {
			current = CurrentGoodsBasis{registered: true, revision: latest.Revision()}
		}
		return RegisterGoodsResult{outcome: GoodsOutcomeBasisStale, current: current}, nil
	case ports.GoodsIdentifierConflict:
		return RegisterGoodsResult{outcome: GoodsOutcomeIdentifierConflict, conflicting: saved.ConflictingCode}, nil
	default:
		return RegisterGoodsResult{}, fmt.Errorf("register goods: 登记册答出了未知落点 %d", saved.Outcome)
	}
}
