# Tenis Ahora — API de prueba (Go + SQLite)

Servidor HTTP liviano para que el front (`../Tenis-Ahora-Frontend`) trabaje contra datos
persistidos en lugar de los mocks en memoria. **Es una API de prueba**: una sola base SQLite,
un documento JSON por entidad y sin pretensiones de soportar tráfico real. La versión
definitiva la hará otra persona; el informe `INFORME-API.pdf` (en `D:\fede`) describe el
alcance, los endpoints y las recomendaciones para esa versión.

## Stack

| Pieza | Elección |
|---|---|
| Lenguaje / runtime | Go 1.22+ |
| HTTP | `net/http` de la librería estándar (router con método y parámetros, CORS permisivo, logs, archivos estáticos) |
| Base de datos | SQLite vía `modernc.org/sqlite` (Go puro: sin cgo, sin compilador C, sin DLL) |
| Auth | bcrypt (`golang.org/x/crypto`) para contraseñas, JWT HS256 (`crypto/hmac`) |
| Datos iniciales | `seed/seed.json`, exportado del mock del front |

## Requisitos

- Go 1.22 o posterior.

## Puesta en marcha

```sh
cd api
go mod tidy            # la primera vez: descarga dependencias y genera go.sum
go run .               # levanta en http://localhost:5090/api
go build -o tenis-ahora-api .   # binario (en Windows: -o tenis-ahora-api.exe)
```

Para dejar todo corriendo en segundo plano (API + front) usar `D:\fede\start-servidor.ps1`;
para detenerlo, `D:\fede\stop-servidor.ps1`.

### Variables de entorno (`.env` opcional, ver `.env.example`)

| Variable | Default | Uso |
|---|---|---|
| `PORT` | `5090` | Puerto HTTP (el front espera `http://localhost:5090/api`). |
| `DATABASE_PATH` | `data/tenis_ahora.sqlite` | Archivo SQLite; se crea solo. |
| `SEED_PATH` | `seed/seed.json` | Datos iniciales; se cargan solo si la base está vacía. |
| `JWT_SECRET` | valor de desarrollo | Clave HS256 de los tokens. |
| `FRONTEND_DIST` | `../Tenis-Ahora-Frontend/dist` | Si existe `index.html` ahí, la API sirve el front en `/`. |

### Usuarios de prueba

Todos los usuarios del seed tienen contraseña **`123456`**.

| Email | Rol |
|---|---|
| `admin@tenisahora.com` | Empleado (panel de administración) |
| `socio@tenisahora.com` | Socio |
| cualquier otro de `seed/seed.json` → `usuarios` | Socio |

`POST /api/admin/reset` vacía la base y recarga el seed (solo rol Empleado; otro rol → 403).

## Deploy (Docker + VPS)

`.github/workflows/deploy.yml` corre en cada push a `main`: construye la imagen con
`ci/Dockerfile`, la copia por SSH al VPS (sin registry) y reemplaza el contenedor
`tenisahora-api` en la red `caddy-net`, sin exponer puertos. La base queda en
`/home/devops/tenisahora-api/data` y la configuración en `/home/devops/tenisahora-api/.env`.

Secrets del repo: `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY` y opcionalmente `VPS_PORT`.

En el VPS:

- Poner un `JWT_SECRET` propio en `/home/devops/tenisahora-api/.env` (si queda vacío se usa el
  de desarrollo) y reiniciar con `docker restart tenisahora-api`.
- Agregar al Caddyfile del Caddy que está en `caddy-net`:

  ```
  tenisahora-api.duckdns.org {
      reverse_proxy tenisahora-api:5090
  }
  ```

La API queda en `https://tenisahora-api.duckdns.org/api`.

## Cómo se conecta el front

- `src/services/httpApi.js` implementa los mismos servicios que el mock contra esta API.
- `src/services/api.js` exporta cada servicio envuelto en `withBackend(mock, http)`: usa HTTP
  cuando `GET /api/health` responde y la sesión **no** es de Modo Demo; si no, mocks.
- Login y registro ya hablaban con `/api/auth/*`; el `AuthResponseDto` se respeta y se
  agregan `dni` y `memberNumber`.
- Para servir el front desde la API: `npm run build` en el front con `VITE_API_URL=/api`
  (lo hace `start-servidor.ps1`) y la API lo expone en `http://localhost:5090/`.

## Endpoints

Todas las rutas van bajo `/api`. Salvo `health`, `auth/login` y `auth/registrar`, exigen
`Authorization: Bearer <token>`. Los errores vuelven como `{ "error": "mensaje" }` con
400 / 401 / 403 / 404 / 409 / 500.

### Públicos

| Método | Ruta | Descripción |
|---|---|---|
| GET | `/health` | Estado, versión, versión de SQLite y conteo por colección. |
| POST | `/auth/login` | `{ email, password }` → `AuthResponseDto` con `token`. |
| POST | `/auth/registrar` | `{ nombre, apellido, email, password, direccion?, numeroTelefono?, dni? }` → 201. |

### CRUD genérico (`{col}` = una colección)

Colecciones: `usuarios`, `canchas`, `tipos-cancha`, `stock`, `reservas`, `torneos`, `ligas`,
`tipos-competencia`, `profesores`, `clases`, `clases-particulares`, `recibos`, `facturas`,
`reembolsos`, `descuentos`.

