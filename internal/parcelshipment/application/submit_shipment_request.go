// Package application 在领域内核与 parcel-shipment 自有端口之上编排本上下文的用例。
// 持久化、事务与事件机制在 adapters，不进本包（ADR-0026）。
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// SubmitOutcome 是应用处理结果，不是委托的生命周期状态。只有 OutcomeSubmitted 会
// 建立委托，且这些取值没有一个表示委托已被接受或拒绝。
type SubmitOutcome uint8

const (
	OutcomeInvalid SubmitOutcome = iota
	OutcomeSubmitted
	OutcomeExistingResult
	OutcomeIngressConflict
	OutcomeInputNotAccepted
	OutcomeOtherProductionAuthority
	OutcomeOwnershipUnresolved
	// OutcomeAdmissionPaused 与 OutcomeOwnershipUnresolved 分开，因为暂停回答的是
	// 本产品当前是否接纳新准入，而不是这个范围归谁。合并会丢掉「权威其实已经确定」
	// 这个事实。
	OutcomeAdmissionPaused
	// OutcomePriorRequestNotFound 统一回答关联指名的三种不可见：查无此委托、编号不符、
	// 跨租户或跨客户指名。可区分即可枚举别人的委托（ADR-0029 的合并理由，`AT-PS-075`
	// 同款纪律在关联指名上一字不差地成立）。
	OutcomePriorRequestNotFound
	// OutcomeLinkIneligible 说原委托找到了、也确属这个客户，只是它的状态不承认这个
	// 关联方向（关联方向锚在原委托的终态上，`已提交`的原委托没有关联方向）。续办动作：
	// 普通资料纠错走同一委托的新提交版本；重组先撤回原委托，再以撤回重提方向关联
	// （ADR-0255）。与「查无」分格：两者的续办动作不同。
	OutcomeLinkIneligible
)

func (outcome SubmitOutcome) String() string {
	switch outcome {
	case OutcomeSubmitted:
		return "SUBMITTED"
	case OutcomeExistingResult:
		return "EXISTING_RESULT"
	case OutcomeIngressConflict:
		return "INGRESS_CONFLICT"
	case OutcomeInputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case OutcomeOtherProductionAuthority:
		return "OTHER_PRODUCTION_AUTHORITY"
	case OutcomeOwnershipUnresolved:
		return "OWNERSHIP_UNRESOLVED"
	case OutcomeAdmissionPaused:
		return "ADMISSION_PAUSED"
	case OutcomePriorRequestNotFound:
		return "PRIOR_REQUEST_NOT_FOUND"
	case OutcomeLinkIneligible:
		return "LINK_INELIGIBLE"
	default:
		return ""
	}
}

// PriorRequestClaim 指名一份要与之建立关联的原委托（`AT-PS-036`②③、`AT-PS-076`）：
// 原委托的来源身份加委托编号双重指名——与撤回同一模式，编号单独出现无从校验归属——
// 再加关联方向。零值即首次委托，不指名任何出处。
type PriorRequestClaim struct {
	PriorIdentity  domain.SourceIdentity
	PriorRequestID domain.ShipmentRequestID
	Kind           domain.RequestLinkKind
}

func (claim PriorRequestClaim) requested() bool {
	return claim != (PriorRequestClaim{})
}

// complete 校验指名的三件套都在场。用导出访问器判空而不是领域内部的 valid()：本包在
// 领域边界之外，判据只能是「客户根本没把话说全」这一层。
func (claim PriorRequestClaim) complete() bool {
	return claim.PriorIdentity.TenantID().String() != "" &&
		claim.PriorIdentity.CustomerAccountID().String() != "" &&
		claim.PriorIdentity.Source().String() != "" &&
		claim.PriorIdentity.RequestKey().String() != "" &&
		claim.PriorRequestID.String() != "" &&
		claim.Kind.String() != ""
}

type SubmitShipmentRequestCommand struct {
	Identity          domain.SourceIdentity
	PayloadDigest     domain.PayloadDigest
	OccurredAt        time.Time
	ReceivedAt        time.Time
	BatchID           domain.SubmissionBatchID
	ShipmentRequestID domain.ShipmentRequestID
	DeclaredParcelIDs []domain.DeclaredParcelID
	AdmissionScope    domain.AdmissionScope
	ExpectedRevision  domain.ProductionOwnershipRevision
	// Link 缺席即首次委托。指名出处的提交与首次提交共用全部管线（来源保全、归属、
	// 门禁、判重）：关联新委托是一份完整的新委托，不是原委托的续篇。
	Link PriorRequestClaim
	// DeclaredProfiles 随首个提交版本申报的成员声明画像（ADR-0048），允许缺席或部分覆盖。
	DeclaredProfiles []domain.DeclaredParcelProfile
	// DeclaredElements 随首个提交版本申报的寄 / 收两段地址要素（pp-seams/05 裁决 3），与画像同口径允许缺席或只报一段。
	// 它与 PayloadDigest 由接单入口对同一份规范化输入的同一次 CanonicalizeSubmission 产出，本编排不重算、不核对。
	DeclaredElements domain.DeclaredAddressElements
	// RequestedServiceProduct 随首个提交版本声明的服务产品（票 psb/17），出处与 DeclaredElements 同一次规范化，允许缺席。
	RequestedServiceProduct domain.DeclaredServiceProduct
	// CustomerParcelNumbers 是客户为声明包裹给出的包裹件号（ADR-0184 决定四），由接单入口与现签的内部标识配好带入，
	// 允许缺席。委托接受时据它形成外部标识关联。
	CustomerParcelNumbers []domain.CustomerParcelNumberDeclaration
}

