# Matriz de Lógica de Intercambio y Notificaciones

Status: **approved** — listo para plan de implementacion  
Supersedes (handshake order): el flujo driver-first del código actual (“I’m here” → “Salir ya”).  
Aligned with: [2026-09-19-offer-based-exchange-design.md](./2026-09-19-offer-based-exchange-design.md) (owner-first) + S9 de esta sesion.  
Push remoto (fase aparte): [2026-09-20-remote-push-implementation-brief.md](./2026-09-20-remote-push-implementation-brief.md).

### Verificacion pre-plan (2026-09-20)

Producto S1–S9 cerrado y matriz coherente. Supuestos tecnicos locked al implementar (no reabrir producto):

1. Status live = `confirmed` hasta `completed`/`cancelled`; se deja de usar `arrived` en el handshake nuevo.
2. Se eliminan endpoints legacy: `driver-arrived`, `clear-driver-arrived`, `driver-ready` (senal distinta), `driver-confirm-entered`, `driver-report-owner-no-show`.
3. API nueva: `POST .../en-route`, `POST .../ready`, `DELETE .../ready` (o `POST .../unready`), `POST .../cancel` (rol por claims).
4. Columna `driver_arrived_at` se migra fuera; unico listo conductor = `driver_ready_at`.
5. Push remoto **fuera** de este plan (S8 + brief aparte). Handshake shippea WS + confirmaciones in-app.
6. Offers/accept **no** se rehacen; solo post-accept handshake.
---

## Flujo tipico (happy path)

1. Conductor marca **Yendo** → notif al dueño.
2. Dueño marca **Yendo** → notif al conductor.
3. Dueño llega, confirma warning, marca **Listo en el punto** (en el coche, listo para salir) → notif al conductor.
4. Conductor se pone detras, confirma warning, marca **Listo en el punto** (listo para meterse) → como el dueño ya estaba listo, la app **completa sola**: spot fuera del mapa, `credit` al dueño, notif “sal ya”.
5. En la calle: el dueño sale y el conductor entra.

El intercambio **no** se cierra con el listo de una sola parte. Se cierra solo con **ambos listos** (`owner_ready_at` y `driver_ready_at`). Orden indiferente. Cualquiera puede **retractar** su listo mientras la reserva no sea terminal (calle estrecha, autobus, vuelta a la manzana).

---

## Leyenda de vocabulario (cada celda)

Cada celda usa estas cuatro lineas:

- **UI** — textos y botones (incl. confirmacion S8b antes de enviar).
- **Estado App** — timestamps / status; WS (`reservation.updated`, `spot.removed`, `spot.added`).
- **Notif** — aviso al otro (contrato; WS/in-app en handshake; push = brief aparte).
- **Dinero** — `hold` / `release` / `credit` / `forfeit` / `ninguno`.

### Senales de handshake (dominio)

| Senal | Campo | Significado |
| --- | --- | --- |
| Yendo | `owner_en_route_at` / `driver_en_route_at` | Voy de camino (opcional; no cierra). |
| Listo en el punto | `owner_ready_at` / `driver_ready_at` | Estoy en posicion para el intercambio. Dueño = en el coche listo para salir. Conductor = detras, listo para meterse. |
| Retractar listo | clear del `*_ready_at` propio | Ya no estoy en posicion; notif al otro; para relojes anclados en esa senal. |
| Completar | ambos `*_ready_at` set | Auto: `completed` + `credit` dueño + “sal ya”. |

No existe `driver_arrived_at` distinto de listo: **una sola senal de listo por rol** (S9).

### Estados de columna (el otro participante)

| Columna | Significado | Ancla |
| --- | --- | --- |
| Sin salir | Acepto la reserva; sin Yendo ni Listo | relojes de progreso nulos / sin ready |
| De camino | Marco **Yendo** (puede o no tener Listo; si tiene Listo la columna relevante es En el sitio) | `*_en_route_at` |
| En el sitio | Marco **Listo en el punto** (`*_ready_at`). Si ambos listos → ya `completed` | S9 |

