package accessidentity

import (
	"errors"
	"strings"
	"time"
)

// ErrDecisionKindUnknown 表示给出的决定种类不在 ADR-0151 决定一列出的那几种里。
var ErrDecisionKindUnknown = errors.New("access identity: decision kind is unknown")

// ErrDecisionKindRequired 表示「运营决定」这一格没带决定种类：它的授予按租户 × 决定种类登记
// （ADR-0151 决定一），不带种类的一笔等于把所有决定一并授出。经 NewOperationDecisionGrant 建。
var ErrDecisionKindRequired = errors.New("access identity: operation decision grant needs a decision kind")

// DecisionKind 是「运营决定」能力面下的决定种类（ADR-0151 决定一）。
//
// 种类是册的列，由产品的角色模型定；某租户把哪位操作者授予哪一种决定，才是租户取值。待识别重核是复核页上对指名的一件待识别
// 实物按存下的号再问一次 PS（ADR-0199 决定三），管理台上作判断的口照决定一归这一面。
// 身份冲突处置是复核页上对一件判身份冲突的作业实物指定一件候选或确认都不是（ADR-0210 决定一）；授予就是它全部的授权门。
// 运输委托提交是运营人员向外部服务提供方
// 提出明确的运输范围与条件（ADR-0202 决定一），委托、订舱、取消各口每口一种，授予的粒度等于权力的粒度。
type DecisionKind string

const (
	DecisionManualReviewCompletion              DecisionKind = "MANUAL_REVIEW_COMPLETION"
	DecisionActiveRejection                     DecisionKind = "ACTIVE_REJECTION"
	DecisionAuthorizedDisposition               DecisionKind = "AUTHORIZED_DISPOSITION"
	DecisionControlledClosure                   DecisionKind = "CONTROLLED_CLOSURE"
	DecisionControlledReopening                 DecisionKind = "CONTROLLED_REOPENING"
	DecisionSegmentClosure                      DecisionKind = "SEGMENT_CLOSURE"
	DecisionDispatchTaskRegistration            DecisionKind = "DISPATCH_TASK_REGISTRATION"
	DecisionLoadAssignment                      DecisionKind = "LOAD_ASSIGNMENT"
	DecisionParticipationTermination            DecisionKind = "PARTICIPATION_TERMINATION"
	DecisionEffectiveTimeJudgment               DecisionKind = "EFFECTIVE_TIME_JUDGMENT"
	DecisionCarrierFirstEffectivePickupJudgment DecisionKind = "CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT"
	DecisionPendingIdentityRecheck              DecisionKind = "PENDING_IDENTITY_RECHECK"
	DecisionIdentityConflictDisposition         DecisionKind = "IDENTITY_CONFLICT_DISPOSITION"
	DecisionTransportCommissionSubmission       DecisionKind = "TRANSPORT_COMMISSION_SUBMISSION"
)

// DecisionBookingRequestSubmission 是订舱提交（ADR-0202 决定一、五）：运营人员针对一份已提交的运输委托，向它的服务提供方
// 就一个在册的具体班次申请舱位。它与委托提交分开授予：能提交委托的不因此能替任何班次订舱。
const DecisionBookingRequestSubmission DecisionKind = "BOOKING_REQUEST_SUBMISSION"

// DecisionTransportChargeOccurrenceRegistration 是运输收费发生项登记（ADR-0207 决定六）：对已发生的履约事实作一次
// 采购判断——断言采用协议的发生条件已满足、指名服务方与协议版本。它不是现场作业事实，所以归运营决定而不归作业事实面。
const DecisionTransportChargeOccurrenceRegistration DecisionKind = "TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION"

// 容量预占与容量释放（ADR-0203 决定三）：为一个业务范围占住一个池的一段容量，与把其中未消耗的部分还回去。两者是运营
// 人员作出的生产决定，各成一种：能占的不因此能放，授予的粒度等于权力的粒度。
const (
	DecisionCapacityReservation DecisionKind = "CAPACITY_RESERVATION"
	DecisionCapacityRelease     DecisionKind = "CAPACITY_RELEASE"
)

