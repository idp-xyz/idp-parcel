package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// UC-CC-009 步 4–7 的编排（票 mechanism-executor-triage/07 CC-c）：形成税费付款协作事项
// （步 4–5）、接收外部资金事实引用（步 6 的 CC 半边）、形成税费付款核对（步 7）。此前
// application/ 里没有步 2–7 的任何文件，verify_release_gate.go 只做步 10。
//
// 三件分开写口、分开幂等键，因为它们是 UC-CC-009「七层对象必须分离」里的三层：付款协作、
// 外部资金事实、关务付款判断——压成一个「税费状态」正是那节禁的事。三者的先后由核对那一
// 步的两道前置守：没有已接收的资金事实、没有已形成的协作事项都核不起来，各有自己的格。
//
// 核对的三轴（覆盖/差额/有效性）与关联依据由调用方交进来，编排不从金额相等推：真实程序
// 的付款条件与关联规则属实例半边（UC-CC-009「付款义务与门禁关系规则」那行），今天没有
// 它们，编排能守的是「无依据不关联」（无权威依据时保持外部资金事实待关联）与「三轴集外不受
// 理」。与 RegisterGateFinding 收登记方判断的形状同一族。
//
// 步 8 向 settlement-accounting 的交接（票 sa-cc/05）在核对**形成**那一格同事务交出：信封只带
// 引用（租户、申报范围、税费引用、资金事实引用、版本指纹），三轴留在本上下文的核对册里由 SA
// 按引用回读——CC 是核对的权威，「核对形成」也不是「代垫成立」（SA CONTEXT：任一单项输入不能
// 直接推导实际代垫）。`已存在`不重发、前置未齐与待关联不发：信封说的是「这一版核对已形成」。
// 事务由进程级入口给出。

// DutyReconciliationOutcome 是本编排三个方法共用的应用处理结果。
type DutyReconciliationOutcome uint8

const (
	DutyReconciliationOutcomeInvalid DutyReconciliationOutcome = iota
	CollaborationFormed
	CollaborationExisting
	CollaborationContentConflict
	FundsFactReceived
	FundsFactExisting
	FundsFactContentConflict
	FundsFactPendingAssociation
	FundsFactNotReceived
	CollaborationNotFormed
	DutyVerificationFormed
	DutyVerificationExisting
	DutyReconciliationNotAccepted
	DutyReconciliationUndecided
)

