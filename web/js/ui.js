import {destroyYamlEditor} from './yaml-editor.js';

// 样式和图标沿用 gemini-vue 的 Button、Input、Table、Modal 和布局组件。
export const $ = (selector, root = document) => root.querySelector(selector);
export const esc = (value = '') => String(value ?? '').replace(/[&<>"']/g, ch => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
})[ch]);

const paths = {
  menu: 'M4 6h16M4 12h16M4 18h16',
  plus: 'M12 4v16m8-8H4',
  close: 'M6 18L18 6M6 6l12 12',
  search: 'M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z',
  download: 'M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4',
  sun: 'M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l.707-.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z',
  moon: 'M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z',
  palette: 'M12 3a9 9 0 0 0 0 18h1.5a1.5 1.5 0 0 0 1.2-2.4 1.5 1.5 0 0 1 1.2-2.4H18a3 3 0 0 0 3-3 9 9 0 0 0-9-10zM7.5 11h.01M10 7h.01m4 0h.01m2.49 4h.01',
  file: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8zm0 0v6h6M8 13h8m-8 4h8',
  server: 'M4 4h16v6H4zm0 10h16v6H4zm3-7h.01M7 17h.01',
  layers: 'M12 2 2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5',
  shield: 'M12 22s8-4 8-11V5l-8-3-8 3v6c0 7 8 11 8 11z',
  link: 'M10 13a4 4 0 0 0 6 .5l4-4a4 4 0 0 0-6-5.5l-2 2m2 5a4 4 0 0 0-6-.5l-4 4a4 4 0 0 0 6 5.5l2-2',
  clock: 'M12 8v4l3 2m6-2a9 9 0 1 1-18 0 9 9 0 0 1 18 0z',
  check: 'm5 12 4 4L19 6',
  edit: 'm16 4 4 4M4 20l4-1L21 6l-3-3L5 16z',
  arrow: 'M9 18l6-6-6-6',
  chevron: 'm6 9 6 6 6-6',
  eye: 'M15 12a3 3 0 11-6 0 3 3 0 016 0z M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z',
};

export function icon(name, classes = 'h-4 w-4') {
  return `<svg class="${classes}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="${paths[name]}"/></svg>`;
}

const baseButton = 'inline-flex items-center justify-center font-medium transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed select-none active:scale-[0.98] rounded-lg';
const variants = {
  primary: 'bg-primary-600 hover:bg-primary-700 text-white shadow-xs hover:shadow focus:ring-primary-500 dark:bg-primary-600 dark:hover:bg-primary-500',
  outline: 'border border-slate-300 dark:border-slate-700 text-slate-700 dark:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-800/60 focus:ring-slate-400 bg-transparent',
  ghost: 'text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/80 focus:ring-slate-400 bg-transparent',
  danger: 'bg-rose-600 hover:bg-rose-700 text-white shadow-xs hover:shadow focus:ring-rose-500 dark:bg-rose-500 dark:hover:bg-rose-600',
};
export function button(text, action, {variant = 'outline', symbol, key, submit = false, block = false, title} = {}) {
  return `<button type="${submit ? 'submit' : 'button'}" class="${baseButton} ${block ? 'px-4 py-2 text-sm gap-2 w-full' : 'px-3 py-1.5 text-xs gap-1.5'} ${variants[variant]}" ${action ? `data-action="${action}"` : ''} ${key === undefined ? '' : `data-key="${esc(key)}"`} ${title ? `title="${esc(title)}"` : ''}>${symbol ? icon(symbol) : ''}${esc(text)}</button>`;
}

export const panel = 'p-5 rounded-2xl bg-white dark:bg-slate-900 border border-slate-200/80 dark:border-slate-800 shadow-sm space-y-4';
const fieldBase = 'block w-full rounded-lg text-sm transition-all duration-200 outline-none bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 border shadow-sm';
export const input = fieldBase + ' px-3.5 py-2 border-slate-300 dark:border-slate-700 focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 dark:focus:border-primary-400';
export const label = 'block text-xs font-semibold uppercase tracking-wider text-slate-600 dark:text-slate-300 mb-1.5';
export const help = 'text-xs text-slate-500 dark:text-slate-400';

export function field(title, name, value = '', type = 'text', attrs = '') {
  const password = type === 'password' && !attrs.includes('readonly');
  return `<div class="w-full"><label class="block"><span class="${label}">${esc(title)}${/\brequired\b/.test(attrs) ? ' <span class="text-rose-500" aria-hidden="true">*</span>' : ''}</span>
    <span class="relative flex items-center"><input class="${input}${password ? ' pr-9' : ''}" name="${name}" type="${type}" value="${esc(value)}" ${attrs}>
    ${password ? `<button type="button" data-password-toggle aria-label="显示密码" aria-pressed="false" class="absolute inset-y-0 right-0 pr-3 flex items-center text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 focus:outline-none transition-colors">${icon('eye')}</button>` : ''}</span></label></div>`;
}

export function area(title, name, value, rows = 6) {
  return `<label class="block w-full"><span class="${label}">${esc(title)}</span><textarea class="${fieldBase} p-3 border-slate-300 dark:border-slate-700 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:focus:border-indigo-400 resize-y" name="${name}" rows="${rows}" spellcheck="false">${esc(value)}</textarea></label>`;
}

