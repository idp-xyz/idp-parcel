package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// UC-CC-003 步 7「核验监管凭证……→ 记录凭证门禁」的记录半边（票 sa-cc/04）：判断半边
// JudgeCredentialApplicability 此前只判不记，本编排调它算出四格，再落成凭证门禁判断登记册的一版。
// 它是逐门禁判断的第一册（ADR-0137 决定一）：就绪判断按不可变引用绑定它、不内嵌其内部结构，
// RegisterReadiness 的形因此不动；后面的门照这一册的形各成一册。
//
// 判断由评估请求驱动（ADR-0137 决定二）：请求到达算一次登一次，凭证登记 / 程序变更不在这里触发
// 重算——后台重算把门禁翻成满足正是 UC-CC-003「原判断失效后不得因为……凭证恢复……自动恢复」
// 禁的事；来源变化的正当作用是让既有就绪判断`不再就绪`（RevokeReadiness 那条编排），不产出新门禁
// 判断。谁在什么时候再发一次评估请求，不是本编排的事。
//
// 只记门禁判断，不占用、不释放、不核销——那三件是凭证使用的生命周期（UC-CC-005 步 7/9、UC-CC-006
// 步 7），时点由真实程序定（PAR-CUS-04）；本编排对凭证册只读。
//
// 幂等键含内容指纹（ports.CredentialGateDigest），同请求重放撞键即`已存在`、不必读回比；换程序、
// 持有人、截至时点、依据或责任角色任一件即换指纹追加新版，不覆盖——判断是不可覆盖的版本，「不适用
// 之后凭证补齐再判适用」是两版并存，不是把前一版改掉。事务由进程级入口给出。

// CredentialGateRecordOutcome 是本编排的应用处理结果——说的是「这一版落成没有」，与判断结论
// 四格（domain.CredentialGateConclusion）是两轴：落成的一版可以是任一格结论。
type CredentialGateRecordOutcome uint8

const (
	CredentialGateRecordOutcomeInvalid CredentialGateRecordOutcome = iota
	CredentialGateRecorded
	CredentialGateExisting
	CredentialGateNotAccepted
	CredentialGateRecordUndecided
)

func (outcome CredentialGateRecordOutcome) String() string {
	switch outcome {
	case CredentialGateRecorded:
		return "RECORDED"
	case CredentialGateExisting:
		return "EXISTING"
	case CredentialGateNotAccepted:
		return "NOT_ACCEPTED"
	case CredentialGateRecordUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// CredentialGateResult 是一次记录请求的结果。Key 只在`已落成`/`已存在`两格非零：它就是就绪判断
// 要绑定的那把不可变引用，没落成的判断不能被当成依据。
type CredentialGateResult struct {
	outcome    CredentialGateRecordOutcome
	conclusion domain.CredentialGateConclusion
	key        ports.CredentialGateKey
}

func (result CredentialGateResult) Outcome() CredentialGateRecordOutcome {
	return result.outcome
}

// Conclusion 是落成那一版的结论四格之一；未受理与未决时为零值。
func (result CredentialGateResult) Conclusion() domain.CredentialGateConclusion {
	return result.conclusion
}

// Key 是落成（或重放撞上）那一版的幂等键。
func (result CredentialGateResult) Key() ports.CredentialGateKey {
	return result.key
}

// RecordCredentialGateCommand 携带一次评估请求上的凭证门禁判断请求：对哪个申报单元、哪张凭证、
// 在哪个程序、由谁、截至何时使用，凭什么依据，由哪个角色负责。AsOf 是拟使用的业务时点（凭证
// 有效期按它算），不是判断发生的系统时间。
type RecordCredentialGateCommand struct {
	TenantID   domain.TenantID
	Unit       domain.DeclarationUnitID
	Credential domain.CredentialID
	Procedure  domain.CustomsProcedureReference
	Holder     domain.CredentialHolderReference
	AsOf       time.Time
	Basis      domain.CredentialGateBasisReference
	Role       domain.ResponsibleRoleReference
}

// RecordCredentialGateDeps 是三口：凭证册的读半边（判断半边读它）、门禁判断登记册的写半边、时钟。
// 判断半边不作一口交进来而由构造门自己接上 JudgeCredentialApplicabilityHandler：它是 UC-CC-003 步 7
// 的判断半边，本编排是它的记录半边，两半合起来才是那一步——装配点要接的是「步 7」，不是两个各
// 自的口；替身也照旧落在凭证册读口上（判断口自己的用例就这么做），不必为本编排另铸一层。
type RecordCredentialGateDeps struct {
	Credentials ports.CredentialView
	Registry    ports.CredentialGateRegistry
	Clock       ports.Clock
}

type RecordCredentialGateHandler struct {
	deps  RecordCredentialGateDeps
	judge *JudgeCredentialApplicabilityHandler
}

// NewRecordCredentialGateHandler 构造门逐口拒 nil（形照 NewDutyPaymentReconciliationHandler；沿用
// 本包的 ErrNilDependency 作唯一答复，哪一口缺在包装信息里点名）。
func NewRecordCredentialGateHandler(deps RecordCredentialGateDeps) (*RecordCredentialGateHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"credential view", deps.Credentials == nil},
		{"credential gate registry", deps.Registry == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: credential gate %s", ErrNilDependency, dependency.name)
		}
	}
	return &RecordCredentialGateHandler{
		deps:  deps,
		judge: NewJudgeCredentialApplicabilityHandler(JudgeCredentialApplicabilityDeps{View: deps.Credentials}),
	}, nil
}

