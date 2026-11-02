package application

import (
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ErrUnexpectedDocumentNumberIssue 说明取号端口交回了封闭集合以外的答案，或说取到了号却没给号。上抛而不译成业务结果：
// 照「没取到号」落单会造出一份无号原因不实的单据，照撞号不落单则把一次本该成立的开立吞掉。
var ErrUnexpectedDocumentNumberIssue = errors.New("visibility exception: unexpected document number issue")

// documentNumberOf 把一次取号译成单据上的那一格。第二个返回值为 true 是撞号：这一次开立不落单。
func documentNumberOf(issue ports.DocumentNumberIssue) (domain.DocumentNumber, bool, error) {
	switch issue.Outcome {
	case ports.DocumentNumberAssigned:
		number, err := domain.NewDocumentNumber(issue.Number)
		if err != nil {
			return domain.DocumentNumber{}, false, fmt.Errorf("%w: %v", ErrUnexpectedDocumentNumberIssue, err)
		}
		return number, false, nil
	case ports.DocumentNumberingTimeZoneNotConfigured:
		return domain.DocumentNumberAbsentForUnconfiguredTimeZone(), false, nil
	case ports.DocumentNumberingRuleCollision:
		return domain.DocumentNumber{}, true, nil
	default:
		return domain.DocumentNumber{}, false, fmt.Errorf("%w: %d", ErrUnexpectedDocumentNumberIssue, issue.Outcome)
	}
}
