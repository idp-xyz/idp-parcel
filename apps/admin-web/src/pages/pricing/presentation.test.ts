import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { chargeLineName, estimateMissingLabels } from './presentation';

// Covers: 费用行主列用词。价卡写的中文说明原样用；引擎写死的英文基础运费说明与缺失的说明改查费用代码词典。
test('中文说明原样用，英文或缺失的说明改用费用代码的中文词', () => {
  equal(chargeLineName({ code: 'FUEL_SURCHARGE', description: '燃油附加费（合成演示值）' }), '燃油附加费（合成演示值）');
  equal(chargeLineName({ code: 'REMOTE_AREA_SURCHARGE', description: 'SYN 偏远区域附加（合成演示值）' }), 'SYN 偏远区域附加（合成演示值）');
  equal(chargeLineName({ code: 'BASE_FREIGHT', description: 'Base rate' }), '基础运费');
  equal(chargeLineName({ code: 'OVERSIZE_SURCHARGE', description: '' }), '超尺寸费');
});

// 计价缺项封闭集新两码（ADR-0381 决定二，票 declared-goods-lines/27）：寄件邮编与尺寸也有运营译法；重量或量纲是评审顺手
// 补齐的先在缺漏。译法盖没盖满封闭集由 shipment-request/stall-reasons.contract.test.ts 对着 Go 源核。
test('缺项码新词条有运营译法', () => {
  equal(estimateMissingLabels.ORIGIN_POSTAL, '寄件邮编（这张价卡的分区目录按始发邮编前缀分区）');
  equal(estimateMissingLabels.DIMENSIONS, '尺寸（这张价卡按尺寸或体积重判价，要逐件给长宽高）');
  equal(estimateMissingLabels.MEASUREMENT, '重量或量纲（这一件的重量或重量单位读不出，计价只取申报的测量，不替你补）');
});
