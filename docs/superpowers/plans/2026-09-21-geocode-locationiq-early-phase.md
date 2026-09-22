# Geocode: LocationIQ Free + endurecer — Implementation Plan

> **For agentic workers:** implement task-by-task. Steps use checkbox (`- [ ]`) syntax.
> **Status:** approved for implementation (chat 2026-09-21/22). Not started.
> **Cursor plan mirror:** `~/.cursor/plans/geocode_early_phase_f137232c.plan.md`

**Goal:** Dejar de depender de Nominatim/Photon públicos; usar LocationIQ Free como primario; endurecer caché/fan-out; corregir portales y categorías; y UX de búsqueda que ayude a distinguir categoría vs local vs calle (sin disparar la cuota).

**Architecture:** Misma fachada `searchPlacesDetailed` / `reverseGeocode` en `apps/mobile/src/map/geocode.ts`. LocationIQ (`eu1`): search/structured + **autocomplete** (nombres parciales) + nearby (categorías) + reverse. Photon solo fallback fuzzy tipográfico. Sin proxy Go en esta fase.

**Tech Stack:** Expo env (`EXPO_PUBLIC_LOCATIONIQ_KEY`), LocationIQ EU, Photon (opcional fuzzy), MapLibre (sin cambio).

## Decisiones

| Tema | Elección |
| --- | --- |
| Proveedor primario | **LocationIQ Free** (search + autocomplete + reverse + nearby) |
| Región | `eu1.locationiq.com` (España) |
| Photon | Solo si LocationIQ no trae hits; desactivable |
| Proxy API Go | No (MVP: key en `EXPO_PUBLIC_*`) |
| Mapbox / Geoapify / Google Places | Fuera de alcance |
| Self-host Nominatim | No |
| Idioma de búsqueda | **Locale de la app** (`es` \| `en`): léxico de categorías + `accept-language` |
| Fuzzy / typos | **Autocomplete LocationIQ** (parciales) + aliases/Levenshtein de marcas en cliente + **Photon** si vacío. No Pelias/Geoapify en esta fase. |
| UX distinción | Row categoría (tap→Nearby) + autocomplete debounced + heurística al confirmar; filtro Todo\|Sitios\|Calles = follow-up 1b |

### Capacidad (referencia)

Uso típico parking con debounce+caché ≈ **2–3 req por usuario activo/día** (búsqueda ± reverse ± nearby).

| Plan | Precio | Cupo | Usuarios activos/día (aprox.) | Pico |
| --- | --- | --- | --- | --- |
| Free | 0 $ | 5k req/día | ~1.5k–2.5k | 2 req/s |
| Developer | **~100 $/mes** | 25k req/día | ~8k–12k | 20 req/s |
| Startup | **~200 $/mes** | 60k req/día | ~20k–30k | 22 req/s |
| Growth Plus | **~500 $/mes** | 7.5M req/mes (~250k/día) | ~80k–120k | 30 req/s |
| Business Plus | **~950 $/mes** | 30M req/mes | cientos de miles | 40 req/s |

Notas:

- **Maps Lite (~45 $) no sirve** para geocode (solo mapas).
- Soft limit en planes de pago: a menudo hasta ~2× el cupo diario antes de cortar.
- Programa startups LocationIQ: hasta ~50 % dto. el 1er año si elegís.
- **Puente a self-host:** subir de plan es solo billing/key; el código del plan (env URL + key) no cambia. Self-host Nominatim/Photon/Pelias es otro proyecto (ops + disco); usar Developer/Startup meses mientras se monta es la vía razonable.
- Cancelación: suscripción mensual; no hace falta compromiso largo para el puente.

### Self-host en Hetzner (cuando toque; no esta fase)

El API hoy va en un **CX barato** (`deploy/hetzner`). Geocode self-host es **otra máquina** (o mucho más RAM/disco). No cabe cómodo junto a PostGIS en el mismo CX23/33.

