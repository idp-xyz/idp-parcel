package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
)

// 送站预报与送站交接单开立时取单据号的那一段（NO CONTEXT「送站预报」「送站交接单」；ADR-0355 决定三、七、八）。送站预报与送站交接单都由网点开立：
// 预报指名送到哪个网点，交接单在网点当场开立，日期都按开立网点此刻生效的业务时区出；网点在网络目录里此刻没有生效版本，退到租户的
// 单据编号时区，两者都没有则不带号照常开立（决定七）。

// DocumentNumberingDeps 是取号要的取号编排与网点的业务时区。送站预报与送站交接单的编排各收一份。
type DocumentNumberingDeps struct {
	Issuer ports.DocumentNumberIssuer
	Nodes  ports.IssuingNodeTimeZones
}

func (deps DocumentNumberingDeps) complete() bool {
	return deps.Issuer != nil && deps.Nodes != nil
}

// documentNumberIssue 是这一段的答案。collided 为真时这一次开立不落单，调用方答「编号规则撞号」，进程级入口据此回滚开立事务。
type documentNumberIssue struct {
	numbering domain.DocumentNumbering
	collided  bool
}

// issueDocumentNumber 为开立网点是 node 的一张单据取号。只在确定要落单的那一步调（决定三）：调用方已锁住重放身份、读过没有在册的
// 那一份。网点的业务时区按 at 选版，at 取编排的时钟——那是服务端落单的时刻，不是设备或载荷声明的业务时刻（决定七）。
func issueDocumentNumber(
	ctx context.Context,
	deps DocumentNumberingDeps,
	tenant domain.TenantID,
	declaration docnumber.TypeDeclaration,
	node domain.NodeReference,
	clock ports.Clock,
	isUsed func(context.Context, domain.TenantID, string) (bool, error),
) (documentNumberIssue, error) {
	zone, found, err := deps.Nodes.NodeBusinessTimeZoneAt(ctx, tenant, node, clock.Now())
	if err != nil {
		return documentNumberIssue{}, fmt.Errorf("read the business time zone of node %s: %w", node, err)
	}
	if !found {
		zone = ""
	}
	issued, err := deps.Issuer.IssueDocumentNumber(ctx, ports.DocumentNumberRequest{
		Tenant:         tenant,
		Type:           declaration.Code,
		IssuerTimeZone: zone,
		IsUsed: func(ctx context.Context, number string) (bool, error) {
			return isUsed(ctx, tenant, number)
		},
	})
	if err != nil {
		return documentNumberIssue{}, err
	}
	switch issued.Outcome {
	case ports.DocumentNumberIssued:
		return documentNumberIssue{numbering: domain.NumberedDocument(issued.Number)}, nil
	case ports.DocumentNumberTimeZoneNotConfigured:
		return documentNumberIssue{numbering: domain.UnnumberedForMissingTimeZone()}, nil
	case ports.DocumentNumberRuleCollision:
		return documentNumberIssue{collided: true}, nil
	default:
		return documentNumberIssue{}, errors.New("issue document number: the issuer answered no outcome")
	}
}
