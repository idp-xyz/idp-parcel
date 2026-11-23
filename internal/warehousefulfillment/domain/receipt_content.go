package domain

import "fmt"

// ReceiptContentIssueKind 是收货事实一处不合的种类。字面量是收货口答 CONTENT_INVALID 时 issues 里的 kind。
type ReceiptContentIssueKind uint8

const (
	ReceiptContentIssueKindInvalid ReceiptContentIssueKind = iota
	ReceiptIssueNoLines
	ReceiptIssueQuantityNotPositive
	ReceiptIssueLocationNotReceiving
	ReceiptIssueUnitNotPackagingLevel
	ReceiptIssueBatchRequired
	ReceiptIssueBatchNotTracked
	ReceiptIssueExpiryRequired
	ReceiptIssueExpiryNotTracked
	ReceiptIssueLabelAliasNotValid
	ReceiptIssueShapeMismatch
	ReceiptIssueWarehouseMismatch
	ReceiptIssueUnclaimedArrivalInvalid
	ReceiptIssueLocationDeactivated
)

func (kind ReceiptContentIssueKind) String() string {
	switch kind {
	case ReceiptIssueNoLines:
		return "NO_LINES"
	case ReceiptIssueQuantityNotPositive:
		return "QUANTITY_NOT_POSITIVE"
	case ReceiptIssueLocationNotReceiving:
		return "LOCATION_NOT_RECEIVING_STAGING"
	case ReceiptIssueUnitNotPackagingLevel:
		return "UNIT_NOT_A_PACKAGING_LEVEL"
	case ReceiptIssueBatchRequired:
		return "BATCH_NUMBER_REQUIRED"
	case ReceiptIssueBatchNotTracked:
		return "BATCH_NUMBER_NOT_TRACKED"
	case ReceiptIssueExpiryRequired:
		return "EXPIRY_REQUIRED"
	case ReceiptIssueExpiryNotTracked:
		return "EXPIRY_NOT_TRACKED"
	case ReceiptIssueLabelAliasNotValid:
		return "LABEL_ALIAS_NOT_VALID"
	case ReceiptIssueShapeMismatch:
		return "SHAPE_MISMATCH"
	case ReceiptIssueWarehouseMismatch:
		return "WAREHOUSE_MISMATCH"
	case ReceiptIssueUnclaimedArrivalInvalid:
		return "UNCLAIMED_ARRIVAL_INVALID"
	case ReceiptIssueLocationDeactivated:
		return "LOCATION_DEACTIVATED"
	default:
		return ""
	}
}

// ReceiptContentIssue 是收货事实的一处不合。Line 是行下标，整条用 -1。
// Carton 在这一口的行上没有箱单内的下标，整条与每一行都是 -1：整箱收货的行本身就是一行。
type ReceiptContentIssue struct {
	line   int
	carton int
	kind   ReceiptContentIssueKind
}

// WholeReceiptIssue 指出整条事实不合，不落在某一行上。
func WholeReceiptIssue(kind ReceiptContentIssueKind) ReceiptContentIssue {
	return ReceiptContentIssue{line: -1, carton: -1, kind: kind}
}

// ReceiptLineIssue 指出某一行不合。
func ReceiptLineIssue(line int, kind ReceiptContentIssueKind) ReceiptContentIssue {
	return ReceiptContentIssue{line: line, carton: -1, kind: kind}
}

func (issue ReceiptContentIssue) Line() int { return issue.line }

func (issue ReceiptContentIssue) Carton() int { return issue.carton }

func (issue ReceiptContentIssue) Kind() ReceiptContentIssueKind { return issue.kind }

func (issue ReceiptContentIssue) String() string {
	return fmt.Sprintf("%d:%d:%s", issue.line, issue.carton, issue.kind)
}
