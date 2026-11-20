package domain

// WaveID 是调用方给的波次标识。波次不取得出库请求的身份。
type WaveID struct{ requiredValue }

func NewWaveID(value string) (WaveID, error) {
	required, err := newRequiredValue("wave ID", value)
	return WaveID{required}, err
}

// PickTaskID 是一份拣货任务的身份，由本上下文签发。
type PickTaskID struct{ requiredValue }

func NewPickTaskID(value string) (PickTaskID, error) {
	required, err := newRequiredValue("pick task ID", value)
	return PickTaskID{required}, err
}

// PickFactID 是设备签发的拣货事实身份。
type PickFactID struct{ requiredValue }

func NewPickFactID(value string) (PickFactID, error) {
	required, err := newRequiredValue("pick fact ID", value)
	return PickFactID{required}, err
}
