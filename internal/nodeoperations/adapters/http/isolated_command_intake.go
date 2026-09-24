package nodeopshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// IsolatedCommandIntake 是隔离写路径准入（ADR-0091）在节点作业命令面的注入式放行（票 operator-channel/08）。
//
// 它与本包的 IsolatedOperationsReadIntake 同层同款：租户与节点两格在构造时由装配点给定，进请求路径后不读任何授权
// 输入——ReceptionIntake 的契约点名这两格「只能来自认证结果」，隔离形态的认证结果就是开关值与装配点的合成常量。与读面
// 不同的是它会构造命令、命令会落行：ADR-0078 的「零持久化」论证在这里不成立，可分辨物由租户维的 `SYN-` 前缀承担
// （ADR-0091 决定三）；前缀门禁、启用与否与启动日志都在 cmd/parcel-api，本类型只收立得住的值。
//
// 事实身份与发生时间照 ADR-0023 从载荷如实收：sourceId 是设备签发的幂等身份、occurredAt 是设备记录的业务时间，本类型
// 不拿服务端时钟或随机数顶替任何一样——代铸了，离线补传的重放就会被判成新事实。
//
// 逐口放行（ADR-0091 Consequences）：本类型只实现已成笔的口的 Intake 接口，未成笔的口在装配点仍挂字面量
// UnconfiguredIntake{}，且在类型上就装不进本类型——每放一口在这里多一个方法，装配点多换一行，两处都看得见。
type IsolatedCommandIntake struct {
	tenant domain.TenantID
	node   domain.NodeReference
}

// 已成笔的口。每放一口在这里多一行断言、多一个方法，装配点多换一行。
var _ ReceptionIntake = (*IsolatedCommandIntake)(nil)

// NewIsolatedCommandIntake 由装配点以显式合成值构造。立不起来的租户与节点在这里拒：装配错误要在启动时暴露，不该等到
// 第一个请求。
func NewIsolatedCommandIntake(tenant, node string) (*IsolatedCommandIntake, error) {
	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		return nil, fmt.Errorf("node operations http: isolated command intake: %w", err)
	}
	nodeReference, err := domain.NewNodeReference(node)
	if err != nil {
		return nil, fmt.Errorf("node operations http: isolated command intake: %w", err)
	}
	return &IsolatedCommandIntake{tenant: tenantID, node: nodeReference}, nil
}

// receptionDocument 是收寄口的载荷：一次一件实物（批量交付由调用方逐件分发，见 ReceiveDeliveredUnitCommand）。键名与
// 接收观察词表同受控批量口（parcel-frontline-import 的收寄模板）逐字一致——两口消费同一用例，同一件事在两处不换词；
// 模板的行事实号在这里换成 sourceId，因为在线口收的是设备签发的事实身份，不是批次内的行号。
type receptionDocument struct {
	SourceID      string `json:"sourceId"`
	DeliveredBy   string `json:"deliveredBy"`
	HandlingUnit  string `json:"handlingUnit"`
	Mark          string `json:"mark"`
	Claim         string `json:"claim"`
	EvidenceRef   string `json:"evidenceRef"`
	RefusalReason string `json:"refusalReason"`
	OccurredAt    string `json:"occurredAt"`
}

