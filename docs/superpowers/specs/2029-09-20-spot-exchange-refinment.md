# Matriz de Lógica de Intercambio y Notificaciones

Status: **approved-v2** — ventanas A–D × 2 roles; S10 locked (cancel dueño en B = release)  
Supersedes (handshake order): el flujo driver-first del código actual (“I’m here” → “Salir ya”).  
Aligned with: [2026-09-19-offer-based-exchange-design.md](./2026-09-19-offer-based-exchange-design.md) + S1–S9 de esta sesion.  
Push remoto + coaching / acciones / geocerca asistida:
[2026-09-21-exchange-push-coaching-design.md](./2026-09-21-exchange-push-coaching-design.md)
(infra Expo: [2026-09-20-remote-push-implementation-brief.md](./2026-09-20-remote-push-implementation-brief.md)).

### Terminología de producto (locked 2026-09-21)

En copy de usuario **no** usar «dueño» ni «conductor» (jerga interna / riesgo legal sobre suelo público):

| Rol interno | Copy de producto (ES) |
| --- | --- |
| owner | «Dejaste libre el hueco» / «quien deja el hueco» |
| driver | «Reservaste el hueco» / «quien reservó» |

Los campos API (`owner_id`, `driver_*`) no cambian.

---

## Como leer las celdas

Cada celda describe **todo** lo que ocurre en esa combinacion:

| Linea | Significado |
| --- | --- |
| **UI** | Copy / botones / confirmacion (S8b) para el actor |
| **Estado App** | Timestamps, status, WS |
| **Notif** | Aviso al **otro** (contrato in-app/WS; push = brief aparte) |
| **Dinero** | `hold` / `release` / `credit` / `forfeit` / `ninguno` |

Columnas = estado del **otro** participante (Sin salir / De camino / En el sitio=listo).

### Senales (S9)

| Senal | Campo | Nota |
| --- | --- | --- |
| Yendo | `*_en_route_at` | Opcional; no cierra; historico (S4) |
| Listo | `*_ready_at` | Compromiso en el punto; retractable |
| Retractar | clear ready | Para relojes anclados; notif al otro |
| Completar | ambos ready | Auto `credit` + “sal ya” |

### Ventanas temporales (4) — [Supuesto v2]

Antes solo habia 3 ventanas. La de **30 min** (`DriverFairCancelWindow`) afectaba al dinero del **conductor** al cancelar, pero no era una columna de matriz. Ahora es ventana propia.

| # | Ventana | Definicion | Relojes 10 min | Cancel conductor |
| --- | --- | --- | --- | --- |
| A | **Con margen** | `now < exchange_at - 30m` | No | `release` |
| B | **Ultima media hora** | `exchange_at - 30m ≤ now < exchange_at` | No | `forfeit` → credit dueño (S7) |
| C | **Cortesia** | `exchange_at ≤ now` y dentro del deadline 10 min aplicable (si hay listo) | Si hay listo: ancla `max(ready, exchange_at)` | `forfeit` (salvo stall owner-no-show → `release`) |
| D | **Fuera de tiempo** | Pasado deadline 10 min y/o cierre 60 min; reserva aun viva o ya terminal | Soft (S2): si viva, aun se puede completar | Ver celdas |

Constantes: `NoShowGrace = 10m`, `DriverFairCancelWindow = 30m`, `OwnerSafetyNet = 60m`.

**Cancelacion del dueño (dinero):** siempre `release` al conductor + spot fuera del producto — **tambien en la ultima media hora (ventana B)** (S10). Excepcion unica: si el dueño ya estuvo listo y vencio el suelo de no-show del conductor → `forfeit`→credit dueño (`OwnerCancelForfeits`). Eso **no** depende de la ventana de 30 min.

Por tanto: **8 tablas** (4 ventanas × dueño/conductor), no 6: cortesia y fuera de tiempo se mantienen aparte.

### Happy path

1. Ambos pueden marcar Yendo (notif).  
2. Dueño Listo (confirm) → notif.  
3. Conductor Listo detras (confirm) → si dueño ya listo, **completa**.  
4. Orden inverso igual. Retractar evita cierre fantasma.

---

## Dinero (resumen locked)

