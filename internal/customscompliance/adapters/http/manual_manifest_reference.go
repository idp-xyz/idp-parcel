package customshttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件是外部监管舱单引用的手工录入口（dox/decisions.md「产品形态」）：舱单由责任承运商在外部形成并提交，没有对接
// 承运商系统时，操作员照承运商给的材料把引用（及其版本推进）录进来。进的是集成客户端口后面的同一只编排。
//
// 引用按（承运商责任，舱单身份）成行，一份舱单只有一条当前引用——它是外部世界里的同一份东西，不因谁录的而分成两条。
// 所以人工录入与承运商系统共用这一格：同版本同内容是重放，同版本异内容是内容冲突（由人复核），异版本走版本推进。

// OperatorDecisionExternalManifestEntry 是人工录入外部监管舱单的引用及其版本推进。
const OperatorDecisionExternalManifestEntry OperatorDecision = "EXTERNAL_MANIFEST_ENTRY"

// manualManifestReferenceDocument 是手工首登口的线格式：与集成客户端口同形，去掉 sourceFact（由 original 充当），另加
// carrier——这份舱单的责任承运商。集成客户端口的承运商取客户端绑定；这里由录入的人照材料指名，录入人记在留痕上。
type manualManifestReferenceDocument struct {
	ManifestID string `json:"manifestId"`
	Version    string `json:"version"`
	Procedure  string `json:"procedure"`
	Direction  string `json:"direction"`
	Scope      string `json:"scope"`
	Carrier    string `json:"carrier"`
	Original   string `json:"original"`
}

// manualManifestRevisionDocument 是手工版本推进口的线格式。
type manualManifestRevisionDocument struct {
	ManifestID string `json:"manifestId"`
	Version    string `json:"version"`
	Scope      string `json:"scope"`
	Carrier    string `json:"carrier"`
	Original   string `json:"original"`
}

type manualManifestIntake struct {
	intake *OperatorDecisionIntake
	now    func() time.Time
}

// ManualManifestIntake 交回手工录入两口的 Intake。接受时刻由服务端给。
func ManualManifestIntake(intake *OperatorDecisionIntake, now func() time.Time) ManifestReferenceIntake {
	return manualManifestIntake{intake: intake, now: now}
}

// entry 认证并核两格共有的内容：承运商与原件。原件同时充当来源事实引用。
func (manual manualManifestIntake) entry(ctx context.Context, request *http.Request, decode func() (carrier, original string, err error)) (OperatorIdentity, domain.CarrierResponsibilityReference, string, error) {
	none := domain.CarrierResponsibilityReference{}
	operator, err := manual.intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionExternalManifestEntry)
	if err != nil {
		return OperatorIdentity{}, none, "", err
	}
	rawCarrier, rawOriginal, err := decode()
	if err != nil {
		return OperatorIdentity{}, none, "", err
	}
	original := strings.TrimSpace(rawOriginal)
	if original == "" || len(original) > 500 || strings.IndexFunc(original, unicode.IsControl) >= 0 {
		return OperatorIdentity{}, none, "", fmt.Errorf("%w: original is required for a manually entered manifest reference", ErrMalformedRequest)
	}
	// 人手录的标识去首尾空白再用：「CX 」与「CX」是同一家承运商，不去的话会在承运商系统那条引用旁边另起一条。
	carrier, err := domain.NewCarrierResponsibilityReference(strings.TrimSpace(rawCarrier))
	if err != nil {
		return OperatorIdentity{}, none, "", fmt.Errorf("%w: carrier: %v", ErrMalformedRequest, err)
	}
	return operator, carrier, original, nil
}

func (manual manualManifestIntake) IntakeManifestReference(ctx context.Context, request *http.Request) (application.ReceiveManifestCommand, error) {
	none := application.ReceiveManifestCommand{}
	var document manualManifestReferenceDocument
	operator, carrier, original, err := manual.entry(ctx, request, func() (string, string, error) {
		err := decodeClosedPayload(request.Body, &document)
		return document.Carrier, document.Original, err
	})
	if err != nil {
		return none, err
	}
	command, err := manifestReferenceDocument{
		ManifestID: strings.TrimSpace(document.ManifestID), Version: strings.TrimSpace(document.Version), Procedure: strings.TrimSpace(document.Procedure),
		Direction: document.Direction, Scope: strings.TrimSpace(document.Scope), SourceFact: original,
	}.command(operator.Tenant, carrier, manual.now().UTC())
	if err != nil {
		return none, err
	}
	command.EnteredBy, command.Original = "OPERATOR/"+operator.Operator, original
	return command, nil
}

func (manual manualManifestIntake) IntakeManifestRevision(ctx context.Context, request *http.Request) (application.ReviseManifestCommand, error) {
	none := application.ReviseManifestCommand{}
	var document manualManifestRevisionDocument
	operator, carrier, original, err := manual.entry(ctx, request, func() (string, string, error) {
		err := decodeClosedPayload(request.Body, &document)
		return document.Carrier, document.Original, err
	})
	if err != nil {
		return none, err
	}
	command, err := manifestRevisionDocument{
		ManifestID: strings.TrimSpace(document.ManifestID), Version: strings.TrimSpace(document.Version), Scope: strings.TrimSpace(document.Scope), SourceFact: original,
	}.command(operator.Tenant, carrier, manual.now().UTC())
	if err != nil {
		return none, err
	}
	command.EnteredBy, command.Original = "OPERATOR/"+operator.Operator, original
	return command, nil
}
