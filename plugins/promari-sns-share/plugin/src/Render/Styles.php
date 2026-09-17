<?php
/**
 * Generate presentation CSS and JavaScript once per page using appearance and style configuration.
 */

declare(strict_types=1);

namespace PromariSnsShare\Render;

use PromariSnsShare\Contracts\ConfigInterface;

final class Styles
{
    public function __construct(private readonly ConfigInterface $config)
    {
    }

    /**
 * Official button dimensions: small is 20px high, large is 28px.
 * @return array{h:int,f:int,i:int,px:int}
 */
    private static function metrics(string $size): array
    {
        return match ($size) {
            'large' => ['h' => 28, 'f' => 13, 'i' => 16, 'px' => 10],
            default => ['h' => 20, 'f' => 11, 'i' => 12, 'px' => 8],
        };
    }

    /**
 * Corner shape: official is pill-shaped for X and rounded for other services.
 */
    private static function radius(string $shape, string $service): string
    {
        return match ($shape) {
            'pill' => '9999px',
            'rounded' => '6px',
            'square' => '0',
            default => $service === 'x' ? '9999px' : '3px',
        };
    }

    public function css(): string
    {
        $c = $this->config;
        $accent = (string) $c->get('style.accent');
        $bg = (string) $c->get('style.floating_background');
        $font = (string) $c->get('appearance.font_family');
        $gap = (int) $c->get('appearance.gap_px');
        $shape = (string) $c->get('appearance.shape');
        $m = self::metrics((string) $c->get('appearance.size'));
        $sSize = (int) $c->get('appearance.secondary_size_px');
        $sStyle = (string) $c->get('appearance.secondary_style');
        $secondary = match ($sStyle) {
            'brand' => '.pm-share__btn--secondary{background:var(--pm-share-brand);color:#fff}.pm-share__btn--secondary:hover{filter:brightness(1.1)}',
            'outline' => '.pm-share__btn--secondary{background:transparent;color:var(--pm-share-brand);box-shadow:inset 0 0 0 1px var(--pm-share-brand)}.pm-share__btn--secondary:hover{background:var(--pm-share-brand);color:#fff}',
            default => '.pm-share__btn--secondary{background:#f1f1f3;color:#5f6368}.pm-share__btn--secondary:hover{background:var(--pm-share-brand);color:#fff}',
        };
        $floatingSide = (string) $c->get('placements.floating_position') === 'top' ? 'top:0;transform:translateY(-110%)' : 'bottom:0;transform:translateY(110%)';
        $rules = [
            ".pm-share{display:flex;flex-wrap:wrap;align-items:center;gap:10px 14px;margin:0;padding:0;font:{$m['f']}px/1 $font}",
            ".pm-share--heading-top{flex-direction:column;align-items:flex-start;gap:8px}",
            ".pm-share__heading{font-size:11px;font-weight:700;letter-spacing:.12em;color:$accent;margin-right:2px}",
            ".pm-share__primary,.pm-share__secondary{display:inline-flex;align-items:center;gap:{$gap}px}",
            ".pm-share__secondary{padding-left:10px;border-left:1px solid rgba(0,0,0,.12)}",
            ".pm-share__btn{display:inline-flex;align-items:center;gap:4px;box-sizing:border-box;text-decoration:none;color:#fff;white-space:nowrap;cursor:pointer;transition:filter .15s,transform .15s,background .15s}",
            ".pm-share__btn:hover{filter:brightness(1.08);color:#fff;text-decoration:none}.pm-share__btn:active{transform:translateY(1px)}",
            ".pm-share__btn:focus-visible{outline:2px solid $accent;outline-offset:2px}",
            ".pm-share__icon{display:inline-flex;width:{$m['i']}px;height:{$m['i']}px}.pm-share__icon svg{width:100%;height:100%;display:block}",
            ".pm-share__btn--primary{height:{$m['h']}px;padding:0 {$m['px']}px 0 " . ($m['px'] - 2) . "px;font-weight:700;font-size:{$m['f']}px;background:var(--pm-share-brand);border-radius:" . self::radius($shape, '') . '}',
            ".pm-share__btn--x{border-radius:" . self::radius($shape, 'x') . ';padding:0 ' . ($m['px'] + 2) . 'px 0 ' . $m['px'] . 'px}',
            ".pm-share__btn--line .pm-share__icon{width:" . ($m['i'] + 2) . 'px;height:' . ($m['i'] + 2) . 'px}',
            ".pm-share__btn--label-icon{padding:0 " . ($m['px'] - 2) . 'px}',
            ".pm-share__btn--secondary{width:{$sSize}px;height:{$sSize}px;justify-content:center;border-radius:50%;padding:0}",
            ".pm-share__btn--secondary .pm-share__icon{width:" . (int) round($sSize * 0.54) . 'px;height:' . (int) round($sSize * 0.54) . 'px}',
            $secondary,
            ".pm-share__btn.is-done{background:$accent;color:#fff}",
            '.pm-share--article_top{margin:0 0 28px}.pm-share--article_bottom{margin:36px 0 0;padding-top:18px;border-top:1px solid rgba(0,0,0,.08)}',
            '.pm-share--sidebar{flex-direction:column;align-items:flex-start;gap:10px}.pm-share--sidebar .pm-share__secondary{border-left:0;padding-left:0}',
            ".pm-share--floating{position:fixed;left:0;right:0;$floatingSide;z-index:10010;justify-content:center;gap:8px;padding:10px 12px calc(10px + env(safe-area-inset-bottom));background:$bg;box-shadow:0 -6px 20px rgba(0,0,0,.08);backdrop-filter:saturate(1.2) blur(6px);transition:transform .25s ease}",
            '.pm-share--floating.is-visible{transform:translateY(0)}',
            '@media(min-width:769px){.pm-share--floating{display:none}}',
            '.pm-share__toast{position:fixed;left:50%;bottom:72px;z-index:10011;transform:translateX(-50%);padding:8px 14px;border-radius:999px;background:#1b1b1b;color:#fff;font-size:12px;opacity:0;transition:opacity .2s;pointer-events:none}',
            '.pm-share__toast.is-visible{opacity:1}',
        ];
        return implode('', $rules);
    }

