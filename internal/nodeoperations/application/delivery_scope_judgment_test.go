package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证收寄与重核的身份判断带上可验证交付范围之后的答法（ADR-0212 决定四至八；UC-NO-002 `AT-NO-052`、`AT-NO-062` 末句、
// `AT-NO-063` 至 `AT-NO-068`）：编排先由依据推导范围、把客户账户交给身份核对读口，再照答复值上的判断格分走向，每一版做过核对的判断
// 记交付范围核对态。身份核对读口的替身照 PS 的答法算范围相容与信号（identityViewDouble），证据读口的替身按标识交回证据状态。夹具全为
// 合成值（S 级）。

const scopeMark = "BARCODE-1"

func scopeWindowFrom(t *testing.T, from time.Time, lead, grace time.Duration) domain.PreAdviceValidityWindow {
	t.Helper()
	window, err := domain.FormPreAdviceValidityWindow(domain.PreAdviceValidityWindowSpec{
		TenantID:      mustValue(t, domain.NewTenantID, "tenant-1"),
		Lead:          lead,
		Grace:         grace,
		EffectiveFrom: from,
		Basis:         "SYN-WINDOW-BASIS",
		RegisteredAt:  from,
	})
	if err != nil {
		t.Fatalf("时窗：%v", err)
	}
	return window
}

// scopeWindow 是演示一类的宽时窗：提前一天、宽限半天，早于各夹具的收寄时刻生效。
func scopeWindow(t *testing.T) domain.PreAdviceValidityWindow {
	t.Helper()
	return scopeWindowFrom(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 24*time.Hour, 12*time.Hour)
}