### Ventanas temporales

| Ventana | Definicion |
| --- | --- |
| Antes de la hora | `now < exchange_at`. Señales permitidas. **Sin** relojes de no-show de 10 min. |
| Tiempo de cortesia | `exchange_at ≤ now` y aun no pasado el deadline de 10 min aplicable. Relojes de 10 min **solo** si al menos una parte marco Listo (ancla `max(ready_at, exchange_at)`). Pasar la hora sin senales **no** cancela solo. |
| Fuera de tiempo | Pasado el deadline de cortesia aplicable, o safety net / cierre a `exchange_at + 60m`. Si sweeper ya cerro → UI “ya resuelta”. **Deadline blando (S2):** pasar 10 min sin cancel/sweeper **no** impide completar si la reserva sigue viva. |

Constantes: `NoShowGrace = 10m`, `DriverFairCancelWindow = 30m`, `OwnerSafetyNet = 60m`.

### Dinero (resumen)

| Evento | Ledger |
| --- | --- |
| Aceptar oferta (fuera de esta matriz) | `hold` al conductor |
| Completar (ambos listos) | `credit` al dueño (hold consumido) |
| Cancelacion dueño | `release` al conductor; spot fuera del producto |
| Cancelacion conductor ≥30 min antes de `exchange_at` | `release`; spot vuelve a mapa si listing vivo |
| Cancelacion conductor &lt;30 min antes (forfeit estricto) | `forfeit` → `credit` dueño; spot vuelve si listing vivo |
| No-show conductor tras dueño listo + 10 min | forfeit al dueño (auto si `auto_cancel_no_show`; si no, cancel manual tras suelo) |
| No-show dueño tras conductor listo post-hora + 10 min | `release` al conductor; spot vuelve si listing vivo |
| Safety net 60 min | `release` al conductor **solo sin** `owner_ready_at`. **Con** `owner_ready_at` → forfeit dueño (S1) |
| Solo Yendo / Listo / Retractar sin completar | `ninguno` |

### Convenciones de relleno

- **Idempotencia:** re-marcar Yendo/Listo (ya set) no cambia dinero ni spamea notif (cooldown).
- **Confirmacion (S8b):** todo cambio (Yendo, Listo, Retractar, Cancelar) → dialogo con copy del destino. Sin confirmar → no hay request.
  - Listo conductor: “¿Seguro que estas detras del coche del anuncio, listo para meterte en cuanto salga?”
  - Listo dueño: “¿Seguro que estas dentro de tu coche, listo para salir en cuanto llegue el conductor?”
  - Retractar: explicar que dejas de figurar listo, avisas al otro y puedes parar relojes de espera.
- Soft geofence: aviso cliente si GPS lejos al marcar Listo; servidor no rechaza por distancia.
- Spot en reserva aceptada: fuera del mapa (`reserved`); vuelve solo con cancelaciones que liberen el listing.

### Problema de calle (S9)

Conductor listo → obligado a irse → **Retractar listo** → dueño deja de verlo esperando; no hay cierre fantasma. Cierre solo con ambos listos a la vez.

---

# BLOQUE 1: ACCIONES DEL DUEÑO DEL SPOT

## 1.1. Ventana Temporal: Antes de la hora

