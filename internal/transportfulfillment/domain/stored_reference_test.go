package domain_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: ADR-0239 决定三的字节界——订舱标识与承运答复标识到 512 字节照收，多一个字节即拒，拒的是 ErrValueTooLongToStore
// （确定的坏输入，不是依赖故障）；按 UTF-8 字节计，不按字符计。答复依据不进键，不受这道界。
func TestBookingAnswerReferencesHaveAStorableByteBound(t *testing.T) {
	constructors := map[string]func(string) error{
		"订舱标识": func(value string) error { _, err := domain.NewBookingReference(value); return err },
		"答复标识": func(value string) error { _, err := domain.NewCarrierAcceptanceReference(value); return err },
	}
	for name, construct := range constructors {
		if err := construct(strings.Repeat("A", 512)); err != nil {
			t.Fatalf("%s 512 字节：%v", name, err)
		}
		if err := construct(strings.Repeat("A", 513)); !errors.Is(err, domain.ErrValueTooLongToStore) {
			t.Fatalf("%s 513 字节：%v，want ErrValueTooLongToStore", name, err)
		}
		// 171 个三字节字符是 513 字节：按字符数只有 171，照收就是按字符计了。
		if err := construct(strings.Repeat("订", 171)); !errors.Is(err, domain.ErrValueTooLongToStore) {
			t.Fatalf("%s 171 个汉字（513 字节）：%v，want ErrValueTooLongToStore", name, err)
		}
		if err := construct(" "); !errors.Is(err, domain.ErrBlankValue) {
			t.Fatalf("%s 全空白：%v——必备值那道门被跳过了", name, err)
		}
	}
	if _, err := domain.NewAcceptanceBasisReference(strings.Repeat("订", 1000)); err != nil {
		t.Fatalf("答复依据 3000 字节：%v——不进键的格不该受字节界", err)
	}
}
