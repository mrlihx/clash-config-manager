import {mountYamlEditor} from './yaml-editor.js';
import { $, area, button, checkbox, esc, field, help, label, modal, select } from './ui.js';

const protocols = ['vless', 'trojan', 'ss', 'anytls', 'hysteria2', 'http'];
const lines = value => String(value ?? '').split(/[,\n]/).map(item => item.trim()).filter(Boolean);
const trim = (form, key) => String(form.get(key) ?? '').trim();
const grid = content => `<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">${content}</div>`;

function options(data, current) {
  const items = [['', '不使用']];
  for (const section of ['proxies', 'proxy-groups']) {
    for (const {name} of data[section] ?? []) if (name !== current) items.push(name);
  }
  return items;
}

function nodeFields(node) {
  const common = grid(
    field('服务器地址', 'server', node.server, 'text', 'required') +
    field('端口', 'port', node.port, 'number', 'required min="1" max="65535"'),
  );
  let fields = '';
  switch (node.type) {
    case 'vless':
      fields = field('UUID', 'uuid', node.uuid, 'password', 'required autocomplete="off"') +
        grid(select('传输方式', 'network', ['tcp', 'ws', 'grpc', ...(node.network && !['tcp','ws','grpc'].includes(node.network) ? [node.network] : [])], node.network || 'tcp') +
          field('SNI / Server Name', 'servername', node.servername)) +
        grid(field('Flow', 'flow', node.flow) + field('客户端指纹', 'client-fingerprint', node['client-fingerprint'])) +
        grid(field('Encryption', 'encryption', node.encryption) + field('gRPC 服务名', 'grpc-service-name', node['grpc-opts']?.['grpc-service-name'])) +
        grid(field('WebSocket Host', 'ws-host', node['ws-opts']?.headers?.Host) + field('WebSocket 路径', 'ws-path', node['ws-opts']?.path)) +
        grid(field('Reality 公钥', 'public-key', node['reality-opts']?.['public-key']) + field('Reality Short ID', 'short-id', node['reality-opts']?.['short-id']));
      break;
    case 'trojan':
      fields=field('密码','password',node.password,'password','required autocomplete="off"')+
        grid(field('SNI','sni',node.sni)+field('客户端指纹','client-fingerprint',node['client-fingerprint']))+
        grid(select('传输方式','network',['tcp','ws','grpc'],node.network || 'tcp')+field('gRPC 服务名','grpc-service-name',node['grpc-opts']?.['grpc-service-name']))+
        grid(field('WebSocket Host','ws-host',node['ws-opts']?.headers?.Host)+field('WebSocket 路径','ws-path',node['ws-opts']?.path));
      break;
    case 'ss':
      fields=grid(field('加密方式（cipher）','cipher',node.cipher || 'aes-128-gcm','text','required')+field('密码','password',node.password,'password','required autocomplete="off"'));
      break;
    case 'http':
      fields = grid(field('账号', 'username', node.username) + field('密码', 'password', node.password, 'password', 'autocomplete="off"')) + field('SNI', 'sni', node.sni);
      break;
    default:
      fields = grid(field('密码', 'password', node.password, 'password', 'required autocomplete="off"') + field('SNI', 'sni', node.sni));
      if (node.type === 'hysteria2') fields += grid(field('混淆类型', 'obfs', node.obfs) + field('混淆密码', 'obfs-password', node['obfs-password'], 'password', 'autocomplete="off"'));
  }
  if(node.type==='ss')return common+fields+checkbox('允许 UDP','udp','1',node.udp ?? true);
  return common + fields + field('ALPN（逗号分隔）', 'alpn', node.alpn?.join(', ')) + grid(
    ['http', 'vless'].includes(node.type) ? checkbox('启用 TLS', 'tls', '1', node.tls) : '',
  ) + grid(checkbox('允许 UDP', 'udp', '1', node.udp) + checkbox('跳过证书验证', 'skip-cert-verify', '1', node['skip-cert-verify']));
}

