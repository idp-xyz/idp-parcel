echo "== 报价发出：给演示租户已有的销售价卡登一条 SYN- 发出记录（证据 S） =="
# 相对方、责任法人与价卡都是前面步骤已经灌进去的，这里不新造价格金额。
# 价卡是 SYN-PLAN-CN-SG-01 v1，已登记。摘要与那份登记文件上的 contentDigest 相同。
"$BIN/parcel-commercial" register-quotation-send -input "$SEEDS/commercial/register-quotation-send.json"
