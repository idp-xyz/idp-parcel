import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { boundTenantOf } from './operator-binding';

// 本文件钉操作者绑定答复的判读（票 group-legal-entities-uiux/11）：只有业务成格且带非空标识才算拿到了当前集团；其余各态
// （未配置、需要登录、没有授予、依赖故障、读不到、成格却缺标识）一律交回 undefined，顶栏作用域退回兜底句、页头不显集团——
// 不拿空串或别的什么顶上。

test('只有成格且带标识才答出租户', () => {
  equal(boundTenantOf({ kind: 'outcome', status: 200, body: { outcome: 'OPERATOR_BINDING_RESOLVED', tenantId: 'SYN-TENANT-01' } }), 'SYN-TENANT-01');
  equal(boundTenantOf({ kind: 'outcome', status: 200, body: { outcome: 'OPERATOR_BINDING_RESOLVED', tenantId: '  ' } }), undefined);
  equal(boundTenantOf({ kind: 'outcome', status: 200, body: { outcome: 'SOMETHING_ELSE', tenantId: 'SYN-TENANT-01' } }), undefined);
  equal(boundTenantOf({ kind: 'unconfigured' }), undefined);
  equal(boundTenantOf({ kind: 'loginRequired' }), undefined);
  equal(boundTenantOf({ kind: 'notGranted' }), undefined);
  equal(boundTenantOf({ kind: 'noAnswer', status: 503, code: 'IDENTITY_DEPENDENCY_UNAVAILABLE' }), undefined);
  equal(boundTenantOf({ kind: 'transport', message: 'ECONNREFUSED' }), undefined);
  equal(boundTenantOf(null), undefined);
});