| Alcance | Qué montar | Hardware típico Hetzner | Coste orden de mag. (sin IVA) | Notas |
| --- | --- | --- | --- | --- |
| **España** (recomendado si self-host) | **Nominatim + Photon** (mismo VPS) | CX53 ~32 GB / 320 GB (+ volumen si hace falta) | **~25–45 €/mes** | **Cubre todo el producto de búsqueda:** Nominatim = search/reverse/portal; Photon = fuzzy/parciales + categorías por tag OSM. Import horas. Updates Geofabrik. Ops no incluidas en el €. |
| **Europa** | Nominatim europe (± Photon) | ~64 GB RAM + cientos GB NVMe (CCX43 o similar + volumen) | **~100–280 €/mes** | Ya compite mal con LocationIQ Startup/Growth. |
| **Planeta** | Nominatim full | ≥64–128 GB RAM, ~1 TB NVMe | **~250–550+ €/mes** | Absurdo vs LocationIQ hasta tráfico enorme. |

**Aclaración:** los ~30 € de España **no son solo Nominatim**. Con Nominatim + Photon en esa máquina tenéis búsqueda normal, fuzzy y categorías. No hace falta un segundo proveedor de pago para eso (sí hace falta vuestro tiempo de ops).

Comparado con LocationIQ:

- Self-host España (~30 €/mes) **gana en € fijos** frente a Developer (100 $) / Startup (200 $), **si** aceptáis ops (import, updates, backups, monitoreo, discos).
- Hasta ~10k usuarios activos/día, **pagar LocationIQ suele salir más barato en tiempo** (cero ops de geocode).
- Nearby/Autocomplete de LocationIQ no salen “gratis” en Nominatim: hay que montar Photon (tags) + autocomplete propio o Pelias.

**Conclusión de fases:** Free → paid LocationIQ (puente) → self-host **España** en Hetzner cuando el plan de pago duela y el mapa siga centrado en ES. No planet.

---

## Diagnóstico: por qué fallan las dos búsquedas hoy

Código actual: solo `q=` free-form a Nominatim (+ Photon fuzzy). Optimizado para marcas (`BRAND_QUERY_ALIASES`), no para direcciones ni categorías.

### 1) «Avenida Las Golondrinas, 18» → calle sí, número 18 no

Causas típicas (combinables):

1. **Formato ES vs lo que espera el geocoder.** Nominatim/LocationIQ rinden mejor con el número delante (`18 Avenida Las Golondrinas`) o con búsqueda **estructurada** `street=18 Avenida Las Golondrinas`. Nosotros mandamos el string tal cual (`…, 18`).
2. **`bounded=1` + viewbox.** Prioriza lo que cae en la caja; el punto del portal 18 puede quedar fuera o perderse frente al way de la calle.
3. **Datos OSM.** Si el portal no está mapeado (ni interpolación), el motor **devuelve la calle a propósito** (penaliza “falta número”). Eso no se arregla solo cambiando de proveedor; sí se puede priorizar hits con `address.house_number` coincidente y avisar cuando solo hay calle.

LocationIQ no cambia el modelo OSM; sí permite structured search y `addressdetails=1` para filtrar/ordenar por número.

### 2) «Peluquería» → no lista peluquerías de la zona

La search free-form busca **texto en nombres**, no el tag OSM `shop=hairdresser`. Un local llamado «Corte & Estilo» no matchea «Peluquería».

LocationIQ tiene **Nearby POI** (`/v1/nearby?lat&lon&radius&tag=…`) pensado exactamente para esto. Contará contra el mismo cupo Free.

---

## Approach tras el cambio a LocationIQ

### 0) Cómo se decide el tipo de query (routing)

No hay un clasificador ML: reglas baratas sobre el texto + locale, en este orden:

```
query normalizada
  │
  ├─ ¿Parece dirección? (tipo de vía + número, o "calle …, 18")
  │     → rama A (structured / portal)
  │
  ├─ ¿Coincide exactamente con una categoría del léxico del locale?
  │     (p. ej. "peluquería", "hairdresser" — no "Peluquería Ana")
  │     → rama B (Nearby por tag)
  │
  └─ resto (marcas, nombres de local, texto libre)
        → rama C (Autocomplete → Search → Photon)
```

