package application

// settlementCanonicalDigest 取回领域形状的摘要。这些文档只含字符串、整数与它们的切片，
// 编码不会失败；失败说明形状写坏了，空摘要会把两份不同内容读成重放。
func settlementCanonicalDigest(_ []byte, digest string, err error) string {
	if err != nil {
		panic(err)
	}
	return digest
}
