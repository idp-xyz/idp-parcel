package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// 节点作业命令载荷的规范化形状（ADR-0149 决定四；ADR-0014）。版本进文档也进摘要前缀：
// 已保存摘要必须带着产生它的形状。NOC-1 之前入库的摘要不带前缀，比对时按那一版重算本次
// 命令再比，见 CompareStoredDigest。集合先排序再编码，提交顺序不是内容。不在 NOC-1 上就地扩列。

const payloadCanonicalizationVersion = "NOC-1"

func canonicalPayloadDigest(document []byte) string {
	sum := sha256.Sum256(document)
	return payloadCanonicalizationVersion + ":" + hex.EncodeToString(sum[:])
}

// StoredDigestComparison 是已存摘要与本次命令的比对结论。摘要只在同一形状版本内可比
// （ADR-0014），所以先按已存摘要的形状重算本次命令，再比。
type StoredDigestComparison int

const (
	// SamePayload：同一形状下同串，是重放。
	SamePayload StoredDigestComparison = iota + 1
	// DifferentPayload：同一形状下异串，是内容冲突。
	DifferentPayload
	// UnknownPayloadShape：已存摘要的版本前缀本口认不出，无从按它重算——不是冲突，也证明不了是重放。
	// 调用方按该处读失败的既有答复作答：读不懂这份记录与读不出来，同样是此刻判断不了。
	UnknownPayloadShape
)

// CompareStoredDigest 按已存摘要选形状：带 NOC-1 前缀的与 current（本次命令的 NOC-1 摘要）比；
// 不带前缀的是 NOC-1 之前写下的无版本摘要，与 unversioned（本次命令按无版本那一版算出的摘要）比；
// 其余前缀答 UnknownPayloadShape。存量不回写，所以无版本那一版要一直能算。
func CompareStoredDigest(stored, current, unversioned string) StoredDigestComparison {
	shape, _, versioned := strings.Cut(stored, ":")
	switch {
	case !versioned:
		return payloadComparison(stored == unversioned)
	case shape == payloadCanonicalizationVersion:
		return payloadComparison(stored == current)
	default:
		return UnknownPayloadShape
	}
}

func payloadComparison(same bool) StoredDigestComparison {
	if same {
		return SamePayload
	}
	return DifferentPayload
}

type acceptancePayloadDocument struct {
	Canonicalization string   `json:"canonicalization"`
	Face             string   `json:"face"`
	Decision         string   `json:"decision"`
	Node             string   `json:"node"`
	Authority        string   `json:"authority"`
	Basis            string   `json:"basis"`
	AcceptedUnits    []string `json:"accepted_units"`
	AcceptedActions  []string `json:"accepted_actions"`
}

// CanonicalizeAcceptancePayload 定承接决定口的 NOC-1 形状，交回文档与摘要。
func CanonicalizeAcceptancePayload(
	decision AcceptanceDecisionKind,
	node string,
	authority string,
	basis string,
	acceptedUnits []string,
	acceptedActions []CollaborationActionKind,
) ([]byte, string, error) {
	units := slices.Clone(acceptedUnits)
	slices.Sort(units)
	actions := make([]string, len(acceptedActions))
	for index, action := range acceptedActions {
		actions[index] = action.String()
	}
	slices.Sort(actions)
	encoded, err := json.Marshal(acceptancePayloadDocument{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "ACCEPT_COLLABORATION",
		Decision:         decision.String(),
		Node:             node,
		Authority:        authority,
		Basis:            basis,
		AcceptedUnits:    units,
		AcceptedActions:  actions,
	})
	if err != nil {
		return nil, "", err
	}
	return encoded, canonicalPayloadDigest(encoded), nil
}

type executionPayloadDocument struct {
	Canonicalization string `json:"canonicalization"`
	Face             string `json:"face"`
	Evidence         string `json:"evidence"`
	PerformedAt      string `json:"performed_at"`
}

// CanonicalizeExecutionPayload 定执行事实登记口的 NOC-1 形状。事项、实物与动作是幂等键，不进摘要。
func CanonicalizeExecutionPayload(evidence string, performedAt time.Time) ([]byte, string, error) {
	encoded, err := json.Marshal(executionPayloadDocument{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "RECORD_EXECUTION",
		Evidence:         evidence,
		PerformedAt:      performedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, "", err
	}
	return encoded, canonicalPayloadDigest(encoded), nil
}

type consolidationPayloadHead struct {
	Canonicalization string `json:"canonicalization"`
	Face             string `json:"face"`
	Action           string `json:"action"`
	PerformedBy      string `json:"performed_by"`
	Evidence         string `json:"evidence"`
	OccurredAt       string `json:"occurred_at"`
	Unit             string `json:"unit"`
}

func consolidationHead(face string, action ConsolidationActionKind, source WorkFactSource, unit ConsolidationUnitID) consolidationPayloadHead {
	return consolidationPayloadHead{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             face,
		Action:           action.String(),
		PerformedBy:      source.PerformedBy().String(),
		Evidence:         source.Evidence().String(),
		OccurredAt:       source.OccurredAt().UTC().Format(time.RFC3339Nano),
		Unit:             unit.String(),
	}
}

func marshalPayload(document any) ([]byte, string, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, "", err
	}
	return encoded, canonicalPayloadDigest(encoded), nil
}

