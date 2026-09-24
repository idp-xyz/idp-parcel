package customshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// IsolatedCommandIntake 是隔离写路径准入（ADR-0091）在关务主链命令面的注入式放行（票 operator-channel/08）。
//
// 它与本包的 IsolatedOperationsReadIntake 同层同款：租户格在构造时由装配点给定，进请求路径后不读任何授权输入——本包各口
// 的 Intake 契约都写着「采信报文自称的租户会穿透 ADR-0003 的隔离边界」，隔离形态的认证结果就是开关值。与读面不同的是它会
// 构造命令、命令会落行：可分辨物由租户维的 `SYN-` 前缀承担（ADR-0091 决定三）；前缀门禁、启用与否与启动日志都在
// cmd/parcel-api，本类型只收立得住的值。
//
// 外部事实的身份与发生时间照 ADR-0023 从载荷如实收；服务端只记它自己的接收时刻（注入时钟），两者分开。
//
// 逐口放行（ADR-0091 Consequences）：本类型只实现已成笔的口的 Intake 接口，未成笔的口在装配点仍挂字面量
// UnconfiguredIntake{}，且在类型上就装不进本类型——每放一口在这里多一个方法，装配点多换一行，两处都看得见。
type IsolatedCommandIntake struct {
	tenant domain.TenantID
	clock  ports.Clock
}

// 已成笔的口。每放一口在这里多一行断言、多一个方法，装配点多换一行。
var _ ResultIntake = (*IsolatedCommandIntake)(nil)

// IsolatedCommandIntakeDeps 是构造本 Intake 的全部输入：装配点给定的合成租户，与本进程的时钟。
type IsolatedCommandIntakeDeps struct {
	Tenant string
	Clock  ports.Clock
}

// NewIsolatedCommandIntake 由装配点以显式合成值构造。立不起来的值在这里拒：装配错误要在启动时暴露，不该等到第一个请求。
func NewIsolatedCommandIntake(deps IsolatedCommandIntakeDeps) (*IsolatedCommandIntake, error) {
	tenant, err := domain.NewTenantID(deps.Tenant)
	if err != nil {
		return nil, fmt.Errorf("customs compliance http: isolated command intake: %w", err)
	}
	if deps.Clock == nil {
		return nil, fmt.Errorf("customs compliance http: isolated command intake: clock is nil")
	}
	return &IsolatedCommandIntake{tenant: tenant, clock: deps.Clock}, nil
}

// externalResultDocument 是外部结果口的载荷，逐格镜像 application.ReceiveExternalResultCommand 去掉租户与接收时间——前者归
// 认证结果，后者是服务端自己的时刻。layer 取 domain.ResultLayer 的封闭词，occurredAt 取 RFC 3339；release 只在放行层给。
type externalResultDocument struct {
	SourceID       string                   `json:"sourceId"`
	Layer          string                   `json:"layer"`
	Role           string                   `json:"role"`
	RawSemantics   string                   `json:"rawSemantics"`
	ClaimedVersion string                   `json:"claimedVersion"`
	Attempt        int                      `json:"attempt"`
	Scope          string                   `json:"scope"`
	OccurredAt     string                   `json:"occurredAt"`
	Release        *externalReleaseDocument `json:"release,omitempty"`
}

// externalReleaseDocument 是放行层的放行三件。kind 取 domain.ReleaseKind 的封闭词。
type externalReleaseDocument struct {
	Kind      string `json:"kind"`
	Authority string `json:"authority"`
	Condition string `json:"condition,omitempty"`
}

// IntakeResult 译一条外部监管响应（`/customs/external-results`）。层与放行种类词表外、时刻解不出、放行机构立不住是坏报文
// （400）；其余成不成形——来源身份缺不缺、声称的提交归不归属得上、层与放行搭不搭——都由编排答（`未受理`与`归属不上`各自
// 成格），本 Intake 不预判。
func (intake *IsolatedCommandIntake) IntakeResult(
	_ context.Context,
	request *http.Request,
) (application.ReceiveExternalResultCommand, error) {
	none := application.ReceiveExternalResultCommand{}
	var document externalResultDocument
	if err := decodeClosedDocument(request.Body, &document); err != nil {
		return none, err
	}
	command := application.ReceiveExternalResultCommand{
		TenantID:       intake.tenant,
		SourceID:       document.SourceID,
		Role:           document.Role,
		RawSemantics:   document.RawSemantics,
		ClaimedVersion: document.ClaimedVersion,
		Attempt:        document.Attempt,
		Scope:          document.Scope,
		ReceivedAt:     intake.clock.Now().UTC(),
	}
	var err error
	if document.Layer != "" {
		if command.Layer, err = resultLayerFromWord(document.Layer); err != nil {
			return none, err
		}
	}
	if raw := strings.TrimSpace(document.OccurredAt); raw != "" {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return none, fmt.Errorf("%w: occurredAt is not an RFC 3339 instant: %v", ErrMalformedRequest, err)
		}
		command.OccurredAt = at.UTC()
	}
	if document.Release != nil {
		if command.Release, err = document.Release.content(); err != nil {
			return none, err
		}
	}
	return command, nil
}

func (release externalReleaseDocument) content() (*application.ReleaseContent, error) {
	kind, err := releaseKindFromWord(release.Kind)
	if err != nil {
		return nil, err
	}
	authority, err := domain.NewRegulatoryAuthorityReference(release.Authority)
	if err != nil {
		return nil, fmt.Errorf("%w: release.authority: %v", ErrMalformedRequest, err)
	}
	return &application.ReleaseContent{Kind: kind, Authority: authority, Condition: release.Condition}, nil
}

// resultLayerFromWord 与 releaseKindFromWord 是两个封闭集的名称镜像，词取各常量自己的 String()，不另立词表。
func resultLayerFromWord(raw string) (domain.ResultLayer, error) {
	for _, layer := range []domain.ResultLayer{
		domain.RegulatoryReceiptLayer, domain.BusinessAcceptanceLayer, domain.ProcessDecisionLayer,
		domain.AssessedDutyLayer, domain.ReleaseResultLayer, domain.DispositionDecisionLayer,
	} {
		if layer.String() == raw {
			return layer, nil
		}
	}
	return domain.ResultLayerInvalid, fmt.Errorf("%w: layer=%q is not a result layer word", ErrMalformedRequest, raw)
}

func releaseKindFromWord(raw string) (domain.ReleaseKind, error) {
	for _, kind := range []domain.ReleaseKind{domain.FullRelease, domain.PartialRelease, domain.ConditionalRelease} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return domain.ReleaseKindInvalid, fmt.Errorf("%w: release.kind=%q is not a release kind word", ErrMalformedRequest, raw)
}

// decodeClosedDocument 以封闭形状解载荷。未知键拒：载荷里出现 tenantId、receivedAt 之类的键不是可以忽略的噪声，是采信自报
// 的入口；尾随的第二个 JSON 值也拒：一份载荷只许一个文档，放过它就是无声丢掉一段输入。
func decodeClosedDocument(body io.Reader, target any) error {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the payload", ErrMalformedRequest)
	}
	return nil
}
