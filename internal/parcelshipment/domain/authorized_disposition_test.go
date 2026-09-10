package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉 ADR-0132 在领域层的落点：`等待授权处置`是接受判断任务上独立的等待态，由受限项采用
// 的失败处置分路进入；授权角色对当前提交版本**决定去向**（`拒绝` / `交客户补充`），放行不在集内，
// 处置记录一版至多一次、不改写受限项的`业务限制`。

var disposedAt = time.Date(2026, 8, 8, 9, 30, 0, 0, time.UTC)

// adoptedDisposition 造一份受限项上的采用引用：失败处置 × 责任引用成对在场。
func adoptedDisposition(t *testing.T, disposition domain.ControlFailureDisposition) domain.AdoptedControlDisposition {
	t.Helper()
	adopted, err := domain.NewAdoptedControlDisposition(
		disposition,
		mustValue(t, domain.NewControlResponsibilityReference, "CONTRACT-CLAUSE-7"),
	)
	if err != nil {
		t.Fatalf("new adopted control disposition: %v", err)
	}
	return adopted
}

// restrictedControlWith 造「第一项冻结成立、第二项信用受限」的采用结果，并把受限项的处置按给定值采用；
// 传零值即不采用（SA 刚交回、还没读正文，或 0021 之前的存量行）。
func restrictedControlWith(t *testing.T, disposition domain.ControlFailureDisposition) domain.FinancialControlResult {
	t.Helper()
	result := executedControl(t,
		controlItem(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied),
		controlItem(t, domain.CreditCheckControlItem, 2, domain.ControlItemRestricted),
	)
	if disposition == domain.ControlFailureDispositionInvalid {
		return result
	}
	adopted, err := result.AdoptControlDispositions(map[domain.ControlItemKind]domain.AdoptedControlDisposition{
		domain.CreditCheckControlItem: adoptedDisposition(t, disposition),
	})
	if err != nil {
		t.Fatalf("adopt control dispositions: %v", err)
	}
	return adopted
}

// checksAwaitingDisposition 是一轮其余全部通过、接受前财务控制译成`无法判定`且续办路径为
// 「授权处置」的校验。
func checksAwaitingDisposition(t *testing.T) []domain.AcceptanceCheck {
	t.Helper()
	control, err := domain.FinancialControlCheckFor(restrictedControlWith(t, domain.AuthorizedDispositionOnControlFailure))
	if err != nil {
		t.Fatalf("financial control check: %v", err)
	}
	checks := make([]domain.AcceptanceCheck, 0, 8)
	for _, check := range allGroupsPassing(t) {
		if check.Group() == domain.PreAcceptanceFinancialControlCheck {
			continue
		}
		checks = append(checks, check)
	}
	return append(checks, control)
}

