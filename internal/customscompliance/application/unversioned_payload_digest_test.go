package application_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// unversionedDigestCase 是一口在 CCC-1 之前入库的一条记录。first 首次提交并交回替身里那条记录的
// 摘要与改写它的办法（键上带指纹的口，改写是把那一行挪到无版本键下）；随后把摘要改写成
// unversioned，再走 replay（同内容，必须认作那一条）与 changed（同身份异内容，必须不是重放）。
// 三步共用一个替身。
//
// unversioned 是基 1121ba61 上的旧代码对 first 那条命令落下的摘要，写成定值而不在这里现算：
// 拿恢复后的函数现算，恢复得不一致也照样绿。
type unversionedDigestCase struct {
	name        string
	unversioned string
	first       func(t *testing.T) (string, func(string))
	replay      func(t *testing.T)
	changed     func(t *testing.T)
}

// onlyDigest 交回替身里唯一那条记录的摘要，与把它改写成别的串的办法。
func onlyDigest[K comparable, V any](t *testing.T, records map[K]V, digest func(*V) *string) (string, func(string)) {
	t.Helper()
	if len(records) != 1 {
		t.Fatalf("替身里有 %d 条记录，want 1", len(records))
	}
	for key, record := range records {
		return *digest(&record), func(rewritten string) {
			*digest(&record) = rewritten
			records[key] = record
		}
	}
	return "", nil
}

// movedDutyVerification 交回 key 那一行核对的指纹，与把那一行挪到另一枚指纹下的办法。
func movedDutyVerification(t *testing.T, store *dutyStoreDouble, key ports.DutyVerificationKey) (string, func(string)) {
	t.Helper()
	record, found := store.verifications[verificationKey(key)]
	if !found {
		t.Fatalf("册上没有 %+v 那一行", key)
	}
	return key.Digest, func(rewritten string) {
		delete(store.verifications, verificationKey(key))
		record.Key.Digest = rewritten
		store.verifications[verificationKey(record.Key)] = record
	}
}

func wantOutcome[O comparable](t *testing.T, got O, err error, want O) {
	t.Helper()
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got != want {
		t.Fatalf("outcome = %v, want %v", got, want)
	}
}

func unversionedDigestCases(t *testing.T) []unversionedDigestCase {
	t.Helper()
	return []unversionedDigestCase{
		externalResultUnversionedCase(t),
		releaseResultUnversionedCase(t),
		declarationUnversionedCase(t),
		correctionUnversionedCase(t),
		dutyVerificationUnversionedCase(t),
		dutyRederivationUnversionedCase(t),
		dispositionVerificationUnversionedCase(t),
	}
}

// Covers: ADR-0014「规范化版本不同不是冲突」——CCC-1 之前入库的记录存的是无版本摘要，同一条命令
// 重放按无版本那一版重算再比，认作原记录；同身份换内容仍不是重放。键上带指纹的口（税费付款核对、
// 处置执行核对）另证不因 CCC-1 键撞不上而再落一行、再交一份意图。
func TestRecordsStoredBeforeCCC1AreComparedUnderTheirOwnShape(t *testing.T) {
	for _, testCase := range unversionedDigestCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			_, rewrite := testCase.first(t)
			rewrite(testCase.unversioned)
			testCase.replay(t)
			testCase.changed(t)
		})
	}
}

// Covers: 新记录写的是 CCC-1 形——无版本那一版只用来比旧记录、拼旧行的键，误写进新记录时上一例
// 照样绿，要在这里断。键上带指纹的口断的是键上那枚指纹。
func TestNewRecordsAreStoredUnderCCC1(t *testing.T) {
	for _, testCase := range unversionedDigestCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			if stored, _ := testCase.first(t); !strings.HasPrefix(stored, "CCC-1:") {
				t.Fatalf("新记录落下的摘要是 %q，want CCC-1 形", stored)
			}
		})
	}
}

