import { $, button } from './ui.js';

const bg = {
  header: {light:['bg-white dark:bg-slate-900','bg-white/80 dark:bg-slate-900/80'],dark:['bg-slate-900 dark:bg-slate-950','bg-slate-900/85 dark:bg-slate-950/85'],primary:['bg-primary-600 dark:bg-primary-700','bg-primary-600/90 dark:bg-primary-700/90'],navy:['bg-[#0a192f]','bg-[#0a192f]/90'],purple:['bg-[#1e1b4b]','bg-[#1e1b4b]/90']},
  sidebar: {light:['bg-white dark:bg-slate-900','bg-white/80 dark:bg-slate-900/80'],dark:['bg-slate-900 dark:bg-slate-950','bg-slate-900/90 dark:bg-slate-950/90'],primary:['bg-primary-950','bg-primary-950/90'],navy:['bg-[#0b1329]','bg-[#0b1329]/90'],zinc:['bg-zinc-950','bg-zinc-950/90']},
};
const defaults = {
  'theme-dark':String(matchMedia('(prefers-color-scheme: dark)').matches), 'theme-color':'indigo',
  'header-theme':'light','sidebar-theme':'light','header-custom-color':'#1e293b','sidebar-custom-color':'#0f172a',
  'theme-glass':'true','theme-page-animation':'true','theme-radius':'smooth',
};
const root = document.documentElement;
const setting = key => localStorage.getItem(key) ?? defaults[key];
export const animated = () => setting('theme-page-animation') === 'true';
export function sidebarDark() {
  const theme = setting('sidebar-theme');
  return theme === 'custom' ? darkHex(setting('sidebar-custom-color')) : theme === 'light' ? root.classList.contains('dark') : true;
}
function darkHex(hex) {
  return [1,3,5].map(i => parseInt(hex.slice(i,i+2),16)).reduce((acc,n,i)=>acc+n*[.299,.587,.114][i],0) < 145;
}
export function applyTheme() {
  const dark = setting('theme-dark') === 'true';
  root.classList.toggle('dark', dark);
  root.dataset.themeColor = setting('theme-color');
  root.dataset.radius = setting('theme-radius');
  const glass = setting('theme-glass') === 'true';
  root.dataset.glass = String(glass);
  root.dataset.pageAnimation = String(animated());
  for (const region of ['header','sidebar']) {
    const node = document.getElementById(region);
    const previous = node.dataset.themeClasses?.split(' ').filter(Boolean) ?? [];
    node.classList.remove(...previous);
    const theme = setting(region+'-theme');
    const shaded = theme === 'custom' ? darkHex(setting(region+'-custom-color')) : theme === 'light' ? dark : true;
    const border = region === 'header' ? 'border-b' : 'border-r';
    const tone = shaded ? 'border-white/10 text-white' : 'border-slate-200/80 dark:border-slate-800 text-slate-800 dark:text-slate-100';
    const classes = [glass ? 'backdrop-blur-md' : 'backdrop-blur-none', border, tone, bg[region][theme]?.[Number(glass)] ?? ''].join(' ').split(' ').filter(Boolean);
    node.classList.add(...classes);
    node.dataset.themeClasses = classes.join(' ');
    node.style.backgroundColor = theme === 'custom' ? setting(region+'-custom-color') : '';
  }
  $('#loginTheme').innerHTML = button('', 'toggle-dark', {symbol:dark ? 'sun' : 'moon', variant:'ghost', title:'切换明暗模式'});
}
export function toggleDark() {
  localStorage.setItem('theme-dark',String(setting('theme-dark') !== 'true'));
  applyTheme();
}
