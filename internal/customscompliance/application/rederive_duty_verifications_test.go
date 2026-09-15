package application_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 票 sa-cc/19 裁决 1 / 2 的行为面：资金事实新版本到达 → 对该事实每一条既往核对谱系（税费、范围、程序）各形成
// 一版 (a′)——覆盖轴承前版、差额 / 有效性显式 PENDING、依据 / 程序承前版、资金版本取新到那一版；前版一字不动；
// 无既往核对 → 什么都不做、不报错；付款人维要求而新版本未提供 → 该谱系不形成、点名未决 reason（唯一不形成的
// 分支）；依赖故障 → 整笔未决交消费门重投。替身照真库代数。

// verifiedLineage 铺好一条谱系：协作事项 + v1 已接收 + 按 v1 形成一版核对（三轴、依据、程序由 verifyDutyCommand 给）。
func verifiedLineage(t *testing.T, store *dutyStoreDouble) (*application.DutyPaymentReconciliationHandler, application.ReceiveExternalFundsFactCommand, ports.DutyVerificationKey) {
	t.Helper()
	handler := newDutyHandler(t, store)
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}
	first := fundsFactCommand(t)
	if _, err := handler.ReceiveFundsFact(t.Context(), first); err != nil {
		t.Fatalf("资金事实 v1：%v", err)
	}
	if result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t)); err != nil ||
		result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("按 v1 核对：err=%v outcome=%v", err, result.Outcome())
	}
	return handler, first, store.handoffs[0].Key
}

func rederiveCommand(t *testing.T, received application.ReceiveExternalFundsFactCommand) application.RederiveDutyVerificationsCommand {
	t.Helper()
	return application.RederiveDutyVerificationsCommand{
		TenantID: received.TenantID,
		Funds:    received.Registration.Fact,
		Version:  received.Registration.Version,
	}
}