// awaitingDisposition 造一份停在`等待授权处置`上的`已提交`委托。
func awaitingDisposition(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	waiting, err := submitted(t).Decide(decisionSpec(t, checksAwaitingDisposition(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	path, present := waiting.AcceptanceDecisionTask().WaitingOn()
	if !present || path != domain.ResumeByAuthorizedDisposition {
		t.Fatalf("fixture waitingOn = %q (present=%v), want AUTHORIZED_DISPOSITION", path, present)
	}
	return waiting
}

func authorizedDisposition(t *testing.T, choice domain.AuthorizedDispositionChoice) domain.AuthorizedDisposition {
	t.Helper()
	disposition, err := domain.NewAuthorizedDisposition(domain.AuthorizedDispositionSpec{
		Choice:     choice,
		Authority:  mustValue(t, domain.NewDispositionAuthorityReference, "PC-DISPOSE-ROLE-1"),
		Disposer:   mustValue(t, domain.NewDisposerReference, "CREDIT-OFFICER-1"),
		Reason:     mustValue(t, domain.NewDispositionReasonReference, "CREDIT_LIMIT_NOT_EXTENDED"),
		Evidence:   mustValue(t, domain.NewDispositionEvidenceReference, "EVID-DISPOSE-1"),
		DisposedAt: disposedAt,
	})
	if err != nil {
		t.Fatalf("new authorized disposition: %v", err)
	}
	return disposition
}

func disposeSpec(t *testing.T, choice domain.AuthorizedDispositionChoice) domain.DisposeUnderAuthoritySpec {
	t.Helper()
	spec := domain.DisposeUnderAuthoritySpec{Disposition: authorizedDisposition(t, choice)}
	if choice == domain.DisposeByRejection {
		spec.DecisionID = mustValue(t, domain.NewAcceptanceDecisionID, "decision-dispose-1")
	}
	return spec
}

// Covers: ADR-0132 决定二——`等待授权处置`是独立的续办路径，与其余等待态互不相同；名字漏补会让
// 处理尝试记不进库。
func TestAuthorizedDispositionIsAResumePathOfItsOwn(t *testing.T) {
	if got := domain.ResumeByAuthorizedDisposition.String(); got != "AUTHORIZED_DISPOSITION" {
		t.Fatalf("resume path name = %q, want AUTHORIZED_DISPOSITION", got)
	}
	others := []domain.ResumePath{
		domain.ResumeByCustomerSupplement,
		domain.ResumeByInternalRetry,
		domain.ResumeByManualReview,
		domain.ResumeByOperatorRegistration,
	}
	for _, other := range others {
		if other == domain.ResumeByAuthorizedDisposition || other.String() == domain.ResumeByAuthorizedDisposition.String() {
			t.Fatalf("`等待授权处置`与 %q 撞车", other)
		}
	}
	// 未决校验可以指名它为续办方：这是 Decide 认出这一格的唯一入口。
	if _, err := domain.NewUndeterminedAcceptanceCheck(
		domain.PreAcceptanceFinancialControlCheck,
		domain.DeclaredParcelID{},
		mustValue(t, domain.NewCheckReason, "CREDIT_CHECK_INSUFFICIENT"),
		domain.ResumeByAuthorizedDisposition,
	); err != nil {
		t.Fatalf("an undetermined check could not name authorized disposition as its resume path: %v", err)
	}
}

// Covers: ADR-0132 决定三、四——失败处置与责任引用作为**采用引用**成对记在受限项上：本上下文自有
// 封闭集镜像 PC 词汇（集外报错不吸收，ADR-0025），成立项两格必空，受限项一次采用不覆盖。
func TestARestrictedItemAdoptsItsFailureDispositionAndResponsibilityTogether(t *testing.T) {
	responsibility := mustValue(t, domain.NewControlResponsibilityReference, "CONTRACT-CLAUSE-7")

	if _, err := domain.NewAdoptedControlDisposition(domain.RejectOnControlFailure, domain.ControlResponsibilityReference{}); !errors.Is(
		err, domain.ErrInvalidControlItemResult,
	) {
		t.Fatalf("err = %v, want ErrInvalidControlItemResult——没有责任引用的处置被采用了", err)
	}
	if _, err := domain.NewAdoptedControlDisposition(domain.ControlFailureDispositionInvalid, responsibility); !errors.Is(
		err, domain.ErrInvalidControlItemResult,
	) {
		t.Fatalf("err = %v, want ErrInvalidControlItemResult——集外处置被采用了", err)
	}
	if _, err := domain.NewAdoptedControlDisposition(domain.AuthorizedDispositionOnControlFailure+1, responsibility); !errors.Is(
		err, domain.ErrInvalidControlItemResult,
	) {
		t.Fatalf("err = %v, want ErrInvalidControlItemResult——上界之外的处置被采用了", err)
	}
	if got := domain.RejectOnControlFailure.String(); got != "REJECT" {
		t.Fatalf("REJECT name = %q", got)
	}
	if got := domain.AuthorizedDispositionOnControlFailure.String(); got != "AUTHORIZED_DISPOSITION" {
		t.Fatalf("AUTHORIZED_DISPOSITION name = %q", got)
	}

	adopted := adoptedDisposition(t, domain.AuthorizedDispositionOnControlFailure)
	satisfied := controlItem(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied)
	if _, err := satisfied.WithAdoptedDisposition(adopted); !errors.Is(err, domain.ErrInvalidControlItemResult) {
		t.Fatalf("err = %v, want ErrInvalidControlItemResult——成立项不该带失败处置", err)
	}

	restricted := controlItem(t, domain.CreditCheckControlItem, 2, domain.ControlItemRestricted)
	if _, present := restricted.AdoptedDisposition(); present {
		t.Fatal("刚由 SA 交回的受限项已经带着处置——处置要经本上下文自己的商业缝读回")
	}
	withDisposition, err := restricted.WithAdoptedDisposition(adopted)
	if err != nil {
		t.Fatalf("with adopted disposition: %v", err)
	}
	got, present := withDisposition.AdoptedDisposition()
	if !present || got.FailureDisposition() != domain.AuthorizedDispositionOnControlFailure ||
		got.Responsibility() != responsibility {
		t.Fatalf("adopted = %+v (present=%v)", got, present)
	}
	if withDisposition.Conclusion() != domain.ControlItemRestricted || withDisposition.Basis() != restricted.Basis() {
		t.Fatal("采用处置改写了受限项的结论或原因——`业务限制`必须原样保留")
	}
	if _, err := withDisposition.WithAdoptedDisposition(adoptedDisposition(t, domain.RejectOnControlFailure)); !errors.Is(
		err, domain.ErrInvalidControlItemResult,
	) {
		t.Fatalf("err = %v, want ErrInvalidControlItemResult——已采用的处置被第二次采用覆盖了", err)
	}
}

// Covers: ADR-0132 决定三——采用只落在受限项上、按控制种类对行；结论、逐项顺序与占用事实都不因
// 采用而变；有受限项对不上处置时整份不成立（对不上是换版竞争或坏数据，不折成任一去向）。
func TestAResultAdoptsDispositionsOntoItsRestrictedItemsOnly(t *testing.T) {
	plain := restrictedControlWith(t, domain.ControlFailureDispositionInvalid)
	adopted := restrictedControlWith(t, domain.AuthorizedDispositionOnControlFailure)

	if adopted.Outcome() != plain.Outcome() || adopted.Basis() != plain.Basis() ||
		adopted.ResultID() != plain.ResultID() || adopted.OccupationFormed() != plain.OccupationFormed() {
		t.Fatal("采用处置改写了结论、依据、标识或占用事实")
	}
	items := adopted.Items()
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if _, present := items[0].AdoptedDisposition(); present || items[0].Kind() != domain.PrepaidFreezeControlItem {
		t.Fatalf("成立项 %q 带上了处置", items[0].Kind())
	}
	disposition, present := items[1].AdoptedDisposition()
	if !present || disposition.FailureDisposition() != domain.AuthorizedDispositionOnControlFailure {
		t.Fatalf("受限项的处置 = %+v (present=%v)", disposition, present)
	}

	// 只给了成立项那一种的处置，受限项没有对上——不成立，也不折成 REJECT。
	if _, err := plain.AdoptControlDispositions(map[domain.ControlItemKind]domain.AdoptedControlDisposition{
		domain.PrepaidFreezeControlItem: adoptedDisposition(t, domain.RejectOnControlFailure),
	}); !errors.Is(err, domain.ErrControlDispositionNotAdopted) {
		t.Fatalf("err = %v, want ErrControlDispositionNotAdopted", err)
	}
	// `明确无控制`没有受限项可采用。
	inapplicable, err := domain.NewInapplicableFinancialControlResult(
		mustValue(t, domain.NewControlBasisReference, "PC-NO-CONTROL-1"), financialControlAsOf(t))
	if err != nil {
		t.Fatalf("new inapplicable result: %v", err)
	}
	if _, err := inapplicable.AdoptControlDispositions(nil); !errors.Is(err, domain.ErrInvalidFinancialControlResult) {
		t.Fatalf("err = %v, want ErrInvalidFinancialControlResult", err)
	}
}

// Covers: ADR-0132 决定一、二的译法——`RESTRICTED` 且全部受限项 `AUTHORIZED_DISPOSITION` →
// `无法判定` + 续办路径「授权处置」，原因取判断顺序最靠前的受限项自己的原因；任一 `REJECT` →
// `未通过`照今天；没有采用处置的受限项（0021 之前的存量行）仍按 ADR-0125 的过渡口径译`未通过`。
func TestFinancialControlCheckRoutesARestrictedResultByItsAdoptedDisposition(t *testing.T) {
	awaiting, err := domain.FinancialControlCheckFor(restrictedControlWith(t, domain.AuthorizedDispositionOnControlFailure))
	if err != nil {
		t.Fatalf("financial control check: %v", err)
	}
	if awaiting.Outcome() != domain.CheckUndetermined {
		t.Fatalf("outcome = %q, want UNDETERMINED——登了授权处置的受限控制被自动拒绝了", awaiting.Outcome())
	}
	if awaiting.ResumePath() != domain.ResumeByAuthorizedDisposition {
		t.Fatalf("resume path = %q, want AUTHORIZED_DISPOSITION", awaiting.ResumePath())
	}
	if awaiting.Reason().String() != "CREDIT_CHECK_INSUFFICIENT" {
		t.Fatalf("reason = %q, want the earliest restricted item's own basis", awaiting.Reason())
	}
	if awaiting.DeclaredParcelID().String() != "" {
		t.Fatal("控制作用在整份委托上，不指名成员")
	}

	rejecting, err := domain.FinancialControlCheckFor(restrictedControlWith(t, domain.RejectOnControlFailure))
	if err != nil {
		t.Fatalf("financial control check: %v", err)
	}
	if rejecting.Outcome() != domain.CheckFailed {
		t.Fatalf("outcome = %q, want FAILED——正文登 REJECT 的受限控制照今天拒绝", rejecting.Outcome())
	}

	legacy, err := domain.FinancialControlCheckFor(restrictedControlWith(t, domain.ControlFailureDispositionInvalid))
	if err != nil {
		t.Fatalf("financial control check: %v", err)
	}
	if legacy.Outcome() != domain.CheckFailed {
		t.Fatalf("outcome = %q, want FAILED——未采用处置的受限项只能按过渡口径拒绝，不得凭空进入授权处置", legacy.Outcome())
	}

	// 两项都受限、一 REJECT 一 AUTHORIZED_DISPOSITION：一行 REJECT 已替合同定了去向（决定一末段）。
	mixed := executedControl(t,
		controlItem(t, domain.PrepaidFreezeControlItem, 1, domain.ControlItemRestricted),
		controlItem(t, domain.CreditCheckControlItem, 2, domain.ControlItemRestricted),
	)
	mixed, err = mixed.AdoptControlDispositions(map[domain.ControlItemKind]domain.AdoptedControlDisposition{
		domain.PrepaidFreezeControlItem: adoptedDisposition(t, domain.AuthorizedDispositionOnControlFailure),
		domain.CreditCheckControlItem:   adoptedDisposition(t, domain.RejectOnControlFailure),
	})
	if err != nil {
		t.Fatalf("adopt control dispositions: %v", err)
	}
	mixedCheck, err := domain.FinancialControlCheckFor(mixed)
	if err != nil {
		t.Fatalf("financial control check: %v", err)
	}
	if mixedCheck.Outcome() != domain.CheckFailed {
		t.Fatalf("outcome = %q, want FAILED——任一 REJECT 即确定不成立", mixedCheck.Outcome())
	}
}

// Covers: ADR-0132 决定二——Decide 从一条续办路径为「授权处置」的未决校验里认出`等待授权处置`并写下；
// 委托保持`已提交`、不形成决定。次序：客户侧缺口仍排最前（外部通知与补充期限），授权处置压过
// 运营登记与内部重试——处置角色的一个去向本就是`交客户补充`，而登记与重试都产不出一次处置。
func TestDecideParksARequestAwaitingAuthorizedDisposition(t *testing.T) {
	waiting := awaitingDisposition(t)
	if waiting.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; 等处置不是决定", waiting.State())
	}
	if _, formed := waiting.AcceptanceDecision(); formed {
		t.Fatal("等处置形成了一份决定")
	}

	disposition := undeterminedCheck(
		t, domain.PreAcceptanceFinancialControlCheck, "CREDIT_CHECK_INSUFFICIENT", domain.ResumeByAuthorizedDisposition)
	registration := undeterminedCheck(
		t, domain.CustomerRelationshipCheck, "AUTHORITY_RULES_NOT_CONFIGURED", domain.ResumeByOperatorRegistration)
	retry := undeterminedCheck(
		t, domain.LegalEntityAndContractCheck, "DEPENDENCY_TIMEOUT", domain.ResumeByInternalRetry)
	supplement := undeterminedCheck(
		t, domain.RequiredDocumentCheck, "DOCUMENT_PENDING", domain.ResumeByCustomerSupplement)
	cases := []struct {
		name   string
		checks []domain.AcceptanceCheck
		want   domain.ResumePath
	}{
		{"disposition beats internal retry", []domain.AcceptanceCheck{retry, disposition}, domain.ResumeByAuthorizedDisposition},
		{"disposition beats registration", []domain.AcceptanceCheck{registration, disposition}, domain.ResumeByAuthorizedDisposition},
		{"customer supplement beats disposition", []domain.AcceptanceCheck{disposition, supplement}, domain.ResumeByCustomerSupplement},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decided, err := submitted(t).Decide(decisionSpec(t, append(allGroupsPassing(t), tc.checks...)))
			if err != nil {
				t.Fatalf("decide: %v", err)
			}
			got, _ := decided.AcceptanceDecisionTask().WaitingOn()
			if got != tc.want {
				t.Fatalf("waitingOn = %q, want %q", got, tc.want)
			}
		})
	}
}