export function nodeEditor(data, key, api, preview) {
  const initial = (data.proxies ?? []).find(node => node.name === key);
  if (initial && !protocols.includes(initial.type)) throw new Error('此协议可在完整 YAML 编辑器中编辑');
  let node = initial ?? {type: 'vless'};
  const groups = data['proxy-groups'] ?? [];
  const dialog = modal(key ? '编辑节点' : '新增节点', `
    ${key ? '' : `<div class="flex items-end gap-2">${field('粘贴节点链接（可选）', 'link', '', 'password', 'autocomplete="off"')}${button('解析链接', 'parse-link', {symbol: 'link'})}</div>`}
    ${grid(field('节点名称', 'name', node.name, 'text', 'required') + select('协议', 'type', protocols, node.type))}
    <div id="nodeFields" class="space-y-4">${nodeFields(node)}</div>
    ${select('前置代理（dialer-proxy）', 'dialer-proxy', options(data, key), node['dialer-proxy'])}
    <fieldset class="space-y-2"><legend class="${label}">加入策略组</legend><div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
      ${groups.map(group => checkbox(group.name, 'group', group.name, (group.proxies ?? []).includes(key))).join('') || `<p class="${help}">当前配置还没有策略组</p>`}
    </div></fieldset>`, (form) => {
    const type = trim(form, 'type');
    const value = {name: trim(form, 'name'), type, server: trim(form, 'server'), port: Number(form.get('port'))};
    const optional = ['cipher','uuid','servername','flow','client-fingerprint','encryption','username','password','sni','obfs','obfs-password','dialer-proxy'];
    for (const name of optional) {
      if (form.has(name)) {
        const entered = ['password', 'obfs-password'].includes(name) ? form.get(name) : trim(form, name);
        if (entered || initial?.[name]) value[name] = entered || null;
      } else if (initial && initial.type !== type && initial[name] !== undefined) value[name] = null;
    }
    for (const name of ['tls','udp','skip-cert-verify']) {
      if (form.has(name) || initial?.[name] !== undefined) value[name] = form.has(name);
    }
    if (!['vless','http'].includes(type)) {
      if (initial?.type !== type && initial?.tls !== undefined) value.tls = null;
      else delete value.tls;
    }
    if(initial?.type==='ss' && type!=='ss'){for(const name of ['plugin','plugin-opts'])if(initial[name]!==undefined)value[name]=null;}
    const alpn = lines(form.get('alpn'));
    if (alpn.length || initial?.alpn) value.alpn = alpn.length ? alpn : null;
    if (['vless','trojan'].includes(type)) {
      if(type==='trojan' && initial?.['reality-opts']) value['reality-opts']=null;
      const network = trim(form, 'network');
      value.network = network;
      const reality = trim(form, 'public-key');
      if (type==='vless' && (reality || initial?.['reality-opts'])) {
        value['reality-opts'] = reality ? {...node['reality-opts'], 'public-key': reality, 'short-id': trim(form, 'short-id')} : null;
        if (reality) value.tls = true;
      }
      if (network === 'ws') {
        const opts = {...node['ws-opts'], path: trim(form, 'ws-path') || '/'};
        const host = trim(form, 'ws-host');
        const headers = {...opts.headers};
        if (host) headers.Host = host;
        else delete headers.Host;
        if (Object.keys(headers).length) opts.headers = headers;
        else delete opts.headers;
        value['ws-opts'] = opts;
      } else if (initial?.['ws-opts']) value['ws-opts'] = null;
      if (network === 'grpc') value['grpc-opts'] = {...node['grpc-opts'], 'grpc-service-name': trim(form, 'grpc-service-name')};
      else if (initial?.['grpc-opts']) value['grpc-opts'] = null;
    } else if (['vless','trojan'].includes(initial?.type)) {
      for (const name of ['network','reality-opts','ws-opts','grpc-opts']) if (initial[name] !== undefined) value[name] = null;
    }
    return preview({action: 'edit', edit: {section: 'proxies', key, value, groups: form.getAll('group')}});
  }, {confirm: '预览变更', description: '填写节点参数并选择要同步更新的策略组。'});
  $('[name="type"]', dialog).addEventListener('change', event => {
    const current = new FormData(dialog);
    node = {...node, type: event.target.value, server: current.get('server'), port: current.get('port')};
    $('#nodeFields').innerHTML = nodeFields(node);
  });
  $('[data-action="parse-link"]', dialog)?.addEventListener('click', async event => {
    const target = event.currentTarget;
    target.disabled = true;
    try {
      node = await api('/api/node-link', {link: $('[name="link"]', dialog).value});
      $('[name="name"]', dialog).value = node.name;
      $('[name="type"]', dialog).value = node.type;
      $('#nodeFields').innerHTML = nodeFields(node);
      $('#modalError').hidden = true;
    } catch (err) {
      $('#modalError').textContent = err.message;
      $('#modalError').hidden = false;
    } finally {
      target.disabled = false;
    }
  });
}

