package nodeopshttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件是节点收寄口的线格式与译装：租户与节点由调用方从认证结果交入，载荷只有设备签发的事实内容（ADR-0023）。

// receptionDocument 是收寄口的载荷：一次一件实物（批量交付由调用方逐件分发，见 ReceiveDeliveredUnitCommand）。键名与
// 接收观察词表同受控批量口（parcel-frontline-import 的收寄模板）逐字一致——两口消费同一用例，同一件事在两处不换词；
// 模板的行事实号在这里换成 sourceId，因为在线口收的是设备签发的事实身份，不是批次内的行号。
//
// 「不换词」只管键名与词表，不管值里的空白——两口在这一格各办各的（UC-NO-002「输入语义契约」）：本口把 mark 原样交进命令，
// 设备交来的观察不改写，扫描附带的回车、换行、Tab 与两端空白归一线客户端送出前去掉；收寄模板逐格去两端空白，是 CSV 模板的
// 形状纪律。在这里补去空白会改写设备事实与收寄内容摘要，带不带后缀的同一来源重投就被判成同一内容。
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

// receptionCommand 把收寄载荷译成命令。租户与节点由调用方从认证结果交入，不从载荷收。
func receptionCommand(tenant domain.TenantID, node domain.NodeReference, request *http.Request) (application.ReceiveDeliveredUnitCommand, error) {
	none := application.ReceiveDeliveredUnitCommand{}
	var document receptionDocument
	if err := decodeClosedDocument(request, &document); err != nil {
		return none, err
	}
	if strings.TrimSpace(document.SourceID) == "" {
		return none, fmt.Errorf("%w: sourceId is blank; the device-issued fact identity is not minted here (ADR-0023)", ErrMalformedRequest)
	}
	// 来源标识原样进收寄意图信封的 Subject（postgres 适配器 OutboxNodeIntakeHandoff）。越过框架的 Subject 上限或带控制字符，
	// 信封就被确定性拒收；本口不提交意图没交出去的收寄（parcel-api 的 transactionalReception），设备便会对着 5xx 永远重发。
	// 来源标识由设备签发（ADR-0023），重发同样的内容不会变好，所以在这里答坏报文，让设备出队交给人。
	if len(document.SourceID) > eventing.MaxSubjectLength || strings.IndexFunc(document.SourceID, unicode.IsControl) >= 0 {
		return none, fmt.Errorf("%w: sourceId exceeds %d bytes or carries a control character, which the intent envelope cannot carry",
			ErrMalformedRequest, eventing.MaxSubjectLength)
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
		TenantID:    tenant,
		SourceID:    document.SourceID,
		Node:        node,
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
// 尾随的第二个 JSON 值也拒：一份载荷只许一个文档，放过它就是无声丢掉一段输入。两道判据同 partycommercial 适配器
// （`internal/partycommercial/adapters/http`）的 decodeClosedDocument。
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