| Entrada ejemplo | Rama | Por qué |
| --- | --- | --- |
| `Avenida Las Golondrinas, 18` | A | vía + número |
| `peluquería` / `hairdresser` | B | match exacto (tras normalizar) en léxico del locale |
| `Burger King` / `Corte & Estilo` | C | no es vía+número ni categoría pura |
| `Peluquería Ana` | C | tiene más tokens → nombre de local, no categoría |

Ambiguidades aceptadas en MVP: si alguien busca solo `calle` sin número → C (o A sin número si detectamos tipo de vía); si escribe una categoría mal ortografiada que no está en el léxico → C (Photon puede ayudar). Ampliar léxico > NLP.

### Orden cerca → lejos (se mantiene)

Con viewport: tras obtener hits, **`sortHitsNearToFar` respecto al centro del viewbox** (como hoy). Nearby ya viene por distancia; igual se unifica el sort. Sin viewport: sin ese bias (o centro GPS si está disponible). `inViewport` / `shouldZoomOut` se conservan.

### A) Direcciones con número

1. Detectar queries tipo dirección: tipo de vía (según locale: avenida/calle/plaza… o avenue/street/square…) + número.
2. Normalizar variantes:
   - free-form: `18 Avenida Las Golondrinas`
   - structured: `street=18 Avenida Las Golondrinas` (+ `countrycodes=es` si aplica)
3. Preferir resultados cuyo `address.house_number` coincida con el pedido; subirlos al top.
4. Si solo hay matches de calle (sin número): devolverlos igual (útil para ir a la zona) pero **no fingir** que es el portal; label de calle sin inventar el 18.
5. Para este modo, no insistir en `bounded=1` demasiado estricto en la primera pasada (viewbox como bias, no muro).
6. `accept-language` según locale de la app.

### B) Categorías genéricas (léxico según idioma de la app)

1. Léxico **por locale de la app** (`AppLocale`: `es` | `en`, el que ya usa i18n) → mismos tags OSM. No un mapa solo en español.
   - `es`: peluquería, farmacia, supermercado, gasolinera, aparcamiento/parking, restaurante, café, banco, hospital…
   - `en`: hairdresser / hair salon, pharmacy, supermarket, gas station / petrol, parking, restaurant, cafe, bank, hospital…
2. `searchPlacesDetailed(query, { …, locale })` (o leer locale actual desde el call site con `useTranslation` / `loadStoredLocale`). Default `es`.
3. Match de categoría: normalizar query (minúsculas, sin acentos) contra las claves del léxico **de ese locale**.
4. Si es categoría:  
   `GET …/v1/nearby?lat&lon&radius&tag=shop:hairdresser`  
   centro = viewport (o GPS), radio del viewbox (cap ~1–3 km).
5. En search/reverse: enviar `accept-language=es|en` según el mismo locale (labels OSM en el idioma de la UI).
6. Mapear Nearby → `AddressSuggestion`. Marcas (Burger King) siguen por Autocomplete/Search (sección C).
7. Cache clave incluye locale: `(locale, tag, lat≈, lon≈, radius)`.

### C) Fuzzy / typos — evaluación de lo que propone Gemini

| Idea Gemini | Veredicto | Por qué |
| --- | --- | --- |
| LocationIQ Autocomplete | **Integrar** | En Free, misma key. Sirve para texto incompleto / typeahead. Ya en Task 1 / 3b. |
| LocationIQ `fuzzy=1` | **No** | No aparece en la docs oficiales de Autocomplete/Search. No inventar APIs. |
| Geoapify Autocomplete / Pelias | **No ahora** | Segundo vendor, otra API, 3k req/día (peor cupo que LocationIQ). No mejora portales ni categorías OSM. |
| Mapbox Geocoding (typos nativos, 100k/mes) | **No ahora** | Mejor UX fuzzy, pero tarjeta, API distinta y tira el criterio «drop-in Nominatim». Reabrir solo si QA de typos falla tras Autocomplete+Photon. |
| Fuse.js + JSON local de sitios | **No en esta fase** | ParkXchange busca calles/portales/POIs OSM en zona variable, no un catálogo cerrado de una ciudad. Un JSON «liviano» no cubre portales; uno completo es un extract OSM + pipeline. No sustituye Nearby ni structured. Los aliases de marca en cliente ya cubren el caso Fuse.js útil (marcas conocidas). |

