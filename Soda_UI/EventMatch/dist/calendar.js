'use strict';

let calendarMonth = '', calendarData = null, calendarMessage = '', calendarRequest = 0, calendarFocus = '';
const monthLabel = value => new Intl.DateTimeFormat('ru-RU',{month:'long',timeZone:'UTC'}).format(new Date(value+'-01T12:00:00Z'));
const calendarQuery = () => ({...structuredClone(state),event_date:''});
const shiftDate = (date,days) => { const d = new Date(date+'T12:00:00Z'); d.setUTCDate(d.getUTCDate()+days); return d.toISOString().slice(0,10); };

function calendarField() {
  return `<section class="calendar-field" aria-label="Выбор даты и занятость"><div class="date-field-head"><div><label for="event_date">Дата мероприятия</label><p class="hint" id="date-hint">Выберите день в календаре или введите дату.</p></div><input id="event_date" name="event_date" type="date" required value="${esc(state.event_date)}" min="${options.first_date}" max="${options.last_date}" aria-describedby="date-hint"></div><div id="availability-calendar"></div></section>`;
}

async function loadCalendar() {
  const request = ++calendarRequest;
  if (!document.getElementById('availability-calendar')) return;
  calendarData = null;
  calendarMonth = (state.event_date || calendarMonth || options.first_date).slice(0,7);
  if (calendarMonth < options.first_date.slice(0,7) || calendarMonth > options.last_date.slice(0,7)) calendarMonth = options.first_date.slice(0,7);
  if (!state.city || !state.category) {
    calendarMessage = 'Выберите город и подрядчика, чтобы увидеть занятость. Дату уже можно указать.';
    paintCalendar(); return;
  }
  calendarMessage = 'Проверяем календарь по выбранным условиям…'; paintCalendar();
  try {
    const data = await api('/api/calendar',calendarQuery());
    if (request !== calendarRequest || !document.getElementById('availability-calendar')) return;
    calendarData = data;
    calendarMessage = data.candidate_count ? `Подходят по указанным условиям: ${data.candidate_count}. В ячейках — сколько свободно на день.` : 'По остальным условиям подрядчиков нет. Изменение только даты не поможет: проверьте категорию, бюджет и другие условия.';
    paintCalendar();
  } catch (e) {
    if (request !== calendarRequest || !document.getElementById('availability-calendar')) return;
    calendarMessage = 'Не удалось проверить занятость. '+requestError(e);
    paintCalendar();
  }
}

