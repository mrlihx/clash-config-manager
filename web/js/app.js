import {mountYamlEditor} from './yaml-editor.js';
import { $, button, checkbox, esc, field, heading, help, icon, input, modal, panel, select, table, toast } from './ui.js';
import { groupEditor, mergeEditor, nodeEditor, providerEditor, ruleEditor } from './editors.js';
import { animated, applyTheme, sidebarDark, toggleDark } from './theme.js';

const titles = {
  profiles: ['配置文件','导入订阅或配置文件，管理节点来源。','file'],
  proxies: ['节点管理','展开配置，勾选节点后生成订阅地址。','server'],
  'proxy-groups': ['策略组','维护组成员、自动测速和选择策略。','layers'],
  rules: ['分流规则','按实际匹配顺序查看规则，支持 IPv4 和 IPv6 分流。','layers'],
  'rule-providers': ['规则集','管理规则来源和更新间隔；更新时间来自本系统缓存，不代表路由器缓存。','file'],
  backups: ['备份管理','生成、上传和恢复数据库备份。恢复前会自动保存当前数据。','file'],
  subscription: ['订阅管理','管理已发布订阅、规则分流和订阅地址。','link'],
};
const labels = {
  proxies: '新增节点', 'proxy-groups':'新增策略组', rules:'新增规则',
  'rule-providers':'新增规则集',
};
let profiles = [];
let routingPlans=[];
let routingPlanID=sessionStorage.getItem('routing-plan') || 'system-routing-template';
const templatePath=()=>'/api/system-template?id='+encodeURIComponent(routingPlanID);
let profile;
let nodeGroups = [];
let subscriptions = [];
let backups = [];
let appVersion;
const selectedNodes = new Set();
const expandedProfiles = new Set();
const routingPages = new Set(['proxy-groups','rules','rule-providers']);
let active = 'profiles';
let search = '';
let page = 1;
const size = 15;

async function api(path, body, method = 'POST') {
  const response = await fetch(path, body === undefined ? {} : {
    method,
    headers: {'Content-Type':'application/json'},
    body: JSON.stringify(body),
  });
  const data = await response.json();
  if (!response.ok) {
    if (response.status === 401 && path !== '/api/login') showLogin();
    throw new Error(data.error || '请求失败');
  }
  return data;
}
const profilePath = () => '/api/profiles/' + encodeURIComponent(profile.id);
const request = command => api(profilePath(), {revision:profile.revision,...command});

async function refresh(id = '') {
  [profiles,nodeGroups,subscriptions,routingPlans] = await Promise.all([api('/api/profiles'),api('/api/node-groups'),api('/api/profiles?kind=subscription'),api('/api/routing-plans')]);
  if(!routingPlans.some(plan=>plan.id===routingPlanID))routingPlanID='system-routing-template';
  for(const key of selectedNodes){const [id,name]=JSON.parse(key);if(!nodeGroups.some(group=>group.id===id&&group.nodes.some(node=>node.name===name)))selectedNodes.delete(key);}
  const current = id || (profile?.kind!=='template' ? profile?.id : '') || sessionStorage.getItem('profile-id');
  const selected = [...profiles,...subscriptions].find(item => item.id === current) ?? profiles[0] ?? subscriptions[0];
  profile = selected ? await api('/api/profiles/' + encodeURIComponent(selected.id)) : undefined;
  if (routingPages.has(active)) profile=await api(templatePath());
  if (active==='backups') backups=await api('/api/backups');
  if (profile && profile.kind!=='template') sessionStorage.setItem('profile-id',profile.id);
  render();
}

function showLogin() {
  profile = undefined;
  profiles = [];
  $('#app').hidden = true;
  $('#login').hidden = false;
  if ($('#modal').open) $('#modal').close();
}
async function showApp() {
  $('#login').hidden = true;
  $('#app').hidden = false;
  active = Object.hasOwn(titles, location.hash.slice(1)) ? location.hash.slice(1) : 'profiles';
  appVersion=await api('/api/version');
  $('#appVersion').innerHTML=`<span>版本 ${esc(appVersion.version==='dev'?'开发版':appVersion.version.slice(0,12))}</span>${button('检查更新','check-update',{variant:'ghost'})}`;
  await refresh();
  renderItems();
}

function nav() {
  $('#navigation').innerHTML = [['配置管理',['profiles','proxies','proxy-groups']],['路由策略',['rules','rule-providers']],['发布管理',['subscription']],['系统管理',['backups']]].map(([title,keys]) => `<div class="space-y-1"><h4 class="px-3 text-[11px] font-bold uppercase tracking-wider mb-2 text-slate-400 dark:text-slate-500">${title}</h4>${keys.map(key => {
    const selected = key === active;
    const dark = sidebarDark();
    const tone = selected ? (dark ? 'bg-primary-600 text-white font-semibold shadow-xs' : 'bg-primary-50 text-primary-700 dark:bg-primary-950/60 dark:text-primary-300 font-semibold shadow-xs') :
      (dark ? 'text-slate-300 hover:bg-white/10 hover:text-white' : 'text-slate-600 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-slate-800/60 hover:text-slate-900 dark:hover:text-slate-200');
    return `<a href="#${key}" ${selected ? 'aria-current="page"' : ''} class="flex items-center justify-between px-3 py-2 rounded-xl text-xs font-medium transition-all duration-150 group ${tone}"><span class="flex items-center gap-2.5">${icon(titles[key][2],`h-4 w-4 shrink-0 transition-colors ${selected && !dark ? 'text-primary-600 dark:text-primary-400' : 'text-slate-400'}`)}${titles[key][0]}</span></a>`;
  }).join('')}</div>`).join('');
}