// SubmitShipmentRequestResult 携带调用方可以据以行动的内容。委托与归属决定各自可选、
// 通过访问器报告：重放一份从未建过单的输入时没有委托可指名，而接入冲突根本不必问准入
// 权威。
type SubmitShipmentRequestResult struct {
	outcome SubmitOutcome
	// priorOutcome 只随`已有结果`在场：原处理答出的那一格（UC-PS-001「已有结果」一行「原接入处理结果」）。拒绝那几格没有
	// 归属决定可带，少了它，重发的客户只知道「办过了」，不知道上一次答的是什么。
	priorOutcome      SubmitOutcome
	shipmentRequestID domain.ShipmentRequestID
	hasRequest        bool
	// submissionVersionID 是所指名委托此刻的当前提交版本（票 first-party-api/06 取舍 E）：客户下一次补充要以它为基准
	// （FormNewSubmissionVersionCommand.BasisVersion），回执带回，客户不必另读。随 hasRequest 同在场。
	submissionVersionID domain.SubmissionVersionID
	parcels             []SubmittedParcel
	ownershipDecision   domain.ProductionOwnershipDecision
	hasDecision         bool
	gateBlockReasons    []domain.FutureSubmissionBlockReason
}

// SubmittedParcel 是结果所指名委托上的一件声明包裹：它的内部标识，配上客户为它声明的包裹件号（ADR-0184 决定四；没声明
// 就缺席）。内部标识是声明包裹的永久标识，委托接受后就是正式包裹的标识、不换号（PS CONTEXT「声明包裹在委托接受时进入
// 固定成员基线，并保持永久内部标识」）；真实拆分或合并除外——新物理服务件另用新的内部标识，客户手里这个号留作历史身份
// （GLOSSARY「包裹内部标识」），后继怎么展示归 VE 的身份谱系。客户系统凭它查客户追踪视图（ADR-0223）。
type SubmittedParcel struct {
	parcel         domain.DeclaredParcelID
	customerNumber domain.ExternalIdentifierNumber
	hasNumber      bool
}

func (parcel SubmittedParcel) Parcel() domain.DeclaredParcelID {
	return parcel.parcel
}

func (parcel SubmittedParcel) CustomerParcelNumber() (domain.ExternalIdentifierNumber, bool) {
	return parcel.customerNumber, parcel.hasNumber
}

func (result SubmitShipmentRequestResult) Outcome() SubmitOutcome {
	return result.outcome
}

func (result SubmitShipmentRequestResult) ShipmentRequestID() (domain.ShipmentRequestID, bool) {
	return result.shipmentRequestID, result.hasRequest
}

// SubmissionVersionID 交回所指名委托此刻的当前提交版本标识，与 ShipmentRequestID 同在同缺。首次提交是首个版本；重放在补充
// 之后读回，是补充形成的当前版本——补充基准要的正是当前版本，不是首个版本。
func (result SubmitShipmentRequestResult) SubmissionVersionID() (domain.SubmissionVersionID, bool) {
	return result.submissionVersionID, result.hasRequest
}

// Parcels 交回所指名委托的逐件包裹（拷贝）；没有指名委托时为空。
func (result SubmitShipmentRequestResult) Parcels() []SubmittedParcel {
	return append([]SubmittedParcel(nil), result.parcels...)
}

// naming 让结果指名一份委托，连同它的逐件包裹。建成、重放、并发建单撞已存在都经这里，件与号一律从委托读、不从命令读：
// 接单入口每次都现签一组新标识，委托已建时重放那一组走不到建单（`CustomerSubmissionIntake.mintInternalIdentities` 的注释），
// 从命令读就会把一组从没落库的号交给客户（ADR-0223 决定二）。
func (result SubmitShipmentRequestResult) naming(request domain.ShipmentRequest) SubmitShipmentRequestResult {
	result.shipmentRequestID = request.ShipmentRequestID()
	result.hasRequest = true
	result.submissionVersionID = request.CurrentSubmissionVersion().VersionID()
	result.parcels = submittedParcelsOf(request)
	return result
}