| Acción del Dueño | Reacción / Impacto | Estado Conductor: Sin salir | Estado Conductor: De camino | Estado Conductor: En el sitio (listo) |
| :----------------------------------- | :------------------------- | :--------------------------------- | :--------------------------------- | :--------------------------------- |
| **Acción 1: Yendo** | **UI / Sistema Dueño** | • UI: confirm → “De camino”; sigue Listo + Cancelar + (si listo) Retractar<br>• Estado App: `owner_en_route_at=now`; WS `reservation.updated`<br>• Notif: —<br>• Dinero: ninguno | • UI: igual + “El conductor tambien va de camino”<br>• Estado App: `owner_en_route_at=now`<br>• Notif: —<br>• Dinero: ninguno | • UI: “Conductor ya listo en el punto — llega y marca Listo para cerrar”<br>• Estado App: `owner_en_route_at=now`<br>• Notif: —<br>• Dinero: ninguno |
| | **UI / Sistema Conductor** | • UI: “El dueño va de camino”<br>• Estado App: ve en_route<br>• Notif: “El dueño va de camino al intercambio”<br>• Dinero: ninguno | • UI: refuerzo<br>• Estado App: igual<br>• Notif: cooldown<br>• Dinero: ninguno | • UI: “Dueño de camino; tu ya estas listo — espera su Listo (o retracta si te vas)”<br>• Estado App: igual<br>• Notif: “El dueño va de camino; sigue en el punto si puedes”<br>• Dinero: ninguno |
| **Acción 2: Listo en el punto** | **UI / Sistema Dueño** | • UI: confirm (copy S8b) → “Listo — esperando al conductor (antes de la hora)”; **no** completa; muestra Retractar<br>• Estado App: `owner_ready_at=now`; **no** reloj 10 min hasta `exchange_at`<br>• Notif: —<br>• Dinero: ninguno | • UI: “Listo — el conductor va de camino”; Retractar disponible<br>• Estado App: `owner_ready_at=now`<br>• Notif: —<br>• Dinero: ninguno | • UI: **ambos listos → completado** “Sal ya”<br>• Estado App: `completed`; spot `completed` / `spot.removed`<br>• Notif: — (propia)<br>• Dinero: `credit` dueño |
| | **UI / Sistema Conductor** | • UI: “Dueño listo en el coche (aun no es la hora)”; tu Listo sigue disponible<br>• Estado App: ve `owner_ready`<br>• Notif: “El dueño esta listo para salir; no hace falta precipitarse antes de la hora”<br>• Dinero: ninguno | • UI: “Dueño listo — cuando estes detras, marca Listo (cerrara)”<br>• Estado App: igual<br>• Notif: “El dueño esta en el coche listo para salir”<br>• Dinero: ninguno | • UI: “Completado — el dueño sale; aparca”<br>• Estado App: `completed`<br>• Notif: “Sal ya” al dueño; conductor ve exito<br>• Dinero: hold → `credit` dueño |
| **Acción 3: Cancelar** | **UI / Sistema Dueño** | • UI: confirm → cancelada<br>• Estado App: `cancelled`; spot fuera; `spot.removed`<br>• Notif: —<br>• Dinero: `release` | • UI: igual<br>• Estado App: igual<br>• Notif: —<br>• Dinero: `release` | • UI: igual<br>• Estado App: igual<br>• Notif: —<br>• Dinero: `release` |
| | **UI / Sistema Conductor** | • UI: “Dueño cancelo; deposito liberado”<br>• Estado App: terminal<br>• Notif: “Reserva cancelada por el dueño — deposito liberado”<br>• Dinero: `release` | • UI: igual<br>• Estado App: igual<br>• Notif: igual<br>• Dinero: `release` | • UI: igual (aunque estuviera listo)<br>• Estado App: igual<br>• Notif: igual<br>• Dinero: `release` |
| **Acción 4: Retractar listo** | **UI / Sistema Dueño** | • UI: N/A (no estaba listo) o no-op<br>• Estado App: —<br>• Notif: —<br>• Dinero: ninguno | • UI: N/A / no-op<br>• Estado App: —<br>• Notif: —<br>• Dinero: ninguno | • UI: N/A para esta columna del *conductor*; si el **dueño** estaba listo y retracta: confirm → deja de estar listo<br>• Estado App: `owner_ready_at=null`; WS update; **no** completa<br>• Notif: —<br>• Dinero: ninguno |
| | **UI / Sistema Conductor** | • (si dueño retracta estando el conductor sin listo / de camino / listo)<br>• UI: “El dueño ya no esta listo en el punto”<br>• Estado App: sin `owner_ready`<br>• Notif: “El dueño retiro su listo — ya no espera en el coche”<br>• Dinero: ninguno | • igual | • igual; si el conductor sigue listo, espera de nuevo; **no** hay cierre |

