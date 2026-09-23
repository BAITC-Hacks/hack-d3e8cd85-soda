'use strict';
const PATHS={arrow:'M5 12h14m-5-5 5 5-5 5',back:'M19 12H5m5-5-5 5 5 5',check:'m5 12 4 4L19 6',pin:'M20 10c0 6-8 11-8 11S4 16 4 10a8 8 0 1 1 16 0ZM15 10a3 3 0 1 1-6 0 3 3 0 0 1 6 0',calendar:'M5 5h14v15H5zM8 3v4m8-4v4M5 10h14',sparkles:'m12 3 2.4 6.6L21 12l-6.6 2.4L12 21l-2.4-6.6L3 12l6.6-2.4L12 3',mic:'M9 5a3 3 0 0 1 6 0v7a3 3 0 0 1-6 0V5ZM6 11v1a6 6 0 0 0 12 0v-1M12 18v4m-4 0h8',camera:'M3 7h4l2-3h6l2 3h4v13H3zM16 13a4 4 0 1 1-8 0 4 4 0 0 1 8 0',building:'M4 21V9l8-6 8 6v12M2 21h20M9 21v-7h6v7M8 10h0m8 0h0',flower:'M12 20v-8m0 7c-4 0-6-2-6-4 4 0 6 2 6 4m0-1c4 0 6-2 6-4-4 0-6 2-6 4M12 4c-5-5-9 2-4 4-5 5 3 8 4 3 3 5 9 0 4-3 5-3 0-8-4-4',heart:'M20 5c-3-3-6-1-8 1-2-2-5-4-8-1-6 6 8 15 8 15s14-9 8-15Z',gift:'M3 8h18v4H3zM5 12v9h14v-9M12 8v13M12 8C2 8 6-2 12 8c6-10 10 0 0 0',image:'M3 3h18v18H3zM3 17l6-6 5 5 3-3 4 4M16 7h0',bed:'M3 4v17m18-10v10M3 16h18M3 8h6v8M9 10h10a2 2 0 0 1 2 2v4',music:'M9 18V5l11-2v13M9 5v4l11-2M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0M20 16a3 3 0 1 1-6 0 3 3 0 0 1 6 0',info:'M12 16v-5m0-4h0M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',search:'M16 10a6 6 0 1 1-12 0 6 6 0 0 1 12 0m-2 4 7 7',clock:'M12 7v5l3 2M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',shield:'M12 3 4 6v6c0 5 8 9 8 9s8-4 8-9V6l-8-3Zm-4 9 3 3 5-6',sliders:'M4 7h16M4 17h16M8 4v6m8 4v6',globe:'M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0M2 12h20M12 2c-6 6-6 14 0 20 6-6 6-14 0-20'};
const icon=name=>`<svg viewBox="0 0 24 24" aria-hidden="true"><path d="${PATHS[name]||PATHS.sparkles}"/></svg>`;
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;'}[c]));
const money = n => new Intl.NumberFormat('ru-RU').format(n) + ' ₸';
const dateLabel = d => /^\d{4}-\d{2}-\d{2}$/.test(d) && !Number.isNaN(Date.parse(d)) ? new Intl.DateTimeFormat('ru-RU', {timeZone:'UTC'}).format(new Date(d + 'T12:00:00Z')) : 'Не указана';
const main = document.getElementById('main');
const STEPS = [['event','Событие','Город, дата и формат'], ['preferences','Условия','Бюджет и пожелания'], ['review','Проверка','Проверьте перед подбором']];
const blankQuery = () => ({city:'', event_date:'', event_type:'', category:'', budget_kzt:null, duration_hours:null, language:'', wishes:[], unverified_requirements:[]});
let state = blankQuery(), options = null, activeResults = null, previousSearch = null, busy = false, rawBrief = '', error = '', reviewRequirements = [];
const button = (text, route, primary = false, ico = 'arrow') => `<button type="button" class="btn ${primary ? 'btn-primary' : ''}" data-go="${esc(route)}">${icon(ico)}${esc(text)}</button>`;
const header = (kicker, title, subtitle) => `<div class="eyebrow">${esc(kicker)}</div><h1>${title}</h1><p class="intro">${esc(subtitle)}</p>`;
const selectOptions = (values, selected, placeholder = 'Выберите') => `<option value="">${esc(placeholder)}</option>` + values.map(v => `<option value="${esc(v)}"${v === selected ? ' selected' : ''}>${esc(v)}</option>`).join('');
const selectField = (label, name, values, value, required = true) => `<div class="field"><label for="${name}">${esc(label)}</label><select id="${name}" name="${name}" ${required ? 'required' : ''}>${selectOptions(values, value, required ? 'Выберите' : 'Не важно')}</select></div>`;
function announce(text) { document.getElementById('announcer').textContent = text; }
function navigate(route) { if (location.hash.slice(1) === route) render(); else location.hash = route; }
async function api(path, payload) {
  const response = await fetch(path, {method: payload === undefined ? 'GET' : 'POST', headers: payload === undefined ? {} : {'Content-Type':'application/json'}, body: payload === undefined ? undefined : JSON.stringify(payload), signal: AbortSignal.timeout(10000)});
  const data = await response.json();
  if (!response.ok) throw new Error(data.error?.message || 'Сервис недоступен. Попробуйте ещё раз.');
  return data;
}
function capture() {
  const form = main.querySelector('form[data-query]');
  if (!form) return;
  const before = JSON.stringify(state), data = new FormData(form);
  for (const [k,v] of data.entries()) {
    if (k === 'wish' || k === 'ignored_requirement') continue;
    state[k] = ['budget_kzt','duration_hours'].includes(k) ? (v === '' ? null : Number(v)) : v;
  }
  if (form.querySelector('[data-wishes]')) state.wishes = data.getAll('wish').sort();
  if (form.querySelector('#requirements-review')) {
    const ignored = new Set(data.getAll('ignored_requirement'));
    state.unverified_requirements = reviewRequirements.filter((_,i) => !ignored.has(String(i)));
  }
  if (JSON.stringify(state) !== before) activeResults = null;
}
function sidebar(route) {
  const active = STEPS.findIndex(s => s[0] === route);
  document.getElementById('step-nav').innerHTML = STEPS.map(([key,name,sub], i) => `<button type="button" data-go="${key}" class="step-button ${i === active ? 'active' : ''}" ${busy ? 'disabled' : ''} ${i === active ? 'aria-current="step"' : ''}><span class="number">${i+1}</span><span><strong>${name}</strong><small>${sub}</small></span></button>`).join('');

}
function dateChange(query, result) {
  if (!previousSearch || previousSearch.query.event_date === query.event_date || previousSearch.result.data_version !== result.data_version) return '';
  const stripDate = q => JSON.stringify({...q,event_date:''});
  if (stripDate(previousSearch.query) !== stripDate(query)) return '';
  const removed = previousSearch.result.cards.filter(p => p.busy_dates.includes(query.event_date)).map(p => p.anon_name);
  return removed.length ? `По сравнению с ${dateLabel(previousSearch.query.event_date)} исключены из прошлой подборки: ${removed.join(', ')} — заняты ${dateLabel(query.event_date)}.` : `Изменена только дата: ${dateLabel(previousSearch.query.event_date)} → ${dateLabel(query.event_date)}; совпадений по календарю было ${previousSearch.result.total}, стало ${result.total}.`;
}
async function runSearch() {
  if (busy) return;
  capture();
  if (state.unverified_requirements.length) {
    const box = document.getElementById('requirements-review');
    box.classList.add('needs-attention');
    document.getElementById('requirements-status').textContent = 'Для каждого пункта ниже уточните описание или явно согласитесь продолжить без его проверки.';
    box.scrollIntoView({block:'start',behavior:'instant'});
    box.querySelector('input:not(:checked)')?.focus({preventScroll:true});
    announce('Подбор ещё не запущен. Нужно решить, как учитывать перечисленные условия.');
    return;
  }
  const query = structuredClone(state);
  busy = true; error = ''; render();
  try {
    const result = await api('/api/matches',query);
    result.date_change = dateChange(query,result);
    activeResults = result;
    previousSearch = {query,result};
    busy = false;
    navigate(result.status === 'matches_found' ? 'results' : result.status === 'category_unavailable' ? 'no-category' : 'no-matches');
    announce(result.message);
  } catch (e) { busy = false; error = requestError(e); render(); }
}
function requestError(e) { return ['TimeoutError','AbortError'].includes(e.name) ? 'Сервис не ответил вовремя. Условия сохранены, попробуйте ещё раз.' : e.message; }
function render() {
  if (!options) return;
  cancelDictation();
  let route = location.hash.slice(1) || 'home';
  document.body.dataset.page = route.split('/')[0];
  if (['results','no-category','no-matches'].includes(route) || route.startsWith('profile/')) {
    if (!activeResults) { navigate('review'); return; }
    if (!route.startsWith('profile/')) {
      const actual = activeResults.status === 'matches_found' ? 'results' : activeResults.status === 'category_unavailable' ? 'no-category' : 'no-matches';
      if (actual !== route) { navigate(actual); return; }
    }
  }
  sidebar(route);
  if (busy && route !== 'brief') { main.setAttribute('aria-busy','true'); main.innerHTML = '<div class="loading-state" role="status"><span class="spinner" aria-hidden="true"></span>Проверяем запрос…</div>'; return; }
  main.setAttribute('aria-busy',String(busy));
  const screens = {home:homePage,brief:briefPage,event:eventPage,preferences:preferencesPage,review:reviewPage,results:resultsPage,'no-category':emptyPage,'no-matches':emptyPage};
  if (route.startsWith('profile/')) main.innerHTML = profilePage(route.split('/')[1]);
  else if (screens[route]) main.innerHTML = screens[route]();
  else { navigate('home'); return; }
  loadCalendar();
  animatePage();
  if (error) { main.insertAdjacentHTML('afterbegin',`<p class="error request-error" role="alert" tabindex="-1">${esc(error)}</p>`); main.querySelector('.request-error').focus(); }
  else main.focus({preventScroll:true});
  document.title = (route === 'home' ? 'Ваше событие начинается здесь' : route === 'brief' ? 'AI-подбор подрядчиков' : STEPS.find(s => s[0] === route)?.[1] || 'Подбор подрядчиков') + ' · EventMatch';
  window.scrollTo({top:0,behavior:'instant'});
}
document.addEventListener('input',e => { if (e.target.id === 'brief') rawBrief = e.target.value; });
document.addEventListener('change',e => {
  if (e.target.name !== 'ignored_requirement') return;
  capture();
  document.getElementById('requirements-review').classList.remove('needs-attention');
  document.getElementById('requirements-status').textContent = state.unverified_requirements.length ? 'Осталось уточнить: '+state.unverified_requirements.length+'.' : 'Эти условия не будут проверяться. Теперь подтвердите остальные параметры ниже.';
});
document.addEventListener('click',e => {
  if (e.target.closest('.skip-link')) { e.preventDefault(); main.focus(); return; }
  if (busy) { if (e.target.closest('a,button')) e.preventDefault(); return; }
  const sample = e.target.closest('[data-example]');
  if (sample) {
    resetConversation(); rawBrief = ''; error = ''; activeResults = null; reviewRequirements = [];
    state = sample.dataset.example === 'hosts' ? {...blankQuery(),city:'Алматы',event_date:'2026-10-06',event_type:'корпоратив',category:'Ведущий',budget_kzt:1300000,duration_hours:6,language:'русский'} : {...blankQuery(),city:'Астана',event_date:sample.dataset.example === 'rare' ? '2026-10-01' : '2026-10-02',event_type:'свадьба',category:'Флорист',budget_kzt:300000,language:'русский'};
    navigate('review'); return;
  }
  const occasion = e.target.closest('[data-occasion]');
  if (occasion) { capture(); state.event_type = occasion.dataset.occasion; activeResults = null; navigate('event'); return; }
  const alternative = e.target.closest('[data-alternative-date]');
  if (alternative) { state.event_date = alternative.dataset.alternativeDate; activeResults = null; calendarMonth = state.event_date.slice(0,7); navigate('review'); announce('Выбрана новая дата. Проверьте остальные условия и подтвердите подбор.'); return; }
  const nearby = e.target.closest('[data-nearby-review]');
  if (nearby) { navigate('review'); announce('Исходные условия сохранены. Измените только те параметры, которые готовы пересмотреть, и подтвердите поиск.'); return; }
  const go = e.target.closest('[data-go]');
  if (go && !go.disabled) { capture(); error = ''; navigate(go.dataset.go); }
});
document.addEventListener('submit',async e => {
  e.preventDefault(); if (busy || dictation || !e.target.reportValidity()) return;
  error = '';
  if (e.target.id === 'brief-form') {
    await sendConsultation(new FormData(e.target).get('text'));
  } else if (e.target.id === 'review-form') await runSearch();
  else { capture(); navigate(e.target.id === 'event-form' ? 'preferences' : 'review'); }
});
window.addEventListener('hashchange',() => { capture(); error = ''; render(); });
async function start() {
  main.innerHTML = '<div class="loading-state" role="status">Загружаем каталог…</div>';
  try { options = await api('/api/options'); render(); }
  catch { main.innerHTML = '<div class="form-card"><h1>Сервис недоступен</h1><p>Не удалось загрузить каталог. Откройте приложение через запущенный сервер и повторите попытку.</p><button class="btn btn-primary" id="retry-start">Повторить</button></div>'; document.getElementById('retry-start').onclick = start; }
}
start();
