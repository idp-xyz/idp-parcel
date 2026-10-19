package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证可验证交付范围的推导（ADR-0212 决定二、三、五；NO CONTEXT「可验证交付范围」）：依据取哪一份、预报那组条件逐条判、
// 交接单全有或全无、时窗没登记答未配置。推导是纯函数：证据状态由调用方读好交进来。夹具全为合成（S 级）。

var scopeReceivedAt = preAdviceExpectedAt.Add(-2 * time.Hour)

func preAdviceBasis(t *testing.T, id string) domain.DeliveryScopeBasisReference {
	t.Helper()
	basis, err := domain.NewDeliveryScopeBasisReference(domain.BasisDeliveryPreAdvice, id)
	if err != nil {
		t.Fatal(err)
	}
	return basis
}

func registeredWindow(t *testing.T) domain.PreAdviceValidityWindow {
	t.Helper()
	window, err := domain.FormPreAdviceValidityWindow(windowSpec(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	return window
}

// preAdviceDerivation 是预报那一支各条件都满足的输入：同租户、节点相同、未撤回、业务时间在时窗里、号码在列。
func preAdviceDerivation(t *testing.T) domain.DeliveryScopeDerivation {
	t.Helper()
	preAdvice, err := domain.FormDeliveryPreAdvice(preAdviceSpec(t, "SYN-CPN-A1", "SYN-CPN-A2"))
	if err != nil {
		t.Fatal(err)
	}
	return domain.DeliveryScopeDerivation{
		TenantID:   preAdvice.TenantID(),
		Basis:      domain.UnitDeliveryScopeBasis{Source: preAdviceBasis(t, preAdvice.ID().String())},
		PreAdvice:  domain.DeliveryPreAdviceState{PreAdvice: preAdvice},
		Window:     registeredWindow(t),
		Node:       preAdvice.Node(),
		ReceivedAt: scopeReceivedAt,
		Mark:       "SYN-CPN-A2",
	}
}

func assertUnverified(t *testing.T, name string, scope domain.VerifiableDeliveryScope, reason domain.DeliveryScopeUnverifiedReason, detail domain.DeliveryScopeInapplicability) {
	t.Helper()
	check := scope.Check()
	if check.State() != domain.DeliveryScopeUnverified || check.Reason() != reason || check.Inapplicability() != detail {
		t.Fatalf("%s: check = %s/%s/%s, want UNVERIFIED/%s/%s", name, check.State(), check.Reason(), check.Inapplicability(), reason, detail)
	}
	if len(scope.Accounts()) != 0 {
		t.Fatalf("%s: 未证却给了客户账户 %v", name, scope.Accounts())
	}
}

// Covers: ADR-0212 决定二「对一件实物算数的条件，逐条都满足」，AT-NO-063 推导半边与 AT-NO-070 推导半边——全满足时范围是
// ｛预报的客户账户｝；节点不符、业务时间在时窗外（两端各差一秒）、已撤回各答依据不适用并分格；号码不在列答号码不在预报；号码
// 逐字比对，带空白或大小写不同即不在列；别的租户的预报与不在册同答。
func TestAPreAdviceCountsOnlyWhenEveryConditionHolds(t *testing.T) {
	verified := domain.DeriveVerifiableDeliveryScope(preAdviceDerivation(t))
	if verified.Check().State() != domain.DeliveryScopeVerified {
		t.Fatalf("all conditions hold: check = %s/%s", verified.Check().State(), verified.Check().Reason())
	}
	if accounts := verified.Accounts(); len(accounts) != 1 || accounts[0].String() != "SYN-ACCOUNT-01" {
		t.Fatalf("accounts = %v, want {SYN-ACCOUNT-01}", accounts)
	}

	withdrawal, err := domain.FormDeliveryPreAdviceWithdrawal(domain.DeliveryPreAdviceWithdrawalSpec{
		TenantID:        mustBuild(t, domain.NewTenantID, "SYN-TENANT-01"),
		PreAdvice:       mustBuild(t, domain.NewDeliveryPreAdviceID, "SYN-PREADV-PSXM08-01"),
		CustomerAccount: mustBuild(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
		Source:          domain.CustomerRequestSource{Channel: "SYN-CLIENT", RequestKey: "SYN-REQ-W", ContentDigest: "sha256:W"},
		WithdrawnAt:     scopeReceivedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	type unverified struct {
		mutate func(*domain.DeliveryScopeDerivation)
		reason domain.DeliveryScopeUnverifiedReason
		detail domain.DeliveryScopeInapplicability
	}
	cases := map[string]unverified{
		"节点不符": {func(input *domain.DeliveryScopeDerivation) {
			input.Node = mustBuild(t, domain.NewNodeReference, "SYN-NODE-SZX-GATE")
		}, domain.ScopeBasisNotApplicable, domain.ScopeNodeMismatch},
		"早于提前量一秒": {func(input *domain.DeliveryScopeDerivation) {
			input.ReceivedAt = preAdviceExpectedAt.Add(-24*time.Hour - time.Second)
		}, domain.ScopeBasisNotApplicable, domain.ScopeOutsideWindow},
		"晚于宽限量一秒": {func(input *domain.DeliveryScopeDerivation) {
			input.ReceivedAt = preAdviceExpectedAt.Add(6*time.Hour + time.Second)
		}, domain.ScopeBasisNotApplicable, domain.ScopeOutsideWindow},
		"已撤回": {func(input *domain.DeliveryScopeDerivation) {
			input.PreAdvice.Withdrawal = withdrawal
		}, domain.ScopeBasisNotApplicable, domain.ScopeBasisWithdrawn},
		"号码不在列": {func(input *domain.DeliveryScopeDerivation) {
			input.Mark = "SYN-CPN-B9"
		}, domain.ScopeNumberNotPreAdvised, domain.ScopeInapplicabilityAbsent},
		"号码多一个前导空格": {func(input *domain.DeliveryScopeDerivation) {
			input.Mark = " SYN-CPN-A2"
		}, domain.ScopeNumberNotPreAdvised, domain.ScopeInapplicabilityAbsent},
		"号码大小写不同": {func(input *domain.DeliveryScopeDerivation) {
			input.Mark = "syn-cpn-a2"
		}, domain.ScopeNumberNotPreAdvised, domain.ScopeInapplicabilityAbsent},
		"预报不在册": {func(input *domain.DeliveryScopeDerivation) {
			input.PreAdvice = domain.DeliveryPreAdviceState{}
		}, domain.ScopeBasisNotRegistered, domain.ScopeInapplicabilityAbsent},
		"预报属别的租户": {func(input *domain.DeliveryScopeDerivation) {
			input.TenantID = mustBuild(t, domain.NewTenantID, "SYN-TENANT-02")
		}, domain.ScopeBasisNotRegistered, domain.ScopeInapplicabilityAbsent},
		"预报的标识对不上依据": {func(input *domain.DeliveryScopeDerivation) {
			input.Basis.Source = preAdviceBasis(t, "SYN-PREADV-PSXM08-OTHER")
		}, domain.ScopeBasisNotRegistered, domain.ScopeInapplicabilityAbsent},
		"没带依据": {func(input *domain.DeliveryScopeDerivation) {
			input.Basis = domain.UnitDeliveryScopeBasis{}
		}, domain.ScopeNoBasis, domain.ScopeInapplicabilityAbsent},
	}
	for name, testCase := range cases {
		input := preAdviceDerivation(t)
		testCase.mutate(&input)
		assertUnverified(t, name, domain.DeriveVerifiableDeliveryScope(input), testCase.reason, testCase.detail)
	}
}

// Covers: ADR-0212 决定二末项、决定五「未配置」与 AT-NO-068 推导半边——时窗册在原收寄业务时间没有生效行（一行都没登，或登的行
// 那一刻还没生效）答未配置，与未证分开、不推定默认时长；时窗是唯一挡路的那一格时才答未配置：号码本来就不在列时答未证。
func TestAnUnregisteredWindowAnswersUnconfiguredApartFromUnverified(t *testing.T) {
	noWindow := preAdviceDerivation(t)
	noWindow.Window = domain.PreAdviceValidityWindow{}
	scope := domain.DeriveVerifiableDeliveryScope(noWindow)
	if check := scope.Check(); check.State() != domain.DeliveryScopeUnconfigured || check.Reason() != domain.ScopeUnverifiedReasonAbsent {
		t.Fatalf("no window: check = %s/%s, want UNCONFIGURED without a reason", check.State(), check.Reason())
	}
	if len(scope.Accounts()) != 0 {
		t.Fatal("未配置却给了客户账户")
	}

	notYetEffective := preAdviceDerivation(t)
	later, err := domain.FormPreAdviceValidityWindow(windowSpec(t, scopeReceivedAt.Add(time.Second), time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	notYetEffective.Window = later
	if check := domain.DeriveVerifiableDeliveryScope(notYetEffective).Check(); check.State() != domain.DeliveryScopeUnconfigured {
		t.Fatalf("window not yet effective at the reception's business time: check = %s, want UNCONFIGURED", check.State())
	}

	notListedEither := preAdviceDerivation(t)
	notListedEither.Window = domain.PreAdviceValidityWindow{}
	notListedEither.Mark = "SYN-CPN-B9"
	assertUnverified(t, "号码不在列、时窗也没登", domain.DeriveVerifiableDeliveryScope(notListedEither),
		domain.ScopeNumberNotPreAdvised, domain.ScopeInapplicabilityAbsent)
}

func handoverDerivation(t *testing.T, bindings ...domain.DeliveryHandoverBinding) domain.DeliveryScopeDerivation {
	t.Helper()
	note := handoverNote(t, "SYN-ACCOUNT-01", "SYN-ACCOUNT-02")
	basis, err := domain.NewDeliveryScopeBasisReference(domain.BasisDeliveryHandoverNote, note.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	return domain.DeliveryScopeDerivation{
		TenantID:     note.TenantID(),
		Basis:        domain.UnitDeliveryScopeBasis{Source: basis},
		HandoverNote: domain.DeliveryHandoverNoteState{Note: note, Bindings: bindings},
		// 这件实物收寄带着这张交接单的标识，是它收寄带来的成员。
		HandoverMember: true,
		Node:           note.Node(),
		ReceivedAt:     scopeReceivedAt,
		Mark:           "SYN-CPN-ANY",
	}
}

// Covers: ADR-0212 决定三「全部确认才成立范围」与 AT-NO-074 推导半边——只确认部分时答交接单绑定未全部确认，任一否认同答；全部
// 确认时范围是声称的整组、不看号码；交接单不需要时窗，时窗没登也照样成立；交接单在别的节点开立答依据不适用（节点不符）。
func TestAHandoverNoteScopeIsAllOrNothing(t *testing.T) {
	note := handoverNote(t, "SYN-ACCOUNT-01", "SYN-ACCOUNT-02")
	confirmedA := binding(t, note, "SYN-ACCOUNT-01", domain.BindingConfirmed)
	confirmedB := binding(t, note, "SYN-ACCOUNT-02", domain.BindingConfirmed)
	deniedB := binding(t, note, "SYN-ACCOUNT-02", domain.BindingDenied)

	assertUnverified(t, "只确认 A", domain.DeriveVerifiableDeliveryScope(handoverDerivation(t, confirmedA)),
		domain.ScopeBindingsNotAllConfirmed, domain.ScopeInapplicabilityAbsent)
	assertUnverified(t, "A 确认、B 否认", domain.DeriveVerifiableDeliveryScope(handoverDerivation(t, confirmedA, deniedB)),
		domain.ScopeBindingsNotAllConfirmed, domain.ScopeInapplicabilityAbsent)

	scope := domain.DeriveVerifiableDeliveryScope(handoverDerivation(t, confirmedA, confirmedB))
	if scope.Check().State() != domain.DeliveryScopeVerified {
		t.Fatalf("all confirmed: check = %s/%s", scope.Check().State(), scope.Check().Reason())
	}
	accounts := make([]string, 0, 2)
	for _, account := range scope.Accounts() {
		accounts = append(accounts, account.String())
	}
	if !slices.Equal(accounts, []string{"SYN-ACCOUNT-01", "SYN-ACCOUNT-02"}) {
		t.Fatalf("accounts = %v, want the whole claimed group", accounts)
	}

	elsewhere := handoverDerivation(t, confirmedA, confirmedB)
	elsewhere.Node = mustBuild(t, domain.NewNodeReference, "SYN-NODE-SZX-GATE")
	assertUnverified(t, "交接单在别的节点开立", domain.DeriveVerifiableDeliveryScope(elsewhere),
		domain.ScopeBasisNotApplicable, domain.ScopeNodeMismatch)

	unregistered := handoverDerivation(t)
	unregistered.HandoverNote = domain.DeliveryHandoverNoteState{}
	assertUnverified(t, "交接单不在册", domain.DeriveVerifiableDeliveryScope(unregistered),
		domain.ScopeBasisNotRegistered, domain.ScopeInapplicabilityAbsent)
}

// Covers: ADR-0212 决定三「一件实物的依据取它最后归入的交接单，没有归入过的取来源里的引用」——来源带着一份成立的预报，实物又
// 归入了一张还没全部确认的交接单，判的是交接单；没归入过的照来源判。
func TestTheLastAssignedHandoverNoteTakesPrecedenceOverTheSourceBasis(t *testing.T) {
	note := handoverNote(t, "SYN-ACCOUNT-01", "SYN-ACCOUNT-02")
	confirmedA := binding(t, note, "SYN-ACCOUNT-01", domain.BindingConfirmed)

	input := preAdviceDerivation(t)
	input.Basis.LastAssignedNote = note.ID()
	input.HandoverNote = domain.DeliveryHandoverNoteState{Note: note, Bindings: []domain.DeliveryHandoverBinding{confirmedA}}
	input.HandoverMember = true
	effective := input.Basis.Effective()
	if effective.Kind() != domain.BasisDeliveryHandoverNote || effective.Identifier() != note.ID().String() {
		t.Fatalf("effective basis = %s %s, want the assigned note", effective.Kind(), effective.Identifier())
	}
	assertUnverified(t, "归入了没全部确认的交接单", domain.DeriveVerifiableDeliveryScope(input),
		domain.ScopeBindingsNotAllConfirmed, domain.ScopeInapplicabilityAbsent)

	notAssigned := preAdviceDerivation(t)
	if notAssigned.Basis.Effective().Kind() != domain.BasisDeliveryPreAdvice {
		t.Fatal("没归入过时应取来源里的引用")
	}
	if (domain.UnitDeliveryScopeBasis{}).Effective().Present() {
		t.Fatal("来源没带、也没归入过时应答没有依据")
	}
}

// Covers: ADR-0212 决定五的封闭词表——核对态各取值、未证各原因与依据不适用各细分，线上名逐一往返；组合不对的立不起来
// （已证或未配置带原因、未证不带原因、细分格挂在别的原因上）。依据引用的种类只有送站预报与送站交接单。
func TestTheDeliveryScopeCheckIsAClosedVocabulary(t *testing.T) {
	for _, state := range []domain.DeliveryScopeCheckState{domain.DeliveryScopeVerified, domain.DeliveryScopeUnverified, domain.DeliveryScopeUnconfigured} {
		if parsed, err := domain.ParseDeliveryScopeCheckState(state.String()); err != nil || parsed != state {
			t.Fatalf("state %s round trip = %v %v", state, parsed, err)
		}
	}
	for _, reason := range []domain.DeliveryScopeUnverifiedReason{domain.ScopeNoBasis, domain.ScopeBasisNotRegistered,
		domain.ScopeBasisNotApplicable, domain.ScopeNumberNotPreAdvised, domain.ScopeBindingsNotAllConfirmed} {
		if parsed, err := domain.ParseDeliveryScopeUnverifiedReason(reason.String()); err != nil || parsed != reason {
			t.Fatalf("reason %s round trip = %v %v", reason, parsed, err)
		}
	}
	for _, detail := range []domain.DeliveryScopeInapplicability{domain.ScopeNodeMismatch, domain.ScopeOutsideWindow, domain.ScopeBasisWithdrawn,
		domain.ScopeNotAHandoverMember} {
		if parsed, err := domain.ParseDeliveryScopeInapplicability(detail.String()); err != nil || parsed != detail {
			t.Fatalf("detail %s round trip = %v %v", detail, parsed, err)
		}
	}
	for _, kind := range []domain.DeliveryScopeBasisKind{domain.BasisDeliveryPreAdvice, domain.BasisDeliveryHandoverNote} {
		if parsed, err := domain.ParseDeliveryScopeBasisKind(kind.String()); err != nil || parsed != kind {
			t.Fatalf("basis kind %s round trip = %v %v", kind, parsed, err)
		}
	}
	if _, err := domain.ParseDeliveryScopeBasisKind("delivery_pre_advice"); err == nil {
		t.Fatal("线上名不折大小写")
	}

	if _, err := domain.NewDeliveryScopeCheck(domain.DeliveryScopeUnverified, domain.ScopeBasisNotApplicable, domain.ScopeBasisWithdrawn); err != nil {
		t.Fatalf("依据不适用·已撤回: %v", err)
	}
	refused := map[string][3]uint8{
		"已证带原因":       {uint8(domain.DeliveryScopeVerified), uint8(domain.ScopeNoBasis), 0},
		"未配置带原因":      {uint8(domain.DeliveryScopeUnconfigured), uint8(domain.ScopeNoBasis), 0},
		"未证不带原因":      {uint8(domain.DeliveryScopeUnverified), 0, 0},
		"依据不适用缺细分":    {uint8(domain.DeliveryScopeUnverified), uint8(domain.ScopeBasisNotApplicable), 0},
		"细分挂在号码不在预报上": {uint8(domain.DeliveryScopeUnverified), uint8(domain.ScopeNumberNotPreAdvised), uint8(domain.ScopeOutsideWindow)},
		"零值":          {0, 0, 0},
	}
	for name, parts := range refused {
		if _, err := domain.NewDeliveryScopeCheck(domain.DeliveryScopeCheckState(parts[0]), domain.DeliveryScopeUnverifiedReason(parts[1]),
			domain.DeliveryScopeInapplicability(parts[2])); !errors.Is(err, domain.ErrInvalidDeliveryScopeCheck) {
			t.Fatalf("%s: err = %v, want ErrInvalidDeliveryScopeCheck", name, err)
		}
	}
	if _, err := domain.NewDeliveryScopeBasisReference(domain.DeliveryScopeBasisKindInvalid, "SYN-X"); err == nil {
		t.Fatal("依据种类不在词表却立得起来")
	}
	if _, err := domain.NewDeliveryScopeBasisReference(domain.BasisDeliveryPreAdvice, "  "); err == nil {
		t.Fatal("依据标识为空却立得起来")
	}
}
