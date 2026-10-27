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
	ETAJudgementUnavailable
)

func (reason CustomerViewUndecidedReason) String() string {
	switch reason {
	case DisclosurePolicyUnavailable:
		return "DISCLOSURE_POLICY_UNAVAILABLE"
	case CustomerViewStoreUnavailable:
		return "CUSTOMER_VIEW_STORE_UNAVAILABLE"
	case CustomerViewIdentityUnavailable:
		return "CUSTOMER_VIEW_IDENTITY_UNAVAILABLE"
	case ETAJudgementUnavailable:
		return "ETA_JUDGEMENT_UNAVAILABLE"
	default:
		return ""
	}
}

// DeriveCustomerViewCommand 携带一份新派生的投影与它所属的租户、货主客户账户。账户由
// 调用方给出而不是从投影推导——投影面向包裹，不认识账户；而「客户视图只包含当前货主
// 客户账户及其授权对象范围」要求账户隔离从入口就是字段。租户同理（ADR-0003）。
//
// ETAJudgementChanged 为真说明这次重派生由 ETA 客户可见性判断换版触发，而不是由一个新投影版本触发：
// 投影版本没变，「同一投影版本重放」的短路因此不能收下它，否则判断换了版、客户视图的 ETA 一维永远
// 不跟。其余入口留假。
type DeriveCustomerViewCommand struct {
	TenantID            domain.TenantID
	Customer            domain.CustomerAccountReference
	Projection          domain.TrackingProjection
	ETAJudgementChanged bool
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
	Policy        ports.DisclosurePolicyView
	ETAJudgements ports.ETAVisibilityJudgementRead
	Views         ports.CustomerViewStore
	Identities    ports.CustomerViewIdentityFactory
	Downstream    ports.CustomerViewHandoff
	Clock         ports.Clock
}

type DeriveCustomerViewHandler struct {
	deps DeriveCustomerViewDeps
}

func NewDeriveCustomerViewHandler(deps DeriveCustomerViewDeps) *DeriveCustomerViewHandler {
	return &DeriveCustomerViewHandler{deps: deps}
}

// Handle 把一个投影新版本推进到客户视图：受理（账户与投影身份缺一即未受理，不读任何
// 依赖）→ 幂等按投影版本（同一投影版本不重发视图，只重发同一份意图）→ 披露策略答复译
// 成四维（未配置即四维全部待确认，如实说等，不虚构可见性；终局一维另看投影里有没有
// 终局，ETA 一维另读这个账户这件包裹的当前 ETA 客户可见性判断）→ 四维与当前版相同则不换代、只前移确认依据（ConfirmedOn），否则首版发布或替代
// 指回前版 → 提交与发布意图。视图只基于当前投影形成，本编排不读内部案件或原始来源
// 消息——依赖清单里根本没有那些端口。
//
// 换不换代只看四维：客户全程追踪视图的版本由披露更正与替代关系驱动，不与内部投影版本
// 一一对应（CONTEXT）。投影每换一版就换一代视图，客户会收到一串内容相同、只是编号不同的
// 「更正」，下游据发布意图作的通知判断也跟着空转。
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
	if found && current.ConfirmedOn() == command.Projection.Version() && !command.ETAJudgementChanged {
		// 比的是确认依据（ConfirmedOn），不是形成时依据：当前版已跟上过的那份投影再来一封，
		// 同样是重放。
		//
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
	// ETA 一维的判断只在策略准许展示时才读：策略不准看的维度，判断说什么都越不过它，读不动判断库
	// 也不该把这一维拖成未决。
	var (
		judgement domain.ETAVisibilityJudgement
		judged    bool
	)
	if configured && answer.ETA.State == domain.DimensionShown {
		judgement, judged, err = handler.deps.ETAJudgements.FindCurrent(
			ctx, command.TenantID, command.Customer, command.Projection.Parcel())
		if err != nil {
			// 判断库调不通是未决，不是「还没有判断」：后者如实答待确认，前者要重试依赖。
			return DeriveCustomerViewResult{outcome: CustomerViewUndecided, reason: ETAJudgementUnavailable}, nil
		}
	}
	dimensions, err := dimensionsFrom(answer, configured, command.Projection, judgement, judged)
	if err != nil {
		return DeriveCustomerViewResult{}, err
	}

	if found && current.Dimensions().Equal(dimensions) {
		if current.ConfirmedOn() == command.Projection.Version() {
			// 判断变化触发的重派生，投影版本早已确认过而四维没变：客户读到的仍是同一版，没有
			// 要前移的确认依据，也没有新版本——答已有结果，意图照重放那样再交一次。
			return DeriveCustomerViewResult{
				outcome:    CustomerViewExistingResult,
				view:       current,
				hasView:    true,
				handoffRef: handler.handOff(ctx, command.TenantID, current),
			}, nil
		}
		// 四维没变：不签新身份、不形成新版本，当前版只跟上这份投影。答已有结果——客户
		// 读到的仍是同一版；意图照重放那样再交一次，同一版本认领同一份，不多发。
		reaffirmed, err := current.Reaffirm(command.Projection.Version(), dimensions)
		if err != nil {
			return DeriveCustomerViewResult{}, fmt.Errorf("reaffirm customer tracking view: %w", err)
		}
		if err := handler.deps.Views.Reaffirm(ctx, command.TenantID, reaffirmed, current.ConfirmedOn()); err != nil {
			return DeriveCustomerViewResult{outcome: CustomerViewUndecided, reason: CustomerViewStoreUnavailable}, nil
		}
		return DeriveCustomerViewResult{
			outcome:    CustomerViewExistingResult,
			view:       reaffirmed,
			hasView:    true,
			handoffRef: handler.handOff(ctx, command.TenantID, reaffirmed),
		}, nil
	}

	version, err := handler.deps.Identities.NextCustomerViewVersionID(ctx)
	if err != nil {
		return DeriveCustomerViewResult{outcome: CustomerViewUndecided, reason: CustomerViewIdentityUnavailable}, nil
	}

	var view domain.CustomerTrackingView
	if found {
		// 四维变了：形成新的客户视图版本并指回前版，原发布历史保留——不再有效的内容
		// 不得继续显示为当前事实（CONTEXT）。
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
//
// 策略只说准不准看，说不了背后有没有东西可看，终局、里程碑与 ETA 各再过一道，都只收窄
// 不放宽：终局一维看投影里有没有 PS 已采用的终局（domain.FinalDimensionOver）；里程碑一维
// 看投影里有没有已归类的标准追踪里程碑（domain.MilestonesDimensionOver）；ETA 一维看
// `UC-VE-003` 的 ETA 客户可见性判断（domain.ETADimensionOver，UC-VE-003 `AT-VE-187`、UC-VE-008 `AT-VE-158`）——
// 没有判断或判断待判断答待确认，判断 ETA 不可见答不展示。追踪说明一维照策略。
func dimensionsFrom(
	answer ports.DisclosureAnswer,
	configured bool,
	projection domain.TrackingProjection,
	judgement domain.ETAVisibilityJudgement,
	judged bool,
) (domain.CustomerViewDimensions, error) {
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
	return domain.CustomerViewDimensions{
		Milestones: domain.MilestonesDimensionOver(milestones, projection),
		ETA:        domain.ETADimensionOver(eta, judgement, judged),
		Final:      domain.FinalDimensionOver(final, projection),
		Note:       note,
	}, nil
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
