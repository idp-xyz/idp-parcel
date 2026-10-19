package parcelshipment_test

import (
	"errors"
	"strings"
	"testing"

	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证送站预报号码的形状门（ADR-0212 决定二「逐字保存」；票 ps-external-mark-relations/08）：界与 PS 入册门
// NewExternalIdentifierNumber 同一道——全空白、带 NUL、UTF-8 超过那道字节界的号进不了预报，其余逐字放行。界值只在 PS 一处，
// 这里不拿常量比，拿那道门本身比：边界两侧的号码长度由测试按字节造，PS 那一界改了，这里随之改判，而不是两份界各说各话。

// Covers: 号码形状门与 PS 入册门同一界——空白、NUL、超长各拒，答 ErrPreAdvisedNumberNotAdmitted；带空白、换行、大小写的号原样放行，
// 一个字符不折算。
func TestThePreAdvisedNumberGateIsTheExternalIdentifierRegisterGate(t *testing.T) {
	gate := noparcel.PreAdvisedNumberGate{}

	for _, admitted := range []string{"SYN-CPN-A1", " SYN-CPN-A1 ", "syn-cpn-a1", "SYN\tCPN\n01", "\x1dSYN-GS-01"} {
		number, err := gate.AdmitPreAdvisedNumber(admitted)
		if err != nil {
			t.Fatalf("%q: %v", admitted, err)
		}
		if number.String() != admitted {
			t.Fatalf("%q came back as %q——号码被折算了", admitted, number.String())
		}
	}

	for name, refused := range map[string]string{
		"空串":    "",
		"全空白":   " \t\n",
		"带 NUL": "SYN-CPN\x00-A1",
		"超长":    strings.Repeat("9", 4096),
	} {
		if _, err := gate.AdmitPreAdvisedNumber(refused); !errors.Is(err, noports.ErrPreAdvisedNumberNotAdmitted) {
			t.Fatalf("%s: err = %v, want ErrPreAdvisedNumberNotAdmitted", name, err)
		}
	}
}

// Covers: 界值不另立一份——在 PS 那一界的两侧各取一个号：最长的那个放得进、多一个字节的放不进。界从门本身二分出来，不抄常量。
func TestThePreAdvisedNumberGateSharesTheByteBoundWithParcelShipment(t *testing.T) {
	gate := noparcel.PreAdvisedNumberGate{}
	admits := func(length int) bool {
		_, err := gate.AdmitPreAdvisedNumber(strings.Repeat("9", length))
		return err == nil
	}
	low, high := 1, 1<<16
	if !admits(low) || admits(high) {
		t.Fatalf("门在 %d 与 %d 两端的答复不成界", low, high)
	}
	for high-low > 1 {
		middle := (low + high) / 2
		if admits(middle) {
			low = middle
		} else {
			high = middle
		}
	}
	bound := low
	registerAdmits := func(value string) bool {
		_, err := psdomain.NewExternalIdentifierNumber(value)
		return err == nil
	}
	if !registerAdmits(strings.Repeat("9", bound)) || registerAdmits(strings.Repeat("9", bound+1)) {
		t.Fatalf("预报号码门的界是 %d 字节，PS 入册门不在同一处分界", bound)
	}
	// 多字节字符按 UTF-8 字节计：界内的字节数用三字节的汉字凑满，再多一个就过界。
	if _, err := gate.AdmitPreAdvisedNumber(strings.Repeat("号", bound/3) + strings.Repeat("9", bound%3+1)); err == nil {
		t.Fatal("界按字节计，多字节字符不该按字符数放过")
	}
}
