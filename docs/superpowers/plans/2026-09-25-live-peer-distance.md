# Distancia en tiempo real al punto de encuentro

**Estado:** implementado localmente el 2026-09-25; pendiente de migración y
smoke PostGIS/dispositivo.

## Goal

Cuando una persona marca «Voy de camino», el servidor debe recibir su última
ubicación válida, calcular la distancia directa al punto de encuentro y mostrar
al otro participante los metros y la antigüedad de la medición. El dato se
actualiza mientras la ubicación en segundo plano siga activa y se conserva solo
durante la reserva activa.

La app debe mostrar «Actual» si la medición tiene menos de un minuto; después,
«Hace X minutos». Si no existe una ubicación válida, no muestra distancia.
Las notificaciones push de salida incluyen una distancia inicial solo si el
servidor ya dispone de ella; no intentan actualizar el texto posteriormente.

## Out of scope

- Distancia por carretera, navegación o cálculo de ETA.
- Historial, trazado o almacenamiento de rutas.
- Compartir coordenadas crudas con el participante contrario.
- Cambiar la política de permisos: Always/background solo después de «Voy de
  camino».
- Rehacer la detección local de llegada al punto.

## Invariantes

- El servidor es la única fuente de verdad para metros y timestamp.
- El servidor guarda únicamente la última posición de cada lado de una reserva.
- Solo se acepta ubicación de un participante autenticado, involucrado, en
  estado `en_route` y con reserva activa.
- Las coordenadas no salen en la respuesta: solo salen metros y timestamp del
  otro participante.
- Al completar, cancelar o expirar una reserva se limpian las ubicaciones.
- La distancia usa la misma distancia geográfica directa/haversine que el
  código de llegada existente y se redondea al metro más cercano.

## File map

| Archivo                                                                      | Cambio                                                                                                    | Capa              |
| ---------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- | ----------------- |
| `services/api/migrations/00022_reservation_peer_locations.sql`               | Añadir columnas de última ubicación y timestamp con rango/limpieza compatible con reservas terminales.    | DB                |
| `services/api/internal/domain/reservation.go`                                | Añadir el valor de ubicación temporal al modelo, sin dependencias HTTP/SQL.                               | domain            |
| `services/api/internal/reservations/ports.go`                                | Declarar el puerto consumidor para registrar ubicación y cargar el dato del peer.                         | use case          |
| `services/api/internal/reservations/service.go` y tests                      | Validar actor/estado, registrar ubicación y proyectar distancia para el peer.                             | use case          |
| `services/api/internal/postgres/reservations.go`, `ports.go`                 | Implementar escritura atómica, lectura de la reserva y limpieza terminal.                                 | adapter           |
| `services/api/internal/api/reservations.go`, `api.go`, tests                 | Añadir endpoint autenticado de ubicación y serializar distancia/timestamp.                                | adapter           |
| `services/api/internal/contract/types.gen.go`                                | Regenerar tipos tras cambiar OpenAPI; no editar a mano salvo que el generador lo requiera.                | contrato          |
| `packages/api-contract/openapi.yaml`                                         | Documentar request de ubicación, campos peer y evento `reservation.updated`.                              | contrato          |
| `services/api/internal/domain/events.go`, realtime/notificación relacionados | Publicar una actualización de reserva cuando cambia la ubicación, con throttling si el canal lo necesita. | adapter           |
| `services/api/internal/push/expo.go` y tests                                 | Añadir metros iniciales al texto de salida cuando estén disponibles.                                      | adapter           |
| `apps/mobile/src/api/client.ts`                                              | Enviar ubicaciones y leer los campos del peer.                                                            | mobile API        |
| `apps/mobile/src/push/geofence.ts`                                           | Enviar cada fix válido al backend sin alterar la detección local de llegada.                              | mobile background |
| `apps/mobile/src/map/exchange.ts` y superficies de intercambio               | Formatear metros/antigüedad y usarlo donde hoy se muestra el estado del peer.                             | mobile UI         |
| `apps/mobile/src/i18n/locales/es.ts`, `en.ts`                                | Añadir textos de distancia, medición actual y minutos.                                                    | mobile i18n       |
| `apps/web/assets/i18n.js`                                                    | Actualizar privacidad y condiciones en español/inglés.                                                    | legal             |
| `apps/web/privacy.html`, `terms.html`                                        | Actualizar fecha visible si cambia el texto legal.                                                        | legal             |
| `PROGRESS.md`                                                                | Registrar implementación y smoke test pendiente si aplica.                                                | docs              |

## Tasks

### 1. Fijar el contrato y las reglas puras

**Objetivo:** definir el payload estable antes de tocar adaptadores.

**Archivos:** `packages/api-contract/openapi.yaml`, helpers de dominio y sus
tests, `services/api/internal/contract/types.gen.go`.

**TDD:**

1. Añadir primero tests para distancia redondeada, medición reciente (< 1 min),
   medición antigua y ausencia de medición; comprobar que fallan.