// Covers: 完成判据 (1) 第一句——v1 已核对（谱系 A），v2 到达 → 谱系 A 形成新版本：覆盖承前、差额 / 有效性 PENDING、
// 依据 / 程序承前、资金版本 = v2、核对时刻取时钟；前版内容零 diff、`FindVerification(前版键)` 仍命中；新版本走 05 的
// 交接一版一封，意图认领的正是新落册那一版。
func TestANewFundsFactVersionFormsAPendingVerificationVersionOnEachLineage(t *testing.T) {
	store := newDutyStore()
	handler, first, previousKey := verifiedLineage(t, store)
	previous := store.verifications[verificationKey(previousKey)]
	second := correctionOf(t, first, "SYN-FUNDS-01/v2", 9000)
	if _, err := handler.ReceiveFundsFact(t.Context(), second); err != nil {
		t.Fatalf("资金事实 v2：%v", err)
	}

	result, err := handler.RederiveDutyVerificationsOnFundsFactVersion(t.Context(), rederiveCommand(t, second))
	if err != nil || result.Outcome() != application.DutyVerificationsRederived {
		t.Fatalf("重派：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	lineages := result.Lineages()
	if len(lineages) != 1 || lineages[0].Result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("谱系 A 该形成一版：%+v", lineages)
	}
	if len(store.verifications) != 2 || len(store.handoffs) != 2 {
		t.Fatalf("该多一行、多一封：%d 行 %d 封", len(store.verifications), len(store.handoffs))
	}
	formed, found := store.verifications[verificationKey(lineages[0].Key)]
	if !found {
		t.Fatalf("谱系答案指的键不在册上：%+v", lineages[0].Key)
	}
	verification := formed.Verification
	if verification.FundsVersion() != second.Registration.Version ||
		verification.Coverage() != previous.Verification.Coverage() ||
		verification.Delta() != domain.DeltaPending ||
		verification.Validity() != domain.FundsFactPending ||
		verification.Procedure() != previous.Verification.Procedure() ||
		formed.Basis != previous.Basis ||
		!verification.VerifiedAt().Equal(dutyBaseAt) ||
		formed.Key.Duty != previousKey.Duty || formed.Key.Scope != previousKey.Scope || formed.Key.Funds != previousKey.Funds {
		t.Fatalf("(a′) 七样走样：%+v", formed)
	}
	if formed.Key.Digest == previousKey.Digest {
		t.Fatal("新版本与前版撞了指纹——资金版本 / 两轴 PENDING 没折进指纹")
	}
	if store.handoffs[1].Key != formed.Key {
		t.Fatalf("新版本该走交接一版一封、认领新落册那一版：%+v", store.handoffs[1].Key)
	}

	again, found, err := store.FindVerification(t.Context(), previousKey)
	if err != nil || !found || again.Verification != previous.Verification || again.Basis != previous.Basis {
		t.Fatalf("前版该一字不动且仍命中：found=%v err=%v %+v", found, err, again)
	}
}

// Covers: 完成判据 (1)「两条谱系各成一版」——同一事实关联到两条谱系（换税费与范围），v2 到达各形成一版；谱系按
// （税费、范围、程序）分、取各组最近一版承前：谱系 A 在 v1 上改判过一次（覆盖 NONE、时刻更晚），新版本承的是改判
// 后那一版，不是首版。
func TestEveryLineageOfTheFactGetsItsOwnNewVersionCarryingItsLatestCoverage(t *testing.T) {
	store := newDutyStore()
	handler, first, _ := verifiedLineage(t, store)
	revised := verifyDutyCommand(t)
	revised.Coverage = domain.CoverageNone
	revised.Basis = "SYN-BANK-01: remittance reversed"
	// 改判晚一小时落——「取各组最近一版」按核对时刻取，同一替身、只换时钟。
	laterDeps := fullDutyDeps(store)
	laterDeps.Clock = dutyClock{at: dutyBaseAt.Add(time.Hour)}
	laterHandler, err := application.NewDutyPaymentReconciliationHandler(laterDeps)
	if err != nil {
		t.Fatalf("构造晚一小时的编排：%v", err)
	}
	if result, err := laterHandler.VerifyPayment(t.Context(), revised); err != nil || result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("谱系 A 改判：err=%v outcome=%v", err, result.Outcome())
	}
	other := assessedCollaborationCommand(t)
	other.Duty = configValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-02/v1")
	other.Scope = configValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-02")
	if _, err := handler.FormCollaboration(t.Context(), other); err != nil {
		t.Fatalf("谱系 B 协作事项：%v", err)
	}
	onOther := verifyDutyCommand(t)
	onOther.Duty, onOther.Scope = other.Duty, other.Scope
	onOther.Coverage = domain.CoveragePartial
	if result, err := handler.VerifyPayment(t.Context(), onOther); err != nil || result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("谱系 B 核对：err=%v outcome=%v", err, result.Outcome())
	}
	second := correctionOf(t, first, "SYN-FUNDS-01/v2", 9000)
	if _, err := handler.ReceiveFundsFact(t.Context(), second); err != nil {
		t.Fatalf("资金事实 v2：%v", err)
	}
	before := len(store.verifications)

	result, err := handler.RederiveDutyVerificationsOnFundsFactVersion(t.Context(), rederiveCommand(t, second))
	if err != nil || result.Outcome() != application.DutyVerificationsRederived || len(result.Lineages()) != 2 {
		t.Fatalf("两条谱系各成一版：err=%v outcome=%v n=%d", err, result.Outcome(), len(result.Lineages()))
	}
	if len(store.verifications) != before+2 {
		t.Fatalf("该多两行：%d → %d", before, len(store.verifications))
	}
	coverageByScope := map[domain.DecisionScopeReference]domain.DutyCoverage{}
	for _, lineage := range result.Lineages() {
		if lineage.Result.Outcome() != application.DutyVerificationFormed {
			t.Fatalf("谱系 %s 没形成：%v", lineage.Scope, lineage.Result.Outcome())
		}
		record := store.verifications[verificationKey(lineage.Key)]
		if record.Verification.FundsVersion() != second.Registration.Version || record.Verification.Delta() != domain.DeltaPending {
			t.Fatalf("谱系 %s 的新版本走样：%+v", lineage.Scope, record)
		}
		coverageByScope[lineage.Scope] = record.Verification.Coverage()
	}
	if coverageByScope[revised.Scope] != domain.CoverageNone || coverageByScope[other.Scope] != domain.CoveragePartial {
		t.Fatalf("覆盖轴该各承自己谱系最近一版：%v", coverageByScope)
	}
}