// Covers: ADR-0132 决定二「处置记录……去向 × 实际处置方 × 授权引用 × 原因引用 × 证据引用 × 处置时点」
// ——少授权说不出凭什么算数、少处置方无从追责、少证据与「有人点了一下」分不开；去向集外（放行）
// 在构造期就不成立。
func TestAnAuthorizedDispositionNeedsAllOfItsReferences(t *testing.T) {
	complete := domain.AuthorizedDispositionSpec{
		Choice:     domain.DisposeByCustomerSupplement,
		Authority:  mustValue(t, domain.NewDispositionAuthorityReference, "PC-DISPOSE-ROLE-1"),
		Disposer:   mustValue(t, domain.NewDisposerReference, "CREDIT-OFFICER-1"),
		Reason:     mustValue(t, domain.NewDispositionReasonReference, "CREDIT_LIMIT_NOT_EXTENDED"),
		Evidence:   mustValue(t, domain.NewDispositionEvidenceReference, "EVID-DISPOSE-1"),
		DisposedAt: disposedAt,
	}
	if _, err := domain.NewAuthorizedDisposition(complete); err != nil {
		t.Fatalf("a complete disposition was refused: %v", err)
	}

	mutations := map[string]func(*domain.AuthorizedDispositionSpec){
		"choice":    func(spec *domain.AuthorizedDispositionSpec) { spec.Choice = domain.AuthorizedDispositionChoiceInvalid },
		"release":   func(spec *domain.AuthorizedDispositionSpec) { spec.Choice = domain.DisposeByCustomerSupplement + 1 },
		"authority": func(spec *domain.AuthorizedDispositionSpec) { spec.Authority = domain.DispositionAuthorityReference{} },
		"disposer":  func(spec *domain.AuthorizedDispositionSpec) { spec.Disposer = domain.DisposerReference{} },
		"reason":    func(spec *domain.AuthorizedDispositionSpec) { spec.Reason = domain.DispositionReasonReference{} },
		"evidence":  func(spec *domain.AuthorizedDispositionSpec) { spec.Evidence = domain.DispositionEvidenceReference{} },
		"time":      func(spec *domain.AuthorizedDispositionSpec) { spec.DisposedAt = time.Time{} },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			spec := complete
			mutate(&spec)
			if _, err := domain.NewAuthorizedDisposition(spec); !errors.Is(err, domain.ErrInvalidAuthorizedDisposition) {
				t.Fatalf("err = %v, want ErrInvalidAuthorizedDisposition", err)
			}
		})
	}
	if got := domain.DisposeByRejection.String(); got != "REJECT" {
		t.Fatalf("REJECT choice name = %q", got)
	}
	if got := domain.DisposeByCustomerSupplement.String(); got != "CUSTOMER_SUPPLEMENT" {
		t.Fatalf("CUSTOMER_SUPPLEMENT choice name = %q", got)
	}
}