2. Implementar el helper puro reutilizando la distancia geográfica existente.
3. Añadir al OpenAPI el request `latitude`, `longitude`, un `204` para aceptar
   la actualización y campos opcionales `peer_distance_m` y
   `peer_distance_measured_at` en `ReservationResponse` y eventos de reserva.
4. Regenerar tipos y comprobar que contrato y tipos no tienen drift.

**Verificación:**

```powershell
task contract:generate
task contract:check
task api:test
```

### 2. Persistir solo la última ubicación de la reserva

**Objetivo:** almacenar dos posiciones, una por participante, sin crear un
historial.

**Archivos:** `services/api/migrations/00022_reservation_peer_locations.sql`,
`internal/domain/reservation.go`, `internal/postgres/reservations.go` y tests
de schema/integración.

**TDD:**

1. Escribir una prueba de integración que registre una posición por lado,
   reemplace la anterior y confirme que solo existe la última.
2. Escribir pruebas de limpieza después de completar, cancelar y expirar.
3. Implementar columnas `*_location`/`*_location_at` con PostGIS o el tipo
   espacial ya usado por el proyecto, validación de coordenadas y actualización
   atómica condicionada a reserva activa y participante correcto.
4. Hacer que `ReservationByID` y las consultas de reservas carguen el dato
   necesario para calcular la distancia del peer.

**Verificación:** `task db:up`, `task db:migrate`, `task api:test`.

### 3. Añadir el caso de uso y el endpoint de ubicación

**Objetivo:** mantener permisos y reglas en `internal/reservations`, no en el
handler.

**Archivos:** `internal/reservations/ports.go`, `service.go`, `service_test.go`,
`internal/api/reservations.go`, `api.go`, `reservations_test.go`.

**TDD:**

1. Añadir tests de servicio para usuario ajeno, reserva terminal, usuario que
   aún no está `en_route`, coordenadas inválidas y actualización válida.
2. Implementar un método de caso de uso con puerto consumidor, por ejemplo
   `UpdatePeerLocation(ctx, reservationID, viewer, lat, lon)`, que devuelva la
   vista de la reserva autorizada para ese usuario.
3. Añadir `POST /v1/reservations/{id}/location` con JSON `{latitude,longitude}`;
   el handler solo decodifica, llama al servicio y serializa.
4. Verificar que la respuesta nunca contiene las coordenadas propias o del
   peer y que el peer recibe solo distancia/timestamp.

**Verificación:** `task api:test`.

### 4. Emitir actualizaciones en tiempo real y limpiar terminales

**Objetivo:** que el otro móvil reciba cambios sin esperar una recarga manual.

**Archivos:** eventos de dominio, adapter Postgres/realtime, contrato generado y
tests WebSocket/API.

**TDD:**

1. Añadir un test que publique `reservation.updated` al aceptar una nueva
   ubicación y otro que no publique coordenadas crudas.
2. Implementar la notificación después del commit, siguiendo el canal
   `LISTEN/NOTIFY` existente.
3. Evitar ruido excesivo: coalescer o limitar eventos de ubicación sin impedir
   que `GET reservation` devuelva siempre el último dato.
4. Confirmar que la limpieza terminal también invalida el dato en snapshots y
   respuestas posteriores.

**Verificación:** `task api:test` y el test de integración WebSocket existente.

### 5. Enriquecer el push de salida

**Objetivo:** incluir una distancia inicial en el push, solo cuando esté
disponible en el momento de construirlo.

**Archivos:** `internal/reservations/service.go`, `internal/reservations/notify.go`,
`internal/push/expo.go`, tests de copia y servicio.

**TDD:**

1. Añadir tests de push con medición presente y ausente, incluyendo la forma
   localizada y el redondeo al metro.
2. Hacer que `EnRoute` cargue la vista actual después de registrar el estado y
   pase el dato inicial en la notificación, sin bloquear la transición si no
   hay ubicación.
3. Mantener el texto actual cuando la medición no existe o no es válida.

**Verificación:** `task api:test`.

### 6. Enviar ubicación desde segundo plano

**Objetivo:** conectar cada fix válido del flujo Expo existente con el nuevo
endpoint sin romper la notificación local de llegada.

**Archivos:** `apps/mobile/src/api/client.ts`, `apps/mobile/src/push/geofence.ts`,
tests de cliente/helpers y configuración de autenticación si el flujo lo exige.

**TDD:**

1. Añadir test del payload y de que los errores de red no detienen la tarea de
   ubicación ni generan reintentos agresivos.
2. Implementar `updateReservationLocation` y llamarlo por cada fix con
   coordenadas y precisión aceptables; mantener la llamada local de radio de
   llegada.
3. Mantener el envío best-effort, con debounce/throttle razonable para batería,
   datos y carga del servidor.
4. Confirmar que al marcar listo, cancelar o finalizar se detiene el tracking y
   el servidor limpia el dato al cambiar a estado terminal.

**Verificación:**

```powershell
cd apps/mobile
pnpm exec tsc --noEmit
node --experimental-strip-types --test src/push/*.test.ts
```

### 7. Mostrar distancia en todas las superficies del peer

**Objetivo:** cualquier UI que hoy diga que la otra parte «va de camino» debe
mostrar también distancia y antigüedad.

