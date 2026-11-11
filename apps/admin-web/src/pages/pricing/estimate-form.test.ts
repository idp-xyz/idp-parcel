import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  emptyEstimateDraft,
  estimateLocalProblems,
  estimatePayloadOf,
  estimateServiceProductNote,
  type EstimateDraft,
} from './estimate-form';

// 本文件钉试算表单（票 operator-workspace-gaps/06，UC-PP-001「试算声明」）的编码层：草稿 → 载荷逐格照抄、墙钟时刻换 RFC 3339、
// 可缺的格留空即缺席（不拿零值或空串顶替）；本地只判编码层问题，领域规则留给服务端逐格答。

const SHANGHAI = 'Asia/Shanghai';

function draft(over: Partial<EstimateDraft>): EstimateDraft {
  return { ...emptyEstimateDraft(), ...over };
}

// Covers: 全格草稿逐格照抄进载荷；时刻按装配点时区换成 RFC 3339（UTC 表示，上海 16:00 即 08:00Z）。
test('全格草稿逐格编进载荷', () => {
  const payload = estimatePayloadOf(
    draft({
      scope: 'SYN-SCOPE-01',
      direction: 'SELL',
      basisAt: '2026-06-01T16:00',
      weightValue: '1.2',
      weightUnit: 'KG',
      length: '30',
      width: '20',
      height: '10',
      lengthUnit: 'CM',
      zone: 'Z1',
      origin: '200000',
      destination: '90210',
      settlementCurrency: 'CNY',
    }),
    SHANGHAI,
  );
  deepEqual(payload, {
    scope: 'SYN-SCOPE-01',
    direction: 'SELL',
    basisAt: '2026-06-01T08:00:00Z',
    weight: { value: '1.2', unit: 'KG' },
    dimensions: { length: '30', width: '20', height: '10', unit: 'CM' },
    zone: 'Z1',
    postalRoute: { origin: '200000', destination: '90210' },
    settlementCurrency: 'CNY',
  });
});

// Covers: 可缺的四格（尺寸、分区、邮编路线、结算币种）留空即缺席——缺席是「未声明」，服务端逐卡判要不要；空串会被读成一个值。
test('可缺的格留空即缺席', () => {
  const payload = estimatePayloadOf(
    draft({ scope: 'SYN-SCOPE-01', direction: 'BUY', basisAt: '2026-06-01T16:00', weightValue: '1', weightUnit: 'KG' }),
    SHANGHAI,
  );
  deepEqual(payload, {
    scope: 'SYN-SCOPE-01',
    direction: 'BUY',
    basisAt: '2026-06-01T08:00:00Z',
    weight: { value: '1', unit: 'KG' },
  });
});

// Covers: 本地只拦编码层——时刻换不出来、尺寸只填了一部分、邮编路线只填了一端；空的必需格不在这里拦，那是服务端要点名的格。
test('本地只判编码层问题', () => {
  deepEqual(estimateLocalProblems(emptyEstimateDraft(), SHANGHAI), {});
  deepEqual(estimateLocalProblems(draft({ basisAt: 'yesterday' }), SHANGHAI), { basisAt: '时刻格式应为本地日期与时间' });
  equal(estimateLocalProblems(draft({ length: '30' }), SHANGHAI).dimensions, '尺寸要三边与单位一起填，或都不填');
  equal(estimateLocalProblems(draft({ origin: '200000' }), SHANGHAI).postalRoute, '邮编路线要始发与目的一起填，或都不填');
});

// Covers: 选了服务产品载荷带 serviceProduct；不选即缺席，不发空串——缺席是「只看不声明产品的卡」（ADR-0378 决定二、三）。
test('服务产品：选了才带 serviceProduct，不选即缺席', () => {
  const base = { scope: 'SYN-SCOPE-01', direction: 'SELL', basisAt: '2026-06-01T16:00', weightValue: '1', weightUnit: 'KG' };
  const withProduct = estimatePayloadOf(draft({ ...base, serviceProduct: 'SYN-SP-FBA' }), SHANGHAI);
  equal(withProduct.serviceProduct, 'SYN-SP-FBA');
  const without = estimatePayloadOf(draft(base), SHANGHAI);
  equal('serviceProduct' in without, false);
  equal('serviceProduct' in estimatePayloadOf(draft({ ...base, serviceProduct: '  ' }), SHANGHAI), false);
});

// Covers: 不选时说清只按不指名产品的卡试算；选了说先用指名它的卡、没有再用不指名的卡。
test('服务产品一格下的说明随选没选而变', () => {
  equal(estimateServiceProductNote(emptyEstimateDraft()), '只按不指名产品的卡试算');
  equal(estimateServiceProductNote(draft({ serviceProduct: 'SYN-SP-FBA' })), '先用指名这个产品的卡，没有再用不指名的卡');
});