func preAdviceState(t *testing.T, id, account, node string, expected time.Time, numbers ...string) domain.DeliveryPreAdviceState {
	t.Helper()
	listed := make([]domain.PreAdvisedParcelNumber, 0, len(numbers))
	for _, number := range numbers {
		listed = append(listed, mustValue(t, domain.NewPreAdvisedParcelNumber, number))
	}
	preAdvice, err := domain.FormDeliveryPreAdvice(domain.DeliveryPreAdviceSpec{
		TenantID:          mustValue(t, domain.NewTenantID, "tenant-1"),
		ID:                mustValue(t, domain.NewDeliveryPreAdviceID, id),
		CustomerAccount:   mustValue(t, domain.NewCustomerAccountReference, account),
		Node:              mustValue(t, domain.NewNodeReference, node),
		ExpectedArrivalAt: expected,
		Numbers:           listed,
		Source:            domain.CustomerRequestSource{Channel: "SYN-CLIENT", RequestKey: "SYN-KEY-" + id, ContentDigest: "NOC-1:" + id},
		FormedAt:          expected.Add(-48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("预报：%v", err)
	}
	return domain.DeliveryPreAdviceState{PreAdvice: preAdvice}
}

func withdrawnState(t *testing.T, state domain.DeliveryPreAdviceState) domain.DeliveryPreAdviceState {
	t.Helper()
	withdrawal, err := domain.FormDeliveryPreAdviceWithdrawal(domain.DeliveryPreAdviceWithdrawalSpec{
		TenantID:        state.PreAdvice.TenantID(),
		PreAdvice:       state.PreAdvice.ID(),
		CustomerAccount: state.PreAdvice.CustomerAccount(),
		Source:          domain.CustomerRequestSource{Channel: "SYN-CLIENT", RequestKey: "SYN-KEY-WITHDRAW", ContentDigest: "NOC-1:withdraw"},
		WithdrawnAt:     state.PreAdvice.ExpectedArrivalAt().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("撤回：%v", err)
	}
	state.Withdrawal = withdrawal
	return state
}

// handoverNoteState 立一张在 node 开立、声称 claimed 的交接单，confirmed 里的客户账户经客户渠道确认。
func handoverNoteState(t *testing.T, id, node string, claimed []string, confirmed ...string) domain.DeliveryHandoverNoteState {
	t.Helper()
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	accounts := make([]domain.CustomerAccountReference, 0, len(claimed))
	for _, account := range claimed {
		accounts = append(accounts, mustValue(t, domain.NewCustomerAccountReference, account))
	}
	note, err := domain.OpenDeliveryHandoverNote(domain.DeliveryHandoverNoteSpec{
		TenantID:        tenant,
		ID:              mustValue(t, domain.NewDeliveryHandoverNoteID, id),
		Node:            mustValue(t, domain.NewNodeReference, node),
		DeliveringParty: mustValue(t, domain.NewDeliveringPartyReference, "SYN-DRIVER-1"),
		ClaimedAccounts: accounts,
		OpenedAt:        deliveredAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("交接单：%v", err)
	}
	state := domain.DeliveryHandoverNoteState{Note: note}
	for _, account := range confirmed {
		binding, err := domain.FormDeliveryHandoverBinding(domain.DeliveryHandoverBindingSpec{
			TenantID:    tenant,
			Note:        note.ID(),
			Account:     mustValue(t, domain.NewCustomerAccountReference, account),
			Conclusion:  domain.BindingConfirmed,
			Route:       domain.ConcludedViaCustomerChannel,
			Concluder:   "SYN-CLIENT",
			ConcludedAt: deliveredAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("绑定：%v", err)
		}
		state.Bindings = append(state.Bindings, binding)
	}
	return state
}

func basisOf(t *testing.T, kind domain.DeliveryScopeBasisKind, id string) domain.DeliveryScopeBasisReference {
	t.Helper()
	basis, err := domain.NewDeliveryScopeBasisReference(kind, id)
	if err != nil {
		t.Fatalf("依据引用：%v", err)
	}
	return basis
}

// answerCandidates 让身份核对替身答 parcels，spaces 记各件号码所在的客户账户编号空间（不在列的照渠道包裹件号答）。
func (fixture *receptionFixture) answerCandidates(t *testing.T, spaces map[string]string, declaredBy []string, parcels ...string) {
	t.Helper()
	fixture.identity.candidates = nil
	for _, parcel := range parcels {
		fixture.identity.candidates = append(fixture.identity.candidates, mustValue(t, domain.NewParcelAssociationReference, parcel))
	}
	fixture.identity.spaces = spaces
	fixture.identity.declaredBy = declaredBy
	fixture.identity.ambiguous = false
}

// preAdviseA 登一份客户账户 A 的有效预报（列着 scopeMark，在收寄节点，预计送达就是收寄时刻）与一行时窗。
func (fixture *receptionFixture) preAdviseA(t *testing.T) domain.DeliveryScopeBasisReference {
	t.Helper()
	fixture.scope.preAdvices["PREADV-A"] = preAdviceState(t, "PREADV-A", "customer-A", "node-origin", deliveredAt, scopeMark)
	fixture.scope.windows = []domain.PreAdviceValidityWindow{scopeWindow(t)}
	return basisOf(t, domain.BasisDeliveryPreAdvice, "PREADV-A")
}

func handleScoped(t *testing.T, fixture *receptionFixture, command application.ReceiveDeliveredUnitCommand) (application.ReceptionOutcome, ports.ReceptionRecord) {
	t.Helper()
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("收寄 %s：%v", command.SourceID, err)
	}
	record, _ := result.Record()
	return result.Outcome(), record
}

func candidateParcels(record ports.ReceptionRecord) []string {
	parcels := make([]string, 0, len(record.Candidates))
	for _, candidate := range record.Candidates {
		parcels = append(parcels, candidate.String())
	}
	slices.Sort(parcels)
	return parcels
}

func checkOf(record ports.ReceptionRecord) string {
	check := record.DeliveryScopeCheck
	return check.State().String() + "/" + check.Reason().String() + "/" + check.Inapplicability().String()
}

// assertScopeUnverifiedPending 断言这一版是范围未证那一格（ADR-0212 决定五）：待识别、记候选、不记冲突、核对过，接收与控制成立，不关联。
func assertScopeUnverifiedPending(t *testing.T, name string, outcome application.ReceptionOutcome, record ports.ReceptionRecord, wantCheck string, candidates ...string) {
	t.Helper()
	if outcome != application.UnitPendingIdentification {
		t.Fatalf("%s：outcome = %v, want PENDING_IDENTIFICATION", name, outcome)
	}
	if _, associated := record.Intake.Association(); associated || record.IdentityConflict ||
		record.IdentityConflictCause != ports.IdentityConflictCauseAbsent || record.IdentityCheck != ports.IdentityChecked ||
		!record.Control.Active() {
		t.Fatalf("%s：记录 = %+v, want 不关联、不记冲突、核对过、控制成立", name, record)
	}
	if got := candidateParcels(record); !slices.Equal(got, candidates) {
		t.Fatalf("%s：候选 = %v, want %v", name, got, candidates)
	}
	if got := checkOf(record); got != wantCheck {
		t.Fatalf("%s：交付范围核对态 = %s, want %s", name, got, wantCheck)
	}
}

// Covers: `AT-NO-063`（ADR-0212 决定四新增那一格、决定五）——号码经客户包裹件号在册里只标识一件、没有歧义信号，送站没有可验证交付
// 范围：没带依据、依据不在册、预报没列这个号、节点不符、业务时间在时窗外、预报已撤回，各答待识别、候选记那一件、不记冲突，交付
// 范围核对态记未证并附原因；接收、控制与来源照常，不关联、不问服务结果、不交意图、不响门铃。身份核对读口被问时一个客户账户都不带。
func TestACustomerNumberHitWithoutAVerifiableScopeIsPendingAndUnverified(t *testing.T) {
	for name, testCase := range map[string]struct {
		arrange func(*testing.T, *receptionFixture) domain.DeliveryScopeBasisReference
		check   string
	}{
		"没带依据": {func(*testing.T, *receptionFixture) domain.DeliveryScopeBasisReference {
			return domain.DeliveryScopeBasisReference{}
		}, "UNVERIFIED/NO_BASIS/"},
		"依据不在册": {func(t *testing.T, fixture *receptionFixture) domain.DeliveryScopeBasisReference {
			fixture.scope.windows = []domain.PreAdviceValidityWindow{scopeWindow(t)}
			return basisOf(t, domain.BasisDeliveryPreAdvice, "PREADV-ABSENT")
		}, "UNVERIFIED/BASIS_NOT_REGISTERED/"},
		"预报没列这个号": {func(t *testing.T, fixture *receptionFixture) domain.DeliveryScopeBasisReference {
			fixture.scope.preAdvices["PREADV-A"] = preAdviceState(t, "PREADV-A", "customer-A", "node-origin", deliveredAt, "BARCODE-OTHER")
			fixture.scope.windows = []domain.PreAdviceValidityWindow{scopeWindow(t)}
			return basisOf(t, domain.BasisDeliveryPreAdvice, "PREADV-A")
		}, "UNVERIFIED/NUMBER_NOT_PRE_ADVISED/"},
		"节点不符": {func(t *testing.T, fixture *receptionFixture) domain.DeliveryScopeBasisReference {
			fixture.scope.preAdvices["PREADV-A"] = preAdviceState(t, "PREADV-A", "customer-A", "node-elsewhere", deliveredAt, scopeMark)
			fixture.scope.windows = []domain.PreAdviceValidityWindow{scopeWindow(t)}
			return basisOf(t, domain.BasisDeliveryPreAdvice, "PREADV-A")
		}, "UNVERIFIED/BASIS_NOT_APPLICABLE/NODE_MISMATCH"},
		"在时窗外": {func(t *testing.T, fixture *receptionFixture) domain.DeliveryScopeBasisReference {
			fixture.scope.preAdvices["PREADV-A"] = preAdviceState(t, "PREADV-A", "customer-A", "node-origin", deliveredAt.Add(72*time.Hour), scopeMark)
			fixture.scope.windows = []domain.PreAdviceValidityWindow{scopeWindow(t)}
			return basisOf(t, domain.BasisDeliveryPreAdvice, "PREADV-A")
		}, "UNVERIFIED/BASIS_NOT_APPLICABLE/OUTSIDE_WINDOW"},
		"已撤回": {func(t *testing.T, fixture *receptionFixture) domain.DeliveryScopeBasisReference {
			fixture.scope.preAdvices["PREADV-A"] = withdrawnState(t, preAdviceState(t, "PREADV-A", "customer-A", "node-origin", deliveredAt, scopeMark))
			fixture.scope.windows = []domain.PreAdviceValidityWindow{scopeWindow(t)}
			return basisOf(t, domain.BasisDeliveryPreAdvice, "PREADV-A")
		}, "UNVERIFIED/BASIS_NOT_APPLICABLE/WITHDRAWN"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			fixture.answerCandidates(t, map[string]string{"parcel-a": "customer-A"}, nil, "parcel-a")
			command := receiveCommand(t)
			command.ScopeBasis = testCase.arrange(t, fixture)
			outcome, record := handleScoped(t, fixture, command)
			assertScopeUnverifiedPending(t, name, outcome, record, testCase.check, "parcel-a")
			if len(fixture.downstream.intents) != 0 || len(fixture.serviceResults.asked) != 0 || len(fixture.doorbell.rung) != 0 {
				t.Fatalf("意图 %d、服务结果问 %d、门铃 %d, want 都没有", len(fixture.downstream.intents),
					len(fixture.serviceResults.asked), len(fixture.doorbell.rung))
			}
			if len(fixture.identity.scopes) != 1 || len(fixture.identity.scopes[0]) != 0 {
				t.Fatalf("身份核对被问时带的客户账户 = %v, want 一次、不带", fixture.identity.scopes)
			}
		})
	}
}

// Covers: `AT-NO-064`（ADR-0212 决定四、决定八）——送站带 A 的预报、号码在列；同号在册标识 A 的一件和 B 的一件，B 另有一份`已提交`
// 委托也报了它：关联 A 那一件、形成收寄与控制、交意图；候选两件都记；核对态已证；身份核对带着 A 去问。
func TestAPreAdviceScopePicksItsAccountsCandidateAmongOthers(t *testing.T) {
	fixture := newReceptionFixture(t)
	fixture.answerCandidates(t, map[string]string{"parcel-a": "customer-A", "parcel-b": "customer-B"}, []string{"customer-B"},
		"parcel-a", "parcel-b")
	command := receiveCommand(t)
	command.ScopeBasis = fixture.preAdviseA(t)
	outcome, record := handleScoped(t, fixture, command)
	if outcome != application.NodeIntakeFormed {
		t.Fatalf("outcome = %v, want INTAKE_FORMED", outcome)
	}
	if association, associated := record.Intake.Association(); !associated || association.String() != "parcel-a" {
		t.Fatalf("关联 = %v（在场 %v）, want parcel-a", association, associated)
	}
	if got := candidateParcels(record); !slices.Equal(got, []string{"parcel-a", "parcel-b"}) || record.IdentityConflict {
		t.Fatalf("候选 = %v 冲突 %v, want 两件都记、不记冲突", got, record.IdentityConflict)
	}
	if got := checkOf(record); got != "VERIFIED//" {
		t.Fatalf("核对态 = %s, want VERIFIED", got)
	}
	if len(fixture.identity.scopes) != 1 || !slices.Equal(fixture.identity.scopes[0], []string{"customer-A"}) {
		t.Fatalf("身份核对带的客户账户 = %v, want [[customer-A]]", fixture.identity.scopes)
	}
	if len(fixture.downstream.intents) != 1 || !slices.Equal(fixture.serviceResults.asked, []string{"tenant-1|parcel-a"}) {
		t.Fatalf("意图 %d、服务结果问 %v, want 一封、问 parcel-a", len(fixture.downstream.intents), fixture.serviceResults.asked)
	}
}

// Covers: `AT-NO-065` 与决定八「冲突来由按范围相容的候选分」——有范围（A）时：A 的两件同号是多候选冲突；A 的一件在册、A 另一份`已提交`
// 委托也报了它是只因歧义信号；范围外另有 B 那件、范围内只差一个信号的，仍记只因歧义信号，不记多候选。候选全部记下，核对态已证，
// 不关联、不交意图。
func TestAScopeStillContestsWithinItsOwnAccounts(t *testing.T) {
	for name, testCase := range map[string]struct {
		spaces     map[string]string
		declaredBy []string
		parcels    []string
		cause      ports.IdentityConflictCause
	}{
		"A 两件同号": {map[string]string{"parcel-a1": "customer-A", "parcel-a2": "customer-A"}, nil,
			[]string{"parcel-a1", "parcel-a2"}, ports.ConflictByMultipleCandidates},
		"A 一件在册、A 另有声明": {map[string]string{"parcel-a": "customer-A"}, []string{"customer-A"},
			[]string{"parcel-a"}, ports.ConflictByAmbiguityOnly},
		"范围外另有候选、范围内只差信号": {map[string]string{"parcel-a": "customer-A", "parcel-b": "customer-B"}, []string{"customer-A"},
			[]string{"parcel-a", "parcel-b"}, ports.ConflictByAmbiguityOnly},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			fixture.answerCandidates(t, testCase.spaces, testCase.declaredBy, testCase.parcels...)
			command := receiveCommand(t)
			command.ScopeBasis = fixture.preAdviseA(t)
			outcome, record := handleScoped(t, fixture, command)
			if outcome != application.UnitPendingIdentification || !record.IdentityConflict || record.IdentityConflictCause != testCase.cause {
				t.Fatalf("outcome %v 冲突 %v 来由 %v, want 待识别、记冲突、来由 %v", outcome, record.IdentityConflict,
					record.IdentityConflictCause, testCase.cause)
			}
			if got := candidateParcels(record); !slices.Equal(got, testCase.parcels) {
				t.Fatalf("候选 = %v, want 全部 %v（含范围外的）", got, testCase.parcels)
			}
			if got := checkOf(record); got != "VERIFIED//" || len(fixture.downstream.intents) != 0 {
				t.Fatalf("核对态 %s、意图 %d, want VERIFIED、没有意图", got, len(fixture.downstream.intents))
			}
		})
	}
}

// Covers: `AT-NO-066`——有范围（A），号码在 A 的预报里，但 A 的委托还没接受，册上只有 B 的一件：待识别、B 那件照记为候选、不记冲突、
// 核对态已证。A 的委托接受之后，关联已形成的门铃按号重核：范围按存下的预报重算，追加关联到 A 那一件的版本，业务时间与控制照原收寄。
func TestAScopeWithNoCompatibleCandidateIsPendingAndARecheckLaterPicksTheScopedOne(t *testing.T) {
	fixture := newRecheckFixture(t)
	fixture.answerCandidates(t, map[string]string{"parcel-b": "customer-B"}, nil, "parcel-b")
	command := receiveCommand(t)
	command.ScopeBasis = fixture.preAdviseA(t)
	outcome, pending := handleScoped(t, fixture.receptionFixture, command)
	assertScopeUnverifiedPending(t, "范围内没有候选", outcome, pending, "VERIFIED//", "parcel-b")

	fixture.answerCandidates(t, map[string]string{"parcel-a": "customer-A", "parcel-b": "customer-B"}, nil, "parcel-a", "parcel-b")
	results := recheckByMark(t, fixture)
	if len(results) != 1 || results[0].Outcome() != application.RecheckIntakeFormed {
		t.Fatalf("按号重核 = %d 件, want 恰一件 INTAKE_FORMED", len(results))
	}
	formed, _ := results[0].Record()
	if association, associated := formed.Intake.Association(); !associated || association.String() != "parcel-a" ||
		!formed.Intake.ReceivedAt().Equal(deliveredAt) || formed.Control.Basis() != pending.Control.Basis() ||
		formed.Predecessor != pending.JudgmentVersion || checkOf(formed) != "VERIFIED//" {
		t.Fatalf("追加版 = %+v, want 关联 parcel-a、业务时间与控制照原收寄、前版是原待识别、核对态已证", formed)
	}
	if got := fixture.identity.scopes[len(fixture.identity.scopes)-1]; !slices.Equal(got, []string{"customer-A"}) {
		t.Fatalf("重核带的客户账户 = %v, want [customer-A]", got)
	}
}

// Covers: `AT-NO-067`——号码经渠道包裹件号在册里只标识一件、没有信号、送站没有可验证交付范围：照常唯一识别，渠道包裹件号的命中
// 范围相容。这一版照样记核对态：没带依据。
func TestAChannelNumberHitIsIdentifiedWithoutAScope(t *testing.T) {
	fixture := newReceptionFixture(t)
	fixture.answerCandidates(t, nil, nil, "parcel-channel")
	outcome, record := handleScoped(t, fixture, receiveCommand(t))
	if association, associated := record.Intake.Association(); outcome != application.NodeIntakeFormed || !associated ||
		association.String() != "parcel-channel" {
		t.Fatalf("outcome %v 关联 %v, want INTAKE_FORMED 关联 parcel-channel", outcome, association)
	}
	if got := checkOf(record); got != "UNVERIFIED/NO_BASIS/" {
		t.Fatalf("核对态 = %s, want UNVERIFIED/NO_BASIS", got)
	}
}

// Covers: `AT-NO-068`——带的预报其余条件都满足，但本租户没有登记预报有效时窗：核对态记未配置、与未证分开；身份按没有可验证交付范围
// 判——只经客户包裹件号命中的一件落范围未证的待识别，身份核对不带客户账户去问。
func TestAnUnregisteredWindowIsUnconfiguredAndJudgedWithoutAScope(t *testing.T) {
	fixture := newReceptionFixture(t)
	fixture.answerCandidates(t, map[string]string{"parcel-a": "customer-A"}, nil, "parcel-a")
	fixture.scope.preAdvices["PREADV-A"] = preAdviceState(t, "PREADV-A", "customer-A", "node-origin", deliveredAt, scopeMark)
	command := receiveCommand(t)
	command.ScopeBasis = basisOf(t, domain.BasisDeliveryPreAdvice, "PREADV-A")
	outcome, record := handleScoped(t, fixture, command)
	assertScopeUnverifiedPending(t, "没登时窗", outcome, record, "UNCONFIGURED//", "parcel-a")
	if len(fixture.identity.scopes) != 1 || len(fixture.identity.scopes[0]) != 0 {
		t.Fatalf("身份核对带的客户账户 = %v, want 不带", fixture.identity.scopes)
	}
}

// Covers: `AT-NO-052` 两半——号码在册里只标识 A 的一件，同时被本租户另一份`已提交`委托声明：没有范围时，声明在租户内认，答身份冲突
// （只因歧义信号）；有范围（A）、声明也在 A 时同样冲突，那份声明撤回之后，同号在范围内到站照常唯一识别。
func TestADeclarationContestsUntilItIsGoneAndThenTheScopedHitIsUnique(t *testing.T) {
	unscoped := newReceptionFixture(t)
	unscoped.answerCandidates(t, map[string]string{"parcel-a": "customer-A"}, []string{"customer-B"}, "parcel-a")
	outcome, record := handleScoped(t, unscoped, receiveCommand(t))
	if outcome != application.UnitPendingIdentification || !record.IdentityConflict ||
		record.IdentityConflictCause != ports.ConflictByAmbiguityOnly {
		t.Fatalf("没有范围：outcome %v 冲突 %v 来由 %v, want 只因歧义信号的身份冲突", outcome, record.IdentityConflict, record.IdentityConflictCause)
	}

	scoped := newReceptionFixture(t)
	scoped.answerCandidates(t, map[string]string{"parcel-a": "customer-A"}, []string{"customer-A"}, "parcel-a")
	basis := scoped.preAdviseA(t)
	before := receiveCommand(t)
	before.ScopeBasis = basis
	if outcome, record := handleScoped(t, scoped, before); outcome != application.UnitPendingIdentification || !record.IdentityConflict {
		t.Fatalf("范围内有声明：outcome %v 冲突 %v, want 身份冲突", outcome, record.IdentityConflict)
	}
	scoped.identity.declaredBy = nil
	after := receiveCommand(t)
	after.SourceID, after.Unit, after.ScopeBasis = "delivery-2", mustValue(t, domain.NewHandlingUnitID, "unit-2"), basis
	outcome, record = handleScoped(t, scoped, after)
	if association, associated := record.Intake.Association(); outcome != application.NodeIntakeFormed || !associated ||
		association.String() != "parcel-a" {
		t.Fatalf("声明撤回之后：outcome %v 关联 %v, want INTAKE_FORMED 关联 parcel-a", outcome, association)
	}
}

// Covers: ADR-0212 决定七——依据引用原样进来源文档；同一来源只在这一格上不同的重投照旧答已有结果，首次存下的依据不被覆盖，也不另落
// 一行。
func TestTheScopeBasisIsPreservedAndAReplayDifferingOnlyThereAnswersTheFirstResult(t *testing.T) {
	fixture := newReceptionFixture(t)
	fixture.answerCandidates(t, map[string]string{"parcel-a": "customer-A"}, nil, "parcel-a")
	first := receiveCommand(t)
	first.ScopeBasis = fixture.preAdviseA(t)
	_, record := handleScoped(t, fixture, first)
	if record.Source.ScopeBasis != first.ScopeBasis {
		t.Fatalf("来源里的依据 = %+v, want %+v", record.Source.ScopeBasis, first.ScopeBasis)
	}
	rows := len(fixture.receptions.byKey)
	for name, basis := range map[string]domain.DeliveryScopeBasisReference{
		"换了一份预报": basisOf(t, domain.BasisDeliveryPreAdvice, "PREADV-OTHER"),
		"换成交接单":  basisOf(t, domain.BasisDeliveryHandoverNote, "HANDOVER-1"),
		"没带依据":   {},
	} {
		replay := receiveCommand(t)
		replay.ScopeBasis = basis
		outcome, again := handleScoped(t, fixture, replay)
		if outcome != application.ReceptionExistingResult || again.Source.ScopeBasis != first.ScopeBasis {
			t.Fatalf("%s：outcome %v、在册依据 %+v, want EXISTING_RESULT、首次的依据", name, outcome, again.Source.ScopeBasis)
		}
	}
	if len(fixture.receptions.byKey) != rows {
		t.Fatalf("重投落了新行：%d → %d", rows, len(fixture.receptions.byKey))
	}
}

// Covers: 证据册读不动是本方册读不动，不是没问到 PS（同 ADR-0199 决定五对按包裹读别的实物收寄的处理）：不形成答案，什么都不落、
// 不响门铃。PS 身份核对读不动照旧落未核对的待识别，这一版没有做成核对，不记交付范围核对态。
func TestScopeEvidenceFailuresFormNoAnswerAndAnUncheckedUnitCarriesNoScopeCheck(t *testing.T) {
	fixture := newReceptionFixture(t)
	command := receiveCommand(t)
	command.ScopeBasis = fixture.preAdviseA(t)
	fixture.scope.err = errors.New("pre-advice register unreachable")
	if _, err := fixture.handler.Handle(context.Background(), command); err == nil {
		t.Fatal("证据册读不动也形成了答案")
	}
	if fixture.receptions.saved != 0 || len(fixture.doorbell.rung) != 0 {
		t.Fatalf("证据册读不动落了 %d 行、门铃 %d", fixture.receptions.saved, len(fixture.doorbell.rung))
	}

	fixture.scope.err = nil
	fixture.identity.err = errors.New("external identifier register unreachable")
	outcome, record := handleScoped(t, fixture, command)
	if outcome != application.UnitPendingIdentification || record.IdentityCheck != ports.IdentityUnchecked ||
		record.DeliveryScopeCheck.State() != domain.DeliveryScopeCheckStateInvalid {
		t.Fatalf("PS 读不动：outcome %v 核对态 %v 范围核对态 %s, want 未核对的待识别、不记范围核对态", outcome, record.IdentityCheck, checkOf(record))
	}
}

// Covers: ADR-0212 决定六「可验证交付范围按存下的依据（来源里的引用，或此后归入的交接单）与此刻的证据状态重算」——来源带的交接单收寄
// 时绑定未全部确认，落范围未证的待识别；此后（夹具里直接改证据状态）客户确认了绑定，人工重核追加关联版，业务时间取原收寄时刻、
// 核对态已证。来源没带依据的那件，此后归入一张范围已成立的交接单，同样经重核续上。
func TestARecheckRederivesTheScopeFromTheStoredBasisAndTheEvidenceNow(t *testing.T) {
	for name, testCase := range map[string]struct {
		basis   func(*testing.T) domain.DeliveryScopeBasisReference
		check   string
		satisfy func(*testing.T, *recheckFixture)
	}{
		"来源带的交接单后来确认": {
			basis: func(t *testing.T) domain.DeliveryScopeBasisReference {
				return basisOf(t, domain.BasisDeliveryHandoverNote, "HANDOVER-1")
			},
			check: "UNVERIFIED/BINDINGS_NOT_ALL_CONFIRMED/",
			satisfy: func(t *testing.T, fixture *recheckFixture) {
				fixture.scope.notes["HANDOVER-1"] = handoverNoteState(t, "HANDOVER-1", "node-origin", []string{"customer-A"}, "customer-A")
			},
		},
		"此后归入一张已成立的交接单": {
			basis: func(*testing.T) domain.DeliveryScopeBasisReference { return domain.DeliveryScopeBasisReference{} },
			check: "UNVERIFIED/NO_BASIS/",
			satisfy: func(t *testing.T, fixture *recheckFixture) {
				fixture.scope.notes["HANDOVER-2"] = handoverNoteState(t, "HANDOVER-2", "node-origin", []string{"customer-A"}, "customer-A")
				fixture.scope.assigned["unit-1"] = mustValue(t, domain.NewDeliveryHandoverNoteID, "HANDOVER-2")
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRecheckFixture(t)
			fixture.answerCandidates(t, map[string]string{"parcel-a": "customer-A"}, nil, "parcel-a")
			fixture.scope.notes["HANDOVER-1"] = handoverNoteState(t, "HANDOVER-1", "node-origin", []string{"customer-A"})
			command := receiveCommand(t)
			command.ScopeBasis = testCase.basis(t)
			outcome, pending := handleScoped(t, fixture.receptionFixture, command)
			assertScopeUnverifiedPending(t, name, outcome, pending, testCase.check, "parcel-a")

			if result := recheckNamed(t, fixture, pending.JudgmentVersion); result.Outcome() != application.RecheckStillUnidentified {
				t.Fatalf("证据没变时重核 = %v, want STILL_UNIDENTIFIED", result.Outcome())
			}
			testCase.satisfy(t, fixture)
			result := recheckNamed(t, fixture, pending.JudgmentVersion)
			formed, _ := result.Record()
			if result.Outcome() != application.RecheckIntakeFormed || !formed.Intake.ReceivedAt().Equal(deliveredAt) ||
				formed.Predecessor != pending.JudgmentVersion || checkOf(formed) != "VERIFIED//" {
				t.Fatalf("证据成立之后重核 = %v、追加版 %+v, want INTAKE_FORMED、业务时间是原收寄时刻、核对态已证", result.Outcome(), formed)
			}
			if len(fixture.downstream.intents) != 1 {
				t.Fatalf("意图 %d 封, want 1", len(fixture.downstream.intents))
			}
		})
	}
}

// Covers: ADR-0212 决定六「预报的有效时窗仍按原收寄的业务时间判，不按重核时刻判」——收寄时预报在时窗里、A 的委托还没接受；重核晚于
// 宽限量才来，范围照样成立、追加关联版，时窗册每一次都按原业务时间找。时窗行收寄之后才生效的，那一刻没有生效行，重核答未配置、
// 不追加。
func TestARecheckJudgesThePreAdviceWindowAtTheOriginalBusinessTime(t *testing.T) {
	fixture := newRecheckFixture(t)
	fixture.answerCandidates(t, map[string]string{"parcel-b": "customer-B"}, nil, "parcel-b")
	fixture.scope.preAdvices["PREADV-A"] = preAdviceState(t, "PREADV-A", "customer-A", "node-origin", deliveredAt, scopeMark)
	// 宽限十分钟：重核的时钟在收寄之后一小时，按重核时刻判就在时窗外。
	fixture.scope.windows = []domain.PreAdviceValidityWindow{
		scopeWindowFrom(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 10*time.Minute, 10*time.Minute),
	}
	command := receiveCommand(t)
	command.ScopeBasis = basisOf(t, domain.BasisDeliveryPreAdvice, "PREADV-A")
	outcome, pending := handleScoped(t, fixture.receptionFixture, command)
	assertScopeUnverifiedPending(t, "收寄", outcome, pending, "VERIFIED//", "parcel-b")

	fixture.answerCandidates(t, map[string]string{"parcel-a": "customer-A", "parcel-b": "customer-B"}, nil, "parcel-a", "parcel-b")
	result := recheckNamed(t, fixture, pending.JudgmentVersion)
	if result.Outcome() != application.RecheckIntakeFormed {
		t.Fatalf("重核 = %v, want INTAKE_FORMED——时窗按重核时刻判了", result.Outcome())
	}
	for _, at := range fixture.scope.windowAsked {
		if !at.Equal(deliveredAt) {
			t.Fatalf("时窗册按 %s 找, want 原业务时间 %s", at, deliveredAt)
		}
	}

	later := newRecheckFixture(t)
	later.answerCandidates(t, map[string]string{"parcel-a": "customer-A"}, nil, "parcel-a")
	later.scope.preAdvices["PREADV-A"] = preAdviceState(t, "PREADV-A", "customer-A", "node-origin", deliveredAt, scopeMark)
	later.scope.windows = []domain.PreAdviceValidityWindow{scopeWindowFrom(t, deliveredAt.Add(time.Minute), 24*time.Hour, 24*time.Hour)}
	outcome, unconfigured := handleScoped(t, later.receptionFixture, command)
	assertScopeUnverifiedPending(t, "时窗后来才生效", outcome, unconfigured, "UNCONFIGURED//", "parcel-a")
	if result := recheckNamed(t, later, unconfigured.JudgmentVersion); result.Outcome() != application.RecheckStillUnidentified {
		t.Fatalf("时窗收寄之后才生效，重核 = %v, want STILL_UNIDENTIFIED", result.Outcome())
	}
}

// Covers: `AT-NO-062` 末句与 ADR-0212 决定六新补那一格——没有范围时，号码只经客户包裹件号命中一件、另有一份`已提交`委托声明，落只因
// 歧义信号的冲突；那份委托撤回之后按号重核，信号没了而只缺范围：追加一版不记冲突的待识别，候选照记、核对态未证（没带依据），
// 业务时间与控制照前版，不交意图。再响一次门铃，这一版已是不记冲突的待识别、范围仍未证，判断不变。
func TestAnAmbiguityOnlyConflictWithoutAScopeIsReleasedIntoAnUnverifiedPendingUnit(t *testing.T) {
	fixture := newRecheckFixture(t)
	fixture.answerCandidates(t, map[string]string{"parcel-a": "customer-A"}, []string{"customer-B"}, "parcel-a")
	_, conflict := handleScoped(t, fixture.receptionFixture, receiveCommand(t))
	if !conflict.IdentityConflict || conflict.IdentityConflictCause != ports.ConflictByAmbiguityOnly {
		t.Fatalf("收寄 = %+v, want 只因歧义信号的身份冲突", conflict)
	}

	fixture.identity.declaredBy = nil
	results := recheckByMark(t, fixture)
	if len(results) != 1 || results[0].Outcome() != application.RecheckAmbiguityGoneScopeUnverified {
		t.Fatalf("声明撤除之后按号重核 = %d 件, want 恰一件 AMBIGUITY_GONE_SCOPE_UNVERIFIED", len(results))
	}
	released, _ := results[0].Record()
	assertScopeUnverifiedPending(t, "解开之后", application.UnitPendingIdentification, released, "UNVERIFIED/NO_BASIS/", "parcel-a")
	if released.Predecessor != conflict.JudgmentVersion || !released.Intake.ReceivedAt().Equal(deliveredAt) ||
		released.Control.Basis() != conflict.Control.Basis() || len(fixture.downstream.intents) != 0 {
		t.Fatalf("追加版 = %+v、意图 %d, want 前版是冲突版、业务时间与控制照前版、不交意图", released, len(fixture.downstream.intents))
	}

	rows := fixture.rows()
	again := recheckByMark(t, fixture)
	if len(again) != 1 || again[0].Outcome() != application.RecheckStillUnidentified || fixture.rows() != rows {
		t.Fatalf("再响一次门铃 = %v、%d → %d 行, want STILL_UNIDENTIFIED、不追加", again, rows, fixture.rows())
	}
}

// Covers: `AT-NO-062` 有范围那一格与决定八——带 A 的预报送站，范围外另有 B 那件、范围内 A 另有一份`已提交`委托也报了这个号：冲突来由按
// 范围相容的候选分，记只因歧义信号；那份声明撤除之后按号重核照常解开，追加关联 A 那一件的版本，意图一封。
func TestAnAmbiguityOnlyConflictWithAScopeIsReleasedIntoAnIntake(t *testing.T) {
	fixture := newRecheckFixture(t)
	fixture.answerCandidates(t, map[string]string{"parcel-a": "customer-A", "parcel-b": "customer-B"}, []string{"customer-A"},
		"parcel-a", "parcel-b")
	command := receiveCommand(t)
	command.ScopeBasis = fixture.preAdviseA(t)
	_, conflict := handleScoped(t, fixture.receptionFixture, command)
	if !conflict.IdentityConflict || conflict.IdentityConflictCause != ports.ConflictByAmbiguityOnly {
		t.Fatalf("收寄 = %+v, want 只因歧义信号的身份冲突（范围外的 B 不使它记多候选）", conflict)
	}

	fixture.identity.declaredBy = nil
	results := recheckByMark(t, fixture)
	if len(results) != 1 || results[0].Outcome() != application.RecheckIntakeFormed {
		t.Fatalf("声明撤除之后按号重核 = %d 件, want 恰一件 INTAKE_FORMED", len(results))
	}
	formed, _ := results[0].Record()
	if association, associated := formed.Intake.Association(); !associated || association.String() != "parcel-a" ||
		formed.Predecessor != conflict.JudgmentVersion || len(fixture.downstream.intents) != 1 {
		t.Fatalf("追加版 = %+v、意图 %d, want 关联 parcel-a、前版是冲突版、意图一封", formed, len(fixture.downstream.intents))
	}
}
