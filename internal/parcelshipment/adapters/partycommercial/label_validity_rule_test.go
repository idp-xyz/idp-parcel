package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 用例时间全部落在过去：渠道结果 2026-08-20 观察到、72 小时后到点。若适配器偷拿墙钟而不是用例给的 asOf，
// 「未过期」那几例会在任何一台今天以后的机器上答 lapsed=true——它们绿着就是「不拿墙钟」的证据。
var (
	labelValidityEstablishedAt = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	labelValidityObservedAt    = labelValidityEstablishedAt.Add(2 * time.Hour)
	labelValidityDuration      = 72 * time.Hour
)

type labelValidityTargetsDouble struct {
	target psdomain.CurrentAcceptedParcelTarget
	found  bool
	err    error
	tenant string
	parcel string
}

func (double *labelValidityTargetsDouble) FindCurrentAcceptedByParcel(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	double.tenant = tenant.String()
	double.parcel = parcel.String()
	if double.err != nil {
		return psdomain.CurrentAcceptedParcelTarget{}, false, double.err
	}
	return double.target, double.found, nil
}

type labelValidityFinalViewDouble struct {
	content pcdomain.FinalRuleContent
	found   bool
	err     error
	calls   int
	tenant  string
	version pcdomain.CommercialVersion
}

func (double *labelValidityFinalViewDouble) LoadFinalRule(
	_ context.Context, tenant pcdomain.TenantID, rulePackage pcdomain.CommercialVersion,
) (pcdomain.FinalRuleContent, bool, error) {
	double.calls++
	double.tenant = tenant.String()
	double.version = rulePackage
	if double.err != nil {
		return pcdomain.FinalRuleContent{}, false, double.err
	}
	return double.content, double.found, nil
}

type labelValidityFixture struct {
	targets *labelValidityTargetsDouble
	final   *labelValidityFinalViewDouble
	subject *adapter.DeclaredLabelValidityRule
}

// newLabelValidityFixture 把包裹反查、接受时固定的规则包回指与终局规则读口装成一只适配器。回指走包内既有的
// DeclaredStageContent（ADR-0062 的消费侧落点），不另造一条「接受时固定」。
func newLabelValidityFixture(
	t *testing.T,
	targets *labelValidityTargetsDouble,
	owner adapter.AdoptedStageOwner,
	final *labelValidityFinalViewDouble,
) *labelValidityFixture {
	t.Helper()
	subject, err := adapter.NewDeclaredLabelValidityRule(
		targets, adapter.NewDeclaredStageContent(nil, final, nil, owner))
	if err != nil {
		t.Fatalf("new declared label validity rule: %v", err)
	}
	return &labelValidityFixture{targets: targets, final: final, subject: subject}
}

func (fixture *labelValidityFixture) judge(
	t *testing.T, transaction psdomain.LabelTransaction, asOf time.Time,
) (bool, bool, error) {
	t.Helper()
	return fixture.subject.JudgeLabelLapsed(
		context.Background(), value(t, psdomain.NewTenantID, "tenant-1"), transaction,
		value(t, psdomain.NewDeclaredParcelID, "parcel-1"), asOf)
}

func acceptedParcelTarget(t *testing.T) *labelValidityTargetsDouble {
	t.Helper()
	target, err := psdomain.NewCurrentAcceptedParcelTarget(
		stageIdentity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		value(t, psdomain.NewSubmissionVersionID, "request-1/v1"),
	)
	if err != nil {
		t.Fatalf("new current accepted parcel target: %v", err)
	}
	return &labelValidityTargetsDouble{target: target, found: true}
}

// submittedLabelTransaction 造一笔覆盖 parcel-1、已提交渠道但结果未回的交易。
func submittedLabelTransaction(t *testing.T) psdomain.LabelTransaction {
	t.Helper()
	parcel := value(t, psdomain.NewDeclaredParcelID, "parcel-1")
	transaction, err := psdomain.EstablishLabelTransaction(psdomain.EstablishLabelTransactionSpec{
		Tenant:                 value(t, psdomain.NewTenantID, "tenant-1"),
		ID:                     value(t, psdomain.NewLabelTransactionID, "label-tx-1"),
		CoveredParcels:         []psdomain.DeclaredParcelID{parcel},
		ChannelAccount:         value(t, psdomain.NewChannelAccountReference, "channel-account-1"),
		AccountHolder:          value(t, psdomain.NewChannelAccountHolderReference, "party-holder-1"),
		ServiceProvider:        value(t, psdomain.NewChannelServiceProviderReference, "party-channel-1"),
		SettlementCounterparty: value(t, psdomain.NewSettlementCounterpartyReference, "party-settlement-1"),
		Contract:               value(t, psdomain.NewChannelContractReference, "contract-1"),
		Rate:                   value(t, psdomain.NewChannelRateReference, "rate-1"),
		ResponsibilityBasis:    value(t, psdomain.NewResponsibilityBasisSnapshotReference, "basis-snapshot-1"),
		EstablishedAt:          labelValidityEstablishedAt,
	})
	if err != nil {
		t.Fatalf("establish label transaction: %v", err)
	}
	transaction, err = transaction.SubmitToChannel(labelValidityEstablishedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("submit label transaction: %v", err)
	}
	return transaction
}

