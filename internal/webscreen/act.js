(function (a) {
  function fail(why) { return JSON.stringify({ ok: false, error: why }); }
  var el = (window.__vansEls || [])[a.id];

  var W = (el && el.ownerDocument && el.ownerDocument.defaultView) || window;
  var D = (el && el.ownerDocument) || document;

  if (a.verb === 'press') {
    var on = document.activeElement || document.body;
    for (var hop = 0; hop < 4 && on && (on.tagName === 'IFRAME' || on.tagName === 'FRAME'); hop++) {
      var within = null;
      try { within = on.contentDocument; } catch (e) { within = null; }
      if (!within || !within.activeElement) break;
      on = within.activeElement;
    }
    var init = {
      key: a.key, code: a.code, keyCode: a.keyCode, which: a.keyCode,
      bubbles: true, cancelable: true,
      metaKey: !!a.meta, ctrlKey: !!a.ctrl, altKey: !!a.alt, shiftKey: !!a.shift
    };
    var went = on.dispatchEvent(new KeyboardEvent('keydown', init));
    on.dispatchEvent(new KeyboardEvent('keypress', init));
    on.dispatchEvent(new KeyboardEvent('keyup', init));
    if (went && a.key === 'Enter' && on.form && on.tagName !== 'TEXTAREA') {
      if (on.form.requestSubmit) on.form.requestSubmit(); else on.form.submit();
    }
    if (a.key === 'Escape' && on.blur) on.blur();
    return JSON.stringify({ ok: true });
  }

  if (a.verb === 'scrollby') {
    var box = el && el.isConnected ? el : null;
    while (box && box !== document.body && box !== document.documentElement) {
      var style = getComputedStyle(box);
      var tall = box.scrollHeight > box.clientHeight + 4 && /(auto|scroll)/.test(style.overflowY);
      var wide = box.scrollWidth > box.clientWidth + 4 && /(auto|scroll)/.test(style.overflowX);
      if (tall || wide) break;
      box = box.parentElement;
    }
    var page = !box || box === document.body || box === document.documentElement;
    var far = Math.round((page ? innerHeight : box.clientHeight) * 0.8);
    var across = Math.round((page ? innerWidth : box.clientWidth) * 0.8);
    var dx = a.text === 'left' ? -across : a.text === 'right' ? across : 0;
    var dy = a.text === 'up' ? -far : a.text === 'down' ? far : 0;
    if (page) window.scrollBy(dx, dy); else box.scrollBy(dx, dy);
    return JSON.stringify({ ok: true });
  }

  if (!el || !el.isConnected) return fail('that control is no longer on the page');

  if (a.verb === 'scroll') {
    el.scrollIntoView({ block: 'center', inline: 'center' });
    return JSON.stringify({ ok: true });
  }

  var r = el.getBoundingClientRect();
  if (r.bottom <= 0 || r.top >= W.innerHeight || r.right <= 0 || r.left >= W.innerWidth) {
    el.scrollIntoView({ block: 'center', inline: 'center' });
    r = el.getBoundingClientRect();
  }
  var at = { bubbles: true, cancelable: true, view: W, clientX: r.left + r.width / 2, clientY: r.top + r.height / 2, button: 0 };

  if (a.verb === 'dblclick') {
    ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'click', 'pointerdown', 'mousedown', 'pointerup', 'mouseup', 'click'].forEach(function (t) {
      el.dispatchEvent(new MouseEvent(t, at));
    });
    var twice = Object.assign({}, at, { detail: 2 });
    el.dispatchEvent(new MouseEvent('dblclick', twice));
    return JSON.stringify({ ok: true });
  }

  if (a.verb === 'contextmenu') {
    var right = Object.assign({}, at, { button: 2, buttons: 2 });
    ['pointerdown', 'mousedown'].forEach(function (t) { el.dispatchEvent(new MouseEvent(t, right)); });
    el.dispatchEvent(new MouseEvent('contextmenu', right));
    ['pointerup', 'mouseup'].forEach(function (t) { el.dispatchEvent(new MouseEvent(t, right)); });
    return JSON.stringify({ ok: true });
  }

  if (a.verb === 'hover') {
    ['pointerover', 'pointerenter', 'mouseover', 'mouseenter', 'pointermove', 'mousemove'].forEach(function (t) {
      el.dispatchEvent(new MouseEvent(t, at));
    });
    if (el.focus && el.tabIndex >= 0) el.focus();
    return JSON.stringify({ ok: true });
  }

  if (a.verb === 'date') {
    var kind = (el.getAttribute('type') || '').toLowerCase();
    if (el.tagName.toLowerCase() === 'input' && (kind === 'date' || kind === 'datetime-local')) {
      var day = kind === 'date' ? a.text : a.text + 'T09:00';
      if (el.focus) el.focus();
      Object.getOwnPropertyDescriptor(W.HTMLInputElement.prototype, 'value').set.call(el, day);
      el.dispatchEvent(new Event('input', { bubbles: true }));
      el.dispatchEvent(new Event('change', { bubbles: true }));
      return JSON.stringify({ ok: true });
    }
    return JSON.stringify({ ok: true, opens: true });
  }

  if (a.verb === 'set') {
    var want = Number(a.text);
    if (isNaN(want)) return fail('that is not a number');
    var said = function (n) { return String(n.getAttribute('aria-label') || n.getAttribute('title') || n.innerText || '').replace(/[\u200b-\u200f\ufeff]/g, '').trim(); };
    var inc = /^(\+|＋|﹢)$|^(increase|increment|add|plus|more)\b/i, dec = /^(-|−|–|－|﹣)$|^(decrease|decrement|remove|minus|fewer|less)\b/i;
    if (el.tagName.toLowerCase() !== 'input' && el.getAttribute('aria-valuenow') == null) {
      var up = null, down = null, shows = null;
      el.querySelectorAll('*').forEach(function (n) {
        var t = said(n);
        if (!up && inc.test(t)) up = n;
        else if (!down && dec.test(t)) down = n;
        else if (!shows && ((n.children.length === 0 && /^\d+$/.test(t)) || (n.tagName === 'INPUT' && /^\d+$/.test(String(n.value).trim())))) shows = n;
      });
      if (!up || !down || !shows) return fail('that is not a counter with a + and a −');
      var now = Number(shows.tagName === 'INPUT' ? shows.value : said(shows));
      if (now === want) return JSON.stringify({ ok: true });
      var press = now < want ? up : down;
      if (press.getAttribute('aria-disabled') === 'true' || press.disabled) return fail('the counter stops at ' + now);
      var pr = press.getBoundingClientRect();
      var pat = { bubbles: true, cancelable: true, view: W, clientX: pr.left + pr.width / 2, clientY: pr.top + pr.height / 2, button: 0 };
      ['pointerdown', 'mousedown', 'pointerup', 'mouseup'].forEach(function (t) { press.dispatchEvent(new MouseEvent(t, pat)); });
      press.click();
      return JSON.stringify({ ok: true, more: true, now: now });
    }
    if (el.tagName.toLowerCase() === 'input') {
      Object.getOwnPropertyDescriptor(W.HTMLInputElement.prototype, 'value').set.call(el, String(want));
      el.dispatchEvent(new Event('input', { bubbles: true }));
      el.dispatchEvent(new Event('change', { bubbles: true }));
      return JSON.stringify({ ok: true });
    }
    if (el.focus) el.focus();
    for (var n = 0; n < 200; n++) {
      var now = Number(el.getAttribute('aria-valuenow'));
      if (isNaN(now) || now === want) break;
      var key = now < want ? 'ArrowRight' : 'ArrowLeft';
      var code = now < want ? 39 : 37;
      el.dispatchEvent(new KeyboardEvent('keydown', { key: key, code: key, keyCode: code, which: code, bubbles: true, cancelable: true }));
      el.dispatchEvent(new KeyboardEvent('keyup', { key: key, code: key, keyCode: code, which: code, bubbles: true, cancelable: true }));
      var after = Number(el.getAttribute('aria-valuenow'));
      if (after === now || (now < want && after > want) || (now > want && after < want)) break;
    }
    return JSON.stringify({ ok: true });
  }

  if (a.verb === 'click') {
    ['pointerdown', 'mousedown', 'pointerup', 'mouseup'].forEach(function (t) {
      el.dispatchEvent(new MouseEvent(t, at));
    });
    if (el.focus) el.focus();
    el.click();
    return JSON.stringify({ ok: true });
  }

  if (a.verb === 'type') {
    var text = a.text == null ? '' : String(a.text);
    if (el.focus) el.focus();
    var tag = el.tagName.toLowerCase();
    if (tag === 'select') {
      var want = String(a.text).trim().toLowerCase();
      var pick = -1;
      for (var i = 0; i < el.options.length; i++) {
        var t = el.options[i].text.trim().toLowerCase();
        if (t === want) { pick = i; break; }
        if (pick < 0 && t.indexOf(want) >= 0) pick = i;
      }
      if (pick < 0) return fail('that list has no choice called ' + a.text);
      el.selectedIndex = pick;
    } else if ((tag === 'input' || tag === 'textarea') && el.readOnly) {
      return JSON.stringify({ ok: false, opens: true, error: 'that field cannot be typed into' });
    } else if (tag === 'input' || tag === 'textarea') {
      var proto = tag === 'textarea' ? W.HTMLTextAreaElement.prototype : W.HTMLInputElement.prototype;
      Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, text);
    } else if (el.isContentEditable) {
      var sel = W.getSelection();
      sel.selectAllChildren(el);
      sel.collapseToEnd();
      D.execCommand('insertText', false, text);
      return JSON.stringify({ ok: true });
    } else {
      return JSON.stringify({ ok: false, opens: true, error: 'that control does not hold typed text' });
    }
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
    return JSON.stringify({ ok: true });
  }

  return fail('unknown verb ' + a.verb);
})