// 履约段采用运输委托、取消运输委托、取消订舱（ADR-0202 决定一、ADR-0204）：三者都是运营人员作出的生产决定，各成一种。
// 采用把一个在册的实际履约段声明为在一份已提交委托下履约，并随之登记委托下的运输已开始——之后这份委托就取消不了；
// 能提交委托的不因此能取消它，能取消的也不因此能把它挂到段上，授予的粒度等于权力的粒度。
const (
	DecisionFulfillmentSegmentCommissionAdoption DecisionKind = "FULFILLMENT_SEGMENT_COMMISSION_ADOPTION"
	DecisionTransportCommissionCancellation      DecisionKind = "TRANSPORT_COMMISSION_CANCELLATION"
	DecisionBookingRequestCancellation           DecisionKind = "BOOKING_REQUEST_CANCELLATION"
)

// DecisionDeliveryHandoverConfirmation 是送站交接确认（ADR-0212 决定三）：持这一种授予的人指名送站交接单、客户账户、交付方参与方与
// 依据引用，依 party-commercial 的授权交付关系确认交接单对那个客户账户的绑定。只能确认、不能否认——否认是客户账户自己的话，走客户面。
const DecisionDeliveryHandoverConfirmation DecisionKind = "DELIVERY_HANDOVER_CONFIRMATION"

// 关务立案、提交授权确认与申报提交执行（ADR-0227 决定一）：三者都是运营人员作出的生产决定，各成一种。批准与实际提交
// 分开授予——关务 CONTEXT 要求请求、批准、撤销与实际提交权限分别表达，持其中一种不因此取得另一种；首发不启用生产自动申报，
// 提交只经持后一种授予的人逐次发起。
const (
	DecisionCustomsCaseEstablishment       DecisionKind = "CUSTOMS_CASE_ESTABLISHMENT"
	DecisionCustomsSubmissionAuthorization DecisionKind = "CUSTOMS_SUBMISSION_AUTHORIZATION"
	DecisionCustomsDeclarationSubmission   DecisionKind = "CUSTOMS_DECLARATION_SUBMISSION"
)

// DecisionDeliveryDispatchReask 是派送重问（ADR-0230 决定四、七）：让派送发起执行器对派送待形成册上的对象再问一次所有者——按册
// 批量重问与对单个（段，对象）补拍是同一种权力，一种授予：它们都只能在所有者给齐派送要求时形成任务，不能指名工作范围的任何一件，
// 与手工建派送任务（DISPATCH_TASK_REGISTRATION）不是一回事。
const DecisionDeliveryDispatchReask DecisionKind = "DELIVERY_DISPATCH_REASK"

// 外部资金事实采用与监管凭证登记（ADR-0236 决定一）：租户的财务、关务人员照银行回单、凭证原件在管理台手工采用。事实本身仍由外部
// 系统拥有，采用的这一步是运营人员作出的生产决定，所以归运营决定而不归作业事实面；两类各成一种——能采用资金事实的不因此能登凭证，
// 反过来也一样，管钱的与管关务的通常不是一个人。资金事实的更正与首版同一种：更正是采用的一种，随首版的认证与授予（ADR-0151 决定四
// 的同一条判据）。
const (
	DecisionExternalFundsFactAdoption        DecisionKind = "EXTERNAL_FUNDS_FACT_ADOPTION"
	DecisionRegulatoryCredentialRegistration DecisionKind = "REGULATORY_CREDENTIAL_REGISTRATION"
)

// DecisionTransportCommissionAgreementRevision 是委托协议换版（ADR-0231 决定三）：运营人员指名一份运输委托自哪一业务时刻起改按同一协议
// 对象的后继版本采购。它改的是委托此后形成的成本按哪一版协议判，与提交委托、取消委托都不是同一种权力——能提交的不因此能事后改一份
// 已在途委托的协议版本，所以另成一种。
const DecisionTransportCommissionAgreementRevision DecisionKind = "TRANSPORT_COMMISSION_AGREEMENT_REVISION"

// 解除履约段采用与运输收费发生项失效（ADR-0232 决定一、四）：两者都是对已登记声明的更正，各成一种。解除采用撤掉一段与一份委托之间
// 那条声明，委托的开始随之撤回或重新派生；发生项失效说一项已登记的发生项所说的范围不成立，结算据此追加纠错。两者分开授予：能让
// 发生项失效的不因此能改采购组织，能解除采用的也不因此能撤一笔已形成的成本来源——解除被挡在已形成的发生项上，正是要那一步由
// 持后一种授予的人显式做。能采用、能登记的也不因此能更正，更正与登记分开授予，先例是受控关闭与重开分成两种。
const (
	DecisionFulfillmentSegmentCommissionAdoptionWithdrawal DecisionKind = "FULFILLMENT_SEGMENT_COMMISSION_ADOPTION_WITHDRAWAL"
	DecisionTransportChargeOccurrenceInvalidation          DecisionKind = "TRANSPORT_CHARGE_OCCURRENCE_INVALIDATION"
)

