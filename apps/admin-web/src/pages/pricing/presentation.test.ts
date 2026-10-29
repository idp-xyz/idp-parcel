import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { chargeLineName } from './presentation';

// Covers: 费用行主列用词。价卡写的中文说明原样用；引擎写死的英文基础运费说明与缺失的说明改查费用代码词典。
test('中文说明原样用，英文或缺失的说明改用费用代码的中文词', () => {
  equal(chargeLineName({ code: 'FUEL_SURCHARGE', description: '燃油附加费（合成演示值）' }), '燃油附加费（合成演示值）');
  equal(chargeLineName({ code: 'REMOTE_AREA_SURCHARGE', description: 'SYN 偏远区域附加（合成演示值）' }), 'SYN 偏远区域附加（合成演示值）');
  equal(chargeLineName({ code: 'BASE_FREIGHT', description: 'Base rate' }), '基础运费');
  equal(chargeLineName({ code: 'OVERSIZE_SURCHARGE', description: '' }), '超尺寸费');
});
