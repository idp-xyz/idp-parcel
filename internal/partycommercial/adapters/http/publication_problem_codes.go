package commercialhttp

import (
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件立逐格问题的稳定身份（ADR-0316）：每一格立不住的记录除了原话（`problem`），还带问题码与参数。原话是领域错误或解析错误的
// `err.Error()`，措辞会随实现改；问题码是封闭词表，只增不改——一个码一旦发出就不换含义、不回收，客户端据此查自己的话术，
// 不必再按原话的形状猜。词表里认不出的格答 `INVALID`，客户端对它退回原话。
//
// 每个码的参数键也是契约的一部分，列在下面各常量的注释里；参数值都是字符串，缺省的键不出现。
const (
	// ProblemCodeInvalid 是兜底：这一格立不住，但没有更具体的码。没有参数。
	ProblemCodeInvalid = "INVALID"
	// ProblemCodeRequired：这一格必须填。参数 `subject`（可缺）：服务端点名的主语。
	ProblemCodeRequired = "REQUIRED"
	// ProblemCodeNotInSet：取值不在封闭集里。参数 `what`：这一格叫什么，`value`：收到的值。
	ProblemCodeNotInSet = "NOT_IN_SET"
	// ProblemCodeTimeFormat：时刻不是 RFC 3339。没有参数。
	ProblemCodeTimeFormat = "TIME_FORMAT"
	// ProblemCodeExactlyOne：几格里只能填其中一格。参数 `subject`：这几格合起来叫什么。
	ProblemCodeExactlyOne = "EXACTLY_ONE"
	// ProblemCodeRatioBaseNotAllowed：金额额度不带比例基数。没有参数。
	ProblemCodeRatioBaseNotAllowed = "RATIO_BASE_NOT_ALLOWED"
	// ProblemCodeRatioBaseRequired：比例额度必须声明基数。没有参数。
	ProblemCodeRatioBaseRequired = "RATIO_BASE_REQUIRED"
	// ProblemCodeDaysNotPositive：天数不是正整数。参数 `value`：收到的数。
	ProblemCodeDaysNotPositive = "DAYS_NOT_POSITIVE"
	// ProblemCodeOrderNotPositive：判断顺序不是从 1 起的正整数。参数 `value`：收到的数。
	ProblemCodeOrderNotPositive = "ORDER_NOT_POSITIVE"
	// ProblemCodeChargedUndeclared：计不计费没有声明。没有参数。
	ProblemCodeChargedUndeclared = "CHARGED_UNDECLARED"
	// ProblemCodeListEmptyOrDuplicate：清单为空，或同一原因出现了两条。没有参数。
	ProblemCodeListEmptyOrDuplicate = "LIST_EMPTY_OR_DUPLICATE"
)

// codedProblem 是带问题码与参数的原因：Error() 仍是原话，errors.Is / As 照常穿过它。
type codedProblem struct {
	code   string
	params map[string]string
	cause  error
}

func (problem *codedProblem) Error() string { return problem.cause.Error() }
func (problem *codedProblem) Unwrap() error { return problem.cause }

// coded 给一个原因贴上问题码与参数；原话不变。
func coded(code string, params map[string]string, cause error) error {
	return &codedProblem{code: code, params: params, cause: cause}
}

// problemCodeOf 取一个原因的问题码与参数：贴过码的取它的；领域的空值错误是`必填`；其余答兜底码。
func problemCodeOf(err error) (string, map[string]string) {
	var carried *codedProblem
	if errors.As(err, &carried) {
		return carried.code, carried.params
	}
	if errors.Is(err, domain.ErrBlankValue) {
		return ProblemCodeRequired, nil
	}
	return ProblemCodeInvalid, nil
}

// notInSet 是「集合外的 X "值"」：what 是这一格的叫法（不带「集合外的」前缀），detail 是原话里跟在值后面的补充（可空）。
func notInSet(what, value, detail string) error {
	return coded(ProblemCodeNotInSet, map[string]string{"what": what, "value": value},
		fmt.Errorf("集合外的%s %q%s", what, value, detail))
}

// requiredCell 是「某格须在场」：message 是原话，subject 是点名的主语（可空）。
func requiredCell(subject, message string) error {
	var params map[string]string
	if subject != "" {
		params = map[string]string{"subject": subject}
	}
	return coded(ProblemCodeRequired, params, errors.New(message))
}

// exactlyOne 是「几格须恰一格在场」。
func exactlyOne(subject, message string) error {
	return coded(ProblemCodeExactlyOne, map[string]string{"subject": subject}, errors.New(message))
}

// timeFormat 是时刻解析失败：空串是没填，答必填；其余是格式不对。
func timeFormat(raw string, cause error) error {
	original := fmt.Errorf("须为 RFC 3339 时刻：%v", cause)
	if strings.TrimSpace(raw) == "" {
		return coded(ProblemCodeRequired, nil, original)
	}
	return coded(ProblemCodeTimeFormat, nil, original)
}