// Covers: 续办引用截的是指纹本身，不是形状前缀——CCC-1 新键上是前缀之后的 8 位；无版本旧键上与改动前
// 同一串（定值取基 1121ba61 上那一版指纹的前 8 位）。
func TestAContinuationReferenceNamesTheFingerprintNotItsShape(t *testing.T) {
	fixture := newVerifyFixture(t)
	fixture.facts.facts = []domain.ExecutionFact{executionFact(t, "DESTRUCTION-EXEC/1", 1)}
	fixture.downstream.err = errors.New("downstream unreachable")

	formed, err := fixture.handler.Handle(context.Background(), verifyCommand(t))
	wantOutcome(t, formed.Outcome(), err, application.VerificationRecorded)
	if !regexp.MustCompile(`^CONT-VERIFICATION/decision-1/[0-9a-f]{8}$`).MatchString(formed.HandoffReference()) {
		t.Fatalf("CCC-1 键上的续办引用 = %q：末段该是 8 位指纹", formed.HandoffReference())
	}

	if len(fixture.store.byKey) != 1 {
		t.Fatalf("册上有 %d 版核对，want 1", len(fixture.store.byKey))
	}
	for key, verification := range fixture.store.byKey {
		delete(fixture.store.byKey, key)
		key.Digest = "9de48e942298c286fe69b31c1c06cdc9ecbc58c433a69f07eb6fbce9c41cfb3f"
		fixture.store.byKey[key] = verification
		break
	}
	replay, err := fixture.handler.Handle(context.Background(), verifyCommand(t))
	wantOutcome(t, replay.Outcome(), err, application.VerificationExistingResult)
	if replay.HandoffReference() != "CONT-VERIFICATION/decision-1/9de48e94" {
		t.Fatalf("无版本旧键上的续办引用 = %q，want 与改动前同一串", replay.HandoffReference())
	}
}

// Covers: 认不出的形状版本不答冲突——按该处读失败的既有答复作答（外部结果口是存储未决）。
func TestAnUnknownStoredShapeIsUndecidedNotAConflict(t *testing.T) {
	fixture := newResultFixture(t)
	command := resultCommand(t, "source-unknown-shape")
	first, err := fixture.handler.Handle(context.Background(), command)
	wantOutcome(t, first.Outcome(), err, application.ResultRecorded)
	_, rewrite := onlyDigest(t, fixture.store.records,
		func(record *ports.ExternalResultRecord) *string { return &record.ContentDigest })
	rewrite("CCC-9:0000000000000000000000000000000000000000000000000000000000000000")

	result, err := fixture.handler.Handle(context.Background(), command)
	wantOutcome(t, result.Outcome(), err, application.ResultUndecided)
	if result.UndecidedReason() != application.ResultStoreUnavailable {
		t.Fatalf("reason = %q", result.UndecidedReason())
	}
}

func externalResultUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newResultFixture(t)
	command := resultCommand(t, "source-legacy-1")
	handle := func(t *testing.T, command application.ReceiveExternalResultCommand, want application.ExternalResultOutcome) {
		t.Helper()
		result, err := fixture.handler.Handle(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "RECEIVE_EXTERNAL_RESULT",
		unversioned: "8b3d68428732b0ee6a42f6945105f416f0996e795beccad3cd2e4ea6c5b87511",
		first: func(t *testing.T) (string, func(string)) {
			handle(t, command, application.ResultRecorded)
			return onlyDigest(t, fixture.store.records,
				func(record *ports.ExternalResultRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { handle(t, command, application.ResultExistingResult) },
		changed: func(t *testing.T) {
			changed := resultCommand(t, "source-legacy-1")
			changed.RawSemantics = "REJECTED"
			handle(t, changed, application.ResultSourceConflict)
		},
	}
}

// releaseResultUnversionedCase 走无版本那一版里放行三件进指纹的那一支。
func releaseResultUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newResultFixture(t)
	command := releaseCommand(t, "source-legacy-release", domain.ConditionalRelease, "re-export within 30 days")
	handle := func(t *testing.T, command application.ReceiveExternalResultCommand, want application.ExternalResultOutcome) {
		t.Helper()
		result, err := fixture.handler.Handle(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "RECEIVE_EXTERNAL_RESULT/RELEASE",
		unversioned: "330d1ccd2d5ea402b2a1102b46a6f557d4dc6e3e5616b2cc284ba8b34b6de5e7",
		first: func(t *testing.T) (string, func(string)) {
			handle(t, command, application.ResultRecorded)
			return onlyDigest(t, fixture.store.records,
				func(record *ports.ExternalResultRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { handle(t, command, application.ResultExistingResult) },
		changed: func(t *testing.T) {
			handle(t, releaseCommand(t, "source-legacy-release", domain.FullRelease, ""), application.ResultSourceConflict)
		},
	}
}

func declarationUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newDeclarationFixture(t)
	command := declarationCommand(t)
	handle := func(t *testing.T, command application.SubmitDeclarationCommand, want application.DeclarationOutcome) {
		t.Helper()
		result, err := fixture.handler.Handle(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "SUBMIT_DECLARATION",
		unversioned: "d177312d37604981e5fbb4929d52be06a65ebafa54c444f207e4e48a1bb338b0",
		first: func(t *testing.T) (string, func(string)) {
			handle(t, command, application.DeclarationSubmitted)
			return onlyDigest(t, fixture.store.records,
				func(record *ports.DeclarationSubmissionRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			handle(t, command, application.DeclarationExistingVersion)
			if fixture.versions.minted != 1 {
				t.Fatalf("minted = %d；重放不得再固定一版", fixture.versions.minted)
			}
		},
		changed: func(t *testing.T) {
			changed := declarationCommand(t)
			changed.Dossier = "dossier-snapshot-9"
			handle(t, changed, application.DeclarationSourceConflict)
		},
	}
}

// correctionUnversionedCase 比的是当前版：同一份更正再来一次，当前版就是它落下的那一版。异内容不是
// 重放，走更正那条路——目标键着前一版，答`更正无据`，不再形成新版本。
func correctionUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newCorrectionFixture(t)
	command := correctionCommand(t)
	handle := func(t *testing.T, command application.CorrectDeclarationCommand, want application.DeclarationOutcome) {
		t.Helper()
		result, err := fixture.handler.Handle(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "CORRECT_DECLARATION",
		unversioned: "e7ccc487e1933e6a14d8a1a28c47361c75ddd0af55b5561079cc24dc73cfdcb8",
		first: func(t *testing.T) (string, func(string)) {
			fixture.formTargetOnCurrent(t, domain.InCaseCorrection, "regulatory-request/RR-9", "submission/v1")
			handle(t, command, application.DeclarationCorrected)
			return onlyDigest(t, fixture.declaration.store.records,
				func(record *ports.DeclarationSubmissionRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			handle(t, command, application.DeclarationExistingVersion)
			if fixture.declaration.versions.minted != 2 {
				t.Fatalf("minted = %d；同一份更正的重放落成了又一个新版本", fixture.declaration.versions.minted)
			}
		},
		changed: func(t *testing.T) {
			changed := correctionCommand(t)
			changed.Dossier = "dossier-snapshot-3"
			handle(t, changed, application.DeclarationCorrectionUnbased)
			if fixture.declaration.versions.minted != 2 {
				t.Fatalf("minted = %d", fixture.declaration.versions.minted)
			}
		},
	}
}

// dutyVerificationUnversionedCase：同三维同内容认作旧行，不再落一版、不再铸信封；换内容是新版本追加。
func dutyVerificationUnversionedCase(t *testing.T) unversionedDigestCase {
	store := newDutyStore()
	handler := newDutyHandler(t, store)
	verify := func(t *testing.T, command application.VerifyDutyPaymentCommand, want application.DutyReconciliationOutcome) {
		t.Helper()
		result, err := handler.VerifyPayment(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "VERIFY_DUTY_PAYMENT",
		unversioned: "cf2fd69973c5b058d2d9d0b185decf914d1ffa263de767aaef930d8c3d085027",
		first: func(t *testing.T) (string, func(string)) {
			if _, err := handler.FormCollaboration(context.Background(), assessedCollaborationCommand(t)); err != nil {
				t.Fatalf("协作事项：%v", err)
			}
			if _, err := handler.ReceiveFundsFact(context.Background(), fundsFactCommand(t)); err != nil {
				t.Fatalf("资金事实：%v", err)
			}
			verify(t, verifyDutyCommand(t), application.DutyVerificationFormed)
			return movedDutyVerification(t, store, store.handoffs[0].Key)
		},
		replay: func(t *testing.T) {
			verify(t, verifyDutyCommand(t), application.DutyVerificationExisting)
			if len(store.verifications) != 1 || len(store.handoffs) != 1 {
				t.Fatalf("同内容重核落了 %d 行、交了 %d 封", len(store.verifications), len(store.handoffs))
			}
		},
		changed: func(t *testing.T) {
			changed := verifyDutyCommand(t)
			changed.Coverage = domain.CoveragePartial
			verify(t, changed, application.DutyVerificationFormed)
			if len(store.verifications) != 2 {
				t.Fatalf("换内容该追加一版：%d 行", len(store.verifications))
			}
		},
	}
}

// dutyRederivationUnversionedCase：同一事实版本再重派一次，派生出的 (a′) 就是上一次落下的那一行；
// 那一行是 CCC-1 之前入册的，谱系答案指回它的无版本键。新的事实版本到达仍形成新一版。
func dutyRederivationUnversionedCase(t *testing.T) unversionedDigestCase {
	store := newDutyStore()
	var handler *application.DutyPaymentReconciliationHandler
	var second application.ReceiveExternalFundsFactCommand
	rederive := func(t *testing.T, received application.ReceiveExternalFundsFactCommand) application.DutyVerificationRederivation {
		t.Helper()
		result, err := handler.RederiveDutyVerificationsOnFundsFactVersion(context.Background(), rederiveCommand(t, received))
		wantOutcome(t, result.Outcome(), err, application.DutyVerificationsRederived)
		if len(result.Lineages()) != 1 {
			t.Fatalf("lineages = %d, want 1", len(result.Lineages()))
		}
		return result.Lineages()[0]
	}
	var unversioned string
	return unversionedDigestCase{
		name:        "REDERIVE_DUTY_VERIFICATIONS",
		unversioned: "aa1000dd91a08c7dee43203ba35d1eb34494ca980a6846aa0c254813d00f1786",
		first: func(t *testing.T) (string, func(string)) {
			var first application.ReceiveExternalFundsFactCommand
			handler, first, _ = verifiedLineage(t, store)
			second = correctionOf(t, first, "SYN-FUNDS-01/v2", 9000)
			if _, err := handler.ReceiveFundsFact(context.Background(), second); err != nil {
				t.Fatalf("资金事实 v2：%v", err)
			}
			formed := rederive(t, second)
			wantOutcome(t, formed.Result.Outcome(), nil, application.DutyVerificationFormed)
			digest, move := movedDutyVerification(t, store, formed.Key)
			return digest, func(rewritten string) {
				unversioned = rewritten
				move(rewritten)
			}
		},
		replay: func(t *testing.T) {
			again := rederive(t, second)
			wantOutcome(t, again.Result.Outcome(), nil, application.DutyVerificationExisting)
			if again.Key.Digest != unversioned {
				t.Fatalf("谱系答案指的是 %q，want 册上那一行的 %q", again.Key.Digest, unversioned)
			}
			if len(store.verifications) != 2 || len(store.handoffs) != 2 {
				t.Fatalf("重派落了 %d 行、交了 %d 封", len(store.verifications), len(store.handoffs))
			}
		},
		changed: func(t *testing.T) {
			third := correctionOf(t, second, "SYN-FUNDS-01/v3", 8000)
			if _, err := handler.ReceiveFundsFact(context.Background(), third); err != nil {
				t.Fatalf("资金事实 v3：%v", err)
			}
			wantOutcome(t, rederive(t, third).Result.Outcome(), nil, application.DutyVerificationFormed)
		},
	}
}

// dispositionVerificationUnversionedCase：同一事实集认作旧行，重发的意图认领的是旧行的键；新事实到达换版。
func dispositionVerificationUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newVerifyFixture(t)
	fixture.facts.facts = []domain.ExecutionFact{executionFact(t, "DESTRUCTION-EXEC/1", 1)}
	handle := func(t *testing.T, want application.VerifyDispositionOutcome) {
		t.Helper()
		result, err := fixture.handler.Handle(context.Background(), verifyCommand(t))
		wantOutcome(t, result.Outcome(), err, want)
	}
	var unversioned string
	return unversionedDigestCase{
		name:        "VERIFY_DISPOSITION",
		unversioned: "9de48e942298c286fe69b31c1c06cdc9ecbc58c433a69f07eb6fbce9c41cfb3f",
		first: func(t *testing.T) (string, func(string)) {
			handle(t, application.VerificationRecorded)
			if len(fixture.store.byKey) != 1 {
				t.Fatalf("册上有 %d 版核对，want 1", len(fixture.store.byKey))
			}
			for key, verification := range fixture.store.byKey {
				return key.Digest, func(rewritten string) {
					unversioned = rewritten
					delete(fixture.store.byKey, key)
					key.Digest = rewritten
					fixture.store.byKey[key] = verification
				}
			}
			return "", nil
		},
		replay: func(t *testing.T) {
			handle(t, application.VerificationExistingResult)
			if fixture.store.saved != 1 {
				t.Fatalf("saved = %d；同事实集又出了一版", fixture.store.saved)
			}
			resent := fixture.downstream.intents[len(fixture.downstream.intents)-1]
			if resent.Key.Digest != unversioned {
				t.Fatalf("重发的意图认领的是 %q，want 册上那一版的 %q", resent.Key.Digest, unversioned)
			}
		},
		changed: func(t *testing.T) {
			fixture.facts.facts = append(fixture.facts.facts, executionFact(t, "DESTRUCTION-EXEC/2", 1))
			handle(t, application.VerificationRecorded)
			if fixture.store.saved != 2 {
				t.Fatalf("saved = %d；新事实到达该换版", fixture.store.saved)
			}
		},
	}
}