// Covers: ADR-0132 决定一`拒绝`——形成授权角色拒绝决定，留痕同主动拒绝（实际决定方 = 处置方、授权
// 依据、结构化原因、证据、决定时间），任务完成、等待态清零，处置记录留在任务上。
func TestDisposingByRejectionFormsAnAuthorizedRejection(t *testing.T) {
	waiting := awaitingDisposition(t)

	rejected, err := waiting.DisposeUnderAuthority(disposeSpec(t, domain.DisposeByRejection))
	if err != nil {
		t.Fatalf("dispose under authority: %v", err)
	}
	if rejected.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", rejected.State())
	}
	decision, formed := rejected.AcceptanceDecision()
	if !formed || decision.Accepted() {
		t.Fatalf("decision formed=%v accepted=%v; want a rejection", formed, decision.Accepted())
	}
	if decision.DecisionID().String() != "decision-dispose-1" || !decision.DecidedAt().Equal(disposedAt) {
		t.Fatalf("decision id=%q decidedAt=%v", decision.DecisionID(), decision.DecidedAt())
	}
	rejection, active := decision.ActiveRejection()
	if !active {
		t.Fatal("处置形成的拒绝没有授权角色留痕——与规则形成的拒绝分不开")
	}
	if rejection.Decider().String() != "CREDIT-OFFICER-1" ||
		rejection.Authority().String() != "PC-DISPOSE-ROLE-1" ||
		rejection.Reason().String() != "CREDIT_LIMIT_NOT_EXTENDED" ||
		rejection.Evidence().String() != "EVID-DISPOSE-1" {
		t.Fatalf("rejection trail = %+v", rejection)
	}
	// 依据经采用结果回指、不复制：决定上不带校验明细，也不带责任引用。
	if len(decision.Checks()) != 0 {
		t.Fatalf("处置拒绝复制了 %d 条校验进决定", len(decision.Checks()))
	}
	task := rejected.AcceptanceDecisionTask()
	if !task.IsComplete() {
		t.Fatal("拒绝之后任务仍未完成")
	}
	if _, stillWaiting := task.WaitingOn(); stillWaiting {
		t.Fatal("拒绝之后等待态没有清零")
	}
	recorded, present := task.AuthorizedDisposition()
	if !present || recorded.Choice() != domain.DisposeByRejection || recorded.Disposer().String() != "CREDIT-OFFICER-1" {
		t.Fatalf("recorded disposition = %+v (present=%v)", recorded, present)
	}
	if _, baseline := rejected.AcceptanceBaseline(); baseline {
		t.Fatal("拒绝形成了接受基线")
	}
	if rejected.Revision() != waiting.Revision() {
		t.Fatal("转移动了聚合版本")
	}
}