// submittedParcelsOf 按首个提交版本的成员次序排：接单入口按草案包裹的次序现签内部标识，首个版本照这个次序立成员；受控
// 补充的新版本只要求成员集合等值、不管次序（domain 的 supersessionMembers），取当前版本，同一份委托的重放会在补充前后
// 交回两种次序。
func submittedParcelsOf(request domain.ShipmentRequest) []SubmittedParcel {
	members := request.CurrentSubmissionVersion().DeclaredParcelIDs()
	if prior := request.PriorSubmissionVersions(); len(prior) > 0 {
		members = prior[0].DeclaredParcelIDs()
	}
	numbers := make(map[domain.DeclaredParcelID]domain.ExternalIdentifierNumber)
	for _, declaration := range request.CustomerParcelNumbers() {
		numbers[declaration.Parcel()] = declaration.Number()
	}
	parcels := make([]SubmittedParcel, 0, len(members))
	for _, member := range members {
		number, declared := numbers[member]
		parcels = append(parcels, SubmittedParcel{parcel: member, customerNumber: number, hasNumber: declared})
	}
	return parcels
}

// PriorOutcome 交回`已有结果`所指的原处理结果；别的结果没有这一格。
func (result SubmitShipmentRequestResult) PriorOutcome() (SubmitOutcome, bool) {
	return result.priorOutcome, result.priorOutcome != OutcomeInvalid
}

func (result SubmitShipmentRequestResult) OwnershipDecision() (domain.ProductionOwnershipDecision, bool) {
	return result.ownershipDecision, result.hasDecision
}

func (result SubmitShipmentRequestResult) GateBlockReasons() []domain.FutureSubmissionBlockReason {
	return append([]domain.FutureSubmissionBlockReason(nil), result.gateBlockReasons...)
}

type SubmitShipmentRequestHandler struct {
	sources    ports.SourceSubmissionRepository
	requests   ports.ShipmentRequestRepository
	ownership  ports.ProductionOwnershipAuthority
	answers    ports.SubmissionAnswers
	guard      ports.SubmissionContinuationGuard
	handoff    ports.OtherProductionAuthorityChannel
	identities ports.SubmissionIdentityFactory
	clock      ports.Clock
}

func NewSubmitShipmentRequestHandler(
	sources ports.SourceSubmissionRepository,
	requests ports.ShipmentRequestRepository,
	ownership ports.ProductionOwnershipAuthority,
	answers ports.SubmissionAnswers,
	guard ports.SubmissionContinuationGuard,
	handoff ports.OtherProductionAuthorityChannel,
	identities ports.SubmissionIdentityFactory,
	clock ports.Clock,
) *SubmitShipmentRequestHandler {
	return &SubmitShipmentRequestHandler{
		sources:    sources,
		requests:   requests,
		ownership:  ownership,
		answers:    answers,
		guard:      guard,
		handoff:    handoff,
		identities: identities,
		clock:      clock,
	}
}

// Handle 先保全来源，再为完整拟受理范围取得生产归属——判为`其他权威`时把范围交给那个权威并
// 评估交接——之后才建立`已提交`委托。它不形成接受、拒绝、可达性或财务控制结果。
func (handler *SubmitShipmentRequestHandler) Handle(
	ctx context.Context,
	command SubmitShipmentRequestCommand,
) (SubmitShipmentRequestResult, error) {
	incoming, err := domain.NewSourceSubmissionFingerprint(
		command.Identity,
		command.PayloadDigest,
		command.OccurredAt,
		command.ReceivedAt,
	)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("preserve source: %w", err)
	}

	existing, found, err := handler.sources.FindPreserved(ctx, command.Identity)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("find preserved source: %w", err)
	}
	if found {
		classification, err := domain.ClassifySourceSubmission(existing, incoming)
		if err != nil {
			return SubmitShipmentRequestResult{}, fmt.Errorf("classify source submission: %w", err)
		}
		if classification == domain.SourceConflict {
			return SubmitShipmentRequestResult{outcome: OutcomeIngressConflict}, nil
		}
		if err := handler.sources.AppendObservation(ctx, incoming); err != nil {
			return SubmitShipmentRequestResult{}, fmt.Errorf("append source observation: %w", err)
		}
		return handler.resolveExclusively(ctx, command, existing)
	}

	if err := handler.sources.Preserve(ctx, incoming); err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("preserve source: %w", err)
	}
	return handler.resolveExclusively(ctx, command, incoming)
}

