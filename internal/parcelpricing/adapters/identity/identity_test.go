package identity_test

import (
	"bytes"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/identity"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// 本文件证本上下文这一侧的三条：签得出合法的评价标识、重复签发不重号、熵源出问题时如实报错。编码字母表与随机
// 段长度是内核的事，钉在 internal/platform/identity 那边，这里不重复。真库不参与——铸造不落库正是这个实现的取舍。

func TestMintedEvaluationIDsAreValidAndNeverRepeat(t *testing.T) {
	factory, err := identity.NewEvaluationIdentities()
	if err != nil {
		t.Fatalf("构造评价标识工厂：%v", err)
	}
	ctx := t.Context()

	seen := make(map[string]bool, 512)
	for range 512 {
		minted, err := factory.MintEvaluationID(ctx)
		if err != nil {
			t.Fatalf("签发评价标识：%v", err)
		}
		value := minted.String()
		if !strings.HasPrefix(value, "EVAL-") {
			t.Fatalf("评价标识没带前缀：%s", value)
		}
		if seen[value] {
			t.Fatalf("评价标识重号：%s", value)
		}
		seen[value] = true
	}
}

// 熵源读不满时报错，而不是用读到的半截凑出一个可预测的标识。
func TestAStarvedEntropySourceRefusesToIssueAnEvaluationID(t *testing.T) {
	starved, err := identity.NewEvaluationIdentities(
		platformidentity.WithEntropy(bytes.NewReader([]byte{1, 2, 3})))
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if _, err := starved.MintEvaluationID(t.Context()); err == nil {
		t.Fatal("熵源只给了三个字节，签发却成功了")
	}
}

func TestANilEntropySourceIsRefusedAtConstruction(t *testing.T) {
	if _, err := identity.NewEvaluationIdentities(platformidentity.WithEntropy(nil)); err == nil {
		t.Fatal("空熵源构造成功了")
	}
}
