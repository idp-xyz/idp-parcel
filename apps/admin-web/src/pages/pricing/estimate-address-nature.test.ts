import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { ADDRESS_NATURE_CODES, emptyEstimateDraft, estimatePayloadOf, type EstimateDraft } from './estimate-form';
import { addressNatureLabels, estimateMissingLabels, labelOf } from './presentation';

// 本文件钉试算页的收件地址性质（ADR-0347 决定四）：下拉不选即载荷缺席（未声明，不拿空串或默认值顶替）；选了原样送封闭词；
// 缺项码有中文说法，取值有中文标签，且与传输层的封闭词表逐个对得上。

const SHANGHAI = 'Asia/Shanghai';

function draft(over: Partial<EstimateDraft>): EstimateDraft {
  return { ...emptyEstimateDraft(), ...over };
}

const base = { scope: 'SYN-SCOPE-03', direction: 'SELL', basisAt: '2026-06-01T16:00', weightValue: '1', weightUnit: 'KG' };

// Covers: 不选地址性质 → 载荷里没有 addressNature 这个键（服务端读作未声明，读它的卡点名缺项）；空白串同样视作没选。
test('不选地址性质，载荷缺席该键', () => {
  const payload = estimatePayloadOf(draft(base), SHANGHAI);
  equal('addressNature' in payload, false);
  equal('addressNature' in estimatePayloadOf(draft({ ...base, addressNature: '  ' }), SHANGHAI), false);
});

// Covers: 选了就原样送封闭词，键名是 addressNature 而不是 addressType（引擎里「地址类型」承载偏远档位）。
test('选了地址性质，原样送封闭词', () => {
  for (const code of ADDRESS_NATURE_CODES) {
    const payload = estimatePayloadOf(draft({ ...base, addressNature: code }), SHANGHAI);
    deepEqual(payload, {
      scope: 'SYN-SCOPE-03',
      direction: 'SELL',
      basisAt: '2026-06-01T08:00:00Z',
      weight: { value: '1', unit: 'KG' },
      addressNature: code,
    });
    equal('addressType' in payload, false);
  }
});

// Covers: 下拉的取值与传输层的封闭词表一一对应，且每个都有中文标签；缺项码 ADDRESS_NATURE 有中文说法。
test('取值、标签与缺项说法齐全', () => {
  deepEqual([...ADDRESS_NATURE_CODES], ['COMMERCIAL', 'RESIDENTIAL']);
  for (const code of ADDRESS_NATURE_CODES) ok(addressNatureLabels[code], `${code} 缺中文标签`);
  equal(labelOf(addressNatureLabels, 'RESIDENTIAL'), '住宅地址');
  ok(estimateMissingLabels.ADDRESS_NATURE?.includes('地址性质'));
});
