package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidDocumentNumber 表示单据号那一格立不起来：号是空白，或读回的无号原因不在封闭集合里，或号与无号原因同时在场。
var ErrInvalidDocumentNumber = errors.New("visibility exception: invalid document number")

// NumberlessReason 说一份单据为什么没有单据号（GLOSSARY「单据号」：没有单据号的单据记下是哪一种情形，事后都不补号）。
type NumberlessReason string

const (
	// IssuedBeforeNumbering：单据在本上下文开始取号之前就已开立（ADR-0355 决定三：不补号）。库里两格都空的旧行就是它，
	// 不另落一个值——旧行没有被迁移改写过。
	IssuedBeforeNumbering NumberlessReason = "ISSUED_BEFORE_NUMBERING"
	// NumberingTimeZoneNotConfigured：开立时租户没有生效的单据编号时区，单据照常开立、不带号（ADR-0355 决定七）。异常案件
	// 与客户索赔项的开立方都没有业务时区，日期只能按租户的单据编号时区，这一格因此对两类单据都可能出现。
	NumberingTimeZoneNotConfigured NumberlessReason = "NUMBERING_TIME_ZONE_NOT_CONFIGURED"
)

// DocumentNumber 是一份单据上的单据号那一格：开立时取到的号，或者没有号以及为什么没有。
//
// 零值是「本规则生效前开立」。新开立的单据不许带着零值落库——那会把「开立时没取号」伪装成「上线前开立」，而后者正是
// 页面上要与「时区未配置」分开说的两种情形；开立路径只用 NewDocumentNumber 与 NumberlessForUnconfiguredTimeZone 两个
// 构造，由 issuedAtOpening 把门。
type DocumentNumber struct {
	number                string
	timeZoneNotConfigured bool
}

// NewDocumentNumber 收下开立事务里取到的那个号。号的格式归规则册（租户版或产品默认），本上下文不复判，只拒空白。
func NewDocumentNumber(value string) (DocumentNumber, error) {
	if strings.TrimSpace(value) == "" {
		return DocumentNumber{}, fmt.Errorf("%w: blank number", ErrInvalidDocumentNumber)
	}
	return DocumentNumber{number: value}, nil
}

// NumberlessForUnconfiguredTimeZone 是开立时单据编号时区未配置的那一格：单据照常开立，记下无号原因。
func NumberlessForUnconfiguredTimeZone() DocumentNumber {
	return DocumentNumber{timeZoneNotConfigured: true}
}

// RehydrateDocumentNumber 从库里两格（单据号、无号原因）重建。两格都空是上线前开立的旧行；无号原因只认「单据编号时区
// 未配置」一个值——「上线前开立」不落值，库里出现它说明写坏了。
func RehydrateDocumentNumber(number, reason string) (DocumentNumber, error) {
	switch {
	case number != "" && reason != "":
		return DocumentNumber{}, fmt.Errorf("%w: both a number and a numberless reason", ErrInvalidDocumentNumber)
	case number != "":
		return NewDocumentNumber(number)
	case reason == "":
		return DocumentNumber{}, nil
	case NumberlessReason(reason) == NumberingTimeZoneNotConfigured:
		return NumberlessForUnconfiguredTimeZone(), nil
	default:
		return DocumentNumber{}, fmt.Errorf("%w: numberless reason %q", ErrInvalidDocumentNumber, reason)
	}
}

// Number 交回单据号；第二个返回值为 false 时这份单据没有号，原因看 NumberlessReason。
func (documentNumber DocumentNumber) Number() (string, bool) {
	return documentNumber.number, documentNumber.number != ""
}

// NumberlessReason 只在没有号时有意义；有号时交回空串。
func (documentNumber DocumentNumber) NumberlessReason() NumberlessReason {
	switch {
	case documentNumber.number != "":
		return ""
	case documentNumber.timeZoneNotConfigured:
		return NumberingTimeZoneNotConfigured
	default:
		return IssuedBeforeNumbering
	}
}

// StoredNumberlessReason 是落库的那一格：只有「单据编号时区未配置」落值，有号与上线前开立都落空。
func (documentNumber DocumentNumber) StoredNumberlessReason() string {
	if documentNumber.number == "" && documentNumber.timeZoneNotConfigured {
		return string(NumberingTimeZoneNotConfigured)
	}
	return ""
}

// issuedAtOpening 答这一格是不是开立那一刻形成的：取到了号，或者记下了时区未配置。零值（上线前开立）不是。
func (documentNumber DocumentNumber) issuedAtOpening() bool {
	return documentNumber.number != "" || documentNumber.timeZoneNotConfigured
}