// resolveExclusively 按这份提交上一次实际走到的位置作答（ADR-0266），要动手（判归属、重投交接、建单）的那几格在续办守卫里做。
//
// 先在守卫外看一眼：委托已在、或上一次的答案已经定了，只读即可作答，不必排队——重发一份已办完的提交不进续办守卫那一笔事务
// （PBC-07「重放路径不重跑事务」；这次到达的观察照旧在守卫外追加）。要动手时进守卫再看一次：守卫让同一来源身份一次只走一路，进门之后读到的答案与委托，在本路落笔之前
// 不会再被别路改动，「记答案前复查」在进门这一刻做一次就够。首次到达也走这一路：保全之后、判归属之前，同一请求的一次重发可能
// 已经抢先续办。
//
// preserved 是保全在册的那份来源事实：委托的当前提交版本据它立，重发时就是首次到达那一份，不是这一次的观察（观察只追加在
// 旁边，UC-PS-001「`occurredAt`/`receivedAt` 变化只追加观察」）。
func (handler *SubmitShipmentRequestHandler) resolveExclusively(
	ctx context.Context,
	command SubmitShipmentRequestCommand,
	preserved domain.SourceSubmissionFingerprint,
) (SubmitShipmentRequestResult, error) {
	standing, err := handler.standing(ctx, command.Identity)
	if err != nil || standing.settled {
		return standing.result, err
	}
	var result SubmitShipmentRequestResult
	err = handler.guard.Exclusively(ctx, command.Identity, func(ctx context.Context) error {
		again, err := handler.standing(ctx, command.Identity)
		if err != nil {
			return err
		}
		if again.settled {
			result = again.result
			return nil
		}
		if again.answered && !again.answer.Allowed() && again.answer.Decision.Authority() == domain.ProductionAuthorityOther {
			result, err = handler.redeliverHandoff(ctx, command.Identity, again.answer)
			return err
		}
		result, err = handler.proceed(ctx, command, preserved)
		return err
	})
	if err != nil {
		return SubmitShipmentRequestResult{}, err
	}
	return result, nil
}

// submissionStanding 是一份来源身份此刻的站位：settled 为真时 result 就是答复；否则 answer 是最近一次答案（answered 为假即什么
// 都没留下），由守卫里那一段决定怎么接着办。
type submissionStanding struct {
	result   SubmitShipmentRequestResult
	settled  bool
	answer   ports.SubmissionAnswer
	answered bool
}

// standing 判这份提交站在哪一格，重放格局见 ADR-0266 决定一，这里一格一个出口：
//
//   - 委托已在 → `已有结果`，指名原委托并带建它所据的那次归属答案；
//   - 上一次答出拒绝（输入未受理、原委托不可见、关联不适用）或已形成挡下的结果（准入暂停、交接已确认的他方归属）→ `已有结果`
//     带原结果，不再判（哪几格挡下算已形成由 settledOnReplay 定）；
//   - 其余没定：上一次是他方而交接未确认（重投同一次尝试）、上一次答`生产归属未决`、放行了却没建成，或什么都没留下。
func (handler *SubmitShipmentRequestHandler) standing(
	ctx context.Context,
	identity domain.SourceIdentity,
) (submissionStanding, error) {
	request, found, err := handler.requests.FindBySourceIdentity(ctx, identity)
	if err != nil {
		return submissionStanding{}, fmt.Errorf("find existing shipment request: %w", err)
	}
	if found {
		result, err := handler.existingRequestResult(ctx, identity, request)
		return submissionStanding{result: result, settled: err == nil}, err
	}

	answer, answered, err := handler.answers.FindLatestSubmissionAnswer(ctx, identity)
	if err != nil {
		return submissionStanding{}, fmt.Errorf("find latest submission answer: %w", err)
	}
	switch {
	case answered && answer.Refused():
		prior := refusalOutcome(answer.Refusal)
		if prior == OutcomeInvalid {
			// 读回一格词表之外的拒绝是坏数据。答一个不带原结果的已有结果，客户只会以为办过了；上抛让它没有形成答案。
			return submissionStanding{}, fmt.Errorf("%w: refusal %d", ErrUntranslatableRefusal, answer.Refusal)
		}
		return submissionStanding{
			result:  SubmitShipmentRequestResult{outcome: OutcomeExistingResult, priorOutcome: prior},
			settled: true,
		}, nil
	case answered && settledOnReplay(answer):
		result := blockedResult(answer)
		result.priorOutcome = result.outcome
		result.outcome = OutcomeExistingResult
		return submissionStanding{result: result, settled: true}, nil
	default:
		return submissionStanding{answer: answer, answered: answered}, nil
	}
}