*Nota Acción 4:* retractar solo aplica si el actor tenia `*_ready_at`. En 1.1 la columna “En el sitio” es el **conductor** listo; el dueño retracta desde su propio estado listo (cualquier columna del conductor). Efectos al conductor: fila inferior.

---

## 1.2. Ventana Temporal: Tiempo de cortesía

| Acción del Dueño | Reacción / Impacto | Estado Conductor: Sin salir | Estado Conductor: De camino | Estado Conductor: En el sitio (listo) |
| :----------------------------------- | :------------------------- | :--------------------------------- | :--------------------------------- | :--------------------------------- |
| **Acción 1: Yendo** | **UI / Sistema Dueño** | • UI: “De camino (hora pasada) — date prisa”<br>• Estado App: `owner_en_route_at=now`<br>• Notif: —<br>• Dinero: ninguno | • UI: igual + conductor en ruta<br>• Estado App: igual<br>• Notif: —<br>• Dinero: ninguno | • UI: “Conductor listo en el punto — marca Listo ya”<br>• Estado App: en_route; reloj owner-no-show puede estar activo por `driver_ready_at`<br>• Notif: —<br>• Dinero: ninguno |
| | **UI / Sistema Conductor** | • UI: “Dueño de camino (tarde)”<br>• Notif: “El dueño va de camino (hora acordada pasada)”<br>• Dinero: ninguno | • UI: refuerzo; Notif cooldown<br>• Dinero: ninguno | • UI: “Dueño en camino; tu reloj de espera sigue (o retracta si te vas)”<br>• Notif: “El dueño va de camino; sigue en el punto si puedes”<br>• Dinero: ninguno |
| **Acción 2: Listo en el punto** | **UI / Sistema Dueño** | • UI: confirm → “Listo — esperando”; deadline driver-no-show `max(owner_ready, exchange_at)+10m`; Retractar; cancel manual tras suelo si no auto<br>• Estado App: `owner_ready_at=now`; **arranca** reloj driver-no-show<br>• Notif: —<br>• Dinero: ninguno | • UI: igual + “conductor de camino”<br>• Estado App: ready + reloj<br>• Notif: —<br>• Dinero: ninguno | • UI: **completar** “Sal ya”<br>• Estado App: `completed`; spot fuera<br>• Notif: —<br>• Dinero: `credit` |
| | **UI / Sistema Conductor** | • UI: “URGENTE: dueño listo — marca Listo antes de {deadline}” (o retracta si no puedes)<br>• Notif: “El dueño esta listo en el coche — tienes ~10 min”<br>• Dinero: ninguno | • UI: urgente; Notif: “Dueño listo — llega y marca Listo”<br>• Dinero: ninguno | • UI: “Completado — aparca”<br>• Notif: “Sal ya” al dueño<br>• Dinero: hold → pago |
| **Acción 3: Cancelar** | **UI / Sistema Dueño** | • UI: cancelada; spot fuera<br>• Dinero: `release` (salvo rama forfeit tras suelo driver-no-show si conductor culpable — ver 1.3) | • igual / `release` tipico | • cancelar con conductor listo = culpa dueño → `release` |
| | **UI / Sistema Conductor** | • Notif: “Cancelada por el dueño — deposito liberado” (o forfeit si aplica)<br>• Dinero: `release` o forfeit | • igual | • “Dueño cancelo tras tu espera — deposito liberado”; `release` |
| **Acción 4: Retractar listo** | **UI / Sistema Dueño** | • Si tenia ready: confirm → clear `owner_ready_at`; **para** reloj driver-no-show<br>• Dinero: ninguno | • igual | • igual; si conductor sigue listo, **no** completa |
| | **UI / Sistema Conductor** | • Notif: “El dueño retiro su listo”<br>• UI: ya no hay deadline de no-show por su ready<br>• Dinero: ninguno | • igual | • igual |

---

## 1.3. Ventana Temporal: Fuera de tiempo

Interpretacion: pasado deadline 10 min **o** cerca/pasado cierre 60 min, reserva **aun viva** (si sweeper cerro → rechazo “ya resuelta”). Completar sigue permitido si viva (S2).

