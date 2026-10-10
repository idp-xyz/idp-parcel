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

// payloadCanonicalizationVersion 是关务合规命令口的现行形状。摘要带版本前缀，
// 已保存的旧摘要不在原处改写（ADR-0014），比对时按它自己的形状重算本次命令，见
// CompareStoredDigest。哪些字段进摘要，沿用各口原先的内容判据。门禁逐项指纹与
// 凭证门禁指纹的词形被既有注释钉住，不在这一版里改写。
const payloadCanonicalizationVersion = "CCC-1"

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

// CompareStoredDigest 按已存摘要选形状：带 CCC-1 前缀的与 current（本次命令的 CCC-1 摘要）比；
// 不带前缀的是 CCC-1 之前写下的无版本摘要，与 unversioned（本次命令按无版本那一版算出的摘要）比；
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

func marshalPayload(document any) ([]byte, string, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, "", fmt.Errorf("customs compliance: canonicalize command payload: %w", err)
	}
	return encoded, canonicalPayloadDigest(encoded), nil
}

func canonicalInstant(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}

func sortedCopy(values []string) []string {
	copied := append([]string(nil), values...)
	slices.Sort(copied)
	if copied == nil {
		return []string{}
	}
	return copied
}

// ExternalReleaseContent 是放行层响应的放行三件。非放行层不带它。
type ExternalReleaseContent struct {
	Kind      ReleaseKind
	Authority string
	Condition string
}

// CanonicalizeExternalResultPayload 定形「接收外部监管结果」。来源身份与角色不进摘要。
func CanonicalizeExternalResultPayload(layer ResultLayer, rawSemantics, claimedVersion string, attempt int, scope string, occurredAt time.Time, release *ExternalReleaseContent) ([]byte, string, error) {
	type releaseDocument struct {
		Kind      string `json:"kind"`
		Authority string `json:"authority"`
		Condition string `json:"condition"`
	}
	var encoded *releaseDocument
	if release != nil {
		encoded = &releaseDocument{
			Kind:      release.Kind.String(),
			Authority: release.Authority,
			Condition: release.Condition,
		}
	}
	return marshalPayload(struct {
		Canonicalization string           `json:"canonicalization"`
		Face             string           `json:"face"`
		Layer            string           `json:"layer"`
		RawSemantics     string           `json:"raw_semantics"`
		ClaimedVersion   string           `json:"claimed_version"`
		Attempt          int              `json:"attempt"`
		Scope            string           `json:"scope"`
		OccurredAt       string           `json:"occurred_at"`
		Release          *releaseDocument `json:"release"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "RECEIVE_EXTERNAL_RESULT",
		Layer:            layer.String(),
		RawSemantics:     rawSemantics,
		ClaimedVersion:   claimedVersion,
		Attempt:          attempt,
		Scope:            scope,
		OccurredAt:       canonicalInstant(occurredAt),
		Release:          encoded,
	})
}

// CanonicalizeDeclarationPayload 定形申报提交与更正共用的内容判据：程序、资料、角色
// 与成员。成员排序，使提交顺序不构成另一份内容。
func CanonicalizeDeclarationPayload(procedure, dossier, roles string, members []string) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string   `json:"canonicalization"`
		Face             string   `json:"face"`
		Procedure        string   `json:"procedure"`
		Dossier          string   `json:"dossier"`
		Roles            string   `json:"roles"`
		Members          []string `json:"members"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "SUBMIT_DECLARATION",
		Procedure:        procedure,
		Dossier:          dossier,
		Roles:            roles,
		Members:          sortedCopy(members),
	})
}

// CanonicalizeDutyVerificationPayload 定形「核对税费付款」。三维身份不进摘要；依据、
// 程序与资金版本去空白后进入。
func CanonicalizeDutyVerificationPayload(coverage DutyCoverage, delta DutyDelta, validity DutyFactValidity, basis, procedure, fundsVersion string) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string `json:"canonicalization"`
		Face             string `json:"face"`
		Coverage         string `json:"coverage"`
		Delta            string `json:"delta"`
		Validity         string `json:"validity"`
		Basis            string `json:"basis"`
		Procedure        string `json:"procedure"`
		FundsVersion     string `json:"funds_version"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "VERIFY_DUTY_PAYMENT",
		Coverage:         coverage.String(),
		Delta:            delta.String(),
		Validity:         validity.String(),
		Basis:            strings.TrimSpace(basis),
		Procedure:        strings.TrimSpace(procedure),
		FundsVersion:     strings.TrimSpace(fundsVersion),
	})
}

// CanonicalizeDispositionFactSetPayload 定形处置执行核对所读到的事实集。事实引用排序，
// 使装载顺序不构成另一份内容。
func CanonicalizeDispositionFactSetPayload(facts []string) ([]byte, string, error) {
	return marshalPayload(struct {
		Canonicalization string   `json:"canonicalization"`
		Face             string   `json:"face"`
		Facts            []string `json:"facts"`
	}{
		Canonicalization: payloadCanonicalizationVersion,
		Face:             "VERIFY_DISPOSITION",
		Facts:            sortedCopy(facts),
	})
}
