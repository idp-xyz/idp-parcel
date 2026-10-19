package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 实际履约发生项随参与结束自动形成（ADR-0218）。
//
// 本上下文消费自己交出的两种信封——权威交接登记与有效交付登记——找出被这件事实结束的那一条参与；那一段已采用运输委托时，
// 经既有登记用例形成 ACTUAL_FULFILLMENT。不另立第二道形成门：依据、范围、业务时间、采购上下文、协议核对、发生条件、数量与
// 覆盖一问都还是 RegisterChargeOccurrenceHandler 那一处判（ADR-0207、ADR-0209 与 ADR-0207 文末补记），这里只答登记用例
// 答不了的三件——这封信结束了哪一条参与、那一段采没采用委托、发生项属于哪一趟旅程。
//
// 只认已采用委托的段：采用连线是登记方对「这一段按哪份委托向外采购」的声明，有它，采购上下文才不靠任何人另报（ADR-0204
// 决定二末条）。没有连线的段不形成，也不推断自营：连线缺席既不说外包也不说自营（ADR-0204 决定四），交给登记用例它会去问
// 实际承运商判断、可能答不适用——那正是要避免的推断。这些段的发生项照旧由运营决定人工登记。
//
// 不内联进交接与交付编排（ADR-0207 决定七前半句、ADR-0098 决定一不变）：本编排跑在派发进程的 inbox 事务里，读不动就整笔
// 回滚、信封重投，不让物理登记为它失败，也不留一份没人重试的派生步骤。

// ActualFulfillmentFormationRegistrant 是自动形成那一路的登记者。登记者记「这一次登记决定出自谁」（ADR-0209 决定五）：
// 人工登记取操作者信封的主体，自动形成没有操作者，决定出自本上下文的内置策略，于是记本上下文自己的身份。写成「发行方#sub」
// 同一形状，发行方取本上下文的信封来源——操作者的发行方是 OIDC 发行方的 URL，两者不会读成同一个人。登记者不进内容比对，
// 人工照同一身份再登答`已存在`、回显这一格。
const ActualFulfillmentFormationRegistrant = "idp-parcel/transport-fulfillment#form-actual-fulfillment-on-participation-end"

// ErrParticipationEndedInSeveralSegments 说明同一件控制事实在不止一个段里结束了这个对象的参与。一件交接或交付只结束对象
// 此刻那一条在场参与（一对象同时只在一个共同控制范围里，CONTEXT），多于一条是库面不一致；不挑一段去计费，响亮报错。
var ErrParticipationEndedInSeveralSegments = errors.New("transport fulfillment: one control fact ended the object's participation in several segments")

// ActualFulfillmentFormationOutcome 是「一封交接或交付信封到了 → 形成实际履约发生项」的结果代数。
//
// 只有`已交给登记用例`一格要往下看登记的答复（Registration）；其余各格在交给登记用例之前就答了。交接不是`已交接`、
// 这件事实没有结束任何参与、段没有采用委托，三格都是这封信不导致自动形成，分开是因为人去查的地方不同：交接裁决、段登记册、
// 采用连线。
type ActualFulfillmentFormationOutcome uint8

const (
	ActualFulfillmentFormationOutcomeInvalid ActualFulfillmentFormationOutcome = iota
	FormationRegistrationAnswered
	FormationHandoverNotHandedOver
	FormationNoParticipationEnded
	FormationSegmentNotAdopted
	FormationNotAccepted
	FormationUndecided
)

func (outcome ActualFulfillmentFormationOutcome) String() string {
	switch outcome {
	case FormationRegistrationAnswered:
		return "REGISTRATION_ANSWERED"
	case FormationHandoverNotHandedOver:
		return "HANDOVER_NOT_HANDED_OVER"
	case FormationNoParticipationEnded:
		return "NO_PARTICIPATION_ENDED"
	case FormationSegmentNotAdopted:
		return "SEGMENT_NOT_ADOPTED"
	case FormationNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case FormationUndecided:
		return "ACTUAL_FULFILLMENT_FORMATION_UNDECIDED"
	default:
		return ""
	}
}

// ActualFulfillmentFormationUndecidedReason 指名交给登记用例之前停在哪一口。交接读不回单列一格：交接与信封同一笔落库，
// 读不回只剩可见性滞后，与登记册读不动的续办不同。
type ActualFulfillmentFormationUndecidedReason uint8

const (
	FormationUndecidedReasonNone ActualFulfillmentFormationUndecidedReason = iota
	FormationHandoverRegistryUnavailable
	FormationHandoverNotVisible
	FormationSegmentRegistryUnavailable
	FormationAdoptionRegistryUnavailable
	FormationJourneyRegistryUnavailable
)

