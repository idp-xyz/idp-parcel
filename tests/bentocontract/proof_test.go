package bentocontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

// 本文件证 `PBC-09` 的产出面：证明 JSON 恰为九个登记字段、值全部锚定本包候选身份、
// 完成时刻是 RFC3339Nano、输出是单个 JSON 值无尾随内容，且立不住的注入项被拒绝。
//
// 断言方式是「检查自己产出了什么」（键集与值逐一核对），不是把框架的校验规则在
// Parcel 侧重建一遍——后者被简报明令不做。

var proofCompletedAt = time.Date(2026, 9, 7, 10, 0, 0, 123456789, time.UTC)

// proofConsumerCommit 是四十位小写十六进制的夹具提交号。真实产出时它是被证明的
// Parcel 基线提交，由协调运行注入。
const proofConsumerCommit = "0123456789abcdef0123456789abcdef01234567"

func TestProofCarriesExactlyTheNineRegisteredFields(t *testing.T) {
	proof, err := NewPassingConsumerProof(proofConsumerCommit, proofCompletedAt)
	if err != nil {
		t.Fatalf("产出证明：%v", err)
	}
	raw, err := proof.JSON()
	if err != nil {
		t.Fatalf("序列化证明：%v", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("解证明 JSON：%v", err)
	}
	expected := []string{
		"format_version", "consumer", "consumer_commit", "candidate_version",
		"module_path", "module_checksum", "contract_suite_version", "result", "completed_at",
	}
	if len(fields) != len(expected) {
		t.Fatalf("证明有 %d 个字段，rc.2 登记 %d 个：%s", len(fields), len(expected), raw)
	}
	for _, key := range expected {
		if _, present := fields[key]; !present {
			t.Errorf("证明缺字段 %q", key)
		}
	}
}

func TestProofValuesAreAnchoredToTheCandidateIdentity(t *testing.T) {
	proof, err := NewPassingConsumerProof(proofConsumerCommit, proofCompletedAt)
	if err != nil {
		t.Fatalf("产出证明：%v", err)
	}

	if proof.FormatVersion != 1 {
		t.Errorf("format_version = %d, want 1", proof.FormatVersion)
	}
	if proof.Consumer != "PARCEL" {
		t.Errorf("consumer = %q, want PARCEL", proof.Consumer)
	}
	if proof.ConsumerCommit != proofConsumerCommit {
		t.Errorf("consumer_commit = %q", proof.ConsumerCommit)
	}
	// 候选身份四件必须逐字来自本包常量：证明不可能声称一个本包没锁定的候选。
	if proof.CandidateVersion != Version {
		t.Errorf("candidate_version = %q, want %q", proof.CandidateVersion, Version)
	}
	if proof.ModulePath != ModulePath {
		t.Errorf("module_path = %q, want %q", proof.ModulePath, ModulePath)
	}
	if proof.ModuleChecksum != ModuleSum {
		t.Errorf("module_checksum = %q, want %q", proof.ModuleChecksum, ModuleSum)
	}
	if proof.ContractSuiteVersion != ContractSuiteVersion {
		t.Errorf("contract_suite_version = %q, want %q", proof.ContractSuiteVersion, ContractSuiteVersion)
	}
	if proof.Result != "PASS" {
		t.Errorf("result = %q, want PASS", proof.Result)
	}
	parsed, err := time.Parse(time.RFC3339Nano, proof.CompletedAt)
	if err != nil {
		t.Fatalf("completed_at 不是 RFC3339Nano：%v", err)
	}
	if !parsed.Equal(proofCompletedAt) {
		t.Errorf("completed_at = %q，与注入时刻不同", proof.CompletedAt)
	}
}

func TestProofOutputIsASingleJSONValue(t *testing.T) {
	proof, err := NewPassingConsumerProof(proofConsumerCommit, proofCompletedAt)
	if err != nil {
		t.Fatalf("产出证明：%v", err)
	}
	raw, err := proof.JSON()
	if err != nil {
		t.Fatalf("序列化证明：%v", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	var first any
	if err := decoder.Decode(&first); err != nil {
		t.Fatalf("解首个 JSON 值：%v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("输出带尾随内容：%v %v", trailing, err)
	}
}

func TestProofRefusesInvalidInjectedInputs(t *testing.T) {
	invalidCommits := map[string]string{
		"太短":    "abc123",
		"含大写":   "0123456789ABCDEF0123456789abcdef01234567",
		"非十六进制": "0123456789abcdef0123456789abcdef0123456g",
		"空":     "",
	}
	for name, commit := range invalidCommits {
		if _, err := NewPassingConsumerProof(commit, proofCompletedAt); err == nil {
			t.Errorf("%s的提交号 %q 被放行了", name, commit)
		}
	}
	if _, err := NewPassingConsumerProof(proofConsumerCommit, time.Time{}); err == nil {
		t.Error("零值完成时刻被放行了")
	}
}
