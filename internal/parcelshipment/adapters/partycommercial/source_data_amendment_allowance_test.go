package partycommercial_test

import (
	"context"
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// allowanceViewDouble 是 PC 资料修订允许声明读口的替身：答一份固定内容，并记下被问的租户与版本。
type allowanceViewDouble struct {
	content pcdomain.SourceDataAmendmentAllowanceContent
	found   bool
	err     error
	calls   int
	tenant  pcdomain.TenantID
	version pcdomain.CommercialVersion
}

func (double *allowanceViewDouble) LoadSourceDataAmendmentAllowance(
	_ context.Context,
	tenant pcdomain.TenantID,
	rulePackage pcdomain.CommercialVersion,
) (pcdomain.SourceDataAmendmentAllowanceContent, bool, error) {
	double.calls++
	double.tenant = tenant
	double.version = rulePackage
	return double.content, double.found, double.err
}

// absentStageOwner 是「采用版本尚未固定」那一格：闭包不在，found=false。
type absentStageOwner struct{ calls int }

func (owner *absentStageOwner) AcceptanceRulePackageFor(
	context.Context,
	psdomain.SourceIdentity,
) (pcdomain.CommercialVersion, bool, error) {
	owner.calls++
	return pcdomain.CommercialVersion{}, false, nil
}

func (owner *absentStageOwner) AuthorizationRuleFor(
	context.Context,
	psdomain.SourceIdentity,
) (pcdomain.CommercialVersion, bool, error) {
	return pcdomain.CommercialVersion{}, false, nil
}

type failingStageOwner struct{ err error }

func (owner failingStageOwner) AcceptanceRulePackageFor(
	context.Context,
	psdomain.SourceIdentity,
) (pcdomain.CommercialVersion, bool, error) {
	return pcdomain.CommercialVersion{}, false, owner.err
}

func (owner failingStageOwner) AuthorizationRuleFor(
	context.Context,
	psdomain.SourceIdentity,
) (pcdomain.CommercialVersion, bool, error) {
	return pcdomain.CommercialVersion{}, false, owner.err
}

func amendmentQuery(
	t *testing.T,
	group string,
	stage psdomain.AmendmentStage,
	intent psdomain.AmendmentIntent,
) psports.SourceDataAmendmentQuery {
	t.Helper()
	scope, err := psdomain.NewShipmentScopedSourceData(
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		value(t, psdomain.NewSourceDataGroupReference, group),
	)
	if err != nil {
		t.Fatalf("资料范围：%v", err)
	}
	return psports.SourceDataAmendmentQuery{
		Identity: stageIdentity(t),
		Scope:    scope,
		Intent:   intent,
		Reason:   value(t, psdomain.NewAmendmentReasonReference, "reason-1"),
		Stage:    stage,
	}
}

func allowanceCell(
	t *testing.T,
	group string,
	stage pcdomain.DeclaredAmendmentStage,
	intent pcdomain.DeclaredAmendmentIntent,
	allowance pcdomain.AmendmentAllowance,
) pcdomain.SourceDataAmendmentRule {
	t.Helper()
	return pcdomain.SourceDataAmendmentRule{
		DataGroup: value(t, pcdomain.NewSourceDataGroupReference, group),
		Stage:     stage,
		Intent:    intent,
		Allowance: allowance,
	}
}

func allowanceContent(
	t *testing.T,
	owner pcdomain.CommercialVersion,
	closed bool,
	rules ...pcdomain.SourceDataAmendmentRule,
) pcdomain.SourceDataAmendmentAllowanceContent {
	t.Helper()
	content, err := pcdomain.NewSourceDataAmendmentAllowanceContent(owner, closed, rules)
	if err != nil {
		t.Fatalf("资料修订允许声明：%v", err)
	}
	return content
}

func newAllowanceAdapter(
	t *testing.T,
	owners adapter.AdoptedStageOwner,
	view *allowanceViewDouble,
) *adapter.DeclaredSourceDataAmendmentAllowance {
	t.Helper()
	subject, err := adapter.NewDeclaredSourceDataAmendmentAllowance(owners, view)
	if err != nil {
		t.Fatalf("构造适配器：%v", err)
	}
	return subject
}

// mirroredStages / mirroredIntents 是两侧封闭集的逐格配对表。表本身就是断言：哪一格该对哪一格由
// ADR-0120 Decision 五说（PC 镜像 PS 的原词，一格不拆不加）。
var mirroredStages = []struct {
	ps psdomain.AmendmentStage
	pc pcdomain.DeclaredAmendmentStage
}{
	{psdomain.StageAcceptedNotYetReceived, pcdomain.DeclaredAcceptedNotYetReceived},
	{psdomain.StageReceivedOrMeasured, pcdomain.DeclaredReceivedOrMeasured},
	{psdomain.StageLabelledOrBagged, pcdomain.DeclaredLabelledOrBagged},
	{psdomain.StageCustomsDataFormingNotSubmitted, pcdomain.DeclaredCustomsDataFormingNotSubmitted},
	{psdomain.StageCustomsSubmitted, pcdomain.DeclaredCustomsSubmitted},
	{psdomain.StageCaseClosedOrServiceCompleted, pcdomain.DeclaredCaseClosedOrServiceCompleted},
}

var mirroredIntents = []struct {
	ps psdomain.AmendmentIntent
	pc pcdomain.DeclaredAmendmentIntent
}{
	{psdomain.SupplementIntent, pcdomain.DeclaredSupplementIntent},
	{psdomain.CorrectionIntent, pcdomain.DeclaredCorrectionIntent},
	{psdomain.ExplicitClearIntent, pcdomain.DeclaredExplicitClearIntent},
}

// Covers: ADR-0120 Decision 五——阶段六格与意图三格的词，单一权威在 parcel-shipment，party-commercial
// 只镜像不另定；两侧领域包互不 import，相等只能在本包（两侧都在手上）用测试钉住。两件一起钉：(a) 每一对的
// String() 逐字相等，且两侧在最后一格之后都没有第七格 / 第四格；(b) 适配器把 PS 那一格译到的正是 PC 的
// 这一格——以「只登这一格 ALLOWED」的声明对全部 6×3 个查询作答，恰好那一个答允许、其余答未声明（未封闭
// 缺格）。只对字不对译，两侧词齐了而 switch 串了一行照样绿；只对译不对字，词漂移了而两侧同步漂照样绿。
func TestTheAdapterMirrorsParcelShipmentStageAndIntentWordsCellByCell(t *testing.T) {
	for _, pair := range mirroredStages {
		if pair.ps.String() == "" || pair.ps.String() != pair.pc.String() {
			t.Fatalf("阶段词两侧不等：PS %q / PC %q", pair.ps, pair.pc)
		}
	}
	for _, pair := range mirroredIntents {
		if pair.ps.String() == "" || pair.ps.String() != pair.pc.String() {
			t.Fatalf("意图词两侧不等：PS %q / PC %q", pair.ps, pair.pc)
		}
	}
	if psdomain.AmendmentStage(len(mirroredStages)+1).String() != "" ||
		pcdomain.DeclaredAmendmentStage(len(mirroredStages)+1).String() != "" {
		t.Fatal("某一侧长出了第七格阶段——镜像纪律要求两侧同步加格，且本表要跟着补一行")
	}
	if psdomain.AmendmentIntent(len(mirroredIntents)+1).String() != "" ||
		pcdomain.DeclaredAmendmentIntent(len(mirroredIntents)+1).String() != "" {
		t.Fatal("某一侧长出了第四格意图——镜像纪律要求两侧同步加格，且本表要跟着补一行")
	}

	rules := stageRulePackage(t)
	const group = "group-consignee"
	for _, declared := range mirroredStages {
		for _, declaredIntent := range mirroredIntents {
			view := &allowanceViewDouble{
				content: allowanceContent(t, rules, false,
					allowanceCell(t, group, declared.pc, declaredIntent.pc, pcdomain.AmendmentAllowed)),
				found: true,
			}
			subject := newAllowanceAdapter(t, boundStageOwner{rules: rules}, view)
			for _, asked := range mirroredStages {
				for _, askedIntent := range mirroredIntents {
					allowance, err := subject.DeclareSourceDataAmendment(
						context.Background(), amendmentQuery(t, group, asked.ps, askedIntent.ps))
					if err != nil {
						t.Fatalf("声明 (%s, %s)，问 (%s, %s)：%v", declared.pc, declaredIntent.pc, asked.ps, askedIntent.ps, err)
					}
					want := psports.SourceDataAmendmentNotDeclared
					if asked.ps == declared.ps && askedIntent.ps == declaredIntent.ps {
						want = psports.SourceDataAmendmentAllowed
					}
					if allowance != want {
						t.Fatalf("声明只登 (%s, %s) 允许，问 (%s, %s) 答 %v, want %v——译到了别的格",
							declared.pc, declaredIntent.pc, asked.ps, askedIntent.ps, allowance, want)
					}
				}
			}
		}
	}
}

// Covers: 票 ps-port-remainder/02 裁决 (c) 与 ADR-0120 Decision 三 / 四——三值由 PC 算出、本适配器一对一译：
// 登了允许答允许、登了不允许答不允许；缺格在未封闭的声明上读「未声明」、在封闭的声明上读「不允许」——
// 「封闭意味着什么」是提供方的规则，这里不解释也不改写。
func TestTheAdapterTranslatesTheThreeValuesOneToOneAndLeavesTheClosedReadingToTheProvider(t *testing.T) {
	rules := stageRulePackage(t)
	const group = "group-consignee"
	stage := psdomain.StageAcceptedNotYetReceived

	open := &allowanceViewDouble{
		content: allowanceContent(t, rules, false,
			allowanceCell(t, group, pcdomain.DeclaredAcceptedNotYetReceived, pcdomain.DeclaredSupplementIntent, pcdomain.AmendmentAllowed),
			allowanceCell(t, group, pcdomain.DeclaredAcceptedNotYetReceived, pcdomain.DeclaredCorrectionIntent, pcdomain.AmendmentDisallowed),
		),
		found: true,
	}
	subject := newAllowanceAdapter(t, boundStageOwner{rules: rules}, open)
	for _, test := range []struct {
		name   string
		intent psdomain.AmendmentIntent
		want   psports.SourceDataAmendmentAllowance
	}{
		{"登了允许", psdomain.SupplementIntent, psports.SourceDataAmendmentAllowed},
		{"登了不允许", psdomain.CorrectionIntent, psports.SourceDataAmendmentDisallowed},
		{"缺格且未封闭", psdomain.ExplicitClearIntent, psports.SourceDataAmendmentNotDeclared},
	} {
		allowance, err := subject.DeclareSourceDataAmendment(context.Background(), amendmentQuery(t, group, stage, test.intent))
		if err != nil {
			t.Fatalf("%s：%v", test.name, err)
		}
		if allowance != test.want {
			t.Fatalf("%s：allowance = %v, want %v", test.name, allowance, test.want)
		}
	}

	closed := &allowanceViewDouble{
		content: allowanceContent(t, rules, true,
			allowanceCell(t, group, pcdomain.DeclaredAcceptedNotYetReceived, pcdomain.DeclaredSupplementIntent, pcdomain.AmendmentAllowed)),
		found: true,
	}
	subject = newAllowanceAdapter(t, boundStageOwner{rules: rules}, closed)
	allowance, err := subject.DeclareSourceDataAmendment(context.Background(), amendmentQuery(t, group, stage, psdomain.ExplicitClearIntent))
	if err != nil {
		t.Fatalf("封闭声明缺格：%v", err)
	}
	if allowance != psports.SourceDataAmendmentDisallowed {
		t.Fatalf("封闭声明缺格答 %v, want DISALLOWED——登记方显式说了「其余都不许」", allowance)
	}
	allowance, err = subject.DeclareSourceDataAmendment(context.Background(), amendmentQuery(t, "group-shipper", stage, psdomain.SupplementIntent))
	if err != nil {
		t.Fatalf("封闭声明别的资料组：%v", err)
	}
	if allowance != psports.SourceDataAmendmentDisallowed {
		t.Fatalf("封闭声明下别的资料组答 %v, want DISALLOWED——封闭对三维缺格一视同仁", allowance)
	}
}

// Covers: 两处 found=false 都是「还没人说这处资料能不能改」——采用版本尚未固定（闭包不在）时不去读声明、
// 答未声明；闭包在而声明无父行时同样答未声明。两格都不是错误：恢复动作在提供方，不是重试。
func TestTheAdapterAnswersNotDeclaredWhenNobodyHasSaidAnything(t *testing.T) {
	rules := stageRulePackage(t)
	query := amendmentQuery(t, "group-consignee", psdomain.StageReceivedOrMeasured, psdomain.SupplementIntent)

	view := &allowanceViewDouble{found: true, content: allowanceContent(t, rules, true)}
	owner := &absentStageOwner{}
	subject := newAllowanceAdapter(t, owner, view)
	allowance, err := subject.DeclareSourceDataAmendment(context.Background(), query)
	if err != nil {
		t.Fatalf("采用版本未固定：%v", err)
	}
	if allowance != psports.SourceDataAmendmentNotDeclared {
		t.Fatalf("采用版本未固定答 %v, want NOT_DECLARED", allowance)
	}
	if owner.calls != 1 || view.calls != 0 {
		t.Fatalf("owner 问了 %d 次、声明读了 %d 次——没有采用版本就没有可读的声明，读了就是拿别的版本顶", owner.calls, view.calls)
	}

	view = &allowanceViewDouble{found: false}
	subject = newAllowanceAdapter(t, boundStageOwner{rules: rules}, view)
	allowance, err = subject.DeclareSourceDataAmendment(context.Background(), query)
	if err != nil {
		t.Fatalf("声明未登记：%v", err)
	}
	if allowance != psports.SourceDataAmendmentNotDeclared {
		t.Fatalf("声明未登记答 %v, want NOT_DECLARED", allowance)
	}
	if view.calls != 1 {
		t.Fatalf("声明读了 %d 次, want 1", view.calls)
	}
}

// Covers: 声明按（租户 + 采用的规则包版本）读——两个键都来自查询身份与回指结果，不来自任何默认。
func TestTheAdapterReadsTheDeclarationOfTheAdoptedRulePackageForTheQueryingTenant(t *testing.T) {
	rules := stageRulePackage(t)
	view := &allowanceViewDouble{found: false}
	subject := newAllowanceAdapter(t, boundStageOwner{rules: rules}, view)
	query := amendmentQuery(t, "group-consignee", psdomain.StageCustomsSubmitted, psdomain.CorrectionIntent)
	if _, err := subject.DeclareSourceDataAmendment(context.Background(), query); err != nil {
		t.Fatalf("declare：%v", err)
	}
	if view.tenant.String() != query.Identity.TenantID().String() {
		t.Fatalf("读声明用的租户 = %q, want %q", view.tenant, query.Identity.TenantID())
	}
	if view.version.ObjectID().String() != rules.ObjectID().String() ||
		view.version.Version().String() != rules.Version().String() {
		t.Fatalf("读声明用的版本 = %s/%s, want 回指到的规则包 %s/%s",
			view.version.ObjectID(), view.version.Version(), rules.ObjectID(), rules.Version())
	}
}

// Covers: 端口头注「实现方收到零值 Stage 应当拒答而不是当最早阶段查」；意图零值同理。两格都在读任何东西
// 之前上抛——不回指、不读声明，也不答任何一格。
func TestTheAdapterRefusesAnUndeterminedStageOrAnUndeclaredIntentBeforeReadingAnything(t *testing.T) {
	rules := stageRulePackage(t)
	for _, test := range []struct {
		name  string
		query psports.SourceDataAmendmentQuery
	}{
		{"判不出阶段", amendmentQuery(t, "group-consignee", psdomain.AmendmentStageUndetermined, psdomain.SupplementIntent)},
		{"集外阶段", amendmentQuery(t, "group-consignee", psdomain.AmendmentStage(99), psdomain.SupplementIntent)},
		{"未声明意图", amendmentQuery(t, "group-consignee", psdomain.StageAcceptedNotYetReceived, psdomain.AmendmentIntentInvalid)},
	} {
		view := &allowanceViewDouble{found: true, content: allowanceContent(t, rules, true)}
		owner := &absentStageOwner{}
		subject := newAllowanceAdapter(t, owner, view)
		_, err := subject.DeclareSourceDataAmendment(context.Background(), test.query)
		if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
			t.Fatalf("%s：err = %v, want ErrUntranslatableAnswer", test.name, err)
		}
		if owner.calls != 0 || view.calls != 0 {
			t.Fatalf("%s：译不过去却回指了 %d 次、读了 %d 次", test.name, owner.calls, view.calls)
		}
	}
}