// Covers: 完成判据 (1)「无既往核对的事实新版本到达 → 不形成、不报错」——从没被核对过的事实换了版本，编排答
// 零条谱系、册上不动、不交信封。
func TestAFactThatWasNeverVerifiedGetsNothingRederived(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)
	first := fundsFactCommand(t)
	if _, err := handler.ReceiveFundsFact(t.Context(), first); err != nil {
		t.Fatalf("资金事实 v1：%v", err)
	}
	second := correctionOf(t, first, "SYN-FUNDS-01/v2", 9000)
	if _, err := handler.ReceiveFundsFact(t.Context(), second); err != nil {
		t.Fatalf("资金事实 v2：%v", err)
	}

	result, err := handler.RederiveDutyVerificationsOnFundsFactVersion(t.Context(), rederiveCommand(t, second))
	if err != nil || result.Outcome() != application.DutyVerificationsRederived || len(result.Lineages()) != 0 {
		t.Fatalf("无既往核对该什么都不做、不报错：err=%v outcome=%v n=%d", err, result.Outcome(), len(result.Lineages()))
	}
	if len(store.verifications) != 0 || len(store.handoffs) != 0 {
		t.Fatalf("册上不该动：%d 行 %d 封", len(store.verifications), len(store.handoffs))
	}
}