// proceed 是来源保全之后、首次作答的那一段：立候选、互证关联指名、判归属、过门禁、建单。首次提交与续办共用它——两者从这里起
// 是同一件事，分开写就会有一边先改、另一边留旧。每一格答出来的答案都先记下再交回：重发要凭它答原结果，没记下就答，重发会把
// 一次暂停重判成放行、把一次关联不适用重判成建单。它只在续办守卫里跑。
func (handler *SubmitShipmentRequestHandler) proceed(
	ctx context.Context,
	command SubmitShipmentRequestCommand,
	preserved domain.SourceSubmissionFingerprint,
) (SubmitShipmentRequestResult, error) {
	// 用例步骤 3A 先于 3B：无法建立最小委托身份的输入，根本没有可拿去问准入权威的
	// 拟受理范围。
	candidate, err := domain.NewSubmissionCandidate(
		preserved,
		command.BatchID,
		command.ShipmentRequestID,
		command.DeclaredParcelIDs,
	)
	if err != nil {
		return handler.refuse(ctx, command.Identity, ports.SubmissionRefusedInputNotAccepted)
	}

	link, refusal, err := handler.resolvePriorClaim(ctx, command)
	if err != nil {
		return SubmitShipmentRequestResult{}, err
	}
	if refusal != ports.SubmissionRefusalInvalid {
		return handler.refuse(ctx, command.Identity, refusal)
	}

	decision, err := handler.ownership.DecideProductionOwnership(ctx, command.AdmissionScope)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("decide production ownership: %w", err)
	}

	// 交接评估、门禁评估与建单共用一次时钟读数，这样委托的提交时刻绝不会落在其门禁所评估的
	// 时刻之外，交接评估的时刻也不会晚于据它形成的门禁。
	decidedAt := handler.clock.Now()
	if decision.Authority() == domain.ProductionAuthorityOther {
		decision, err = handler.handOverToOtherAuthority(ctx, decision, decidedAt)
		if err != nil {
			return SubmitShipmentRequestResult{}, err
		}
	}

	gate, err := domain.EvaluateFutureSubmissionGate(
		decision,
		command.AdmissionScope.Digest(),
		command.ExpectedRevision,
		decidedAt,
	)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("evaluate future submission gate: %w", err)
	}
	if !gate.IsAllowed() {
		blocked := ports.SubmissionAnswer{Decision: decision, BlockReasons: gate.BlockReasons()}
		if err := handler.answers.RecordSubmissionAnswer(ctx, command.Identity, blocked); err != nil {
			return SubmitShipmentRequestResult{}, fmt.Errorf("record submission answer: %w", err)
		}
		return blockedResult(blocked), nil
	}

	versionID, err := handler.identities.NextSubmissionVersionID(ctx)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("next submission version ID: %w", err)
	}
	taskID, err := handler.identities.NextAcceptanceDecisionTaskID(ctx)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("next acceptance decision task ID: %w", err)
	}

	request, err := domain.SubmitShipmentRequest(domain.SubmitShipmentRequestSpec{
		Candidate:             candidate,
		Gate:                  gate,
		VersionID:             versionID,
		TaskID:                taskID,
		SubmittedAt:           decidedAt,
		Link:                  link,
		Profiles:              command.DeclaredProfiles,
		Elements:              command.DeclaredElements,
		RequestedProduct:      command.RequestedServiceProduct,
		CustomerParcelNumbers: command.CustomerParcelNumbers,
	})
	if errors.Is(err, domain.ErrInvalidDeclaredMeasurement) || errors.Is(err, domain.ErrInvalidCustomerParcelNumber) {
		// 画像或客户包裹件号不贴合成员集合（指着不存在的成员、一员两张或两个号、半截测量）与候选立不起来同格：
		// 客户输入的问题，答`输入未受理`让他改请求，不是本方的故障。
		return handler.refuse(ctx, command.Identity, ports.SubmissionRefusedInputNotAccepted)
	}
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("submit shipment request: %w", err)
	}
	// 放行的答案先于建单记下，带着将要建的委托号：建成之后，重发按委托号找回建单所据的就是这一次。生产装配里两者在守卫那一笔
	// 事务里同生共死；守卫不开事务的装配下建单失败会留下「放行了、委托却不在」，重发照「接着办」那一格处理，同样不丢单。
	allowed := ports.SubmissionAnswer{Decision: decision, ShipmentRequestID: request.ShipmentRequestID()}
	if err := handler.answers.RecordSubmissionAnswer(ctx, command.Identity, allowed); err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("record submission answer: %w", err)
	}
	inserted, err := handler.requests.Insert(ctx, command.Identity, request)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("insert shipment request: %w", err)
	}
	switch inserted {
	case ports.ShipmentRequestInserted:
		return SubmitShipmentRequestResult{
			outcome:           OutcomeSubmitted,
			ownershipDecision: decision,
			hasDecision:       true,
		}.naming(request), nil
	case ports.ShipmentRequestAlreadyExists:
		// 同一来源身份的建单在守卫里一次一路，走到这一格多半是委托号撞了别的来源键，少数是守卫之外的写入。回去按重放规则
		// 重答；这次到达已经留下过痕迹（首次是保全，续办是那一次观察），不再追加观察。
		return handler.resolveAfterInsertConflict(ctx, command.Identity, preserved)
	default:
		return SubmitShipmentRequestResult{}, ErrUnexpectedInsertOutcome
	}
}

// ErrUnexpectedInsertOutcome 说明建单仓储交回了封闭集合以外的答复。上抛而不译成业务结果：
// 集合外的取值没有恢复动作可派。
var ErrUnexpectedInsertOutcome = errors.New("parcel shipment: unexpected shipment request insert outcome")