| Evento | Ledger |
| --- | --- |
| Aceptar oferta | `hold` conductor |
| Completar (ambos listos) | `credit` dueño |
| Cancel dueño (caso tipico) | `release` conductor; spot fuera |
| Cancel dueño tras suelo driver-no-show | `forfeit`→`credit` dueño |
| Cancel conductor con margen (≥30m antes) | `release`; spot vuelve si listing vivo |
| Cancel conductor ultima media hora / post-hora (sin stall) | `forfeit`→`credit` dueño; spot vuelve si listing vivo |
| Cancel conductor con stall owner-no-show a su favor | `release` |
| Driver no-show (auto/manual tras suelo) | `forfeit` dueño; spot se retira |
| Owner no-show | `release` conductor; spot vuelve si listing vivo |
| Safety +60m sin owner_ready | `release` |
| Safety +60m con owner_ready (S1) | `forfeit` dueño |
| Yendo / Listo / Retractar sin cerrar | `ninguno` |

---

# BLOQUE 1 — DUEÑO

## 1.A Con margen (`now < exchange_at - 30m`)

| Accion | Impacto | Conductor: Sin salir | Conductor: De camino | Conductor: Listo |
| --- | --- | --- | --- | --- |
| **Yendo** | **Dueño** | UI: confirm “¿Avisar que vas de camino?” → “De camino”; Listo/Cancel/Retractar si aplica. Estado: `owner_en_route_at`. Notif: —. Dinero: ninguno | UI: “El conductor tambien va”. Estado: en_route. Dinero: ninguno | UI: “Conductor ya listo — llega y marca Listo para cerrar”. Estado: en_route. Dinero: ninguno |
| | **Conductor** | UI: “El dueño va de camino”. Notif: “El dueño va de camino al intercambio”. Dinero: ninguno | Notif cooldown. Dinero: ninguno | UI: “Dueño de camino; tu ya listo — espera o retracta”. Notif: “El dueño va de camino”. Dinero: ninguno |
| **Listo** | **Dueño** | UI: confirm S8b → “Listo — esperando (con margen)”; Retractar. Estado: `owner_ready_at`; **no** reloj 10m. Dinero: ninguno | Igual + “conductor de camino”. Dinero: ninguno | UI: **completo** “Sal ya”. Estado: `completed` + `spot.removed`. Dinero: `credit` |
| | **Conductor** | UI: “Dueño listo en el coche (aun hay margen)”. Notif: “El dueño esta listo para salir; no hace falta precipitarse”. Dinero: ninguno | Notif: “Dueño listo — cuando estes detras, Listo cierra”. Dinero: ninguno | UI: “Completado — aparca”. Notif “Sal ya” al dueño. Dinero: hold→credit |
| **Cancelar** | **Dueño** | UI: confirm “Cancelar libera el deposito al conductor y retira el anuncio”. Estado: cancelled; spot fuera. Dinero: **`release`** | Igual / `release` | Igual / `release` (aunque conductor listo) |
| | **Conductor** | UI+Notif: “Dueño cancelo — deposito liberado”. Dinero: recibe `release` | Igual | Igual |
| **Retractar** | **Dueño** | Si tenia ready: confirm → clear `owner_ready_at`. Dinero: ninguno | Igual | Igual; no cierra |
| | **Conductor** | Notif: “El dueño retiro su listo”. Dinero: ninguno | Igual | Igual |

## 1.B Ultima media hora (`exchange_at - 30m ≤ now < exchange_at`)

Misma maquina de estados que 1.A (aun **sin** relojes 10 min). Cambia **urgencia de copy** y el hecho de que si **el conductor** cancelara aqui forfeitaria (el dueño, al cancelar, sigue liberando).

| Accion | Impacto | Conductor: Sin salir | Conductor: De camino | Conductor: Listo |
| --- | --- | --- | --- | --- |
| **Yendo** | **Dueño** | UI: “De camino — faltan &lt;30 min”. Estado: en_route. Dinero: ninguno | Igual | UI: “Conductor listo — date prisa a marcar Listo” |
| | **Conductor** | Notif: “Dueño de camino (faltan &lt;30 min)”. Dinero: ninguno | Cooldown | Notif: “Dueño en camino; sigue listo o retracta si te vas” |
| **Listo** | **Dueño** | UI: confirm → “Listo — el conductor tiene poco margen”; Retractar; sin reloj 10m hasta `exchange_at`. Dinero: ninguno | Igual | **Completo** “Sal ya”; `credit` |
| | **Conductor** | UI+Notif: “Dueño listo — date prisa (aun no es la hora; al marcar Listo puedes cerrar)”. Dinero: ninguno | Urgente | Completado / “Sal ya” |
| **Cancelar** | **Dueño** | UI: confirm aclara “se libera el deposito al conductor” (no cobras por cancelar tu). Dinero: **`release`**; spot fuera | `release` | `release` |
| | **Conductor** | Notif: “Cancelada — deposito liberado”. Dinero: `release` | Igual | Igual |
| **Retractar** | Ambos | Igual que 1.A (clear + notif; sin dinero) | | |

