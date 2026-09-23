'use strict';

const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
let motionOff = reducedMotion.matches;
try { motionOff ||= localStorage.getItem('eventmatch-motion') === 'off'; } catch {}
function paintMotion() {
  const stopped = motionOff || reducedMotion.matches;
  document.documentElement.dataset.motion = stopped ? 'off' : 'on';
  const toggle = document.getElementById('motion-toggle');
  toggle.disabled = reducedMotion.matches;
  toggle.textContent = stopped ? '▷' : 'Ⅱ';
  toggle.setAttribute('aria-pressed',String(!stopped));
  toggle.setAttribute('aria-label',reducedMotion.matches ? 'Анимации отключены в настройках устройства' : stopped ? 'Включить анимации' : 'Отключить анимации');
  toggle.title = toggle.getAttribute('aria-label');
  if (stopped) document.getAnimations().forEach(a=>a.cancel());
}
let lastAnimatedRoute = null;
function animatePage() {
  if (motionOff || reducedMotion.matches) return;
  const selector = lastAnimatedRoute === location.hash && location.hash === '#brief' ? '#latest-message,.thinking-message,.summary-progress' : '.hero-copy,.event-art,.consultant-heading,.conversation-panel,.live-summary,.contractor-card,.summary-card';
  lastAnimatedRoute = location.hash;
  document.querySelectorAll(selector).forEach((el,i)=>{
    el.animate([{opacity:0,transform:'translateY(10px)'},{opacity:1,transform:'translateY(0)'}],{duration:420,delay:Math.min(i,4)*45,easing:'cubic-bezier(.2,.65,.3,1)'});
  });
}
document.getElementById('motion-toggle').addEventListener('click',()=>{
  motionOff = !motionOff;
  try { localStorage.setItem('eventmatch-motion',motionOff ? 'off' : 'on'); } catch {}
  paintMotion();
});
reducedMotion.addEventListener('change',paintMotion);
paintMotion();
