echo "== 单据编号时区登记（document-numbering：演示租户一版；票 document-numbering/02） =="
# 单据编号时区是租户取值、产品不给默认（ADR-0355 决定七；参数登记册 PAR-COM-19）：开立方没有业务时区的单据（委托、异常案件等）按它出号里的
# 日期，没登就照常开立、不带单据号。这一版是演示租户的合成取值（Asia/Shanghai，2026-01-01 起），不是任何租户的时区，
# 证据只记 S。不登任何租户模板：各类单据用产品默认模板。排在各节开立单据之前，种子开立的单据才带得上号。重灌同一份
# 答已有结果。
"$BIN/parcel-document-numbering-register" -kind time-zone \
  -file "$SEEDS/document-numbering/01-numbering-time-zone-syn-tenant-01.json"
