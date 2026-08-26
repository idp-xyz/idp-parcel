// 取「非默认页签」的 DOM。dom-dump.sh 用的 --dump-dom 只能取首屏，而页签是点出来的，
// 于是那种页的新增列在无头产物里永远看不见——这不是页面没接上，是取证手段够不着。
//
// 这里走 DevTools 协议：开无头 Edge、按可见文字点一次页签、等它取完数再抓 DOM。
// 只用 node 内置的 WebSocket（node 22 起是全局），不引依赖——取证脚本自己拖一棵依赖树
// 会让「取证环境坏了」和「被测页坏了」难分。
//
// 用法：node dom-dump-tab.mjs <url> <页签文字> <产物路径>

import { execFileSync, spawn } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const EDGE = '/mnt/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe';
const [url, tabText, outPath] = process.argv.slice(2);
if (!url || !tabText || !outPath) {
  console.error('用法：node dom-dump-tab.mjs <url> <页签文字> <产物路径>');
  process.exit(2);
}

const profile = mkdtempSync(join(tmpdir(), 'parcel-dom-'));
const winProfile = execFileSync('wslpath', ['-w', profile]).toString().trim();

// 调试端口每次随机取。固定端口会让上一轮**没死干净**的 Edge 继续占着它，于是这一轮
// 连上的是那个旧实例的空白页——Page.navigate 打在别人身上，本轮的求值对着空文档跑，
// 症状是「一个按钮都没有」，与页面真的没渲染无从分辨。踩过一次，现场留了 59 个残留
// 进程。收尾那段的 Browser.close 是同一个问题的另一半。
const port = 9300 + Math.floor(Math.random() * 400);

const edge = spawn(EDGE, [
  '--headless=new',
  '--disable-gpu',
  '--no-sandbox',
  `--remote-debugging-port=${port}`,
  `--user-data-dir=${winProfile}`,
  'about:blank',
]);
edge.stderr.on('data', () => {});

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

// 接的是浏览器自带的那张 about:blank 页，再用 Page.navigate 走过去。不用
// /json/new?url 开新页：那条路开出来的 target 在本机 Edge 上停在空白页，
// Runtime.evaluate 于是对着一张什么都没有的文档求值，症状是「一个按钮都没有」，
// 看起来像页面渲染失败。
async function targetWebSocketURL() {
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/list`);
      const targets = await response.json();
      const page = targets.find((target) => target.type === 'page' && target.webSocketDebuggerUrl);
      if (page) return page.webSocketDebuggerUrl;
    } catch {
      // 浏览器还没起监听，退避重试。
    }
    await sleep(250);
  }
  throw new Error('DevTools 端点未就绪');
}

function rpc(socket) {
  let nextID = 0;
  const pending = new Map();
  socket.addEventListener('message', (event) => {
    const message = JSON.parse(event.data);
    const resolve = pending.get(message.id);
    if (resolve) {
      pending.delete(message.id);
      resolve(message.result);
    }
  });
  return (method, params = {}) => {
    const id = (nextID += 1);
    return new Promise((resolve) => {
      pending.set(id, resolve);
      socket.send(JSON.stringify({ id, method, params }));
    });
  };
}

const evaluate = (call, expression) =>
  call('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });

try {
  const socket = new WebSocket(await targetWebSocketURL());
  await new Promise((resolve) => socket.addEventListener('open', resolve));
  const call = rpc(socket);
  await call('Page.enable');
  await call('Runtime.enable');
  await call('Page.navigate', { url });

  // 轮询等页签出现，不用固定等待。开发服首次加载要现场转译整棵模块树，冷启动
  // 常常几秒都不止，而固定等待要么定得太短（点不到，看起来像页面没接上），要么
  // 一律定得很长。轮询两头都省。
  const clickTab = `(() => {
     const buttons = [...document.querySelectorAll('button')];
     const button = buttons.find(
       (element) => element.textContent.trim() === ${JSON.stringify(tabText)},
     );
     if (!button) return buttons.map((element) => element.textContent.trim());
     button.click();
     return true;
   })()`;

  let clicked = false;
  let seenLabels = [];
  for (let attempt = 0; attempt < 30 && !clicked; attempt += 1) {
    await sleep(1000);
    const outcome = await evaluate(call, clickTab);
    if (outcome.result?.value === true) clicked = true;
    else seenLabels = outcome.result?.value ?? [];
  }
  if (!clicked) {
    throw new Error(
      `页面上找不到写着「${tabText}」的页签；当前按钮有：${JSON.stringify(seenLabels)}`,
    );
  }

  // 换页签会重新发请求，再等一轮。
  await sleep(3000);

  const dom = await evaluate(call, 'document.documentElement.outerHTML');
  writeFileSync(outPath, dom.result.value);
  console.log(`${tabText} -> ${dom.result.value.length} chars -> ${outPath}`);

  // Browser.close 而不是只 kill 子进程：Windows 侧的 msedge.exe 会另起一整棵进程树，
  // 从 WSL 杀掉那个启动壳只是让它变成孤儿，浏览器本体照常活着并占着调试端口。
  await call('Browser.close');
  socket.close();
} finally {
  edge.kill();
  rmSync(profile, { recursive: true, force: true });
}
