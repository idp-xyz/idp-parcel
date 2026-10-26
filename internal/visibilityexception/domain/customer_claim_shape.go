package domain

// CustomerClaimShapeVersion 是首方索赔提交的规范化形状版本（ADR-0139 决定八；ADR-0310；票 first-party-api/06）。只定名与版本，字段由
// 票 first-party-api/09 填：索赔以（账户、批次、项）为自然键（ADR-0310 取舍 F），摘要只判同一索赔的内容是否同一份，接入请求标识
// 不进摘要（ADR-0139 决定五）。填之前索赔的首方口不开。
const CustomerClaimShapeVersion = "VCL-1"
