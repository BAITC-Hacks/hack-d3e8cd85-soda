"""uv run --with selenium python ui_consultation_check.py (running app + Firefox).
The AI is mocked; catalog, calendar and matching use the actual local server.
"""
import json
import threading
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.firefox.options import Options

requests = []
fail_next = False


class Proxy(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        self.forward()

    def do_POST(self):
        self.forward()

    def forward(self):
        global fail_next
        payload = self.rfile.read(int(self.headers.get('Content-Length', 0))) if self.command == 'POST' else None
        status, kind = 200, 'application/json'
        if self.path.startswith('/frame?'):
            width = int(self.path.split('?')[1])
            data = f'<iframe title="Conversation check" src="/#brief" style="width:{width}px;height:1100px;border:0"></iframe>'.encode()
            kind = 'text/html'
        elif self.path == '/api/consultations':
            request = json.loads(payload)
            requests.append(request)
            if fail_next:
                fail_next = False
                status = 503
                data = json.dumps({'error': {'message': 'Тестовый сбой AI. Повторите отправку.'}}).encode()
            else:
                q = request['query'].copy()
                last = request['messages'][-1]['text']
                focus, question, choices = 'review', '', []
                if 'оформить' in last:
                    q.update(city='Алматы', event_date='2026-10-10', event_type='свадьба', budget_kzt=300000)
                    focus, question, choices = 'category', 'Цветы или оформление пространства?', ['Цветы и букеты', 'Оформление зала']
                elif last == 'Цветы и букеты':
                    q['category'] = 'Флорист'
                    focus, question, choices = 'style', 'Какое оформление ближе?', ['Сезонные цветы', 'Минимализм']
                elif last == 'Сезонные цветы':
                    q['wishes'] = ['flowers']
                elif '400000' in last:
                    q['budget_kzt'] = 400000
                data = json.dumps(dict(query=q, focus=focus, question=question, choices=choices)).encode()
        else:
            req = urllib.request.Request('http://127.0.0.1:8080' + self.path, data=payload,
                headers={'Content-Type': 'application/json'} if payload else {})
            try:
                with urllib.request.urlopen(req) as r:
                    data, kind = r.read(), r.headers.get('Content-Type')
            except urllib.error.HTTPError as e:
                status, data = e.code, e.read()
        self.send_response(status)
        self.send_header('Content-Type', kind)
        self.end_headers()
        self.wfile.write(data)


proxy = ThreadingHTTPServer(('127.0.0.1', 0), Proxy)
threading.Thread(target=proxy.serve_forever, daemon=True).start()
options = Options()
options.add_argument('-headless')
try:
    with webdriver.Firefox(options=options) as browser:
        wait = WebDriverWait(browser, 12)
        def element(css):
            return browser.find_element(By.CSS_SELECTOR, css)
        def click(css):
            target = next(e for e in browser.find_elements(By.CSS_SELECTOR, css) if e.is_displayed())
            browser.execute_script("arguments[0].scrollIntoView({block:'center'})", target)
            target.click()
        def settled():
            wait.until(lambda d: d.find_element(By.ID, 'main').get_attribute('aria-busy') == 'false')
        def send(text):
            element('#brief').send_keys(text)
            click('#brief-form button[type=submit]')
            settled()
        for width in [1440, 1024, 768, 320]:
            browser.set_window_size(max(1280, width + 40), 1200)
            browser.get(f'http://127.0.0.1:{proxy.server_port}/frame?{width}')
            browser.switch_to.frame(element('iframe'))
            wait.until(lambda d: d.find_elements(By.ID, 'brief'))
            send('Хочу оформить свадьбу.')
            assert 'Алматы' in element('.live-summary').text and '300' in element('.live-summary').text
            assert len(browser.find_elements(By.CSS_SELECTOR, '[data-consult-choice]')) == 2
            click('[data-consult-choice="0"]'); settled()
            assert 'Флорист' in element('.live-summary').text
            assert 'Какое оформление ближе?' in element('#latest-message').text
            assert requests[-1]['messages'][-2]['text'] == 'Цветы или оформление пространства?'
            assert browser.execute_script('return innerWidth === arguments[0] && document.documentElement.scrollWidth <= innerWidth', width)
            browser.execute_async_script('const done=arguments[0];document.fonts.ready.then(done)')
            browser.execute_async_script('const done=arguments[0]; Promise.all(document.getAnimations().filter(a=>a.effect.getTiming().iterations!==Infinity).map(a=>a.finished.catch(()=>{}))).then(done)')
            browser.execute_script('scrollTo(0,0)')
            if width in [1440, 320]:
                browser.save_screenshot(f'/tmp/consultation-{width}.png')
            if width == 320:
                count = len(requests)
                click('[data-skip-style]')
                assert len(requests) == count, 'skip should not call AI'
            else:
                click('[data-consult-choice="0"]'); settled()
                assert requests[-1]['style_asked'] is True
                assert 'Сезонные цветы' in element('.live-summary').text
            assert browser.find_elements(By.CSS_SELECTOR, '.conversation-ready')
            click('.conversation-ready [data-go=review]')
            wait.until(lambda d: d.find_elements(By.ID, 'review-form'))
            assert element('#category').get_attribute('value') == 'Флорист'
            assert element('#budget_kzt').get_attribute('value') == '300000'
            budget = element('#budget_kzt'); budget.clear(); budget.send_keys('350000')
            click('.header-start')
            wait.until(lambda d: d.find_elements(By.ID, 'brief'))
            send('Бюджет 400000')
            assert requests[-1]['query']['budget_kzt'] == 350000, 'manual edits missing from context'
            assert '400' in element('.live-summary').text
            previous = element('.conversation-log').text
            fail_next = True
            send('Дополнительный ответ')
            assert element('#brief').get_attribute('value') == 'Дополнительный ответ'
            assert element('.conversation-log').text == previous
            assert browser.find_elements(By.CSS_SELECTOR, '.request-error')
            click('#brief-form button[type=submit]'); settled()
            assert len(requests[-1]['messages']) == len(requests[-2]['messages']), 'retry duplicated user message'
            assert requests[-1]['messages'] == requests[-2]['messages']
            click('#motion-toggle')
            if element('html').get_attribute('data-motion') != 'off':
                click('#motion-toggle')
            assert browser.execute_script('return document.getAnimations().length') == 0
            click('[data-reset-conversation]')
            assert not browser.find_elements(By.CSS_SELECTOR, '[data-consult-choice]')
            assert 'Алматы' not in element('.live-summary').text
            assert element('#brief').get_attribute('value') == ''
            print('PASS', width, 'conversation, choices, live brief, optional skip, edits, failure/retry, reset, motion and overflow')
finally:
    proxy.shutdown()
    proxy.server_close()
