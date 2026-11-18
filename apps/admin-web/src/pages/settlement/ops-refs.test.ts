import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  accountCounterparties,
  feeItemLabel,
  parcelFromRef,
  pricedWeightText,
  sourceFactLabel,
  weightBasisText,
} from './ops-refs';

test('从来源事实与确认依据里认出包裹号；认不出答 undefined', () => {
  equal(
    parcelFromRef('network-intake/DPCL-P24H5Q3QU6KJJ5TOFTY24AOGXY/NODE_INTAKE/INTAKEV-74GLJZMVQCL3AAP2OMEP7CXXBE'),
    'DPCL-P24H5Q3QU6KJJ5TOFTY24AOGXY',
  );
  equal(parcelFromRef('final-outcome/DPCL-MX5MKQIT5GMHDACY3YEARBCTSY/EFFECTIVE_DELIVERY/DRV-000000000001'), 'DPCL-MX5MKQIT5GMHDACY3YEARBCTSY');
  equal(parcelFromRef('EVAL-34R7GUH42IYM4DUSA7LQNWCYSI'), undefined);
  equal(parcelFromRef(undefined), undefined);
});

test('来源事实的族翻成中文，族认不出时整条原样回显', () => {
  equal(sourceFactLabel('network-intake/DPCL-X/NODE_INTAKE/INTAKEV-1'), '网点收寄');
  equal(sourceFactLabel('final-outcome/DPCL-X/EFFECTIVE_DELIVERY/DRV-1'), '签收终局');
  equal(sourceFactLabel('something-else/x'), 'something-else/x');
  equal(sourceFactLabel(undefined), undefined);
});

test('费用项目按码里的种类段翻中文，不写死租户整码；认不出原样回显', () => {
  equal(feeItemLabel('SYN-FEE-ITEM-FREIGHT-01'), '运费');
  equal(feeItemLabel('SYN-FEE-ITEM-TRUNK-COST-01'), '干线成本');
  equal(feeItemLabel('ACME-FEE-ITEM-FREIGHT-07'), '运费');
  equal(feeItemLabel('FEE-ITEM-LAST-MILE'), '末端派送费');
  equal(feeItemLabel('SYN-FEE-ITEM-MYSTERY-01'), 'SYN-FEE-ITEM-MYSTERY-01');
  equal(feeItemLabel('FREIGHT'), 'FREIGHT');
});

test('结算账户到相对方的对应只取已确认费用上登了的，首次出现为准', () => {
  const map = accountCounterparties([
    { settlementAccount: 'A1', counterparty: 'P1' },
    { settlementAccount: 'A1', counterparty: 'P2' },
    { counterparty: 'P3' },
    { settlementAccount: 'A2' },
  ]);
  deepEqual([...map.entries()], [['A1', 'P1']]);
});

// Covers: 计价重量与实重是两格。取较大者的卡上复测只把尺寸量大时，实重不变而计价重量变了；读口没给计价重量就空着，
// 不拿实重顶上。
test('计价重量只认读口给的那一格，不拿实重顶上', () => {
  equal(
    pricedWeightText({ pricedWeight: '3', pricedWeightUnit: 'KG', actualWeight: '2.1', actualWeightUnit: 'KG' }),
    '3 KG',
  );
  equal(
    pricedWeightText({ weightSource: 'actual-measurement', actualWeight: '2.1', actualWeightUnit: 'KG' }),
    undefined,
  );
  equal(pricedWeightText({}), undefined);
});

test('来源一格写明出自实测还是申报、实重与尺寸；申报没报尺寸写「未报尺寸」', () => {
  const labels: Record<string, string> = { 'actual-measurement': '实测', 'declared-measurement': '申报' };
  const label = (code: string) => labels[code] ?? code;
  equal(
    weightBasisText(
      {
        weightSource: 'actual-measurement',
        actualWeight: '2.1',
        actualWeightUnit: 'KG',
        dimensionSides: ['30', '25', '20'],
        dimensionUnit: 'CM',
      },
      label,
    ),
    '实测：实重 2.1 KG，尺寸 30×25×20 CM',
  );
  equal(
    weightBasisText({ weightSource: 'declared-measurement', actualWeight: '1.2', actualWeightUnit: 'KG' }, label),
    '申报：实重 1.2 KG，未报尺寸',
  );
  // 评价没有测量事实：来源整格不出，哪怕计价重量在。
  equal(weightBasisText({ pricedWeight: '5', pricedWeightUnit: 'KG' }, label), undefined);
});