    /**
 * Popup, clipboard, native sharing, and floating-bar behavior without runtime libraries. Hide native sharing on unsupported devices.
 */
    public function js(): string
    {
        $c = $this->config;
        $after = (int) $c->get('placements.floating_after_px');
        $hideNearEnd = $c->get('placements.floating_hide_near_end') === true ? 'true' : 'false';
        $copied = wp_json_encode((string) $c->get('messages.copied'));
        $event = wp_json_encode((string) $c->get('tracking.event_name'));
        $attr = wp_json_encode((string) $c->get('tracking.attribute'));
        return <<<JS
(()=>{const say=(()=>{let t;return m=>{t??=Object.assign(document.body.appendChild(document.createElement("div")),{className:"pm-share__toast"});t.setAttribute("role","status");t.textContent=m;t.classList.add("is-visible");clearTimeout(t._t);t._t=setTimeout(()=>t.classList.remove("is-visible"),1800)}})();
const can=typeof navigator.share==="function";document.querySelectorAll(".pm-share__btn[data-pm-share-action=native]").forEach(b=>can?b.removeAttribute("hidden"):b.remove());
const copy=(b,u)=>{const done=()=>{b.classList.add("is-done");say($copied);setTimeout(()=>b.classList.remove("is-done"),1500)};(navigator.clipboard?.writeText?navigator.clipboard.writeText(u).then(done,()=>prompt("URL",u)):prompt("URL",u))};
const popup=(href,size)=>{const[w,h]=size.split("x").map(Number),l=Math.max(0,(screen.width-w)/2),t=Math.max(0,(screen.height-h)/2);return window.open(href,"pm-share","width="+w+",height="+h+",left="+l+",top="+t+",noopener,noreferrer")};
document.addEventListener("click",e=>{const b=e.target.closest?.(".pm-share__btn");if(!b)return;const a=b.dataset.pmShareAction,u=b.dataset.pmShareUrl;
if(a==="copy"){e.preventDefault();copy(b,u)}else if(a==="native"){e.preventDefault();navigator.share({title:b.dataset.pmShareTitle,url:u}).catch(()=>{})}else if(b.dataset.pmSharePopup&&popup(b.href,b.dataset.pmSharePopup)){e.preventDefault()}
document.dispatchEvent(new CustomEvent($event,{detail:{service:b.getAttribute($attr),url:u,placement:b.closest(".pm-share")?.dataset.pmSharePlacement}}))},false);
const bar=document.querySelector(".pm-share--floating");if(bar){let tick=false;const show=()=>{const y=scrollY,end=document.documentElement.scrollHeight-innerHeight-120;bar.classList.toggle("is-visible",y>$after&&(!$hideNearEnd||y<end));tick=false};addEventListener("scroll",()=>{if(!tick){tick=true;requestAnimationFrame(show)}},{passive:true});show()}})();
JS;
    }
}
