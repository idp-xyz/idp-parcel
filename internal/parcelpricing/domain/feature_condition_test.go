package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 卡把尺寸触发写成严格的「大于」：BND-003 记 48 英寸最长边为未命中，BND-004 记 48.01 为
// 命中。两处的阈值都是声明数据，从来不是代码里带的常量，所以测试按一个价卡版本会提供它的
// 方式供给。
func TestLengthFeatureConditionGreaterThanExcludesTheThresholdItself(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		longestSide string
		expected    bool
	}{
		{"below the threshold", "47.99", false},
		{"at the threshold", "48", false},
		{"just above the threshold", "48.01", true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			condition, err := domain.NewLengthFeatureCondition(
				domain.FeatureLongestSide,
				domain.ComparisonGreaterThan,
				length(t, "48", domain.LengthUnitInch),
			)
			if err != nil {
				t.Fatalf("condition: %v", err)
			}

			matched, err := condition.Matches(
				features(t, dimensions(t, testCase.longestSide, "30", "20", domain.LengthUnitInch)),
			)
			if err != nil {
				t.Fatalf("matches: %v", err)
			}
			if matched != testCase.expected {
				t.Fatalf("matched = %t, want %t", matched, testCase.expected)
			}
		})
	}
}

// Covers: CONTEXT「换算规则必须版本化声明，判定中不得隐式换算」— R40 以厘米声明上限而价表
// 按英寸工作，所以一个条件与它所读的特征确实可能在单位上不一致。本包不得替它们换算：卡必须
// 先把换算声明成一条版本化规则。这种不一致不是学术问题——108 英寸的边是 274.32 cm，越过了
// 274 cm 的上限，而直接比裸数字会报出相反的结论。
func TestLengthFeatureConditionRefusesToCompareAcrossUnits(t *testing.T) {
	condition, err := domain.NewLengthFeatureCondition(
		domain.FeatureLongestSide,
		domain.ComparisonGreaterThan,
		length(t, "274", domain.LengthUnitCentimeter),
	)
	if err != nil {
		t.Fatalf("condition: %v", err)
	}

	_, err = condition.Matches(features(t, dimensions(t, "108", "30", "20", domain.LengthUnitInch)))
	if !errors.Is(err, domain.ErrLengthUnitMismatch) {
		t.Fatalf("err = %v, want %v", err, domain.ErrLengthUnitMismatch)
	}
}