// IntakeReception 把一次隔离形态的收寄请求译成收寄命令。形状级失败包 ErrMalformedRequest（4xx，重发同样内容不会好）。
//
// 服务结果标记（ServiceMarkers）不从载荷收：命令注释写明它「由接入层查好带入」，隔离形态没有那道查询，收了就是采信
// 调用方自报的 PS 结论。留空的后果照实说：已取消或已终局的包裹在隔离环境里收寄时不带标记。
func (intake *IsolatedCommandIntake) IntakeReception(
	_ context.Context,
	request *http.Request,
) (application.ReceiveDeliveredUnitCommand, error) {
	none := application.ReceiveDeliveredUnitCommand{}
	var document receptionDocument
	if err := decodeClosedDocument(request, &document); err != nil {
		return none, err
	}
	if strings.TrimSpace(document.SourceID) == "" {
		return none, fmt.Errorf("%w: sourceId is blank; the device-issued fact identity is not minted here (ADR-0023)", ErrMalformedRequest)
	}
	deliveredBy, err := domain.NewDeliveringPartyReference(document.DeliveredBy)
	if err != nil {
		return none, fmt.Errorf("%w: deliveredBy: %v", ErrMalformedRequest, err)
	}
	unit, err := domain.NewHandlingUnitID(document.HandlingUnit)
	if err != nil {
		return none, fmt.Errorf("%w: handlingUnit: %v", ErrMalformedRequest, err)
	}
	occurredAt, err := time.Parse(time.RFC3339, document.OccurredAt)
	if err != nil {
		return none, fmt.Errorf("%w: occurredAt is not an RFC 3339 instant: %v", ErrMalformedRequest, err)
	}
	command := application.ReceiveDeliveredUnitCommand{
		TenantID:    intake.tenant,
		SourceID:    document.SourceID,
		Node:        intake.node,
		DeliveredBy: deliveredBy,
		Unit:        unit,
		Mark:        ports.ExternalMarkObservation{Mark: document.Mark},
		OccurredAt:  occurredAt,
	}
	if err := applyReceptionClaim(&command, document); err != nil {
		return none, err
	}
	return command, nil
}

// applyReceptionClaim 按接收观察的各态核搭配并译进命令。搭配门与受控批量口的收寄模板同一组：明确接收必带标识与接收证据、
// 不带拒收原因；拒收必带原因、不带证据；仅扫描必带标识、两样都不带。给了不该给的是拒不是忽略——既有记录形状没有地方
// 放它，静默丢掉会让操作员以为登进去了。
func applyReceptionClaim(command *application.ReceiveDeliveredUnitCommand, document receptionDocument) error {
	switch document.Claim {
	case "RECEIVED":
		if document.Mark == "" || document.RefusalReason != "" {
			return fmt.Errorf("%w: RECEIVED carries a mark and no refusalReason", ErrMalformedRequest)
		}
		evidence, err := domain.NewReceptionEvidenceReference(document.EvidenceRef)
		if err != nil {
			return fmt.Errorf("%w: RECEIVED needs evidenceRef: %v", ErrMalformedRequest, err)
		}
		command.Claim = application.ExplicitReception
		command.Evidence = evidence
	case "REFUSED":
		if strings.TrimSpace(document.RefusalReason) == "" || document.EvidenceRef != "" {
			return fmt.Errorf("%w: REFUSED carries a refusalReason and no evidenceRef", ErrMalformedRequest)
		}
		command.Claim = application.ExplicitRefusal
		command.Refusal = document.RefusalReason
	case "SCAN_ONLY":
		if document.Mark == "" || document.EvidenceRef != "" || document.RefusalReason != "" {
			return fmt.Errorf("%w: SCAN_ONLY carries a mark and neither evidenceRef nor refusalReason", ErrMalformedRequest)
		}
		command.Claim = application.ScanOnlyObservation
	default:
		return fmt.Errorf("%w: claim=%q is not one of RECEIVED / REFUSED / SCAN_ONLY", ErrMalformedRequest, document.Claim)
	}
	return nil
}

// decodeClosedDocument 以封闭形状解载荷。未知键拒：载荷里出现 tenantId 之类的键不是可以忽略的噪声，是采信自报的入口；
// 尾随的第二个 JSON 值也拒：一份载荷只许一个文档，放过它就是无声丢掉一段输入。两道判据同 partycommercial 的隔离身份
// Intake。
func decodeClosedDocument(request *http.Request, target any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing content after the payload", ErrMalformedRequest)
	}
	return nil
}