export function groupEditor(data, key, preview, system = false, template = false) {
  const item = (data['proxy-groups'] ?? []).find(group => group.name === key) ?? {};
  if(key==='🛡️ 广告拦截'){modal('广告拦截策略组',`<p class="${help}">固定执行 REJECT，无需选择节点。需要关闭拦截时，请在订阅编辑中取消对应广告规则集。</p>`,null);return;}
  const types = key==='♻️ 自动选择' ? ['url-test'] : ['select','url-test','fallback','load-balance'];
  if (item.type && !types.includes(item.type)) types.push(item.type);
  const members = (data.proxies ?? []).map(node=>node.name);
  const form = modal(key ? '编辑策略组' : '新增策略组', `
    ${grid(field('组名称', 'name', item.name, 'text', system ? 'required readonly' : 'required') + select('类型', 'type', types, item.type || 'select'))}
    ${template ? `<p class="${help}">新订阅默认加入全部已选节点；服务组提供手动和自动选择入口。</p>` : `<fieldset class="space-y-2"><legend class="${label}">策略组成员</legend><div class="grid grid-cols-1 sm:grid-cols-2 gap-2">${members.map(name => checkbox(name,'members',name,item.proxies?.includes(name))).join('')}</div></fieldset>`}
    <div id="balanceStrategy">${select('负载均衡策略','strategy',['consistent-hashing','round-robin','sticky-sessions'],item.strategy || 'consistent-hashing')}</div>
    ${field('测速 URL', 'url', item.url, 'url')}
    ${grid(field('测速间隔（秒）', 'interval', item.interval, 'number', 'min="1"') + field('容差（毫秒）', 'tolerance', item.tolerance, 'number', 'min="0"'))}
    ${checkbox('空闲时暂停测速（lazy）','lazy','1',item.lazy ?? false)}`,
  form => {
    const value = {name:trim(form,'name'), type:trim(form,'type'),lazy:form.has('lazy')};
    const selected = form.getAll('members');
    if (!template && !selected.length) throw new Error('请至少选择一个节点');
    if (!template) value.proxies=selected;
    if(value.type==='load-balance') value.strategy=trim(form,'strategy');
    else if(item.strategy) value.strategy=null;
    if(trim(form,'url') || item.url || value.type!=='select') value.url=trim(form,'url') || (value.type==='select' ? null : 'https://www.gstatic.com/generate_204');
    for (const name of ['interval','tolerance']) {
      if (trim(form,name) || item[name] !== undefined) value[name] = trim(form,name) ? Number(form.get(name)) : null;
    }
    return preview({action:'edit', edit:{section:'proxy-groups',key,value}});
  }, {confirm:'预览变更'});
  const showStrategy=()=>{ $('#balanceStrategy').hidden=$('[name=type]',form).value!=='load-balance'; };
  $('[name=type]',form).addEventListener('change',showStrategy);showStrategy();
}

