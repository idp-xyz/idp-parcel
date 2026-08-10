package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: CONTEXT「特征」—「从计价输入快照派生、供判定条件读取的可判定量」：一次评价里的
// 每条规则都对着同一批冻结取值做判断。若各条规则各自去够原始边长，同一次评价里的两条规则
// 就可能对同一个包裹给出不一致的判断。
func TestPricingInputSnapshotDerivesFeaturesFromItsDimensions(t *testing.T) {
	input := syntheticInputWithDimensions(t, "1", "Z1", dimensions(t, "70", "8", "9", domain.LengthUnitInch))

	packageFeatures, err := input.Features()
	if err != nil {
		t.Fatalf("features: %v", err)
	}

	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, "48", domain.LengthUnitInch),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	matched, err := condition.Matches(packageFeatures)
	if err != nil {
		t.Fatalf("matches: %v", err)
	}
	if !matched {
		t.Fatalf("matched = false, want a 70 inch longest side to clear a 48 inch threshold")
	}
}

// Covers: CONTEXT「依据不足形成待判断…请求不合法或计算失败形成未形成；四者不得互相替代」—
// 从未携带尺寸的快照是事实缺失，补齐后可以重新评价；测量在场却不可用则是请求不合法。上下文
// 是特意把这两种结果分开的，所以它们不能以同一个错误抵达。
func TestPricingInputSnapshotWithoutDimensionsReportsThemMissingNotUnusable(t *testing.T) {
	input := syntheticInput(t, "1", "Z1")

	_, err := input.Features()
	if !errors.Is(err, domain.ErrMissingDimensions) {
		t.Fatalf("err = %v, want %v", err, domain.ErrMissingDimensions)
	}
	if errors.Is(err, domain.ErrInvalidDimensions) {
		t.Fatalf("err = %v, must not also read as unusable measurements", err)
	}
}

// Covers: CONTEXT「评价对象」—「它有且只有两种：已受理包裹…试算对象，在包裹尚未存在时由
// 发起试算的一方给出」：比几家供应商的卡发生在客户尚未承诺任何事之前，此时还没有已受理
// 包裹可评。硬要快照带上包裹身份，整个试算就没法表达了。
func TestPricingInputSnapshotAcceptsAnEstimateBeforeAnyPackageExists(t *testing.T) {
	subject, err := domain.NewEstimateSubject("estimate-1")
	if err != nil {
		t.Fatalf("estimate subject: %v", err)
	}
	if subject.Kind() != domain.SubjectEstimate {
		t.Fatalf("kind = %s, want %s", subject.Kind(), domain.SubjectEstimate)
	}

	input := syntheticInputForSubject(t, subject, "1", "Z1")

	if got, _ := input.Subject(); got.Kind() != domain.SubjectEstimate {
		t.Fatalf("snapshot subject kind = %s, want %s", got.Kind(), domain.SubjectEstimate)
	}
	if _, isPackage := input.PackageID(); isPackage {
		t.Fatalf("an estimate snapshot must not report a package identity")
	}
}

// 尺寸决定哪些规则命中，所以只差包裹边长的两次评价是两次不同的评价。共享评价语义摘要会让
// 对其中一次的重放冒充成对另一次的忠实重放，而摘要存在的意义正是挡住这件事。
func TestEvaluationDigestSeparatesInputsThatDifferOnlyInDimensions(t *testing.T) {
	plan := syntheticPlan(t, "digest-dimensions", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.PricingWeightActualOnly, nil)

	compact := evaluate(t, "eval-compact", plan,
		syntheticInputWithDimensions(t, "1", "Z1", dimensions(t, "10", "10", "10", domain.LengthUnitInch)))
	elongated := evaluate(t, "eval-elongated", plan,
		syntheticInputWithDimensions(t, "1", "Z1", dimensions(t, "70", "8", "9", domain.LengthUnitInch)))

	if compact.SemanticDigest() == elongated.SemanticDigest() {
		t.Fatalf("evaluations differing only in dimensions share digest %s", compact.SemanticDigest())
	}
}

// Covers: CONTEXT「对象种类进入评价语义摘要，因此回放时不可能把一种误认成另一种」— 同一个
// 引用串可以先指一次试算，之后再指业务据其受理的那个包裹。摘要若只记引用，重放就分不出试算
// 与那次允许变成钱的评价。
func TestEvaluationDigestSeparatesEstimateFromPackageSharingAReference(t *testing.T) {
	plan := syntheticPlan(t, "digest-subject", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.PricingWeightActualOnly, nil)
	estimate, err := domain.NewEstimateSubject("shared-reference")
	if err != nil {
		t.Fatalf("estimate subject: %v", err)
	}

	accepted := evaluate(t, "eval-accepted", plan,
		syntheticInputForSubject(t, packageSubject(t, "shared-reference"), "1", "Z1"))
	estimated := evaluate(t, "eval-estimated", plan,
		syntheticInputForSubject(t, estimate, "1", "Z1"))

	if accepted.SemanticDigest() == estimated.SemanticDigest() {
		t.Fatalf("estimate and accepted package share digest %s", accepted.SemanticDigest())
	}
}
