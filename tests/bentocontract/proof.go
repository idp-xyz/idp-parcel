package bentocontract

import (
	"encoding/json"
	"errors"
	"time"
)

// 本文件承载 `PBC-09` 的产出半边：按所锁候选 `v0.1.0-rc.2` 的严格格式产出证明 JSON。
// 校验归 Bento 仓的协调作业——框架的 `consumerproof` 与 `proofcheck` 都在其 `internal/`
// 下，Parcel 导入不了（简报「实现准入检查」已实测）；在 Parcel 侧镜像一份校验规则等于给
// 同一口径立第二处定义，不做。
//
// 放本包而不放生产包：证明只为框架合同取证与 B-06 协调而存在，与候选身份常量同源同命；
// 将来若立「专用合同命令」，按简报既定安排走其精确包路径的豁免，届时再谈导入。

// rc.2 证明格式的三个字面值。它们与 candidate.go 的候选身份常量同族——是所锁候选的
// 格式事实（抄录自候选源码 `internal/consumerproof` 的格式常量），不是 Parcel 自立的
// 校验口径。候选换版时随 candidate.go 一并复评；Bento 一旦切出带
// `acknowledged_governance_mode` 的候选，本文件与简报「发布与治理边界」的治理状态条
// 同时复评。
const (
	proofFormatVersion = 1
	proofConsumer      = "PARCEL"
	proofResultPass    = "PASS"
)

// consumerProofDocument 是 rc.2 证明 JSON 的九字段形状，一个不多一个不少：rc.2 的读取
// 开着 DisallowUnknownFields，多写一个「未来字段」当场被拒；少一个必填字段协调作业同样
// 拒收。字段声明顺序即产出顺序。
type consumerProofDocument struct {
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

// ProduceConsumerProof 产出一份 PASS 证明。入参只有每次取证真正会变的两件——被证明的
// Parcel 提交与完成时刻；其余七格全部由所锁候选与本包常量钉死。
//
// 只产出 PASS：证明在合同套件通过之后才产出，失败的跑动不产出证明而是修到通过——把
// result 参数化只会多一条「没跑就写 PASS」的路。入参缺席是装配缺陷，响亮报错；提交号
// 的形状（40/64 位十六进制对象号）与时间戳的合法性校验归协调作业，这里不复制。
func ProduceConsumerProof(consumerCommit string, completedAt time.Time) ([]byte, error) {
	if consumerCommit == "" {
		return nil, errors.New("produce consumer proof: consumer commit is required")
	}
	if completedAt.IsZero() {
		return nil, errors.New("produce consumer proof: completed at is required")
	}
	return json.Marshal(consumerProofDocument{
		FormatVersion:        proofFormatVersion,
		Consumer:             proofConsumer,
		ConsumerCommit:       consumerCommit,
		CandidateVersion:     Version,
		ModulePath:           ModulePath,
		ModuleChecksum:       ModuleSum,
		ContractSuiteVersion: ContractSuiteVersion,
		Result:               proofResultPass,
		CompletedAt:          completedAt.UTC().Format(time.RFC3339Nano),
	})
}
