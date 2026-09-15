package outboxintent_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// Covers: 票 sa-cc/34 裁决 2 / 做法 (1)——FingerprintEventID 是各上下文 outbox 适配器铸信封 ID 的公共公式：
// 口名前缀 + "/" + 各维以 \x00 拼接后的 sha256 十六进制。期望值取独立算出的字面（PowerShell 对字节 61 00 62 的
// SHA256），不用生产公式重算——重算等于让用例永远同意代码。
func TestFingerprintEventIDIsThePortNamePlusTheHexDigestOfNulJoinedDimensions(t *testing.T) {
	got := outboxintent.FingerprintEventID("port", "a", "b")
	want := eventing.EventID("port/59b271ae1bbcb1d31d41929817f4b16fb439eb4f31520b5ad1d5ce98920a7138")
	if got != want {
		t.Fatalf("FingerprintEventID(port, a, b) = %q, want %q", got, want)
	}
}

// Covers: 判据 (1) 的定长半边——维度多长 ID 都是「前缀 + 六十四位」，且落在 eventing.MaxEventIDLength 内；
// 引用长度归实例半边，这里用对上限量出来的合成串，不假定任何租户的引用多长。
func TestFingerprintEventIDStaysWithinTheFrameworkLimitHoweverLongTheDimensionsAre(t *testing.T) {
	long := strings.Repeat("x", eventing.MaxEventIDLength)
	got := outboxintent.FingerprintEventID("customs-case", long, long, long, long, long)
	if len(got) != len("customs-case/")+64 {
		t.Fatalf("ID 长度 = %d，该是口名前缀加六十四位十六进制", len(got))
	}
	if len(got) > eventing.MaxEventIDLength {
		t.Fatalf("ID 长度 %d 超过框架上限 %d", len(got), eventing.MaxEventIDLength)
	}
	if again := outboxintent.FingerprintEventID("customs-case", long, long, long, long, long); again != got {
		t.Fatalf("同输入两次算出不同 ID：%q / %q", got, again)
	}
}

// Covers: 做法 (1) 的两条互异要求——同维不同口名互不相交（EnqueueOnce 按（来源, 事件 ID）查重，几只口共用一个
// 来源名）；维度以 \x00 拼接，所以「a/b」+「c」与「a」+「b/c」不是同一份——串接形正是在这一点上会把两份算成一个 ID。
func TestFingerprintEventIDKeepsPortsAndDimensionBoundariesApart(t *testing.T) {
	if outboxintent.FingerprintEventID("gate-verification", "t", "s") == outboxintent.FingerprintEventID("disposition-verification", "t", "s") {
		t.Fatal("不同口名同维度算出了同一个 ID")
	}
	if outboxintent.FingerprintEventID("port", "a/b", "c") == outboxintent.FingerprintEventID("port", "a", "b/c") {
		t.Fatal("维度边界移位算出了同一个 ID——分隔符没有把维度隔开")
	}
}