func (outcome DutyReconciliationOutcome) String() string {
	switch outcome {
	case CollaborationFormed:
		return "COLLABORATION_FORMED"
	case CollaborationExisting:
		return "EXISTING_COLLABORATION"
	case CollaborationContentConflict:
		return "COLLABORATION_CONTENT_CONFLICT"
	case FundsFactReceived:
		return "FUNDS_FACT_RECEIVED"
	case FundsFactExisting:
		return "EXISTING_FUNDS_FACT"
	case FundsFactContentConflict:
		return "FUNDS_FACT_CONTENT_CONFLICT"
	case FundsFactPendingAssociation:
		return "FUNDS_FACT_PENDING_ASSOCIATION"
	case FundsFactNotReceived:
		return "FUNDS_FACT_NOT_RECEIVED"
	case CollaborationNotFormed:
		return "COLLABORATION_NOT_FORMED"
	case DutyVerificationFormed:
		return "DUTY_VERIFICATION_FORMED"
	case DutyVerificationExisting:
		return "EXISTING_DUTY_VERIFICATION"
	case DutyReconciliationNotAccepted:
		return "NOT_ACCEPTED"
	case DutyReconciliationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// DutyReconciliationReason 指名未决停在哪一步等谁。DutyObligationBasisAbsent 是业务上的未决
// （UC-CC-009 步 4 的第四个结果「未决」）：既无核定税费也无明确无需付款依据——UC-CC-009「没有税费结果不能被解释为无需付款」，
// 编排不形成支付指令也不替它选一格。其余三格是依赖故障。
type DutyReconciliationReason uint8

const (
	DutyReconciliationReasonNone DutyReconciliationReason = iota
	DutyObligationBasisAbsent
	CollaborationStoreUnavailable
	FundsFactRegisterUnavailable
	DutyVerificationStoreUnavailable
)

func (reason DutyReconciliationReason) String() string {
	switch reason {
	case DutyObligationBasisAbsent:
		return "DUTY_OBLIGATION_BASIS_ABSENT"
	case CollaborationStoreUnavailable:
		return "COLLABORATION_STORE_UNAVAILABLE"
	case FundsFactRegisterUnavailable:
		return "FUNDS_FACT_REGISTER_UNAVAILABLE"
	case DutyVerificationStoreUnavailable:
		return "DUTY_VERIFICATION_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

type DutyReconciliationResult struct {
	outcome    DutyReconciliationOutcome
	reason     DutyReconciliationReason
	handoffRef string
}

func (result DutyReconciliationResult) Outcome() DutyReconciliationOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result DutyReconciliationResult) UndecidedReason() DutyReconciliationReason {
	return result.reason
}

// HandoffReference 非空说明这一版核对已入册但结算意图还没交出去；只在`核对已形成`那一格
// 可能非空。它不像本上下文其余编排那样靠「重放重发」兑现：本口`已存在`不重发（VerifyPayment
// 注释里的理由），续办引用因此是给运维看的坐标，不是给下一次调用的重投指令。
func (result DutyReconciliationResult) HandoffReference() string {
	return result.handoffRef
}

func dutyUndecided(reason DutyReconciliationReason) DutyReconciliationResult {
	return DutyReconciliationResult{outcome: DutyReconciliationUndecided, reason: reason}
}

// FormDutyCollaborationCommand 携带一次协作事项形成：义务依据两格之一（核定税费引用，或
// 真实程序的明确无需付款依据）加范围、法定义务人、付款要求来源与责任交接目标。FormedAt 不是
// 输入，取时钟。
type FormDutyCollaborationCommand struct {
	TenantID    domain.TenantID
	Kind        domain.DutyObligationKind
	Duty        domain.AssessedDutyReference
	NoPayBasis  string
	Scope       domain.DecisionScopeReference
	Obligor     domain.LegalObligorReference
	Requirement domain.PaymentRequirementSource
	Target      domain.ResponsibilityTargetReference
}

// ReceiveExternalFundsFactCommand 携带一次外部资金事实的入向登记（步 6 的 CC 半边）。
type ReceiveExternalFundsFactCommand struct {
	TenantID     domain.TenantID
	Registration ports.ExternalFundsFactRegistration
}

// VerifyDutyPaymentCommand 携带一次税费付款核对：核对身份三维（税费版本、资金事实、范围）、
// 三轴判断与关联依据。Basis 是「凭什么把这笔资金关联到这版税费」的证据引用（真实规则或来源
// 提供的关联依据）——空即无权威依据，编排保持资金事实待关联而不形成核对。
type VerifyDutyPaymentCommand struct {
	TenantID domain.TenantID
	Duty     domain.AssessedDutyReference
	Funds    domain.ExternalFundsFactReference
	Scope    domain.DecisionScopeReference
	Coverage domain.DutyCoverage
	Delta    domain.DutyDelta
	Validity domain.DutyFactValidity
	Basis    string
}

type DutyPaymentReconciliationDeps struct {
	Collaborations ports.DutyCollaborationStore
	Funds          ports.ExternalFundsFactRegister
	Verifications  ports.DutyVerificationStore
	// Handoff 是步 8 的结算交接口，只被 VerifyPayment 在核对形成那一格调用。只走 FormCollaboration
	// 或 ReceiveFundsFact 的装配点也得接真口——构造门对每一口一视同仁，漏装要在启动那一刻炸出来。
	Handoff ports.DutyPaymentVerificationHandoff
	Clock   ports.Clock
}

type DutyPaymentReconciliationHandler struct {
	deps DutyPaymentReconciliationDeps
}

// ErrNilDependency 是构造门对缺件的唯一答复；哪一口缺在包装信息里点名。它必须是构造期的错误而不是
// 运行期的 panic：装配疏漏要在进程启动那一刻炸出来，而不是等第一封资金事实信封到达、编排解引用
// 那一口时才发现——那时它与「登记册暂不可用」折出的`未决`在消费门那侧长得一样，重投也救不回来。
// 与 settlementaccounting/application 的 NewApplyPreAcceptanceControlHandler 同一纪律（票 sa-cc/14）。
var ErrNilDependency = errors.New("customs compliance: duty payment reconciliation dependency is nil")

func NewDutyPaymentReconciliationHandler(deps DutyPaymentReconciliationDeps) (*DutyPaymentReconciliationHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"duty collaboration store", deps.Collaborations == nil},
		{"external funds fact register", deps.Funds == nil},
		{"duty verification store", deps.Verifications == nil},
		{"duty payment verification handoff", deps.Handoff == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &DutyPaymentReconciliationHandler{deps: deps}, nil
}

// FormCollaboration 形成协作事项（步 4–5）。领域两格的形状（核定格必带税费引用不带无需
// 付款依据、无需付款格反之）由 FormDutyCollaboration 把门；「哪一格都不是」是它的独立哨兵
// ErrCollaborationNotFundable，翻成业务未决而不是未受理——矛盾输入与「税费结果还没到」是
// 两回事，后者的续办是等 UC-CC-006 的核定税费或真实程序的无需付款依据。
func (handler *DutyPaymentReconciliationHandler) FormCollaboration(
	ctx context.Context,
	command FormDutyCollaborationCommand,
) (DutyReconciliationResult, error) {
	if blankTenant(command.TenantID) {
		return DutyReconciliationResult{outcome: DutyReconciliationNotAccepted}, nil
	}
	collaboration, err := domain.FormDutyCollaboration(domain.DutyCollaborationSpec{
		Kind:        command.Kind,
		Duty:        command.Duty,
		NoPayBasis:  command.NoPayBasis,
		Scope:       command.Scope,
		Obligor:     command.Obligor,
		Requirement: command.Requirement,
		Target:      command.Target,
		FormedAt:    handler.deps.Clock.Now(),
	})
	switch {
	case errors.Is(err, domain.ErrCollaborationNotFundable):
		return dutyUndecided(DutyObligationBasisAbsent), nil
	case err != nil:
		return DutyReconciliationResult{outcome: DutyReconciliationNotAccepted}, nil
	}

	saved, err := handler.deps.Collaborations.SaveCollaboration(ctx, command.TenantID, collaboration)
	if err != nil {
		return dutyUndecided(CollaborationStoreUnavailable), nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return DutyReconciliationResult{outcome: CollaborationFormed}, nil
	}

	duty, _ := collaboration.Duty()
	existing, found, err := handler.deps.Collaborations.FindCollaboration(ctx, command.TenantID, collaboration.Scope(), duty)
	if err != nil || !found {
		return dutyUndecided(CollaborationStoreUnavailable), nil
	}
	if !sameCollaboration(existing, collaboration) {
		return DutyReconciliationResult{outcome: CollaborationContentConflict}, nil
	}
	return DutyReconciliationResult{outcome: CollaborationExisting}, nil
}

// sameCollaboration 逐字段比两份协作事项，形成时间不比：重放时时钟已经走了，而重放比的是
// 内容不是时刻。
func sameCollaboration(existing, requested domain.DutyPaymentCollaboration) bool {
	existingDuty, _ := existing.Duty()
	requestedDuty, _ := requested.Duty()
	existingBasis, _ := existing.NoPayBasis()
	requestedBasis, _ := requested.NoPayBasis()
	return existing.Kind() == requested.Kind() &&
		existingDuty == requestedDuty &&
		existingBasis == requestedBasis &&
		existing.Scope() == requested.Scope() &&
		existing.Obligor() == requested.Obligor() &&
		existing.Requirement() == requested.Requirement() &&
		existing.Target() == requested.Target()
}

// ReceiveFundsFact 登记一条外部资金事实引用（步 6 的 CC 半边）。来源身份、付款人、币种、
// 业务时间必备——来源未提供且程序不要求的维度要「明确记录」，而这四件是关联核对最少要读
// 的；金额允许为零（付款失败、撤销这类来源事实本就没有正向金额），为负是矛盾输入。
func (handler *DutyPaymentReconciliationHandler) ReceiveFundsFact(
	ctx context.Context,
	command ReceiveExternalFundsFactCommand,
) (DutyReconciliationResult, error) {
	registration := command.Registration
	if blankTenant(command.TenantID) ||
		strings.TrimSpace(registration.Fact.String()) == "" ||
		strings.TrimSpace(registration.Source) == "" ||
		strings.TrimSpace(registration.Payer) == "" ||
		strings.TrimSpace(registration.Currency) == "" ||
		registration.AmountMinor < 0 ||
		registration.OccurredAt.IsZero() {
		return DutyReconciliationResult{outcome: DutyReconciliationNotAccepted}, nil
	}

	saved, err := handler.deps.Funds.RegisterFundsFact(ctx, command.TenantID, registration)
	if err != nil {
		return dutyUndecided(FundsFactRegisterUnavailable), nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return DutyReconciliationResult{outcome: FundsFactReceived}, nil
	}

	existing, found, err := handler.deps.Funds.LoadFundsFact(ctx, command.TenantID, registration.Fact)
	if err != nil || !found {
		return dutyUndecided(FundsFactRegisterUnavailable), nil
	}
	if existing.Source != registration.Source || existing.Payer != registration.Payer ||
		existing.Currency != registration.Currency || existing.AmountMinor != registration.AmountMinor ||
		!existing.OccurredAt.Equal(registration.OccurredAt) {
		return DutyReconciliationResult{outcome: FundsFactContentConflict}, nil
	}
	return DutyReconciliationResult{outcome: FundsFactExisting}, nil
}

// VerifyPayment 形成税费付款核对（步 7）。两道前置各有自己的格（资金事实未接收 / 协作事项
// 未形成），无关联依据保持待关联；三轴集外由领域构造拒。同三维同内容是重放，同三维换内容
// 是新版本追加——迟到事实按新版本进，不按到达顺序覆盖。
//
// 形成那一格同事务交结算意图（步 8）。`已存在`不重发，与本上下文其余编排「重放重发同一份」
// 不同形，理由在事务边界上：信封与核对版本由同一笔事务落下，库侧入队失败会让整笔事务连核对
// 一起中止，重跑仍走`形成`那一格并再铸同一封——重放时没有「版本在、信封不在」要补的那一格；
// 而每一版核对各有自己的信封（ID 含指纹），重放`已存在`再交一次只会被 EnqueueOnce 吞掉，
// 对下游是零信息。续办引用覆盖的是非库侧的交接失败（装配缺陷一类），那一格响亮而不是等重放。
func (handler *DutyPaymentReconciliationHandler) VerifyPayment(
	ctx context.Context,
	command VerifyDutyPaymentCommand,
) (DutyReconciliationResult, error) {
	if blankTenant(command.TenantID) ||
		strings.TrimSpace(command.Duty.String()) == "" ||
		strings.TrimSpace(command.Funds.String()) == "" ||
		strings.TrimSpace(command.Scope.String()) == "" ||
		command.Coverage.String() == "" ||
		command.Delta.String() == "" ||
		command.Validity.String() == "" {
		return DutyReconciliationResult{outcome: DutyReconciliationNotAccepted}, nil
	}
	if strings.TrimSpace(command.Basis) == "" {
		// 金额相等、同一范围、同一付款人都不单独构成权威关联（UC-CC-009 核对规则）；说不出
		// 依据就不关联，事实留在待关联。
		return DutyReconciliationResult{outcome: FundsFactPendingAssociation}, nil
	}

	if _, found, err := handler.deps.Funds.LoadFundsFact(ctx, command.TenantID, command.Funds); err != nil {
		return dutyUndecided(FundsFactRegisterUnavailable), nil
	} else if !found {
		return DutyReconciliationResult{outcome: FundsFactNotReceived}, nil
	}
	if _, found, err := handler.deps.Collaborations.FindCollaboration(
		ctx, command.TenantID, command.Scope, command.Duty); err != nil {
		return dutyUndecided(CollaborationStoreUnavailable), nil
	} else if !found {
		return DutyReconciliationResult{outcome: CollaborationNotFormed}, nil
	}

	verification, err := domain.VerifyDutyPayment(
		command.Duty, command.Funds, command.Scope,
		command.Coverage, command.Delta, command.Validity, handler.deps.Clock.Now())
	if err != nil {
		return DutyReconciliationResult{outcome: DutyReconciliationNotAccepted}, nil
	}
	record := ports.DutyVerificationRecord{
		Key: ports.DutyVerificationKey{
			TenantID: command.TenantID,
			Duty:     command.Duty,
			Funds:    command.Funds,
			Scope:    command.Scope,
			Digest:   verificationDigest(command),
		},
		Verification: verification,
		Basis:        command.Basis,
	}
	saved, err := handler.deps.Verifications.SaveVerification(ctx, record)
	if err != nil {
		return dutyUndecided(DutyVerificationStoreUnavailable), nil
	}
	if saved == ports.CaseConfigurationRegistered {
		result := DutyReconciliationResult{outcome: DutyVerificationFormed}
		result.handoffRef = handler.handOffVerification(ctx, record.Key, verification)
		return result, nil
	}
	// 指纹里已含三轴与依据：撞键即同内容，不必再读回比。
	return DutyReconciliationResult{outcome: DutyVerificationExisting}, nil
}

// handOffVerification 交结算意图。失败不翻核对，留续办引用指名哪一版没交出去。
func (handler *DutyPaymentReconciliationHandler) handOffVerification(
	ctx context.Context,
	key ports.DutyVerificationKey,
	verification domain.DutyPaymentVerification,
) string {
	if err := handler.deps.Handoff.HandOffDutyPaymentVerification(ctx, ports.DutyPaymentVerificationHandoffIntent{
		Key:          key,
		Verification: verification,
	}); err == nil {
		return ""
	}
	return "CONT-DUTY-VERIFICATION/" + key.Scope.String() + "/" + key.Digest[:8]
}

// verificationDigest 是核对内容的稳定指纹：三轴加关联依据。三维身份在键上，不进指纹。
func verificationDigest(command VerifyDutyPaymentCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		strconv.Itoa(int(command.Coverage)),
		strconv.Itoa(int(command.Delta)),
		strconv.Itoa(int(command.Validity)),
		strings.TrimSpace(command.Basis),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
