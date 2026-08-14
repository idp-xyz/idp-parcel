package identity_test

import (
	"bytes"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// 本文件证本上下文这一侧的三条：签出来的值互不相同且过得了领域构造、带得上本上下文的
// 前缀、熵源出问题时如实报错而不是交回一个可预测的值。编码字母表与随机段长度是内核的
// 事，钉在 internal/platform/identity 那边，这里不重复。

func TestEachIssuedIntakeResultVersionIsNew(t *testing.T) {
	factory := newFactory(t)
	ctx := t.Context()

	issued := make(map[string]struct{}, 512)
	for range 512 {
		version, err := factory.NextIntakeResultVersion(ctx)
		if err != nil {
			t.Fatalf("签发：%v", err)
		}
		if version.String() == "" {
			t.Fatal("签出了空版本；领域构造本该先拒")
		}
		if _, repeated := issued[version.String()]; repeated {
			t.Fatalf("重号：%s", version.String())
		}
		issued[version.String()] = struct{}{}
	}
}

// TestIssuedVersionCarriesItsOrigin 钉住前缀：日志与工单里认得出这是个收寄结果版本。
// 唯一性不靠它，所以这条只看前缀在，不看后面那段长什么样。
func TestIssuedVersionCarriesItsOrigin(t *testing.T) {
	version, err := newFactory(t).NextIntakeResultVersion(t.Context())
	if err != nil {
		t.Fatalf("签发：%v", err)
	}
	if !strings.HasPrefix(version.String(), "INTAKEV-") {
		t.Fatalf("版本 %q 没带来源前缀", version.String())
	}
}

// TestAStarvedEntropySourceRefusesToIssue 证熵源读不满时报错，而不是用读到的半截凑出
// 一个可预测的版本——收寄结果版本要跨上下文当幂等键，可预测等于可撞。
func TestAStarvedEntropySourceRefusesToIssue(t *testing.T) {
	starved, err := identity.NewIntakeResultVersions(
		platformidentity.WithEntropy(bytes.NewReader([]byte{1, 2, 3})))
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if _, err := starved.NextIntakeResultVersion(t.Context()); err == nil {
		t.Fatal("熵源只给了三个字节，签发却成功了")
	}
}

func TestANilEntropySourceIsRefusedAtConstruction(t *testing.T) {
	if _, err := identity.NewIntakeResultVersions(platformidentity.WithEntropy(nil)); err == nil {
		t.Fatal("空熵源构造成功了")
	}
}

func newFactory(t *testing.T) *identity.IntakeResultVersions {
	t.Helper()
	factory, err := identity.NewIntakeResultVersions()
	if err != nil {
		t.Fatalf("构造签发器：%v", err)
	}
	return factory
}
