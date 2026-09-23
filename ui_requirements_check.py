"""Run against localhost:8080: uv run --with selenium python ui_requirements_check.py.
Requires Firefox. Only the AI response is mocked; no external AI call is made.
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

requirements = ['Обязательно без конкурсов', '<img src=x onerror="window.injected=true">']
searches = []


class Proxy(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        self.forward()

    def do_POST(self):
        self.forward()

    def forward(self):
        payload = self.rfile.read(int(self.headers.get('Content-Length', 0))) if self.command == 'POST' else None
        status, kind = 200, 'application/json'
        if self.path == '/frame':
            data, kind = b'<iframe title="Mobile check" src="/#brief" style="width:320px;height:1100px;border:0"></iframe>', 'text/html'
        elif self.path == '/api/briefs':
            data = json.dumps(dict(city='Алматы', category='Ведущий', event_type='корпоратив',
                event_date='2026-10-06', budget_kzt=1300000, duration_hours=6, language='русский',
                wishes=[], unverified_requirements=requirements)).encode()
        else:
            if self.path == '/api/matches':
                searches.append(json.loads(payload))
            req = urllib.request.Request('http://127.0.0.1:8080' + self.path, data=payload,
                headers={'Content-Type': 'application/json'} if payload else {})
            try:
                with urllib.request.urlopen(req) as response:
                    data, kind = response.read(), response.headers.get('Content-Type')
            except urllib.error.HTTPError as error:
                status, data, kind = error.code, error.read(), 'application/json'
            if self.path == '/api/options' and status == 200:
                options = json.loads(data)
                options['ai_enabled'] = True
                data = json.dumps(options).encode()
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
        def element(selector):
            return browser.find_element(By.CSS_SELECTOR, selector)
        def click(selector):
            target = next(e for e in browser.find_elements(By.CSS_SELECTOR, selector) if e.is_displayed())
            browser.execute_script("arguments[0].scrollIntoView({block:'center'})", target)
            target.click()
        def route(name):
            wait.until(lambda d: d.execute_script('return location.hash') == '#' + name and not d.find_elements(By.CSS_SELECTOR, '.loading-state'))
        browser.set_window_size(1280, 960)
        browser.get(f'http://127.0.0.1:{proxy.server_port}/#brief')
        wait.until(lambda d: d.find_elements(By.ID, 'brief'))
        element('#brief').send_keys('Корпоратив в Алматы, ведущий без конкурсов.')
        click('#brief-form button[type=submit]')
        route('review')
        assert requirements[0] in element('#requirements-review').text
        assert browser.execute_script('return document.querySelector("#requirements-review").offsetTop < document.querySelector(".summary-grid").offsetTop')
        assert browser.execute_script('return window.injected') is None
        click('#review-form button[type=submit]')
        assert searches == []
        assert browser.execute_script('return document.activeElement.name') == 'ignored_requirement'
        assert browser.execute_script('return document.querySelector("#requirements-review").getBoundingClientRect().top >= 0')
        click('[name=ignored_requirement][value="0"]')
        click('#review-form button[type=submit]')
        assert searches == [], 'unchecked requirement was silently dropped'
        click('[name=ignored_requirement][value="1"]')
        click('#step-nav [data-go=preferences]')
        route('preferences')
        assert all(e.is_selected() for e in browser.find_elements(By.NAME, 'ignored_requirement'))
        click('#preferences-form button[type=submit]')
        route('review')
        click('[name=ignored_requirement][value="0"]')
        click('#review-form button[type=submit]')
        assert searches == [], 'undoing consent failed to restore requirement'
        click('[name=ignored_requirement][value="0"]')
        click('#review-form button[type=submit]')
        route('results')
        assert len(searches) == 1 and searches[0]['unverified_requirements'] == []
        assert searches[0]['budget_kzt'] == 1300000 and searches[0]['duration_hours'] == 6
        click('.ignored-note summary')
        assert all(text in element('.ignored-note').text for text in requirements)
        assert len(browser.find_elements(By.CSS_SELECTOR, '.contractor-card')) == 3
        click('.header-start')
        route('brief')
        assert element('#brief').get_attribute('value') == 'Корпоратив в Алматы, ведущий без конкурсов.'
        click('#brief-form button[type=submit]')
        route('review')
        assert not any(e.is_selected() for e in browser.find_elements(By.NAME, 'ignored_requirement')), 'old consent reused after new AI parsing'
        click('#requirements-review [data-go=brief]')
        route('brief')
        click('[data-example=hosts]')
        route('review')
        assert not browser.find_elements(By.ID, 'requirements-review'), 'requirements leaked into another brief'
        browser.get(f'http://127.0.0.1:{proxy.server_port}/frame')
        browser.switch_to.frame(element('iframe'))
        wait.until(lambda d: d.find_elements(By.ID, 'brief'))
        element('#brief').send_keys('Корпоратив, ведущий без конкурсов.')
        click('#brief-form button[type=submit]')
        route('review')
        assert browser.execute_script('return innerWidth === 320 && document.documentElement.scrollWidth <= 320')
        click('#review-form button[type=submit]')
        assert len(searches) == 1 and browser.execute_script('return document.activeElement.name') == 'ignored_requirement'
        print('PASS: concrete requirements visible, blocking/focus, explicit per-item consent, undo, navigation, result disclosure, XSS escaping, reset after new parsing/example')
finally:
    proxy.shutdown()
    proxy.server_close()