// Handle 判一次、登一版。受理门分两层：租户、单元、依据引用、责任角色本编排守；凭证身份、程序、
// 持有人、截至时点由判断口的受理门守，它答`未受理`这里同样答`未受理`——判不出的输入不该被翻成
// 任何一格结论，也不落册。
func (handler *RecordCredentialGateHandler) Handle(
	ctx context.Context,
	command RecordCredentialGateCommand,
) (CredentialGateResult, error) {
	if blankTenant(command.TenantID) || command.Unit.String() == "" ||
		command.Basis.String() == "" || command.Role.String() == "" {
		return CredentialGateResult{outcome: CredentialGateNotAccepted}, nil
	}

	applicability, err := handler.judge.Handle(ctx, JudgeCredentialApplicabilityCommand{
		TenantID:   command.TenantID,
		Credential: command.Credential,
		Procedure:  command.Procedure,
		Holder:     command.Holder,
		At:         command.AsOf,
	})
	if err != nil {
		// 判断口只在编程错误时带 error 返回（受理门之后领域仍答 ErrInvalidCredential），不折成任何格。
		return CredentialGateResult{}, err
	}
	conclusion, judged := gateConclusionOf(applicability)
	if !judged {
		return CredentialGateResult{outcome: CredentialGateNotAccepted}, nil
	}

	judgment, err := domain.RecordCredentialGate(domain.CredentialGateSpec{
		Unit:       command.Unit,
		Credential: command.Credential,
		Procedure:  command.Procedure,
		Holder:     command.Holder,
		AsOf:       command.AsOf,
		Conclusion: conclusion,
		Basis:      command.Basis,
		Role:       command.Role,
		JudgedAt:   handler.deps.Clock.Now(),
	})
	if err != nil {
		return CredentialGateResult{outcome: CredentialGateNotAccepted}, nil
	}

	record := ports.CredentialGateRecord{
		Key: ports.CredentialGateKey{
			TenantID:   command.TenantID,
			Unit:       command.Unit,
			Credential: command.Credential,
			Digest:     ports.CredentialGateDigest(judgment),
		},
		Judgment: judgment,
	}
	saved, err := handler.deps.Registry.RegisterCredentialGate(ctx, record)
	if err != nil {
		return CredentialGateResult{outcome: CredentialGateRecordUndecided}, nil
	}
	result := CredentialGateResult{outcome: CredentialGateExisting, conclusion: conclusion, key: record.Key}
	if saved == ports.CaseConfigurationRegistered {
		result.outcome = CredentialGateRecorded
	}
	// 指纹里已含全部内容：撞键即同内容，不必再读回比（判据同 VerifyPayment）。
	return result, nil
}

// gateConclusionOf 把判断口的应用结果翻成登记册的结论四格。`未受理`不是结论（judged=false）；
// 四格之外的值只可能是 CredentialApplicabilityOutcomeInvalid，而判断口在那一格必带 error，
// 调用方已在此之前返回——走到 default 是编程错误，同样按未判处理而不猜一格。
func gateConclusionOf(outcome CredentialApplicabilityOutcome) (domain.CredentialGateConclusion, bool) {
	switch outcome {
	case CredentialApplicable:
		return domain.CredentialGateApplicable, true
	case CredentialNotApplicable:
		return domain.CredentialGateNotApplicable, true
	case CredentialNotRegistered:
		return domain.CredentialGateCredentialNotRegistered, true
	case CredentialApplicabilityUndecided:
		return domain.CredentialGateUndecided, true
	default:
		return domain.CredentialGateConclusionInvalid, false
	}
}