**Qué sí hacemos (híbrido ligero, sin Fuse.js ni segundo proveedor):**

1. Rama **nombre libre**: **Autocomplete** LocationIQ (`viewbox` + `accept-language`) + debounce existente.
2. Si vacío → Search free-form.
3. Si sigue vacío → **Photon** (fuzzy Elasticsearch real), una pasada.
4. Variantes de marca en cliente (`BRAND_QUERY_ALIASES` + `editDistance`) **antes** de la red — eso es el equivalente sensato al «híbrido en cliente» de Gemini, sin dataset geográfico.
5. Escalada documentada: si QA sigue viendo typos graves → valorar Mapbox **o** Geoapify solo para la rama nombre (no para Nearby/portal).

### D) UX para ayudar a distinguir (sin disparar la cuota)

Google no se limita a heurísticas: el usuario **elige** una sugerencia tipada. En ParkXchange añadimos una capa ligera (poco coste de API si hay debounce + caché):

1. **Row de categoría (local, sin API hasta el tap)**  
   Si el texto se acerca a una entrada del léxico del locale (prefijo / match parcial, p. ej. `pelu` → peluquería), mostrar arriba una acción:  
   «Buscar peluquerías cerca» / «Search hairdressers nearby».  
   Al tocarla → rama B (Nearby, 1 req). No llamar Nearby solo por teclear.

2. **Autocomplete de nombres/direcciones debajo**  
   Lista debounced vía LocationIQ Autocomplete (rama C). El usuario elige un hit → ya es un lugar concreto (coords), sin re-clasificar.  
   Misma Autocomplete del plan; no es una API extra.

3. **Heurística de respaldo al confirmar sin elegir**  
   Si pulsa buscar / Enter sin tocar sugerencia → routing de la sección 0 (A/B/C). Igual que una búsqueda “a ciegas”.

4. **Filtro `Todo | Sitios | Calles` (fase 1b, si hace falta)**  
   Segmented control opcional tras usar (1)+(2)+(3). No multiplica reqs: solo fija la rama.  
   **No** en el MVP inicial de este plan; documentado como follow-up si la ambigüedad sigue molestando.

**Cuota:** (1) y (4) ≈ 0 créditos extra; (2) = 1 Autocomplete/debounce; (3) = 1 búsqueda al confirmar. Prohibido: Autocomplete sin debounce o Nearby+Autocomplete+Search en paralelo por tecla.

**i18n:** copy del row de categoría y del filtro (si llega) en es/en.

---

## Global Constraints

- No llamar a `nominatim.openstreetmap.org` en prod si falta la key: fallo controlado.
- Mantener contrato `AddressSuggestion`; la UI de búsqueda (`index.tsx`, announce) **sí** se amplía para sugerencias tipadas / row de categoría.
- Debounce en UI: obligatorio para Autocomplete; no quitarlo.
- Nearby y structured cuentan al cupo Free: priorizar caché y no fan-out innecesario.
- No disparar Nearby hasta tap en row de categoría (o confirmación heurística rama B).

---

### Task 1: LocationIQ como primario

**Files:** `apps/mobile/src/map/geocode.ts`, `apps/mobile/.env.example`, envs

- [ ] Search: `https://eu1.locationiq.com/v1/search`
- [ ] Autocomplete: `https://eu1.locationiq.com/v1/autocomplete` (rama nombre libre)
- [ ] Reverse: `https://eu1.locationiq.com/v1/reverse`
- [ ] Key: `EXPO_PUBLIC_LOCATIONIQ_KEY`
- [ ] Photon solo tras Autocomplete+Search vacíos (fuzzy tipográfico)

### Task 2: Direcciones con número de portal

**Files:** `geocode.ts`, `geocode.test.ts`

