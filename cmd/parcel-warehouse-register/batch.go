package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
)

type usageError struct{ reason string }

func (err usageError) Error() string { return err.reason }

func errUsage(reason string) error { return usageError{reason: reason} }

func decodeBatch(raw []byte) ([]batchRow, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var rows []batchRow
	if err := decoder.Decode(&rows); err != nil {
		return nil, fmt.Errorf("不是登记行数组：%w", err)
	}
	if decoder.More() {
		return nil, errors.New("登记行数组之后还有内容")
	}
	if len(rows) == 0 {
		return nil, errors.New("登记行数组是空的")
	}
	return rows, nil
}

func applyBatch(
	ctx context.Context,
	transactor bentoapp.Transactor,
	rows []batchRow,
	register func(context.Context, batchRow) (string, string, error),
) []rowReport {
	reports := make([]rowReport, 0, len(rows))
	for index, row := range rows {
		report := rowReport{Index: index + 1}
		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			outcome, refusal, regErr := register(txCtx, row)
			if regErr != nil {
				return regErr
			}
			report.Outcome = outcome
			report.Refusal = refusal
			report.Settled = outcome == "REGISTERED" || outcome == "ALREADY_REGISTERED"
			return nil
		})
		if err != nil {
			if usage, ok := asUsage(err); ok {
				report.Detail = usage
			} else {
				report.Technical = true
				report.Detail = err.Error()
			}
		}
		reports = append(reports, report)
	}
	return reports
}

func asUsage(err error) (string, bool) {
	var usage usageError
	if errors.As(err, &usage) {
		return usage.reason, true
	}
	var malformed *registrationjson.MalformedDocumentError
	if errors.As(err, &malformed) {
		return malformed.Reason, true
	}
	return "", false
}

func exitCode(reports []rowReport) int {
	code := exitSettled
	for _, report := range reports {
		switch {
		case report.Technical:
			return exitUndecided
		case report.Outcome == "CONTENT_CONFLICT":
			code = exitConflict
		case !report.Settled && code != exitConflict:
			code = exitRefused
		}
	}
	return code
}

func (report rowReport) line() string {
	switch {
	case report.Technical:
		return fmt.Sprintf("第 %d 行 未决 %s", report.Index, report.Detail)
	case report.Detail != "":
		return fmt.Sprintf("第 %d 行 未受理 %s", report.Index, report.Detail)
	case report.Refusal != "":
		return fmt.Sprintf("第 %d 行 %s %s", report.Index, report.Outcome, report.Refusal)
	default:
		return fmt.Sprintf("第 %d 行 %s", report.Index, report.Outcome)
	}
}