// settledOnReplay 判一次挡下的归属答案在重发时是不是已形成的结果。只有两格是：`非本产品生产归属`（范围已安全交给他方，
// ADR-0128 决定三）与`准入暂停`（首发试点叠加条件：原因解除也不自动恢复，恢复要一次明确的恢复决定，而不是客户再发一次）。
// `生产归属未决`不是——UC-PS-001 未决结束写的就是「原始提交和处理尝试可安全续办」，重发即续办（ADR-0266 决定一）。放行过的
// 答案也不是：委托不在，说明建单那一笔没落成。
func settledOnReplay(answer ports.SubmissionAnswer) bool {
	if answer.Refused() || answer.Allowed() {
		return false
	}
	switch blockedOutcome(answer.Decision) {
	case OutcomeOtherProductionAuthority, OutcomeAdmissionPaused:
		return true
	default:
		return false
	}
}

// blockedResult 把一次挡下的归属答案译成结果：首次答它，重发答已形成的那几格时也照它带原归属结果与门禁原因。
func blockedResult(answer ports.SubmissionAnswer) SubmitShipmentRequestResult {
	return SubmitShipmentRequestResult{
		outcome:           blockedOutcome(answer.Decision),
		ownershipDecision: answer.Decision,
		hasDecision:       true,
		gateBlockReasons:  append([]domain.FutureSubmissionBlockReason(nil), answer.BlockReasons...),
	}
}

// refuse 记下一格拒绝再答它。
func (handler *SubmitShipmentRequestHandler) refuse(
	ctx context.Context,
	identity domain.SourceIdentity,
	refusal ports.SubmissionRefusal,
) (SubmitShipmentRequestResult, error) {
	if err := handler.answers.RecordSubmissionAnswer(ctx, identity, ports.SubmissionAnswer{Refusal: refusal}); err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("record submission answer: %w", err)
	}
	return SubmitShipmentRequestResult{outcome: refusalOutcome(refusal)}, nil
}

// ErrUntranslatableRefusal 说明留存的答案里有一格词表之外的拒绝。上抛而不译成业务结果：集合外的取值没有原结果可答。
var ErrUntranslatableRefusal = errors.New("parcel shipment: recorded refusal is outside the vocabulary")

// refusalOutcome 是留存的拒绝与它答出的那一格之间唯一的对照；拒绝之外的取值没有结果格，答 OutcomeInvalid，调用方据此上抛
// ErrUntranslatableRefusal。
func refusalOutcome(refusal ports.SubmissionRefusal) SubmitOutcome {
	switch refusal {
	case ports.SubmissionRefusedInputNotAccepted:
		return OutcomeInputNotAccepted
	case ports.SubmissionRefusedPriorRequestNotFound:
		return OutcomePriorRequestNotFound
	case ports.SubmissionRefusedLinkIneligible:
		return OutcomeLinkIneligible
	default:
		return OutcomeInvalid
	}
}

// existingRequestResult 回答委托已在的那一格：指名它，并带建它所据的那次归属答案（UC-PS-001「已有结果」一行）。按委托号找
// 而不取最近一次：并发续办时落败那一方也记了自己的答案。找不到就不带——那是答案记录落地之前建成的委托，没有可答的原归属
// 结果，不拿别的一次顶替。
func (handler *SubmitShipmentRequestHandler) existingRequestResult(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (SubmitShipmentRequestResult, error) {
	result := SubmitShipmentRequestResult{outcome: OutcomeExistingResult, priorOutcome: OutcomeSubmitted}.naming(request)
	answer, answered, err := handler.answers.FindSubmissionAnswerForRequest(ctx, identity, request.ShipmentRequestID())
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("find submission answer for request: %w", err)
	}
	if answered {
		result.ownershipDecision = answer.Decision
		result.hasDecision = true
	}
	return result, nil
}

// resolveAfterInsertConflict 回答 Insert 交回「已存在」之后的那一格。与重放同一套分类规则，
// 但不再追加观察：走到这里时这次到达已经保全过或追加过观察。
//
// 「已存在」却按来源身份读不回委托，撞的是委托号唯一键（另一个来源键占了同一个号）：这份提交并没有建成，上抛让调用方
// 原样重发、接单入口另签一组号接着办，不答一个不指名委托的已有结果——那正是这份提交从此不再建单的样子。
func (handler *SubmitShipmentRequestHandler) resolveAfterInsertConflict(
	ctx context.Context,
	identity domain.SourceIdentity,
	incoming domain.SourceSubmissionFingerprint,
) (SubmitShipmentRequestResult, error) {
	existing, found, err := handler.sources.FindPreserved(ctx, identity)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("find preserved source after insert conflict: %w", err)
	}
	if !found {
		return SubmitShipmentRequestResult{}, fmt.Errorf("insert reported already exists but preserved source is missing")
	}

	classification, err := domain.ClassifySourceSubmission(existing, incoming)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("classify source submission: %w", err)
	}
	if classification == domain.SourceConflict {
		return SubmitShipmentRequestResult{outcome: OutcomeIngressConflict}, nil
	}

	request, found, err := handler.requests.FindBySourceIdentity(ctx, identity)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("find existing shipment request: %w", err)
	}
	if !found {
		return SubmitShipmentRequestResult{}, fmt.Errorf("insert reported already exists but no shipment request is found")
	}
	return handler.existingRequestResult(ctx, identity, request)
}

