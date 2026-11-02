package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CustomsPortDisplayNameMaxRunes 是显示名称按码点计的上限（ADR-0353 决定三，产品策略）。
const CustomsPortDisplayNameMaxRunes = 100

// ErrCustomsPortDisplayNameMalformed 表示登记方给了显示名称、却不成形。给了就要成形：空白串是写坏了，不是没给（缺席才是没登）。
var ErrCustomsPortDisplayNameMalformed = errors.New("customs compliance: customs port display name is malformed")

// CustomsPortDisplayName 是口岸目录一版上可选的显示名称（ADR-0353）：只供人辨认，不是标识、不唯一，也不进关务适用性判断的作答、出处与判断标识。
// 零值即这一版没登名称——读者照旧以口岸标识称呼它，不拿标识或别的什么顶一个名称出来。
//
// 名称随版本登记：改名是登一个起点更晚的新版本，旧版本照留它当时的名称。同一起点重登时名称并入逐字段比对。
type CustomsPortDisplayName struct{ text string }

// NewCustomsPortDisplayName 按产品策略的约束收下名称；不合格即拒，不截断、不去空白、不改写——静默改写会让登进去的与登记方写的
// 不是同一串字。
func NewCustomsPortDisplayName(text string) (CustomsPortDisplayName, error) {
	if defect := displayNameDefect(text, CustomsPortDisplayNameMaxRunes); defect != "" {
		return CustomsPortDisplayName{}, fmt.Errorf("%w：%s", ErrCustomsPortDisplayNameMalformed, defect)
	}
	return CustomsPortDisplayName{text: text}, nil
}

// Registered 说这一版登没登名称。
func (name CustomsPortDisplayName) Registered() bool { return name.text != "" }

func (name CustomsPortDisplayName) String() string { return name.text }

// displayNameDefect 指名一串名称不成形在哪；成形答空串。首尾空白、控制字符、格式字符与行段分隔符都拒：前两样在列表里
// 看不出差别却让两个名称不等，格式字符里有双向覆盖符，能让显示出来的字序与存下的不同。
//
// 约束与物流节点的显示名称是同一套（ADR-0353 决定八）。两个上下文各写一份而不共用：领域包互不依赖，`internal/platform`
// 只放技术设施；两边的用例各自钉住同一组边界。
func displayNameDefect(text string, maxRunes int) string {
	switch {
	case !utf8.ValidString(text):
		return "不是有效的 UTF-8"
	case strings.TrimSpace(text) == "":
		return "是空白"
	case strings.TrimSpace(text) != text:
		return "首尾带空白"
	case utf8.RuneCountInString(text) > maxRunes:
		return fmt.Sprintf("超过 %d 个字符", maxRunes)
	}
	for _, r := range text {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return fmt.Sprintf("含不可显示的字符 %U", r)
		}
	}
	return ""
}
