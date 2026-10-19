package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// 收寄面来源文档的形状版本（ADR-0199 决定一；ADR-0014）。它与 NOC-1 是两份文档、各自换版：NOC-1 只收幂等与冲突分界的
// 那几格，来源文档另收证据、拒收原因、交付方与交付范围依据引用。把这几格并进指纹就得另起摘要形状并长期比对两版（ADR-0199
// 越权风险点；ADR-0212 决定七）。NRS-2 比 NRS-1 多交付范围依据引用两格；新落的一律写 NRS-2，NRS-1 照旧读得回、读作没有依据
// （决定七「既有行不回填」）。
const (
	deliverySourceShapeVersion      = "NRS-2"
	deliverySourceShapeWithoutBasis = "NRS-1"
)

// ErrInvalidDeliverySource 说明一份来源文档缺了每种观察都有的格（作业实物、接收观察、节点、发生时间），依据引用只给了半格或种类
// 认不出，或形状版本认不出。
var ErrInvalidDeliverySource = errors.New("node operations: invalid delivery source")

// DeliverySource 是收寄面一次交付的来源文档（ADR-0199 决定一）：与判断同一行写下，此后的重投、追加版本与解析失败都不改写它
// （NO CONTEXT「待识别实物与身份关联」）。三种观察共用这一个形状，某格在这种观察里本就缺席——拒收没有接收证据、接收没有
// 拒收原因——就是空串。
//
// Mark 原样存：在线口是设备送来的那一格，受控批量口是去两端空白之后交给编排的那一格（UC-NO-002 输入语义契约）。重核拿的就是
// 它，改写一个字符就问的是另一个号。Claim 是接收观察的名字，领域不拥有那个枚举，由编排照两个入口共用的词表给出。
//
// ScopeBasis 是送站时上报的交付范围依据引用（ADR-0212 决定七），零值即没带。它与交付方同类，只进来源文档、不进内容指纹：同一来源
// 只在这一格上不同的重投照旧答已有结果，首次存下的不被覆盖；扫错了的补救走归入交接单，不走重投。重核按它与此刻的证据状态重算
// 范围（决定六），所以它原样存、不判它指的东西在不在册。
//
// 零值即「来源未保全」：来源保全（ADR-0199 决定一）之前落下的行没有来源文档，号码从未留存，补不回。
type DeliverySource struct {
	Unit        HandlingUnitID
	Mark        string
	Claim       string
	Evidence    ReceptionEvidenceReference
	Refusal     string
	DeliveredBy DeliveringPartyReference
	Node        NodeReference
	OccurredAt  time.Time
	ScopeBasis  DeliveryScopeBasisReference
}

// Preserved 报告这份来源文档在不在。
func (source DeliverySource) Preserved() bool {
	return source != DeliverySource{}
}

// deliverySourceDocument 不带 omitempty：缺席格写空串、不省键，读的人分得清「这种观察没有这一格」与「这一版形状没有这一格」。
// 依据两格在 NRS-1 的文档里没有键，读回时就是空串，与没带依据同读。
type deliverySourceDocument struct {
	Shape          string `json:"shape"`
	Unit           string `json:"unit"`
	Mark           string `json:"mark"`
	Claim          string `json:"claim"`
	Evidence       string `json:"evidence"`
	Refusal        string `json:"refusal"`
	DeliveredBy    string `json:"delivered_by"`
	Node           string `json:"node"`
	OccurredAt     string `json:"occurred_at"`
	ScopeBasisKind string `json:"scope_basis_kind"`
	ScopeBasisID   string `json:"scope_basis_id"`
}

