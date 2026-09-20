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
      "hero.headline": "Encuentra aparcamiento en cualquier lugar.",
      "hero.support":
        "ParkXchange conecta a quien busca un sitio para aparcar en la ciudad con quien se va a ir de todas formas.",
      "hero.cta": "Cómo funciona",
      "hero.imageAlt":
        "Calle urbana llena de coches aparcados, sin huecos libres",
      "how.title": "Cómo funciona",
      "how.step1.title": "Anuncia",
      "how.step1.body":
        "Cuando te vas, publicas cuándo y dónde liberas tu plaza de aparcamiento.",
      "how.step2.title": "Reserva",
      "how.step2.body":
        "Quien busca aparcamiento lo ve en el mapa y reserva el intercambio.",
      "how.step3.title": "Intercambio",
      "how.step3.body":
        "En cuanto el otro coche llega, el que ocupaba el hueco sale para dejárselo libre.",
      "footer.contact": "Contacto",
      "footer.tagline":
        "Información y cortesía sobre aparcamiento — no vendemos suelo público.",

      "privacy.title": "Política de privacidad",
      "privacy.updated": "Última actualización: 20 de septiembre de 2026",
      "privacy.disclaimer":
        "Este texto es un borrador de buena fe alineado con el RGPD y la LOPDGDD. No constituye asesoramiento jurídico.",
      "privacy.controller.h": "Responsable del tratamiento",
      "privacy.controller.p":
        "El responsable es Marcos Salvo (persona física), España. Contacto: marcossalvo95@gmail.com.",
      "privacy.service.h": "Qué es ParkXchange",
      "privacy.service.p":
        "ParkXchange es una plataforma entre particulares que facilita el intercambio de información sobre cuándo y dónde alguien va a liberar un hueco de aparcamiento, y un servicio de cortesía de espera breve. No vendemos, alquilamos ni cedemos derechos sobre suelo o vía pública.",
      "privacy.data.h": "Datos que tratamos",
      "privacy.data.p":
        "Podemos tratar: datos de cuenta (email, nombre visible, teléfono opcional, hash de contraseña o vínculo con Google); datos de ofertas y ubicación (coordenadas, con difuminado en el mapa público hasta la reserva, horarios, notas, precio orientativo en puntos); vehículos (matrícula, marca/modelo, tamaño, color, año, foto opcional); reservas e intercambio; valoraciones; saldo de puntos (créditos internos); sesiones (tokens de refresco y, cuando procede, user-agent).",
      "privacy.google.h": "Datos de usuario de Google",
      "privacy.google.p":
        "Si eliges «Iniciar sesión con Google», recibimos un token de identidad de Google y, a partir de él, tu email (y nombre si Google lo proporciona) solo para autenticarte y crear o vincular tu cuenta ParkXchange. No usamos datos de Google para publicidad. El uso se limita a las prácticas descritas en esta política y a los requisitos de Limited Use de Google.",
      "privacy.purposes.h": "Finalidades",
      "privacy.purposes.p":
        "Prestar el servicio (cuenta, mapa, reservas, encuentros, puntos), autenticación, prevención de abuso y fraude, y atención al usuario.",
      "privacy.bases.h": "Bases jurídicas",
      "privacy.bases.p":
        "Ejecución del contrato (art. 6.1.b RGPD); interés legítimo en seguridad y prevención de abuso (art. 6.1.f); obligación legal cuando proceda (art. 6.1.c). No enviamos marketing no solicitado.",
      "privacy.recipients.h": "Destinatarios",
      "privacy.recipients.p":
        "Proveedores de infraestructura y correo que actúan como encargados del tratamiento; Google, como proveedor de identidad si usas Google Sign-In. No vendemos datos personales.",
      "privacy.transfers.h": "Transferencias internacionales",
      "privacy.transfers.p":
        "Algunos encargados (por ejemplo Google u hospedaje) pueden tratar datos fuera del EEE. Cuando ocurra, se aplicarán garantías adecuadas (como cláusulas contractuales tipo) según la normativa aplicable.",
      "privacy.retention.h": "Conservación",
      "privacy.retention.p":
        "Conservamos los datos mientras la cuenta esté activa y el tiempo necesario para disputas, seguridad u obligaciones legales. Si borras la cuenta desde la app (Cuenta → Borrar cuenta), anonimizamos o eliminamos los datos personales de forma automática. El historial operativo (por ejemplo asientos del libro de puntos) puede conservarse sin datos identificativos cuando la ley o la seguridad lo exijan. En el MVP, el saldo de puntos restante se pierde al borrar la cuenta.",
      "privacy.rights.h": "Tus derechos",
      "privacy.rights.p":
        "Puedes ejercer la supresión borrando tu cuenta en la app (Cuenta → Borrar cuenta), con confirmación previa. Para acceso, rectificación, limitación, portabilidad, oposición u otras solicitudes, escribe a marcossalvo95@gmail.com. También puedes reclamar ante la Agencia Española de Protección de Datos (AEPD).",
      "privacy.children.h": "Menores",
      "privacy.children.p":
        "El servicio está pensado para usuarios de 16 años o más.",
      "privacy.changes.h": "Cambios",
      "privacy.changes.p":
        "Podemos actualizar esta política. La fecha de la parte superior indica la versión vigente. Los cambios relevantes se reflejarán en esta página.",

      "terms.title": "Condiciones de servicio",
      "terms.updated": "Última actualización: 20 de septiembre de 2026",
      "terms.disclaimer":
        "Este texto es un borrador de buena fe. No constituye asesoramiento jurídico.",
      "terms.operator.h": "Operador",
      "terms.operator.p":
        "ParkXchange es operado por Marcos Salvo, España. Contacto: marcossalvo95@gmail.com.",
      "terms.object.h": "Objeto del servicio",
      "terms.object.p":
        "El servicio facilita el intercambio de información sobre salidas de aparcamiento (dónde y cuándo se libera un hueco) y un servicio de cortesía de espera breve entre conductores. No otorga propiedad, arrendamiento ni derecho de ocupación sobre la vía o el suelo público.",
      "terms.obligations.h": "Obligaciones del usuario",
      "terms.obligations.p":
        "Debes proporcionar información veraz, respetar las normas de tráfico y estacionamiento aplicables, no acosar a otros usuarios y no eludir la plataforma de mala fe para evitar puntos u otras contraprestaciones del sistema.",
      "terms.points.h": "Puntos",
      "terms.points.p":
        "Los puntos son créditos internos de la aplicación. No son dinero de curso legal y no garantizan canje en efectivo salvo que en el futuro se active una función de pago o cobro real y se describa en estas condiciones. Si borras la cuenta en el MVP, pierdes el saldo de puntos restante.",
      "terms.future.h": "Monetización futura",
      "terms.future.p":
        "El operador puede introducir pagos o cobros en dinero real. Se informará a los usuarios; el uso continuado tras el aviso puede implicar la aceptación de las condiciones actualizadas cuando la ley lo permita.",
      "terms.noguarantee.h": "Sin garantía de hueco",
      "terms.noguarantee.p":
        "ParkXchange es un mercado entre pares de mejor esfuerzo. No garantizamos que encuentres un hueco ni la conducta de otros usuarios.",
      "terms.liability.h": "Responsabilidad",
      "terms.liability.p":
        "En la medida permitida por la ley, la responsabilidad del operador se limita de forma razonable para un servicio entre particulares. Tú sigues siendo responsable de tu conducción y del cumplimiento de las normas de estacionamiento.",
      "terms.suspend.h": "Suspensión",
      "terms.suspend.p":
        "Podemos suspender o cerrar cuentas ante abuso, fraude o incumplimiento de estas condiciones.",
      "terms.law.h": "Ley aplicable",
      "terms.law.p":
        "Estas condiciones se rigen por la ley española. Si eres consumidor en España, puedes acudir a los tribunales de tu domicilio; en los demás casos, a los tribunales competentes en España.",
      "terms.contact.h": "Contacto",
      "terms.contact.p":
        "Para notificaciones relacionadas con estas condiciones: marcossalvo95@gmail.com.",
    },
    en: {
      "nav.privacy": "Privacy",
      "nav.terms": "Terms",
      "nav.lang.es": "ES",
      "nav.lang.en": "EN",
      brand: "ParkXchange",
      "hero.headline": "Find parking anywhere.",
      "hero.support":
        "ParkXchange connects people looking for a place to park in the city with drivers who are leaving anyway.",
      "hero.cta": "How it works",
      "hero.imageAlt":
        "City street packed with parked cars and no free spaces",
      "how.title": "How it works",
      "how.step1.title": "Announce",
      "how.step1.body":
        "When you leave, you publish when and where your parking space frees up.",
      "how.step2.title": "Reserve",
      "how.step2.body":
        "Drivers looking for parking see it on the map and reserve the exchange.",
      "how.step3.title": "Handover",
      "how.step3.body":
        "As soon as the other car arrives, the one occupying the space leaves and frees it up.",
      "footer.contact": "Contact",
      "footer.tagline":
        "Information and courtesy about parking — we don’t sell public land.",

      "privacy.title": "Privacy policy",
      "privacy.updated": "Last updated: 20 September 2026",
      "privacy.disclaimer":
        "This text is a good-faith draft aligned with the GDPR and Spain’s LOPDGDD. It is not legal advice.",
      "privacy.controller.h": "Data controller",
      "privacy.controller.p":
        "The controller is Marcos Salvo (natural person), Spain. Contact: marcossalvo95@gmail.com.",
      "privacy.service.h": "What ParkXchange is",
      "privacy.service.p":
        "ParkXchange is a peer platform that facilitates sharing information about when and where someone is about to free a parking space, plus a short courtesy waiting service. We do not sell, lease, or transfer rights over public land or roadway.",
      "privacy.data.h": "Data we process",
      "privacy.data.p":
        "We may process: account data (email, display name, optional phone, password hash or Google link); offer and location data (coordinates, with map fuzzing on the public map until reservation, timing, notes, guide points price); vehicles (plate, make/model, size, color, year, optional photo); reservations and exchange state; ratings; points balance (internal credits); sessions (refresh tokens and, where relevant, user-agent).",
      "privacy.google.h": "Google user data",
      "privacy.google.p":
        "If you choose Google Sign-In, we receive a Google ID token and, from it, your email (and name if Google provides it) solely to authenticate you and create or link your ParkXchange account. We do not use Google data for advertising. Use is limited to the practices in this policy and Google’s Limited Use requirements.",
      "privacy.purposes.h": "Purposes",
      "privacy.purposes.p":
        "Providing the service (account, map, reservations, handovers, points), authentication, abuse and fraud prevention, and user support.",
      "privacy.bases.h": "Legal bases",
      "privacy.bases.p":
        "Contract performance (GDPR art. 6.1.b); legitimate interest in security and abuse prevention (art. 6.1.f); legal obligation where applicable (art. 6.1.c). We do not send unsolicited marketing.",
      "privacy.recipients.h": "Recipients",
      "privacy.recipients.p":
        "Infrastructure and email providers acting as processors; Google as identity provider if you use Google Sign-In. We do not sell personal data.",
      "privacy.transfers.h": "International transfers",
      "privacy.transfers.p":
        "Some processors (for example Google or hosting) may process data outside the EEA. Where that happens, appropriate safeguards (such as standard contractual clauses) will apply under applicable law.",
      "privacy.retention.h": "Retention",
      "privacy.retention.p":
        "We keep data while the account is active and as needed for disputes, security, or legal duties. If you delete your account in the app (Account → Delete account), we automatically delete or anonymise personal data. Operational history (for example points ledger entries) may be retained without identifying data where required for law or security. In the MVP, any remaining points balance is forfeited when you delete your account.",
      "privacy.rights.h": "Your rights",
      "privacy.rights.p":
        "You can erase your data by deleting your account in the app (Account → Delete account), after a confirmation prompt. For access, rectification, restriction, portability, objection, or other requests, email marcossalvo95@gmail.com. You may also lodge a complaint with Spain’s AEPD.",
      "privacy.children.h": "Children",
      "privacy.children.p":
        "The service is intended for users aged 16 or older.",
      "privacy.changes.h": "Changes",
      "privacy.changes.p":
        "We may update this policy. The date above shows the current version. Material changes will appear on this page.",

      "terms.title": "Terms of service",
      "terms.updated": "Last updated: 20 September 2026",
      "terms.disclaimer":
        "This text is a good-faith draft. It is not legal advice.",
      "terms.operator.h": "Operator",
      "terms.operator.p":
        "ParkXchange is operated by Marcos Salvo, Spain. Contact: marcossalvo95@gmail.com.",
      "terms.object.h": "Object of the service",
      "terms.object.p":
        "The service facilitates exchange of information about parking departures (where and when a space frees up) and a short courtesy waiting service between drivers. It does not grant ownership, lease, or occupancy rights over public roadway or land.",
      "terms.obligations.h": "User obligations",
      "terms.obligations.p":
        "You must provide truthful information, follow applicable traffic and parking rules, not harass other users, and not circumvent the platform in bad faith to avoid points or other in-system consideration.",
      "terms.points.h": "Points",
      "terms.points.p":
        "Points are internal app credits. They are not legal tender and do not guarantee cash-out unless a real payment or payout feature ships and is described in these terms. If you delete your account in the MVP, any remaining points balance is forfeited.",
      "terms.future.h": "Future monetisation",
      "terms.future.p":
        "The operator may introduce real-money payments or payouts. Users will be informed; continued use after notice may constitute acceptance of updated terms where legally permitted.",
      "terms.noguarantee.h": "No guarantee of a spot",
      "terms.noguarantee.p":
        "ParkXchange is a best-effort peer marketplace. We do not guarantee you will find a spot or how other users behave.",
      "terms.liability.h": "Liability",
      "terms.liability.p":
        "To the extent permitted by law, the operator’s liability is reasonably limited for a peer marketplace. You remain responsible for your driving and for complying with parking rules.",
      "terms.suspend.h": "Suspension",
      "terms.suspend.p":
        "We may suspend or close accounts for abuse, fraud, or breach of these terms.",
      "terms.law.h": "Governing law",
      "terms.law.p":
        "These terms are governed by Spanish law. If you are a consumer in Spain, you may bring claims in the courts of your domicile; otherwise in the competent courts of Spain.",
      "terms.contact.h": "Contact",
      "terms.contact.p":
        "For notices about these terms: marcossalvo95@gmail.com.",
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
    var altNodes = document.querySelectorAll("[data-i18n-alt]");
    for (var a = 0; a < altNodes.length; a++) {
      var altEl = altNodes[a];
      var altKey = altEl.getAttribute("data-i18n-alt");
      if (altKey && Object.prototype.hasOwnProperty.call(dict, altKey)) {
        altEl.setAttribute("alt", dict[altKey]);
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
