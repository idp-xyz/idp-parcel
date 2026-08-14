package identity_test

import (
	"bytes"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
)

// 本文件证签发面的三条：签出来的值互不相同且能过领域构造、随机段不含易混字符、
// 熵源出问题时报错而不是交回一个可预测的值。

func TestEachIssuedIntakeResultVersionIsNew(t *testing.T) {
	factory := identity.NewIntakeResultVersions()
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

// TestIssuedVersionCarriesItsOrigin 钉住前缀：库行与日志里认得出这个版本是谁签的。
// 唯一性不靠它，所以这条只看前缀在，不看后面那段长什么样。
func TestIssuedVersionCarriesItsOrigin(t *testing.T) {
	version, err := identity.NewIntakeResultVersions().NextIntakeResultVersion(t.Context())
	if err != nil {
		t.Fatalf("签发：%v", err)
	}
	if !strings.HasPrefix(version.String(), "INTAKEV-") {
		t.Fatalf("版本 %q 没带来源前缀", version.String())
	}
}

// TestTheRandomSegmentAvoidsConfusableCharacters 钉住 base32 那条裁定的实际收益：
// 随机段里不出现 0、1、8、9，因此 0 与 O、1 与 I 不可能在照着工单念的时候混掉。
// 十六进制过不了这一条。
func TestTheRandomSegmentAvoidsConfusableCharacters(t *testing.T) {
	factory := identity.NewIntakeResultVersions()

	for range 256 {
		version, err := factory.NextIntakeResultVersion(t.Context())
		if err != nil {
			t.Fatalf("签发：%v", err)
		}
		segment := strings.TrimPrefix(version.String(), "INTAKEV-")
		if strings.ContainsAny(segment, "0189") {
			t.Fatalf("随机段 %q 含易混字符", segment)
		}
		if strings.ToUpper(segment) != segment {
			t.Fatalf("随机段 %q 不是全大写", segment)
		}
	}
}

// TestAStarvedEntropySourceRefusesToIssue 证熵源读不满时报错，而不是用读到的半截凑出
// 一个可预测的版本——收寄结果版本要跨上下文当幂等键，可预测等于可撞。
func TestAStarvedEntropySourceRefusesToIssue(t *testing.T) {
	starved, err := identity.NewIntakeResultVersionsFrom(bytes.NewReader([]byte{1, 2, 3}))
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if _, err := starved.NextIntakeResultVersion(t.Context()); err == nil {
		t.Fatal("熵源只给了三个字节，签发却成功了")
	}
}

func TestANilEntropySourceIsRefusedAtConstruction(t *testing.T) {
	if _, err := identity.NewIntakeResultVersionsFrom(nil); err == nil {
		t.Fatal("空熵源构造成功了")
	}
}