// resolvePriorClaim 把关联指名换成已互证的关联出处（`AT-PS-036`②③、`AT-PS-076`）。
//
// 它在归属判定之前跑：指名立不住的关联没有必要消耗一次准入决定。三层裁决各归各格——
//
//  1. 话没说全（三件套缺项、指名自己）→ 输入未受理；
//  2. 统一不可见：跨租户或跨客户指名**不查库直接**答`查无原委托`，与真的查无、编号
//     不符同一个答案——可区分即可枚举别人的委托；
//  3. 方向与原委托终态不符 → 关联不适用，由领域互证裁定。待决委托没有方向可用，
//     普通纠错走同一委托的新提交版本。
//
// 原委托全程只读：出处落在新委托身上，原版本与原决定一概不动。
func (handler *SubmitShipmentRequestHandler) resolvePriorClaim(
	ctx context.Context,
	command SubmitShipmentRequestCommand,
) (domain.PriorRequestLink, ports.SubmissionRefusal, error) {
	claim := command.Link
	if !claim.requested() {
		return domain.PriorRequestLink{}, ports.SubmissionRefusalInvalid, nil
	}
	if !claim.complete() || claim.PriorRequestID == command.ShipmentRequestID {
		return domain.PriorRequestLink{}, ports.SubmissionRefusedInputNotAccepted, nil
	}
	if claim.PriorIdentity.TenantID() != command.Identity.TenantID() ||
		claim.PriorIdentity.CustomerAccountID() != command.Identity.CustomerAccountID() {
		return domain.PriorRequestLink{}, ports.SubmissionRefusedPriorRequestNotFound, nil
	}

	prior, found, err := handler.requests.FindBySourceIdentity(ctx, claim.PriorIdentity)
	if err != nil {
		return domain.PriorRequestLink{}, ports.SubmissionRefusalInvalid, fmt.Errorf("find prior shipment request: %w", err)
	}
	if !found || prior.ShipmentRequestID() != claim.PriorRequestID {
		return domain.PriorRequestLink{}, ports.SubmissionRefusedPriorRequestNotFound, nil
	}

	link, err := domain.EstablishPriorRequestLink(prior, claim.Kind)
	if errors.Is(err, domain.ErrPriorStateIncompatibleWithLink) {
		return domain.PriorRequestLink{}, ports.SubmissionRefusedLinkIneligible, nil
	}
	if err != nil {
		return domain.PriorRequestLink{}, ports.SubmissionRefusalInvalid, fmt.Errorf("establish prior request link: %w", err)
	}
	return link, ports.SubmissionRefusalInvalid, nil
}

// handOverToOtherAuthority 是 UC-PS-001 步骤 3B「其他权威时安全交接」那一步（ADR-0128 决定二）：
// 归属已凭治理接管记录判为`其他权威`之后，把完整拟受理范围投递给那个权威、观察确认、经
// AssessSafeHandoff 形成评估并记到决定上。先后固定——归属先定、交接后验：投递是对外动作、不可撤，
// 只在归属已定时做；反过来拿一次应答当归属证据，会让没有停写证据的一方凭应答成为权威。
//
// 尝试身份与续办引用都由决定派生而不签发：同一份决定走到这里必须得到同一次尝试，对方才能据以认领重放。续办引用不看观察结果
// 就形成——它说的是「从这一次尝试续办」，原因由评估自己带；按观察结果决定给不给，等于在这里复刻一遍领域的判定条件。
func (handler *SubmitShipmentRequestHandler) handOverToOtherAuthority(
	ctx context.Context,
	decision domain.ProductionOwnershipDecision,
	assessedAt time.Time,
) (domain.ProductionOwnershipDecision, error) {
	attemptID, err := domain.NewHandoffAttemptID("PS-HANDOFF/" + decision.DecisionID().String())
	if err != nil {
		return domain.ProductionOwnershipDecision{}, fmt.Errorf("hand over to other authority: attempt ID: %w", err)
	}
	continuationRef, err := domain.NewOwnershipContinuationReference("CONT-PS-HANDOFF/" + attemptID.String())
	if err != nil {
		return domain.ProductionOwnershipDecision{}, fmt.Errorf("hand over to other authority: continuation reference: %w", err)
	}
	assessment, err := handler.deliverAndAssess(ctx, decision, attemptID, continuationRef, assessedAt)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, err
	}
	withHandoff, err := decision.WithSafeHandoff(assessment)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, fmt.Errorf("record safe handoff on ownership decision: %w", err)
	}
	return withHandoff, nil
}

