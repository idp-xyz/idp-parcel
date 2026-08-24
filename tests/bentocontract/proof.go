package bentocontract

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// 本文件是 `PBC-09` 的产出器：按所锁候选 `v0.1.0-rc.2` 的严格格式产出消费者证明 JSON。
//
// 格式权威在框架侧（候选内的 consumerproof 读取开 DisallowUnknownFields、拒绝尾随值与
// 缺失必填字段），校验由 Bento 仓的协调作业执行；Parcel 只产出，不镜像一份校验规则——
// 在这边重建校验等于给同一口径立第二处定义（简报「PBC-09 形状约束」）。因此本文件只
// 校验**注入项**（消费者提交与完成时刻），候选身份字段一律取本包常量，不接受调用方
// 另给一份。
//
// rc.2 的证明恰九个字段。治理模式（GovernanceMode）按 candidate.go 的说明进不了这份
// JSON：候选只认九个字段并拒收未知字段，带 `acknowledged_governance_mode` 的候选出现
// 时本文件与该说明同时复评。

// ConsumerProof 是 Parcel 提交给框架协调作业的合同证明文档。字段名与候选内
// consumerproof.Proof 的 JSON 标签逐字一致。
type ConsumerProof struct {
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

// ProofFormatVersion 是 rc.2 证明格式的版本号。
const ProofFormatVersion = 1

// ProofConsumer 是本仓在框架治理里的消费者名。
const ProofConsumer = "PARCEL"

// fullGitObjectID 校验注入的消费者提交是完整小写 Git 对象 ID（SHA-1 四十位或
// SHA-256 六十四位）。这是对自己输入的把关，不是替框架校验证明。
var fullGitObjectID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// NewPassingConsumerProof 为一次全部通过的合同运行产出证明。
//
// result 恒为 PASS 且不作参数：证明只该在九项 PBC 对同一候选全部通过后产出，一份
// FAIL 证明没有提交对象——失败的运行修到过为止，不登记。产出本身不宣称闸门状态；
// 把它交给协调作业是 `B-06` 登记那一步的事，仍在完成门禁之后。
func NewPassingConsumerProof(consumerCommit string, completedAt time.Time) (ConsumerProof, error) {
	if !fullGitObjectID.MatchString(consumerCommit) {
		return ConsumerProof{}, fmt.Errorf(
			"bentocontract: 消费者提交 %q 不是完整小写 Git 对象 ID", consumerCommit)
	}
	if completedAt.IsZero() {
		return ConsumerProof{}, fmt.Errorf("bentocontract: 完成时刻不得为零值")
	}
	return ConsumerProof{
		FormatVersion:        ProofFormatVersion,
		Consumer:             ProofConsumer,
		ConsumerCommit:       consumerCommit,
		CandidateVersion:     Version,
		ModulePath:           ModulePath,
		ModuleChecksum:       ModuleSum,
		ContractSuiteVersion: ContractSuiteVersion,
		Result:               "PASS",
		CompletedAt:          completedAt.UTC().Format(time.RFC3339Nano),
	}, nil
}

// JSON 产出证明文件的字节形态：单个 JSON 值，无尾随内容。
func (proof ConsumerProof) JSON() ([]byte, error) {
	return json.Marshal(proof)
}
