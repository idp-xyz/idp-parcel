package accessidentity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxOperatorNameLength 是操作者姓名的长度上限（按字符）。姓名是给人认的一格，不是放备注的地方。
const MaxOperatorNameLength = 60

// ErrInvalidOperatorName 表示给出的不是一个能登的姓名。
var ErrInvalidOperatorName = errors.New("access identity: invalid operator name")

// NewOperatorName 交回去掉首尾空白的姓名。空白、超长、带控制字符的都拒：空白不是「删掉」的写法（删掉是不给姓名）；
// 控制字符不是名字的一部分——NUL 库不收，换行会把列表上的一格撑成几行。
func NewOperatorName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	switch {
	case name == "":
		return "", fmt.Errorf("%w: 姓名是空白", ErrInvalidOperatorName)
	case utf8.RuneCountInString(name) > MaxOperatorNameLength:
		return "", fmt.Errorf("%w: 姓名超过 %d 个字符", ErrInvalidOperatorName, MaxOperatorNameLength)
	case strings.ContainsFunc(name, unicode.IsControl):
		return "", fmt.Errorf("%w: 姓名里有控制字符", ErrInvalidOperatorName)
	}
	return name, nil
}

// OperatorNameOutcome 是登一次操作者姓名的答复。
type OperatorNameOutcome string

const (
	// OperatorNameRecorded 是姓名登上了，或换成了新的；册上本来就是这个姓名也答这一格。
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
	// SaveOperatorName 给本租户册上的一个主体登姓名；name 须是 NewOperatorName 交回的，空串是删掉。recordedBy 是谁登的：
	// 在线口给调用者的「发行方#sub」，受控命令行给批文头写的那一格。册上本来就是这个姓名时不动那一行，
	// 重放不把「谁登的」改写成重放的人。
	SaveOperatorName(ctx context.Context, tenantID string, subject OperatorSubject, name, recordedBy string) (OperatorNameOutcome, error)
	// ListOperatorNames 列出本租户登了姓名的操作者。
	ListOperatorNames(ctx context.Context, tenantID string) ([]OperatorNameRow, error)
}
