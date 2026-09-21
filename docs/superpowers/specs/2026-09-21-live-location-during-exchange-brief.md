# Brief: live location durante el intercambio (tipo Uber)

Status: **deferred** — sesión futura; **fuera del MVP** de push + geocerca asistida  
Depends on: handshake bilateral + push remoto + (recomendado) geocerca asistida  
Related:
- [2029-09-20-spot-exchange-refinment.md](./2029-09-20-spot-exchange-refinment.md)
- [2026-09-20-remote-push-implementation-brief.md](./2026-09-20-remote-push-implementation-brief.md)
- [2026-09-19-location-privacy-reveal-design.md](./2026-09-19-location-privacy-reveal-design.md)

---

## Qué es

Cuando una parte marca **Yendo** (`*_en_route_at`), la otra puede ver en el mapa
**dónde va** esa persona (pin o trail corto que se acerca al punto de encuentro),
como en Uber mientras el coche se acerca. No sustituye **Listo**: GPS no es el
compromiso contractual; Listo sigue cerrando el handshake y el dinero.

**No es:** tracking permanente, compartir ubicación con extraños en el mapa
público, ni sustituir fuzz pre-reserva.

---

## Por qué después del MVP de push

El MVP de notificaciones ya cubre “avanzar sin abrir la app” (avisos + acciones).
Live location es **otro canal** (stream de coordenadas + UI mapa), con más
superficie de privacidad, batería, permisos y backend. Se diseña aquí para no
perder el producto; se implementa cuando push + geocerca asistida estén estables.

---

## Ventana de producto (locked intent)

| Condición | Comportamiento |
| --- | --- |
| Reserva `confirmed` / live | Candidata |
| Emisor tiene `*_en_route_at` | Puede publicar posición |
| Receptor = la otra parte de la reserva | Único espectador |
| Completar / cancelar / expirar | Dejar de publicar; borrar buffer en servidor |
| Retractar Listo | No apaga necesariamente el share (sigue en camino); opcional: setting |
| Emisor apaga “compartir en vivo” (si existe toggle) | Para el stream; Yendo puede seguir |

Activación sugerida: **opt-in implícito al marcar Yendo** (copy en el confirm
S8b: “al ir de camino, la otra persona verá tu ubicación aproximada hasta el
intercambio”) + toggle en settings para desactivar share aunque Yendo esté on
(entonces solo geocerca asistida local / push, sin pin al peer).

---

## Privacidad (alineación con fuzz)

- Pre-reserva: el mapa público **sigue** fuzzed; live location **nunca** sale a
  WS de spots ni a listados.
- Post-reserva: el punto de encuentro ya es exacto para holder/owner. La
  posición en vivo del peer es **otra** señal: solo peer-to-peer de esa reserva.
- No persistir historial largo: buffer corto en memoria / Redis / tabla efímera
  con TTL (p. ej. última posición + opcional últimos N puntos ≤ 2–5 min).
- No escribir trail en el ledger ni en `spots`.
- GDPR: al borrar cuenta, tokens y cualquier fila de live location del user.

---

## Arquitectura (hexagonal)

```
reservations (use case)  — decide “este user puede publicar / suscribir”
        ↑ ports
postgres (quién es peer) | realtime (fanout WS) | (opcional) presence store
cmd/api wires
```

Reglas:

1. **No** meter lógica de dinero en el adaptador de posición.
2. El use case autoriza: `PublishLiveLocation(ctx, reservationID, userID, lat, lon)`
   y `Subscribe` implícito vía WS room por `reservation_id`.
3. Adapters no se importan entre sí; `realtime` emite; un store efímero opcional
   implementa “última posición” para quien abre la app tarde.
4. Rate-limit en el use case o middleware fino: p. ej. máx 1 punto / 2–5 s por
   emisor; descartar si dist &lt; umbral (ruido GPS).
5. Fallo de stream **no** afecta handshake (best-effort).

### Evento WS sugerido

```json
{
  "type": "reservation.peer_location",
  "reservation_id": "...",
  "lat": 40.41,
  "lon": -3.70,
  "recorded_at": "2026-09-21T10:00:00Z",
  "role": "driver"
}
```

Canal: misma conexión WS autenticada; filtrar por membresía de la reserva
(owner/driver). No broadcast a hubs globales de spots.

### HTTP (opcional, si no solo WS)

- `PUT /v1/reservations/{id}/live-location` body `{ lat, lon }` — solo si caller
  es parte y tiene en-route (o setting on).
- `GET /v1/reservations/{id}/live-location` — última posición del peer (o 204).

Preferible: cliente publica por WS o HTTP ligero; servidor fanout por WS.

---

## Mobile (Expo)

1. Tras Yendo (+ permiso ubicación “mientras usas la app” o background si ya
   existe por geocerca asistida): task que cada N s lee GPS y llama publish.
2. Foreground: pin del peer en el mapa del intercambio / SpotSheet.
3. Background: solo si ya se concedió para geocerca; **no** exigir always-on
   solo por live location si el usuario desactivó share.
4. UI: ETA opcional (haversine / straight-line al punto; no Google Directions
   obligatorio en v1).
5. Apagar publishers al terminal status o al logout.

Dependencias típicas: ya hay localización en el mapa; reutilizar el mismo
pipeline de permisos que la geocerca asistida del MVP de notifs.

---

## Relación con geocerca asistida (MVP notifs)

| | Geocerca asistida (MVP) | Live location (esta fase) |
| --- | --- | --- |
| Quién usa el GPS | Solo el propio dispositivo | Emisor publica; receptor observa |
| Destino del evento | Push local / remota al **mismo** user (“¿Listo?”) | Pin al **peer** |
| Contrato | No marca Listo solo | No marca Listo solo |
| Activación | Tras Yendo | Tras Yendo (+ opt-in copy) |

Misma ventana de permisos; dos consumidores del GPS.

---

## Criterios de aceptación (cuando se implemente)

1. Con ambos en Yendo (o uno), el peer ve actualizarse el pin ≤ ~5–10 s en
   condiciones normales; al completar, el pin desaparece.
2. Un tercero autenticado no recibe `peer_location` de esa reserva.
3. Sin Yendo / share off → 403 o silent drop; sin fuga a mapa público.
4. `arch_test` verde; sin import de `realtime` desde use cases incorrectos.
5. Demo: dos devices, uno en movimiento (o mock GPS), el otro ve el acercamiento.

---

## Orden de implementación sugerido (sesión futura)

1. Authz + evento WS + última posición en memoria/TTL.
2. Mobile publisher tras Yendo + pin en UI de reserva.
3. Rate-limit + apagado en terminal.
4. Copy S8b / settings “compartir ubicación en camino”.
5. (Opcional) ETA + trail corto.
6. Documentar demo en `PROGRESS.md`.

---

## Fuera de alcance explícito

- Navegación turn-by-turn.
- Compartir ubicación antes de aceptar oferta.
- Grabación/forense de rutas para disputas (si algún día: spec aparte + retención).
- Sustituir teléfono / WhatsApp post-reserva.

---

## Nota de producto

Live location **no** entra en el MVP de notificaciones. El diseño de push +
acciones + geocerca one-shot + tips del conductor está en
[2026-09-21-exchange-push-coaching-design.md](./2026-09-21-exchange-push-coaching-design.md).
Este brief es solo handoff para una sesión futura de ubicación en vivo.
