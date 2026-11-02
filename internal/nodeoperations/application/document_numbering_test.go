package application_test

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 送站预报与送站交接单取单据号要的两只替身（ADR-0355 决定三、七、八）。夹具全为合成（S 级）。

// documentNumbersDouble 是取号编排替身：按类型依次出 `<类型>-<序>`，出号前问 IsUsed、已用就跳过；outcome 非零时一律答那一格，err 非空
// 时交回错误。asked 记下每一次取号，测试据此判「重放没有再取号」与交进来的业务时区。
type documentNumbersDouble struct {
	next    map[string]int
	outcome ports.DocumentNumberIssueOutcome
	err     error
	asked   []ports.DocumentNumberRequest
}

func newDocumentNumbers() *documentNumbersDouble {
	return &documentNumbersDouble{next: map[string]int{}}
}

func (double *documentNumbersDouble) IssueDocumentNumber(ctx context.Context, request ports.DocumentNumberRequest) (ports.DocumentNumberIssue, error) {
	double.asked = append(double.asked, request)
	if double.err != nil {
		return ports.DocumentNumberIssue{}, double.err
	}
	if double.outcome != ports.DocumentNumberIssueInvalid && double.outcome != ports.DocumentNumberIssued {
		return ports.DocumentNumberIssue{Outcome: double.outcome}, nil
	}
	for {
		double.next[string(request.Type)]++
		value := fmt.Sprintf("%s-%04d", request.Type, double.next[string(request.Type)])
		used, err := request.IsUsed(ctx, value)
		if err != nil {
			return ports.DocumentNumberIssue{}, err
		}
		if used {
			continue
		}
		number, err := domain.NewDocumentNumber(value)
		if err != nil {
			return ports.DocumentNumberIssue{}, err
		}
		return ports.DocumentNumberIssue{Outcome: ports.DocumentNumberIssued, Number: number}, nil
	}
}

// nodeTimeZonesDouble 按网点答预设的业务时区；没登的网点答 found=false。asked 记下每一问的时刻。
type nodeTimeZonesDouble struct {
	zones map[string]string
	err   error
	asked []time.Time
}

func (double *nodeTimeZonesDouble) NodeBusinessTimeZoneAt(_ context.Context, _ domain.TenantID, node domain.NodeReference, at time.Time) (string, bool, error) {
	double.asked = append(double.asked, at)
	if double.err != nil {
		return "", false, double.err
	}
	zone, found := double.zones[node.String()]
	return zone, found, nil
}

func documentNumberingDeps(issuer ports.DocumentNumberIssuer, nodes ports.IssuingNodeTimeZones) application.DocumentNumberingDeps {
	return application.DocumentNumberingDeps{Issuer: issuer, Nodes: nodes}
}
