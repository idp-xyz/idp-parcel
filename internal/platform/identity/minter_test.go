package identity_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/identity"
)

// randomSegmentLength 是 128 位随机量编成不带填充 base32 后的字符数。
const randomSegmentLength = 26

func newMinter(t *testing.T, prefix string, options ...identity.Option) identity.Minter {
	t.Helper()

	minter, err := identity.NewMinter(prefix, options...)
	if err != nil {
		t.Fatalf("构造签发器：%v", err)
	}
	return minter
}

func TestAMintedIdentityCarriesItsPrefixAndAFullRandomSegment(t *testing.T) {
	minted, err := newMinter(t, "SUBV").Next()
	if err != nil {
		t.Fatalf("签发：%v", err)
	}

	prefix, segment, found := strings.Cut(minted, "-")
	if !found {
		t.Fatalf("标识没有分隔符：%q", minted)
	}
	if prefix != "SUBV" {
		t.Fatalf("前缀 = %q，want SUBV", prefix)
	}
	if len(segment) != randomSegmentLength {
		t.Fatalf("随机段长 = %d，want %d（少一位就是少几位熵）", len(segment), randomSegmentLength)
	}
}

// TestTheAlphabetExcludesTheDigitsThatLookLikeLetters 钉住选 base32 而非 hex 的那条理由。
// 没有这条，将来有人换回 hex 不会有任何东西变红，而工单上 0 与 O 就重新分不开了。
func TestTheAlphabetExcludesTheDigitsThatLookLikeLetters(t *testing.T) {
	minter := newMinter(t, "SUBV")

	for attempt := 0; attempt < 200; attempt++ {
		minted, err := minter.Next()
		if err != nil {
			t.Fatalf("第 %d 次签发：%v", attempt, err)
		}
		_, segment, _ := strings.Cut(minted, "-")
		if strings.ContainsAny(segment, "0189") {
			t.Fatalf("随机段出现易混数字：%q", segment)
		}
	}
}

func TestTwoMintedIdentitiesDiffer(t *testing.T) {
	minter := newMinter(t, "SUBV")

	first, err := minter.Next()
	if err != nil {
		t.Fatalf("首次签发：%v", err)
	}
	second, err := minter.Next()
	if err != nil {
		t.Fatalf("再次签发：%v", err)
	}
	if first == second {
		t.Fatalf("两次签发同值：%q", first)
	}
}

// TestAShortEntropyReadIsReportedNotPaddedOut 证熵源读不满时如实报错。这一支拿真实熵源
// 逼不出来，可注入熵源的全部意义就在这里。
func TestAShortEntropyReadIsReportedNotPaddedOut(t *testing.T) {
	// 只给 4 字节，io.ReadFull 因而读不满 16。
	minter := newMinter(t, "SUBV", identity.WithEntropy(strings.NewReader("abcd")))

	minted, err := minter.Next()
	if err == nil {
		t.Fatalf("熵源读不满却签出了 %q——半截随机是可预测的", minted)
	}
	if minted != "" {
		t.Fatalf("报错时仍交回了标识：%q", minted)
	}
}

func TestAPrefixCarryingTheSeparatorIsRejected(t *testing.T) {
	if _, err := identity.NewMinter("CC-CASE"); !errors.Is(err, identity.ErrInvalidPrefix) {
		t.Fatalf("含分隔符的前缀应被拒，实得：%v", err)
	}
}

func TestABlankPrefixIsRejected(t *testing.T) {
	if _, err := identity.NewMinter("   "); !errors.Is(err, identity.ErrInvalidPrefix) {
		t.Fatalf("空白前缀应被拒，实得：%v", err)
	}
}

// TestANilEntropyReaderIsRejected 证 nil 熵源不被静默回落。回落会让一处传错的装配看起来
// 一切正常，而它本来要换的熵源根本没生效。
func TestANilEntropyReaderIsRejected(t *testing.T) {
	if _, err := identity.NewMinter("SUBV", identity.WithEntropy(nil)); !errors.Is(err, identity.ErrInvalidEntropy) {
		t.Fatalf("nil 熵源应被拒，实得：%v", err)
	}
}

// TestTheZeroMinterFailsInsteadOfPanicking 证零值不可用时如实报错——它的熵源是 nil，
// 直接读会 panic，而 panic 在签发这条路径上会把一次装配错误变成一次进程崩溃。
func TestTheZeroMinterFailsInsteadOfPanicking(t *testing.T) {
	var zero identity.Minter

	if _, err := zero.Next(); !errors.Is(err, identity.ErrUnusableMinter) {
		t.Fatalf("零值签发器应报错，实得：%v", err)
	}
}