export function providerEditor(section, data, key, preview) {
  const item=data[section]?.[key] ?? {};
  const rule=(data.rules ?? []).map((value,index)=>({value,index})).find(entry=>entry.value.startsWith('RULE-SET,'+key+','));
  const fixed=['Ads','System_Reject'].includes(key);
  const targets=[...new Set([...(data['proxy-groups'] ?? []).map(group=>group.name),'DIRECT','REJECT','PASS'])];
  const types=[['http','订阅 URL'],['inline','手动编写文本规则'],...(item.type==='file'?[['file','本地文件']]:[])];
  const dialog=modal(key ? '编辑规则集' : '新增规则集',
    grid(field('名称','name',key,'text','required')+select('规则来源','type',types,item.type || 'http'))+
    grid(select('匹配类型','behavior',[['classical','综合规则（域名、IP、端口等）'],['domain','纯域名'],['ipcidr','IP 网段']],item.behavior || 'classical')+select('远程文件格式','format',['yaml','text','mrs'],item.format || 'yaml'))+
    `<div id="providerRemote" class="space-y-4">${field('订阅 URL','url',item.url,'url')}${field('自动更新间隔（秒）','interval',item.interval || 86400,'number','min="1"')}</div>
    <div id="providerFilePath">${field('文件路径','path',item.path)}</div>
    <div id="providerInline" class="space-y-2"><p class="${help}">每行一条匹配条件，也可粘贴 payload YAML。不要在每行填写目标策略。</p><div data-yaml-editor class="yaml-editor"></div></div>`+
    grid(fixed?`<p class="${help}">广告规则固定拦截（REJECT）</p>`:select('目标策略','target',targets,rule?.value.split(',')[2] || '🚀 手动选择'))+
    field('优先级（规则位置，留空自动安排）','position',rule?rule.index+1:'','number','min="1"')+
    `<p class="${help}">保存时自动生成分流引用，并更新跟随此规则方案的订阅。</p>`,
    form=>{
      const type=trim(form,'type');
      const value={name:trim(form,'name'),type,behavior:trim(form,'behavior'),format:type==='inline'?'yaml':trim(form,'format')};
      if(type==='http'){value.url=trim(form,'url');value.interval=Number(form.get('interval'));value.path=null;if(item.payload)value.payload=null;}
      else if(type==='file'){value.path=trim(form,'path');}
      else {value.url=null;value.interval=null;value.path=null;}
      return preview({action:'edit',edit:{section,key,value,target:fixed?'🛡️ 广告拦截':trim(form,'target'),position:trim(form,'position')?Number(form.get('position'))-1:undefined,payloadText:type==='inline'?form.get('payloadText'):undefined}});
    },{wide:true,confirm:'预览变更'});
  mountYamlEditor(dialog,(item.payload ?? []).join('\n'),{name:'payloadText',label:'手动规则文本'});
  const showSource=()=>{
    const type=dialog.querySelector('[name=type]').value;
    dialog.querySelector('#providerRemote').hidden=type!=='http';
    dialog.querySelector('#providerFilePath').hidden=type!=='file';
    dialog.querySelector('#providerInline').hidden=type!=='inline';
    dialog.querySelector('[name=format]').disabled=type==='inline';
    dialog.querySelector('[name=url]').required=type==='http';
    dialog.querySelector('[name=url]').disabled=type!=='http';
    dialog.querySelector('[name=interval]').disabled=type!=='http';
  };
  dialog.querySelector('[name=type]').addEventListener('change',showSource);showSource();
}

export function ruleEditor(data, key, preview, exception = false) {
  const targets=options(data).slice(1).concat(['DIRECT','REJECT','REJECT-DROP','PASS']);
  const current=key===undefined?'':data.rules[Number(key)];
  const types=exception?['DOMAIN','DOMAIN-SUFFIX']:['DOMAIN','DOMAIN-SUFFIX','DOMAIN-KEYWORD','GEOSITE','GEOIP','IP-CIDR','IP-CIDR6','SRC-IP-CIDR','RULE-SET','DST-PORT','SRC-PORT','NETWORK','MATCH'];
  const parts=current?current.split(',').map(value=>value.trim()):[];
  if(current && (!types.includes(parts[0]) || current.includes('('))){
    modal('编辑分流规则',area('Mihomo 规则','rule',current,4),form=>preview({action:'edit',edit:{section:'rules',key,value:trim(form,'rule')}}),{confirm:'预览变更'});return;
  }
  const adIndex=(data.rules ?? []).findIndex(rule=>/^RULE-SET,(Ads|System_Reject),/.test(rule));
  const defaultPosition=(data.rules ?? []).findIndex(rule=>rule.startsWith('RULE-SET,System_')&&!['System_Private','System_LanCIDR','System_SteamCDN'].some(name=>rule.startsWith('RULE-SET,'+name+',')));
  const target=current?parts[parts.length-(parts.at(-1)==='no-resolve'?2:1)]:exception?'':'🚀 手动选择';
  modal(current?'编辑分流规则':exception?'新增域名例外':'新增分流规则',
    select('匹配类型','kind',types,parts[0] || (exception?'DOMAIN-SUFFIX':'DOMAIN'))+
    field('匹配值','pattern',parts[0]==='MATCH'?'':parts[1] || '')+
    select('目标策略','target',exception?[['','请选择目标策略'],...targets]:targets,target)+
    field('规则位置（数字越小优先级越高）','position',current?Number(key)+1:exception&&adIndex>=0?adIndex+1:defaultPosition>=0?defaultPosition+1:'','number','min="1"')+
    checkbox('不额外触发 DNS 解析（IP 规则）','no-resolve','1',parts.at(-1)==='no-resolve'),
    form=>{
      const kind=trim(form,'kind'),target=trim(form,'target'),pattern=trim(form,'pattern');
      if(!target)throw new Error('请选择直连、代理组或其他目标策略');
      if(kind!=='MATCH'&&!pattern)throw new Error('请输入匹配值');
      const noResolve=form.has('no-resolve')&&['GEOIP','IP-CIDR','IP-CIDR6','RULE-SET'].includes(kind);
      const value=kind==='MATCH'?`MATCH,${target}`:[kind,pattern,target,...(noResolve?['no-resolve']:[])].join(',');
      return preview({action:'edit',edit:{section:'rules',key,value,position:trim(form,'position')?Number(form.get('position'))-1:undefined}});
    },{confirm:'预览变更',description:exception?'例外规则优先于广告规则，目标由你明确选择。':'规则从上到下匹配；保存后更新关联订阅。'});
}

