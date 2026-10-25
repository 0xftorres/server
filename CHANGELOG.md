# Changelog — Tenis Ahora API de prueba

Formato: [Keep a Changelog](https://keepachangelog.com/es/1.1.0/). Versionado semántico.

## [Sin publicar]

### Corregido
- Un usuario que edita su propio perfil solo puede cambiar nombre, teléfono y foto: cualquier otro
  campo (`role`, `email`, `dni`, descuentos) responde 403 en vez de ignorarse.

### Eliminado
- Datos de profesionales que el TP no pide: `specialty`, `experienceYears`, `bio`, `photo` y la
  tarifa por docente `hourlyRatePrivate`. La clase particular se cobra a la tarifa única del club
  (`privateLessonHourlyRate` en `GET /api/configuracion`).
- `PUT /api/configuracion`: los plazos de reserva y cancelación los fija el club y el sistema solo
  los consulta (`GET`).
- Estado de asistencia `J` (justificado): la asistencia se registra como P o A y los endpoints
  rechazan otros valores con 400.
- Umbral mínimo de alerta de stock (`minAlertThreshold`, estado `alerta_baja`): el estado pasa a
  ser `disponible` / `agotado`.
- `POST /api/facturas/{id}/anular`, tipo de factura (A/B/C) y alícuota de IVA: la factura se numera
  `FC-0001-…` y su total es la suma de los ítems.
- `PATCH /api/reembolsos/{id}/estado` y el campo `status` de los reembolsos.

### Agregado
- Roles Profesor y Entrenador: usuarios vinculados a una ficha de `profesores`. Un profesor solo
  tiene clases y un entrenador solo entrenamientos (`kind`), también cuando las carga el admin.
- `GET /api/reservas/ocupacion`: turnos tomados, sin datos de quién reservó.
- `GET /api/socios`: padrón reducido (id, nombre, DNI, N° de socio) para elegir compañeros al
  reservar. Lo consultan socios y personal.
- El token incluye `dni` y `coachId`; el login también devuelve `coachId`.
- `GET /api/configuracion`: plazo de cancelación sin cargo, retención por cancelación tardía y
  antelación máxima de reserva (RD02–RD05): 6 hs / 50% / 30 días.
- Alta y cambio de fecha de reservas: 400 si la fecha supera la antelación máxima (RD05).
- `POST /api/documentos` y `GET /api/documentos/{id}`: archivos PDF, JPG o PNG de hasta 5 MB,
  guardados en base64 en la misma base. Torneos y ligas guardan el reglamento adjunto en
  `internalRulesDocument` (RF041, RF048) y las fichas de docentes el título en `licenseDetails.document`.
- Clases, entrenamientos y turnos particulares: 400 si el docente no tiene el título habilitante
  vigente, habilitado y con documento (RD09). Se valida al crear y al cambiar de docente.

### Cambiado
- Permisos por rol en todas las rutas (`authz.go`). Antes cualquier usuario logueado podía
  modificar todo. Un socio ve solo sus reservas y recibos y solo se inscribe a sí mismo; un
  profesional ve y gestiona solo sus clases y su ficha.
- `POST /api/reservas/{id}/cancelar` calcula las horas de antelación con la fecha y hora de inicio
  de la reserva; `hoursInAdvance` del cuerpo se ignora. Antes el plazo (6 hs) y la retención (50%)
  estaban fijos y las horas las informaba quien cancelaba.
- Sin medio de pago informado, el pago de saldo queda en Mercado Pago (QR) y la factura en Tarjeta
  de Débito (antes Efectivo, que no está entre los medios de RD11).
- `tests/smoke.py` usa fechas relativas a hoy para las reservas.
- Alta de docentes: exige título, institución, licencia, vencimiento y documento (RD08). Antes se
  completaban con valores inventados y un nombre de PDF ficticio (`digitalDocumentUrl`, que se quita).
- El seed trae un PDF de muestra por docente, en la colección `documentos`.
- `PUT /reglamento` ya no borra el texto si el cuerpo no trae `internalRules`.

## [2.0.0] — 2026-10-23

### Agregado
- Deploy con Docker: `ci/Dockerfile` y `.github/workflows/deploy.yml` (build en GitHub Actions,
  copia de la imagen por SSH al VPS y contenedor en `caddy-net` detrás de Caddy en
  `tenisahora-api.duckdns.org`).

### Cambiado
- La API se reescribió en Go (`net/http` de la librería estándar + `modernc.org/sqlite`). Se
  mantienen las rutas, los cuerpos de respuesta, los mensajes de error y el esquema de la base;
  una base creada por la versión Rust sigue siendo válida (incluidos los hashes bcrypt).
- El build ya no necesita MinGW ni `lib/sqlite3.dll`: SQLite es Go puro (sin cgo).
- Un cuerpo JSON inválido o con tipos incorrectos responde 400 con `{ error }` (antes 415/422
  de axum).
- `POST /api/admin/reset` exige rol Empleado; con otro rol responde 403.
- El log de requests es fijo (método, ruta, status, duración); `RUST_LOG` ya no se usa.

### Quitado
- Código Rust (`src/`, `Cargo.toml`, `Cargo.lock`, `build.rs`, `.cargo/`), `lib/sqlite3.dll`
  y los scripts `build.ps1` / `run.ps1`.

## [1.0.0] — 2026-10-23

### Agregado
- Scripts `D:\fede\start-servidor.ps1` / `stop-servidor.ps1`: compilan API y front, levantan la
  API en segundo plano sirviendo el front y muestran las URLs locales y por `nip.io`.
- `tests/smoke.py`: prueba de humo de punta a punta (45 verificaciones).
- Informe final `INFORME-API.pdf` con versiones, endpoints, modelo de datos y recomendaciones.

### Cambiado
- La API responde `index.html` con 200 para las rutas de la SPA (antes 404 con el contenido).
- Build: se usa el toolchain MinGW-w64 completo de `D:\fede\tools\mingw64` (el `ld` de rustup
  no acepta los import libs del `dlltool` moderno que exigen las crates `windows-sys`).

### Verificado
- `tests/smoke.py`: 45/45 verificaciones OK, en local y a través de `http://<ip>.nip.io:5090`.
- Front servido por la API y operando contra datos persistidos desde el navegador (login real,
  altas que sobreviven a la recarga, cancelación con cargo y reembolso, Modo Demo intacto).
- Build en Windows sin compilador C para SQLite (DLL oficial 3.50.4).

## [0.4.0] — 2026-10-23

### Agregado
- La API sirve el build del front (`FRONTEND_DIST`) con fallback a `index.html` para las rutas
  de react-router.
- Front: `src/services/httpApi.js` (servicios HTTP) y `withBackend()` en `src/services/api.js`
  (HTTP si `/api/health` responde y la sesión no es Modo Demo; mocks en caso contrario).
- `AuthContext` acepta `dni` y `memberNumber` del `AuthResponseDto` y el rol `Profesor`.

## [0.3.0] — 2026-10-23

### Agregado
- Endpoints de negocio: estado y disponibilidad de canchas, ajuste de stock, alta de reserva
  con recibo de seña, pago de saldo, cancelación con reembolso o cargo (RD02–RD04, RD06),
  reglamentos, inscripciones y enfrentamientos de torneos y ligas, resultado de liga con tabla,
  disponibilidad y certificación de docentes, alumnos y asistencia de clases (RD07, RD10),
  cancelación de turnos particulares, anulación de facturas, estado de reembolsos, descuentos
  por usuario y `POST /admin/reset`.

## [0.2.0] — 2026-10-23

### Agregado
- CRUD genérico `GET/POST /api/{col}` y `GET/PUT/DELETE /api/{col}/{id}` para las 15
  colecciones, con generación de ids con los mismos prefijos que el front y las reglas de
  alta/modificación/baja del mock (unicidad, cupos, recálculos, cascadas).
- Ocultamiento de `passwordHash` en todas las respuestas de `usuarios`.

## [0.1.0] — 2026-10-23

### Agregado
- Proyecto axum + tokio; `Store` sobre SQLite (tabla `documents`, WAL) enlazando la DLL
  oficial de SQLite 3.50.4 (sin `bundled`, sin compilador C).
- Seed automático desde `seed/seed.json` cuando la base está vacía; contraseña `123456`
  para todos los usuarios del seed.
- Autenticación: `POST /auth/login`, `POST /auth/registrar`, `GET /auth/yo`; JWT HS256 de 8 h;
  middleware `Bearer` en todas las rutas protegidas.
- `GET /health` con versión, versión de SQLite y conteo de documentos.
- Errores uniformes `{ "error": "..." }`.
