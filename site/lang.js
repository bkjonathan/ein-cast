// Language switch for the landing page: English or Burmese, remembered in this
// browser only. Storage can be missing or blocked, so every access is guarded.
(function () {
  var root = document.documentElement;
  var KEY = "eincast.site.lang";

  function stored() {
    try { return localStorage.getItem(KEY); } catch (e) { return null; }
  }
  function remember(lang) {
    try { localStorage.setItem(KEY, lang); } catch (e) { /* the choice lasts until the tab closes */ }
  }
  function apply(lang) {
    root.setAttribute("data-lang", lang);
    root.setAttribute("lang", lang);
    document.querySelectorAll("[data-set-lang]").forEach(function (b) {
      b.setAttribute("aria-pressed", String(b.getAttribute("data-set-lang") === lang));
    });
  }

  var first = stored() || ((navigator.language || "").toLowerCase().indexOf("my") === 0 ? "my" : "en");
  apply(first === "my" ? "my" : "en");

  document.querySelectorAll("[data-set-lang]").forEach(function (b) {
    b.addEventListener("click", function () {
      var lang = b.getAttribute("data-set-lang") === "my" ? "my" : "en";
      remember(lang);
      apply(lang);
    });
  });
})();
