(function () {
  var PICK = 'a[href],button,input,textarea,select,summary,[role=button],[role=link],[role=tab],' +
    '[role=menuitem],[role=option],[role=checkbox],[role=radio],[role=switch],[role=combobox],' +
    '[role=textbox],[role=searchbox],[role=gridcell],[role=slider],[contenteditable=""],[contenteditable=true],[onclick]';
  var MOST = 400;
  var MOST_CELLS = 120;

  function gather(root, into) {
    var found = root.querySelectorAll(PICK);
    for (var i = 0; i < found.length; i++) into.push(found[i]);
    var all = root.querySelectorAll('*');
    for (var j = 0; j < all.length; j++) {
      if (all[j].shadowRoot) gather(all[j].shadowRoot, into);
    }
  }

  function squash(s, n) {
    s = String(s || '').replace(/\s+/g, ' ').trim();
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

  function roleOf(el) {
    var tag = el.tagName.toLowerCase();
    var role = (el.getAttribute('role') || '').toLowerCase();
    var type = (el.getAttribute('type') || '').toLowerCase();
    if (secret(el)) return 'AXSecureTextField';
    if (role === 'slider' || (tag === 'input' && type === 'range')) return 'AXSlider';
    if (role === 'textbox' || role === 'searchbox') return 'AXTextField';
    if (role === 'combobox') return 'AXComboBox';
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
      return 'AXTextField';
    }
    if (el.isContentEditable) return 'AXTextArea';
    return 'AXButton';
  }

  function nameOf(el) {
    var tag = el.tagName.toLowerCase();
    var by = el.getAttribute('aria-labelledby');
    if (by) {
      var parts = by.split(/\s+/).map(function (id) {
        var n = document.getElementById(id);
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
  gather(document, found);

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
    var r = el.getBoundingClientRect();
    if (!shown(el, r)) continue;
    var name = nameOf(el);
    var role = roleOf(el);
    if (!name && (role === 'AXButton' || role === 'AXLink' || role === 'AXTab' || role === 'AXMenuItem')) continue;
    var item = {
      el: el,
      role: role,
      title: name,
      value: valueOf(el),
      enabled: !(el.disabled || el.getAttribute('aria-disabled') === 'true'),
      focused: document.activeElement === el,
      notice: role === 'AXButton' || role === 'AXLink' ? inNotice(el) : false,
      low: Number(el.getAttribute('min') || el.getAttribute('aria-valuemin') || 0),
      high: Number(el.getAttribute('max') || el.getAttribute('aria-valuemax') || (role === 'AXSlider' ? 100 : 0)),
      x: Math.round(ox + r.left * zoom),
      y: Math.round(oy + r.top * zoom),
      w: Math.round(r.width * zoom),
      h: Math.round(r.height * zoom)
    };
    var visible = r.bottom > 0 && r.right > 0 && r.top < innerHeight && r.left < innerWidth;
    (visible ? inView : away).push(item);
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
