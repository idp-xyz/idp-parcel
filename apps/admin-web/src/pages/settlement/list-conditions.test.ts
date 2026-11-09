import { test } from 'node:test';
import { deepEqual, equal, notEqual } from 'node:assert/strict';
import { catalogueConditionsQuery, catalogueRequestPath } from '../catalogue-query';
import {
  billListConditions,
  defaultBillSort,
  defaultPayableSort,
  isStatementRegistryConditions,
  pageEmptyNote,
  pageOnlyNote,
  pageStatusLabels,
  payableListConditions,
  statementListConditions,
  statementSummary,
} from './list-conditions';

// Covers: 「只看待对账」切到争议队列那一问：不带选择器；对账单册那一问带 registry——两问签名不同，勾选一变就回第一页。
test('对账单签：勾不勾「只看待对账」是两个读口两种条件', () => {
  const all = statementListConditions(false, ' SYN-ST-2610 ');
  const disputed = statementListConditions(true, ' SYN-ST-2610 ');
  equal(isStatementRegistryConditions(all), true);
  equal(isStatementRegistryConditions(disputed), false);
  equal(catalogueRequestPath('/settlement-statements', all, null), '/settlement-statements?registry=customer-statement&q=SYN-ST-2610');
  equal(catalogueRequestPath('/settlement-disputed-statements', disputed, 'c1'), '/settlement-disputed-statements?q=SYN-ST-2610&after=c1');
  equal(catalogueRequestPath('/settlement-statements', statementListConditions(false, ''), null), '/settlement-statements?registry=customer-statement');
  notEqual(catalogueConditionsQuery(statementListConditions(true, '')), catalogueConditionsQuery(statementListConditions(false, '')));
});

// Covers: 争议张数是整个队列的数，照读口答的说；作废几张只数这一页，字面写「本页」。
test('对账单签的条数：争议张数说整个队列，其余只说本页', () => {
  const rows = [{}, { voidBasis: 'SYN-VOID-01' }, {}];
  equal(statementSummary(rows, undefined), '本页 3 张 · 已作废 1 张');
  equal(statementSummary(rows, 12), '共 12 张有未了结异议 · 本页 3 张，其中已作废 1 张');
});

// Covers: 账单签表头两列下推成读口的 receivedAt、period；缺省序（收到时间新→旧）不写进查询串；账单号列没有可排维，存下的旧排序当缺省序。
test('账单签：检索词与收到时间、账期的先后下推，缺省序不带 sort', () => {
  const path = (keyword: string, sort: Parameters<typeof billListConditions>[1]) =>
    catalogueRequestPath('/settlement-statements', billListConditions(keyword, sort), null);
  equal(path('', defaultBillSort), '/settlement-statements?registry=supplier-bill-reception');
  equal(path('INV-2610', { key: 'received', dir: 1 }), '/settlement-statements?registry=supplier-bill-reception&sort=receivedAt&q=INV-2610');
  equal(path('', { key: 'period', dir: -1 }), '/settlement-statements?registry=supplier-bill-reception&sort=-period');
  equal(path('', { key: 'period', dir: 1 }), '/settlement-statements?registry=supplier-bill-reception&sort=period');
  equal(path('', { key: 'bill', dir: 1 }), '/settlement-statements?registry=supplier-bill-reception');
});

// Covers: 应付签只有审核时间一维可排；按应付单号检索走 q，不再另发逐字比的 documentNumber。
test('应付签：检索词走 q，只有审核时间可排', () => {
  const path = (keyword: string, sort: Parameters<typeof payableListConditions>[1]) =>
    catalogueRequestPath('/settlement-audited-payables', payableListConditions(keyword, sort), null);
  equal(path('', defaultPayableSort), '/settlement-audited-payables');
  equal(path('AP-202611-00001', defaultPayableSort), '/settlement-audited-payables?q=AP-202611-00001');
  equal(path('', { key: 'audited', dir: 1 }), '/settlement-audited-payables?sort=auditedAt');
  equal(path('', { key: 'number', dir: 1 }), '/settlement-audited-payables');
});

// Covers: 只筛本页的胶囊只列这一页上出现过的状态，按给定的先后；选中的那一格翻到没有它的页也留着。
test('只筛本页的状态胶囊：本页出现过的按先后列，选中的那一格留着', () => {
  const order = ['待审核', '审核人未配置', '有争议待再审', '已审完'];
  const rows = [{ label: '已审完' }, { label: '待审核' }, { label: '已审完' }];
  deepEqual(pageStatusLabels(rows, (row) => row.label, order, 'ALL'), ['待审核', '已审完']);
  deepEqual(pageStatusLabels(rows, (row) => row.label, order, '有争议待再审'), ['待审核', '有争议待再审', '已审完']);
  deepEqual(pageStatusLabels([], (row: { label: string }) => row.label, order, 'ALL'), []);
});

// Covers: 只筛本页的那一句与筛空时的那一句——页内筛空了要说翻页看看，读口答零行就只说没有匹配的。
test('只筛本页的提醒与筛空时的话', () => {
  equal(pageOnlyNote(12, 12, false, '份'), '状态只筛本页');
  equal(pageOnlyNote(12, 3, true, '份'), '只筛本页：本页 12 份里有 3 份');
  equal(pageEmptyNote('账单', 5), '这一页上没有匹配的账单；只筛本页的条件不看其余几页，翻页看看');
  equal(pageEmptyNote('账单', 0), '没有匹配的账单');
});
