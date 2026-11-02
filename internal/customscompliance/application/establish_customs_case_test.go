package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var caseAt = time.Date(2026, 8, 13, 21, 0, 0, 0, time.UTC)

type caseRequirementViewDouble struct {
	judgment   ports.CaseRequirementJudgment
	configured bool
}

func (double *caseRequirementViewDouble) JudgeCaseRequirement(
	_ context.Context,
	_ domain.TenantID,
	_ domain.RegulatoryJurisdictionReference,
	_ domain.ManifestDirection,
	_ domain.CustomsProcedureReference,
) (ports.CaseRequirementJudgment, bool, error) {
	return double.judgment, double.configured, nil
}

type customsCaseStoreDouble struct {
	byKey map[ports.CustomsCaseKey]domain.CustomsCase
	saved int
}

func (double *customsCaseStoreDouble) FindByKey(
	_ context.Context,
	key ports.CustomsCaseKey,
) (domain.CustomsCase, bool, error) {
	customsCase, found := double.byKey[key]
	return customsCase, found, nil
}

// FindByID 照真库反查读口的口径：按铸造标识在全册里找（ADR-0073 决定五）。
func (double *customsCaseStoreDouble) FindByID(
	_ context.Context,
	_ domain.TenantID,
	id domain.CustomsCaseID,
) (domain.CustomsCase, bool, error) {
	for _, customsCase := range double.byKey {
		if customsCase.ID() == id {
			return customsCase, true, nil
		}
	}
	return domain.CustomsCase{}, false, nil
}

func (double *customsCaseStoreDouble) Save(
	_ context.Context,
	key ports.CustomsCaseKey,
	customsCase domain.CustomsCase,
) (ports.CustomsCaseSaveOutcome, error) {
	if _, exists := double.byKey[key]; exists {
		return ports.CustomsCaseAlreadyRecorded, nil
	}
	double.byKey[key] = customsCase
	double.saved++
	return ports.CustomsCaseSaved, nil
}

type caseIdentityFactoryDouble struct{ minted int }

func (double *caseIdentityFactoryDouble) MintCaseID(_ context.Context) (domain.CustomsCaseID, error) {
	double.minted++
	return domain.NewCustomsCaseID("customs-case-" + string(rune('0'+double.minted)))
}

// caseNumbersDouble 按 outcome 答取号；取到号时按调用次数出 `CC-TEST-000n`，err 非空时交回它。
type caseNumbersDouble struct {
	outcome ports.CaseNumberOutcome
	err     error
	calls   int
}

func (double *caseNumbersDouble) AssignCaseNumber(_ context.Context, _ domain.TenantID) (ports.CaseNumberAssignment, error) {
	double.calls++
	if double.err != nil {
		return ports.CaseNumberAssignment{}, double.err
	}
	assignment := ports.CaseNumberAssignment{Outcome: double.outcome}
	if double.outcome == ports.CaseNumberAssigned {
		assignment.Number = fmt.Sprintf("CC-TEST-%04d", double.calls)
	}
	return assignment, nil
}

type caseDownstreamDouble struct {
	intents []ports.CustomsCaseHandoffIntent
}

func (double *caseDownstreamDouble) HandOffCase(
	_ context.Context,
	intent ports.CustomsCaseHandoffIntent,
) error {
	double.intents = append(double.intents, intent)
	return nil
}

type caseFixture struct {
	handler     *application.EstablishCaseHandler
	requirement *caseRequirementViewDouble
	store       *customsCaseStoreDouble
	identity    *caseIdentityFactoryDouble
	numbers     *caseNumbersDouble
}

func newCaseFixture(t *testing.T) *caseFixture {
	t.Helper()
	fixture := &caseFixture{
		requirement: &caseRequirementViewDouble{
			judgment:   ports.CaseRequirementJudgment{Required: true, Basis: "PRODUCT-RULE/import-obligation"},
			configured: true,
		},
		store:    &customsCaseStoreDouble{byKey: map[ports.CustomsCaseKey]domain.CustomsCase{}},
		identity: &caseIdentityFactoryDouble{},
		numbers:  &caseNumbersDouble{outcome: ports.CaseNumberAssigned},
	}
	fixture.handler = application.NewEstablishCaseHandler(application.EstablishCaseDeps{
		Requirement: fixture.requirement,
		Store:       fixture.store,
		Identity:    fixture.identity,
		Numbers:     fixture.numbers,
		Downstream:  &caseDownstreamDouble{},
		Clock:       fixedClock{at: caseAt},
	})
	return fixture
}

