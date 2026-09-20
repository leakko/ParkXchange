# Brief: implementar push remoto (Expo)

Status: ready for a future implementation session  
Depends on: handshake + matriz de notifs ya definidas  
Source of truth (copy / cuándo / a quién):
[2029-09-20-spot-exchange-refinment.md](./2029-09-20-spot-exchange-refinment.md)  
Also: [2026-09-19-offer-based-exchange-design.md](./2026-09-19-offer-based-exchange-design.md) § Notifications

---

## Para el modelo / agente que implemente esto

Lee este brief **antes** de tocar código. No reinventes el catálogo de mensajes: está en la matriz. No metas reglas de negocio (dinero, deadlines) en el adaptador de push.

**Objetivo de esta fase:** que un usuario con la app en background o cerrada reciba las mismas notificaciones que la matriz exige, vía **Expo Push Notifications**, sin romper el hexagonal ni duplicar lógica del handshake.

**Fuera de alcance de esta fase (ya cubierto o aplazado):**
- Definir textos / momentos (ya locked en la matriz + S8).
- Sustituir WebSocket: el push **complementa** WS; con app en foreground, preferir UI/WS y **deduplicar** (no spamear banner + push).
- Chat, disputas, marketing push.

---

## Contexto del repo (estado esperado al empezar)

- Mobile: Expo SDK 57, `expo-notifications` ya en `package.json` / `app.config.ts`, **sin** flujo de registro de tokens ni handlers remotos cableados.
- API: Go hexagonal; WS en `internal/realtime`; **sin** envío push ni tabla de device tokens.
- Handshake emitirá (o ya emite) eventos de dominio / WS (`reservation.updated`, terminal cancel/complete). El push debe **engancharse a esos mismos puntos**, no a handlers HTTP sueltos.

Si al llegar el código del handshake aún no expone un puerto de “avisar a la otra parte”, **créalo** en el use case (ver arquitectura abajo). No notifiques desde `internal/api`.

---

## Arquitectura obligatoria

```
domain / reservations|offers|spots (use case)
  → declara puerto Notifier (o ReservationNotifier)
postgres | expo_push | realtime   ← adapters; no se importan entre sí
cmd/api wire: use case ← Notifier compuesto (WS + Push) o dos puertos
```

Reglas:
1. El use case decide **qué evento** ocurrió (`OwnerEnRoute`, `OwnerReady`, `DriverArrived`, `ExchangeCompleted`, `CancelledByOwner`, …).
2. El adaptador solo traduce evento → título/cuerpo/data y lo envía.
3. Fallo de push **no** debe revertir la transacción de negocio (best-effort async o post-commit). Loguear y seguir.
4. Idempotencia / cooldown: re-marcar Yendo/Listo no re-dispara push (misma regla que la matriz). Clave sugerida: `(reservation_id, event_kind)` o “solo si el timestamp de esa señal era NULL y ahora no”.
5. Registrar el paquete nuevo en `internal/arch/arch_test.go` si aplica.

### Persistencia de tokens

Tabla (nombre orientativo) `device_push_tokens`:
- `user_id`, `expo_push_token` (unique), `platform` (`ios`|`android`), `updated_at`, opcional `app_version`
- Un usuario puede tener varios dispositivos.
- Al logout / 410 InvalidCredentials de Expo: borrar o marcar inválido el token.

API:
- `PUT /v1/me/push-token` (o `POST`) body `{ "token": "ExponentPushToken[...]", "platform": "ios"|"android" }`
- Autenticado. Upsert por token.

---

## Mobile (Expo)

1. Pedir permisos en el momento adecuado (tras login / al entrar en una reserva activa), no solo al cold start agresivo.
2. Obtener Expo push token (`Notifications.getExpoPushTokenAsync` con `projectId` EAS).
3. Registrar en la API; re-registrar si cambia el token.
4. Handlers:
   - **Foreground:** opcionalmente no mostrar sistema push si ya hay banner/WS; o mostrar con cuidado.
   - **Tap / cold start:** deep link a `/account/reservations/{id}` usando `data.reservation_id`.
5. Canal Android (importance alta) para eventos urgentes (`owner_ready` post-hora, `driver_arrived` post-hora, `sal_ya`).
6. No uses notificaciones locales programadas como sustituto del push remoto para señales de la otra parte (sí puedes usar local solo para recordatorios de “falta 15 min para tu exchange_at” si se especifica después).

Variables / EAS: seguir el patrón de [mobile-env-eas](./2026-09-19-mobile-env-eas-design.md) si existe; el `projectId` debe estar disponible en runtime.

---

## Backend (envio)

Proveedor recomendado: **Expo Push API** (`https://exp.host/--/api/v2/push/send`) desde un adapter `internal/push` (o similar), HTTP client stdlib.

Payload mínimo por envío:
```json
{
  "to": "ExponentPushToken[...]",
  "title": "...",
  "body": "...",
  "sound": "default",
  "priority": "high",
  "channelId": "exchange-urgent",
  "data": {
    "type": "reservation.owner_ready",
    "reservation_id": "...",
    "exchange_at": "..."
  }
}
```

