// erreia board view: binds Sortable.js to column bodies for drag-and-drop,
// and re-binds when the board fragment is replaced via SSE.
// Loaded as an external script so it complies with the strict CSP
// (script-src 'self' with no 'unsafe-inline').

(function () {
  function bindSortable() {
    document.querySelectorAll('[data-cards]').forEach(function (el) {
      if (el._sortable) return;
      el._sortable = new Sortable(el, {
        group: 'cards',
        animation: 120,
        onEnd: function (evt) {
          var cardEl = evt.item;
          var cardId = cardEl.getAttribute('data-card-id');
          var destColumn = evt.to.getAttribute('data-column-id');
          var pos = evt.newIndex;
          var csrf = document.querySelector('meta[name="csrf-token"]') && document.querySelector('meta[name="csrf-token"]').content || '';
          fetch('/cards/' + cardId + '/move', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'X-CSRF-Token': csrf },
            body: 'column_id=' + encodeURIComponent(destColumn) + '&position=' + pos + '&csrf_token=' + encodeURIComponent(csrf)
          });
        }
      });
    });
  }

  document.addEventListener('htmx:afterSwap', bindSortable);
  document.addEventListener('DOMContentLoaded', bindSortable);
})();
