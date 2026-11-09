import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { catalogueConditionsQuery } from '../catalogue-query';
import {
  allStatuses,
  newestFirst,
  routeListConditions,
  routeSortParam,
  routeStatusChips,
  routeStatusDimensions,
  stateWordOf,
} from './route-plan-conditions';
import { rerouteSuggestionStateLabels } from './reroute-decision';

// Covers: 各册胶囊下推到读口点名的那一维（初始路由与复核按 conclusion，改路建议与成本并列按 state），词即该维词表的词，
// 中文词与行上的状态胶囊同一个。
test('各册的状态胶囊下推到读口点名的那一维，词表照读口', () => {
  deepEqual(
    Object.fromEntries(Object.entries(routeStatusDimensions).map(([register, status]) => [register, [status.dimension, Object.keys(status.labels)]])),
    {
      'initial-route': ['conclusion', ['ROUTE_FORMED', 'NO_CURRENT_ROUTE']],
      reassessment: ['conclusion', ['STILL_APPLICABLE', 'PLAN_LAPSED', 'FIRST_PLAN_FORMED', 'REROUTED']],
      'reroute-suggestion': ['state', ['PENDING', 'DECIDED', 'SUPERSEDED']],
      'initial-route-tie': ['state', ['PENDING', 'DECIDED', 'FORMED_OTHERWISE']],
    },
  );
  equal(routeStatusDimensions['initial-route'].labels.NO_CURRENT_ROUTE, '无可用路线');
  equal(routeStatusDimensions['reroute-suggestion'].labels.SUPERSEDED, stateWordOf(rerouteSuggestionStateLabels, 'SUPERSEDED'));
  equal(stateWordOf(rerouteSuggestionStateLabels, 'SUPERSEDED'), '已过时');
  equal(stateWordOf(rerouteSuggestionStateLabels, 'UNKNOWN'), 'UNKNOWN');
});

// Covers: 「全部」不带状态一维，检索词照交、去空白由 catalogueRequestPath 做；选中一格即那一维的一个值。
test('检索词、胶囊与排序拼成一问的条件', () => {
  const tie = routeStatusDimensions['initial-route-tie'];
  equal(catalogueConditionsQuery(routeListConditions('', tie, allStatuses)), '');
  equal(catalogueConditionsQuery(routeListConditions(' SYN ', tie, 'PENDING', 'settledAt')), 'sort=settledAt&state=PENDING&q=SYN');
});

// Covers: 缺省序（这一维倒序）不写进查询串，翻成正序才写维名；点的不是这一列也不写。
test('表头先后只在正序时写进 sort', () => {
  const column = { key: 'recorded-at', field: 'recordedAt' };
  equal(routeSortParam(newestFirst(column), column), undefined);
  equal(routeSortParam({ key: 'recorded-at', dir: 1 }, column), 'recordedAt');
  equal(routeSortParam({ key: 'other', dir: 1 }, column), undefined);
});

// Covers: 胶囊不带计数；点选中的那一格回到全部，点「全部」就是全部。
test('下推的胶囊不带计数，再点一下回到全部', () => {
  const picks: string[] = [];
  const chips = routeStatusChips(routeStatusDimensions.reassessment, 'PLAN_LAPSED', (code) => picks.push(code));
  deepEqual(
    chips.map((chip) => [chip.id, chip.active, chip.count]),
    [
      [allStatuses, false, undefined],
      ['STILL_APPLICABLE', false, undefined],
      ['PLAN_LAPSED', true, undefined],
      ['FIRST_PLAN_FORMED', false, undefined],
      ['REROUTED', false, undefined],
    ],
  );
  chips[2].onClick();
  chips[4].onClick();
  chips[0].onClick();
  deepEqual(picks, [allStatuses, 'REROUTED', allStatuses]);
});