// acceptedLabelTransaction 在提交的基础上记下渠道结果：parcel-1 被受理，结果业务时间为 observedAt。
func acceptedLabelTransaction(t *testing.T, observedAt time.Time) psdomain.LabelTransaction {
	t.Helper()
	transaction, err := submittedLabelTransaction(t).RecordChannelResult(psdomain.RecordChannelResultSpec{
		Outcome: psdomain.LabelTransactionSucceeded,
		ParcelResults: []psdomain.LabelTransactionParcelResultSpec{{
			Parcel:     value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
			Accepted:   true,
			Identifier: value(t, psdomain.NewChannelParcelIdentifier, "CHN-label-tx-1"),
		}},
		ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatalf("record channel result: %v", err)
	}
	return transaction
}

func finalRuleRows(t *testing.T) []pcdomain.FinalizationDeclaration {
	t.Helper()
	return []pcdomain.FinalizationDeclaration{{
		Outcome:   pcdomain.DeclaredEffectiveDelivery,
		FinalKind: commercialRule(t, "NETWORK_SERVICE_DELIVERED"),
	}}
}

// finalRuleWithValidity 造一版带面单有效期声明的终局规则：锚种类「渠道结果业务时间」，时长由用例给——
// 取值只是夹具锚点，不是任何租户的声明（PAR-COM-17 待提供）。
func finalRuleWithValidity(t *testing.T, duration time.Duration) pcdomain.FinalRuleContent {
	t.Helper()
	validity, err := pcdomain.NewLabelValidityDeclaration(pcdomain.ChannelResultObservedAnchor, duration)
	if err != nil {
		t.Fatalf("new label validity declaration: %v", err)
	}
	content, err := pcdomain.NewFinalRuleContentWithValidity(stageRulePackage(t), finalRuleRows(t), validity)
	if err != nil {
		t.Fatalf("new final rule content with validity: %v", err)
	}
	return content
}

// finalRuleWithoutValidity 造一版终局规则行在场、有效期这一格缺席的声明。
func finalRuleWithoutValidity(t *testing.T) pcdomain.FinalRuleContent {
	t.Helper()
	content, err := pcdomain.NewFinalRuleContent(stageRulePackage(t), finalRuleRows(t))
	if err != nil {
		t.Fatalf("new final rule content: %v", err)
	}
	return content
}

var _ psports.LabelValidityRuleView = (*adapter.DeclaredLabelValidityRule)(nil)

// Covers: 票面裁决①「asOf ≥ 锚 + 时长即 lapsed=true」，锚 = ResultObservedAt（ADR-0119 Decision 二，Q1 唯一
// 一格）。边界按「≥」：恰好到点就失效。同时核三条线路：反查用的是调用方给的租户与包裹；终局规则按接受时
// 固定的那版规则包读（ADR-0062），不在 PS 另存；租户译到 PC 侧仍是同一个。
func TestDeclaredValidityLapsesOnceAsOfReachesAnchorPlusDuration(t *testing.T) {
	cases := []struct {
		name string
		asOf time.Time
	}{
		{name: "asOf 恰在锚 + 时长", asOf: labelValidityObservedAt.Add(labelValidityDuration)},
		{name: "asOf 晚于锚 + 时长", asOf: labelValidityObservedAt.Add(labelValidityDuration + 30*24*time.Hour)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := stageRulePackage(t)
			fixture := newLabelValidityFixture(t, acceptedParcelTarget(t),
				boundStageOwner{rules: rules, auth: stageAuthorizationRule(t)},
				&labelValidityFinalViewDouble{content: finalRuleWithValidity(t, labelValidityDuration), found: true})

			lapsed, configured, err := fixture.judge(t, acceptedLabelTransaction(t, labelValidityObservedAt), tc.asOf)
			if err != nil {
				t.Fatalf("judge: %v", err)
			}
			if !configured || !lapsed {
				t.Fatalf("lapsed = %v configured = %v, want true/true", lapsed, configured)
			}
			if fixture.targets.tenant != "tenant-1" || fixture.targets.parcel != "parcel-1" {
				t.Fatalf("parcel target looked up by tenant %q parcel %q", fixture.targets.tenant, fixture.targets.parcel)
			}
			if !fixture.final.version.SameVersionAs(rules) || fixture.final.tenant != "tenant-1" {
				t.Fatalf("final rule read for version %#v tenant %q, want the acceptance-fixed package",
					fixture.final.version, fixture.final.tenant)
			}
		})
	}
}