func caseCommand(t *testing.T, parcels ...string) application.EstablishCaseCommand {
	t.Helper()
	associations := make([]domain.CaseParcelAssociation, 0, len(parcels))
	for _, parcel := range parcels {
		associations = append(associations, domain.CaseParcelAssociation{
			Parcel:    parcel,
			Customer:  "customer-1",
			SourceRef: "PS-SOURCE/" + parcel,
		})
	}
	return application.EstablishCaseCommand{
		TenantID:     mustValue(t, domain.NewTenantID, "tenant-1"),
		Jurisdiction: mustValue(t, domain.NewRegulatoryJurisdictionReference, "US"),
		Direction:    domain.ImportManifest,
		Procedure:    mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		Obligation:   mustValue(t, domain.NewObligationScopeReference, "IMPORT-DECLARATION"),
		Parcels:      associations,
		Roles: []domain.CaseRoleSnapshot{{
			Role:      "IMPORTER_OF_RECORD",
			Party:     "party-1",
			Authority: "PC-AUTHORITY/mandate-7",
		}},
		RequestedAt: caseAt.Add(-time.Hour),
	}
}

// Covers: UC-CC-001「同一委托、同袋、同总单、同舱单、同班次或同一路由都不能自动证明应建立同一个关务案件」
// 与四维身份——同键同包裹集重放返原案件不重立（一案一次），
// 同键异包裹集是范围冲突不顶替（扩大范围走案件自己的变更，不经建案入口吸收）。点名
// `AT-CC-009`「相同请求身份和相同业务内容重复到达→返回已有案件」与 `AT-CC-010`
// 「相同请求身份携带不同包裹或监管范围→返回请求冲突并保留原请求与案件」；换
// 监管程序自然换键立独立案件（一包裹可关联多个彼此独立的案件）。
func TestOneCasePerRegulatoryScopeAndNoSilentScopeGrowth(t *testing.T) {
	fixture := newCaseFixture(t)

	established, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1", "parcel-2"))
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	if established.Outcome() != application.CaseEstablished {
		t.Fatalf("outcome = %q", established.Outcome())
	}

	replay, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-2", "parcel-1"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.CaseExisting {
		t.Fatalf("replay = %q; 同包裹集换序也是重放", replay.Outcome())
	}
	if fixture.store.saved != 1 || fixture.identity.minted != 1 {
		t.Fatalf("saved = %d minted = %d", fixture.store.saved, fixture.identity.minted)
	}

	grown, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1", "parcel-2", "parcel-3"))
	if err != nil {
		t.Fatalf("grown: %v", err)
	}
	if grown.Outcome() != application.CaseScopeConflict {
		t.Fatalf("grown = %q; 建案入口吸收了范围扩大", grown.Outcome())
	}

	exportCommand := caseCommand(t, "parcel-1")
	exportCommand.Procedure = mustValue(t, domain.NewCustomsProcedureReference, "CN-EXPORT/9610")
	exportCommand.Direction = domain.ExportManifest
	independent, err := fixture.handler.Handle(context.Background(), exportCommand)
	if err != nil {
		t.Fatalf("independent: %v", err)
	}
	if independent.Outcome() != application.CaseEstablished {
		t.Fatalf("independent = %q; 另一监管程序是独立案件", independent.Outcome())
	}
	if fixture.store.saved != 2 {
		t.Fatalf("saved = %d", fixture.store.saved)
	}
}

// Covers: UC-CC-001 裁决分格——建案规则未登记未决（不是「不要求」）；明确不要求是
// 不适用格带判断依据（服务责任不含此范围的关务履责）；空包裹集未受理（领域把门）。
// 点名 `AT-CC-008`「运营企业不承担该关务范围→返回不适用并保存产品、合同和责任依据；
// 不虚构关务案件」的不适用半边。
func TestCaseRequirementGapsAndRefusalsStayDistinct(t *testing.T) {
	fixture := newCaseFixture(t)
	fixture.requirement.configured = false

	unconfigured, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("unconfigured: %v", err)
	}
	if unconfigured.Outcome() != application.EstablishCaseUndecided {
		t.Fatalf("outcome = %q; 规则未登记不是不要求", unconfigured.Outcome())
	}

	fixture.requirement.configured = true
	fixture.requirement.judgment = ports.CaseRequirementJudgment{
		Required: false,
		Basis:    "PRODUCT-RULE/domestic-only",
	}
	notRequired, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("not required: %v", err)
	}
	if notRequired.Outcome() != application.CaseNotRequired ||
		notRequired.Basis() != "PRODUCT-RULE/domestic-only" {
		t.Fatalf("outcome = %q basis = %q", notRequired.Outcome(), notRequired.Basis())
	}
	if fixture.store.saved != 0 {
		t.Fatal("不适用还立了案")
	}

	fixture.requirement.judgment = ports.CaseRequirementJudgment{Required: true, Basis: "PRODUCT-RULE/import"}
	empty, err := fixture.handler.Handle(context.Background(), caseCommand(t))
	if err != nil {
		t.Fatalf("empty parcels: %v", err)
	}
	if empty.Outcome() != application.EstablishCaseNotAccepted {
		t.Fatalf("outcome = %q; 没有包裹的案件被收下了", empty.Outcome())
	}
}

