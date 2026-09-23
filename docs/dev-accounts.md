# Cuentas de seed (desarrollo)

Tras `task db:seed`, todas las cuentas usan la misma contraseña.
La API local debe estar en marcha (`task api:run`) y la app apuntando a ella
(`http://10.0.2.2:8080` en emulador).

**Contraseña (todas):** `parkxchange`

## Cuentas recomendadas

| Uso | Email | Nombre | Notas |
| --- | --- | --- | --- |
| **Anunciar plazas** | `driver@parkxchange.test` | Carlos Méndez | Sin anuncios abiertos; 5 reseñas. Puede crear «me voy ya» / flexible / preferida. |
| **Mapa + perfil con reseñas** | `owner@parkxchange.test` | Lucía Navarro | Tiene landmarks y 6 reseñas; **no** uses para anunciar (muchos spots abiertos → conflicto). |

Los pins de Malvaloca / Triana tienen otros dueños con reseñas (p. ej. Ana Beltrán =
`driver01@parkxchange.test`).

Fuente: `services/api/internal/seed` (`DevPassword` + datos de demo).