// Covers: 有声明但 asOf 尚未到锚 + 时长 → configured=true、lapsed=false。两例都落在墙钟已过点之后的过去：
// 差一纳秒那例是「≥」的另一侧，结果刚回那例是锚本身。
func TestDeclaredValidityDoesNotLapseBeforeAnchorPlusDuration(t *testing.T) {
	cases := []struct {
		name string
		asOf time.Time
	}{
		{name: "asOf 早于锚 + 时长一纳秒", asOf: labelValidityObservedAt.Add(labelValidityDuration - time.Nanosecond)},
		{name: "asOf 就是结果观察时刻", asOf: labelValidityObservedAt},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newLabelValidityFixture(t, acceptedParcelTarget(t),
				boundStageOwner{rules: stageRulePackage(t), auth: stageAuthorizationRule(t)},
				&labelValidityFinalViewDouble{content: finalRuleWithValidity(t, labelValidityDuration), found: true})

			lapsed, configured, err := fixture.judge(t, acceptedLabelTransaction(t, labelValidityObservedAt), tc.asOf)
			if err != nil {
				t.Fatalf("judge: %v", err)
			}
			if !configured || lapsed {
				t.Fatalf("lapsed = %v configured = %v, want false/true", lapsed, configured)
			}
		})
	}
}

// Covers: 票面红线「无声明恒不失效」与 ADR-0119 Decision 六——终局规则行在场而有效期这一格缺席，答
// configured=false，**不得因为其它行在场就把有效期当已配置**；asOf 远在结果之后也不推算。
func TestFinalRuleWithoutValidityDeclarationLeavesLapseUnconfigured(t *testing.T) {
	fixture := newLabelValidityFixture(t, acceptedParcelTarget(t),
		boundStageOwner{rules: stageRulePackage(t), auth: stageAuthorizationRule(t)},
		&labelValidityFinalViewDouble{content: finalRuleWithoutValidity(t), found: true})

	lapsed, configured, err := fixture.judge(t,
		acceptedLabelTransaction(t, labelValidityObservedAt), labelValidityObservedAt.Add(365*24*time.Hour))
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if configured || lapsed {
		t.Fatalf("lapsed = %v configured = %v, want false/false", lapsed, configured)
	}
	if fixture.final.calls != 1 {
		t.Fatalf("final rule read %d times, want exactly once", fixture.final.calls)
	}
}

// Covers: 终局规则无父行（LoadFinalRule found=false）→ configured=false。规则包固定了但租户还没登终局规则，
// 与「没这一格」同归实例半边未到。
func TestFinalRuleWithoutParentRowLeavesLapseUnconfigured(t *testing.T) {
	fixture := newLabelValidityFixture(t, acceptedParcelTarget(t),
		boundStageOwner{rules: stageRulePackage(t), auth: stageAuthorizationRule(t)},
		&labelValidityFinalViewDouble{found: false})

	lapsed, configured, err := fixture.judge(t,
		acceptedLabelTransaction(t, labelValidityObservedAt), labelValidityObservedAt.Add(365*24*time.Hour))
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if configured || lapsed {
		t.Fatalf("lapsed = %v configured = %v, want false/false", lapsed, configured)
	}
}

// Covers: 包裹不属任何当前已接受委托 → configured=false，且不再往下读规则包与终局规则——没有委托就没有
// 「接受时固定」可回指。
func TestParcelWithoutCurrentAcceptedRequestLeavesLapseUnconfigured(t *testing.T) {
	final := &labelValidityFinalViewDouble{content: finalRuleWithValidity(t, labelValidityDuration), found: true}
	fixture := newLabelValidityFixture(t, &labelValidityTargetsDouble{found: false},
		boundStageOwner{rules: stageRulePackage(t), auth: stageAuthorizationRule(t)}, final)

	lapsed, configured, err := fixture.judge(t,
		acceptedLabelTransaction(t, labelValidityObservedAt), labelValidityObservedAt.Add(365*24*time.Hour))
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if configured || lapsed {
		t.Fatalf("lapsed = %v configured = %v, want false/false", lapsed, configured)
	}
	if final.calls != 0 {
		t.Fatalf("final rule read %d times without an accepted request, want none", final.calls)
	}
}