**Archivos:** localizar los consumidores de `peerPhase`/`owner_en_route_at` y
los componentes de reserva, `apps/mobile/src/map/exchange.ts`, locales ES/EN y
tests de render/formateo.

**TDD:**

1. Añadir tests de formato: `Actual`, `Hace 1 minuto`, pluralización y ausencia
   de medición; no mostrar distancia en `idle`/`ready`.
2. Implementar un helper único para evitar que cada pantalla interprete el
   timestamp de forma distinta.
3. Conectar `ReservationResponse` y eventos WebSocket a todas las superficies:
   banner, hoja/mapa, detalle de reserva, estados de conductor y propietario,
   y cualquier modal equivalente localizado durante la búsqueda.
4. Verificar que el propio estado «Voy de camino» no muestra por error la
   distancia del usuario como si fuera la del peer.

**Verificación:** `cd apps/mobile; pnpm exec tsc --noEmit; node --experimental-strip-types --test src/**/*.test.ts`.

### 8. Actualizar privacidad y condiciones públicas

**Objetivo:** documentar el tratamiento real de ubicación de intercambio y su
retención limitada.

**Archivos:** `apps/web/assets/i18n.js`, `apps/web/privacy.html`,
`apps/web/terms.html`.

**Cambios:**

1. En privacidad, explicar que al activar «Voy de camino» se puede procesar
   ubicación en segundo plano para calcular y mostrar al otro participante la
   distancia directa al punto de encuentro.
2. Aclarar que se comparte con el otro participante una distancia y la hora de
   medición, no las coordenadas exactas, y que la disponibilidad depende de
   permisos, GPS y conectividad.
3. En conservación, indicar que solo se mantiene la última posición durante la
   reserva activa y se elimina al completarse, cancelarse o expirar; conservar
   únicamente los registros operativos que ya describa la política.
4. En condiciones, aclarar que la distancia es aproximada, no es navegación ni
   garantía de llegada, y puede ser antigua o no estar disponible.
5. Actualizar la fecha de ambos textos a la fecha real de publicación. Mantener
   el aviso de que es un borrador y revisar legalmente antes de publicar.

**Verificación:** `cd apps/web; node --test assets/i18n.test.js` y comprobación
manual ES/EN de `privacy.html` y `terms.html`.

### 9. Preflight y demo de dispositivo

**Objetivo:** detectar fallos de privacidad, batería, permisos o contratos antes
de declarar la feature terminada.

**Pasos:**

1. Ejecutar revisión crítica sobre autorización, exposición de coordenadas,
   carreras entre ubicación y estado terminal, limpieza y arquitectura.
2. Ejecutar `task doctor`, `task contract:check`, `task api:test` y la suite
   móvil/web correspondiente.
3. Con dos dispositivos: reservar, marcar «Voy de camino», verificar push,
   comprobar actualización de metros y timestamp, desactivar ubicación,
   recuperar conectividad y marcar «Listo».
4. Confirmar que una reserva terminada ya no devuelve distancia ni actualiza al
   peer y que no se solicita Always en frío.
5. Actualizar `PROGRESS.md` con la evidencia y cualquier smoke test pendiente.

## Risks

- **Privacidad:** una respuesta o evento podría filtrar coordenadas; los tests
  deben comprobar explícitamente que solo aparecen metros y timestamp.
- **Autorización:** el endpoint debe impedir consultar o escribir la reserva de
  otra persona; el caso de uso debe ser la autoridad.
- **Concurrencia:** una ubicación puede llegar mientras se marca `ready`, se
  cancela o expira; las escrituras deben ser condicionales y la limpieza debe
  ganar para estados terminales.
- **Carga y batería:** una actualización por fix puede generar demasiado tráfico;
  limitar envíos sin impedir que la última medición sea razonablemente fresca.
- **Permisos y proceso muerto:** sin Always, GPS o red no habrá distancia; la UI
  debe degradar a estado sin medición sin bloquear «Listo».
- **Push:** el push es una instantánea; nunca debe presentarse como distancia
  garantizada o en tiempo real.
- **Contrato generado:** OpenAPI, tipos Go y cliente TypeScript deben regenerarse
  juntos y validarse contra drift.
- **Legal:** la copia pública debe describir la implementación real y pasar una
  revisión jurídica antes de producción.

## Done when

- [ ] La última ubicación válida se guarda solo durante la reserva activa.
- [ ] El servidor calcula metros directos y el timestamp, sin exponer coordenadas.
- [ ] El peer ve la distancia en todas las superficies con estado `en_route`.
- [ ] «Actual» y «Hace X minutos» funcionan en español e inglés.
- [ ] El push de salida incluye distancia inicial cuando existe y conserva el
      fallback cuando no existe.
- [ ] La distancia se actualiza por WebSocket/refresh y no requiere historial.
- [ ] Completar, cancelar o expirar elimina las ubicaciones.
- [ ] API, contrato, móvil, tests y páginas públicas están sincronizados.
- [ ] `task api:test`, `task contract:check`, tests móviles/web y smoke de dos
      dispositivos tienen evidencia reciente.