function closeSidebar(force) {
  const sidebar = $('#sidebar');
  const show = force ?? sidebar.classList.contains('-translate-x-full');
  sidebar.classList.toggle('-translate-x-full',!show);
  sidebar.classList.toggle('translate-x-0',show);
  $('#sidebarOverlay').hidden = !show;
  $('[data-action="sidebar"]').setAttribute('aria-expanded',String(show));
}

function render() {
  nav();
  $('#headerControls').innerHTML = button('', 'toggle-dark', {symbol:document.documentElement.classList.contains('dark') ? 'sun':'moon', variant:'ghost',title:'切换明暗模式'}) +
    button('修改密码','change-password',{variant:'ghost'}) + button('退出','logout',{variant:'ghost'});
  let actions = '';
  if (active==='backups') actions=button('上传备份','upload-backup',{symbol:'file'})+button('立即备份','create-backup',{variant:'primary',symbol:'plus'});
  else if (active === 'profiles') actions = button('导入 / 新建','create',{variant:'primary',symbol:'plus'});
  else if (active === 'proxies') actions = button(`生成订阅地址${selectedNodes.size ? '（'+selectedNodes.size+'）' : ''}`, 'create-subscription',{variant:'primary',symbol:'link'});
  else if (active !== 'proxies' && profile && labels[active]) actions = button(labels[active], 'add-item',{variant:'primary',symbol:'plus'});
  const [title,description] = titles[active];
  const block = active === 'backups' ? backupsPage() : active === 'proxies' ? nodeGroupsPage() : active === 'profiles' ? profileList() : !profile ? `<div class="${panel}"><p class="${help}">先导入或新建一份配置。</p>${button('导入 / 新建','create',{variant:'primary'})}</div>` : pageContent();
  const content = $('#content');
  content.innerHTML = `<div class="space-y-6 w-full ${animated() ? 'page-enter' : ''}">${heading(title,description,actions)}${routingPages.has(active)?`<div class="${panel}"><div class="flex flex-wrap items-end gap-3"><div class="w-full sm:w-72">${select('规则方案','routing-plan',routingPlans.map(plan=>[plan.id,plan.name]),routingPlanID)}</div>${button('新增方案','create-plan',{symbol:'plus'})}${button('重命名','rename-plan')}${routingPlanID!=='system-routing-template'?button('删除方案','delete-plan'):''}</div><p class="${help} mt-2">修改后自动更新关联订阅；订阅专属覆盖保留，客户端需刷新订阅。</p></div>`:''}${block}</div>`;
}

function nodeGroupsPage() {
  if (!nodeGroups.length) return `<div class="${panel}"><p class="${help}">先导入节点配置。</p>${button('导入 / 新建','create',{variant:'primary'})}</div>`;
  return `<div class="node-source-list"><div class="node-list-hint">左右滑动查看完整节点信息</div><div class="node-source-scroll"><table class="node-table"><thead><tr><th class="node-check-cell">选择</th><th>节点名称</th><th>协议</th><th>服务器</th><th class="node-actions-cell">操作</th></tr></thead><tbody>${nodeGroups.map(group=>{
    const expanded=expandedProfiles.has(group.id);
    const selected=group.nodes.filter(node=>selectedNodes.has(JSON.stringify([group.id,node.name]))).length;
    const source=`<tr class="node-source-row" data-source-row="${esc(group.id)}"><td colspan="5"><div class="node-source-heading"><button type="button" data-action="toggle-source" data-key="${esc(group.id)}" aria-expanded="${expanded}" class="node-source-toggle"><span class="node-source-chevron">${icon('chevron')}</span><span class="node-source-icon">${icon('file')}</span><span class="node-source-name">${esc(group.name)}</span></button><span class="node-source-count">${group.nodes.length} 个节点</span>${selected?`<span class="node-selection-count">已选 ${selected}</span>`:''}<span class="node-source-add">${button('新增节点','group-add',{key:group.id,symbol:'plus',variant:'ghost'})}</span></div></td></tr>`;
    if(!expanded)return source;
    if(!group.nodes.length)return source+`<tr><td colspan="5" class="node-empty">暂无节点，点击“新增节点”开始添加。</td></tr>`;
    return source+`<tr class="node-selection-bar"><td colspan="5"><div>${checkbox('全选本配置','select-profile-nodes',group.id,selected===group.nodes.length)}<span>${selected} / ${group.nodes.length} 已选择</span></div></td></tr>`+
      group.nodes.map(node=>`<tr class="node-row ${selectedNodes.has(JSON.stringify([group.id,node.name]))?'node-row-selected':''}">
        <td class="node-check-cell">${checkbox('','selected-node',JSON.stringify([group.id,node.name]),selectedNodes.has(JSON.stringify([group.id,node.name]))).replace('<input ',`<input aria-label="${esc('选择 '+node.name)}" `)}</td>
        <td><div class="node-name"><span class="node-name-icon">${icon('server')}</span><span title="${esc(node.name)}">${esc(node.name)}</span></div></td>
        <td><span class="node-protocol">${esc(node.type==='ss'?'SS':node.type)}</span></td>
        <td><span class="node-endpoint" title="${esc(node.server)}">${esc(node.server ?? '—')}</span><span class="node-port">:${esc(node.port ?? '—')}</span></td>
        <td class="node-actions-cell"><div>${button('编辑','group-edit',{key:JSON.stringify([group.id,node.name]),variant:'ghost',symbol:'edit'})}${button('删除','group-delete',{key:JSON.stringify([group.id,node.name]),variant:'ghost'})}</div></td></tr>`).join('');
  }).join('')}</tbody></table></div></div>`;
}

