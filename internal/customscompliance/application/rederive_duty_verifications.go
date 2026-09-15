package application

import (
	"context"
	"strings"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// UC-CC-009 一致性节「外部资金事实迟到、更正、资金退回或付款撤销时，形成新核对版本并保留原覆盖判断；不删除原
// 付款、不按最后到达覆盖」在 CC 侧的入口（票 sa-cc/19 裁决 1 / 2）。它接在 ReceiveFundsFact 答`已接收`之后，由消费侧
// 适配器在同一事务里调（版本行、新核对版本、交接意图三者同生同灭）；消费侧适配器仍只译不判，判在这里。
//
// (a′) 的七样，每一样都写明从哪来、为什么不是编排猜的（票面红线「三轴不由编排猜」）：
//   - 覆盖轴承前版——UC 原句「保留原覆盖判断」，被保留的正是这一轴；
//   - 差额轴 PENDING——金额可能变了，DeltaPending 是既有格，写的是「未判」；
//   - 有效性轴 PENDING——前版所依的那一版事实已被新版本取代，FundsFactPending 是既有格，写的是「未判」；
//   - 依据承前版——「凭什么把这笔资金关联到这版税费」说的是事实身份与税费的关联，事实换版本不换身份；
//   - 程序承前版——付款人维按哪个程序的规则判是记录的依据维（票 sa-cc/22），不随事实版本变；
//   - 资金版本取新到的那一版——本票加的维；
//   - 核对时刻取编排的时钟——与 VerifyPayment 同源。
//
// 触发只看「该事实已有既往核对版本」、不看 corrects（裁决 2）：不带回指的新版本可以在既往核对之后落地，带回指的首版
// 也可能在任何核对之前到，两者互不蕴含。谱系按（税费、范围、程序）分、各取最近一版承前——同一事实可以被关联到多条
// 谱系，每条各成一版；「最近」按核对时刻、同一时刻按指纹字典序取定，与 CurrentDutyVerificationView 取「当前」同一把尺。
//
// 形成走既有 VerifyPayment：两道前置、付款人三停格、指纹、落册、交接一版一封都是同一条路——(a′) 与人重核的版本在册上
// 没有第二种形。付款人维因此按新版本的付款人 + 前版程序的规则重判：要求而新版本未提供 → 该谱系不形成、点名
// PayerRequiredNotProvided 交人（等来源补事实，不是依赖故障、不重投）——这是 (a′) 唯一不形成的分支；程序没登规则
// 同理点名 PayerRequirementNotConfigured。依赖故障（哪一口不可用）是整笔的未决：停下、指名、交消费门重投——同一事务里
// 已形成的谱系也会随之回滚，所以不把它们当成功报出去。
//
// 新版本成为 CurrentDutyVerificationView 的当前一版后，放行门禁那一道对 PENDING 答未决（gateUndecided(DutyVerificationPending)）
// 而不是未满足——事实变了、人没重核之前门禁不该放也不该判失败（裁决 1「取证量到的后果照单接受」）；人重核走 VerifyPayment
// 带新资金版本、断言三轴再成一版，门禁随之。

// RederiveDutyVerificationsCommand 携带「资金事实新版本到达」这一事实：租户、事实、新到的那一版。
type RederiveDutyVerificationsCommand struct {
	TenantID domain.TenantID
	Funds    domain.ExternalFundsFactReference
	Version  domain.FundsFactVersion
}

// DutyVerificationRederivationOutcome 是本编排的封闭结果：`已重派`（每条谱系各有答案，含零条谱系）、`未受理`
// （命令缺格，调用方编程错误）、`未决`（依赖故障，整笔重投）。谱系各自的业务答案在 Lineages 上，不折进这里。
type DutyVerificationRederivationOutcome uint8

const (
	DutyVerificationRederivationOutcomeInvalid DutyVerificationRederivationOutcome = iota
	DutyVerificationsRederived
	DutyVerificationRederivationNotAccepted
	DutyVerificationRederivationUndecided
)

func (outcome DutyVerificationRederivationOutcome) String() string {
	switch outcome {
	case DutyVerificationsRederived:
		return "DUTY_VERIFICATIONS_REDERIVED"
	case DutyVerificationRederivationNotAccepted:
		return "NOT_ACCEPTED"
	case DutyVerificationRederivationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// DutyVerificationRederivation 是一条既往核对谱系（税费、范围、程序）在这次重派上的答案：Result 是该谱系走
// VerifyPayment 的原答案（`核对已形成` / `已存在` / 业务未决点名 reason），Key 在形成或已存在时指向册上那一版。
type DutyVerificationRederivation struct {
	Duty      domain.AssessedDutyReference
	Scope     domain.DecisionScopeReference
	Procedure domain.CustomsProcedureReference
	Key       ports.DutyVerificationKey
	Result    DutyReconciliationResult
}

type DutyVerificationRederivationResult struct {
	outcome  DutyVerificationRederivationOutcome
	reason   DutyReconciliationReason
	lineages []DutyVerificationRederivation
}

func (result DutyVerificationRederivationResult) Outcome() DutyVerificationRederivationOutcome {
	return result.outcome
}

// UndecidedReason 只在整笔`未决`时非零——指名哪一口不可用。
func (result DutyVerificationRederivationResult) UndecidedReason() DutyReconciliationReason {
	return result.reason
}

// Lineages 是每条既往核对谱系各自的答案，按册上核对时刻先后首次出现的顺序排；整笔`未决`或`未受理`时为空。
func (result DutyVerificationRederivationResult) Lineages() []DutyVerificationRederivation {
	return append([]DutyVerificationRederivation(nil), result.lineages...)
}

// RederiveDutyVerificationsOnFundsFactVersion 对一条事实的每条既往核对谱系各形成一版 (a′)（文件头注）。
func (handler *DutyPaymentReconciliationHandler) RederiveDutyVerificationsOnFundsFactVersion(
	ctx context.Context,
	command RederiveDutyVerificationsCommand,
) (DutyVerificationRederivationResult, error) {
	if blankTenant(command.TenantID) ||
		strings.TrimSpace(command.Funds.String()) == "" ||
		strings.TrimSpace(command.Version.String()) == "" {
		return DutyVerificationRederivationResult{outcome: DutyVerificationRederivationNotAccepted}, nil
	}

	existing, err := handler.deps.Verifications.ListVerificationsByFundsFact(ctx, command.TenantID, command.Funds)
	if err != nil {
		return rederivationUndecided(DutyVerificationStoreUnavailable), nil
	}

	result := DutyVerificationRederivationResult{outcome: DutyVerificationsRederived}
	for _, latest := range latestVerificationPerLineage(existing) {
		lineage := DutyVerificationRederivation{
			Duty:      latest.Key.Duty,
			Scope:     latest.Key.Scope,
			Procedure: latest.Verification.Procedure(),
		}
		derived := VerifyDutyPaymentCommand{
			TenantID:     command.TenantID,
			Duty:         latest.Key.Duty,
			Funds:        command.Funds,
			FundsVersion: command.Version,
			Scope:        latest.Key.Scope,
			Procedure:    latest.Verification.Procedure(),
			Coverage:     latest.Verification.Coverage(),
			Delta:        domain.DeltaPending,
			Validity:     domain.FundsFactPending,
			Basis:        latest.Basis,
		}
		lineage.Result, err = handler.VerifyPayment(ctx, derived)
		if err != nil {
			return DutyVerificationRederivationResult{}, err
		}
		if lineage.Result.Outcome() == DutyReconciliationUndecided && lineage.Result.UndecidedReason().dependencyFailure() {
			return rederivationUndecided(lineage.Result.UndecidedReason()), nil
		}
		if lineage.Result.Outcome() == DutyVerificationFormed || lineage.Result.Outcome() == DutyVerificationExisting {
			lineage.Key = ports.DutyVerificationKey{
				TenantID: command.TenantID,
				Duty:     derived.Duty,
				Funds:    derived.Funds,
				Scope:    derived.Scope,
				Digest:   verificationDigest(derived),
			}
		}
		result.lineages = append(result.lineages, lineage)
	}
	return result, nil
}

func rederivationUndecided(reason DutyReconciliationReason) DutyVerificationRederivationResult {
	return DutyVerificationRederivationResult{outcome: DutyVerificationRederivationUndecided, reason: reason}
}

// lineageKey 是谱系的身份：税费、范围、程序——同一事实上这三样相同的核对版本是同一条判断的历次说法。
type lineageKey struct {
	duty      domain.AssessedDutyReference
	scope     domain.DecisionScopeReference
	procedure domain.CustomsProcedureReference
}

// latestVerificationPerLineage 从一条事实的全部核对版本里按谱系各取最近一版，按谱系首次出现的顺序交回。输入按
// 核对时刻升序、同一时刻按指纹字典序（DutyVerificationStore.ListVerificationsByFundsFact 的口径），所以只在时刻
// **严格更晚**时换人——同一时刻并存的两版留下指纹字典序小的那版，与 LoadCurrentDutyVerification 取「当前」一致。
func latestVerificationPerLineage(records []ports.DutyVerificationRecord) []ports.DutyVerificationRecord {
	var (
		order  []lineageKey
		latest = map[lineageKey]ports.DutyVerificationRecord{}
	)
	for _, record := range records {
		key := lineageKey{duty: record.Key.Duty, scope: record.Key.Scope, procedure: record.Verification.Procedure()}
		current, seen := latest[key]
		if !seen {
			order = append(order, key)
			latest[key] = record
			continue
		}
		if record.Verification.VerifiedAt().After(current.Verification.VerifiedAt()) {
			latest[key] = record
		}
	}
	picked := make([]ports.DutyVerificationRecord, 0, len(order))
	for _, key := range order {
		picked = append(picked, latest[key])
	}
	return picked
}
