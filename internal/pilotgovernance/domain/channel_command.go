package domain

// ChannelCommand 是受控通道命令的封闭集合（票 pilot-governance-context-gaps/01 裁决）。
//
// 这个集合归产品定，不归租户扩：`parcel-governance-register` 按批开放子命令（首批三个，阶段评审与
// 接管是第二批），留痕表 `channel_execution.command` 记的就是「哪一个受控命令被执行过」——一个入口
// 不认识的词到不了留痕那一步（入口先按集合拒），所以把集合关在领域里不会让任何真实发生过的执行
// 写不进去。此前集合只封闭在 CLI 的字符串常量与一个 switch 上；搬到这里之后 enum 门禁替它守
// 「新增取值必须同时补 String()」，库上另有一道 CHECK 作第二道镜像（迁移 0006）。
//
// String() 交回的是 CLI 子命令的字面拼法而不是大写常量名：留痕表里已经落着这些字面值，
// 审计查询按它们筛，换一套拼法等于把同一个命令在册上写成两种字。
type ChannelCommand uint8

const (
	ChannelCommandInvalid ChannelCommand = iota
	ChannelCommandAuthorityInterval
	ChannelCommandSuspend
	ChannelCommandResume
)

func (command ChannelCommand) valid() bool {
	return command >= ChannelCommandAuthorityInterval && command <= ChannelCommandResume
}

func (command ChannelCommand) String() string {
	switch command {
	case ChannelCommandAuthorityInterval:
		return "authority-interval"
	case ChannelCommandSuspend:
		return "suspend"
	case ChannelCommandResume:
		return "resume"
	default:
		return ""
	}
}

// ParseChannelCommand 把入口收到的子命令词译回封闭集；集合外的词答假，不猜近似。
func ParseChannelCommand(text string) (ChannelCommand, bool) {
	for command := ChannelCommandAuthorityInterval; command.valid(); command++ {
		if command.String() == text {
			return command, true
		}
	}
	return ChannelCommandInvalid, false
}
