'use strict';

let dictation = null;
const voiceSupported = () => window.isSecureContext && !!navigator.mediaDevices?.getUserMedia && !!window.MediaRecorder;

function voiceControls() {
  const available = options.voice_enabled && voiceSupported();
  return `<div class="voice-controls"><div class="actions"><button type="button" class="btn" id="voice-toggle" ${available ? '' : 'disabled'}>${icon('mic')}Диктовать</button><button type="button" class="btn" id="voice-cancel" hidden>Отменить запись</button></div><p id="voice-status" class="hint" role="status" aria-live="polite">${!voiceSupported() ? 'Диктовка недоступна в этом браузере или соединении. Используйте localhost или HTTPS; текст можно ввести вручную.' : !options.voice_enabled ? 'Голосовой ввод пока недоступен. Пожелания можно напечатать.' : 'До 60 секунд. После остановки запись отправится в OpenAI, а текст появится здесь для вашей правки.'}</p></div>`;
}

function paintVoice(text) {
  const status = document.getElementById('voice-status');
  if (!status) return;
  status.textContent = text;
  const toggle = document.getElementById('voice-toggle');
  toggle.textContent = dictation?.phase === 'recording' ? 'Остановить и распознать' : 'Диктовать';
  toggle.disabled = !!dictation && dictation.phase !== 'recording';
  toggle.setAttribute('aria-pressed',String(dictation?.phase === 'recording'));
  document.getElementById('voice-cancel').hidden = !dictation;
  const field = document.getElementById('brief');
  if (field) field.readOnly = !!dictation;
  const submit = document.querySelector('#brief-form button[type=submit]');
  if (submit) submit.disabled = !!dictation || !options.ai_enabled;
}

function releaseMicrophone(session) {
  clearInterval(session.timer);
  session.stream?.getTracks().forEach(track => track.stop());
}

function cancelDictation() {
  const session = dictation;
  if (!session) return;
  dictation = null;
  session.cancelled = true;
  session.abort?.abort();
  if (session.recorder?.state === 'recording') session.recorder.stop();
  releaseMicrophone(session);
  paintVoice('Запись отменена. Введённый текст сохранён.');
}

async function startDictation() {
  if (dictation || busy || !options.voice_enabled || !voiceSupported()) return;
  rawBrief = document.getElementById('brief').value;
  const session = {phase:'permission',chunks:[],bytes:0,cancelled:false};
  dictation = session;
  paintVoice('Разрешите доступ к микрофону в браузере.');
  try {
    const mimeType = ['audio/webm;codecs=opus','audio/mp4'].find(t => MediaRecorder.isTypeSupported(t));
    if (!mimeType) throw new Error('Браузер не поддерживает подходящий формат записи. Введите пожелания текстом.');
    const stream = await navigator.mediaDevices.getUserMedia({audio:true});
    session.stream = stream;
    if (dictation !== session) { releaseMicrophone(session); return; }
    const recorder = new MediaRecorder(stream,{mimeType,audioBitsPerSecond:64000});
    session.recorder = recorder;
    recorder.ondataavailable = e => {
      if (session.cancelled || !e.data.size) return;
      session.chunks.push(e.data); session.bytes += e.data.size;
      if (session.bytes > 10 * 1024 * 1024) { cancelDictation(); paintVoice('Запись слишком большая. Попробуйте более короткую диктовку.'); }
    };
    recorder.onerror = () => { if (dictation !== session) return; cancelDictation(); paintVoice('Микрофон перестал записывать. Текст сохранён; попробуйте ещё раз.'); };
    recorder.onstop = () => finishDictation(session,mimeType);
    recorder.start(1000);
    session.phase = 'recording'; session.started = Date.now();
    paintVoice('Идёт запись: 0 из 60 секунд. Говорите о мероприятии и своих пожеланиях.');
    session.timer = setInterval(() => {
      if (dictation !== session || session.phase !== 'recording') return;
      const seconds = Math.floor((Date.now()-session.started)/1000);
      if (seconds >= 60) stopDictation();
      else paintVoice(`Идёт запись: ${seconds} из 60 секунд.`);
    },500);
  } catch (e) {
    if (dictation !== session) { releaseMicrophone(session); return; }
    cancelDictation();
    const message = e.name === 'NotAllowedError' ? 'Доступ к микрофону не разрешён. Измените разрешение в браузере или введите пожелания текстом.' : e.name === 'NotFoundError' ? 'Микрофон не найден. Подключите его или введите пожелания текстом.' : e.message;
    paintVoice(message);
  }
}

function stopDictation() {
  if (dictation?.recorder?.state !== 'recording') return;
  dictation.phase = 'transcribing';
  clearInterval(dictation.timer);
  dictation.recorder.stop();
  // Stop the hardware immediately; the final dataavailable event still arrives.
  releaseMicrophone(dictation);
  paintVoice('Распознаём речь…');
}

async function finishDictation(session,mimeType) {
  releaseMicrophone(session);
  if (dictation !== session || session.cancelled) return;
  session.phase = 'transcribing'; paintVoice('Распознаём речь…');
  const baseType = mimeType.split(';')[0], ext = baseType.includes('mp4') ? 'mp4' : 'webm';
  const audio = new Blob(session.chunks,{type:baseType});
  session.chunks = [];
  session.abort = new AbortController();
  const timeout = setTimeout(() => session.abort.abort(),10000);
  try {
    if (!audio.size || audio.size > 10*1024*1024) throw new Error('Запись пустая или слишком большая. Попробуйте ещё раз.');
    const form = new FormData(); form.append('audio',audio,'dictation.'+ext);
    const response = await fetch('/api/transcriptions',{method:'POST',body:form,signal:session.abort.signal});
    const result = await response.json();
    if (!response.ok) throw new Error(result.error?.message || 'Не удалось распознать речь.');
    if (typeof result.text !== 'string' || !result.text.trim()) throw new Error('Речь не распознана. Попробуйте говорить ближе к микрофону.');
    if (dictation !== session) return;
    const combined = [rawBrief.trim(),result.text.trim()].filter(Boolean).join('\n');
    if (combined.length > 2000 || new TextEncoder().encode(combined).length > 6000) throw new Error('Получилось слишком много текста. Сократите описание и повторите короткую диктовку.');
    rawBrief = combined;
    document.getElementById('brief').value = rawBrief;
    dictation = null;
    paintVoice('Готово. Проверьте текст и нажмите «Понять запрос», когда всё верно.');
    document.getElementById('brief').focus();
  } catch (e) {
    if (dictation !== session) return;
    dictation = null;
    paintVoice(e.name === 'AbortError' ? 'Распознавание не ответило вовремя. Текст сохранён; попробуйте ещё раз.' : e.message);
  } finally { clearTimeout(timeout); }
}

document.addEventListener('click',e => {
  if (e.target.closest('#voice-toggle')) { if (dictation) stopDictation(); else startDictation(); }
  if (e.target.closest('#voice-cancel')) cancelDictation();
});
window.addEventListener('pagehide',cancelDictation);