func (reason ActualFulfillmentFormationUndecidedReason) String() string {
	switch reason {
	case FormationHandoverRegistryUnavailable:
		return "HANDOVER_REGISTRY_UNAVAILABLE"
	case FormationHandoverNotVisible:
		return "HANDOVER_NOT_VISIBLE"
	case FormationSegmentRegistryUnavailable:
		return "SEGMENT_REGISTRY_UNAVAILABLE"
	case FormationAdoptionRegistryUnavailable:
		return "ADOPTION_REGISTRY_UNAVAILABLE"
	case FormationJourneyRegistryUnavailable:
		return "JOURNEY_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// ActualFulfillmentFormationDisposition 是这一次答复对那封信意味着什么，判据同 ADR-0216 决定三：原样重投会变的才重投；
// 不会变的业务答案入账，本上下文自己的缺陷响亮失败而不入账。放在应用层：哪一格是业务答案要认得登记用例的每一格才分得出。
type ActualFulfillmentFormationDisposition uint8

const (
	ActualFulfillmentFormationDispositionInvalid ActualFulfillmentFormationDisposition = iota
	FormationSettled
	FormationRetry
	FormationDefect
)

func (disposition ActualFulfillmentFormationDisposition) String() string {
	switch disposition {
	case FormationSettled:
		return "SETTLED"
	case FormationRetry:
		return "RETRY"
	case FormationDefect:
		return "DEFECT"
	default:
		return ""
	}
}

// FormActualFulfillmentOnHandoverCommand 是交接信封上的判断键：租户、对象、交接范围与版本。版本是键的一维，更正各有自己的
// 信封。
type FormActualFulfillmentOnHandoverCommand struct {
	TenantID domain.TenantID
	Object   string
	Scope    string
	Version  string
}

// FormActualFulfillmentOnDeliveryCommand 是有效交付信封上的判断键：租户、对象、尝试，外加这一封所指的那一版交付结果。
// 版本必备：参与的终点依据记的是结束它的那一版（domain.EffectiveDeliveryEndBasis），POD 更正出的新版本没有结束任何参与。
type FormActualFulfillmentOnDeliveryCommand struct {
	TenantID domain.TenantID
	Object   string
	Attempt  string
	Version  string
}

type FormActualFulfillmentResult struct {
	outcome      ActualFulfillmentFormationOutcome
	reason       ActualFulfillmentFormationUndecidedReason
	segment      string
	registration RegisterChargeOccurrenceResult
}

func (result FormActualFulfillmentResult) Outcome() ActualFulfillmentFormationOutcome {
	return result.outcome
}

// UndecidedReason 只在本编排自己`未决`时非零；登记用例的未决在 Registration 里。
func (result FormActualFulfillmentResult) UndecidedReason() ActualFulfillmentFormationUndecidedReason {
	return result.reason
}

// Segment 是被这件事实结束参与的那一段；没找到时为空。
func (result FormActualFulfillmentResult) Segment() string {
	return result.segment
}

// Registration 只在`已交给登记用例`时有意义，交回登记的完整答复。
func (result FormActualFulfillmentResult) Registration() RegisterChargeOccurrenceResult {
	return result.registration
}

// Disposition 把本编排与登记用例两层答复折成对信封的处置。登记的各格逐一列出、不留「其余入账」的默认：登记用例日后多一格
// 时落进缺陷响起来，而不是悄悄入账吞掉一笔成本来源。
func (result FormActualFulfillmentResult) Disposition() ActualFulfillmentFormationDisposition {
	switch result.outcome {
	case FormationHandoverNotHandedOver, FormationNoParticipationEnded, FormationSegmentNotAdopted:
		return FormationSettled
	case FormationUndecided:
		return FormationRetry
	case FormationRegistrationAnswered:
		return registrationDisposition(result.registration)
	default:
		return FormationDefect
	}
}

// registrationDisposition 判登记的答复：
//   - 入账：形成、已存在、冲突（同一身份异内容、成员或范围已被计过——册上那一项不因重投改变）、协议四个否定格、未配置、
//     不计费；以及未受理「缺采购上下文」——迁移 0038 之前提交的委托没记下协议版本，要人登记时报出，重投报不出来。
//   - 重投：登记用例的未决，某一口此刻读不动。
//   - 缺陷：其余未受理（本编排交的命令过不了登记用例的门，或找到的已结束参与在登记时不认）与不适用（本编排只交已采用
//     委托的段、不声明自营，登记用例不会答它）。
func registrationDisposition(registered RegisterChargeOccurrenceResult) ActualFulfillmentFormationDisposition {
	switch registered.Outcome() {
	case ChargeOccurrenceRegistered,
		ChargeOccurrenceExisting,
		ChargeOccurrenceConflict,
		ChargeOccurrenceAgreementNotRegistered,
		ChargeOccurrenceAgreementNotEffective,
		ChargeOccurrenceAgreementTerminated,
		ChargeOccurrenceProviderMismatch,
		ChargeOccurrenceNotConfigured,
		ChargeOccurrenceNotCharged:
		return FormationSettled
	case ChargeOccurrenceInputNotAccepted:
		if registered.Detail() == ProcurementContextMissing {
			return FormationSettled
		}
		return FormationDefect
	case ChargeOccurrenceUndecided:
		return FormationRetry
	default:
		return FormationDefect
	}
}

// ParticipationSegmentsView 是段登记册按对象列段、按键读段的两口：结束参与的事实不带段号，要由登记册替它找。
type ParticipationSegmentsView interface {
	FindSegmentsForObject(ctx context.Context, tenant domain.TenantID, object domain.CarriedObjectReference) ([]ports.FulfillmentSegmentKey, error)
	FindByKey(ctx context.Context, key ports.FulfillmentSegmentKey) (ports.FulfillmentSegmentRecord, bool, error)
}

// ChargeOccurrenceRegistration 是既有登记用例那一口，真实装配接 RegisterChargeOccurrenceHandler。
type ChargeOccurrenceRegistration interface {
	Register(ctx context.Context, command RegisterChargeOccurrenceCommand) (RegisterChargeOccurrenceResult, error)
}

// FormActualFulfillmentOnParticipationEndDeps 全部必备，都是只读口，外加登记用例。
type FormActualFulfillmentOnParticipationEndDeps struct {
	Handovers    TransportHandoverFinder
	Segments     ParticipationSegmentsView
	Adoptions    ports.SegmentCommissionAdoptionView
	Journeys     ports.AlternateJourneyMemberView
	Registration ChargeOccurrenceRegistration
}

type FormActualFulfillmentOnParticipationEndHandler struct {
	deps FormActualFulfillmentOnParticipationEndDeps
}

func NewFormActualFulfillmentOnParticipationEndHandler(
	deps FormActualFulfillmentOnParticipationEndDeps,
) *FormActualFulfillmentOnParticipationEndHandler {
	return &FormActualFulfillmentOnParticipationEndHandler{deps: deps}
}

// OnHandover 跑交接那一路：受理键 → 读回那一版交接（须是`已交接`，转出依据由交接自己交出）→ 找它结束的参与 → 形成。
func (handler *FormActualFulfillmentOnParticipationEndHandler) OnHandover(
	ctx context.Context,
	command FormActualFulfillmentOnHandoverCommand,
) (FormActualFulfillmentResult, error) {
	key, accepted := transportHandoverKeyOf(command.TenantID, command.Object, command.Scope, command.Version)
	if !accepted {
		return FormActualFulfillmentResult{outcome: FormationNotAccepted}, nil
	}
	record, found, err := handler.deps.Handovers.FindByKey(ctx, key)
	if err != nil {
		return formationUndecided(FormationHandoverRegistryUnavailable), nil
	}
	if !found {
		return formationUndecided(FormationHandoverNotVisible), nil
	}
	reference, transfers := record.Handover.TransferOutBasis()
	if !transfers {
		return FormActualFulfillmentResult{outcome: FormationHandoverNotHandedOver}, nil
	}
	basis, err := domain.NewParticipationBasisReference(reference)
	if err != nil {
		return FormActualFulfillmentResult{}, fmt.Errorf("form actual fulfillment on handover: transfer-out basis: %w", err)
	}
	return handler.formFor(ctx, key.TenantID, key.Object, domain.EndedByNextHandover, basis)
}

// OnDelivery 跑有效交付那一路：受理键 → 由信封所指的那一版交付结果得出终点依据 → 找它结束的参与 → 形成。不读交付登记册：
// 参与的终点与交付、信封同一笔落库，段登记册上那一条就是这件事实结束了什么的权威记录。
func (handler *FormActualFulfillmentOnParticipationEndHandler) OnDelivery(
	ctx context.Context,
	command FormActualFulfillmentOnDeliveryCommand,
) (FormActualFulfillmentResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return FormActualFulfillmentResult{outcome: FormationNotAccepted}, nil
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return FormActualFulfillmentResult{outcome: FormationNotAccepted}, nil
	}
	if _, err := domain.NewAttemptReference(command.Attempt); err != nil {
		return FormActualFulfillmentResult{outcome: FormationNotAccepted}, nil
	}
	version, err := domain.NewDeliveryResultVersion(command.Version)
	if err != nil {
		return FormActualFulfillmentResult{outcome: FormationNotAccepted}, nil
	}
	basis, err := domain.EffectiveDeliveryEndBasis(version)
	if err != nil {
		return FormActualFulfillmentResult{outcome: FormationNotAccepted}, nil
	}
	return handler.formFor(ctx, command.TenantID, object, domain.EndedByEffectiveDelivery, basis)
}

// formFor 是两路共用的后半：在对象有过参与的各段里找终点恰是这件事实的那一条 → 段须已采用委托 → 定旅程 → 交登记用例。
//
// 按「终点是这件事实」找，不按「此刻在场」找：参与此刻已经离场。也不取对象最近一段：信封晚到时对象可能又进了下一段，
// 只有终点依据说得清结束的是哪一条。没找到就是这件事实没结束任何参与——拒收与待确认不转出控制、更正出的新版本没有结束
// 什么、对象本就不在段里，都落这一格。
//
// 成员只有这一个对象：一件交接或交付结束的是一个对象的参与，同一段上别的成员在各自的事实上各形成各的（按件计时逐件，
// 按段计时第一件盖住整段、其余答范围已计过，ADR-0209 决定四）。
func (handler *FormActualFulfillmentOnParticipationEndHandler) formFor(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
	kind domain.ParticipationEndKind,
	basis domain.ParticipationBasisReference,
) (FormActualFulfillmentResult, error) {
	keys, err := handler.deps.Segments.FindSegmentsForObject(ctx, tenant, object)
	if err != nil {
		return formationUndecided(FormationSegmentRegistryUnavailable), nil
	}
	var segment domain.FulfillmentSegmentReference
	var participation domain.FulfillmentParticipation
	ended := 0
	for _, key := range keys {
		record, found, err := handler.deps.Segments.FindByKey(ctx, key)
		if err != nil {
			return formationUndecided(FormationSegmentRegistryUnavailable), nil
		}
		if !found {
			// 段不删（CONTEXT「已经成立的实际履约段……不能被取消、删除」），登记册按对象列得出、按键却读不回是它自己的不一致。
			return FormActualFulfillmentResult{}, fmt.Errorf(
				"form actual fulfillment: segment %s listed for object %s is not registered", key.Segment, object)
		}
		if current, matched := record.Segment.ParticipationEndedBy(object, kind, basis); matched {
			segment, participation = key.Segment, current
			ended++
		}
	}
	switch ended {
	case 0:
		return FormActualFulfillmentResult{outcome: FormationNoParticipationEnded}, nil
	case 1:
	default:
		return FormActualFulfillmentResult{}, fmt.Errorf("%w: object %s, basis %s", ErrParticipationEndedInSeveralSegments, object, basis)
	}

	_, adopted, err := handler.deps.Adoptions.FindBySegment(ctx, ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: segment})
	if err != nil {
		return formationUndecided(FormationAdoptionRegistryUnavailable), nil
	}
	if !adopted {
		return FormActualFulfillmentResult{outcome: FormationSegmentNotAdopted, segment: segment.String()}, nil
	}

	records, err := handler.deps.Journeys.FindByMember(ctx, tenant, object)
	if err != nil {
		return formationUndecided(FormationJourneyRegistryUnavailable), nil
	}
	alternates := make([]domain.AlternateJourney, 0, len(records))
	for _, record := range records {
		alternates = append(alternates, record.Journey)
	}
	// 同一刻开始两趟是旅程册自己的不一致，重投不会变，上抛。
	journey, err := domain.ActualFulfillmentJourney(object, participation.EnteredAt(), alternates)
	if err != nil {
		return FormActualFulfillmentResult{}, fmt.Errorf("form actual fulfillment: journey of %s: %w", object, err)
	}

	registered, err := handler.deps.Registration.Register(ctx, RegisterChargeOccurrenceCommand{
		TenantID:   tenant,
		Registrant: ActualFulfillmentFormationRegistrant,
		Reason:     domain.ActualFulfillmentOccurrence,
		Segment:    segment.String(),
		Objects:    []string{object.String()},
		Journey:    journey.String(),
	})
	if err != nil {
		return FormActualFulfillmentResult{}, err
	}
	return FormActualFulfillmentResult{
		outcome:      FormationRegistrationAnswered,
		segment:      segment.String(),
		registration: registered,
	}, nil
}

func formationUndecided(reason ActualFulfillmentFormationUndecidedReason) FormActualFulfillmentResult {
	return FormActualFulfillmentResult{outcome: FormationUndecided, reason: reason}
}