- Enviar a **todos** los tokens activos del destinatario.
- Respetar receipts / errores Expo; limpiar tokens muertos.
- Rate: batch si hay varios tokens; no bloquees el request HTTP del usuario.

Wiring: tras commit exitoso del use case (o en el mismo servicio después del store), llamar `Notifier.Notify(ctx, event)`.

---

## Catálogo de eventos push (handshake)

Destinatario = la **otra** parte. El actor no recibe push de su propia acción (salvo que la matriz diga lo contrario; hoy no).

| `type` (data) | Disparador | Destinatario | Title/body (ES, contrato; i18n después) | Prioridad |
| --- | --- | --- | --- | --- |
| `reservation.owner_en_route` | Dueño marca Yendo | Conductor | Dueño de camino al intercambio (tono “tarde” si `now ≥ exchange_at`) | normal |
| `reservation.driver_en_route` | Conductor marca Yendo | Dueño | Conductor de camino al intercambio (tono tarde si post-hora) | normal |
| `reservation.owner_ready` | Dueño Listo (sin completar) | Conductor | Dueño listo en el coche; si post-hora: urgente + ~10 min | high si post-hora |
| `reservation.driver_ready` | Conductor Listo en el punto (sin completar) | Dueño | Conductor listo en el punto; si post-hora: urgente + ~10 min | high si post-hora |
| `reservation.driver_unready` | Conductor retracta listo | Dueño | El conductor ya no esta listo en el punto | normal |
| `reservation.owner_unready` | Dueño retracta listo | Conductor | El dueño retiro su listo | normal |
| `reservation.completed` | Doble en-sitio / cierre | Dueño (obligatorio “Sal ya”); conductor puede banner in-app | **Sal ya — intercambio cerrado** | high |
| `reservation.cancelled_by_owner` | Cancel dueño | Conductor | Cancelada por el dueño — depósito liberado (o forfeit si rama no-show conductor) | high |
| `reservation.cancelled_by_driver` | Cancel conductor | Dueño | Canceló el conductor — liberado **o** cobras depósito (rama 30 min / forfeit) | high |
| `reservation.driver_no_show` | Sweeper / auto forfeit | Ambos | Conductor no se presentó — depósito al dueño | high |
| `reservation.owner_no_show` | Sweeper / release | Ambos | Dueño no marcó a tiempo — depósito liberado al conductor | high |
| `reservation.safety_net` | +60m sin `owner_ready` | Ambos | Reserva cancelada por tiempo — depósito liberado | high |
| `reservation.safety_net_owner_ready` | +60m **con** `owner_ready` (S1) | Ambos | Cierre por tiempo — depósito al dueño (forfeit) | high |

Cooldown: mismos `type` + `reservation_id` no se reenvían si la señal es idempotente (re-tap Yendo).

Textos exactos: alinear con celdas **Notif** de la matriz; si hay conflicto, gana la matriz refinada + decisiones S1–S8.

---

## Catálogo adicional (ofertas) — mismo adaptador

Del diseño 2026 (misma infra, otra fase o el mismo PR si el handshake ya está):

| `type` | Disparador | Destinatario |
| --- | --- | --- |
| `offer.created` | Nueva oferta | Dueño del spot |
| `offer.accepted` | Accept | Conductor oferente |
| `offer.rejected` | Reject / sibling reject on accept | Conductor |
| `offer.expired` | TTL 24h | Conductor |
| `offer.withdrawn` | (opcional avisar dueño) | Dueño |
| `spot.preferred_departure_changed` | Owner edita preferencia | Conductores con offer pending |

---

## Criterios de aceptación

1. Con app en background, al marcar la otra parte Yendo / Listo / En sitio / cancel / complete, llega push con `data.reservation_id` y al tocarla abre el detalle correcto.
2. Re-marcar Yendo no genera un segundo push.
3. Completar no deja la reserva a medias si Expo está caído (negocio OK; push best-effort).
4. Token inválido se elimina; el usuario puede volver a registrar.
5. `task api:test` + prueba manual en dispositivo real (simulador iOS tiene límites de push; preferir device).
6. Arch test verde; sin lógica de ledger en `internal/push`.

---

## Orden de implementación sugerido

1. Migración `device_push_tokens` + `PUT /v1/me/push-token`.
2. Puerto `Notifier` + no-op / log adapter en tests.
3. Adapter Expo HTTP + wiring en `cmd/api`.
4. Enganchar eventos del handshake (y luego ofertas).
5. Mobile: permisos, registro token, response listener → deep link.
6. Deduplicación foreground vs push.
7. Documentar en `PROGRESS.md` el demo (dos devices o device + curl al Expo push tool).

---

## Decisiones de producto ya locked (no reabrir)

Ver S8 en la matriz: las celdas Notif son **contrato**; el handshake puede shippear antes con WS/in-app; **este** brief es la fase push remoto.

S1–S7 afectan copy y ramas de cancel/safety net: léelos antes de mapear `reservation.safety_net*`.