// Covers: 裁决 3 (2) 唯一不形成的分支——付款人维按新版本的付款人 + 前版程序的规则重判：程序要求付款人而 v2 的来源
// 没给 → 该谱系不形成新版本、点名 PayerRequiredNotProvided（等来源补事实，不是依赖故障、不重投）；前版仍在、不交
// 新信封；整笔仍答`已重派`——别的谱系不受它牵连。
func TestALineageWhoseNewVersionLacksARequiredPayerStaysPendingByName(t *testing.T) {
	store := newDutyStore()
	handler, first, previousKey := verifiedLineage(t, store)
	second := correctionOf(t, first, "SYN-FUNDS-01/v2", 9000)
	second.Registration.Payer = domain.FundsPayerNotProvided()
	if _, err := handler.ReceiveFundsFact(t.Context(), second); err != nil {
		t.Fatalf("资金事实 v2（未提供付款人）：%v", err)
	}

	result, err := handler.RederiveDutyVerificationsOnFundsFactVersion(t.Context(), rederiveCommand(t, second))
	if err != nil || result.Outcome() != application.DutyVerificationsRederived {
		t.Fatalf("业务未决不是依赖故障，整笔该`已重派`：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	lineages := result.Lineages()
	if len(lineages) != 1 || lineages[0].Result.Outcome() != application.DutyReconciliationUndecided ||
		lineages[0].Result.UndecidedReason() != application.PayerRequiredNotProvided ||
		lineages[0].Duty != previousKey.Duty || lineages[0].Scope != previousKey.Scope {
		t.Fatalf("该谱系该点名缺付款人：%+v", lineages)
	}
	if len(store.verifications) != 1 || len(store.handoffs) != 1 {
		t.Fatalf("不形成就不落行、不交封：%d 行 %d 封", len(store.verifications), len(store.handoffs))
	}
}

// Covers: 依赖故障是整笔的未决（ADR-0029：重投会变）——核对册列不出既往版本、或形成某条谱系时哪一口不可用，
// 编排停下并指名那一口，交消费门连同接收一起回滚重投；不把已形成的谱系当成功报出去（同一事务里它们也会回滚）。
// 命令缺格是调用方编程错误，`未受理`。
func TestARederivationStopsOnDependencyFailureAndRefusesBlankCommands(t *testing.T) {
	store := newDutyStore()
	handler, first, _ := verifiedLineage(t, store)
	second := correctionOf(t, first, "SYN-FUNDS-01/v2", 9000)
	if _, err := handler.ReceiveFundsFact(t.Context(), second); err != nil {
		t.Fatalf("资金事实 v2：%v", err)
	}

	store.verificationErr = errors.New("verification store down")
	result, err := handler.RederiveDutyVerificationsOnFundsFactVersion(t.Context(), rederiveCommand(t, second))
	if err != nil || result.Outcome() != application.DutyVerificationRederivationUndecided ||
		result.UndecidedReason() != application.DutyVerificationStoreUnavailable {
		t.Fatalf("核对册故障该整笔未决并指名：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	store.verificationErr = nil
	store.payerRuleErr = errors.New("payer rule view down")
	result, err = handler.RederiveDutyVerificationsOnFundsFactVersion(t.Context(), rederiveCommand(t, second))
	if err != nil || result.Outcome() != application.DutyVerificationRederivationUndecided ||
		result.UndecidedReason() != application.PayerRequirementViewUnavailable {
		t.Fatalf("规则读口故障该整笔未决并指名：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	store.payerRuleErr = nil
	if len(store.verifications) != 1 {
		t.Fatalf("未决不得落行：%d", len(store.verifications))
	}

	blank := rederiveCommand(t, second)
	blank.Version = domain.FundsFactVersion{}
	if result, err := handler.RederiveDutyVerificationsOnFundsFactVersion(t.Context(), blank); err != nil ||
		result.Outcome() != application.DutyVerificationRederivationNotAccepted {
		t.Fatalf("不说新到哪一版该`未受理`：err=%v outcome=%v", err, result.Outcome())
	}
}

// Covers: 票 sa-cc/29 裁决 2 (a)——重派路上交接口答「依赖不可用」（Outbox 存储错、事务错）不再折成无人读的续办引用：
// 整笔`未决`、点名 DutyVerificationHandoffUnavailable，与其余几口不可用同格，消费门连同接收一起回滚重投。
// 谱系答案不报出去：同一事务里已落的核对行会随之回滚，报成功就是撒谎。
func TestARederivationTreatsAnUnavailableHandoffAsUndecided(t *testing.T) {
	store := newDutyStore()
	handler, first, _ := verifiedLineage(t, store)
	second := correctionOf(t, first, "SYN-FUNDS-01/v2", 9000)
	if _, err := handler.ReceiveFundsFact(t.Context(), second); err != nil {
		t.Fatalf("资金事实 v2：%v", err)
	}

	store.handoffErr = errors.New("outbox unavailable")
	result, err := handler.RederiveDutyVerificationsOnFundsFactVersion(t.Context(), rederiveCommand(t, second))
	if err != nil || result.Outcome() != application.DutyVerificationRederivationUndecided ||
		result.UndecidedReason() != application.DutyVerificationHandoffUnavailable {
		t.Fatalf("交接口不可用该整笔未决并点名：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	if len(result.Lineages()) != 0 {
		t.Fatalf("未决不得把谱系当成功报出去：%+v", result.Lineages())
	}
}

// Covers: 票 sa-cc/29 裁决 2 (b)——交接口把信封被框架确定性校验拒收（ports.ErrHandoffEnvelopeRejected）交出来时，
// 重投同一份永远同一个结果，未决之名只会耗尽失败预算；编排以 ErrDutyVerificationHandoffRejected 响亮报错、整笔回滚，
// 不给结果、不给续办引用，留给人动手。
func TestARederivationFailsLoudlyWhenTheHandoffEnvelopeIsRejected(t *testing.T) {
	store := newDutyStore()
	handler, first, _ := verifiedLineage(t, store)
	second := correctionOf(t, first, "SYN-FUNDS-01/v2", 9000)
	if _, err := handler.ReceiveFundsFact(t.Context(), second); err != nil {
		t.Fatalf("资金事实 v2：%v", err)
	}

	store.handoffErr = fmt.Errorf("%w: id exceeds 128 bytes", ports.ErrHandoffEnvelopeRejected)
	result, err := handler.RederiveDutyVerificationsOnFundsFactVersion(t.Context(), rederiveCommand(t, second))
	if !errors.Is(err, application.ErrDutyVerificationHandoffRejected) || !errors.Is(err, ports.ErrHandoffEnvelopeRejected) {
		t.Fatalf("信封被拒该是硬失败且带着原因：err=%v", err)
	}
	if result.Outcome() != application.DutyVerificationRederivationOutcomeInvalid || len(result.Lineages()) != 0 {
		t.Fatalf("硬失败不该交回任何结果：%+v", result)
	}
}