| Acción del Dueño | Reacción / Impacto | Estado Conductor: Sin salir | Estado Conductor: De camino | Estado Conductor: En el sitio (listo) |
| :----------------------------------- | :------------------------- | :--------------------------------- | :--------------------------------- | :--------------------------------- |
| **Acción 1: Yendo** | **UI / Sistema Dueño** | • UI: si viva, “muy tarde — posible cierre a 60 min”<br>• Estado App: en_route o error terminal<br>• Dinero: ninguno | • igual | • si viva y conductor listo: “Conductor lleva rato — marca Listo” |
| | **UI / Sistema Conductor** | • Notif solo si viva<br>• Dinero: ninguno | • igual | • “Dueño en camino (muy tarde)” o “Ya cancelada” si owner-no-show/`release` |
| **Acción 2: Listo en el punto** | **UI / Sistema Dueño** | • UI: si viva: aceptar listo (S2) aunque pasaran 10 min; si ya forfeit auto → error<br>• Estado App: `owner_ready_at` + reloj, o terminal<br>• Dinero: ninguno | • igual | • si viva: **completar**; si ya cancelada owner-no-show → error<br>• Dinero: `credit` si completa |
| | **UI / Sistema Conductor** | • Notif: “Dueño listo (tarde)” o “Cancelada — no-show” (`forfeit`)<br>• Dinero: ninguno o forfeit ya | • igual | • “Completado” o “Dueño no show — deposito liberado” (`release`) |
| **Acción 3: Cancelar** | **UI / Sistema Dueño** | • Dinero: antes de suelo driver-no-show tras *su* listo → `release`; tras suelo (conductor culpable) → `forfeit`→`credit`; nadie listo solo retraso → `release`. Cierre +60m: sin owner_ready → `release`; con owner_ready → forfeit (S1) | • misma culpa | • conductor listo esperando → `release` |
| | **UI / Sistema Conductor** | • Notif segun rama (`release` / forfeit)<br>• Dinero: acorde | • igual | • “Dueño cancelo tras tu espera — liberado”; `release` |
| **Acción 4: Retractar listo** | **Ambos** | Misma semantica que 1.2: clear ready, para reloj, notif “retiro listo”. Si ya terminal → error. | | |

---

# BLOQUE 2: ACCIONES DEL CONDUCTOR

## 2.1. Ventana Temporal: Antes de la hora