// Covers: ADR-0132 决定一`交客户补充`——本版本不再判，等待态转到`等待受控补充`（由既有「新提交版本
// 已形成」信封续办），不形成决定、不动状态、不动版本，处置记录留在任务上。
func TestDisposingToCustomerSupplementMovesTheWaitWithoutDeciding(t *testing.T) {
	waiting := awaitingDisposition(t)

	handed, err := waiting.DisposeUnderAuthority(disposeSpec(t, domain.DisposeByCustomerSupplement))
	if err != nil {
		t.Fatalf("dispose under authority: %v", err)
	}
	if handed.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED", handed.State())
	}
	if _, formed := handed.AcceptanceDecision(); formed {
		t.Fatal("交客户补充形成了一份决定")
	}
	task := handed.AcceptanceDecisionTask()
	if task.IsComplete() || task.IsStopped() {
		t.Fatal("交客户补充收了任务的工——它还等着新版本")
	}
	path, present := task.WaitingOn()
	if !present || path != domain.ResumeByCustomerSupplement {
		t.Fatalf("waitingOn = %q (present=%v), want CUSTOMER_SUPPLEMENT", path, present)
	}
	recorded, present := task.AuthorizedDisposition()
	if !present || recorded.Choice() != domain.DisposeByCustomerSupplement {
		t.Fatalf("recorded disposition = %+v (present=%v)", recorded, present)
	}
	if !recorded.DisposedAt().Equal(disposedAt) || recorded.Authority().String() != "PC-DISPOSE-ROLE-1" ||
		recorded.Reason().String() != "CREDIT_LIMIT_NOT_EXTENDED" || recorded.Evidence().String() != "EVID-DISPOSE-1" {
		t.Fatalf("recorded disposition = %+v", recorded)
	}
	if handed.Revision() != waiting.Revision() {
		t.Fatal("转移动了聚合版本")
	}
	// 新提交版本照旧能形成：受控补充那条路没被处置堵住。
	if _, err := handed.FormNewSubmissionVersion(supersessionSpecFor(t, "parcel-1", "parcel-2")); err != nil {
		t.Fatalf("form new submission version after disposition: %v", err)
	}
}