// DecisionAuthorizedReroute 是授权改路（ADR-0284 决定一）：授权角色据一份改路建议择一候选、形成改路决定与新计划。它是路由侧
// 的生产决定，与运输、关务各口的授予互不相通——能提交委托的不因此能改一件包裹的路由。
const DecisionAuthorizedReroute DecisionKind = "AUTHORIZED_REROUTE"

// DecisionControlReleaseRefusalHandling 是释放拒收处理（ADR-0293 决定三）：运维核对后在结算侧释放一笔被提供方确定不受理的占用，或把它
// 标记为已在别处处理，并让接受链重驱那份委托。它不与授权处置（AUTHORIZED_DISPOSITION）共用——授权处置是对一份受限委托去向的业务处置，
// 这一种是对已越过业务决定的资金占用的运维处理，职责不同，授予也分开：能处置委托的不因此能替一笔被拒收的释放销账。
const DecisionControlReleaseRefusalHandling DecisionKind = "CONTROL_RELEASE_REFUSAL_HANDLING"

// DecisionCancellationOrderConflictResolution 是取消先后冲突的解开（ADR-0295 决定二）：有权主体对册上一份冲突决定「认定取消依据不成立」或「认定
// 边界事实有误、交来源方更正」。它不与授权处置（AUTHORIZED_DISPOSITION）共用——授权处置是对一份受限委托去向的业务处置，这一种是对一件包裹上
// 取消与边界事实谁在先的裁断，认错了会让一件已取消的包裹长出责任起点，或让一件已在网络手里的包裹挂着取消终局，职责不同，授予也分开。
const DecisionCancellationOrderConflictResolution DecisionKind = "CANCELLATION_ORDER_CONFLICT_RESOLUTION"

// 履约尝试待确认结果的认定（ADR-0297 决定二）：复核人核对来源之后，把一件对象在一次上门里还停在待确认的结果定成确定结果。揽收与
// 派送各成一种——能认定揽收的不因此能认定派送，两侧通常不是同一批人；也不与承运商首次有效收寄判断（CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT）
// 共用，那一种判的是承运主体首次接触，这一种认定的是这一次上门里某件对象的结果。
const (
	DecisionPickupAttemptResultConfirmation   DecisionKind = "PICKUP_ATTEMPT_RESULT_CONFIRMATION"
	DecisionDeliveryAttemptResultConfirmation DecisionKind = "DELIVERY_ATTEMPT_RESULT_CONFIRMATION"
)

// DecisionPostIntakeServiceDisposition 是收寄后服务处置（ADR-0332 决定二）：有权主体对一件越过取消边界的包裹形成继续、拦截、改送、退运或服务终止的处置决定，
// 向各对象所有者提出范围化请求。它不与授权处置（AUTHORIZED_DISPOSITION）共用——授权处置是对一份受限委托去向的业务处置，这一种改的是一件已在网络手里的包裹
// 的实物去向与后续服务，认错了会让包裹被扣、被退或被终止服务；也不与取消先后冲突的解开共用，那一种裁断的是取消与边界事实谁在先。谁持有是租户取值，未登记时没有人能决定。
const DecisionPostIntakeServiceDisposition DecisionKind = "POST_INTAKE_SERVICE_DISPOSITION"

// 监管运输处置承接与替代/退运旅程启动（ADR-0337 决定一）：运输运营对关务协作事项形成承接决定，与授权业务角色启动独立的备用或退运
// 旅程，各成一种。不与运输委托提交（TRANSPORT_COMMISSION_SUBMISSION）共用——那一种是向服务提供方提出运输范围，这两种是接受
// 监管移动责任与开一条新旅程；能承接的不因此能开旅程，开旅程的也不因此能替运输方接受监管责任。
const (
	DecisionRegulatoryDispositionAcceptance DecisionKind = "REGULATORY_DISPOSITION_ACCEPTANCE"
	DecisionAlternateJourneyStart           DecisionKind = "ALTERNATE_JOURNEY_START"
)