## 1.C Cortesia (`exchange_at ≤ now`, dentro de ventana 10 min si aplica)

| Accion | Impacto | Conductor: Sin salir | Conductor: De camino | Conductor: Listo |
| --- | --- | --- | --- | --- |
| **Yendo** | **Dueño** | UI: “De camino (hora pasada)”. Estado: en_route. Dinero: ninguno | Igual | UI: “Conductor listo — marca Listo”; reloj owner-no-show puede estar activo |
| | **Conductor** | Notif: “Dueño de camino (hora pasada)”. Dinero: ninguno | Cooldown | Notif: “Dueño en camino; tu reloj sigue (o retracta)” |
| **Listo** | **Dueño** | UI: confirm → “Listo”; muestra deadline driver-no-show `max(ready,exchange)+10m`; Retractar. Estado: ready + **arranca** reloj. Dinero: ninguno | Igual | **Completo**; `credit` |
| | **Conductor** | UI+Notif **URGENTE**: “Dueño listo — marca Listo antes de {deadline}”. Dinero: ninguno | Urgente | Completado / Sal ya |
| **Cancelar** | **Dueño** | Dinero: tipico `release`. Si ya vencio suelo driver-no-show tras *tu* listo → `forfeit`→credit tuyo | Misma culpa | Conductor listo esperando → siempre `release` (culpa dueño) |
| | **Conductor** | Notif segun rama (liberado / forfeit no-show) | Igual | “Dueño cancelo tras tu espera — liberado” |
| **Retractar** | Ambos | Clear ready; **para** reloj driver-no-show; notif. Dinero: ninguno | | |

## 1.D Fuera de tiempo (deadline 10m pasado y/o cerca de +60m; viva o terminal)

| Accion | Impacto | Conductor: Sin salir | Conductor: De camino | Conductor: Listo |
| --- | --- | --- | --- | --- |
| **Yendo** | **Dueño** | Si viva: “muy tarde — posible cierre a 60m”. Si terminal: error | Igual | Si viva: “Conductor lleva rato — Listo ya” |
| | **Conductor** | Notif solo si viva | Igual | “Dueño tarde” o “Ya cancelada / release|forfeit” |
| **Listo** | **Dueño** | Si viva (S2): aceptar listo aunque pasaran 10m. Si ya forfeit auto: error | Igual | Si viva: **completar**; si owner-no-show ya cerro: error |
| | **Conductor** | Notif listo tarde o “cancelada no-show” | Igual | Completado o “dueño no-show — liberado” |
| **Cancelar** | **Dueño** | Reglas 1.C + cierre +60m: sin owner_ready→`release`; con owner_ready→forfeit (S1, sweeper) | Igual | `release` |
| | **Conductor** | Notif segun dinero | Igual | Liberado |
| **Retractar** | Ambos | Si viva: igual 1.C. Si terminal: error | | |

---

# BLOQUE 2 — CONDUCTOR

## 2.A Con margen (`now < exchange_at - 30m`)

| Accion | Impacto | Dueño: Sin salir | Dueño: De camino | Dueño: Listo |
| --- | --- | --- | --- | --- |
| **Yendo** | **Conductor** | UI: confirm → “De camino”. Estado: `driver_en_route_at`. Dinero: ninguno | “Dueño tambien”. Dinero: ninguno | “Dueño ya listo — al estar detras marca Listo” |
| | **Dueño** | Notif: “El conductor va de camino”. Dinero: ninguno | Cooldown | Notif: “Conductor de camino — espera su Listo” |
| **Listo** | **Conductor** | UI: confirm S8b → “Listo en el punto — esperando”; Retractar; sin reloj 10m. Estado: `driver_ready_at`. Dinero: ninguno | Igual | **Completo**; credit al dueño |
| | **Dueño** | UI+Notif: “Conductor listo en el punto — ven y marca Listo”. Dinero: ninguno | “Date prisa”. Dinero: ninguno | “Sal ya”; `credit` |
| **Cancelar** | **Conductor** | UI: confirm “Con este margen se te **libera** el deposito”. Estado: cancelled; spot vuelve si listing vivo. Dinero: **`release`** | `release` | `release` (aunque dueño listo) |
| | **Dueño** | Notif: “Conductor cancelo — deposito liberado”. Dinero: ninguno (release al conductor) | Igual | Igual |
| **Retractar** | Ambos | Clear `driver_ready_at` + notif “ya no esta listo”. Dinero: ninguno | | |

## 2.B Ultima media hora (`exchange_at - 30m ≤ now < exchange_at`)

