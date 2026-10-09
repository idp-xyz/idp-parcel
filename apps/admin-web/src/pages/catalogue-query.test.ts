import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { catalogueConditionsNarrow, catalogueConditionsQuery, catalogueRequestPath } from './catalogue-query';

// 本文件钉目录读口请求半边的编码（票 catalogue-read-pagination/05）。期望值取 ADR-0144 决定一、三、四的原意：同一组条件只有一种
// 写法，条件签名与请求出自同一个函数；夹具里的读口路径写端点表上实有的那两条（architecture 门禁扫 src 下全部 .ts，含测试）。

// Covers: 选择器、排序、筛选维、检索词依次编进查询串；键按名排、维内去重升序，次序与重复不改变含义（维内为或），同一组条件只有一种写法。
test('同一组条件只有一种写法：键按名排、维内去重升序', () => {
  const query = catalogueConditionsQuery({
    selectors: { family: 'connection' },
    sort: '-effectiveFrom',
    filters: { toNode: ['SYN-NODE-B'], fromNode: ['SYN-NODE-Z', 'SYN-NODE-A', 'SYN-NODE-Z'] },
    q: 'SYN',
  });
  equal(
    query,
    'family=connection&sort=-effectiveFrom&fromNode=SYN-NODE-A&fromNode=SYN-NODE-Z&toNode=SYN-NODE-B&q=SYN',
  );
  equal(
    catalogueConditionsQuery({
      q: 'SYN',
      filters: { fromNode: ['SYN-NODE-Z', 'SYN-NODE-A'], toNode: ['SYN-NODE-B'] },
      sort: '-effectiveFrom',
      selectors: { family: 'connection' },
    }),
    query,
  );
});

// Covers: 检索词去首尾空白、空白即缺席；空数组的维、空串的排序都不出现在查询串里（缺席即该册的缺省序与不筛）。
test('检索词去首尾空白、空即缺席；空维与空排序不出现', () => {
  equal(catalogueConditionsQuery({ q: '  SYN-ACCOUNT  ' }), 'q=SYN-ACCOUNT');
  equal(catalogueConditionsQuery({ q: '   ', sort: '', filters: { status: [] } }), '');
  equal(catalogueConditionsQuery({}), '');
});

// Covers: 中文、空格、& 与 = 这类字符按查询串编码，服务端解回的正是原值（检索词原样下推，不在前端改写）。
test('检索词里的中文与保留字符按查询串编码，解回原值', () => {
  const query = catalogueConditionsQuery({ q: '合成 客户&A=1' });
  deepEqual(new URLSearchParams(query).getAll('q'), ['合成 客户&A=1']);
});

// Covers: 没有条件、第一页 → 裸路径；带游标 → 末尾追加 after；去掉 after 之后的请求串就是条件签名——请求里带的每一维签名里都有。
test('请求路径带上条件与游标，去掉游标即条件签名', () => {
  equal(catalogueRequestPath('/commercial-customer-accounts', {}, null), '/commercial-customer-accounts');

  const conditions = { selectors: { family: 'node' }, q: 'SHA' };
  const first = catalogueRequestPath('/network-catalog', conditions, null);
  const second = catalogueRequestPath('/network-catalog', conditions, 'eyJjdXJzb3IiOjF9');
  equal(first, `/network-catalog?${catalogueConditionsQuery(conditions)}`);
  equal(second, `${first}&after=eyJjdXJzb3IiOjF9`);

  const params = new URLSearchParams(second.slice(second.indexOf('?') + 1));
  params.delete('after');
  equal(params.toString(), catalogueConditionsQuery(conditions));
});

// Covers: 检索词或筛选维有值即收窄（零行是「这组条件下没有」）；选择器、排序、空白检索词、空维都不收窄（零行才是册为空）。
test('检索词或筛选维有值才算收窄', () => {
  equal(catalogueConditionsNarrow({ q: 'SYN' }), true);
  equal(catalogueConditionsNarrow({ filters: { status: ['EFFECTIVE'] } }), true);
  equal(catalogueConditionsNarrow({ q: '  ', filters: { status: [] } }), false);
  equal(catalogueConditionsNarrow({ selectors: { family: 'node' }, sort: '-code' }), false);
});
