import { test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { spellAuthorizedDeliveryScope } from './authorized-delivery-scope';

// 本文件钉的是授权交付方适用范围只有一种拼法，且与服务端逐字一致。期望串的前三格取自票 10 后端
// TestAuthorizedDeliveryScopeHasOneSpelling 的用例原样；其余几格是 encodeURIComponent 与 Go url.QueryEscape 分道
// 的那些字符，期望串取 Go 的 url.Values.Encode 实跑结果。

// Covers: 只给客户账户——不带 node 键。
test('只给账户：customerAccount 一键', () => {
  deepEqual(spellAuthorizedDeliveryScope({ customerAccount: 'SYN-ACCOUNT-01', node: '' }), {
    ok: true,
    scope: 'customerAccount=SYN-ACCOUNT-01',
  });
});

// Covers: 账户加节点——键序固定 customerAccount 在前（Go 的 url.Values.Encode 按键字典序）。
test('账户加节点：customerAccount 在前、node 在后', () => {
  deepEqual(spellAuthorizedDeliveryScope({ customerAccount: 'SYN-ACCOUNT-01', node: 'SYN-NODE-SHA-HUB' }), {
    ok: true,
    scope: 'customerAccount=SYN-ACCOUNT-01&node=SYN-NODE-SHA-HUB',
  });
});

// Covers: 值按 Go url.QueryEscape 转义——分隔符、斜杠、空格写 `+`；`!'()*` 要转、`~` 不转（URLSearchParams 两边都
// 拼错）；非 ASCII 按 UTF-8 字节写大写 %XX；首尾空白不裁，照转。
test('需转义的值照 Go 的转义表逐字拼', () => {
  const spelled = (customerAccount: string, node: string) => spellAuthorizedDeliveryScope({ customerAccount, node });
  deepEqual(spelled('A&B=C', 'N/1 2'), { ok: true, scope: 'customerAccount=A%26B%3DC&node=N%2F1+2' });
  deepEqual(spelled("a!b'c(d)e*f~g", ''), { ok: true, scope: 'customerAccount=a%21b%27c%28d%29e%2Af~g' });
  deepEqual(spelled('合成账户', '节点 #1?%'), {
    ok: true,
    scope: 'customerAccount=%E5%90%88%E6%88%90%E8%B4%A6%E6%88%B7&node=%E8%8A%82%E7%82%B9+%231%3F%25',
  });
  deepEqual(spelled(' SYN-ACCOUNT-01 ', ''), { ok: true, scope: 'customerAccount=+SYN-ACCOUNT-01+' });
});

// Covers: 空账户被拒（空串与只有空白同判，服务端也这样判）；节点填了却只有空白被拒，不悄悄当成没填；编不成 UTF-8
// 的孤立代理码元被拒而不是抛。
test('空账户、空白节点与编不成 UTF-8 的值被拒', () => {
  const missing = { ok: false, problems: ['客户账户要填：授权交付方的适用范围至少收到一个客户账户'] };
  deepEqual(spellAuthorizedDeliveryScope({ customerAccount: '', node: '' }), missing);
  deepEqual(spellAuthorizedDeliveryScope({ customerAccount: '   ', node: '' }), missing);
  deepEqual(spellAuthorizedDeliveryScope({ customerAccount: '', node: 'SYN-NODE-SHA-HUB' }), missing);
  deepEqual(spellAuthorizedDeliveryScope({ customerAccount: 'SYN-ACCOUNT-01', node: ' ' }), {
    ok: false,
    problems: ['节点只填了空白：不收窄到节点就留空'],
  });
  deepEqual(spellAuthorizedDeliveryScope({ customerAccount: 'A\uD800', node: '\uDC00' }), {
    ok: false,
    problems: ['客户账户里有编不成 UTF-8 的字符', '节点里有编不成 UTF-8 的字符'],
  });
});
