package identity_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/identity"
)

// 本文件证标识签发的三条：签得出合法领域标识、重复签发不重号、两个工厂的标识空间
// 互不重叠。真库不参与——签发不落库正是这个实现的取舍。

func TestMintedCaseIDsAreValidAndNeverRepeat(t *testing.T) {
	factory := identity.NewCaseIdentities()
	ctx := t.Context()

	seen := make(map[string]bool, 512)
	for range 512 {
		minted, err := factory.MintCaseID(ctx)
		if err != nil {
			t.Fatalf("签发案件标识：%v", err)
		}
		value := minted.String()
		if !strings.HasPrefix(value, "CC-CASE-") {
			t.Fatalf("案件标识没带前缀：%s", value)
		}
		if seen[value] {
			t.Fatalf("案件标识重号：%s", value)
		}
		seen[value] = true
	}
}

func TestMintedSubmissionVersionsAreValidAndNeverRepeat(t *testing.T) {
	factory := identity.NewDeclarationVersions()
	ctx := t.Context()

	seen := make(map[string]bool, 512)
	for range 512 {
		minted, err := factory.NextSubmissionVersion(ctx)
		if err != nil {
			t.Fatalf("签发提交版本标识：%v", err)
		}
		value := minted.String()
		if !strings.HasPrefix(value, "CC-SUBV-") {
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
	cases := identity.NewCaseIdentities()
	versions := identity.NewDeclarationVersions()
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
	if strings.HasPrefix(versionID.String(), "CC-CASE-") {
		t.Fatalf("提交版本落进了案件标识空间：%s", versionID)
	}
}