export function select(title, name, options, selected = '') {
  return `<label class="block w-full"><span class="${label}">${esc(title)}</span><span class="relative block">
    <select class="${input} appearance-none min-h-[40px] pr-9 font-medium cursor-pointer hover:border-slate-400 dark:hover:border-slate-600" name="${name}">${options.map(option => {
    const [v, text] = Array.isArray(option) ? option : [option, option];
    return `<option value="${esc(v)}" ${v === selected ? 'selected' : ''}>${esc(text)}</option>`;
  }).join('')}</select><span class="pointer-events-none absolute inset-y-0 right-0 pr-3 flex items-center text-slate-400">${icon('chevron')}</span></span></label>`;
}

document.addEventListener('click', event => {
  const button = event.target.closest('[data-password-toggle]');
  if (!button) return;
  const field = button.previousElementSibling;
  const show = field.type === 'password';
  field.type = show ? 'text' : 'password';
  button.setAttribute('aria-label',show ? '隐藏密码' : '显示密码');
  button.setAttribute('aria-pressed',String(show));
});

export function checkbox(title, name, value = '1', checked = false) {
  return `<label class="flex items-center gap-2 text-xs"><input type="checkbox" name="${name}" value="${esc(value)}" ${checked ? 'checked' : ''} class="rounded border-slate-300 dark:border-slate-700 text-indigo-600 focus:ring-indigo-500 cursor-pointer">${esc(title)}</label>`;
}

export function table(columns, rows) {
  return `<div class="w-full border border-slate-200 dark:border-slate-800 rounded-xl bg-white dark:bg-slate-900 shadow-sm relative overflow-hidden">
    <div class="sm:hidden flex items-center justify-between px-3.5 py-1.5 border-b border-slate-100 dark:border-slate-800/80 bg-slate-50/70 dark:bg-slate-800/40 text-[11px] text-slate-400 select-none"><span>左右滑动查看</span><span>表格滚动</span></div>
    <div class="overflow-x-auto scrolling-touch"><table class="w-full text-left border-collapse text-sm" style="min-width: 640px">
      <thead><tr class="border-b border-slate-200 dark:border-slate-800 bg-slate-50/75 dark:bg-slate-800/50 text-slate-500 dark:text-slate-400 font-semibold text-xs tracking-wider uppercase">${columns.map(col => `<th scope="col" class="px-4 py-3.5">${esc(col)}</th>`).join('')}</tr></thead>
      <tbody class="divide-y divide-slate-100 dark:divide-slate-800 text-slate-700 dark:text-slate-200">${rows.length ? rows.map(cells => `<tr class="hover:bg-slate-50/80 dark:hover:bg-slate-800/50 transition-colors">${cells.map(cell => `<td class="px-4 py-3.5 align-middle">${cell}</td>`).join('')}</tr>`).join('') : `<tr><td class="p-8 text-center ${help}" colspan="${columns.length}">暂无记录</td></tr>`}</tbody>
    </table></div></div>`;
}

export function heading(title, description, actions = '', parent = '配置管理') {
  return `<div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
    <div><div class="flex items-center gap-1.5 text-xs text-slate-400">${esc(parent)} ${icon('arrow', 'w-3 h-3')} <span class="text-slate-700 dark:text-slate-300">${esc(title)}</span></div>
    <h1 class="text-2xl font-bold text-slate-900 dark:text-slate-100 mt-2">${esc(title)}</h1><p class="${help} mt-0.5">${esc(description)}</p></div>
    <div class="flex flex-wrap items-center gap-2.5">${actions}</div></div>`;
}

export function toast(message, error = false) {
  const item = document.createElement('div');
  item.className = 'max-w-md px-4 py-3 rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-lg flex items-start gap-2.5 text-xs';
  item.innerHTML = `<span class="${error ? 'text-rose-600' : 'text-emerald-600'}">${icon(error ? 'close' : 'check')}</span><span class="whitespace-pre-wrap">${esc(message)}</span>`;
  $('#toasts').append(item);
  setTimeout(() => item.remove(), error ? 7500 : 3500);
}

let handler;
export function modal(title, body, submit, options = {}) {
  destroyYamlEditor();
  const dialog = $('#modal');
  dialog.classList.toggle('subscription-editor', Boolean(options.wide));
  $('#modalTitle').textContent = title;
  $('#modalDescription').textContent = options.description || '';
  $('#modalBody').innerHTML = body;
  $('#modalError').hidden = true;
  $('#modalFooter').innerHTML = button(submit ? '取消' : '关闭', 'close-modal') + (submit ? button(options.confirm || '保存', null, {submit: true, variant: options.danger ? 'danger' : 'primary'}) : '');
  handler = submit;
  if (!dialog.open) dialog.showModal();
  return $('#modalForm');
}

$('#modalForm').addEventListener('submit', async event => {
  event.preventDefault();
  if (!handler) return;
  const submit = $('button[type=submit]', event.currentTarget);
  if (submit.disabled) return;
  submit.disabled = true;
  $('#modalError').hidden = true;
  try {
    const close = await handler(new FormData(event.currentTarget), event.currentTarget);
    if (close !== false) $('#modal').close();
  } catch (err) {
    $('#modalError').textContent = err.message;
    $('#modalError').hidden = false;
  } finally {
    submit.disabled = false;
  }
});

{
  const id = 'modal';
  const dialog = document.getElementById(id);
  $('button[data-action^="close-"]', dialog).innerHTML = icon('close', 'h-5 w-5');
  dialog.addEventListener('click', event => {
    if (event.target === dialog && !event.composedPath().includes($('#modalForm'))) {
      const box = dialog.getBoundingClientRect();
      if (event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom) dialog.close();
    }
  });
}

$('#modal').addEventListener('close', destroyYamlEditor);