// Covers: ADR-0132 决定二——处置命令的前置：任务停在`等待授权处置`；未越过决定边界；一版至多一次。
// 撞上已成立的决定说出真实原因（`任务已完结`由编排据以交回那一个决定）。
func TestDisposeUnderAuthorityRefusesOutsideItsWaitState(t *testing.T) {
	if _, err := submitted(t).DisposeUnderAuthority(disposeSpec(t, domain.DisposeByRejection)); !errors.Is(
		err, domain.ErrNotWaitingOnAuthorizedDisposition,
	) {
		t.Fatalf("err = %v, want ErrNotWaitingOnAuthorizedDisposition——没停在等处置的委托被处置了", err)
	}

	reviewing, err := submitted(t).Decide(decisionSpec(t, append(allGroupsPassing(t), undeterminedCheck(
		t, domain.RequiredDocumentCheck, "DOCUMENT_PENDING", domain.ResumeByCustomerSupplement))))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if _, err := reviewing.DisposeUnderAuthority(disposeSpec(t, domain.DisposeByRejection)); !errors.Is(
		err, domain.ErrNotWaitingOnAuthorizedDisposition,
	) {
		t.Fatalf("err = %v, want ErrNotWaitingOnAuthorizedDisposition——停在别的等待态也被处置了", err)
	}

	decided, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if _, err := decided.DisposeUnderAuthority(disposeSpec(t, domain.DisposeByRejection)); !errors.Is(
		err, domain.ErrDecisionAlreadyFormed,
	) {
		t.Fatalf("err = %v, want ErrDecisionAlreadyFormed", err)
	}

	handed, err := awaitingDisposition(t).DisposeUnderAuthority(disposeSpec(t, domain.DisposeByCustomerSupplement))
	if err != nil {
		t.Fatalf("dispose under authority: %v", err)
	}
	if _, err := handed.DisposeUnderAuthority(disposeSpec(t, domain.DisposeByCustomerSupplement)); !errors.Is(
		err, domain.ErrAuthorizedDispositionAlreadyRecorded,
	) {
		t.Fatalf("err = %v, want ErrAuthorizedDispositionAlreadyRecorded——同一版本被处置了两次", err)
	}

	// `拒绝`去向要形成决定，决定标识必填；`交客户补充`不形成决定，不收标识。
	spec := disposeSpec(t, domain.DisposeByRejection)
	spec.DecisionID = domain.AcceptanceDecisionID{}
	if _, err := awaitingDisposition(t).DisposeUnderAuthority(spec); !errors.Is(err, domain.ErrInvalidAcceptanceDecision) {
		t.Fatalf("err = %v, want ErrInvalidAcceptanceDecision", err)
	}
	if _, err := awaitingDisposition(t).DisposeUnderAuthority(domain.DisposeUnderAuthoritySpec{}); !errors.Is(
		err, domain.ErrInvalidAuthorizedDisposition,
	) {
		t.Fatalf("err = %v, want ErrInvalidAuthorizedDisposition", err)
	}
}

