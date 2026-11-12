import { test } from 'node:test';
import { deepEqual, ok } from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { knownRestrictionReasonCodes } from './financial-control';
import { knownResumePaths } from './last-attempt';
import { estimateGapCardSideWords, knownSettlementStallCodes, knownShipmentStallCodes } from './stall-reasons';
import { knownEstimateSupplementGapCodes } from './supplement-gaps';
import { estimateMissingLabels } from '../pricing/presentation';

// 停因、续办方与限制原因的词表对着 Go 源核（做法同 pages/network/route-plans.contract.test.ts：读同一份源，不在前端树里抄第二份）。
// Go 侧加了码、词表没跟上就红，红的那条点名漏了哪个码；词表里留着 Go 侧已经没有的码也红，免得改了名的旧码一直挂着。
// 路径相对 apps/admin-web：scripts/run-tests.mjs 固定以它为 cwd 起 node --test。

const repoRoot = '../..';

function goSource(path: string): string {
  return readFileSync(`${repoRoot}/${path}`, 'utf8');
}

/** 一个 String() 方法体里 `return "X"` 的全部 X。方法签名找不到就抛：改了名的方法不该让这条测试悄悄变成空检查。 */
function stringMethodCodes(source: string, signature: string): string[] {
  const start = source.indexOf(signature);
  ok(start >= 0, `Go 源里找不到 ${signature}`);
  const end = source.indexOf('\n}\n', start);
  const body = source.slice(start, end);
  return [...body.matchAll(/return "([A-Z_]+)"/g)].map((match) => match[1]!);
}

function literalCodes(source: string, pattern: RegExp): string[] {
  return [...source.matchAll(pattern)].map((match) => match[1]!);
}

function sorted(codes: Iterable<string>): string[] {
  return [...new Set(codes)].sort();
}

test('小包托运的未决原因：Go 侧每个码都有译法，词表里没有 Go 侧已经没有的码', () => {
  const goCodes = stringMethodCodes(
    goSource('internal/parcelshipment/application/judgment_continuation.go'),
    'func (reason JudgmentPendingReason) String() string',
  );
  ok(goCodes.length > 0, '一个未决原因码都没读出来');
  deepEqual(sorted(knownShipmentStallCodes), sorted(goCodes));
});

test('续办方：领域 ResumePath 的每个词都认得，认得的词 Go 侧都还有', () => {
  const goWords = stringMethodCodes(
    goSource('internal/parcelshipment/domain/acceptance_task.go'),
    'func (path ResumePath) String() string',
  );
  ok(goWords.length > 0, '一个续办方都没读出来');
  deepEqual(sorted(knownResumePaths), sorted(goWords));
});

test('结算的 `SA-` 停因：未形成原因与两种拒收都有译法，词表里没有 Go 侧已经没有的码', () => {
  const notFormed = stringMethodCodes(
    goSource('internal/settlementaccounting/application/apply_pre_acceptance_control.go'),
    'func (reason NotFormedReason) String() string',
  ).map((code) => `SA-${code}`);
  // 施加那一路上适配器直接写死的两种拒收；释放那一路的码进的是释放未受理册，不进受理任务，不在这张表。
  const refused = literalCodes(
    goSource('internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go'),
    /refusedAssessment\([^,]+,\s*"(SA-[A-Z_]+)"\)/g,
  );
  ok(notFormed.length > 0 && refused.length > 0, '结算停因一个都没读出来');
  deepEqual(sorted(knownSettlementStallCodes), sorted([...notFormed, ...refused]));
});

test('受理前估价的补件缺口码：词表那一族与小包托运适配器的常量字面一致', () => {
  const goCodes = literalCodes(
    goSource('internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go'),
    /gap[A-Za-z]+Missing\s*=\s*"([A-Z_]+)"/g,
  );
  ok(goCodes.length > 0, '一个缺口码都没读出来');
  deepEqual(sorted(knownEstimateSupplementGapCodes), sorted(goCodes));
});

test('计价缺项的封闭集：卡侧词表的键都在集内，运营译法盖满整集', () => {
  const goCodes = literalCodes(
    goSource('internal/parcelpricing/application/form_estimate_evaluations.go'),
    /EstimateMissing[A-Za-z]+\s*=\s*"([A-Z_]+)"/g,
  );
  ok(goCodes.length > 0, '一个缺项码都没读出来');
  // 卡侧分流（declarationIncompleteViewOf）拿字面码判：计价一旦改码，这里先红，不让卡侧停因静默退化成客户补充那一句。
  deepEqual(
    Object.keys(estimateGapCardSideWords).filter((code) => !goCodes.includes(code)),
    [],
  );
  deepEqual(sorted(Object.keys(estimateMissingLabels)), sorted(goCodes));
});

test('结算的限制原因：两本账写下的码都有译法，词表里没有 Go 侧已经没有的码', () => {
  const freeze = literalCodes(goSource('internal/settlementaccounting/domain/funds_freeze.go'), /NewRestrictionReason\("([A-Z_]+)"\)/g);
  const exposure = literalCodes(
    goSource('internal/settlementaccounting/domain/credit_exposure.go'),
    /\.restricted\([^)]*"([A-Z_]+)"\)/g,
  );
  ok(freeze.length > 0 && exposure.length > 0, '限制原因一个都没读出来');
  deepEqual(sorted(knownRestrictionReasonCodes), sorted([...freeze, ...exposure]));
});