// 内部合规限制的建立与决定（ADR-0334 决定七）：限制责任来源之下的运营人员形成一份内部合规限制，与对限制自己的覆盖项追加确认或解除决定，
// 各成一种。能建立的不因此能解除——解除是限制责任来源的话，授予的粒度等于权力的粒度；也不与授权处置（AUTHORIZED_DISPOSITION）共用，那一种
// 处置的是一份受限委托的去向，这两种改的是动作准入读到的限制册。谁持有是租户取值，未登记时没有人能建立或解除。
const (
	DecisionInternalRestrictionEstablishment DecisionKind = "INTERNAL_RESTRICTION_ESTABLISHMENT"
	DecisionInternalRestrictionDecision      DecisionKind = "INTERNAL_RESTRICTION_DECISION"
)

// DecisionCustomsCaseClosure 是关务案件关闭（UC-CC-010 的关闭入口）：有权角色对进行中的关务案件请求关闭，关闭核对逐项盘点义务、全部终结或被有效承接才
// 形成关闭决定。它与关务建案（CUSTOMS_CASE_ESTABLISHMENT）分开授予——关务 CONTEXT 要求关闭决定角色不由管理员或通用关务角色代替，能建案的
// 不因此能关案。受控重开是另一种权力（原关闭责任来源约束），不在这一种里。谁持有是租户取值，未登记时没有人能关案。
const DecisionCustomsCaseClosure DecisionKind = "CUSTOMS_CASE_CLOSURE"

// DecisionReleaseGateVerification 是放行门禁核对（UC-CC-009）：有权角色对一个申报范围、一个拟执行动作和一个监管边界请求形成放行门禁核对，
// 落册并向下游发意图。门禁满足不生成放行，所以它与关务建案、申报提交、关案都不共用——UC-CC-009 要求税费结果接收、付款核对、门禁核对与
// 外部放行接收分别授权，持其中一种不因此取得另一种。谁持有是租户取值，未登记时没有人能请求核对。
const DecisionReleaseGateVerification DecisionKind = "RELEASE_GATE_VERIFICATION"

// 节点监管协作承接（ADR-0338 决定一）：节点作业责任角色对关务协作事项形成承接决定（已承接、部分承接、待补充、拒绝），自成一种。不与
// 送站交接确认（DELIVERY_HANDOVER_CONFIRMATION）或身份冲突处置共用——那两种裁断的是交付范围与实物身份，这一种是接受现场监管作业责任；
// 也不与现场执行事实登记（作业事实登记面的 NODE_COLLABORATION_EXECUTION）共用：能承接的人不因此能替现场登记已做了什么。
const DecisionNodeCollaborationAcceptance DecisionKind = "NODE_COLLABORATION_ACCEPTANCE"

// 结算的两种运营决定（ADR-0339 决定一、二）：点名一笔客户费用补拍确认，与发布一期客户对账单。确认只把一笔费用按它自己的确认条件
// 定格，发布要列一期的费用、报合计并固定单号与账期，做这两件事的通常不是同一批人，授予分开。
const (
	DecisionChargeConfirmation   DecisionKind = "CHARGE_CONFIRMATION"
	DecisionStatementPublication DecisionKind = "STATEMENT_PUBLICATION"
)

// 供应商账单的两种运营决定（ADR-0340 决定一、二）：接收一份账单并裁决逐行匹配，与对一行形成审核应付。UC-SA-004 要求账单接收与
// 审核应付分别授权——能录账单的人不因此能放行应付，审核的人也不因此能改供应商的主张。
const (
	DecisionSupplierBillReception DecisionKind = "SUPPLIER_BILL_RECEPTION"
	DecisionSupplierBillAudit     DecisionKind = "SUPPLIER_BILL_AUDIT"
)

// DecisionPostIntakeManualItemHandling 是待人工处理项的处理结果登记（ADR-0340 决定三）：运营人员对收寄后服务处置决定留下的一项人工请求项登记「已处理」
// 或「无法处理」。它与收寄后服务处置（POST_INTAKE_SERVICE_DISPOSITION）分开授予——形成决定的人与在现场办完这些事的人通常不是同一批，登记的结果还是
// 其后各方读回承接情况的依据。谁持有是租户取值，未登记时没有人能登记。
const DecisionPostIntakeManualItemHandling DecisionKind = "POST_INTAKE_MANUAL_ITEM_HANDLING"