// CanonicalizeDeliverySource 把来源文档写成 NRS-2 形状，时刻按 UTC。
func CanonicalizeDeliverySource(source DeliverySource) ([]byte, error) {
	if !source.Unit.valid() || !source.Node.valid() || strings.TrimSpace(source.Claim) == "" || source.OccurredAt.IsZero() {
		return nil, ErrInvalidDeliverySource
	}
	if source.ScopeBasis != (DeliveryScopeBasisReference{}) && !source.ScopeBasis.Present() {
		return nil, fmt.Errorf("%w: half a delivery scope basis", ErrInvalidDeliverySource)
	}
	return json.Marshal(deliverySourceDocument{
		Shape:          deliverySourceShapeVersion,
		Unit:           source.Unit.String(),
		Mark:           source.Mark,
		Claim:          source.Claim,
		Evidence:       source.Evidence.String(),
		Refusal:        source.Refusal,
		DeliveredBy:    source.DeliveredBy.String(),
		Node:           source.Node.String(),
		OccurredAt:     source.OccurredAt.UTC().Format(time.RFC3339Nano),
		ScopeBasisKind: source.ScopeBasis.Kind().String(),
		ScopeBasisID:   source.ScopeBasis.Identifier(),
	})
}

// ParseDeliverySource 读回一份来源文档。形状版本认不出就拒，不按已知的版本硬读：将来加格的一版被旧口读成已知版本，会静默丢掉
// 新格（ADR-0014「只在同一形状内可比」同一条理由）。NRS-1 读作没有依据；NRS-2 的依据两格要么同空、要么成对且种类在词表里——
// 半格读成「没有依据」，会把一次带了依据的送站说成没带。
func ParseDeliverySource(document []byte) (DeliverySource, error) {
	var stored deliverySourceDocument
	if err := json.Unmarshal(document, &stored); err != nil {
		return DeliverySource{}, fmt.Errorf("%w: %v", ErrInvalidDeliverySource, err)
	}
	switch stored.Shape {
	case deliverySourceShapeVersion:
	case deliverySourceShapeWithoutBasis:
		stored.ScopeBasisKind, stored.ScopeBasisID = "", ""
	default:
		return DeliverySource{}, fmt.Errorf("%w: shape %q", ErrInvalidDeliverySource, stored.Shape)
	}
	unit, err := NewHandlingUnitID(stored.Unit)
	if err != nil {
		return DeliverySource{}, fmt.Errorf("%w: %v", ErrInvalidDeliverySource, err)
	}
	node, err := NewNodeReference(stored.Node)
	if err != nil {
		return DeliverySource{}, fmt.Errorf("%w: %v", ErrInvalidDeliverySource, err)
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, stored.OccurredAt)
	if err != nil {
		return DeliverySource{}, fmt.Errorf("%w: %v", ErrInvalidDeliverySource, err)
	}
	source := DeliverySource{
		Unit:       unit,
		Mark:       stored.Mark,
		Claim:      stored.Claim,
		Refusal:    stored.Refusal,
		Node:       node,
		OccurredAt: occurredAt.UTC(),
	}
	// 证据与交付方缺席时是零值：写下时就是零值，原样读回。
	if stored.Evidence != "" {
		if source.Evidence, err = NewReceptionEvidenceReference(stored.Evidence); err != nil {
			return DeliverySource{}, fmt.Errorf("%w: %v", ErrInvalidDeliverySource, err)
		}
	}
	if stored.DeliveredBy != "" {
		if source.DeliveredBy, err = NewDeliveringPartyReference(stored.DeliveredBy); err != nil {
			return DeliverySource{}, fmt.Errorf("%w: %v", ErrInvalidDeliverySource, err)
		}
	}
	if stored.ScopeBasisKind != "" || stored.ScopeBasisID != "" {
		kind, err := ParseDeliveryScopeBasisKind(stored.ScopeBasisKind)
		if err != nil {
			return DeliverySource{}, fmt.Errorf("%w: %v", ErrInvalidDeliverySource, err)
		}
		if source.ScopeBasis, err = NewDeliveryScopeBasisReference(kind, stored.ScopeBasisID); err != nil {
			return DeliverySource{}, fmt.Errorf("%w: %v", ErrInvalidDeliverySource, err)
		}
	}
	if strings.TrimSpace(source.Claim) == "" {
		return DeliverySource{}, fmt.Errorf("%w: claim is blank", ErrInvalidDeliverySource)
	}
	return source, nil
}