| Método | Ruta | Descripción |
|---|---|---|
| GET | `/{col}` | Lista completa (orden de inserción). |
| GET | `/{col}/{id}` | Un documento. |
| POST | `/{col}` | Alta. Genera `id` si falta y aplica los defaults/validaciones de la entidad. 201. |
| PUT | `/{col}/{id}` | Modificación por mezcla superficial (`{ ...actual, ...body }`), `id` inmutable. |
| DELETE | `/{col}/{id}` | Baja. `tipos-cancha` en uso → 409; `descuentos` se quita de los usuarios. |

Reglas de alta/modificación destacadas: `usuarios` (email único, `password` → `passwordHash`,
hash nunca se devuelve), `tipos-cancha` (nombre único, cupo ≤ 30), `stock` (recalcula disponible y
alerta), `clases` (cupo ≤ 30, RD07), `descuentos` (código único en mayúsculas), `facturas`
(calcula neto, IVA y total), `reembolsos` (total/parcial), `reservas` (PUT recalcula fin, costo y saldo).

### Negocio

| Método | Ruta | Cuerpo | Respuesta |
|---|---|---|---|
| GET | `/auth/yo` | — | claims del token |
| POST | `/admin/reset` | — | `{ success, documents }` (solo rol Empleado, 403 si no) |
| PATCH | `/canchas/{id}/estado` | `{ status, maintenanceNotes }` | cancha |
| PUT | `/canchas/{id}/disponibilidad` | `{ days, openTime, closeTime, slotMinutes, blockedRanges, notes }` | cancha |
| PATCH | `/stock/{id}/ajuste` | `{ totalStock }` | ítem |
| POST | `/reservas` | datos de la reserva | `{ data, receipt }` (recibo de seña 50%, RD01) — 409 si la cancha está en mantenimiento (RD06) |
| POST | `/reservas/{id}/pagar-saldo` | `{ paymentMethod }` | reserva finalizada + recibo final |
| POST | `/reservas/{id}/cancelar` | `{ hoursInAdvance }` | `{ data, isWithinFreeWindow, refund, penaltyReceipt }` (RD02–RD04) |
| PUT | `/torneos/{id}/reglamento` · `/ligas/{id}/reglamento` | `{ internalRules }` | competencia |
| POST | `/torneos/{id}/inscripciones` · `/ligas/{id}/inscripciones` | `{ name, dni, phone, partnerName, partnerDni }` | `{ success, participant }` (409 si no hay cupo) |
| DELETE | `/torneos/{id}/inscripciones/{pid}` · `/ligas/{id}/inscripciones/{pid}` | — | `{ success }` |
| POST | `/torneos/{id}/enfrentamientos` · `/ligas/{id}/enfrentamientos` | `{ round, p1, p2, court, date }` | `{ success, match }` |
| PUT | `/torneos/{id}/enfrentamientos/{mid}` · `/ligas/{id}/enfrentamientos/{mid}` | campos a modificar | `{ success, match }` |
| POST | `/ligas/{id}/enfrentamientos/{mid}/resultado` | `{ score, winner }` | `{ success, match, standings }` (3 pts por victoria, no duplica si ya estaba finalizado) |
| PUT | `/profesores/{id}/disponibilidad` | `{ availableHours }` | docente |
| PUT | `/profesores/{id}/certificacion` | campos de `licenseDetails` | docente |
| POST | `/clases/{id}/alumnos` | `{ name, dni, phone }` | `{ success, student }` (409 si el cupo está completo, RD10) |
| DELETE | `/clases/{id}/alumnos/{sid}` | — | `{ success }` |
| POST | `/clases/{id}/asistencia/alternar` | `{ studentId, sessionIndex }` | `{ success, classes }` (P → A → J → P) |
| POST | `/clases/{id}/asistencia/sesion` | `{ sessionIndex, status }` | `{ success, classes }` |
| POST | `/clases/{id}/asistencia/registrar` | `{ studentId, status }` | `{ success }` |
| POST | `/clases-particulares/{id}/cancelar` | — | `{ success, data }` |
| POST | `/facturas/{id}/anular` | — | factura |
| PATCH | `/reembolsos/{id}/estado` | `{ status }` (`pendiente` / `procesado` / `rechazado`) | reembolso |
| PUT | `/usuarios/{id}/descuentos` | `{ activeDiscounts: [códigos] }` | usuario |

## Pruebas

```powershell
# con la API corriendo
python tests\smoke.py                      # http://localhost:5090/api
python tests\smoke.py http://otra-ip:5090/api
```

El script recorre autenticación, CRUD, reglas de dominio y los endpoints de negocio
(46 verificaciones) y deja la base reseteada al terminar.

## Estructura

```
api/
├── go.mod / go.sum
├── ci/Dockerfile      # imagen del deploy
├── .github/workflows/deploy.yml
├── seed/seed.json     # datos iniciales (del mock del front)
├── main.go            # configuración, .env, router, CORS, logs, /health, archivos estáticos del front
├── db.go              # Store: tabla documents (collection, id, data JSON)
├── auth.go            # login, registro, JWT, middleware
├── crud.go            # CRUD genérico + reglas por colección
├── business.go        # endpoints de negocio
├── seed.go            # carga del seed
├── errors.go          # apiError -> { error }
├── util.go            # helpers de JSON y fechas
├── tests/smoke.py
└── CHANGELOG.md
```