function profileList() {
  return table(['配置名称','节点数','自动更新','上次更新时间（北京时间）','操作'],profiles.map(item=>[
    `<a href="#proxies" data-action="open-profile" data-key="${esc(item.id)}" class="font-semibold text-primary-600 hover:underline">${esc(item.name)}</a>`,
    String(nodeGroups.find(group=>group.id===item.id)?.nodes.length ?? 0),
    `<span class="text-xs">${item.updateAt ? '每天 '+esc(item.updateAt) : item.updateMinutes ? '每 '+item.updateMinutes+' 分钟' : '未开启'}</span>${item.updateError ? `<p class="text-xs text-rose-600 mt-1">${esc(item.updateError)}</p>` : ''}`,
    item.lastUpdate > 0 ? esc(new Date(item.lastUpdate*1000).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',hour12:false})) : '—',
    `<div class="flex gap-2">${button('编辑','edit-source',{key:item.id,variant:'ghost'})}${item.hasSource ? button('立即更新','refresh-source',{key:item.id,variant:'ghost'}) : ''}${button('删除','delete-source',{key:item.id,variant:'ghost'})}</div>`,
  ]));
}

async function sourceEditor(id) {
  const source=await api('/api/profiles/'+encodeURIComponent(id));
  modal('编辑来源配置',field('配置名称','name',source.name,'text','required')+
    field('订阅链接','sourceURL',source.sourceURL,'url')+field('上传文件（替换节点）','file','','file')+
    checkbox('自动更新来源订阅','auto-update','1',Boolean(source.updateMinutes || source.updateAt))+
    select('更新方式','update-mode',[['interval','按间隔更新'],['daily','每天指定时间']],source.updateAt ? 'daily' : 'interval')+
    `<div id="updateInterval">${field('更新间隔（分钟）','update-minutes',source.updateMinutes || 1440,'number','min="1" max="525600"')}</div>`+
    `<div id="updateDaily">${field('每日更新时间（北京时间）','update-at',source.updateAt || '06:00','time')}</div>`+
    `<p class="${help}">来源更新后同步到已关联订阅；订阅地址和专属规则保留。</p>`,
  async values=>{
    const file=values.get('file');
    await api('/api/sources/'+encodeURIComponent(id),{revision:source.revision,name:values.get('name'),url:values.get('sourceURL'),yaml:file?.size ? await file.text() : '',autoUpdate:values.has('auto-update'),mode:values.get('update-mode'),updateMinutes:Number(values.get('update-minutes')),updateAt:values.get('update-at')});
    await refresh(id);renderItems();toast('来源配置已保存');
  },{confirm:'保存配置'});
  const mode=$('[name="update-mode"]',$('#modal'));
  const showSchedule=()=>{$('#updateInterval').hidden=mode.value!=='interval';$('#updateDaily').hidden=mode.value!=='daily';};
  mode.addEventListener('change',showSchedule);showSchedule();
}

function subscriptionsPage() {
  const items=[...subscriptions,...profiles].filter(item=>item.published);
  return table(['订阅名称','订阅后缀','规则方案','同步状态','操作'],items.map(item=>[
    esc(item.name),esc(item.slug),esc(routingPlans.find(plan=>plan.id===(item.routingPlan || 'system-routing-template'))?.name ?? '系统默认规则'),item.updateError ? `<span class="text-xs text-rose-600">${esc(item.updateError)}</span>` : item.followRouting===false?'独立配置':'跟随方案，客户端刷新生效',`<div class="flex flex-wrap gap-2">${button('编辑配置','edit-subscription',{key:item.id,variant:'ghost',symbol:'edit'})}${button('订阅地址','subscription-address',{key:item.id,variant:'ghost',symbol:'link'})}${button('删除','delete-subscription',{key:item.id,variant:'ghost'})}</div>`,
  ]));
}

function backupsPage() {
  const rows=backups.map(item=>[
    `<span class="font-mono text-xs">${esc(item.id)}</span>`,
    esc(item.source),
    esc(item.size<1048576 ? (item.size/1024).toFixed(1)+' KB' : (item.size/1048576).toFixed(1)+' MB'),
    esc(new Date(item.createdAt).toLocaleString()),
    `<div class="flex gap-1.5">${button('下载','download-backup',{key:item.id,variant:'ghost'})}${button('导入恢复','restore-backup',{key:item.id,variant:'ghost'})}${button('删除','delete-backup',{key:item.id,variant:'ghost'})}</div>`,
  ]);
  return `<div class="${panel}"><p class="${help}">备份包含节点、规则方案和订阅。导入会覆盖当前数据，并要求重新登录；管理员密码保持不变。上传文件不会立即恢复。</p><p class="${help}">共 ${backups.length} 份备份 · 支持本系统导出的 .db 文件，最大 128 MB</p></div>${table(['备份文件','来源','大小','时间','操作'],rows)}`;
}

async function refreshBackups() {
  backups=await api('/api/backups');
  renderAll();
}

function pageContent() {
  if (active === 'subscription') return subscriptionsPage();
  return `${active === 'rules' ? `<div class="${panel}"><div class="flex flex-wrap items-center gap-2">${button('选入外部规则','merge-rules',{symbol:'file'})}${button('新增域名例外','rule-exception',{symbol:'plus'})}</div><p class="${help}">${'规则按列表顺序匹配，每条规则均可编辑、删除和调整顺序。'}</p></div>` : ''}<div class="${panel}"><label class="block max-w-md"><span class="sr-only">搜索当前列表</span><input id="search" class="${input}" type="search" placeholder="搜索当前列表..." value="${esc(search)}"></label><p id="count" class="${help}"></p></div>
    <div id="items" class="space-y-4"></div>`;
}

function editActions(key, index) {
  return `<div class="flex gap-1.5">${button('编辑','edit-item',{key,variant:'ghost'})}
    ${index === undefined ? '' : button('排序','move-rule',{key,variant:'ghost'})}${button('删除','delete-item',{key,variant:'ghost'})}</div>`;
}

function renderItems() {
  if (!$('#items') || !profile) return;
  const data = profile.data ?? {};
  let headers, rows;
  if (active === 'proxies') {
    headers = ['节点名称','协议','服务器','前置代理','操作'];
    rows = (data.proxies ?? []).map(node => [node.name,node.type,`${node.server ?? '—'}:${node.port ?? '—'}`,node['dialer-proxy'] ?? '—',editActions(node.name)]);
  } else if (active === 'proxy-groups') {
    headers = ['策略组','类型','成员','操作'];
    rows = (data['proxy-groups'] ?? []).map(group => [group.name,group.type,(group.proxies ?? []).join('、'),editActions(group.name)]);
  } else if (active === 'rules') {
    headers = ['顺序','Mihomo 规则','操作'];
    rows = (data.rules ?? []).map((rule,i) => [String(i+1),rule, editActions(String(i),i)]);
  } else {
    headers = ['名称','来源','匹配类型','格式','目标策略','更新间隔','上次更新时间（北京时间）','操作'];
    rows = Object.entries(data[active] ?? {}).map(([name,item]) => [name,item.type==='inline'?'手动文本':item.type,item.behavior ?? '—',item.format ?? 'yaml',(data.rules ?? []).find(rule=>rule.startsWith('RULE-SET,'+name+','))?.split(',')[2] ?? '未引用',item.type === 'http' ? (item.interval || 86400)+' 秒' : '手动维护',profile.ruleProviderUpdated?.[name] ? new Date(profile.ruleProviderUpdated[name]*1000).toLocaleString('zh-CN',{timeZone:'Asia/Shanghai',hour12:false})+(profile.ruleProviderStale?.[name]?'（更新失败，沿用缓存）':'') : item.type==='inline'?'—':'未缓存',editActions(name)]);
  }
  rows = rows.filter(row => row.slice(0,-1).some(value => String(value).toLowerCase().includes(search.toLowerCase())));
  const pages = Math.max(1,Math.ceil(rows.length/size));
  page = Math.min(page,pages);
  $('#count').textContent = `共检索到 ${rows.length} 条匹配记录`;
  $('#items').innerHTML = table(headers,rows.slice((page-1)*size,page*size).map(row=>row.map((value,i)=>i===row.length-1 ? value : esc(value)))) +
    `<div class="flex items-center justify-end gap-2">${page>1 ? button('上一页','prev') : ''}<span class="${help}">${page} / ${pages}</span>${page<pages ? button('下一页','next') : ''}</div>`;
}

function subscriptionQR(url) {
  return `<div class="flex justify-center mb-4"><img src="/api/subscription-qr?url=${esc(encodeURIComponent(url))}" width="256" height="256" alt="订阅地址二维码" class="rounded-lg" style="background:white;max-width:100%;height:auto"></div>`;
}

function subscriptionLink(url) {
  return `<div class="flex items-end gap-2"><div class="flex-1 min-w-0">${field('订阅地址','url',url,'text','readonly id="subURL"')}</div>${button('复制链接','copy-link',{symbol:'link'})}</div>`;
}

async function subscriptionAddress(id) {
  await selectProfile(id);
  const url=new URL(profile.subscription,location.origin).href;
  const form=modal('订阅地址设置',subscriptionQR(url)+subscriptionLink(url)+
    `<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">${field('订阅名称','subscription-name',profile.name,'text','required')}${field('客户端更新间隔（小时）','client-update-hours',profile.clientUpdateHours||24,'number','min="1" max="720" required')}</div>`+
    `<p class="${help}">支持的客户端会自动读取名称与更新间隔；部分 APP 仍需手动开启自动更新。</p>`+
    `<div class="flex items-end gap-2"><div class="flex-1 min-w-0">${field('自定义订阅后缀','slug',profile.slug,'text','pattern="[A-Za-z0-9][A-Za-z0-9_\\-]{0,63}" placeholder="留空随机生成"')}</div>${button('随机','random-slug')}</div>`+
    checkbox('启用外部资源加速','resource-proxy-enabled','1',Boolean(profile.resourceProxyPrefix))+
    `<div id="resourceProxySettings">${field('GitHub 加速地址前缀','resource-proxy-prefix',profile.resourceProxyPrefix||'https://gh-proxy.org/','url','placeholder="https://加速服务地址/"')}<p class="${help}">仅转换公开 GitHub 规则文件的下载地址，不改动节点或私人订阅。加速速度取决于服务和线路。</p></div>`+
    checkbox('启用订阅','enabled','1',profile.enabled)+
    `<p class="${help}">更换后缀后，旧地址失效。修改名称、更新间隔或加速设置不会更换订阅地址。</p>`,
  async values=>{
    await update({action:'subscription',name:values.get('subscription-name').trim(),clientUpdateHours:Number(values.get('client-update-hours')),resourceProxyPrefix:values.has('resource-proxy-enabled')?values.get('resource-proxy-prefix').trim():'',slug:values.get('slug').trim(),enabled:values.has('enabled')});
    const nextURL=new URL(profile.subscription,location.origin).href;
    modal('订阅地址已保存',subscriptionQR(nextURL)+subscriptionLink(nextURL),null);
    return false;
  },{confirm:'保存设置'});
  const toggleAcceleration=()=>{
    const enabled=form.querySelector('[name=resource-proxy-enabled]').checked;
    form.querySelector('#resourceProxySettings').hidden=!enabled;
    const input=form.querySelector('[name=resource-proxy-prefix]');input.disabled=!enabled;input.required=enabled;
  };
  form.querySelector('[name=resource-proxy-enabled]').addEventListener('change',toggleAcceleration);
  toggleAcceleration();
}

function renderAll() {
  render();
  renderItems();
}

async function update(command, message) {
  profile = await request(command);
  [nodeGroups,subscriptions,routingPlans] = await Promise.all([api('/api/node-groups'),api('/api/profiles?kind=subscription'),api('/api/routing-plans')]);
  if (['subscription','rename'].includes(command.action)) profiles = await api('/api/profiles');
  else {
    const row = profiles.find(item => item.id === profile.id);
    if (row) Object.assign(row,{published:profile.published,dirty:profile.dirty});
  }
  renderAll();
  if (message) toast(message);
}

async function preview(command) {
  const result = await request({...command, preview:true});
  if (!result.diff) { toast('配置没有变更'); return; }
  const revision = profile.revision;
  modal('确认变更', diffBody(result.diff), async () => {
    await update({...command,revision},profile.kind==='subscription' ? '订阅已更新' : profile.kind==='template' ? '规则方案已保存' : '配置已保存');
  }, {confirm:'保存配置',description:profile.kind==='subscription' ? '校验通过后更新订阅。' : profile.kind==='template' ? '保存后自动更新关联订阅；专属覆盖保留。' : '确认后保存配置。'});
  return false;
}

function diffBody(diff) {
  return diff ? `<pre class="max-h-[60vh] overflow-auto text-xs leading-relaxed rounded-lg bg-slate-50 dark:bg-slate-950 p-3">${diff.split('\n').map(line=>`<span class="${line.startsWith('+') ? 'text-emerald-700 dark:text-emerald-400' : line.startsWith('-') ? 'text-rose-700 dark:text-rose-400' : ''}">${esc(line)}</span>`).join('\n')}</pre>` : `<p class="${help}">配置没有变更</p>`;
}

function ruleTargets(){return [...(profile.data?.['proxy-groups'] ?? []).map(g=>g.name),'DIRECT','REJECT'];}

function itemEditor(key, exception = false) {
  const data = profile.data ?? {};
  if (active === 'proxies') nodeEditor(data,key,api,preview);
  else if (active === 'proxy-groups') groupEditor(data,key,preview,profile.preset && profile.systemGroups.includes(key),profile.kind==='template');
  else if (active === 'rules' || exception) ruleEditor(data,key,preview,exception);
  else providerEditor(active,data,key,preview);
}

function create() {
  modal('导入 / 新建配置',
    field('配置名称','name','', 'text','required') +
    field('订阅链接','sourceURL','','url','autocomplete="off" placeholder="https://…"') +
    field('上传文件','file','','file'),
  async form => {
    const file=form.get('file');
    const yaml=file?.size ? await file.text() : '';
    const next=await api('/api/profiles',{name:form.get('name'),sourceURL:form.get('sourceURL').trim(),yaml});
    await refresh(next.id);
    location.hash='proxies';
    renderItems();
    toast('配置已导入');
  },{confirm:'导入'});
}

function subscriptionChoices(options,data=null) {
  const rules=data?.rules ?? [];
  return `<fieldset class="space-y-3"><legend class="block text-xs font-semibold">加入的规则集</legend><div class="subscription-options">${options.ruleSets.map(item=>{
    const selected=(data===null?(options.defaultRuleSets??[]).includes(item.id):false) || item.required || rules.some(rule=>rule.startsWith('RULE-SET,'+item.id+',')||rule.startsWith('RULE-SET,System_'+item.id+','));
    return checkbox(item.name,'ruleSet',item.id,selected).replace('<input ',`<input ${item.required?'disabled ':''}`);
  }).join('')}</div></fieldset><fieldset class="space-y-3"><legend class="block text-xs font-semibold">海外服务分流组</legend><div class="subscription-options">${options.services.map(item=>checkbox(item.name,'service',item.id,data===null?(options.defaultServices??[]).includes(item.id):(data['proxy-groups'] ?? []).some(group=>group.name===item.name))).join('')}</div></fieldset>`;
}

async function createSubscription() {
  if (!selectedNodes.size) throw new Error('请先展开配置并勾选节点');
  let options=await api('/api/subscription-options?plan='+encodeURIComponent(routingPlanID));
  const dialog=modal('生成订阅地址',
    select('规则方案','subscription-plan',routingPlans.map(plan=>[plan.id,plan.name]),routingPlanID)+
    checkbox('跟随规则方案更新','follow-routing','1',true)+
    `<p class="${help}">已选 ${selectedNodes.size} 个节点。方案修改后自动更新此订阅，专属修改保留。</p><div id="createSubscriptionChoices" class="space-y-4">${subscriptionChoices(options)}</div>`,
    async form=>{
      const selection=new Map();
      for(const key of selectedNodes){const [id,name]=JSON.parse(key);if(!selection.has(id))selection.set(id,[]);selection.get(id).push(name);}
      const generated=await api('/api/subscriptions',{selections:[...selection].map(([id,nodes])=>({id,nodes})),services:form.getAll('service'),ruleSets:form.getAll('ruleSet'),routingPlan:form.get('subscription-plan'),followRouting:form.has('follow-routing')});
      await refresh(generated.id);
      await subscriptionAddress(generated.id);
      return false;
    },{wide:true,confirm:'生成订阅地址'});
  dialog.querySelector('[name=subscription-plan]').addEventListener('change',async event=>{
    const save=dialog.querySelector('button[type=submit]');save.disabled=true;
    try{options=await api('/api/subscription-options?plan='+encodeURIComponent(event.target.value));dialog.querySelector('#createSubscriptionChoices').innerHTML=subscriptionChoices(options);}
    catch(err){toast(err.message,true);}finally{save.disabled=false;}
  });
}

async function editSubscription(id) {
  await selectProfile(id);
  let options=await api('/api/subscription-options?plan='+encodeURIComponent(profile.routingPlan || 'system-routing-template'));
  let draft=profile.yaml;
  let mode='routing';
  let dirty=false;
  const choices=subscriptionChoices(options,profile.data ?? {});
  const applyChoices=data=>{
    const rules=data.rules ?? [];
    for(const item of options.ruleSets){
      const key=item.id==='Ads'?'Ads':item.id;
      form.querySelector(`[name=ruleSet][value="${CSS.escape(item.id)}"]`).checked=item.required || rules.some(rule=>rule.startsWith('RULE-SET,'+key+',') || rule.startsWith('RULE-SET,System_'+key+','));
    }
    for(const item of options.services){form.querySelector(`[name=service][value="${CSS.escape(item.id)}"]`).checked=(data['proxy-groups'] ?? []).some(group=>group.name===item.name);}
  };
  const syncRouting=async()=>{
    if(!dirty)return;
    const values=new FormData(form);
    const result=await request({action:'preview-subscription-routing',routingPlan:form.querySelector('[name=subscription-plan]').value,yaml:draft,services:values.getAll('service'),ruleSets:values.getAll('ruleSet')});
    draft=result.yaml;
    applyChoices(result.data);
    dirty=false;
  };
  const form=modal('编辑订阅配置',
    select('规则方案','subscription-plan',routingPlans.map(plan=>[plan.id,plan.name]),profile.routingPlan || 'system-routing-template')+
    checkbox('跟随规则方案更新','follow-routing','1',profile.followRouting!==false)+
    `<p class="${help}">手工修改的 YAML 字段作为订阅专属覆盖保留。${profile.routingOverrides?.length?'自定义区段：'+esc(profile.routingOverrides.join('、'))+'。':''}${profile.syncError?esc(profile.syncError):''}</p>`+
    `<fieldset id="subscriptionMode" class="space-y-2"><legend class="block text-xs font-semibold">编辑方式</legend><div class="editor-mode-cards">
      <label class="editor-mode-card"><input class="sr-only" type="radio" name="edit-mode" value="routing" checked><span class="editor-mode-title">${icon('layers','h-4 w-4')}规则集与分流组</span><span class="editor-mode-description">勾选规则集与海外服务分组</span></label>
      <label class="editor-mode-card"><input class="sr-only" type="radio" name="edit-mode" value="yaml"><span class="editor-mode-title">${icon('file','h-4 w-4')}代码编辑</span><span class="editor-mode-description">直接编辑完整 YAML 配置</span></label>
    </div></fieldset>`+
    `<p class="${help}">两种方式编辑同一份草稿，切换时同步，保存后更新原订阅地址。</p><div id="subscriptionRouting" class="space-y-4">${choices}</div>
    <div id="subscriptionYAML" hidden><div data-yaml-editor class="yaml-editor"></div></div>`,
    async values=>{
      if(mode==='yaml')draft=values.get('yaml');
      else await syncRouting();
      await update({action:'save-subscription',yaml:draft,routingPlan:values.get('subscription-plan'),followRouting:values.has('follow-routing')},'订阅配置已更新');
    },{wide:true,confirm:'保存配置',description:'勾选规则与分组，或直接编辑带高亮的 YAML。'});
  applyChoices(profile.data ?? {});
  let selectedPlan=profile.routingPlan || 'system-routing-template';
  form.querySelector('[name=subscription-plan]').addEventListener('change',async event=>{
    const save=form.querySelector('button[type=submit]');save.disabled=true;event.target.disabled=true;
    try{
      if(mode==='yaml')draft=form.querySelector('[name=yaml]').value;
      const parsed=await request({action:'inspect-subscription',yaml:draft});
      options=await api('/api/subscription-options?plan='+encodeURIComponent(event.target.value));
      form.querySelector('#subscriptionRouting').innerHTML=subscriptionChoices(options);
      for(const item of options.services){form.querySelector(`[name=service][value="${CSS.escape(item.id)}"]`).checked=(parsed.data['proxy-groups'] ?? []).some(group=>group.name===item.name);}
      dirty=true;await syncRouting();
      if(mode==='yaml'){form.querySelector('[data-yaml-editor]').innerHTML='';mountYamlEditor(form,draft);}
      selectedPlan=event.target.value;
    }catch(err){event.target.value=selectedPlan;toast(err.message,true);}
    finally{event.target.disabled=false;save.disabled=false;}
  });
  form.querySelector('#subscriptionRouting').addEventListener('change',()=>{dirty=true;});
  form.querySelector('#subscriptionMode').addEventListener('change',async event=>{
    const next=event.target.value;
    const save=form.querySelector('button[type=submit]');
    const controls=form.querySelectorAll('[name=edit-mode]');
    controls.forEach(control=>{control.disabled=true;});save.disabled=true;
    try{
      if(mode==='routing'){
        await syncRouting();
        form.querySelector('[data-yaml-editor]').innerHTML='';
        form.querySelector('#subscriptionYAML').hidden=false;
        mountYamlEditor(form,draft);
      }else{
        const text=form.querySelector('[name=yaml]').value;
        const result=await request({action:'inspect-subscription',yaml:text});
        draft=text;applyChoices(result.data);dirty=false;
      }
      mode=next;
      form.querySelector('#subscriptionRouting').hidden=mode==='yaml';
      form.querySelector('#subscriptionYAML').hidden=mode!=='yaml';
      $('#modalError').hidden=true;
    }catch(err){controls.forEach(control=>{control.checked=control.value===mode;});$('#modalError').textContent=err.message;$('#modalError').hidden=false;}
    finally{controls.forEach(control=>{control.disabled=false;});save.disabled=false;}
  });
}

async function selectProfile(id) {
  profile = await api('/api/profiles/'+encodeURIComponent(id));
  sessionStorage.setItem('profile-id',id);
  renderAll();
}

const actions = {
  'toggle-source':id=>{if(expandedProfiles.has(id))expandedProfiles.delete(id);else expandedProfiles.add(id);renderAll();},
  'rule-exception':()=>itemEditor(undefined,true),
  'create-plan':()=>modal('新增规则方案',field('方案名称','name','','text','required'),async form=>{const result=await api('/api/routing-plans',{name:form.get('name'),copyID:routingPlanID});routingPlanID=result.id;sessionStorage.setItem('routing-plan',routingPlanID);await refresh();renderAll();},{confirm:'新建',description:'复制当前方案，可独立调整规则和策略。'}),
  'rename-plan':()=>modal('重命名规则方案',field('方案名称','name',profile.name,'text','required'),form=>update({action:'rename',name:form.get('name')},'方案已重命名')),
  'delete-plan':()=>modal('删除规则方案',`<p class="${help}">删除当前方案；仍被订阅引用的方案不能删除。</p>`,async()=>{await api('/api/routing-plans/'+encodeURIComponent(profile.id),{revision:profile.revision},'DELETE');routingPlanID='system-routing-template';sessionStorage.setItem('routing-plan',routingPlanID);await refresh();renderAll();},{confirm:'删除',danger:true}),
  sidebar:()=>closeSidebar(),
  'close-modal':()=>$('#modal').close(),
  'toggle-dark':()=>{toggleDark();if (!$('#app').hidden) renderAll();},
  'change-password':()=>modal('修改管理员密码',field('当前密码','currentPassword','','password','required autocomplete="current-password"')+field('新密码','newPassword','','password','required autocomplete="new-password"')+field('确认新密码','confirmPassword','','password','required autocomplete="new-password"'),async form=>{
    if(form.get('newPassword')!==form.get('confirmPassword')) throw new Error('两次新密码不一致');
    await api('/api/password',{currentPassword:form.get('currentPassword'),newPassword:form.get('newPassword')});
    showLogin();toast('密码已修改，请重新登录');
  },{confirm:'保存密码'}),
  'check-update':async()=>{
    if(!appVersion.canUpdate){toast('当前启动方式不支持在线更新，请先使用新版 Docker 镜像部署',true);return;}
    toast('正在检查 GitHub Releases');
    const info=await api('/api/update');
    if(!info.available){toast('当前已是最新版本');return;}
    modal('更新应用程序',`<p class="${help}">可更新至 ${esc(info.tag)}。系统会先保存数据库备份，再下载并校验可执行程序，随后自动重启。配置和数据会保留。</p>`,async()=>{
      $('#modalDescription').textContent='正在下载并校验程序，请稍候…';
      await api('/api/update',{tag:info.tag});
      $('#modal').close();toast('程序已更新，正在重启');
      $('#appVersion').textContent='正在重启，请稍候…';
      for(let attempt=0;attempt<60;attempt++){
        await new Promise(resolve=>setTimeout(resolve,1000));
        try{const response=await fetch('/api/version');if(response.ok){const current=await response.json();if(current.version===info.commit){location.reload();return;}}}catch{}
      }
      toast('重启等待超时，请刷新页面查看状态',true);
    },{confirm:'一键更新'});
  },
  'create-backup':async()=>{await api('/api/backups',{});await refreshBackups();toast('备份已生成');},
  'download-backup':id=>{window.location.assign('/api/backups/'+encodeURIComponent(id)+'/download');},
  'upload-backup':()=>modal('上传数据库备份',field('备份文件','backup','','file','required accept=".db,application/octet-stream"')+`<p class="${help}">支持本系统导出的数据库文件，最大 128 MB。上传后可在列表中选择导入。</p>`,async form=>{
    const file=form.get('backup');
    if(file.size>128*1024*1024)throw new Error('备份不能超过 128 MB');
    const response=await fetch('/api/backups/upload',{method:'POST',headers:{'Content-Type':'application/octet-stream'},body:file});
    const data=await response.json();
    if(!response.ok){if(response.status===401)showLogin();throw new Error(data.error||'上传失败');}
    await refreshBackups();toast('备份已上传，尚未导入');
  },{confirm:'上传'}),
  'delete-backup':id=>modal('删除备份',`<p class="${help}">确定删除 ${esc(id)}？当前系统数据不会改变。</p>`,async()=>{await api('/api/backups/'+encodeURIComponent(id),{},'DELETE');await refreshBackups();toast('备份已删除');},{confirm:'删除',danger:true}),
  'restore-backup':id=>modal('导入备份并恢复',`<p class="${help}">将使用 ${esc(id)} 覆盖当前全部节点、规则方案和订阅。系统会先保存一份恢复前备份，管理员密码不变。</p><p class="${help}">恢复后所有设备需要重新登录。确认导入这份备份？</p>`,async()=>{
    await api('/api/backups/'+encodeURIComponent(id)+'/restore',{});
    selectedNodes.clear();expandedProfiles.clear();sessionStorage.removeItem('profile-id');
    showLogin();toast('数据已恢复，请使用当前管理员密码重新登录');
  },{confirm:'确认导入恢复',danger:true}),
  logout:async()=>{await api('/api/logout',{});showLogin();},
  create:()=>create(),
  'edit-source':sourceEditor,
  'refresh-source':async id=>{await api('/api/sources/'+encodeURIComponent(id)+'/refresh',{});await refresh(id);renderItems();toast('来源节点已更新');},
  'delete-source':async id=>{const source=await api('/api/profiles/'+encodeURIComponent(id));modal('删除来源配置',`<p class="${help}">删除 ${esc(source.name)} 的来源节点？已生成的订阅仍会保留。</p>`,async()=>{await api('/api/sources/'+encodeURIComponent(id),{revision:source.revision},'DELETE');for(const key of [...selectedNodes]){if(JSON.parse(key)[0]===id)selectedNodes.delete(key);}await refresh();renderItems();toast('来源配置已删除');},{confirm:'删除',danger:true});},
  'create-subscription':createSubscription,
  'subscription-address':subscriptionAddress,
  'random-slug':()=>{const bytes=crypto.getRandomValues(new Uint8Array(12));$('#modal [name=slug]').value='cfg-'+Array.from(bytes,b=>b.toString(16).padStart(2,'0')).join('');},
  'delete-subscription':async id=>{const subscription=await api('/api/profiles/'+encodeURIComponent(id));modal('删除订阅',`<p class="${help}">确定删除 ${esc(subscription.name)}？删除后订阅地址失效，来源配置仍保留。</p>`,async()=>{await api('/api/subscriptions/'+encodeURIComponent(id),{revision:subscription.revision},'DELETE');await refresh();renderItems();toast('订阅已删除');},{confirm:'删除',danger:true});},
  'edit-subscription':editSubscription,
  'go-subscriptions':()=>{$('#modal').close();location.hash='subscription';},
  'open-profile':async id=>{expandedProfiles.add(id);await selectProfile(id);location.hash='proxies';},
  'add-item':()=>itemEditor(),
  'edit-item':itemEditor,
  'move-rule':key=>modal('调整规则顺序',field('新位置','position',Number(key)+1,'number',`min="1" max="${profile.data.rules.length}" required`),
    form=>preview({action:'edit',edit:{section:'rules',key,move:Number(form.get('position'))-1}}),{confirm:'预览变更'}),
  'delete-item':key=>modal('删除条目',`<p class="${help}">确定删除这条记录？引用中的节点或规则集需要先解除引用。</p>`,
    ()=>preview({action:'edit',edit:{section:active,key,delete:true}}),{confirm:'预览删除',danger:true}),
  'merge-rules':()=>mergeEditor(api,preview,true,ruleTargets()),
  'group-add':async id=>{await selectProfile(id);itemEditor();},
  'group-edit':async key=>{const [id,name]=JSON.parse(key);await selectProfile(id);itemEditor(name);},
  'group-delete':async key=>{const [id,name]=JSON.parse(key);await selectProfile(id);modal('删除节点',`<p class="${help}">删除 ${esc(name)} 时会同步移除策略组成员引用。链式代理或自定义规则的依赖需要先调整。</p>`,()=>preview({action:'edit',edit:{section:'proxies',key:name,delete:true}}),{confirm:'预览删除'});},
  'copy-link':async()=>{
    const node = $('#subURL');
    if (navigator.clipboard) {await navigator.clipboard.writeText(node.value);toast('订阅链接已复制');}
    else {node.type='text';node.select();toast('请复制选中的订阅链接');}
  },
  prev:()=>{page--;renderItems();},
  next:()=>{page++;renderItems();},
};

document.addEventListener('click', async event => {
  if(!event.target.closest('button,input,a,label')){const row=event.target.closest('[data-source-row]');if(row){actions['toggle-source'](row.dataset.sourceRow);return;}}
  const control = event.target.closest('[data-action]');
  if (!control || !Object.hasOwn(actions,control.dataset.action)) return;
  const key = control.dataset.key;
  control.disabled = true;
  try {await actions[control.dataset.action](key);}
  catch (err) {toast(err.message,true);}
  finally {control.disabled = false;}
});

document.addEventListener('input',event=>{
  if (event.target.id==='search'){search=event.target.value;page=1;renderItems();}
});

document.addEventListener('change',async event=>{
  const node=event.target;
  try {
    if (node.name==='selected-node') {if(node.checked)selectedNodes.add(node.value);else selectedNodes.delete(node.value);renderAll();}
    if (node.name==='select-profile-nodes') {const group=nodeGroups.find(g=>g.id===node.value);for(const n of group.nodes){const key=JSON.stringify([group.id,n.name]);if(node.checked)selectedNodes.add(key);else selectedNodes.delete(key);}renderAll();}
    if (node.name==='routing-plan' && !node.closest('#modal')) {routingPlanID=node.value;sessionStorage.setItem('routing-plan',routingPlanID);profile=await api(templatePath());renderAll();}
  } catch (err) {toast(err.message,true);}
});
window.addEventListener('hashchange',async()=>{
  const route=location.hash.slice(1);
  active=Object.hasOwn(titles,route)?route:'profiles';
  search='';page=1;
  closeSidebar(false);
  if (!$('#app').hidden) {try{if(routingPages.has(active))profile=await api(templatePath());else if(profile?.kind==='template')await refresh();if(active==='backups')backups=await api('/api/backups');renderAll();}catch(err){toast(err.message,true);}}
});

const logo = `<svg class="h-5 w-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>`;
$('#brandLogo').innerHTML=logo;
$('#loginLogo').innerHTML=logo.replace('h-5 w-5','h-7 w-7');
$('[data-action="sidebar"]').innerHTML=icon('menu','h-5 w-5');
$('#loginField').innerHTML=field('管理员密码','password','','password','required autocomplete="current-password"');
$('#loginSubmit').innerHTML=button('登录',null,{submit:true,block:true,variant:'primary'});
$('#loginForm').addEventListener('submit',async event=>{
  event.preventDefault();
  const submit=$('button[type=submit]',event.target);
  submit.disabled=true;
  $('#loginError').hidden=true;
  try{
    await api('/api/login',{password:new FormData(event.target).get('password')});
    event.target.reset();
    await showApp();
  }catch(err){
    $('#loginError').textContent=err.message;
    $('#loginError').hidden=false;
  }finally{submit.disabled=false;}
});
applyTheme();
try {
  if ((await api('/api/session')).authenticated) await showApp();
  else showLogin();
} catch(err){showLogin();toast(err.message,true);}
