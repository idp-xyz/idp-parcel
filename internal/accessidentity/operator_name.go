package accessidentity

import "context"

// MaxOperatorNameLength 是操作者姓名的长度上限（按字符）。姓名是给人认的一格，不是放备注的地方。
const MaxOperatorNameLength = 60

// OperatorNameOutcome 是登一次操作者姓名的答复。
type OperatorNameOutcome string

const (
	// OperatorNameRecorded 是姓名登上了，或换成了新的。
	OperatorNameRecorded OperatorNameOutcome = "OPERATOR_NAME_RECORDED"
	// OperatorNameCleared 是姓名删掉了；本来就没有也答这一格。
	OperatorNameCleared OperatorNameOutcome = "OPERATOR_NAME_CLEARED"
	// OperatorNameOperatorNotFound 是本租户的操作者册上没有这个主体。不说它在不在别的租户。
	OperatorNameOperatorNotFound OperatorNameOutcome = "OPERATOR_NOT_FOUND"
)

func (outcome OperatorNameOutcome) String() string { return string(outcome) }

// OperatorNameRow 是一位登了姓名的操作者。
type OperatorNameRow struct {
	Subject OperatorSubject
	Name    string
}

// OperatorNameBook 读写操作者的姓名（access_identity.operator_name）。姓名是 AGENTS.md 红线「敏感实例外置」对操作者
// 开的例外：只登姓名这一格，可以改、可以删。写入在没有环境事务时拒绝。
type OperatorNameBook interface {
	// SaveOperatorName 给本租户册上的一个主体登姓名；name 为空是删掉。by 是登的人。
	SaveOperatorName(ctx context.Context, tenantID string, subject OperatorSubject, name string, by OperatorSubject) (OperatorNameOutcome, error)
	// ListOperatorNames 列出本租户登了姓名的操作者。
	ListOperatorNames(ctx context.Context, tenantID string) ([]OperatorNameRow, error)
}
