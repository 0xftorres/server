# Tenis Ahora — API de prueba (Rust + SQLite)

Servidor HTTP liviano para que el front (`../Tenis-Ahora-Frontend`) trabaje contra datos
persistidos en lugar de los mocks en memoria. **Es una API de prueba**: una sola base SQLite,
un documento JSON por entidad y sin pretensiones de soportar tráfico real. La versión
definitiva la hará otra persona; el informe `INFORME-API.pdf` (en `D:\fede`) describe el
alcance, los endpoints y las recomendaciones para esa versión.

## Stack

| Pieza | Elección |
|---|---|
| Lenguaje / runtime | Rust 2021, tokio |
| HTTP | axum 0.8 + tower-http (CORS permisivo, trazas, archivos estáticos) |
| Base de datos | SQLite 3.50 vía rusqlite, enlazado contra `lib/sqlite3.dll` (sin compilador C) |
| Auth | bcrypt para contraseñas, JWT HS256 (hmac + sha2) |
| Datos iniciales | `seed/seed.json`, exportado del mock del front |

## Requisitos

- Rust estable (toolchain `x86_64-pc-windows-gnu`, el que instala rustup por defecto en Windows).
- MinGW-w64 (WinLibs) descomprimido en `D:\fede\tools\mingw64` (o la variable `MINGW_BIN`
  apuntando a su carpeta `bin`). `build.ps1` lo pone en el PATH: las crates `windows-sys`
  necesitan `dlltool`/`as` al enlazar y el `ld` que trae rustup no acepta los import libs de
  un `dlltool` más nuevo, así que se usa el toolchain completo. Descarga: https://winlibs.com
  (zip `x86_64-posix-seh ... ucrt`), ~950 MB descomprimido.
- No hace falta compilador C para SQLite: la DLL oficial está en `lib/`.

## Puesta en marcha

```powershell
cd D:\fede\api
.\run.ps1              # compila y levanta en http://localhost:5090/api
.\run.ps1 -Release     # idem, binario optimizado
```

`run.ps1` llama a `build.ps1`, que agrega al PATH el toolchain MinGW antes de `cargo build`.
Desde Git Bash: `export PATH="/d/fede/tools/mingw64/bin:$PATH" && cargo build`. Para dejar todo corriendo en segundo plano (API + front) usar
`D:\fede\start-servidor.ps1`; para detenerlo, `D:\fede\stop-servidor.ps1`.

### Variables de entorno (`.env` opcional, ver `.env.example`)

| Variable | Default | Uso |
|---|---|---|
| `PORT` | `5090` | Puerto HTTP (el front espera `http://localhost:5090/api`). |
| `DATABASE_PATH` | `data/tenis_ahora.sqlite` | Archivo SQLite; se crea solo. |
| `SEED_PATH` | `seed/seed.json` | Datos iniciales; se cargan solo si la base está vacía. |
| `JWT_SECRET` | valor de desarrollo | Clave HS256 de los tokens. |
| `FRONTEND_DIST` | `../Tenis-Ahora-Frontend/dist` | Si existe `index.html` ahí, la API sirve el front en `/`. |
| `RUST_LOG` | `info,tower_http=info` | Nivel de log. |

### Usuarios de prueba

Todos los usuarios del seed tienen contraseña **`123456`**.

| Email | Rol |
|---|---|
| `admin@tenisahora.com` | Empleado (panel de administración) |
| `socio@tenisahora.com` | Socio |
| cualquier otro de `seed/seed.json` → `usuarios` | Socio |

`POST /api/admin/reset` vacía la base y recarga el seed.

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
400 / 401 / 404 / 409 / 500.

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
| POST | `/admin/reset` | — | `{ success, documents }` |
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
(45 verificaciones) y deja la base reseteada al terminar.

## Estructura

```
api/
├── Cargo.toml / build.rs / .cargo/config.toml   # build; enlaza lib/sqlite3.dll
├── lib/sqlite3.dll, sqlite3.def                 # SQLite oficial (sqlite.org, win-x64)
├── seed/seed.json                               # datos iniciales (del mock del front)
├── src/
│   ├── main.rs        # configuración, router, /health, archivos estáticos del front
│   ├── db.rs          # Store: tabla documents (collection, id, data JSON)
│   ├── auth.rs        # login, registro, JWT, middleware
│   ├── crud.rs        # CRUD genérico + reglas por colección
│   ├── business.rs    # endpoints de negocio
│   ├── seed.rs        # carga del seed
│   ├── error.rs       # ApiError -> { error }
│   └── util.rs        # helpers de JSON y fechas
├── tests/smoke.py
├── build.ps1 / run.ps1
└── CHANGELOG.md
```
