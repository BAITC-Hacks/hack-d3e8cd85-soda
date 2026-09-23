'use strict';

let conversation = [], consultation = null, styleAsked = false;
const readyMessage = 'Основное собрали. Проверьте условия — после вашего подтверждения начнётся подбор.';
function resetConversation() { conversation = []; consultation = null; styleAsked = false; }
function briefSource() { return conversation.length ? conversation.map(m => (m.role === 'user' ? 'Вы: ' : 'Консультант: ') + m.text).join('\n\n') : rawBrief; }
function consultantBuddy(thinking = false) { return `<span class="consultant-buddy ${thinking ? 'thinking' : ''}" aria-hidden="true"><i></i><i></i><span></span></span>`; }
function liveSummary() {
  const facts = [['Город',state.city],['Дата',state.event_date ? dateLabel(state.event_date) : ''],['Событие',state.event_type],['Специализация',state.category],['Бюджет',state.budget_kzt ? money(state.budget_kzt) : '']];
  const complete = facts.filter(([,value]) => value).length;
  return `<aside class="live-summary" aria-labelledby="summary-title"><div class="summary-decoration" aria-hidden="true">✳</div><span class="eyebrow">Собираем вашу идею</span><h2 id="summary-title">Мы поняли <em>так.</em></h2><div class="summary-progress" aria-label="Заполнено ${complete} из 5 обязательных параметров">${facts.map(([,v]) => `<span class="${v ? 'filled' : ''}"></span>`).join('')}</div><dl>${facts.map(([label,value]) => `<div class="${value ? 'known' : ''}"><dt>${esc(label)}</dt><dd>${esc(value || 'Ещё уточним')}</dd></div>`).join('')}${state.language ? `<div class="known"><dt>Язык</dt><dd>${esc(state.language)}</dd></div>` : ''}${state.duration_hours ? `<div class="known"><dt>Длительность</dt><dd>${esc(state.duration_hours)} ч</dd></div>` : ''}</dl>${state.wishes.length ? `<p class="summary-wishes">${esc(state.wishes.map(id=>options.wishes.find(w=>w.id===id)?.label).filter(Boolean).join(' · '))}</p>` : ''}${state.unverified_requirements.length ? '<p class="hint">Есть условия, которые нужно отдельно проверить перед подбором.</p>' : ''}<p class="hint">Любой пункт можно исправить. Цена в каталоге — «от».</p>${button('Изменить поля','review',false,'sliders')}</aside>`;
}
function briefPage() {
  const ready = consultation?.focus === 'review';
  const messages = conversation.length ? conversation : [{role:'assistant',text:'Расскажите, какое событие задумали. Если ещё не знаете, кто нужен, начнём с вашей идеи — разберёмся вместе.'}];
  return `<div class="consultant-heading"><div><span class="eyebrow">AI-консультант · в вашем темпе</span><h1>Начнём с идеи.<br><em>Остальное уточним.</em></h1></div><div class="buddy-greeting">${consultantBuddy(busy)}<span>Пару деталей —<br>и картинка сложится.</span></div></div><div class="consultant-layout"><section class="conversation-panel" aria-label="Диалог о мероприятии"><div class="conversation-top"><span class="conversation-status"><span aria-hidden="true"></span>${options.ai_enabled ? 'Ваш консультант' : 'Ручной подбор доступен'}</span>${conversation.length ? '<button type="button" class="text-button" data-reset-conversation>Новое событие</button>' : ''}</div><div class="conversation-log" aria-label="История диалога">${messages.map((m,i)=>`<article class="chat-message ${m.role === 'user' ? 'from-user' : 'from-assistant'}" ${i === messages.length-1 ? 'id="latest-message" tabindex="-1"' : ''}><span class="message-author">${m.role === 'user' ? 'Вы' : 'EventMatch'}</span><p>${esc(m.text)}</p></article>`).join('')}${busy ? `<article class="chat-message from-user"><span class="message-author">Вы</span><p>${esc(rawBrief)}</p></article><div class="thinking-message" role="status"><span class="thinking-dots" aria-hidden="true"><i></i><i></i><i></i></span>Вникаю в детали…</div>` : ''}</div>${!busy && consultation?.choices?.length ? `<div class="answer-choices" aria-label="Варианты ответа">${consultation.choices.map((c,i)=>`<button type="button" data-consult-choice="${i}">${esc(c)}${icon('arrow')}</button>`).join('')}</div>` : ''}${!busy && consultation?.focus === 'style' ? '<button type="button" class="text-button skip-style" data-skip-style>Пропустить — стиль не принципиален</button>' : ''}${ready && !busy ? `<div class="conversation-ready">${icon('check')}<p>Вы управляете условиями. Подбор начнётся после проверки.</p>${button('Проверить условия','review',true)}</div>` : ''}<form id="brief-form" class="conversation-composer"><label for="brief">${conversation.length ? 'Ваш ответ или уточнение' : 'Что вы планируете?'}</label><textarea id="brief" name="text" rows="3" maxlength="2000" required ${busy ? 'disabled' : ''} placeholder="Например: хочу камерную свадьбу в Алматы, но пока не знаю, кто поможет с оформлением.">${esc(rawBrief)}</textarea>${voiceControls()}<div class="actions"><button class="btn btn-primary" type="submit" ${busy || !options.ai_enabled ? 'disabled' : ''}>Отправить ${icon('arrow')}</button>${button('Заполнить вручную','review',false,'sliders')}</div>${!options.ai_enabled ? '<p class="service-note">Диалог сейчас недоступен. Условия можно заполнить вручную.</p>' : ''}<p class="privacy-note">Сообщения и запись отправляются в OpenAI. Не указывайте личные данные гостей. Вопросы помогают уточнить запрос и не подтверждают услуги подрядчиков.</p></form></section>${liveSummary()}</div>${demoExamples()}`;
}
async function sendConsultation(text) {
  if (busy || dictation || !text?.trim()) return;
  rawBrief = text.trim();
  const messages = [...conversation,{role:'user',text:rawBrief}];
  const payload = {query:structuredClone(state),messages,style_asked:styleAsked};
  if (messages.length > 21 || new TextEncoder().encode(JSON.stringify(payload)).length > 16000) {
    error = 'Диалог получился длинным. Все ответы сохранены — продолжите в полях условий.'; render(); return;
  }
  busy = true; error = ''; render();
  document.querySelector('.thinking-message')?.scrollIntoView({block:'nearest',behavior:'instant'});
  try {
    const answer = await api('/api/consultations',payload);
    state = {...blankQuery(),...answer.query,wishes:answer.query.wishes || [],unverified_requirements:answer.query.unverified_requirements || []};
    for (const key of ['city','category','event_type','event_date','language']) state[key] ||= '';
    reviewRequirements = [...new Set(state.unverified_requirements)];
    state.unverified_requirements = [...reviewRequirements];
    consultation = answer;
    styleAsked ||= answer.focus === 'style';
    conversation = [...messages,{role:'assistant',text:answer.focus === 'review' ? readyMessage : answer.question}];
    rawBrief = ''; activeResults = null; busy = false; render();
    if (location.hash === '#brief') {
      const latest = document.getElementById('latest-message');
      latest?.focus({preventScroll:true}); latest?.scrollIntoView({block:'nearest',behavior:'instant'});
      announce(conversation.at(-1).text);
    }
  } catch (e) { busy = false; error = requestError(e); render(); }
}
document.addEventListener('click',e => {
  if (busy || dictation) return;
  const choice = e.target.closest('[data-consult-choice]');
  if (choice) { sendConsultation(consultation.choices[Number(choice.dataset.consultChoice)]); return; }
  if (e.target.closest('[data-skip-style]')) {
    conversation.push({role:'user',text:'Стиль не принципиален.'},{role:'assistant',text:readyMessage});
    styleAsked = true; consultation = {focus:'review',choices:[]}; render(); announce(readyMessage);
  }
  if (e.target.closest('[data-reset-conversation]')) {
    resetConversation(); state = blankQuery(); rawBrief = ''; reviewRequirements = []; activeResults = null; previousSearch = null; error = ''; render(); document.getElementById('brief')?.focus();
  }
});
