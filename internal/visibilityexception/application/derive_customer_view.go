package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ErrUnexpectedDisclosureState 说明披露策略端口交回了封闭三态以外的维度答复。上抛而
// 不译成业务结果：集合外的取值没有恢复动作可派。
var ErrUnexpectedDisclosureState = errors.New("visibility exception: unexpected disclosure dimension state")

// DeriveCustomerViewOutcome 是按投影新版本派生客户视图的应用处理结果。
type DeriveCustomerViewOutcome uint8

const (
	DeriveCustomerViewOutcomeInvalid DeriveCustomerViewOutcome = iota
	CustomerViewPublished
	CustomerViewExistingResult
	CustomerViewUndecided
	CustomerViewNotAccepted
)

func (outcome DeriveCustomerViewOutcome) String() string {
	switch outcome {
	case CustomerViewPublished:
		return "PUBLISHED"
	case CustomerViewExistingResult:
		return "EXISTING_RESULT"
	case CustomerViewUndecided:
		return "UNDECIDED"
	case CustomerViewNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// CustomerViewUndecidedReason 指名派生停在哪一步。
type CustomerViewUndecidedReason uint8

const (
	CustomerViewUndecidedReasonNone CustomerViewUndecidedReason = iota
	DisclosurePolicyUnavailable
	CustomerViewStoreUnavailable
	CustomerViewIdentityUnavailable
)

func (reason CustomerViewUndecidedReason) String() string {
	switch reason {
	case DisclosurePolicyUnavailable:
		return "DISCLOSURE_POLICY_UNAVAILABLE"
	case CustomerViewStoreUnavailable:
		return "CUSTOMER_VIEW_STORE_UNAVAILABLE"
	case CustomerViewIdentityUnavailable:
		return "CUSTOMER_VIEW_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// DeriveCustomerViewCommand 携带一份新派生的投影与它所属的租户、货主客户账户。账户由
// 调用方给出而不是从投影推导——投影面向包裹，不认识账户；而「客户视图只包含当前货主
// 客户账户及其授权对象范围」要求账户隔离从入口就是字段。租户同理（ADR-0003）。
type DeriveCustomerViewCommand struct {
	TenantID   domain.TenantID
	Customer   domain.CustomerAccountReference
	Projection domain.TrackingProjection
}

type DeriveCustomerViewResult struct {
	outcome    DeriveCustomerViewOutcome
	view       domain.CustomerTrackingView
	hasView    bool
	reason     CustomerViewUndecidedReason
	handoffRef string
}

func (result DeriveCustomerViewResult) Outcome() DeriveCustomerViewOutcome {
	return result.outcome
}

// View 只在发布成功或读回已有视图时给出。
func (result DeriveCustomerViewResult) View() (domain.CustomerTrackingView, bool) {
	return result.view, result.hasView
}

func (result DeriveCustomerViewResult) UndecidedReason() CustomerViewUndecidedReason {
	return result.reason
}

// HandoffReference 非空说明视图已发布（或已存在）但意图还没交出去，重放会重发同一份。
func (result DeriveCustomerViewResult) HandoffReference() string {
	return result.handoffRef
}

type DeriveCustomerViewDeps struct {
	Policy     ports.DisclosurePolicyView
	Views      ports.CustomerViewStore
	Identities ports.CustomerViewIdentityFactory
	Downstream ports.CustomerViewHandoff
	Clock      ports.Clock
}

type DeriveCustomerViewHandler struct {
	deps DeriveCustomerViewDeps
}

func NewDeriveCustomerViewHandler(deps DeriveCustomerViewDeps) *DeriveCustomerViewHandler {
	return &DeriveCustomerViewHandler{deps: deps}
}

// Handle 把一个投影新版本推进到客户视图版本：受理（账户与投影身份缺一即未受理，不读
// 任何依赖）→ 幂等按投影版本（同一投影版本不重发视图，只重发同一份意图）→ 披露策略
// 答复译成四维（未配置即四维全部待确认，如实说等，不虚构可见性）→ 首版发布或替代指回
// 前版 → 提交与发布意图。视图只基于当前投影形成，本编排不读内部案件或原始来源消息——
// 依赖清单里根本没有那些端口。
func (handler *DeriveCustomerViewHandler) Handle(
	ctx context.Context,
	command DeriveCustomerViewCommand,
) (DeriveCustomerViewResult, error) {
	// 先判身份再读依赖：租户、账户或投影立不起来时用例语义是未受理，而一次已经发出
	// 的查询收不回来。
	if command.TenantID.String() == "" ||
		command.Customer.String() == "" ||
		command.Projection.Version().String() == "" ||
		command.Projection.Parcel().String() == "" {
		return DeriveCustomerViewResult{outcome: CustomerViewNotAccepted}, nil
	}

	current, found, err := handler.deps.Views.FindCurrent(ctx, command.TenantID, command.Customer, command.Projection.Parcel())
	if err != nil {
		return DeriveCustomerViewResult{outcome: CustomerViewUndecided, reason: CustomerViewStoreUnavailable}, nil
	}
	if found && current.BasedOn() == command.Projection.Version() {
		// 同一投影版本重放：不签新身份、不重问策略、不形成第二个视图版本，但把同一份
		// 意图再交一次——本上下文不记意图完没完成，只答已有结果就收工，一份首次发布
		// 失败的视图会永远停在「已发布、下游不知道」（ADR-0043，重放重发同一份）。
		return DeriveCustomerViewResult{
			outcome:    CustomerViewExistingResult,
			view:       current,
			hasView:    true,
			handoffRef: handler.handOff(ctx, command.TenantID, current),
		}, nil
	}

	answer, configured, err := handler.deps.Policy.AssessDisclosure(ctx, command.Customer, command.Projection)
	if err != nil {
		// 策略调不通是未决，不是「四维都等」：前者要重试依赖，后者是规则未配置的如实
		// 空白，混起来会让一次故障被读成租户还没登记披露规则。
		return DeriveCustomerViewResult{outcome: CustomerViewUndecided, reason: DisclosurePolicyUnavailable}, nil
	}
	dimensions, err := dimensionsFrom(answer, configured)
	if err != nil {
		return DeriveCustomerViewResult{}, err
	}

	version, err := handler.deps.Identities.NextCustomerViewVersionID(ctx)
	if err != nil {
		return DeriveCustomerViewResult{outcome: CustomerViewUndecided, reason: CustomerViewIdentityUnavailable}, nil
	}

	var view domain.CustomerTrackingView
	if found {
		// 投影换了版本：形成新的客户视图版本并指回前版，原发布历史保留——不再有效的
		// 内容不得继续显示为当前事实（CONTEXT）。
		view, err = current.Supersede(version, command.Projection.Version(), dimensions, handler.deps.Clock.Now())
	} else {
		view, err = domain.PublishCustomerView(
			version,
			command.Customer,
			command.Projection.Parcel(),
			command.Projection.Version(),
			dimensions,
			handler.deps.Clock.Now(),
		)
	}
	if err != nil {
		return DeriveCustomerViewResult{}, fmt.Errorf("publish customer tracking view: %w", err)
	}

	if err := handler.deps.Views.Save(ctx, command.TenantID, view); err != nil {
		// 视图没落库就不算发布：交回一个查不回来的视图，门户会按一份不存在的当前版本
		// 展示。这一支不发意图。
		return DeriveCustomerViewResult{outcome: CustomerViewUndecided, reason: CustomerViewStoreUnavailable}, nil
	}

	return DeriveCustomerViewResult{
		outcome:    CustomerViewPublished,
		view:       view,
		hasView:    true,
		handoffRef: handler.handOff(ctx, command.TenantID, view),
	}, nil
}

// handOff 把已发布的视图交给适用下游，交不出去时交回发布续办引用。失败不改写视图，
// 也不算进未决——视图已经发布，要续办的是发布（ADR-0043）。
func (handler *DeriveCustomerViewHandler) handOff(
	ctx context.Context,
	tenant domain.TenantID,
	view domain.CustomerTrackingView,
) string {
	if err := handler.deps.Downstream.HandOffCustomerView(ctx, ports.CustomerViewHandoffIntent{
		TenantID: tenant,
		View:     view,
	}); err != nil {
		return "CONT-" + shortDigest("CUSTOMER_VIEW_HANDOFF", view.Version().String())
	}
	return ""
}

// dimensionsFrom 把策略答复译成四个展示维。未配置时四维全部待确认——真实披露范围属
// 待登记实例参数，如实说等；不虚构可见性，也不把没人作过的披露决定说成「不展示」。
// 已配置时逐维经领域构造器翻译，展示无内容在构造期立不起来，上抛而不吞。
func dimensionsFrom(answer ports.DisclosureAnswer, configured bool) (domain.CustomerViewDimensions, error) {
	if !configured {
		return domain.CustomerViewDimensions{
			Milestones: domain.PendDimension(),
			ETA:        domain.PendDimension(),
			Final:      domain.PendDimension(),
			Note:       domain.PendDimension(),
		}, nil
	}

	milestones, err := dimensionFrom(answer.Milestones)
	if err != nil {
		return domain.CustomerViewDimensions{}, fmt.Errorf("milestones dimension: %w", err)
	}
	eta, err := dimensionFrom(answer.ETA)
	if err != nil {
		return domain.CustomerViewDimensions{}, fmt.Errorf("eta dimension: %w", err)
	}
	final, err := dimensionFrom(answer.Final)
	if err != nil {
		return domain.CustomerViewDimensions{}, fmt.Errorf("final dimension: %w", err)
	}
	note, err := dimensionFrom(answer.Note)
	if err != nil {
		return domain.CustomerViewDimensions{}, fmt.Errorf("note dimension: %w", err)
	}
	return domain.CustomerViewDimensions{Milestones: milestones, ETA: eta, Final: final, Note: note}, nil
}

// dimensionFrom 逐取值分派，不留兜底：策略端口日后新增一种维度答复时这里报错，而不是
// 静默归入某一格。
func dimensionFrom(disclosure ports.DimensionDisclosure) (domain.ViewDimension, error) {
	switch disclosure.State {
	case domain.DimensionShown:
		return domain.ShowDimension(disclosure.Content)
	case domain.DimensionPendingConfirmation:
		return domain.PendDimension(), nil
	case domain.DimensionNotDisclosed:
		return domain.WithholdDimension(), nil
	default:
		return domain.ViewDimension{}, fmt.Errorf("%w: %d", ErrUnexpectedDisclosureState, disclosure.State)
	}
}
