package domain

// DocumentNumber 是本系统在一份运输委托或订舱开立时分配的单据号（GLOSSARY「单据号」，ADR-0355 决定一）。它不是内部
// 标识：委托、订舱标识照旧做主键与幂等键，单据号不参与判重；它也不是外部标识——承运方答复里的订舱号由承运方分配，
// 不取代它。
//
// 单据号进（租户，单据号）唯一约束，所以与订舱标识同过字节界：超界的值在构造期就拒，不落到写库那一刻整笔回滚。
type DocumentNumber struct{ requiredValue }

func NewDocumentNumber(value string) (DocumentNumber, error) {
	required, err := newStoredReference("document number", value)
	return DocumentNumber{required}, err
}

// DocumentNumberAbsence 说一份没有单据号的单据为什么没有（GLOSSARY「单据号」两种情形）。零值是「本规则生效前开立」：
// 库里单据号与无号原因两列都空的旧行读回就是它，迁移不回填，事后也不补号。
type DocumentNumberAbsence uint8

const (
	// IssuedBeforeDocumentNumbering：本规则生效前开立。
	IssuedBeforeDocumentNumbering DocumentNumberAbsence = iota
	// DocumentNumberingTimeZoneNotConfigured：开立时开立方没有业务时区、租户也没登记单据编号时区（ADR-0355 决定七），单据
	// 照常开立、不带号。之后补登时区也不给它补号。
	DocumentNumberingTimeZoneNotConfigured
)

func (absence DocumentNumberAbsence) String() string {
	switch absence {
	case IssuedBeforeDocumentNumbering:
		return "ISSUED_BEFORE_NUMBERING"
	case DocumentNumberingTimeZoneNotConfigured:
		return "NUMBERING_TIME_ZONE_NOT_CONFIGURED"
	default:
		return ""
	}
}

// DocumentNumbering 是单据上「单据号」那一格：要么有号，要么记着为什么没有。两者互斥，零值读作本规则生效前开立。
//
// 它与提交人同属记录上的一项，不进委托、订舱本体，也不进内容摘要：同一开立意图重放比的是内容，答的是在册那一份连同
// 它的号；号随单开立后不变，换版、取消、开始都不碰它（ADR-0355 决定三）。
type DocumentNumbering struct {
	number  DocumentNumber
	absence DocumentNumberAbsence
}

// NumberedDocument 交回带号的那一格。
func NumberedDocument(number DocumentNumber) DocumentNumbering {
	return DocumentNumbering{number: number}
}

// UnnumberedDocument 交回不带号、记着原因的那一格。
func UnnumberedDocument(absence DocumentNumberAbsence) DocumentNumbering {
	return DocumentNumbering{absence: absence}
}

// Number 交回单据号；没有号时 ok 为假。
func (numbering DocumentNumbering) Number() (DocumentNumber, bool) {
	return numbering.number, numbering.number.valid()
}

// Absence 交回无号原因；有号时 ok 为假。
func (numbering DocumentNumbering) Absence() (DocumentNumberAbsence, bool) {
	if numbering.number.valid() {
		return 0, false
	}
	return numbering.absence, true
}