var decisionKinds = []DecisionKind{
	DecisionManualReviewCompletion, DecisionActiveRejection, DecisionAuthorizedDisposition,
	DecisionControlledClosure, DecisionControlledReopening,
	DecisionSegmentClosure, DecisionDispatchTaskRegistration, DecisionLoadAssignment, DecisionParticipationTermination,
	DecisionEffectiveTimeJudgment, DecisionCarrierFirstEffectivePickupJudgment,
	DecisionPendingIdentityRecheck,
	DecisionIdentityConflictDisposition,
	DecisionTransportCommissionSubmission,
	DecisionTransportChargeOccurrenceRegistration,
	DecisionCapacityReservation, DecisionCapacityRelease,
	DecisionBookingRequestSubmission,
	DecisionFulfillmentSegmentCommissionAdoption, DecisionTransportCommissionCancellation, DecisionBookingRequestCancellation,
	DecisionDeliveryHandoverConfirmation,
	DecisionCustomsCaseEstablishment, DecisionCustomsSubmissionAuthorization, DecisionCustomsDeclarationSubmission,
	DecisionDeliveryDispatchReask,
	DecisionExternalFundsFactAdoption, DecisionRegulatoryCredentialRegistration,
	DecisionTransportCommissionAgreementRevision,
	DecisionFulfillmentSegmentCommissionAdoptionWithdrawal, DecisionTransportChargeOccurrenceInvalidation,
	DecisionAuthorizedReroute,
	DecisionControlReleaseRefusalHandling,
	DecisionCancellationOrderConflictResolution,
	DecisionPickupAttemptResultConfirmation, DecisionDeliveryAttemptResultConfirmation,
	DecisionPostIntakeServiceDisposition,
	DecisionRegulatoryDispositionAcceptance, DecisionAlternateJourneyStart,
	DecisionInternalRestrictionEstablishment, DecisionInternalRestrictionDecision,
	DecisionCustomsCaseClosure,
	DecisionReleaseGateVerification,
	DecisionNodeCollaborationAcceptance,
	DecisionChargeConfirmation, DecisionStatementPublication,
	DecisionSupplierBillReception, DecisionSupplierBillAudit,
	DecisionPostIntakeManualItemHandling,
}

// ParseDecisionKind 按字面取值认决定种类，大小写不折叠：册里存的就是这些字面量。
func ParseDecisionKind(value string) (DecisionKind, error) {
	for _, kind := range decisionKinds {
		if string(kind) == value {
			return kind, nil
		}
	}
	return "", ErrDecisionKindUnknown
}

func (kind DecisionKind) String() string { return string(kind) }

// NewOperationDecisionGrant 登一笔「运营决定」授予：能力面固定为 CapabilityOperationDecision，
// 决定种类必填且须是已知的一种。其余各件的纪律同 NewOperatorGrant。
func NewOperationDecisionGrant(
	tenantID string,
	grantID string,
	subject OperatorSubject,
	kind DecisionKind,
	interval EffectiveInterval,
	basis string,
) (OperatorGrant, error) {
	if _, err := ParseDecisionKind(string(kind)); err != nil {
		return OperatorGrant{}, err
	}
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(grantID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || id == "" || subject.issuer == "" || interval.startsAt.IsZero() || reference == "" {
		return OperatorGrant{}, ErrIncompleteOperatorRegistration
	}
	return OperatorGrant{
		tenantID:     tenant,
		grantID:      id,
		subject:      subject,
		face:         CapabilityOperationDecision,
		decisionKind: kind,
		interval:     interval,
		basis:        reference,
	}, nil
}

// DecisionKind 交回「运营决定」授予的决定种类；别的能力面上为空。
func (grant OperatorGrant) DecisionKind() DecisionKind { return grant.decisionKind }

// HoldsDecisionAt 答这个操作者在 at 那一刻是否持有 kind 这一种运营决定的生效授予。
func (standing OperatorStanding) HoldsDecisionAt(kind DecisionKind, at time.Time) bool {
	if kind == "" {
		return false
	}
	for _, recorded := range standing.grants {
		if recorded.grant.face == CapabilityOperationDecision && recorded.grant.decisionKind == kind && recorded.EffectiveAt(at) {
			return true
		}
	}
	return false
}