| Accion | Impacto | Dueño: Sin salir | Dueño: De camino | Dueño: Listo |
| --- | --- | --- | --- | --- |
| **Yendo** | **Conductor** | UI: “De camino — faltan &lt;30 min”. Dinero: ninguno | Igual | “Dueño listo — date prisa” |
| | **Dueño** | Notif: “Conductor de camino (&lt;30 min)”. Dinero: ninguno | Cooldown | Refuerzo |
| **Listo** | **Conductor** | UI: confirm → listo; sin reloj 10m aun. Retractar. Dinero: ninguno | Igual | **Completo** |
| | **Dueño** | Notif: “Conductor listo (poco margen) — marca Listo”. Dinero: ninguno | Urgente | Sal ya / `credit` |
| **Cancelar** | **Conductor** | UI: confirm **fuerte**: “Si cancelas ahora **pierdes el deposito** (forfeit al dueño)”. Dinero: **`forfeit`→`credit` dueño**; spot vuelve si listing vivo | `forfeit` | `forfeit` (S7 estricto) |
| | **Dueño** | Notif: “Conductor cancelo tarde — cobras el deposito”. Dinero: `credit` | Igual | Igual |
| **Retractar** | Ambos | Igual 2.A | | |

## 2.C Cortesia (post-hora, dentro de 10 min si aplica)

| Accion | Impacto | Dueño: Sin salir | Dueño: De camino | Dueño: Listo |
| --- | --- | --- | --- | --- |
| **Yendo** | **Conductor** | UI: “De camino (hora pasada)”. Dinero: ninguno | Igual | “Dueño listo — Listo cierra” |
| | **Dueño** | Notif tarde. Dinero: ninguno | Igual | “Tu deadline sigue” |
| **Listo** | **Conductor** | UI: confirm → listo; deadline owner-no-show; Retractar. Estado: ready + **arranca** reloj. Dinero: ninguno | Igual | **Completo** |
| | **Dueño** | UI+Notif **URGENTE**: “Conductor listo — marca Listo antes de {deadline}”. Dinero: ninguno | Urgente | Sal ya |
| **Cancelar** | **Conductor** | Default `forfeit`. Si owner-no-show a tu favor ya vencido → `release` (stall) | Igual | Dueño listo → `forfeit` |
| | **Dueño** | `credit` o `release` si stall tuyo | Igual | Forfeit a tu favor |
| **Retractar** | Ambos | Clear ready; para reloj owner-no-show; notif. Dinero: ninguno | | |

## 2.D Fuera de tiempo

| Accion | Impacto | Dueño: Sin salir | Dueño: De camino | Dueño: Listo |
| --- | --- | --- | --- | --- |
| **Yendo** | **Conductor** | Si viva: “muy tarde”; si terminal: error | Igual | Yendo no salva forfeit en curso |
| | **Dueño** | Update o “ya cancelada” | Igual | Tarde / no-show cobrado |
| **Listo** | **Conductor** | Si viva (S2): ready+reloj; si ya release/safety sin owner_ready: error | Igual | Completar si viva; si ya forfeit por tu no-show: error |
| | **Dueño** | Urgente o “tu no-show — liberado” | Igual | Sal ya o no-show conductor |
| **Cancelar** | **Conductor** | `forfeit` salvo (a) owner-no-show a favor (b) safety sin owner_ready (c) FairCancel stall | Igual | `forfeit` |
| | **Dueño** | `credit` o `release` | Igual | Forfeit |
| **Retractar** | Ambos | Si viva: como 2.C; si terminal: error | | |

---

## Eventos de sistema

| Evento | Condicion | Efectos |
| --- | --- | --- |
| Driver no-show | Owner ready y `now ≥ max(owner_ready, exchange)+10m` sin cierre | Auto si `auto_cancel_no_show`: cancel+forfeit. Si no: dueño puede cancelar tras suelo con misma economia. Retractar listo dueño cancela el reloj. Spot se retira. Notif ambos. |
| Owner no-show | Driver ready, owner no, post-hora, `now ≥ max(driver_ready, exchange)+10m` | Cancel+`release`; spot vuelve si listing vivo. Retractar conductor cancela reloj. Notif ambos. |
| Safety / +60m | Sin completar | Sin `owner_ready`: `release`. Con `owner_ready`: forfeit dueño (S1). Notif ambos. |

---

## Decisiones locked (S1–S9) — sin cambio de producto salvo ventanas A/B

S1–S9 siguen. **S10 (locked):** la ultima media hora es ventana de matriz propia. Cambia copy + dinero de **cancel conductor** (`forfeit`). **Cancel dueño en A y B:** siempre `release` al conductor (MVP sencillo; el conductor recupera el deposito aunque ya hubiera salido de casa). Excepcion forfeit dueño solo por rama no-show conductor tras listo del dueño.
