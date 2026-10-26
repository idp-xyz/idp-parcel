package domain

// CustomerClaimShapeVersion 是首方索赔提交的规范化形状版本（ADR-0139 决定八；ADR-0310；票 first-party-api/06、09）。
//
// 形状 VCL-1 的字段见 CustomerClaimShapeFields。索赔以（账户、批次、项）为自然键（ADR-0310 取舍 F），所以这一口没有内容摘要，
// 也没有「同键换内容」的冲突格：同一对（批次、项）再次到达答已在案的原项，不比对内容（ReceiveClaim 的幂等按键不按内容）；
// 接入请求标识只管传输重放，不进任何判据（ADR-0139 决定五）。租户、账户取自信封，不在形状里。
const CustomerClaimShapeVersion = "VCL-1"

// CustomerClaimShapeFields 是 VCL-1 载荷的封闭字段全集，次序即契约文档里的次序。载荷出现其外的任何一格（自报的租户、账户、
// 操作者都在此列）即拒。提交时间不在其中：取到达信封（receive_claim 头注）。
var CustomerClaimShapeFields = [...]string{
	"accessRequestId", // 接入请求标识，信封元数据
	"batch",           // 客户侧批次引用
	"item",            // 客户侧项标识，批次内唯一
	"applicant",       // 申请人引用，授权维由资格审核核
	"contract",        // 合同责任范围引用，须登记在信封账户名下
	"target",          // 索赔目标包裹，须属信封账户
	"kind",            // 索赔类型引用
}
