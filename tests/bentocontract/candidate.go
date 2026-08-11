// Package bentocontract 是 Parcel 针对某一个不可变 `idp-bento-go` 候选的消费者合同。
// 它只为在 Parcel 自身安全边界内证明框架合同而存在；任何生产包都不得导入它。
package bentocontract

import (
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentodomain "go.idp.xyz/idp-bento-go/domain"
)

// 候选身份。这些值在测试时与真实 module 图比对，因此证明不可能声称一个构建
// 实际没有解析到的版本。
const (
	// ModulePath 是正式 module path。合同必须经由它解析候选，
	// 不得走本地目录、GitHub 导入路径或源码副本。
	ModulePath = "go.idp.xyz/idp-bento-go"

	// Version 是受测的不可变候选。
	Version = "v0.1.0-rc.2"

	// ModuleSum 是候选 module 的 checksum。checksum 不同即工件不同，合同失败。
	// 该值已由 sum.golang.org 记入公共透明日志（记录号 58659836）。
	ModuleSum = "h1:W9T1KsbXwuVs3lHTNwSUcEBY0Ids00hUH7I9KuBtCEo="

	// GoModSum 是候选 `go.mod` 的 checksum。
	GoModSum = "h1:RFR6ylLNIIA7e4PPGVCzojYiH6DB8eHd2s5vI9XcUgY="

	// OriginRef 是候选必须来自的不可变 Git 引用。
	OriginRef = "refs/tags/v0.1.0-rc.2"

	// OriginCommit 是该候选 tag 剥离后指向的框架提交。tag 对象本身是 5aa0974，
	// 与此不同；要绑的是提交。
	OriginCommit = "56322dc25373b872ab5c49a74c0d544d7c088184"
)

// GovernanceMode 是本证明所确认的框架治理模式。Parcel 记录它是为了表明自己清楚
// 在为什么背书，而不是伪造第二个评审人。该模式意味着哪些保证由机器门禁承担、
// 哪些无人承担，是框架的话语权，写在它的维护与发布治理合同里。
//
// 注意：`v0.1.0-rc.2` 的证明 JSON 只认 9 个字段并拒收未知字段，该值因而进不了
// 证明本身，只能由 Parcel 自身记录承载。框架 `main` 已把
// `acknowledged_governance_mode` 设为必填，一旦出现带该字段的候选，此处与
// 证明产出器须同时复评。
const GovernanceMode = "SOLO_BOOTSTRAP"

// ContractSuiteVersion 是框架协调作业钉住的合同矩阵版本。两个消费者必须报同一个
// 值，否则一方可能拿陈旧矩阵去证明，所以它不是 Parcel 自己的标签。
const ContractSuiteVersion = "r05-v1"

// 以下声明令本包对候选形成真实且非测试的编译依赖，而不只是把版本号写成字符串常量。
// `PBC-01` 要证的是「从正式 module path 与精确版本编译」，仅在 go.mod 里挂一条
// 间接依赖证不到这一点——那样的依赖还会被任何一次 `go mod tidy` 静默删除。
//
// 取的是首个切片确实要用到的符号：事件与事件缓冲、时间与标识来源、事务边界。
// 不取 `testkit`：按 `PBC-08`，它只允许出现在测试文件里。
var (
	_ bentodomain.Event
	_ bentodomain.EventBuffer
	_ bentoapp.Clock
	_ bentoapp.Transactor
	_ bentoapp.IDGenerator[string]
)
