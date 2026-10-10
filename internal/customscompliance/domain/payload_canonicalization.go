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
// 已保存的旧摘要不在原处改写（ADR-0014）。哪些字段进摘要，沿用各口原先的内容
// 判据。门禁逐项指纹与凭证门禁指纹的词形被既有注释钉住，不在这一版里改写。
const payloadCanonicalizationVersion = "CCC-1"

func canonicalPayloadDigest(document []byte) string {
	sum := sha256.Sum256(document)
	return payloadCanonicalizationVersion + ":" + hex.EncodeToString(sum[:])
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