// Covers: 委托在、但尚未固定采用哪版规则包（AdoptedStageOwner found=false，首发诚实未配置）→ configured=false，
// 终局规则读口不被点到。
func TestUnfixedRulePackageLeavesLapseUnconfigured(t *testing.T) {
	final := &labelValidityFinalViewDouble{content: finalRuleWithValidity(t, labelValidityDuration), found: true}
	fixture := newLabelValidityFixture(t, acceptedParcelTarget(t), adapter.UnconfiguredAdoptedStageOwner{}, final)

	lapsed, configured, err := fixture.judge(t,
		acceptedLabelTransaction(t, labelValidityObservedAt), labelValidityObservedAt.Add(365*24*time.Hour))
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if configured || lapsed {
		t.Fatalf("lapsed = %v configured = %v, want false/false", lapsed, configured)
	}
	if final.calls != 0 {
		t.Fatalf("final rule read %d times without a fixed rule package, want none", final.calls)
	}
}

// Covers: 读口出错 → error 原样上抛（消费编排据此停在 LABEL_VALIDITY_RULE_UNAVAILABLE），不折成未配置——折了
// 会让一次依赖故障与租户没登记长得一样。两口各一例。
func TestReadFailuresPropagateAsErrors(t *testing.T) {
	t.Run("包裹反查出错", func(t *testing.T) {
		boom := errors.New("parcel target view down")
		fixture := newLabelValidityFixture(t, &labelValidityTargetsDouble{err: boom},
			boundStageOwner{rules: stageRulePackage(t), auth: stageAuthorizationRule(t)},
			&labelValidityFinalViewDouble{content: finalRuleWithValidity(t, labelValidityDuration), found: true})

		lapsed, configured, err := fixture.judge(t,
			acceptedLabelTransaction(t, labelValidityObservedAt), labelValidityObservedAt.Add(labelValidityDuration))
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want the parcel target failure", err)
		}
		if lapsed || configured {
			t.Fatalf("lapsed = %v configured = %v on error, want false/false", lapsed, configured)
		}
	})
	t.Run("终局规则读口出错", func(t *testing.T) {
		boom := errors.New("final rule view down")
		fixture := newLabelValidityFixture(t, acceptedParcelTarget(t),
			boundStageOwner{rules: stageRulePackage(t), auth: stageAuthorizationRule(t)},
			&labelValidityFinalViewDouble{err: boom})

		lapsed, configured, err := fixture.judge(t,
			acceptedLabelTransaction(t, labelValidityObservedAt), labelValidityObservedAt.Add(labelValidityDuration))
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want the final rule failure", err)
		}
		if lapsed || configured {
			t.Fatalf("lapsed = %v configured = %v on error, want false/false", lapsed, configured)
		}
	})
}

// Covers: 锚种类「渠道结果业务时间」所指的那一刻在这笔交易上还不存在（结果未回，ResultObservedAt 为零值）
// → ErrUntranslatableAnswer。零值当锚会让任何 asOf 都算已过期；消费编排只对已受理的结果问本口，走到这里是
// 调用契约被打破，响亮报错不吸收。
func TestTransactionWithoutChannelResultIsUntranslatable(t *testing.T) {
	fixture := newLabelValidityFixture(t, acceptedParcelTarget(t),
		boundStageOwner{rules: stageRulePackage(t), auth: stageAuthorizationRule(t)},
		&labelValidityFinalViewDouble{content: finalRuleWithValidity(t, labelValidityDuration), found: true})

	_, configured, err := fixture.judge(t, submittedLabelTransaction(t), labelValidityObservedAt.Add(labelValidityDuration))
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("error = %v, want ErrUntranslatableAnswer", err)
	}
	if configured {
		t.Fatalf("configured = true on error")
	}
}

// Covers: 装配缺一半不许静默——本适配器一旦装上就是要真去问商业侧的，nil 依赖在构造期拒绝。
func TestDeclaredLabelValidityRuleRejectsNilDependencies(t *testing.T) {
	source := adapter.NewDeclaredStageContent(nil, &labelValidityFinalViewDouble{}, nil, adapter.UnconfiguredAdoptedStageOwner{})
	if _, err := adapter.NewDeclaredLabelValidityRule(nil, source); err == nil {
		t.Fatalf("nil parcel target view accepted")
	}
	if _, err := adapter.NewDeclaredLabelValidityRule(acceptedParcelTarget(t), nil); err == nil {
		t.Fatalf("nil final content source accepted")
	}
}