function paintCalendar(focus = '') {
  const host = document.getElementById('availability-calendar');
  if (!host) return;
  const active = document.activeElement;
  const restoreDate = focus || (host.contains(active) ? active.dataset.date : '') || '';
  const restoreMonth = host.contains(active) ? active.dataset.month : '';
  const months = [];
  for (let month = Number(options.first_date.slice(5,7)); month <= Number(options.last_date.slice(5,7)); month++) months.push('2026-'+String(month).padStart(2,'0'));
  const first = calendarMonth+'-01', start = new Date(first+'T12:00:00Z'), offset = (start.getUTCDay()+6)%7;
  const length = new Date(Date.UTC(start.getUTCFullYear(),start.getUTCMonth()+1,0)).getUTCDate();
  const map = new Map((calendarData?.days || []).map(day=>[day.date,day]));
  const earliest = first < options.first_date ? options.first_date : first;
  const tabDate = restoreDate || (calendarFocus.startsWith(calendarMonth) ? calendarFocus : state.event_date.startsWith(calendarMonth) ? state.event_date : earliest);
  let cells = '<span class="calendar-pad" aria-hidden="true"></span>'.repeat(offset);
  for (let day = 1; day <= length; day++) {
    const date = calendarMonth+'-'+String(day).padStart(2,'0'), known = date >= options.first_date && date <= options.last_date;
    const entry = map.get(date), hasCandidates = calendarData?.candidate_count > 0;
    const level = !entry || !hasCandidates ? 'unknown' : entry.free === 0 ? 'full' : entry.busy/calendarData.candidate_count >= .7 ? 'high' : 'open';
    const label = !known ? 'нет данных' : !entry ? 'занятость не проверена' : !hasCandidates ? 'нет подходящих подрядчиков' : `свободно ${entry.free}, заняты ${entry.busy}`;
    cells += `<button type="button" class="calendar-day ${level}" data-date="${date}" ${known ? '' : 'disabled'} tabindex="${known && date === tabDate ? '0' : '-1'}" aria-pressed="${date === state.event_date}" aria-label="${dateLabel(date)}: ${label}"><span>${day}</span><small>${known && entry && hasCandidates ? entry.free+' своб.' : '—'}</small></button>`;
  }
  const month = calendarData?.months.find(m=>m.month===calendarMonth), selected = map.get(state.event_date);
  host.innerHTML = `<div class="month-list" aria-label="Месяц календаря">${months.map(m=>{const stats=calendarData?.months.find(x=>x.month===m);return `<button type="button" data-month="${m}" aria-pressed="${m===calendarMonth}"><span>${monthLabel(m)}</span><small>${stats?.busy_percent == null ? 'Нет оценки' : stats.busy_percent+'% занято'}</small></button>`;}).join('')}</div><div class="calendar-heading"><strong>${monthLabel(calendarMonth)} <span>2026</span></strong><span>${month?.busy_percent == null ? 'Занятость не рассчитана' : month.free_days+' дней со свободными вариантами'}</span></div><div class="weekdays" aria-hidden="true">${['Пн','Вт','Ср','Чт','Пт','Сб','Вс'].map(d=>`<span>${d}</span>`).join('')}</div><div class="calendar-grid" role="group" aria-label="Дни ${monthLabel(calendarMonth)}">${cells}</div><div class="calendar-legend"><span><i class="legend-open"></i>Есть свободные</span><span><i class="legend-high"></i>Большинство занято</span><span><i class="legend-full"></i>Все заняты</span></div><p class="calendar-status" role="status">${esc(calendarMessage)}${selected && calendarData.candidate_count ? ` На ${dateLabel(state.event_date)}: свободно ${selected.free} из ${calendarData.candidate_count}.` : ''}</p><p class="hint">Процент за месяц — средняя доля занятых подрядчиков в день. Учитываются только указанные проверяемые условия; пожелания не меняют занятость. ${calendarMonth === '2026-09' ? 'В сентябре есть данные только за 23–30 число.' : ''} Нет отметки о занятости — по данным каталога, без гарантии бронирования.</p>`;
  if (restoreDate) host.querySelector(`[data-date="${restoreDate}"]`)?.focus({preventScroll:true});
  else if (restoreMonth) host.querySelector(`[data-month="${restoreMonth}"]`)?.focus({preventScroll:true});
}

document.addEventListener('click',e=>{
  const day = e.target.closest('[data-date]'), month = e.target.closest('[data-month]');
  if (day && !day.disabled) {
    state.event_date = day.dataset.date; calendarFocus = state.event_date; activeResults = null;
    document.getElementById('event_date').value = state.event_date;
    paintCalendar(state.event_date);
    announce('Выбрана дата '+dateLabel(state.event_date));
  }
  if (month) { calendarMonth = month.dataset.month; calendarFocus = ''; paintCalendar(); document.querySelector(`[data-month="${calendarMonth}"]`)?.focus({preventScroll:true}); }
});
document.addEventListener('change',e=>{
  if (!e.target.closest('form[data-query]')) return;
  capture();
  if (e.target.name === 'event_date') calendarFocus = '';
  if (document.getElementById('availability-calendar')) loadCalendar();
});
document.addEventListener('keydown',e=>{
  const day = e.target.closest('[data-date]');
  if (!day) return;
  const date = day.dataset.date, weekDay = (new Date(date+'T12:00:00Z').getUTCDay()+6)%7;
  const steps = {ArrowLeft:-1,ArrowRight:1,ArrowUp:-7,ArrowDown:7,Home:-weekDay,End:6-weekDay};
  if (!(e.key in steps)) return;
  e.preventDefault();
  const next = shiftDate(date,steps[e.key]);
  if (next < options.first_date || next > options.last_date) return;
  calendarMonth = next.slice(0,7); calendarFocus = next; paintCalendar(next);
});
