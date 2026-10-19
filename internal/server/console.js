document.documentElement.classList.add('js');
document.addEventListener('DOMContentLoaded', function () {
  function copied(button, text) {
    if (button.classList.contains('copied')) return;
    var was = button.getAttribute('data-label') || button.textContent;
    button.setAttribute('data-label', was);
    var icon = button.hasAttribute('data-icon');
    var shape = button.innerHTML;
    var done = function () {
      if (icon) button.innerHTML = '<svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 8.5 6.5 12 13 4.5"/></svg>';
      else button.textContent = 'Copied';
      button.classList.add('copied');
      setTimeout(function () {
        if (icon) button.innerHTML = shape; else button.textContent = was;
        button.classList.remove('copied');
      }, 1400);
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(done, function () {});
    }
  }

  document.querySelectorAll('[data-copy]').forEach(function (button) {
    button.hidden = false;
    button.addEventListener('click', function (event) {
      event.preventDefault();
      copied(button, button.getAttribute('data-copy'));
    });
  });

  document.querySelectorAll('.flash [data-close]').forEach(function (button) {
    button.hidden = false;
    button.addEventListener('click', function () { button.closest('.flash').remove(); });
  });

  document.querySelectorAll('form.allowance').forEach(function (form) {
    var input = form.querySelector('input[name=daily]');
    var start = input.value;
    input.addEventListener('input', function () {
      form.classList.toggle('dirty', input.value !== start);
    });
  });

  var bar = document.querySelector('.many');
  if (!bar) return;
  var boxes = Array.prototype.slice.call(document.querySelectorAll('input[form=many][name=code]'));
  var all = document.querySelector('[data-all]');
  var count = bar.querySelector('[data-count]');
  function update() {
    var ticked = boxes.filter(function (b) { return b.checked; }).length;
    bar.classList.toggle('shown', ticked > 0);
    if (count) count.textContent = ticked + ' selected';
    if (all) {
      all.checked = ticked > 0 && ticked === boxes.length;
      all.indeterminate = ticked > 0 && ticked < boxes.length;
    }
    boxes.forEach(function (b) { b.closest('tr').classList.toggle('picked', b.checked); });
  }
  boxes.forEach(function (b) { b.addEventListener('change', update); });
  if (all) {
    all.hidden = false;
    all.addEventListener('change', function () {
      boxes.forEach(function (b) { b.checked = all.checked; });
      update();
    });
  }
  var clear = bar.querySelector('[data-clear]');
  if (clear) {
    clear.hidden = false;
    clear.addEventListener('click', function () {
      boxes.forEach(function (b) { b.checked = false; });
      update();
    });
  }
  update();
});
