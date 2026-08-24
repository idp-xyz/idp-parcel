package bentocontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

// 本文件取 `PBC-09`：产出的证明 JSON 恰好九字段、无未知字段、无尾随 JSON 值、
// `completed_at` 可按 RFC3339Nano 解析。断言逐条对应协调作业读取侧会拒收的形状，
// 但只断言产出物本身，不重实现校验器——校验口径归框架，Parcel 不立第二处。

// producedProofShape 镜像九字段格子，只在断言里用（与 PBC-05 载荷断言同一手法）：
// DisallowUnknownFields 拿它证「无多」，键数证「无缺」。
type producedProofShape struct {
	FormatVersion        int    `json:"format_version"`
	Consumer             string `json:"consumer"`
	ConsumerCommit       string `json:"consumer_commit"`
	CandidateVersion     string `json:"candidate_version"`
	ModulePath           string `json:"module_path"`
	ModuleChecksum       string `json:"module_checksum"`
	ContractSuiteVersion string `json:"contract_suite_version"`
	Result               string `json:"result"`
	CompletedAt          string `json:"completed_at"`
}

// proofCommitFixture 是命名夹具：40 位十六进制的提交号形状，不指向任何真实提交。
// 真实取证时的提交号由 B-06 协调那一步传入，不在本票内。
const proofCommitFixture = "0123456789abcdef0123456789abcdef01234567"

func TestProducedProofMatchesTheLockedCandidateStrictFormat(t *testing.T) {
	completedAt := time.Date(2026, 8, 21, 15, 4, 5, 123456789, time.UTC)

	raw, err := ProduceConsumerProof(proofCommitFixture, completedAt)
	if err != nil {
		t.Fatalf("产出证明:%v", err)
	}

	// —— 无未知字段、无尾随 JSON 值:与 rc.2 读取侧的拒收形状逐条对应。——
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var proof producedProofShape
	if err := decoder.Decode(&proof); err != nil {
		t.Fatalf("证明里有九字段之外的内容:%v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("证明后面跟着多余的 JSON 值:err=%v", err)
	}

	// —— 键数恰好九个:少一个是缺必填,多一个在上面已拒,这里防的是「缺」。——
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatalf("证明不是 JSON 对象:%v", err)
	}
	if len(keys) != 9 {
		t.Fatalf("证明有 %d 个键, want 恰好 9 个", len(keys))
	}

	// —— 逐格核对:七格钉在所锁候选上,两格来自入参。——
	if proof.FormatVersion != 1 {
		t.Fatalf("format_version = %d, want 1", proof.FormatVersion)
	}
	if proof.Consumer != "PARCEL" {
		t.Fatalf("consumer = %q, want PARCEL", proof.Consumer)
	}
	if proof.ConsumerCommit != proofCommitFixture {
		t.Fatalf("consumer_commit = %q, want 入参提交号", proof.ConsumerCommit)
	}
	if proof.CandidateVersion != Version {
		t.Fatalf("candidate_version = %q, want %q", proof.CandidateVersion, Version)
	}
	if proof.ModulePath != ModulePath {
		t.Fatalf("module_path = %q, want %q", proof.ModulePath, ModulePath)
	}
	if proof.ModuleChecksum != ModuleSum {
		t.Fatalf("module_checksum = %q, want 所锁候选 checksum", proof.ModuleChecksum)
	}
	if proof.ContractSuiteVersion != ContractSuiteVersion {
		t.Fatalf("contract_suite_version = %q, want %q", proof.ContractSuiteVersion, ContractSuiteVersion)
	}
	if proof.Result != "PASS" {
		t.Fatalf("result = %q, want PASS", proof.Result)
	}
	parsed, err := time.Parse(time.RFC3339Nano, proof.CompletedAt)
	if err != nil {
		t.Fatalf("completed_at 不是 RFC3339Nano:%v", err)
	}
	if !parsed.Equal(completedAt) {
		t.Fatalf("completed_at = %v, want %v", parsed, completedAt)
	}
}

func TestProofRefusesMissingInputs(t *testing.T) {
	if _, err := ProduceConsumerProof("", time.Now()); err == nil {
		t.Fatal("缺提交号的产出必须响亮报错")
	}
	if _, err := ProduceConsumerProof(proofCommitFixture, time.Time{}); err == nil {
		t.Fatal("缺完成时刻的产出必须响亮报错")
	}
}