// Covers: 读不回与「没登记」分开——回指失败与声明读口失败原样上抛（编排落 SourceDataRuleUnavailable，
// 等依赖恢复），不折成未声明（那会让续办方去催登记，而其实该重试）。
func TestTheAdapterSurfacesProviderFailuresInsteadOfFoldingThemIntoNotDeclared(t *testing.T) {
	rules := stageRulePackage(t)
	query := amendmentQuery(t, "group-consignee", psdomain.StageLabelledOrBagged, psdomain.SupplementIntent)

	ownerFailure := errors.New("closure store down")
	subject := newAllowanceAdapter(t, failingStageOwner{err: ownerFailure}, &allowanceViewDouble{})
	if _, err := subject.DeclareSourceDataAmendment(context.Background(), query); !errors.Is(err, ownerFailure) {
		t.Fatalf("回指失败：err = %v, want 包住 %v", err, ownerFailure)
	}

	viewFailure := errors.New("declaration store down")
	subject = newAllowanceAdapter(t, boundStageOwner{rules: rules}, &allowanceViewDouble{err: viewFailure})
	if _, err := subject.DeclareSourceDataAmendment(context.Background(), query); !errors.Is(err, viewFailure) {
		t.Fatalf("声明读口失败：err = %v, want 包住 %v", err, viewFailure)
	}
}

// Covers: 构造门——两个协作者都不得为 nil。nil 在装配点上是接线漏了一半，不是「未配置」那一格
// （未配置有自己的诚实答复：闭包不在或无父行）。
func TestTheAdapterRefusesNilCollaborators(t *testing.T) {
	if _, err := adapter.NewDeclaredSourceDataAmendmentAllowance(nil, &allowanceViewDouble{}); err == nil {
		t.Fatal("nil owner 该被拒")
	}
	if _, err := adapter.NewDeclaredSourceDataAmendmentAllowance(boundStageOwner{}, nil); err == nil {
		t.Fatal("nil 声明读口该被拒")
	}
}
