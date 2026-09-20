#!/usr/bin/env python3
"""配布物で公開契約を検証する。Python PlaywrightとChromiumを使用する。"""
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

BUNDLE = Path(__file__).resolve().parents[1] / 'dist/promari-sns-share.min.js'

with sync_playwright() as api:
    browser = api.chromium.launch(channel='chromium')
    for width in (1440, 390, 320):
        page = browser.new_page(viewport={'width': width, 'height': 900})
        errors = []
        page.on('pageerror', lambda error: errors.append(str(error)))
        page.set_content('''<!doctype html><html lang="ja"><head><title>境界の検証</title></head>
        <body><promari-sns-share id="a" variant="circle" like url="https://example.com/article/"
        title="TypeScript & PHP #1" services="x,line,facebook,copy,native" secondary="hatena"></promari-sns-share>
        <promari-sns-share id="b" variant="circle" like url="https://example.com/article/"
        services="copy" secondary=""></promari-sns-share>
        <promari-sns-share id="plain" services="copy" secondary="copy"></promari-sns-share></body></html>''')
        page.evaluate('''() => {
          window.copies=[]; window.shares=[]; window.tracked=[]; window.likes=[]; window.prompts=[];
          window.copyMode='pending'; window.shareMode='success';
          Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:text=>{
            window.copies.push(text);
            if(window.copyMode==='reject')return Promise.reject(new Error('denied'));
            if(window.copyMode==='success')return Promise.resolve();
            return new Promise(resolve=>window.finishCopy=resolve);
          }}});
          Object.defineProperty(navigator,'share',{configurable:true,value:async data=>{
            window.shares.push(data); if(window.shareMode==='reject')throw new Error('cancel');
          }});
          window.prompt=(label,text)=>{window.prompts.push(text);return null;};
          document.addEventListener('promari-sns-share',e=>window.tracked.push(e.detail));
          document.addEventListener('promari-sns-share-like',e=>window.likes.push(e.detail));
        }''')
        page.add_script_tag(content=BUNDLE.read_text())
        page.evaluate("customElements.whenDefined('promari-sns-share')")
        a, b = page.locator('#a'), page.locator('#b')
        expect(a.locator('.like')).to_be_disabled()
        page.evaluate("document.querySelectorAll('[like]').forEach(e=>e.setLikeState({liked:false,count:3}))")
        a.locator('.like').click()
        assert page.evaluate('window.likes') == [{'liked': True}]
        expect(a.locator('.like')).to_be_disabled()
        page.evaluate("document.querySelectorAll('[like]').forEach(e=>e.setLikeState({liked:true,count:4}))")
        for element in (a, b):
            expect(element.locator('.like')).to_have_attribute('aria-pressed', 'true')
            expect(element.locator('.count')).to_have_text('4')
        a.locator('[data-key="copy"]').click()
        expect(a.locator('.status')).to_be_empty()
        page.evaluate('window.finishCopy()')
        expect(a.locator('.status')).to_have_text('URLをコピーしました')
        assert page.evaluate('window.copies') == ['https://example.com/article/']
        assert page.evaluate('window.tracked[0].service') == 'copy'
        page.evaluate("window.copyMode='reject'")
        b.locator('[data-key="copy"]').click()
        page.wait_for_function('window.prompts.length===1')
        expect(b.locator('.status')).to_be_empty()
        for mode in ('success', 'reject'):
            page.evaluate('(mode)=>window.shareMode=mode', mode)
            a.locator('[data-key="native"]').click()
        page.wait_for_function('window.shares.length===2')
        page.wait_for_function('window.tracked.filter(x=>x.service==="native").length===2')
        # API非対応では丸型表示の追加メニューを開く。
        page.evaluate("Object.defineProperty(navigator,'share',{configurable:true,value:undefined})")
        a.locator('[data-key="native"]').click()
        expect(a.locator('details')).to_have_attribute('open', '')
        a.locator('summary').press('Escape')
        expect(a.locator('details')).not_to_have_attribute('open', '')
        # 再描画・再接続後も、一操作につき一イベント。
        page.evaluate("window.copyMode='success'; const a=document.querySelector('#a'); a.setAttribute('caption','再描画'); a.remove();document.body.prepend(a);")
        count = page.evaluate('window.tracked.length')
        a.locator('[data-key="copy"]').click()
        page.wait_for_function('(count)=>window.tracked.length===count+1', arg=count)
        assert page.evaluate('window.tracked.length') == count + 1
        # 同じサービスが二つある場合も、通知先は押した要素だけ。
        copies = page.locator('#plain [data-key="copy"]')
        copies.nth(1).click()
        expect(copies.nth(1)).to_have_class('copy icon done')
        expect(copies.nth(0)).not_to_have_class('copy icon_text done')
        assert page.locator('#plain').evaluate('e=>e.shadowRoot.querySelectorAll(".done").length') == 1
        assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
        assert not errors, errors
        print(f'PASS: {width}px / コピー待機・拒否・共有成功／拒否／非対応・いいね同期・再接続・通知先', flush=True)
        page.close()
    browser.close()