// redeliverHandoff 回答上一次是他方、交接却没确认的那份提交的重发：按库里记着的那次尝试重投、重查确认，不重判归属。
//
// 重判是错的那一边：登记册此间若翻成本产品，重判会放行建单，而上一次的投递已经发出去了，同一范围就一边交给他方、一边在本地
// 建单。ADR-0128 决定二「同一份决定重复走到这里必须得到同一次尝试」，对方据尝试身份认领重放，所以重投不会在对方那里多出一份。确认了，这一次
// 才第一次答成`非本产品生产归属`；仍未确认就仍答`生产归属未决`。重查的评估同样记下，后一次不覆盖前一次。
func (handler *SubmitShipmentRequestHandler) redeliverHandoff(
	ctx context.Context,
	identity domain.SourceIdentity,
	answer ports.SubmissionAnswer,
) (SubmitShipmentRequestResult, error) {
	prior, recorded := answer.Decision.SafeHandoff()
	if !recorded {
		// 他方决定一律先交接再记答案，没带评估的他方答案只能是坏数据；答一格就是替它编一次交接。
		return SubmitShipmentRequestResult{}, fmt.Errorf("redeliver handoff: recorded other-authority answer carries no handoff assessment")
	}
	continuationRef, _ := prior.ContinuationReference()
	assessment, err := handler.deliverAndAssess(ctx, answer.Decision, prior.AttemptID(), continuationRef, handler.clock.Now())
	if err != nil {
		return SubmitShipmentRequestResult{}, err
	}
	reassessed, err := answer.Decision.WithReassessedHandoff(assessment)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("record reassessed handoff on ownership decision: %w", err)
	}
	again := ports.SubmissionAnswer{Decision: reassessed, BlockReasons: answer.BlockReasons}
	if err := handler.answers.RecordSubmissionAnswer(ctx, identity, again); err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("record submission answer: %w", err)
	}
	return blockedResult(again), nil
}

// deliverAndAssess 把决定的完整拟受理范围按给定的那次尝试投递出去，据对方的答复形成交接评估。出向通道自身失败原样上抛，不折成
// 任何一格观察：那些格说的是对方的答复，不是本方的故障。
func (handler *SubmitShipmentRequestHandler) deliverAndAssess(
	ctx context.Context,
	decision domain.ProductionOwnershipDecision,
	attemptID domain.HandoffAttemptID,
	continuationRef domain.OwnershipContinuationReference,
	assessedAt time.Time,
) (domain.SafeHandoffAssessment, error) {
	target, present := decision.OtherAuthorityReference()
	if !present {
		return domain.SafeHandoffAssessment{}, fmt.Errorf("hand over to other authority: decision names no other authority")
	}
	observed, err := handler.handoff.DeliverAdmissionScope(ctx, ports.ProductionHandoffDelivery{
		AttemptID:       attemptID,
		Scope:           decision.Scope(),
		TargetAuthority: target,
	})
	if err != nil {
		return domain.SafeHandoffAssessment{}, fmt.Errorf("deliver admission scope to other authority: %w", err)
	}
	assessment, err := domain.AssessSafeHandoff(domain.SafeHandoffAssessmentSpec{
		AttemptID:            attemptID,
		Scope:                decision.Scope(),
		TargetAuthority:      target,
		Observation:          observed.Observation,
		ConfirmedScopeDigest: observed.ConfirmedScopeDigest,
		ConfirmationRef:      observed.ConfirmationRef,
		QueryRef:             observed.QueryRef,
		ContinuationRef:      continuationRef,
		EffectiveAt:          observed.EffectiveAt,
		AssessedAt:           assessedAt,
	})
	if err != nil {
		return domain.SafeHandoffAssessment{}, fmt.Errorf("assess safe handoff: %w", err)
	}
	return assessment, nil
}

// blockedOutcome 让三种拒绝各自成立。其他权威承接该范围、权威无法确定、以及本产品暂停
// 新准入，是对不同问题的不同回答；其中暂停既不是第四种权威身份，也不是客户业务拒绝。
//
// `其他权威`只在交接评估**已确认**时才是「非本产品归属结束」（ADR-0128 决定三）：接管记录与交接
// 确认是两种证据，缺一不许交。评估未决（哪几种算未决、各自的证据形由 domain.HandoffObservation 与
// domain.HandoffUnresolvedReason 定，这里不复述）——归属决定仍是`其他权威`，结果却落「生产归属未决」，
// 续办引用由评估带出。没记评估的`其他权威`决定
// 走不到这里（编排在 Other 分支一律先交接），但若走到，也只能按未决答：宣布交出去了而没有确认，
// 与没有停写证据一样是替对方宣布交接完成。
func blockedOutcome(decision domain.ProductionOwnershipDecision) SubmitOutcome {
	switch decision.Authority() {
	case domain.ProductionAuthorityOther:
		if assessment, recorded := decision.SafeHandoff(); recorded && assessment.Status() == domain.SafeHandoffConfirmed {
			return OutcomeOtherProductionAuthority
		}
		return OutcomeOwnershipUnresolved
	case domain.ProductionAuthorityUnresolved:
		return OutcomeOwnershipUnresolved
	}
	if decision.AdmissionControl() == domain.AdmissionControlPaused {
		return OutcomeAdmissionPaused
	}
	return OutcomeOwnershipUnresolved
}