// Covers: CONTEXT「人工处理不得绕过硬规则或把缺少的权威结果改成通过」——处置记录不是复核完成：
// 一份已处置为`交客户补充`的版本再被判断，接受前财务控制那一组仍然不通过，Decide 不会因为任务上
// 有一条处置记录就接受；去向既已选定，等待态也留在`等待受控补充`而不是退回等处置。
func TestADispositionRecordNeverPassesTheFinancialControlGroup(t *testing.T) {
	handed, err := awaitingDisposition(t).DisposeUnderAuthority(disposeSpec(t, domain.DisposeByCustomerSupplement))
	if err != nil {
		t.Fatalf("dispose under authority: %v", err)
	}

	rejudged, err := handed.Decide(decisionSpec(t, checksAwaitingDisposition(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if rejudged.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; 一条处置记录把受限控制判成了通过", rejudged.State())
	}
	if _, formed := rejudged.AcceptanceDecision(); formed {
		t.Fatal("处置记录之后 Decide 形成了决定")
	}
	path, _ := rejudged.AcceptanceDecisionTask().WaitingOn()
	if path != domain.ResumeByCustomerSupplement {
		t.Fatalf("waitingOn = %q, want CUSTOMER_SUPPLEMENT——去向已选，不退回等处置", path)
	}

	// 受限项登 REJECT 时照今天拒绝，处置记录同样帮不上。
	failing, err := domain.FinancialControlCheckFor(restrictedControlWith(t, domain.RejectOnControlFailure))
	if err != nil {
		t.Fatalf("financial control check: %v", err)
	}
	checks := checksAwaitingDisposition(t)
	checks[len(checks)-1] = failing
	rejected, err := handed.Decide(decisionSpec(t, checks))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if rejected.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", rejected.State())
	}
}

// Covers: ADR-0028 重建只校验不重算——处置记录随任务快照往返；一条已处置的运行中任务不可能仍停在
// `等待授权处置`（处置转移把它带走了），这样的行是坏数据。
func TestRehydrationCarriesTheAuthorizedDisposition(t *testing.T) {
	snapshot := submittedSnapshot(t)
	snapshot.AcceptanceTask.WaitingOn = domain.ResumeByCustomerSupplement
	snapshot.AcceptanceTask.AuthorizedDisposition = authorizedDisposition(t, domain.DisposeByCustomerSupplement)

	request, err := domain.RehydrateShipmentRequest(snapshot)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	recorded, present := request.AcceptanceDecisionTask().AuthorizedDisposition()
	if !present || recorded.Choice() != domain.DisposeByCustomerSupplement || recorded.Disposer().String() != "CREDIT-OFFICER-1" {
		t.Fatalf("recorded disposition = %+v (present=%v)", recorded, present)
	}
	if _, err := request.DisposeUnderAuthority(disposeSpec(t, domain.DisposeByCustomerSupplement)); !errors.Is(
		err, domain.ErrAuthorizedDispositionAlreadyRecorded,
	) {
		t.Fatalf("err = %v, want ErrAuthorizedDispositionAlreadyRecorded——重建后的处置记录没有守住一版一次", err)
	}

	stale := submittedSnapshot(t)
	stale.AcceptanceTask.WaitingOn = domain.ResumeByAuthorizedDisposition
	stale.AcceptanceTask.AuthorizedDisposition = authorizedDisposition(t, domain.DisposeByCustomerSupplement)
	if _, err := domain.RehydrateShipmentRequest(stale); !errors.Is(err, domain.ErrInvalidRehydratedShipmentRequest) {
		t.Fatalf("err = %v, want ErrInvalidRehydratedShipmentRequest——已处置却仍停在等处置的任务被收下了", err)
	}

	plain := submittedSnapshot(t)
	plain.AcceptanceTask.WaitingOn = domain.ResumeByAuthorizedDisposition
	waiting, err := domain.RehydrateShipmentRequest(plain)
	if err != nil {
		t.Fatalf("rehydrate a request awaiting disposition: %v", err)
	}
	if _, present := waiting.AcceptanceDecisionTask().AuthorizedDisposition(); present {
		t.Fatal("没处置过的任务重建后带着一条处置记录")
	}
}
