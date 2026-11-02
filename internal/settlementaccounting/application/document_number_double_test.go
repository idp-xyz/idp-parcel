package application_test

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// documentNumbersDouble 是取号端口的替身：每类单据按自己的前缀出三位流水，渲染出的号经调用方的「是否已用」判撞、已用就取
// 下一个，与编号组件同一个判法。outcome 置成时区未配置或撞号时直接答那一格；err 非空时答错误。
type documentNumbersDouble struct {
	prefixes map[docnumber.TypeCode]string
	next     map[docnumber.TypeCode]int
	outcome  ports.DocumentNumberOutcome
	err      error
	requests []ports.DocumentNumberRequest
}

func newDocumentNumbers() *documentNumbersDouble {
	return &documentNumbersDouble{
		prefixes: map[docnumber.TypeCode]string{
			domain.CustomerStatementDocumentNumberType.Code: "STMT-2026-08-",
			domain.StatementDisputeDocumentNumberType.Code:  "DSP-2026-09-",
			domain.AuditedPayableDocumentNumberType.Code:    "AP-2026-09-",
		},
		next: map[docnumber.TypeCode]int{},
	}
}

func (double *documentNumbersDouble) AssignDocumentNumber(
	ctx context.Context,
	request ports.DocumentNumberRequest,
) (ports.DocumentNumberAssignment, error) {
	double.requests = append(double.requests, request)
	if double.err != nil {
		return ports.DocumentNumberAssignment{}, double.err
	}
	if double.outcome != ports.DocumentNumberOutcomeInvalid && double.outcome != ports.DocumentNumberAssigned {
		return ports.DocumentNumberAssignment{Outcome: double.outcome}, nil
	}
	for {
		double.next[request.Type]++
		number := fmt.Sprintf("%s%03d", double.prefixes[request.Type], double.next[request.Type])
		used, err := request.IsUsed(ctx, number)
		if err != nil {
			return ports.DocumentNumberAssignment{}, err
		}
		if !used {
			return ports.DocumentNumberAssignment{Outcome: ports.DocumentNumberAssigned, Number: number}, nil
		}
	}
}

// requestsOf 交回某一类单据的取号请求，用例据此钉「取了几次号、按哪个时区」。
func (double *documentNumbersDouble) requestsOf(code docnumber.TypeCode) []ports.DocumentNumberRequest {
	var matched []ports.DocumentNumberRequest
	for _, request := range double.requests {
		if request.Type == code {
			matched = append(matched, request)
		}
	}
	return matched
}
