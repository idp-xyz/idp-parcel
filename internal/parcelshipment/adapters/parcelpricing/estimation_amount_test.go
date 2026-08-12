package parcelpricing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/parcelpricing"
	saadapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 装配器必须结构性满足 SA 适配器的金额缝接口——装配处直接接上，不再垫翻译层。
var _ saadapter.ControlAmountSource = (*adapter.EstimationAmountSource)(nil)

var estimationAsOfAt = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

func value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

// syntheticPlan 用导出构造器复刻提供方测试的最小价卡：Z1 区 0–10kg 一条基础费率，
// ceiling 0.5kg 圆整。真实价卡属实例半边，这份合成卡只钉装配规则（S 级纪律）。
func syntheticPlan(t *testing.T, baseAmount string) ppdomain.PricingPlanVersion {
	t.Helper()
	currency := value(t, ppdomain.NewCurrency, "USD")
	period, err := ppdomain.NewEffectivePeriod(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("effective period: %v", err)
	}
	entryAmount, err := ppdomain.NewMoneyFromString(baseAmount, currency)
	if err != nil {
		t.Fatalf("entry amount: %v", err)
	}
	entry, err := ppdomain.NewRateEntry(
		value(t, ppdomain.NewRateEntryID, "entry-1"),
		"Z1",
		mustWeight(t, "0"),
		mustWeight(t, "10"),
		entryAmount,
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	tableRef, err := ppdomain.NewVersionReference(ppdomain.ArtifactRateTable, "table-1", "v1", "sha256:syn-table-1")
	if err != nil {
		t.Fatalf("table reference: %v", err)
	}
	table, err := ppdomain.NewRateTableVersion(
		tableRef, ppdomain.RateTableFamilyWeightZone, currency,
		ppdomain.WeightUnitKilogram, period, []ppdomain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingCeiling, mustWeight(t, "0.5"))
	if err != nil {
		t.Fatalf("rounding: %v", err)
	}
	weightRef, err := ppdomain.NewVersionReference(ppdomain.ArtifactWeightPolicy, "weight-1", "v1", "sha256:syn-weight-1")
	if err != nil {
		t.Fatalf("weight reference: %v", err)
	}
	weightPolicy, err := ppdomain.NewPricingWeightPolicy(weightRef, ppdomain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	planRef, err := ppdomain.NewVersionReference(ppdomain.ArtifactPricingPlan, "plan-1", "v1", "sha256:syn-plan-1")
	if err != nil {
		t.Fatalf("plan reference: %v", err)
	}
	plan, err := ppdomain.NewPricingPlanVersion(
		planRef,
		value(t, ppdomain.NewPricingScopeID, "scope-1"),
		ppdomain.PricingDirectionSell,
		ppdomain.PricingPurposeCustomerCharge,
		value(t, ppdomain.NewChargeCode, "BASE_FREIGHT"),
		period,
		table,
		weightPolicy,
		nil,
		ppdomain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

func mustWeight(t *testing.T, raw string) ppdomain.Weight {
	t.Helper()
	weight, err := ppdomain.NewWeightFromString(raw, ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("weight %q: %v", raw, err)
	}
	return weight
}

type basisDouble struct {
	basis ppdomain.PricingPlanVersion
	found bool
	err   error
	asked int
}

func (double *basisDouble) FindEstimationBasis(
	_ context.Context,
	_ psports.FinancialControlRequest,
) (adapter.EstimationBasis, bool, error) {
	double.asked++
	if double.err != nil {
		return adapter.EstimationBasis{}, false, double.err
	}
	if !double.found {
		return adapter.EstimationBasis{}, false, nil
	}
	return adapter.EstimationBasis{
		Plan:        double.basis,
		Scope:       ppdomain.PricingScopeID{},
		Zone:        "Z1",
		Currency:    ppdomain.Currency{},
		MinorDigits: 2,
		Evidence:    ppdomain.EvidenceSynthetic,
	}, true, nil
}

type requestStoreDouble struct {
	request psdomain.ShipmentRequest
	found   bool
	err     error
	asked   int
}

func (double *requestStoreDouble) FindBySourceIdentity(
	_ context.Context,
	_ psdomain.SourceIdentity,
) (psdomain.ShipmentRequest, bool, error) {
	double.asked++
	if double.err != nil {
		return psdomain.ShipmentRequest{}, false, double.err
	}
	return double.request, double.found, nil
}

func (double *requestStoreDouble) Insert(
	_ context.Context, _ psdomain.SourceIdentity, _ psdomain.ShipmentRequest,
) (psports.ShipmentRequestInsertOutcome, error) {
	return 0, errors.New("not part of this seam")
}

func (double *requestStoreDouble) Save(
	_ context.Context, _ psdomain.SourceIdentity, _ psdomain.ShipmentRequest,
) (psports.ShipmentRequestSaveOutcome, error) {
	return 0, errors.New("not part of this seam")
}

func identity(t *testing.T) psdomain.SourceIdentity {
	t.Helper()
	built, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return built
}

func profileFor(t *testing.T, parcel, weightRaw, unit string) psdomain.DeclaredParcelProfile {
	t.Helper()
	weight, err := psdomain.NewDeclaredWeight(
		value(t, psdomain.NewMeasurementValue, weightRaw),
		value(t, psdomain.NewMeasurementUnitReference, unit),
	)
	if err != nil {
		t.Fatalf("declared weight: %v", err)
	}
	measurement, err := psdomain.NewDeclaredMeasurement(weight, psdomain.DeclaredDimensions{})
	if err != nil {
		t.Fatalf("declared measurement: %v", err)
	}
	profile, err := psdomain.NewDeclaredParcelProfile(value(t, psdomain.NewDeclaredParcelID, parcel), measurement)
	if err != nil {
		t.Fatalf("declared profile: %v", err)
	}
	return profile
}

// submittedRequest 造一份带画像的已提交委托（真经领域建单，假状态钉不住读取路径）。
// 成员名单与画像分开给：缺画像的成员正是要演练的那一格。
func submittedRequest(t *testing.T, parcelIDs []string, profiles ...psdomain.DeclaredParcelProfile) psdomain.ShipmentRequest {
	t.Helper()
	parcels := make([]psdomain.DeclaredParcelID, 0, len(parcelIDs))
	for _, raw := range parcelIDs {
		parcels = append(parcels, value(t, psdomain.NewDeclaredParcelID, raw))
	}
	fingerprint, err := psdomain.NewSourceSubmissionFingerprint(
		identity(t),
		value(t, psdomain.NewPayloadDigest, "digest-1"),
		estimationAsOfAt.Add(-time.Hour),
		estimationAsOfAt.Add(-time.Hour+time.Second),
	)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	candidate, err := psdomain.NewSubmissionCandidate(
		fingerprint,
		value(t, psdomain.NewSubmissionBatchID, "batch-1"),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		parcels,
	)
	if err != nil {
		t.Fatalf("candidate: %v", err)
	}
	decision := allowedOwnership(t)
	gate, err := psdomain.EvaluateFutureSubmissionGate(
		decision, value(t, psdomain.NewAdmissionScopeDigest, "scope-1"),
		value(t, psdomain.NewProductionOwnershipRevision, "rev-1"), estimationAsOfAt.Add(-time.Minute))
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	request, err := psdomain.SubmitShipmentRequest(psdomain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   value(t, psdomain.NewSubmissionVersionID, "version-1"),
		TaskID:      value(t, psdomain.NewAcceptanceDecisionTaskID, "task-1"),
		SubmittedAt: estimationAsOfAt.Add(-time.Minute),
		Profiles:    profiles,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return request
}

func allowedOwnership(t *testing.T) psdomain.ProductionOwnershipDecision {
	t.Helper()
	scope, err := psdomain.NewAdmissionScope(
		value(t, psdomain.NewAdmissionScopeReference, "scope-ref-1"),
		value(t, psdomain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("admission scope: %v", err)
	}
	validity, err := psdomain.NewOwnershipValidityInterval(
		estimationAsOfAt.Add(-24*time.Hour), estimationAsOfAt.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("validity: %v", err)
	}
	decision, err := psdomain.NewProductionOwnershipDecision(psdomain.ProductionOwnershipDecisionSpec{
		DecisionID:       value(t, psdomain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        psdomain.ProductionAuthorityIDPParcel,
		AdmissionControl: psdomain.AdmissionControlOpen,
		RuleVersion:      value(t, psdomain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             estimationAsOfAt.Add(-time.Hour),
		Validity:         validity,
		Revision:         value(t, psdomain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       estimationAsOfAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("ownership decision: %v", err)
	}
	return decision
}

func controlRequest(t *testing.T) psports.FinancialControlRequest {
	t.Helper()
	echoed, err := psdomain.NewEchoedAsOfPolicy(
		psdomain.FinancialControlJudgmentKind,
		value(t, psdomain.NewAsOfSemanticsReference, "CONTROL_EVALUATION_AT"),
		value(t, psdomain.NewAsOfPolicyVersion, "asof-policy-v1"),
	)
	if err != nil {
		t.Fatalf("echoed policy: %v", err)
	}
	asOf, err := psdomain.NewJudgmentAsOf(estimationAsOfAt, echoed)
	if err != nil {
		t.Fatalf("judgment as-of: %v", err)
	}
	return psports.FinancialControlRequest{
		Identity:          identity(t),
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
		AsOf:              asOf,
	}
}

// Covers: ADR-0048 第三增量——控制金额 = 当前提交版本逐成员纯评价合计：画像保真进
// 计价输入，合计按结算币种最小单位交回。合成价卡只钉装配规则（S 级纪律），不冒充
// 真实价卡。
func TestAControlAmountIsTheSumOfPerMemberEstimates(t *testing.T) {
	scope := value(t, ppdomain.NewPricingScopeID, "scope-1")
	currency := value(t, ppdomain.NewCurrency, "USD")
	store := &requestStoreDouble{
		request: submittedRequest(t, []string{"parcel-1", "parcel-2"},
			profileFor(t, "parcel-1", "1.2", "kg"),
			profileFor(t, "parcel-2", "3", "kg"),
		),
		found: true,
	}
	source := adapter.NewEstimationAmountSource(store, &scopedBasis{plan: syntheticPlan(t, "7.25"), scope: scope, currency: currency})

	amount, formed, err := source.FormControlAmount(context.Background(), controlRequest(t))
	if err != nil {
		t.Fatalf("form control amount: %v", err)
	}
	if !formed {
		t.Fatal("画像与价卡齐备，金额却没形成")
	}
	if amount != 1450 {
		t.Fatalf("amount = %d, want 1450（两件各 7.25 USD 合计的最小单位）", amount)
	}
}

// scopedBasis 是带齐范围与币种的估价基替身。
type scopedBasis struct {
	plan     ppdomain.PricingPlanVersion
	scope    ppdomain.PricingScopeID
	currency ppdomain.Currency
	found    bool
	err      error
	asked    int
	missing  bool
}

func (double *scopedBasis) FindEstimationBasis(
	_ context.Context,
	_ psports.FinancialControlRequest,
) (adapter.EstimationBasis, bool, error) {
	double.asked++
	if double.err != nil {
		return adapter.EstimationBasis{}, false, double.err
	}
	if double.missing {
		return adapter.EstimationBasis{}, false, nil
	}
	return adapter.EstimationBasis{
		Plan:        double.plan,
		Scope:       double.scope,
		Zone:        "Z1",
		Currency:    double.currency,
		MinorDigits: 2,
		Evidence:    ppdomain.EvidenceSynthetic,
	}, true, nil
}

// Covers: 逐格停在未形成而不发明数字——估价基未配置、成员缺画像、单位读不出、评价给不出
// 合计（超出费率区间）。每一格都是 false 不是错误：一个在缺口上凑出来的金额占的是客户
// 的真金白银。
func TestAMissingInputStopsTheAmountWithoutInventingOne(t *testing.T) {
	scope := value(t, ppdomain.NewPricingScopeID, "scope-1")
	currency := value(t, ppdomain.NewCurrency, "USD")
	plan := syntheticPlan(t, "7.25")

	t.Run("basis not configured", func(t *testing.T) {
		store := &requestStoreDouble{request: submittedRequest(t, []string{"parcel-1"}, profileFor(t, "parcel-1", "1", "kg")), found: true}
		source := adapter.NewEstimationAmountSource(store, &scopedBasis{missing: true})
		_, formed, err := source.FormControlAmount(context.Background(), controlRequest(t))
		if err != nil || formed {
			t.Fatalf("formed = %v err = %v, want unformed without error", formed, err)
		}
		if store.asked != 0 {
			t.Fatal("估价基未配置还去读了委托")
		}
	})

	t.Run("a member without a profile", func(t *testing.T) {
		// 两个成员只有一张画像：缺的那个成员挡下整份金额。
		store := &requestStoreDouble{
			request: submittedRequest(t, []string{"parcel-1", "parcel-2"}, profileFor(t, "parcel-1", "1", "kg")),
			found:   true,
		}
		source := adapter.NewEstimationAmountSource(store, &scopedBasis{plan: plan, scope: scope, currency: currency})
		_, formed, err := source.FormControlAmount(context.Background(), controlRequest(t))
		if err != nil || formed {
			t.Fatalf("formed = %v err = %v; 客户没报测量不该有人替他估", formed, err)
		}
	})

	t.Run("an unreadable unit", func(t *testing.T) {
		store := &requestStoreDouble{request: submittedRequest(t, []string{"parcel-1"}, profileFor(t, "parcel-1", "1", "JIN")), found: true}
		source := adapter.NewEstimationAmountSource(store, &scopedBasis{plan: plan, scope: scope, currency: currency})
		_, formed, err := source.FormControlAmount(context.Background(), controlRequest(t))
		if err != nil || formed {
			t.Fatalf("formed = %v err = %v; 读不出的单位不能猜", formed, err)
		}
	})

	t.Run("no rate for the declared weight", func(t *testing.T) {
		store := &requestStoreDouble{request: submittedRequest(t, []string{"parcel-1"}, profileFor(t, "parcel-1", "15", "kg")), found: true}
		source := adapter.NewEstimationAmountSource(store, &scopedBasis{plan: plan, scope: scope, currency: currency})
		_, formed, err := source.FormControlAmount(context.Background(), controlRequest(t))
		if err != nil || formed {
			t.Fatalf("formed = %v err = %v; 评价没有合计就没有金额", formed, err)
		}
	})

	t.Run("a superseded version", func(t *testing.T) {
		store := &requestStoreDouble{request: submittedRequest(t, []string{"parcel-1"}, profileFor(t, "parcel-1", "1", "kg")), found: true}
		source := adapter.NewEstimationAmountSource(store, &scopedBasis{plan: plan, scope: scope, currency: currency})
		request := controlRequest(t)
		request.SubmissionVersion = value(t, psdomain.NewSubmissionVersionID, "version-9")
		_, formed, err := source.FormControlAmount(context.Background(), request)
		if err != nil || formed {
			t.Fatalf("formed = %v err = %v; 为不再待判断的版本占资金无从释放", formed, err)
		}
	})
}

// Covers: 单位符号折大写进提供方封闭词表是翻译不是判断——kg 与 KG 是同一个单位的两种
// 写法，都译得动；词表外的单位（JIN）在上一个测试里被拒。
func TestUnitTranslationFoldsCaseIntoTheProvidersVocabulary(t *testing.T) {
	scope := value(t, ppdomain.NewPricingScopeID, "scope-1")
	currency := value(t, ppdomain.NewCurrency, "USD")
	store := &requestStoreDouble{request: submittedRequest(t, []string{"parcel-1"}, profileFor(t, "parcel-1", "1", "kg")), found: true}
	source := adapter.NewEstimationAmountSource(store, &scopedBasis{plan: syntheticPlan(t, "7.25"), scope: scope, currency: currency})

	amount, formed, err := source.FormControlAmount(context.Background(), controlRequest(t))
	if err != nil || !formed {
		t.Fatalf("formed = %v err = %v", formed, err)
	}
	if amount != 725 {
		t.Fatalf("amount = %d, want 725", amount)
	}
}
