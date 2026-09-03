import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ModuleInfo } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import type { HandoverScopeSummaryResponseBody } from './records-api';
import { scopeSummaryRowsOf, scopeSummaryViewState } from './handover-scope-summary';

// 四格结果代数各答一次（票 admin-web-audit-followups/06）。这一区最容易错的一格是
// `不成立汇总`——它与「三格都是 0」在页面上一旦长成同一个样子，看的人就分不出「这个范围
// 还没有交接」与「这个范围有交接，只是各格恰好为零」，而两者要做的事不同。

const info: ModuleInfo = {
  title: '运输履约查阅',
  owner: 'transport-fulfillment',
  source: 'CONTEXT.md',
};

const options = { module: info, endpoint: 'GET /transport-fulfillment-handover-scope-summary?scope=…' };
const noRetry = () => {};

function outcome(body: HandoverScopeSummaryResponseBody): ApiResult<HandoverScopeSummaryResponseBody> {
  return { kind: 'outcome', status: 200, body };
}

test('已汇总：三格计数与合计原样上列，`全部已交接`取服务端派生的那一格', () => {
  const body: HandoverScopeSummaryResponseBody = {
    outcome: 'SCOPE_SUMMARIZED',
    summary: {
      scope: 'scope-1',
      handedOver: 2,
      refused: 1,
      unconfirmed: 0,
      total: 3,
      allHandedOver: false,
    },
  };

  deepEqual(scopeSummaryRowsOf(body), [
    {
      key: 'scope:scope-1',
      values: {
        scope: 'scope-1',
        handedOver: '2',
        refused: '1',
        unconfirmed: '0',
        total: '3',
        allHandedOver: '否',
      },
    },
  ]);
  equal(scopeSummaryViewState(outcome(body), 'scope-1', noRetry, options).kind, 'ready');
});

test('已汇总里某一格为零照实显示 0，不显示成缺席', () => {
  const [row] = scopeSummaryRowsOf({
    outcome: 'SCOPE_SUMMARIZED',
    summary: {
      scope: 'scope-all',
      handedOver: 4,
      refused: 0,
      unconfirmed: 0,
      total: 4,
      allHandedOver: true,
    },
  });

  equal(row.values.refused, '0');
  equal(row.values.unconfirmed, '0');
  equal(row.values.allHandedOver, '是');
});

test('不成立汇总：一行都不给，措辞点名它不等于三个零', () => {
  const body: HandoverScopeSummaryResponseBody = { outcome: 'SCOPE_NOT_SUMMARIZABLE' };

  deepEqual(scopeSummaryRowsOf(body), []);

  const state = scopeSummaryViewState(outcome(body), 'scope-empty', noRetry, options);
  equal(state.kind, 'empty');
  ok(state.kind === 'empty');
  ok(state.title?.includes('scope-empty'), `空态标题没点名范围：${state.title}`);
  ok(state.title?.includes('还没有交接'), `空态标题没说清是哪一种空：${state.title}`);
  // 三个零那种答案说的是另一件事，措辞必须把两者分开——这一句是本区存在的全部理由。
  ok(state.description?.includes('0'), `空态说明没有把「三格都是 0」摘出来对比：${state.description}`);
});

test('未决：带成因与续办引用，可重试；不冒充「这个范围没有交接」', () => {
  const state = scopeSummaryViewState(
    outcome({
      outcome: 'SCOPE_UNDECIDED',
      reason: 'HANDOVER_REGISTRY_UNAVAILABLE',
      continuationReference: 'CONT-abc123',
    }),
    'scope-1',
    noRetry,
    options,
  );

  equal(state.kind, 'error');
  ok(state.kind === 'error');
  ok(state.description?.includes('交接登记册读不回来'), state.description);
  ok(state.description?.includes('CONT-abc123'), state.description);
  ok(typeof state.onRetry === 'function', '未决要给重试——它不是终局答案');
});

test('输入未受理：不给重试，同样的输入重试仍是同一个答案', () => {
  const state = scopeSummaryViewState(
    outcome({ outcome: 'INPUT_NOT_ACCEPTED' }),
    'scope-??',
    noRetry,
    options,
  );

  equal(state.kind, 'error');
  ok(state.kind === 'error');
  equal(state.onRetry, undefined);
});

test('范围未填不发请求，且与「问过了、这个范围还没有交接」分开措辞', () => {
  const idle = scopeSummaryViewState(null, '   ', noRetry, options);
  equal(idle.kind, 'empty');
  ok(idle.kind === 'empty');
  ok(idle.title?.includes('尚未指定'), idle.title);

  // 填了范围但答案还没回来是加载中，不是空。
  equal(scopeSummaryViewState(null, 'scope-1', noRetry, options).kind, 'loading');
});

test('未配置与未形成答案仍走全站那套 HTTP 代数，不在本区另抄一份', () => {
  equal(
    scopeSummaryViewState({ kind: 'unconfigured' }, 'scope-1', noRetry, options).kind,
    'unconfigured',
  );
  equal(
    scopeSummaryViewState(
      { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' },
      'scope-1',
      noRetry,
      options,
    ).kind,
    'error',
  );
});