| Acción del Conductor | Reacción / Impacto | Estado Dueño: Sin salir | Estado Dueño: De camino | Estado Dueño: En el sitio (listo) |
| :------------------------------------------ | :------------------------- | :--------------------------------- | :--------------------------------- | :--------------------------------- |
| **Acción 1: Yendo** | **UI / Sistema Conductor** | • UI: confirm → “De camino”; Listo + Cancelar<br>• Estado App: `driver_en_route_at=now`<br>• Notif: —<br>• Dinero: ninguno | • UI: “De camino — el dueño tambien”<br>• Estado App: igual<br>• Dinero: ninguno | • UI: “Dueño ya listo — al estar detras marca Listo para cerrar”<br>• Estado App: igual<br>• Dinero: ninguno |
| | **UI / Sistema Dueño** | • Notif: “El conductor va de camino al intercambio”<br>• Dinero: ninguno | • Notif cooldown<br>• Dinero: ninguno | • UI: “Conductor de camino — tu ya listo; espera su Listo”<br>• Notif: “El conductor va de camino”<br>• Dinero: ninguno |
| **Acción 2: Listo en el punto** | **UI / Sistema Conductor** | • UI: confirm (detras del coche) → “Listo en el punto — esperando al dueño”; **no** completa; Retractar visible<br>• Estado App: `driver_ready_at=now`; **no** reloj 10 min hasta `exchange_at`<br>• Notif: —<br>• Dinero: ninguno | • UI: “Dueño de camino; espera” (o retracta si te vas)<br>• Estado App: `driver_ready_at=now`<br>• Dinero: ninguno | • UI: **ambos listos → completado**<br>• Estado App: `completed`; spot removed<br>• Notif: —<br>• Dinero: ninguno directo (`credit` al dueño) |
| | **UI / Sistema Dueño** | • UI: “Conductor listo en el punto — ven y marca Listo”<br>• Notif: “El conductor esta listo en el punto de intercambio”<br>• Dinero: ninguno | • UI: “Conductor listo — termina de llegar y marca Listo”<br>• Notif: “El conductor esta en el punto; date prisa”<br>• Dinero: ninguno | • UI: “Sal ya — el conductor esta listo detras”<br>• Estado App: `completed`<br>• Notif: “Sal ya — intercambio cerrado”<br>• Dinero: `credit` |
| **Acción 3: Cancelar** | **UI / Sistema Conductor** | • Dinero: ≥30m → `release`; &lt;30m → `forfeit` (S7)<br>• Spot: `available` si listing vivo | • misma regla 30m | • misma regla 30m (forfeit estricto aunque dueño listo) |
| | **UI / Sistema Dueño** | • Notif: liberado o “cobras el deposito”<br>• Dinero: `release` o `credit` | • igual | • “Conductor cancelo pese a que estabas listo” + rama 30m |
| **Acción 4: Retractar listo** | **UI / Sistema Conductor** | • Si tenia ready: confirm → `driver_ready_at=null`<br>• Dinero: ninguno | • igual | • igual; **no** hay completed si retracta antes del cierre |
| | **UI / Sistema Dueño** | • Notif: “El conductor ya no esta listo en el punto” (pudo irse a dar la vuelta)<br>• Dinero: ninguno | • igual | • igual; dueño sigue listo esperando de nuevo |

---

## 2.2. Ventana Temporal: Tiempo de cortesía

| Acción del Conductor | Reacción / Impacto | Estado Dueño: Sin salir | Estado Dueño: De camino | Estado Dueño: En el sitio (listo) |
| :------------------------------------------ | :------------------------- | :--------------------------------- | :--------------------------------- | :--------------------------------- |
| **Acción 1: Yendo** | **UI / Sistema Conductor** | • UI: “De camino (hora pasada)”<br>• Estado App: `driver_en_route_at`<br>• Dinero: ninguno | • igual | • UI: “Dueño listo — date prisa; Listo cierra” |
| | **UI / Sistema Dueño** | • Notif: “Conductor de camino (hora pasada)” | • igual | • “Conductor en camino; tu deadline sigue” |
| **Acción 2: Listo en el punto** | **UI / Sistema Conductor** | • UI: confirm → listo; deadline owner-no-show `max(driver_ready, exchange_at)+10m`; Retractar<br>• Estado App: `driver_ready_at=now`; **arranca** reloj owner-no-show<br>• Dinero: ninguno | • igual + reloj | • **completar**<br>• Dinero: `credit` dueño |
| | **UI / Sistema Dueño** | • UI: “URGENTE: conductor listo — marca Listo antes de {deadline}”<br>• Notif: “El conductor esta en el punto — tienes ~10 min” | • urgente; Notif: “Conductor listo; marca Listo” | • “Sal ya”; Notif cierre; `credit` |
| **Acción 3: Cancelar** | **UI / Sistema Conductor** | • Post-hora ⇒ fuera FairCancel 30m → `forfeit` (salvo stall: owner-no-show a tu favor ya vencido → `release`) | • `forfeit` (misma excepcion stall) | • dueño listo y cancelas → `forfeit` |
| | **UI / Sistema Dueño** | • `credit` o `release` si stall tuyo | • igual | • “Forfeit a tu favor”; `credit` |
| **Acción 4: Retractar listo** | **Ambos** | Clear `driver_ready_at`; **para** reloj owner-no-show; notif al dueño “ya no esta listo”. Dinero: ninguno. | | |

---

## 2.3. Ventana Temporal: Fuera de tiempo

