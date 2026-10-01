(function (a) {
  function fail(why) { return JSON.stringify({ ok: false, error: why }); }
  var el = (window.__vansEls || [])[a.id];

  if (a.verb === 'press') {
    var on = document.activeElement || document.body;
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

  if (!el || !el.isConnected) return fail('that control is no longer on the page');

  if (a.verb === 'scroll') {
    el.scrollIntoView({ block: 'center', inline: 'center' });
    return JSON.stringify({ ok: true });
  }

  var r = el.getBoundingClientRect();
  if (r.bottom <= 0 || r.top >= innerHeight || r.right <= 0 || r.left >= innerWidth) {
    el.scrollIntoView({ block: 'center', inline: 'center' });
    r = el.getBoundingClientRect();
  }
  var at = { bubbles: true, cancelable: true, view: window, clientX: r.left + r.width / 2, clientY: r.top + r.height / 2, button: 0 };

  if (a.verb === 'click') {
    ['pointerdown', 'mousedown', 'pointerup', 'mouseup'].forEach(function (t) {
      el.dispatchEvent(new MouseEvent(t, at));
    });
    if (el.focus) el.focus();
    el.click();
    return JSON.stringify({ ok: true });
  }

  if (a.verb === 'type') {
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
    } else if (tag === 'input' || tag === 'textarea') {
      var proto = tag === 'textarea' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
      Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, a.text);
    } else if (el.isContentEditable) {
      var sel = window.getSelection();
      sel.selectAllChildren(el);
      sel.collapseToEnd();
      document.execCommand('insertText', false, a.text);
      return JSON.stringify({ ok: true });
    } else {
      return fail('that control does not hold typed text');
    }
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
    return JSON.stringify({ ok: true });
  }

  return fail('unknown verb ' + a.verb);
})
