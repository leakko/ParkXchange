(function (root) {
  "use strict";

  var STORAGE_KEY = "parkxchange.lang";

  function resolveLocale(tag) {
    if (typeof tag === "string" && tag.toLowerCase().indexOf("en") === 0) {
      return "en";
    }
    return "es";
  }

  var messages = {
    es: {
      "nav.privacy": "Privacidad",
      "nav.terms": "Condiciones",
      "nav.lang.es": "ES",
      "nav.lang.en": "EN",
      brand: "ParkXchange",
      "hero.headline":
        "Encuentra sitio donde casi no hay — o gana por dejar el tuyo al irte",
      "hero.support":
        "Quien busca reserva un hueco real. Quien se va monetiza una salida que ya iba a hacer.",
      "hero.cta": "Cómo funciona",
      "how.title": "Cómo funciona",
      "how.step1.title": "Anuncia",
      "how.step1.body":
        "Cuando te vas, publicas cuándo y dónde liberas el hueco.",
      "how.step2.title": "Reserva",
      "how.step2.body":
        "Quien busca lo ve en el mapa y reserva el intercambio.",
      "how.step3.title": "Encuentro breve",
      "how.step3.body":
        "Os encontráis un momento: cortesía de espera a cambio de puntos.",
      "footer.contact": "Contacto",
      "footer.tagline":
        "Información y cortesía — no vendemos suelo público.",
    },
    en: {
      "nav.privacy": "Privacy",
      "nav.terms": "Terms",
      "nav.lang.es": "ES",
      "nav.lang.en": "EN",
      brand: "ParkXchange",
      "hero.headline":
        "Find parking where it’s scarce — or earn when you leave yours",
      "hero.support":
        "Seekers reserve a real handover. Leavers earn from a departure they were making anyway.",
      "hero.cta": "How it works",
      "how.title": "How it works",
      "how.step1.title": "Announce",
      "how.step1.body":
        "When you leave, you publish when and where the space frees up.",
      "how.step2.title": "Reserve",
      "how.step2.body":
        "Seekers see it on the map and reserve the exchange.",
      "how.step3.title": "Brief meetup",
      "how.step3.body":
        "You meet briefly: courtesy waiting in exchange for points.",
      "footer.contact": "Contact",
      "footer.tagline":
        "Information and courtesy — we don’t sell public land.",
    },
  };

  function applyTranslations(locale) {
    if (locale !== "es" && locale !== "en") {
      locale = "es";
    }
    var dict = messages[locale];
    if (typeof document === "undefined") {
      return;
    }
    document.documentElement.lang = locale;
    var nodes = document.querySelectorAll("[data-i18n]");
    for (var i = 0; i < nodes.length; i++) {
      var el = nodes[i];
      var key = el.getAttribute("data-i18n");
      if (key && Object.prototype.hasOwnProperty.call(dict, key)) {
        el.textContent = dict[key];
      }
    }
    var hrefNodes = document.querySelectorAll("[data-i18n-href]");
    for (var j = 0; j < hrefNodes.length; j++) {
      var hrefEl = hrefNodes[j];
      var hrefKey = hrefEl.getAttribute("data-i18n-href");
      if (hrefKey && Object.prototype.hasOwnProperty.call(dict, hrefKey)) {
        hrefEl.setAttribute("href", dict[hrefKey]);
      }
    }
    try {
      localStorage.setItem(STORAGE_KEY, locale);
    } catch (e) {
      /* ignore quota / private mode */
    }
    var buttons = document.querySelectorAll("[data-lang]");
    for (var k = 0; k < buttons.length; k++) {
      var btn = buttons[k];
      var active = btn.getAttribute("data-lang") === locale;
      btn.setAttribute("aria-pressed", active ? "true" : "false");
      btn.classList.toggle("is-active", active);
    }
  }

  function initI18n() {
    if (typeof document === "undefined") {
      return;
    }
    var stored = null;
    try {
      stored = localStorage.getItem(STORAGE_KEY);
    } catch (e) {
      stored = null;
    }
    var locale =
      stored === "es" || stored === "en"
        ? stored
        : resolveLocale(
            typeof navigator !== "undefined" ? navigator.language : undefined,
          );
    applyTranslations(locale);
    var buttons = document.querySelectorAll("[data-lang]");
    for (var i = 0; i < buttons.length; i++) {
      buttons[i].addEventListener("click", function (ev) {
        var lang = ev.currentTarget.getAttribute("data-lang");
        if (lang === "es" || lang === "en") {
          applyTranslations(lang);
        }
      });
    }
  }

  var api = {
    STORAGE_KEY: STORAGE_KEY,
    resolveLocale: resolveLocale,
    messages: messages,
    applyTranslations: applyTranslations,
    initI18n: initI18n,
  };

  root.ParkXchangeI18n = api;

  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
})(typeof window !== "undefined" ? window : globalThis);
