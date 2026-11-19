package ports

// RegistrySaveOutcome 是一笔修订在持久化面的落点（ADR-0031），各册的登记册共用这一套：`已落库`是新写入的一笔，`已在册`是
// 同键同内容的重放，`内容冲突`是同键异内容。业务格不用 error 表达，事务因此保持可用。比的只是完整内容，不比登记责任方与
// 登记时刻。
type RegistrySaveOutcome uint8

const (
	RegistrySaveOutcomeInvalid RegistrySaveOutcome = iota
	RegistryRevisionSaved
	RegistryRevisionAlreadyRegistered
	RegistryRevisionContentConflict
)

func (outcome RegistrySaveOutcome) String() string {
	switch outcome {
	case RegistryRevisionSaved:
		return "SAVED"
	case RegistryRevisionAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case RegistryRevisionContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}