// Covers: CC CONTEXT「关务案件」——建立案件时在同一事务内取得案件单据号，关联后续案件取新号；ADR-0355 决定三「同一开立
// 意图重放答原号——号存在单据上，重放读回，不再取号」。点名 `AT-CC-459`。取号只在确定要落单的那一步：不适用、未受理、
// 范围冲突都不取号。
func TestACaseTakesItsNumberOnceAndAReplayAnswersTheOriginal(t *testing.T) {
	fixture := newCaseFixture(t)

	established, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1"))
	if err != nil {
		t.Fatal(err)
	}
	customsCase, _ := established.Case()
	if number, present := customsCase.DocumentNumber(); established.Outcome() != application.CaseEstablished ||
		!present || number != "CC-TEST-0001" || established.RollsBack() {
		t.Fatalf("established = %v %q rollsBack=%v", established.Outcome(), number, established.RollsBack())
	}
	if saved := fixture.store.byKey[mustKey(t, caseCommand(t, "parcel-1"))]; saved.ID() != customsCase.ID() {
		t.Fatal("落库的不是带号的那一份")
	} else if number, _ := saved.DocumentNumber(); number != "CC-TEST-0001" {
		t.Fatalf("落库的案件号 = %q", number)
	}

	replay, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1"))
	if err != nil {
		t.Fatal(err)
	}
	replayed, _ := replay.Case()
	if number, _ := replayed.DocumentNumber(); replay.Outcome() != application.CaseExisting || number != "CC-TEST-0001" || replay.RollsBack() {
		t.Fatalf("replay = %v %q", replay.Outcome(), number)
	}

	conflict, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1", "parcel-2"))
	if err != nil || conflict.Outcome() != application.CaseScopeConflict {
		t.Fatalf("conflict = %v %v", conflict.Outcome(), err)
	}
	empty := caseCommand(t)
	empty.Obligation = mustValue(t, domain.NewObligationScopeReference, "EMPTY-SCOPE")
	notAccepted, err := fixture.handler.Handle(context.Background(), empty)
	if err != nil || notAccepted.Outcome() != application.EstablishCaseNotAccepted {
		t.Fatalf("not accepted = %v %v", notAccepted.Outcome(), err)
	}
	fixture.requirement.judgment = ports.CaseRequirementJudgment{Required: false, Basis: "PRODUCT-RULE/domestic-only"}
	if notRequired, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-9")); err != nil ||
		notRequired.Outcome() != application.CaseNotRequired {
		t.Fatalf("not required = %v %v", notRequired.Outcome(), err)
	}
	if fixture.numbers.calls != 1 {
		t.Fatalf("取号 %d 次；重放、冲突、未受理、不适用都不该取号", fixture.numbers.calls)
	}

	fixture.requirement.judgment = ports.CaseRequirementJudgment{Required: true, Basis: "PRODUCT-RULE/import"}
	followUp := caseCommand(t, "parcel-1")
	followUp.Obligation = mustValue(t, domain.NewObligationScopeReference, "POST-CLEARANCE-AUDIT")
	related, err := fixture.handler.Handle(context.Background(), followUp)
	if err != nil {
		t.Fatal(err)
	}
	relatedCase, _ := related.Case()
	if number, _ := relatedCase.DocumentNumber(); related.Outcome() != application.CaseEstablished || number != "CC-TEST-0002" {
		t.Fatalf("关联的后续案件 = %v %q，要取新号", related.Outcome(), number)
	}
}

// Covers: ADR-0355 决定七——关务案件的开立方没有业务时区，租户也没登单据编号时区时，案件照常建立、不带号，记下无号
// 情形，答复不拿案件标识顶号。点名 `AT-CC-460`。
func TestACaseIsEstablishedWithoutANumberWhenTheNumberingTimeZoneIsMissing(t *testing.T) {
	fixture := newCaseFixture(t)
	fixture.numbers.outcome = ports.CaseNumberingTimeZoneNotConfigured

	result, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1"))
	if err != nil {
		t.Fatal(err)
	}
	customsCase, present := result.Case()
	if result.Outcome() != application.CaseEstablished || !present || result.RollsBack() {
		t.Fatalf("outcome = %v rollsBack=%v", result.Outcome(), result.RollsBack())
	}
	if number, numbered := customsCase.DocumentNumber(); numbered || number != "" ||
		customsCase.DocumentNumberAbsence() != domain.NumberingTimeZoneNotConfigured {
		t.Fatalf("number = %q absence = %v", number, customsCase.DocumentNumberAbsence())
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d；无号也照常建案", fixture.store.saved)
	}
}