export function mergeEditor(api, preview, rulesOnly = false, targets = []) {
  let source = '';
  const form = modal(rulesOnly ? '选择外部规则' : '导入节点 / 规则', `
    ${field('来源订阅链接','sourceURL','','url','autocomplete="off" placeholder="https://…"')}
    ${field('或选择配置文件','file','','file')}
    ${select('来源', 'existing', [['','使用订阅链接或文件']], '')}
    ${select('选中规则的目标策略','ruleTarget',targets,'🚀 手动选择')}
    ${button('读取条目','read-merge',{symbol:'file'})}
    <div id="mergeItems" class="space-y-4"></div>`, values => {
    const selection = {};
    for (const name of ['proxies','proxy-groups','rule-providers','proxy-providers','rules']) selection[name] = values.getAll(name);
    if (!source || !Object.values(selection).some(items => items.length)) throw new Error('请先读取来源并选择要合并的条目');
    return preview({action:'merge',source,selection,ruleTarget:values.get('ruleTarget')});
  }, {confirm:'预览合并',description:'默认只勾选节点。外部规则需手动选择，导入后排在系统内置规则之后。'});
  api('/api/profiles').then(items=>{const choice=$('[name="existing"]',form);for(const p of items) choice.add(new Option(p.name,p.id));}).catch(err=>{ $('#modalError').textContent=err.message;$('#modalError').hidden=false;});
  for (const name of ['existing','file']) $('[name="'+name+'"]',form).addEventListener('change',()=>{source='';$('#mergeItems').replaceChildren();});
  $('[name="sourceURL"]',form).addEventListener('input',()=>{
    source='';
    $('#mergeItems').replaceChildren();
  });
  $('[data-action="read-merge"]',form).addEventListener('click', async event => {
    const target = event.currentTarget;
    target.disabled = true;
    source = '';
    $('#mergeItems').replaceChildren();
    try {
      const values=new FormData(form);
      const file=values.get('file');
      const selected=values.get('existing');
      let data;
      if (selected) {const p=await api('/api/profiles/'+encodeURIComponent(selected));data={yaml:p.yaml,data:p.data};}
      else data=await api('/api/import',{url:values.get('sourceURL').trim(),yaml:file?.size ? await file.text() : ''});
      source = data.yaml;
      const sections = data.data;
      const labels = {'proxies':'节点','proxy-groups':'策略组','rule-providers':'规则集','proxy-providers':'代理集合','rules':'分流规则'};
      $('#mergeItems').innerHTML = Object.entries(labels).map(([name,title]) => {
        const items = name === 'rules' ? (sections.rules ?? []).flatMap((rule,i) => /^\s*MATCH\s*,/.test(rule) ? [] : [[String(i),rule]]) :
          Array.isArray(sections[name]) ? sections[name].map(item => [item.name,item.name]) : Object.keys(sections[name] ?? {}).map(key => [key,key]);
        return `<fieldset class="space-y-2"><legend class="${label}">${title}（${items.length}）</legend>${items.map(([value,label])=>checkbox(label,name,value,!rulesOnly && name==='proxies')).join('')}</fieldset>`;
      }).join('');
      $('#modalError').hidden = true;
    } catch (err) {
      $('#modalError').textContent = err.message;
      $('#modalError').hidden = false;
    } finally {
      target.disabled = false;
    }
  });
}