| Acción del Conductor | Reacción / Impacto | Estado Dueño: Sin salir | Estado Dueño: De camino | Estado Dueño: En el sitio (listo) |
| :------------------------------------------ | :------------------------- | :--------------------------------- | :--------------------------------- | :--------------------------------- |
| **Acción 1: Yendo** | **UI / Sistema Conductor** | • si viva: “muy tarde”; si terminal → error | • igual | • marcar Yendo no salva un forfeit ya en curso |
| | **UI / Sistema Dueño** | • update o “ya cancelada” | • igual | • “Conductor tarde” o “No-show — cobraste” |
| **Acción 2: Listo en el punto** | **UI / Sistema Conductor** | • si viva: set ready + reloj (S2); si ya `release` owner-no-show o safety sin owner_ready → error | • igual | • si viva: **completar**; si ya forfeit por tu no-show → error |
| | **UI / Sistema Dueño** | • urgente o “tu no-show — liberado al conductor” | • igual | • “Sal ya” o “No-show conductor” (`credit` forfeit) |
| **Acción 3: Cancelar** | **UI / Sistema Conductor** | • default `forfeit`; `release` si (a) owner-no-show a tu favor, (b) safety sin owner_ready, (c) FairCancel stall | • igual | • dueño listo → `forfeit` |
| | **UI / Sistema Dueño** | • `credit` o `release` | • igual | • forfeit a tu favor |
| **Acción 4: Retractar listo** | **Ambos** | Igual que 2.2 si viva; error si terminal. | | |

---

## Eventos de sistema (fuera de botones)

| Evento | Condicion | Efectos |
| --- | --- | --- |
| Driver no-show | Dueño listo y `now ≥ max(owner_ready_at, exchange_at) + 10m` sin cierre | Si `auto_cancel_no_show`: cancel + `forfeit` dueño. Si no: dueño puede cancelar manual tras suelo con misma economia. Notif a ambos. Spot: listing se retira. Dueño **retractar** listo antes cancela el reloj. |
| Owner no-show | Conductor listo (`driver_ready_at`) post-hora y `now ≥ max(driver_ready_at, exchange_at) + 10m` sin `owner_ready` | Cancel + `release` conductor; spot vuelve si listing vivo. Notif a ambos. Conductor **retractar** listo cancela el reloj (S9). |
| Safety net / cierre 60m | `now ≥ exchange_at + 60m` sin completar | **Sin** `owner_ready_at`: cancel + `release`. **Con** `owner_ready_at`: forfeit → `credit` dueño (S1). Notif a ambos. |

---

## Decisiones locked (esta sesion)

| # | Decision | Eleccion |
| --- | --- | --- |
| S1 | Safety net vs dueño ya listo | Con `owner_ready_at`, a +60m → forfeit dueño (no `release`). Con auto: forfeit tambien a +10m. Sin auto: +10..+60 puede seguir viva (S2). |
| S2 | Deadline 10 min blando | Pasar el suelo sin cancel/sweeper no bloquea completar. |
| S3 | Control del conductor | Un “listo en el punto” (matizado por S9); no par He llegado/Listo. |
| S4 | Persistir Yendo | `owner_en_route_at` / `driver_en_route_at`; historico; idempotente. |
| S5 | Conductor listo primero post-hora | Reloj owner-no-show; retractar cancela. |
| S6 | Dueño listo antes de hora | Ancla `max(owner_ready, exchange_at)`; retractar cancela reloj. |
| S7 | Cancel conductor &lt;30m, dueño sin salir | Forfeit estricto. |
| S8 | Notif matriz | Contrato; push = brief aparte. |
| S8b | Confirmacion | Dialogo obligatorio; copy del destino. |
| S9 | Listo simetrico + retractar + auto-cierre | Ambos listos → complete; retractar limpia ready + notif + para relojes. |

## Estado del documento

- Pie original (1–6) + S1/S2/S8b/S9: **locked**.
- Matriz: **alineada** a S9; status **approved**.
- Plan: [../plans/2026-09-20-spot-exchange-handshake.md](../plans/2026-09-20-spot-exchange-handshake.md).