// CanonicalizeOpenUnitPayload 定开集包单元口的 NOC-1 形状。来源身份是幂等键，不进摘要。
func CanonicalizeOpenUnitPayload(source WorkFactSource, unit ConsolidationUnitID, asset CarrierAssetReference) ([]byte, string, error) {
	return marshalPayload(struct {
		consolidationPayloadHead
		Asset string `json:"asset"`
	}{consolidationHead("OPEN_UNIT", OpenUnitAction, source, unit), asset.String()})
}

// CanonicalizeAddMemberPayload 定加入承运资产口的 NOC-1 形状。
func CanonicalizeAddMemberPayload(source WorkFactSource, unit ConsolidationUnitID, member HandlingUnitID) ([]byte, string, error) {
	return marshalPayload(struct {
		consolidationPayloadHead
		Member string `json:"member"`
	}{consolidationHead("ADD_MEMBER", AddMemberAction, source, unit), member.String()})
}

// CanonicalizeRemoveMemberPayload 定移出承运资产口的 NOC-1 形状。
func CanonicalizeRemoveMemberPayload(source WorkFactSource, unit ConsolidationUnitID, member HandlingUnitID) ([]byte, string, error) {
	return marshalPayload(struct {
		consolidationPayloadHead
		Member string `json:"member"`
	}{consolidationHead("REMOVE_MEMBER", RemoveMemberAction, source, unit), member.String()})
}

// CanonicalizeSealUnitPayload 定封存集包单元口的 NOC-1 形状。依据在场与缺席必须是不同摘要。
func CanonicalizeSealUnitPayload(source WorkFactSource, unit ConsolidationUnitID, seal SealReference, basis WorkBasisReference) ([]byte, string, error) {
	return marshalPayload(struct {
		consolidationPayloadHead
		Seal  string `json:"seal"`
		Basis string `json:"basis"`
	}{consolidationHead("SEAL_UNIT", SealUnitAction, source, unit), seal.String(), basis.String()})
}

// CanonicalizeUnsealUnitPayload 定解封集包单元口的 NOC-1 形状。
func CanonicalizeUnsealUnitPayload(source WorkFactSource, unit ConsolidationUnitID, basis WorkBasisReference) ([]byte, string, error) {
	return marshalPayload(struct {
		consolidationPayloadHead
		Basis string `json:"basis"`
	}{consolidationHead("UNSEAL_UNIT", UnsealUnitAction, source, unit), basis.String()})
}

// CanonicalizeCloseUnitPayload 定关集包单元口的 NOC-1 形状。进摘要的是处置依据，不是处置种类。
func CanonicalizeCloseUnitPayload(source WorkFactSource, unit ConsolidationUnitID, disposition WorkBasisReference) ([]byte, string, error) {
	return marshalPayload(struct {
		consolidationPayloadHead
		Disposition string `json:"disposition"`
	}{consolidationHead("CLOSE_UNIT", CloseUnitAction, source, unit), disposition.String()})
}

type deliveryPayloadDocument struct {
	Canonicalization string `json:"canonicalization"`
	Face             string `json:"face"`
	Unit             string `json:"unit"`
	Mark             string `json:"mark"`
	Claim            string `json:"claim"`
	Node             string `json:"node"`
	OccurredAt       string `json:"occurred_at"`
}

// CanonicalizeDeliveryPayload 定交付实物事实口的 NOC-1 形状。来源身份是幂等键，不进摘要。
// claim 是接收观察的封闭码，领域不拥有那个枚举，按命令已经在比的那一格原样记。
func CanonicalizeDeliveryPayload(unit HandlingUnitID, mark string, claim uint8, node NodeReference, occurredAt time.Time) ([]byte, string, error) {
	return marshalPayload(deliveryPayloadDocument{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "RECEIVE_DELIVERED_UNIT",
		Unit:             unit.String(),
		Mark:             mark,
		Claim:            fmt.Sprintf("%d", claim),
		Node:             node.String(),
		OccurredAt:       occurredAt.UTC().Format(time.RFC3339Nano),
	})
}