// Covers: ADR-0355 决定八——编号规则撞号是业务答案：这一次建案不落单，答复要求事务回滚以释放取过的流水；取号读不动是
// 未决并指名编号，同样回滚。点名 `AT-CC-461`。
func TestANumberingCollisionOrFailureLandsNoCaseAndRollsBack(t *testing.T) {
	fixture := newCaseFixture(t)
	fixture.numbers.outcome = ports.CaseNumberingRuleCollision

	collision, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1"))
	if err != nil {
		t.Fatal(err)
	}
	if collision.Outcome() != application.CaseNumberingRuleCollision || collision.Outcome().String() != "NUMBERING_RULE_COLLISION" ||
		!collision.RollsBack() {
		t.Fatalf("collision = %v rollsBack=%v", collision.Outcome(), collision.RollsBack())
	}
	if _, present := collision.Case(); present || fixture.store.saved != 0 {
		t.Fatal("撞号还落了案件")
	}

	fixture.numbers.err = errors.New("sequence store unavailable")
	failed, err := fixture.handler.Handle(context.Background(), caseCommand(t, "parcel-1"))
	if err != nil {
		t.Fatal(err)
	}
	if failed.Outcome() != application.EstablishCaseUndecided || failed.UndecidedReason().String() != "CASE_NUMBERING_UNAVAILABLE" ||
		!failed.RollsBack() || fixture.store.saved != 0 {
		t.Fatalf("failed = %v %v rollsBack=%v saved=%d", failed.Outcome(), failed.UndecidedReason(), failed.RollsBack(), fixture.store.saved)
	}
}

// racingCaseStoreDouble 模拟并发建案：FindByKey 头一次答没有，Save 时另一笔已先落下。
type racingCaseStoreDouble struct {
	customsCaseStoreDouble
	winner  domain.CustomsCase
	checked bool
}

func (double *racingCaseStoreDouble) FindByKey(_ context.Context, _ ports.CustomsCaseKey) (domain.CustomsCase, bool, error) {
	if !double.checked {
		double.checked = true
		return domain.CustomsCase{}, false, nil
	}
	return double.winner, true, nil
}

func (double *racingCaseStoreDouble) Save(_ context.Context, _ ports.CustomsCaseKey, _ domain.CustomsCase) (ports.CustomsCaseSaveOutcome, error) {
	return ports.CustomsCaseAlreadyRecorded, nil
}

// Covers: 并发建案输掉的那一笔取过号却没落下案件：答复照重放办（赢家与它的原号），并要求回滚释放自己取过的流水
// （ADR-0355 决定三：只许随单落定的跳号成为空号）。
func TestALosingConcurrentEstablishmentAnswersTheWinnerAndRollsBack(t *testing.T) {
	fixture := newCaseFixture(t)
	winner, err := domain.EstablishCustomsCase(domain.CustomsCaseSpec{
		ID:            mustValue(t, domain.NewCustomsCaseID, "customs-case-winner"),
		Jurisdiction:  mustValue(t, domain.NewRegulatoryJurisdictionReference, "US"),
		Direction:     domain.ImportManifest,
		Procedure:     mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		Obligation:    mustValue(t, domain.NewObligationScopeReference, "IMPORT-DECLARATION"),
		Parcels:       []domain.CaseParcelAssociation{{Parcel: "parcel-1", Customer: "customer-1", SourceRef: "PS-SOURCE/parcel-1"}},
		EstablishedAt: caseAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if winner, err = winner.Numbered("CC-WINNER-0001"); err != nil {
		t.Fatal(err)
	}
	racing := &racingCaseStoreDouble{winner: winner}
	handler := application.NewEstablishCaseHandler(application.EstablishCaseDeps{
		Requirement: fixture.requirement,
		Store:       racing,
		Identity:    fixture.identity,
		Numbers:     fixture.numbers,
		Downstream:  &caseDownstreamDouble{},
		Clock:       fixedClock{at: caseAt},
	})

	result, err := handler.Handle(context.Background(), caseCommand(t, "parcel-1"))
	if err != nil {
		t.Fatal(err)
	}
	answered, _ := result.Case()
	if number, _ := answered.DocumentNumber(); result.Outcome() != application.CaseExisting || number != "CC-WINNER-0001" ||
		!result.RollsBack() || fixture.numbers.calls != 1 {
		t.Fatalf("outcome = %v number = %q rollsBack=%v calls=%d", result.Outcome(), number, result.RollsBack(), fixture.numbers.calls)
	}
}

func mustKey(t *testing.T, command application.EstablishCaseCommand) ports.CustomsCaseKey {
	t.Helper()
	return ports.CustomsCaseKey{
		TenantID:     command.TenantID,
		Jurisdiction: command.Jurisdiction,
		Direction:    command.Direction,
		Procedure:    command.Procedure,
		Obligation:   command.Obligation,
	}
}
