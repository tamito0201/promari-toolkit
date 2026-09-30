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
        title="TypeScript & PHP #1" destinations="x,line,facebook,copy,native" secondary="hatena"></promari-sns-share>
        <promari-sns-share id="b" variant="circle" like url="https://example.com/article/"
        destinations="copy" secondary=""></promari-sns-share>
        <promari-sns-share id="plain" destinations="copy" secondary="copy"></promari-sns-share>
        <promari-sns-share id="accent-attr" destinations="copy" secondary="" heading="SHARE" accent="#123456"></promari-sns-share>
        <promari-sns-share id="accent-leak" destinations="copy" secondary="" heading="SHARE" style="--accent:#ff0000"></promari-sns-share></body></html>''')
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
        # 色の入口は accent 属性だけ。ページ側から要素へ当てた --accent は内側へ届かない。
        heading = "id => getComputedStyle(document.getElementById(id).shadowRoot.querySelector('.h')).color"
        assert page.evaluate(f'({heading})("accent-attr")') == 'rgb(18, 52, 86)'
        assert page.evaluate(f'({heading})("accent-leak")') == 'rgb(84, 52, 126)', page.evaluate(f'({heading})("accent-leak")')
        a, b = page.locator('#a'), page.locator('#b')
        expect(a.locator('.like')).to_be_disabled()
        # 件数が届くまでは、仮の0ではなく未取得の「—」を出す。
        expect(a.locator('.count')).to_have_text('—')
        expect(a.locator('.count')).to_have_attribute('aria-label', 'いいねの件数は未取得')
        # 取得に失敗した接続コードは count: null を渡す。0件と見分けが付かない0にはしない。
        page.evaluate("document.querySelectorAll('[like]').forEach(e=>e.setLikeState({liked:false,count:null,message:'取得できませんでした'}))")
        expect(a.locator('.count')).to_have_text('—')
        expect(a.locator('.like')).to_be_enabled()
        # サーバーが0件と返したときだけ0を出す。不正な値（負・小数・文字列）は受け付けない。
        page.evaluate("document.querySelectorAll('[like]').forEach(e=>e.setLikeState({liked:false,count:0}))")
        expect(a.locator('.count')).to_have_text('0')
        expect(a.locator('.count')).to_have_attribute('aria-label', 'いいねの件数')
        for bad in ('-1', '1.5', "'3'"):
            page.evaluate(f"document.querySelectorAll('[like]').forEach(e=>e.setLikeState({{liked:false,count:{bad}}}))")
            expect(a.locator('.count')).to_have_text('0')
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
        assert page.evaluate('window.tracked[0].destination') == 'copy'
        page.evaluate("window.copyMode='reject'")
        b.locator('[data-key="copy"]').click()
        page.wait_for_function('window.prompts.length===1')
        expect(b.locator('.status')).to_be_empty()
        for mode in ('success', 'reject'):
            page.evaluate('(mode)=>window.shareMode=mode', mode)
            a.locator('[data-key="native"]').click()
        page.wait_for_function('window.shares.length===2')
        page.wait_for_function('window.tracked.filter(x=>x.destination==="native").length===2')
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
        print(f'PASS: {width}px / コピー待機・拒否・共有成功／拒否／非対応・いいね同期・再接続・通知先・色の入口', flush=True)
        page.close()

        # 書く場所（compose）と、主の列に10個並べた円い表示。
        page = browser.new_page(viewport={'width': width, 'height': 900})
        errors = []
        page.on('pageerror', lambda error: errors.append(str(error)))
        page.set_content('''<!doctype html><html lang="ja"><head><title>書く場所の検証</title></head>
        <body><promari-sns-share id="c" variant="circle" like url="https://example.com/article/" title="記事の題名"
        destinations="x,line,facebook,copy,native,qiita,zenn,note,medium" secondary="ameba"></promari-sns-share>
        <promari-sns-share id="w" url="https://example.com/article/" title="記事の題名"
        destinations="qiita,note" secondary="zenn,medium,ameba"></promari-sns-share></body></html>''')
        page.evaluate('''() => {
          window.events=[]; window.copyMode='pending';
          Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:text=>{
            window.events.push('copy:'+text);
            if(window.copyMode==='reject')return Promise.reject(new Error('denied'));
            return new Promise(resolve=>window.finishCopy=resolve);
          }}});
          Object.defineProperty(navigator,'share',{configurable:true,value:async()=>{}});
          window.open=(href,target,features)=>{window.events.push(['open',href,target,features].join('|'));return null;};
        }''')
        page.add_script_tag(content=BUNDLE.read_text())
        page.evaluate("customElements.whenDefined('promari-sns-share')")
        c, w = page.locator('#c'), page.locator('#w')
        # 主の列の9個と「その他」が、横スクロールを出さずに画面内へ収まる（折り返す）。
        assert c.locator('.socials > .social').count() == 9
        inside = '''e => [...e.shadowRoot.querySelectorAll('.socials > .social, summary')].map(n => n.getBoundingClientRect())
          .every(r => r.left >= 0 && r.right <= innerWidth)'''
        assert c.evaluate(inside), f'{width}px: 円いボタンが画面外にはみ出す'
        assert page.evaluate('document.documentElement.scrollWidth<=innerWidth'), page.evaluate('[document.documentElement.scrollWidth, innerWidth]')
        rows = c.evaluate("e => new Set([...e.shadowRoot.querySelectorAll('.socials > .social, summary')].map(n => Math.round(n.getBoundingClientRect().top))).size")
        c.locator('summary').click()
        expect(c.locator('details')).to_have_attribute('open', '')
        # 位置の補正は toggle イベント（非同期）で入るので、収まるまで待つ。収まらなければ時間切れで失敗する。
        page.wait_for_function('''() => { const r = document.querySelector('#c').shadowRoot.querySelector('.options').getBoundingClientRect();
          return r.left >= 15.5 && r.right <= innerWidth - 15.5; }''', timeout=3000)
        c.locator('summary').press('Escape')
        # 既定の共有欄: compose はボタン、note は公式の共有入口へのリンク。
        assert w.locator('[data-key="qiita"]').evaluate('n => n.tagName') == 'BUTTON'
        expect(w.locator('[data-key="note"]')).to_have_attribute('href', 'https://note.com/intent/post?url=https%3A%2F%2Fexample.com%2Farticle%2F')
        # 同じクリックの同期処理の中で、クリップボードへの書き込みと新しいタブの両方を始める。
        c.locator('[data-key="qiita"]').click()
        assert page.evaluate('window.events') == ['copy:記事の題名\nhttps://example.com/article/', 'open|https://qiita.com/drafts/new|_blank|noopener,noreferrer'], page.evaluate('window.events')
        expect(c.locator('.status')).to_be_empty()  # 書き込みが終わるまで成功を名乗らない。
        page.evaluate('window.finishCopy()')
        expect(c.locator('.status')).to_have_text('タイトルとURLをコピーしました。投稿画面に貼り付けてください')
        # 書き込みが拒否されても投稿画面は開き、失敗を知らせる（prompt は出さない）。
        page.evaluate("window.events=[]; window.copyMode='reject'")
        w.locator('[data-key="zenn"]').click()
        expect(w.locator('.t')).to_have_text('コピーできませんでした。投稿画面にタイトルとURLを入力してください')
        assert page.evaluate('window.events[1]') == 'open|https://zenn.dev/dashboard|_blank|noopener,noreferrer'
        assert not errors, errors
        print(f'PASS: {width}px / 主の列10個が{rows}段で収まる・その他の選択肢が画面内・compose の同期開始・成功／失敗の通知', flush=True)
        page.close()
    browser.close()
