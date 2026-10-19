(function () {
  var PICK = 'a[href],button,input,textarea,select,summary,[role=button],[role=link],[role=tab],' +
    '[role=menuitem],[role=option],[role=checkbox],[role=radio],[role=switch],[role=combobox],' +
    '[role=textbox],[role=searchbox],[role=gridcell],[role=slider],[contenteditable=""],[contenteditable=true],[onclick]';
  var MOST = 400;
  var MOST_CELLS = 120;

  var MOST_LOOKED = 6000;
  var MOST_FRAMES = 12;
  var DEEPEST_FRAME = 3;
  var inFrame = new Map();
  var closed = [];
  var frames = 0;

  function gather(root, into, frame, depth) {
    var found = root.querySelectorAll(PICK);
    for (var i = 0; i < found.length; i++) {
      into.push(found[i]);
      if (frame) inFrame.set(found[i], frame);
    }
    var all = root.querySelectorAll('*');
    for (var j = 0; j < all.length; j++) {
      if (all[j].shadowRoot) gather(all[j].shadowRoot, into, frame, depth);
    }
    var holders = root.querySelectorAll('iframe,frame');
    for (var k = 0; k < holders.length; k++) {
      var holder = holders[k];
      var box = holder.getBoundingClientRect();
      if (box.width < 40 || box.height < 30) continue;
      var at = {
        x: (frame ? frame.x : 0) + box.left + holder.clientLeft,
        y: (frame ? frame.y : 0) + box.top + holder.clientTop,
        w: holder.clientWidth, h: holder.clientHeight
      };
      var inner = null;
      try { inner = holder.contentDocument; } catch (e) { inner = null; }
      if (!inner || !inner.documentElement) {
        var from = '';
        try { from = new URL(holder.src, location.href).hostname.replace(/^www\./, ''); } catch (e) { from = ''; }
        closed.push({ x: at.x, y: at.y, w: box.width, h: box.height, from: from });
        continue;
      }
      if (depth >= DEEPEST_FRAME || frames >= MOST_FRAMES) continue;
      frames++;
      gather(inner, into, at, depth + 1);
    }
  }

  function squash(s, n) {
    s = String(s || '').replace(/[\u200b-\u200f\u202a-\u202e\u2060-\u2064\ufeff]/g, '').replace(/\s+/g, ' ').trim();
    return s.length > n ? s.slice(0, n) : s;
  }

  function secret(el) {
    var tag = el.tagName.toLowerCase();
    if (tag !== 'input' && tag !== 'textarea') return false;
    var type = (el.getAttribute('type') || '').toLowerCase();
    if (type === 'password') return true;
    var auto = (el.getAttribute('autocomplete') || '').toLowerCase();
    if (/^cc-|one-time-code|current-password|new-password/.test(auto)) return true;
    var hint = [el.name, el.id, el.getAttribute('placeholder'), el.getAttribute('aria-label')].join(' ').toLowerCase();
    return /\b(cvv|cvc|card ?number|cardnumber|otp|one[- ]time|passcode|upi pin|\bpin\b|captcha|security code)\b/.test(hint);
  }

  var DATE_WORDS = /\b(date|dates|depart|departure|departing|return|returning|check[- ]?in|check[- ]?out|arrival|arrive|pick[- ]?up|drop[- ]?off|when|dob|birth)\b/i;
  var SEARCHY = /search/i;

  function hint(el) {
    return [el.id, el.getAttribute('name'), el.getAttribute('placeholder'), el.getAttribute('aria-label'), el.title].join(' ');
  }

  function suggests(el) {
    var tag = el.tagName.toLowerCase();
    var role = (el.getAttribute('role') || '').toLowerCase();
    var type = (el.getAttribute('type') || '').toLowerCase();
    if (tag === 'select' || type === 'search' || role === 'searchbox' || SEARCHY.test([el.getAttribute('placeholder'), el.getAttribute('aria-label'), el.title].join(' '))) return false;
    var auto = (el.getAttribute('aria-autocomplete') || '').toLowerCase();
    return role === 'combobox' || auto === 'list' || auto === 'both' || (tag === 'input' && el.hasAttribute('list'));
  }

  function datey(el, name) {
    var tag = el.tagName.toLowerCase();
    var type = (el.getAttribute('type') || '').toLowerCase();
    if (tag === 'input' && (type === 'date' || type === 'datetime-local')) return true;
    var fixed = tag === 'input' ? el.readOnly : !(tag === 'textarea' || el.isContentEditable);
    return fixed && DATE_WORDS.test(String(name).split(' ').slice(0, 3).join(' ') + ' ' + hint(el)) && tag !== 'a' && !/^(tab|link|menuitem|option|checkbox|radio|switch)$/.test((el.getAttribute('role') || '').toLowerCase());
  }

  function roleOf(el) {
    var tag = el.tagName.toLowerCase();
    var role = (el.getAttribute('role') || '').toLowerCase();
    var type = (el.getAttribute('type') || '').toLowerCase();
    if (steppers.has(el)) return 'AXIncrementor';
    if (secret(el)) return 'AXSecureTextField';
    if (role === 'spinbutton') return 'AXIncrementor';
    if (role === 'slider' || (tag === 'input' && type === 'range')) return 'AXSlider';
    if (role === 'textbox' || role === 'searchbox') return 'AXTextField';
    if (role === 'combobox') return suggests(el) ? 'AXComboBox' : 'AXTextField';
    if (role === 'checkbox' || role === 'switch') return 'AXCheckBox';
    if (role === 'radio') return 'AXRadioButton';
    if (role === 'tab') return 'AXTab';
    if (role === 'menuitem' || role === 'option') return 'AXMenuItem';
    if (role === 'link') return 'AXLink';
    if (role === 'button') return 'AXButton';
    if (tag === 'a') return 'AXLink';
    if (tag === 'textarea') return 'AXTextArea';
    if (tag === 'select') return 'AXComboBox';
    if (tag === 'input') {
      if (type === 'checkbox') return 'AXCheckBox';
      if (type === 'radio') return 'AXRadioButton';
      if (type === 'button' || type === 'submit' || type === 'reset' || type === 'image' || type === 'file') return 'AXButton';
      if (type === 'number') return 'AXIncrementor';
      return suggests(el) ? 'AXComboBox' : 'AXTextField';
    }
    if (el.isContentEditable) return 'AXTextArea';
    return 'AXButton';
  }

  function groupOf(el) {
    var group = el.closest && el.closest('[role=group],[role=radiogroup],fieldset');
    if (group) {
      var said = squash(group.getAttribute('aria-label') || (group.querySelector('legend') || {}).innerText, 60);
      if (said) return said;
    }
    var node = el;
    for (var up = 0; up < 5 && node && node !== document.body; up++, node = node.parentElement) {
      var peer = node.previousElementSibling;
      for (var k = 0; peer && k < 4; k++, peer = peer.previousElementSibling) {
        if (picked.has(peer) || (peer.querySelector && peer.querySelector(PICK))) continue;
        var first = String(peer.innerText || '').split('\n').map(function (l) { return squash(l, 60); }).filter(Boolean)[0] || '';
        if (/[a-z]/i.test(first)) return first;
      }
    }
    return '';
  }

  function plainName(el) {
    var name = ownName(el);
    if (!name) {
      var mark = [el.id, typeof el.className === 'string' ? el.className : '', el.getAttribute('data-cy'), el.getAttribute('data-testid')].join(' ');
      if (/close|dismiss/i.test(mark)) return 'Close';
    }
    if (name && name.length <= 4 && !/[a-z]/i.test(name)) {
      var group = groupOf(el);
      if (group) return group + ': ' + name;
    }
    return name;
  }

  function nameOf(el) {
    if (steppers.has(el)) return steppers.get(el).name;
    return plainName(el);
  }

  function ownName(el) {
    var tag = el.tagName.toLowerCase();
    var by = el.getAttribute('aria-labelledby');
    if (by) {
      var parts = by.split(/\s+/).map(function (id) {
        var n = el.ownerDocument.getElementById(id);
        return n ? n.innerText : '';
      }).join(' ');
      if (squash(parts, 80)) return squash(parts, 80);
    }
    var direct = el.getAttribute('aria-label');
    if (squash(direct, 80)) return squash(direct, 80);
    if (el.labels && el.labels.length && squash(el.labels[0].innerText, 80)) return squash(el.labels[0].innerText, 80);
    if (tag === 'input' || tag === 'textarea' || tag === 'select') {
      var type = (el.getAttribute('type') || '').toLowerCase();
      if ((type === 'submit' || type === 'button' || type === 'reset') && squash(el.value, 80)) return squash(el.value, 80);
      return squash(el.getAttribute('placeholder') || el.getAttribute('title') || el.getAttribute('name') || '', 80);
    }
    var text = squash(el.innerText, 80);
    if (text) return text;
    var img = el.querySelector('img[alt]');
    if (img && squash(img.alt, 80)) return squash(img.alt, 80);
    return squash(el.getAttribute('title') || '', 80);
  }

  function valueOf(el) {
    if (steppers.has(el)) return steppers.get(el).value;
    var tag = el.tagName.toLowerCase();
    var type = (el.getAttribute('type') || '').toLowerCase();
    if (type === 'password') return '';
    if (type === 'checkbox' || type === 'radio') return el.checked ? 'checked' : 'unchecked';
    if ((el.getAttribute('role') || '').toLowerCase() === 'slider') return squash(el.getAttribute('aria-valuenow'), 40);
    var pressed = el.getAttribute('aria-checked') || el.getAttribute('aria-selected');
    if (pressed) return pressed === 'true' ? 'checked' : 'unchecked';
    if (tag === 'select') {
      var opt = el.options[el.selectedIndex];
      return opt ? squash(opt.text, 200) : '';
    }
    if (tag === 'input' || tag === 'textarea') return squash(el.value, 200);
    if (el.isContentEditable) return squash(el.innerText, 200);
    if (!picked.has(el) && typeof el.className === 'string' && /(^|[\s_-])(selected|active|checked|current|chosen)($|[\s_-])/i.test(el.className)) return 'selected';
    return '';
  }

  function inNotice(el) {
    var n = el;
    for (var d = 0; n && n !== document.body && d < 8; d++, n = n.parentElement) {
      var mark = ((n.id || '') + ' ' + (typeof n.className === 'string' ? n.className : '')).toLowerCase();
      if (/cookie|consent|onetrust|gdpr|truste|cc-window|cmp-/.test(mark)) return true;
      var role = (n.getAttribute('role') || '').toLowerCase();
      if ((role === 'dialog' || role === 'alertdialog') && /cookie|consent/.test((n.innerText || '').slice(0, 1500).toLowerCase())) return true;
    }
    return false;
  }

  function shown(el, r) {
    if (r.width < 2 || r.height < 2) return false;
    if (el.checkVisibility && !el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) return false;
    if (el.closest && el.closest('[aria-hidden=true],[inert]')) return false;
    return true;
  }

  var steps = [0.25, 0.33, 0.5, 0.67, 0.75, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2, 2.5, 3];
  var zoom = 1;
  if (innerWidth > 0 && outerWidth > 0) {
    var guess = outerWidth / innerWidth;
    for (var s = 0; s < steps.length; s++) {
      if (Math.abs(guess - steps[s]) / steps[s] < 0.02) zoom = steps[s];
    }
  }
  var ox = screenX;
  var oy = screenY + Math.max(0, outerHeight - innerHeight * zoom);

  var found = [];
  gather(document, found, null, 0);

  var INC = /^(\+|＋|﹢)$|^(increase|increment|add|plus|more)\b/i;
  var DEC = /^(-|−|–|－|﹣)$|^(decrease|decrement|remove|minus|fewer|less)\b/i;
  var steppers = new Map();
  var picked = new Set(found);

  function said(el) {
    return squash(el.getAttribute('aria-label') || el.getAttribute('title') || el.innerText, 40);
  }

  function pointy(el) {
    try { return getComputedStyle(el).cursor === 'pointer'; } catch (e) { return false; }
  }

  var looked = document.body ? document.body.querySelectorAll('*') : [];
  var extra = [];
  for (var p = 0; p < looked.length && p < MOST_LOOKED; p++) {
    var cand = looked[p];
    if (picked.has(cand) || !pointy(cand)) continue;
    if (cand.parentElement && pointy(cand.parentElement) && cand.parentElement !== document.body) continue;
    if (/^(svg|path|img|html|body|option)$/i.test(cand.tagName)) continue;
    if (cand.closest(PICK) && cand.closest(PICK) !== cand) continue;
    var inner = cand.querySelectorAll(PICK), covers = false;
    for (var q = 0; q < inner.length; q++) {
      var ib = inner[q].getBoundingClientRect();
      if (ib.width >= 2 && ib.height >= 2 && (!inner[q].checkVisibility || inner[q].checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }))) { covers = true; break; }
    }
    if (covers) continue;
    extra.push(cand);
  }

  var buttons = found.concat(extra);
  for (var b = 0; b < buttons.length; b++) {
    var up = buttons[b];
    if (!INC.test(said(up))) continue;
    var box = up.parentElement;
    for (var lift = 0; lift < 3 && box && box !== document.body; lift++, box = box.parentElement) {
      var down = null, count = null;
      var kids = box.querySelectorAll('*');
      for (var z = 0; z < kids.length; z++) {
        var kid = kids[z];
        if (kid === up || up.contains(kid)) continue;
        if (!down && buttons.indexOf(kid) >= 0 && DEC.test(said(kid))) down = kid;
        else if (!count && kid.children.length === 0 && /^\d+$/.test(squash(kid.innerText, 8))) count = kid;
        else if (!count && kid.tagName === 'INPUT' && /^\d+$/.test(String(kid.value).trim())) count = kid;
      }
      if (down && count) {
        if (!steppers.has(box)) {
          var label = squash(box.getAttribute('aria-label'), 60);
          if (!label && box.parentElement) {
            var whole = String(box.parentElement.innerText || '').replace(String(box.innerText || ''), '');
            label = whole.split('\n').map(function (l) { return squash(l, 60); }).filter(function (l) { return /[a-z]/i.test(l); })[0] || '';
          }
          if (!label) label = groupOf(box);
          steppers.set(box, { name: label, value: count.tagName === 'INPUT' ? String(count.value).trim() : squash(count.innerText, 8) });
          extra.push(box);
        }
        break;
      }
    }
  }

  if (extra.length) {
    found = found.concat(extra);
    found.sort(function (a, b) {
      if (a === b || a.ownerDocument !== b.ownerDocument) return 0;
      return a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1;
    });
  }

  var cells = 0;
  for (var c = 0; c < found.length; c++) {
    if ((found[c].getAttribute('role') || '').toLowerCase() === 'gridcell') cells++;
  }
  var aGridOfData = cells > MOST_CELLS;

  var seen = new Set();
  var inView = [], away = [];
  for (var i = 0; i < found.length; i++) {
    var el = found[i];
    if (seen.has(el)) continue;
    seen.add(el);
    if (aGridOfData && (el.getAttribute('role') || '').toLowerCase() === 'gridcell') continue;
    var own = el.getBoundingClientRect();
    if (!shown(el, own)) continue;
    var frame = inFrame.get(el);
    var r = { left: own.left, top: own.top, right: own.right, bottom: own.bottom, width: own.width, height: own.height };
    var inItsFrame = true;
    if (frame) {
      inItsFrame = own.bottom > 0 && own.right > 0 && own.top < frame.h && own.left < frame.w;
      r.left += frame.x; r.right += frame.x; r.top += frame.y; r.bottom += frame.y;
    }
    var name = nameOf(el);
    var role = roleOf(el);
    if ((role === 'AXButton' || role === 'AXTextField') && datey(el, name)) role = 'AXDateField';
    if (!name && (role === 'AXButton' || role === 'AXLink' || role === 'AXTab' || role === 'AXMenuItem')) continue;
    var item = {
      el: el,
      role: role,
      title: name,
      value: valueOf(el),
      enabled: !(el.disabled || el.getAttribute('aria-disabled') === 'true'),
      focused: el.ownerDocument.activeElement === el,
      notice: role === 'AXButton' || role === 'AXLink' ? inNotice(el) : false,
      low: Number(el.getAttribute('min') || el.getAttribute('aria-valuemin') || 0),
      high: Number(el.getAttribute('max') || el.getAttribute('aria-valuemax') || (role === 'AXSlider' ? 100 : 0)),
      x: Math.round(ox + r.left * zoom),
      y: Math.round(oy + r.top * zoom),
      w: Math.round(r.width * zoom),
      h: Math.round(r.height * zoom)
    };
    var visible = inItsFrame && r.bottom > 0 && r.right > 0 && r.top < innerHeight && r.left < innerWidth;
    (visible ? inView : away).push(item);
  }

  for (var f = 0; f < closed.length; f++) {
    var gap = closed[f];
    if (gap.y + gap.h <= 0 || gap.x + gap.w <= 0 || gap.y >= innerHeight || gap.x >= innerWidth) continue;
    inView.push({
      el: null,
      role: 'AXGroup',
      title: 'embedded content' + (gap.from ? ' from ' + gap.from : '') + ', which cannot be read or used from here',
      value: '', enabled: false, focused: false, notice: false, low: 0, high: 0,
      x: Math.round(ox + gap.x * zoom), y: Math.round(oy + gap.y * zoom),
      w: Math.round(gap.w * zoom), h: Math.round(gap.h * zoom)
    });
  }

  var kept = inView.concat(away).slice(0, MOST);
  window.__vansEls = kept.map(function (k) { return k.el; });

  return JSON.stringify({
    url: location.href,
    host: location.hostname.replace(/^www\./, ''),
    title: document.title,
    ready: document.readyState,
    view: { x: Math.round(ox), y: Math.round(oy), w: Math.round(innerWidth * zoom), h: Math.round(innerHeight * zoom) },
    controls: kept.map(function (k) {
      return { role: k.role, title: k.title, value: k.value, enabled: k.enabled, focused: k.focused, notice: k.notice, low: k.low, high: k.high, x: k.x, y: k.y, w: k.w, h: k.h };
    })
  });
})()
