'use strict';
const PATHS={arrow:'M5 12h14m-5-5 5 5-5 5',back:'M19 12H5m5-5-5 5 5 5',check:'m5 12 4 4L19 6',pin:'M20 10c0 6-8 11-8 11S4 16 4 10a8 8 0 1 1 16 0ZM15 10a3 3 0 1 1-6 0 3 3 0 0 1 6 0',calendar:'M5 5h14v15H5zM8 3v4m8-4v4M5 10h14',sparkles:'m12 3 2.4 6.6L21 12l-6.6 2.4L12 21l-2.4-6.6L3 12l6.6-2.4L12 3',mic:'M9 5a3 3 0 0 1 6 0v7a3 3 0 0 1-6 0V5ZM6 11v1a6 6 0 0 0 12 0v-1M12 18v4m-4 0h8',camera:'M3 7h4l2-3h6l2 3h4v13H3zM16 13a4 4 0 1 1-8 0 4 4 0 0 1 8 0',building:'M4 21V9l8-6 8 6v12M2 21h20M9 21v-7h6v7M8 10h0m8 0h0',flower:'M12 20v-8m0 7c-4 0-6-2-6-4 4 0 6 2 6 4m0-1c4 0 6-2 6-4-4 0-6 2-6 4M12 4c-5-5-9 2-4 4-5 5 3 8 4 3 3 5 9 0 4-3 5-3 0-8-4-4',heart:'M20 5c-3-3-6-1-8 1-2-2-5-4-8-1-6 6 8 15 8 15s14-9 8-15Z',gift:'M3 8h18v4H3zM5 12v9h14v-9M12 8v13M12 8C2 8 6-2 12 8c6-10 10 0 0 0',image:'M3 3h18v18H3zM3 17l6-6 5 5 3-3 4 4M16 7h0',bed:'M3 4v17m18-10v10M3 16h18M3 8h6v8M9 10h10a2 2 0 0 1 2 2v4',music:'M9 18V5l11-2v13M9 5v4l11-2M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0M20 16a3 3 0 1 1-6 0 3 3 0 0 1 6 0',info:'M12 16v-5m0-4h0M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',search:'M16 10a6 6 0 1 1-12 0 6 6 0 0 1 12 0m-2 4 7 7',clock:'M12 7v5l3 2M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',shield:'M12 3 4 6v6c0 5 8 9 8 9s8-4 8-9V6l-8-3Zm-4 9 3 3 5-6',sliders:'M4 7h16M4 17h16M8 4v6m8 4v6',globe:'M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0M2 12h20M12 2c-6 6-6 14 0 20 6-6 6-14 0-20'};
const icon=name=>`<svg viewBox="0 0 24 24" aria-hidden="true"><path d="${PATHS[name]||PATHS.sparkles}"/></svg>`;
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;'}[c]));
const money = n => new Intl.NumberFormat('ru-RU').format(n) + ' ₸';
const dateLabel = d => /^\d{4}-\d{2}-\d{2}$/.test(d) && !Number.isNaN(Date.parse(d)) ? new Intl.DateTimeFormat('ru-RU', {timeZone:'UTC'}).format(new Date(d + 'T12:00:00Z')) : 'Не указана';
const main = document.getElementById('main');
const STEPS = [['event','Событие','Город, дата и формат'], ['category','Подрядчик','Кто вам нужен'], ['preferences','Условия','Бюджет и пожелания'], ['review','Проверка','Проверьте перед подбором']];
const blankQuery = () => ({city:'', event_date:'', event_type:'', category:'', budget_kzt:null, duration_hours:null, language:'', wishes:[], unverified_requirements:[]});
let state = blankQuery(), options = null, activeResults = null, previousSearch = null, busy = false, rawBrief = '', error = '';
const categoryIcons = {'Ведущий':'mic','Фотограф':'camera','Видеограф':'camera','Флорист':'flower','Декоратор':'sparkles','Банкетный зал':'building','Ресторан':'building','Отель':'bed','Загородная площадка':'building','Подарки и сувениры':'gift','Ведущий церемонии':'heart','Фото и видеобудки':'image','Инструменталист':'music','Лайв-бэнд':'music','Национальный ансамбль':'music','Танцевальный коллектив':'sparkles','Шоу-программа':'sparkles'};
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
    if (k === 'wish') continue;
    if (k === 'unverified_requirements') state[k] = String(v).split('\n').map(s => s.trim()).filter(Boolean);
    else state[k] = ['budget_kzt','duration_hours'].includes(k) ? (v === '' ? null : Number(v)) : v;
  }
  if (form.querySelector('[data-wishes]')) state.wishes = data.getAll('wish').sort();
  if (JSON.stringify(state) !== before) activeResults = null;
}
function sidebar(route) {
  const active = STEPS.findIndex(s => s[0] === route);
  document.getElementById('step-nav').innerHTML = STEPS.map(([key,name,sub], i) => `<button type="button" data-go="${key}" class="step-button ${i === active ? 'active' : ''}" ${busy ? 'disabled' : ''} ${i === active ? 'aria-current="step"' : ''}><span class="number">${i+1}</span><span><strong>${name}</strong><small>${sub}</small></span></button>`).join('');
  document.getElementById('brief-data').innerHTML = [['Город',state.city || 'Не указан'],['Дата',dateLabel(state.event_date)],['Формат',state.event_type || 'Не указан'],['Категория',state.category || 'Не указана'],['Бюджет',state.budget_kzt ? money(state.budget_kzt) : 'Не указан']].map(([k,v]) => `<div><dt>${k}</dt><dd>${esc(v)}</dd></div>`).join('');
}
function briefPage() {
  return header('AI-консультант','Расскажите о событии.<br>Найдём <em>ваших людей.</em>','Опишите мероприятие своими словами. Мы предложим условия для вашего подтверждения.') +
    `<form id="brief-form" class="content-form"><div class="form-card"><label for="brief">Какое мероприятие вы планируете?</label><textarea id="brief" name="text" rows="5" maxlength="2000" required placeholder="Ведущий на корпоратив в Алматы 6 октября 2026, до 1 300 000 тенге, на русском, на 6 часов. Хочется спокойной атмосферы.">${esc(rawBrief)}</textarea>${voiceControls()}<p class="hint">Город, дата, формат, категория и бюджет обязательны. Язык, часы работы и пожелания — по желанию.</p><p class="hint">Текст отправляется в OpenAI для разбора. Не указывайте контакты и личные данные гостей.</p>${!options.ai_enabled ? '<p class="error">AI-консультант пока недоступен. Условия можно заполнить вручную.</p>' : ''}</div><div class="actions">${button('Заполнить вручную','event',false,'sliders')}<button class="btn btn-primary" type="submit" ${!options.ai_enabled ? 'disabled' : ''}>Понять запрос ${icon('sparkles')}</button></div></form><div class="helper-note">${icon('shield')}<span>Никого не бронируем. Сначала вы проверите параметры, затем получите до трёх рекомендаций.</span></div><section class="demo-examples" aria-label="Примеры по исходному каталогу"><p class="hint">Можно начать с проверенного примера:</p><div class="actions"><button class="btn" data-example="hosts">Ведущие в Алматы</button><button class="btn" data-example="rare">Флорист в Астане</button><button class="btn" data-example="empty">Занято на дату</button></div></section>`;
}
function eventFields() {
  return selectField('Город','city',options.cities,state.city) + selectField('Формат мероприятия','event_type',options.event_types,state.event_type) + `<div class="field"><label for="event_date">Дата мероприятия</label><input id="event_date" name="event_date" type="date" required value="${esc(state.event_date)}" min="${options.first_date}" max="${options.last_date}" aria-describedby="date-hint"><p class="hint" id="date-hint">Календарь доступен с 23.09.2026 по 31.12.2026.</p></div>`;
}
function eventPage() {
  return header('01 / Мероприятие','Каждое событие начинается<br>с <em>деталей.</em>','Укажите место, дату и повод для встречи.') + `<form id="event-form" data-query class="content-form"><div class="form-card">${eventFields()}</div><div class="actions">${button('К описанию','brief',false,'back')}<button type="submit" class="btn btn-primary">Выбрать подрядчика ${icon('arrow')}</button></div></form>`;
}
function categoryPage() {
  return header('02 / Подрядчик','Кто поможет воплотить<br><em>вашу идею?</em>','Выберите одну категорию. Проверим доступность и условия по каталогу.') + `<div class="category-grid" role="radiogroup" aria-label="Категория подрядчика">${options.categories.map((name,i) => `<button type="button" role="radio" aria-checked="${state.category === name}" tabindex="${state.category === name || !state.category && i === 0 ? '0' : '-1'}" class="category-option" data-category="${esc(name)}"><span class="category-icon">${icon(categoryIcons[name])}</span><strong>${esc(name)}</strong><span class="radio-mark" aria-hidden="true"></span></button>`).join('')}</div><div class="actions">${button('Назад','event',false,'back')}<button class="btn btn-primary" data-go="preferences" ${!state.category ? 'disabled' : ''}>Указать условия ${icon('arrow')}</button></div>`;
}
function preferenceFields() {
  return `<div class="field"><label for="budget_kzt">Максимальный бюджет, ₸</label><input id="budget_kzt" name="budget_kzt" type="number" min="1" max="1000000000" step="1" required value="${state.budget_kzt ?? ''}" aria-describedby="budget-hint"><p class="hint" id="budget-hint">На одного подрядчика за мероприятие. Цена «от» не гарантирует итоговую стоимость.</p></div><div class="field-row"><div class="field"><label for="duration_hours">Часы работы подрядчика</label><input id="duration_hours" name="duration_hours" type="number" min="0.1" max="168" step="any" placeholder="Не важно" value="${state.duration_hours ?? ''}"></div>${selectField('Язык работы','language',options.languages,state.language,false)}</div>`;
}
function wishFields() {
  return `<fieldset data-wishes class="wish-fieldset"><legend>Пожелания к стилю — необязательно</legend><p class="hint">Влияют на порядок, но не являются гарантией услуги. Выберите то, что действительно важно.</p>${!options.semantic_enabled ? '<p class="hint">Смысловой подбор пока недоступен: сейчас результаты будут упорядочены по начальной цене.</p>' : ''}<div class="wish-grid">${options.wishes.map(w => `<label class="wish-option"><input type="checkbox" name="wish" value="${w.id}" ${state.wishes.includes(w.id) ? 'checked' : ''}><span>${esc(w.label)}</span></label>`).join('')}</div></fieldset>`;
}
function unresolvedFields() {
  if (!state.unverified_requirements.length) return '';
  return `<div class="form-card unresolved"><label for="unverified_requirements">Нужно уточнить перед подбором</label><p class="hint">Эти условия нельзя проверить по доступным полям или учесть выбранными пожеланиями. Уточните описание события либо уберите их из этого поля, только если согласны продолжить без них.</p><textarea id="unverified_requirements" name="unverified_requirements" rows="3">${esc(state.unverified_requirements.join('\n'))}</textarea></div>`;
}
function preferencesPage() {
  return header('03 / Условия','Подходящий вариант.<br><em>В рамках бюджета.</em>','Язык и длительность станут строгими условиями, если вы их укажете.') + `<form id="preferences-form" data-query class="content-form"><div class="form-card">${preferenceFields()}</div>${wishFields()}${unresolvedFields()}<div class="actions">${button('Назад','category',false,'back')}<button type="submit" class="btn btn-primary">Проверить параметры ${icon('arrow')}</button></div></form>`;
}
function reviewPage() {
  return header('04 / Подтверждение','Всё верно?<br>Найдём <em>подходящих.</em>','Проверьте распознанные условия. Любое поле можно исправить перед поиском.') + `${rawBrief ? `<details class="source-brief"><summary>Исходное описание</summary><p>${esc(rawBrief)}</p>${button('Уточнить описание','brief',false,'back')}</details>` : ''}<form id="review-form" data-query><div class="summary-grid"><section class="summary-card"><h2>Ваше мероприятие</h2>${eventFields()}</section><section class="summary-card"><h2>Ваш подрядчик</h2>${selectField('Категория','category',options.categories,state.category)}${preferenceFields()}</section></div>${wishFields()}${unresolvedFields()}<div class="review-note">${icon('sparkles')}<p><strong>До трёх вариантов, с конкретными причинами.</strong>Сначала проверим условия и занятость, затем учтём выбранные пожелания.</p></div><div class="actions">${button('К описанию','brief',false,'back')}<button type="submit" class="btn btn-primary">Подтвердить и подобрать ${icon('arrow')}</button></div></form>`;
}
function chips() {
  const values = [state.city,dateLabel(state.event_date),state.event_type,state.category,'До ' + money(state.budget_kzt)];
  if (state.duration_hours) values.push(state.duration_hours + ' ч работы');
  if (state.language) values.push(state.language);
  for (const id of state.wishes) values.push(options.wishes.find(w => w.id === id)?.label || id);
  return `<div class="chips">${values.map(v => `<span class="chip">${esc(v)}</span>`).join('')}</div>`;
}
function provenance(p) {
  return `<span class="synthetic">${p.data_origin === 'team' ? (p.synthetic ? 'Синтетический профиль команды' : 'Добавлено командой') : p.synthetic ? 'Синтетический профиль организаторов' : 'Исходный каталог · анонимизировано'}</span>${p.price_imputed ? '<p class="hint">Цена дополнена при подготовке данных.</p>' : ''}${p.city_imputed ? '<p class="hint">Город дополнен при подготовке данных.</p>' : ''}`;
}
const initials = p => p.anon_name.split(' ').slice(0,2).map(x => x[0]).join('');
function card(p,i) {
  return `<article class="contractor-card"><div class="contractor-top"><div class="avatar ${i % 2 ? 'secondary' : ''}" aria-hidden="true">${esc(initials(p))}</div><div><h2>${esc(p.anon_name)}</h2><div class="meta">${esc(p.matched_category)} · ${esc(p.city)}</div></div></div>${provenance(p)}<div class="price"><small>От </small>${money(p.price_from_kzt)}</div><p class="meta">За мероприятие · начальная цена</p><div class="available">${icon('check')}Нет отметки о занятости ${dateLabel(state.event_date)}</div><div class="match-reason"><strong>${icon('sparkles')}ПОЧЕМУ В ПОДБОРКЕ</strong><p>${esc(p.explanation)}</p></div>${button('Посмотреть профиль','profile/' + p.id)}</article>`;
}
function resultsPage() {
  const r = activeResults;
  return `<div class="result-heading"><div>${header('Ваша подборка',`Подходящих вариантов: <em>${r.cards.length}</em>`,'Выбор из исходного каталога по подтверждённым условиям.')}</div>${button('Изменить условия','review',false,'sliders')}</div>${chips()}<div class="result-note">${icon('info')}<span>${esc(r.message)}</span></div>${r.notice ? `<p class="result-note">${esc(r.notice)}</p>` : ''}<p class="result-note">Порядок: ${r.ranking === 'semantic' ? 'смысловая близость пожеланиям, затем начальная цена и id' : 'начальная цена, затем id'}.</p>${r.date_change ? `<p class="review-note">${esc(r.date_change)}</p>` : ''}<div class="contractor-grid">${r.cards.map(card).join('')}</div><p class="result-bottom">Цены «от» не являются окончательным предложением. Сведения и заявления взяты из каталога; бронирование не выполняется.</p>`;
}
function emptyPage() {
  const absent = activeResults.status === 'category_unavailable';
  return `<section class="empty-page"><div class="empty-icon">${icon(absent ? 'search' : 'calendar')}</div>${header('Результат подбора',absent ? 'В городе нет<br><em>такой категории.</em>' : 'По этим условиям<br><em>совпадений нет.</em>','Условия сохранены — их можно изменить и повторить поиск.')}${chips()}<div class="summary-card"><p>${esc(activeResults.message)}</p></div>${activeResults.date_change ? `<p class="review-note">${esc(activeResults.date_change)}</p>` : ''}${activeResults.notice ? `<p class="hint">${esc(activeResults.notice)}</p>` : ''}<div class="actions">${button('Изменить условия','review',true,'sliders')}${button('Изменить дату','event',false,'calendar')}</div></section>`;
}
function profilePage(id) {
  const p = activeResults?.cards.find(p => p.id === id);
  if (!p) { navigate('review'); return ''; }
  return `${button('К подборке','results',false,'back')}<div class="profile-layout"><article class="profile-main"><div class="profile-head"><div class="avatar" aria-hidden="true">${esc(initials(p))}</div><div><h1>${esc(p.anon_name)}</h1><p class="meta">${esc(p.categories.join(' · '))} · ${esc(p.city)}</p>${provenance(p)}</div></div><div class="match-reason"><strong>ПОЧЕМУ В ПОДБОРКЕ</strong><p>${esc(p.explanation)}</p></div><h2>Описание из каталога</h2><p>${esc(p.description)}</p><p class="hint">Текст профиля содержит заявления автора; награды и отзывы отдельно не проверялись. Для строгих условий используются поля ниже.</p><h2>Условия работы</h2><dl class="facts"><div><dt>Форматы</dt><dd>${esc(p.event_formats.join(' · '))}</dd></div><div><dt>Языки</dt><dd>${esc(p.languages.join(' · '))}</dd></div><div><dt>Длительность</dt><dd>${p.max_hours === null ? 'Присутствие на площадке не требуется' : 'До ' + p.max_hours + ' ч на площадке'}</dd></div></dl></article><aside class="profile-aside"><div class="eyebrow">Условия предложения</div><div class="price">От ${money(p.price_from_kzt)}</div><p>За мероприятие, окончательная цена может отличаться.</p><div class="available">${icon('check')}Нет отметки о занятости ${dateLabel(state.event_date)}</div><p>Бюджет: ${money(state.budget_kzt)}.</p>${button('К подборке','results',false,'back')}</aside></div>`;
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
  if (state.unverified_requirements.length) { error = 'Остались непроверяемые требования. Уточните их или явно уберите перед подбором.'; render(); return; }
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
  let route = location.hash.slice(1) || 'brief';
  if (['results','no-category','no-matches'].includes(route) || route.startsWith('profile/')) {
    if (!activeResults) { navigate('review'); return; }
    if (!route.startsWith('profile/')) {
      const actual = activeResults.status === 'matches_found' ? 'results' : activeResults.status === 'category_unavailable' ? 'no-category' : 'no-matches';
      if (actual !== route) { navigate(actual); return; }
    }
  }
  sidebar(route);
  if (busy) { main.setAttribute('aria-busy','true'); main.innerHTML = '<div class="loading-state" role="status"><span class="spinner" aria-hidden="true"></span>Проверяем запрос…</div>'; return; }
  main.removeAttribute('aria-busy');
  const screens = {brief:briefPage,event:eventPage,category:categoryPage,preferences:preferencesPage,review:reviewPage,results:resultsPage,'no-category':emptyPage,'no-matches':emptyPage};
  if (route.startsWith('profile/')) main.innerHTML = profilePage(route.split('/')[1]);
  else if (screens[route]) main.innerHTML = screens[route]();
  else { navigate('brief'); return; }
  if (error) { main.insertAdjacentHTML('afterbegin',`<p class="error request-error" role="alert" tabindex="-1">${esc(error)}</p>`); main.querySelector('.request-error').focus(); }
  else main.focus({preventScroll:true});
  document.title = (route === 'brief' ? 'AI-подбор подрядчиков' : STEPS.find(s => s[0] === route)?.[1] || 'Подбор подрядчиков') + ' · EventMatch';
  window.scrollTo({top:0,behavior:'instant'});
}
document.addEventListener('input',e => { if (e.target.id === 'brief') rawBrief = e.target.value; });
document.addEventListener('click',e => {
  if (e.target.closest('.skip-link')) { e.preventDefault(); main.focus(); return; }
  if (busy) { if (e.target.closest('a,button')) e.preventDefault(); return; }
  const sample = e.target.closest('[data-example]');
  if (sample) {
    rawBrief = ''; error = ''; activeResults = null;
    state = sample.dataset.example === 'hosts' ? {...blankQuery(),city:'Алматы',event_date:'2026-10-06',event_type:'корпоратив',category:'Ведущий',budget_kzt:1300000,duration_hours:6,language:'русский'} : {...blankQuery(),city:'Астана',event_date:sample.dataset.example === 'rare' ? '2026-10-01' : '2026-10-02',event_type:'свадьба',category:'Флорист',budget_kzt:300000,language:'русский'};
    navigate('review'); return;
  }
  const category = e.target.closest('[data-category]');
  if (category) { state.category = category.dataset.category; activeResults = null; render(); main.querySelector(`[data-category="${CSS.escape(state.category)}"]`).focus(); return; }
  const go = e.target.closest('[data-go]');
  if (go && !go.disabled) { capture(); error = ''; navigate(go.dataset.go); }
});
document.addEventListener('keydown',e => {
  const el = e.target.closest('[data-category]');
  if (!el || !['ArrowLeft','ArrowRight','ArrowUp','ArrowDown','Home','End'].includes(e.key)) return;
  e.preventDefault(); const buttons = [...main.querySelectorAll('[data-category]')];
  const i = buttons.indexOf(el), next = e.key === 'Home' ? 0 : e.key === 'End' ? buttons.length-1 : (i + (['ArrowLeft','ArrowUp'].includes(e.key) ? -1 : 1) + buttons.length) % buttons.length;
  buttons[next].click();
});
document.addEventListener('submit',async e => {
  e.preventDefault(); if (busy || dictation || !e.target.reportValidity()) return;
  error = '';
  if (e.target.id === 'brief-form') {
    rawBrief = new FormData(e.target).get('text'); busy = true; render();
    try {
      const q = await api('/api/briefs',{text:rawBrief});
      state = {...blankQuery(),...q,city:q.city || '',category:q.category || '',event_type:q.event_type || '',event_date:q.event_date || '',language:q.language || '',wishes:q.wishes || [],unverified_requirements:q.unverified_requirements || []};
      activeResults = null; busy = false; navigate('review');
    } catch (err) { busy = false; error = requestError(err); render(); }
  } else if (e.target.id === 'review-form') await runSearch();
  else { capture(); navigate(e.target.id === 'event-form' ? 'category' : 'review'); }
});
window.addEventListener('hashchange',() => { capture(); error = ''; render(); });
async function start() {
  main.innerHTML = '<div class="loading-state" role="status">Загружаем каталог…</div>';
  try { options = await api('/api/options'); document.getElementById('catalog-count').textContent = options.profile_count + ' профилей'; render(); }
  catch { main.innerHTML = '<div class="form-card"><h1>Сервис недоступен</h1><p>Не удалось загрузить каталог. Откройте приложение через запущенный сервер и повторите попытку.</p><button class="btn btn-primary" id="retry-start">Повторить</button></div>'; document.getElementById('retry-start').onclick = start; }
}
start();
