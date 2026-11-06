package cataloguepage_test

import (
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// lifecycle 是测试用的领域枚举：零值无效，中间缺一格（3），末格之后答空串。
type lifecycle uint8

func (state lifecycle) String() string {
	switch state {
	case 1:
		return "SYN-REGISTERED"
	case 2:
		return "SYN-EFFECTIVE"
	case 4:
		return "SYN-DEACTIVATED"
	default:
		return ""
	}
}

// Covers: 词表取自领域枚举的 String()，按数值次序；中间缺格也不漏掉后面的格——领域枚举加一格时词表跟着加，声明里不另抄。
func TestWordsListEveryStateOfAnEnumInValueOrder(t *testing.T) {
	want := []string{"SYN-REGISTERED", "SYN-EFFECTIVE", "SYN-DEACTIVATED"}
	if got := cataloguepage.Words[lifecycle](); !reflect.DeepEqual(got, want) {
		t.Fatalf("得 %v，应为 %v", got, want)
	}
}

// Covers: 换算交回登记表里存的数值；词表之外的词报错，不换成零值去静默答空。
func TestCodeTurnsAWordIntoTheStoredValue(t *testing.T) {
	if code, err := cataloguepage.Code[lifecycle]("SYN-DEACTIVATED"); err != nil || code != 4 {
		t.Fatalf("得 %d（%v）", code, err)
	}
	for _, word := range []string{"", "SYN-ARCHIVED"} {
		if _, err := cataloguepage.Code[lifecycle](word); err == nil {
			t.Fatalf("%q 应换不成", word)
		}
	}
}
