package identity_test

import (
	"bytes"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/identity"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// 本文件证本上下文这一侧的四条：签得出合法领域标识、重复签发不重号、两个工厂的标识
// 空间互不重叠、熵源出问题时如实报错。编码字母表与随机段长度是内核的事，钉在
// internal/platform/identity 那边，这里不重复。真库不参与——签发不落库正是这个实现
// 的取舍。

func TestMintedCaseIDsAreValidAndNeverRepeat(t *testing.T) {
	factory := newCaseIdentities(t)
	ctx := t.Context()

	seen := make(map[string]bool, 512)
	for range 512 {
		minted, err := factory.MintCaseID(ctx)
		if err != nil {
			t.Fatalf("签发案件标识：%v", err)
		}
		value := minted.String()
		if !strings.HasPrefix(value, "CASE-") {
			t.Fatalf("案件标识没带前缀：%s", value)
		}
		if seen[value] {
			t.Fatalf("案件标识重号：%s", value)
		}
		seen[value] = true
	}
}

func TestMintedSubmissionVersionsAreValidAndNeverRepeat(t *testing.T) {
	factory := newDeclarationVersions(t)
	ctx := t.Context()

	seen := make(map[string]bool, 512)
	for range 512 {
		minted, err := factory.NextSubmissionVersion(ctx)
		if err != nil {
			t.Fatalf("签发提交版本标识：%v", err)
		}
		value := minted.String()
		if !strings.HasPrefix(value, "DECLV-") {
			t.Fatalf("提交版本标识没带前缀：%s", value)
		}
		if seen[value] {
			t.Fatalf("提交版本标识重号：%s", value)
		}
		seen[value] = true
	}
}

// 两个标识空间分得开：一份提交版本号不会与某个案件号相等，因而不会有代码凭巧合
// 把其中一个当成另一个用。
func TestTheTwoIdentitySpacesDoNotOverlap(t *testing.T) {
	cases := newCaseIdentities(t)
	versions := newDeclarationVersions(t)
	ctx := t.Context()

	caseID, err := cases.MintCaseID(ctx)
	if err != nil {
		t.Fatalf("签发案件标识：%v", err)
	}
	versionID, err := versions.NextSubmissionVersion(ctx)
	if err != nil {
		t.Fatalf("签发提交版本标识：%v", err)
	}
	if caseID.String() == versionID.String() {
		t.Fatal("两个工厂签出了同一个标识")
	}
	if strings.HasPrefix(versionID.String(), "CASE-") {
		t.Fatalf("提交版本落进了案件标识空间：%s", versionID)
	}
}

// TestAStarvedEntropySourceRefusesToIssue 是切到内核后新钉住的一支：熵源读不满时
// 报错，而不是用读到的半截凑出一个可预测的标识。切之前的包级 mint() 不可注入，
// 这一支拿真熵源逼不出来，因此当时没有任何东西在守它。
func TestAStarvedEntropySourceRefusesToIssue(t *testing.T) {
	starved, err := identity.NewCaseIdentities(
		platformidentity.WithEntropy(bytes.NewReader([]byte{1, 2, 3})))
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if _, err := starved.MintCaseID(t.Context()); err == nil {
		t.Fatal("熵源只给了三个字节，签发却成功了")
	}
}

func TestANilEntropySourceIsRefusedAtConstruction(t *testing.T) {
	if _, err := identity.NewDeclarationVersions(platformidentity.WithEntropy(nil)); err == nil {
		t.Fatal("空熵源构造成功了")
	}
}

func newCaseIdentities(t *testing.T) *identity.CaseIdentities {
	t.Helper()
	factory, err := identity.NewCaseIdentities()
	if err != nil {
		t.Fatalf("构造案件标识工厂：%v", err)
	}
	return factory
}

func newDeclarationVersions(t *testing.T) *identity.DeclarationVersions {
	t.Helper()
	factory, err := identity.NewDeclarationVersions()
	if err != nil {
		t.Fatalf("构造提交版本工厂：%v", err)
	}
	return factory
}