- [ ] `classifySearchQuery` / routing: dirección vs categoría vs nombre (tests de tabla)
- [ ] Parser `parseStreetAddressQuery` (calle + número ES/EN)
- [ ] Variantes free-form + structured `street=`
- [ ] Ordenar por `house_number` coincidente; no inventar portal si OSM no lo tiene
- [ ] Tras hits: `sortHitsNearToFar` al centro del viewport (todas las ramas)
- [ ] Tests unitarios con queries tipo `Avenida Las Golondrinas, 18`

### Task 3: Categorías vía Nearby POI (locale-aware)

**Files:** `geocode.ts`, `geocode.test.ts`, call sites (`index.tsx`, announce search si aplica)

- [ ] Léxico `Record<AppLocale, Record<normalizedTerm, osmTag[]>>` (es + en → mismos tags)
- [ ] `SearchPlacesOptions.locale?: AppLocale`; pasar locale desde UI i18n
- [ ] `accept-language` en search/reverse/nearby según locale
- [ ] `fetchNearby(lat, lon, radius, tag)` → `AddressSuggestion[]`
- [ ] Si query es categoría del locale activo → Nearby primero
- [ ] Tests: «peluquería» (es) y «hairdresser» (en) → mismo tag; URL con accept-language

### Task 3b: Fuzzy / nombres parciales

**Files:** `geocode.ts`, `geocode.test.ts`

- [ ] Rama nombre: Autocomplete → Search → Photon
- [ ] Mantener aliases + editDistance de marcas en cliente
- [ ] No usar `fuzzy=1` (no documentado)
- [ ] Tests de construcción de URL autocomplete

### Task 4: Endurecer — caché + menos fan-out + User-Agent

- [ ] Caché TTL 5–15 min (search, autocomplete, reverse, nearby)
- [ ] Parar al ≥N hits; no Photon/expansión si ya hay resultados
- [ ] User-Agent `ParkXchange/<version> (contacto)`

### Task 5: Docs

**Files:** `ARCHITECTURE.md`, `PROGRESS.md`, atribución UI si aplica

- [ ] LocationIQ Free, límites, escalado a Developer (~100 $/mes)
- [ ] Nota: direcciones dependen de OSM; categorías vía Nearby; typos vía Autocomplete+Photon (no Fuse.js/Mapbox en v1)
- [ ] Atribución OSM / «Search by LocationIQ.com» (Free comercial)
- [ ] Escalada documentada: si typos fallan en QA → Mapbox o Geoapify solo rama nombre
- [ ] Nota UX: row categoría + autocomplete tipado; filtro Todo|Sitios|Calles como follow-up

### Task 6: UX distinción en el buscador

**Files:** `apps/mobile/src/app/index.tsx` (y announce search si comparte barra), i18n es/en, posiblemente componente pequeño `SearchSuggestions.tsx`

- [ ] Al tipar (debounce): match parcial contra léxico → row acción «Buscar {categoría} cerca» (sin API hasta tap)
- [ ] Debajo: sugerencias Autocomplete LocationIQ (nombres/direcciones); tap → ir a coords / pins
- [ ] Confirmar sin elegir sugerencia → `classifySearchQuery` + rama A/B/C
- [ ] Placeholder hint: calle / sitio / tipo (i18n)
- [ ] **No** implementar aún el segmented `Todo | Sitios | Calles` (follow-up 1b si hace falta)
- [ ] No Nearby/Autocomplete/Search en paralelo por tecla

## Fuera de alcance

- Endpoint geocode en `services/api`
- Mapbox / Geoapify / Google Places / Pelias / Geocode Earth (segundo motor fuzzy) — salvo escalada post-QA
- Fuse.js + dataset local de POIs/calles
- Self-host Nominatim/Photon (fase posterior documentada arriba)
- Parámetro `fuzzy=1` no documentado
- Autocomplete sin debounce / fan-out por tecla
- Segmented `Todo | Sitios | Calles` en este MVP (queda como 1b)
- Chips de tipo en cada sugerencia estilo Google completo
- Cobertura OSM completa de todos los portales de España
