// 「我的工作」两页的对外出口，供装配侧接导航（票 admin-web-ux-alignment/02）。本目录装的不是限界上下文页：
// 两页读的都是本机浏览器的事实（打开过的对象地址、存下的筛选态），不发请求、不属任何上下文；纯逻辑（recent-objects.ts /
// saved-views.ts）故意不从这里桶导——外壳与工作台按路径引它们，页面件与存储逻辑分别被引，改一头不牵另一头。
export { RecentObjectsPage } from './RecentObjectsPage';
export { SavedViewsPage } from './SavedViewsPage';
