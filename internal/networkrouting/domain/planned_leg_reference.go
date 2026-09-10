package domain

import (
	"errors"
	"strconv"
	"strings"
)

// ErrInvalidPlannedLegReference 拒绝立不住的计划履约段引用：没有版本、序位非正、或拼写不能原样往返。
var ErrInvalidPlannedLegReference = errors.New("network routing: invalid planned leg reference")

// plannedLegReferenceSeparator 是引用拼写里版本与序位之间的分隔符。只在本文件出现一次：拼写由本上下文
// 一处定义（ADR-0131 决定一），TF 与路由指令只搬运这个串，不解读它。
const plannedLegReferenceSeparator = "#"

// PlannedLegReference 是计划履约段对外的引用：路由计划版本标识 + 段在段链中的序位。本上下文不给
// 计划履约段另铸身份（ADR-0131 决定一）——版本不可变、段链连续且至少一段由 FormInitialRoutePlan
// 保证，「版本 + 序位」已经是完整且稳定的身份，再铸一个段标识只是给同一样东西第二个名字。
//
// 序位自首段起计，首段为 1（与 CONTEXT「自首段起计」同口径）。不从 0 起：0 在拼写里读不出「第一段」
// 的意思，而这个串会被登记方与现场的人读到。
//
// 引用只钉「哪一版的哪一段」，不带适用性：取的是引用所钉那一版的内容，那一版此刻还算不算数由适用性
// 记录另答（ADR-0131 决定二）。
type PlannedLegReference struct {
	version RoutePlanVersionID
	ordinal int
}

// NewPlannedLegReference 是构造门：版本缺席或序位非正一律拒——没有版本的序位不指任何一版计划，
// 0 与负数不指任何一段。
func NewPlannedLegReference(version RoutePlanVersionID, ordinal int) (PlannedLegReference, error) {
	if !version.valid() || ordinal < 1 {
		return PlannedLegReference{}, ErrInvalidPlannedLegReference
	}
	return PlannedLegReference{version: version, ordinal: ordinal}, nil
}

// ParsePlannedLegReference 把 String 写出的拼写解析回引用，两者往返相等。
//
// 从**最后一个**分隔符切：版本标识是一个自由字串，不禁止它含分隔符，而序位在末尾且只由数字组成，
// 从末尾切总是无歧义。序位那一段只认 strconv.Itoa 会写出的形状——前导零、正号、空白都拒：接受一个
// 写回去不一样的串，TF 存的与本上下文认的就是两份拼写，往返相等守不住。
func ParsePlannedLegReference(value string) (PlannedLegReference, error) {
	cut := strings.LastIndex(value, plannedLegReferenceSeparator)
	if cut < 0 {
		return PlannedLegReference{}, ErrInvalidPlannedLegReference
	}
	rawVersion, rawOrdinal := value[:cut], value[cut+len(plannedLegReferenceSeparator):]
	ordinal, err := strconv.Atoi(rawOrdinal)
	if err != nil || strconv.Itoa(ordinal) != rawOrdinal {
		return PlannedLegReference{}, ErrInvalidPlannedLegReference
	}
	version, err := NewRoutePlanVersionID(rawVersion)
	if err != nil {
		return PlannedLegReference{}, ErrInvalidPlannedLegReference
	}
	return NewPlannedLegReference(version, ordinal)
}

func (reference PlannedLegReference) Version() RoutePlanVersionID {
	return reference.version
}

// Ordinal 交回段在段链中的序位，自首段起计、首段为 1。
func (reference PlannedLegReference) Ordinal() int {
	return reference.ordinal
}

// String 交回唯一拼写：`<版本>#<序位>`，如 `RPV-000000000007#2` 指该版本的第二段。
func (reference PlannedLegReference) String() string {
	return reference.version.String() + plannedLegReferenceSeparator + strconv.Itoa(reference.ordinal)
}
