// Tenis Ahora — API de prueba (Go + net/http + SQLite).
//
// Servidor liviano para que el front (`Tenis-Ahora-Frontend`) trabaje contra datos
// persistidos en lugar de los mocks en memoria. No está pensado para producción: ver
// INFORME-API.pdf para el alcance y las recomendaciones para la versión definitiva.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const version = "2.0.0"

type app struct {
	store     *Store
	jwtSecret string
	seedPath  string
	startedAt time.Time
}

// apiFunc devuelve el status y el cuerpo a serializar, o un error que se responde como `{ error }`.
type apiFunc func(r *http.Request) (int, any, error)

func api(fn apiFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, body, err := fn(r)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, status, body)
	}
}

func decodeBody(r *http.Request, dst any) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return badRequest("Cuerpo JSON inválido: " + err.Error())
	}
	return nil
}

// readJSON decodifica el cuerpo; si no es un objeto lo trata como vacío.
func readJSON(r *http.Request) (M, error) {
	var v any
	if err := decodeBody(r, &v); err != nil {
		return nil, err
	}
	if m, ok := v.(M); ok {
		return m, nil
	}
	return M{}, nil
}

// readObject decodifica el cuerpo y exige que sea un objeto.
func readObject(r *http.Request) (M, error) {
	var v any
	if err := decodeBody(r, &v); err != nil {
		return nil, err
	}
	m, ok := v.(M)
	if !ok {
		return nil, badRequest("El cuerpo debe ser un objeto JSON.")
	}
	return m, nil
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return os.Getenv(key)
	}
	return def
}

// loadDotEnv carga `.env` si existe, sin pisar variables ya definidas en el entorno.
func loadDotEnv(file string) {
	fh, err := os.Open(file)
	if err != nil {
		return
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, value)
		}
	}
}

// GET /api/health — público; el front lo usa para decidir si trabaja contra la API o con mocks.
func (a *app) health(r *http.Request) (int, any, error) {
	counts, err := a.store.counts()
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{
		"status":        "ok",
		"service":       "tenis-ahora-api",
		"version":       version,
		"sqlite":        a.store.sqliteVersion,
		"uptimeSeconds": int64(time.Since(a.startedAt).Seconds()),
		"collections":   counts,
	}, nil
}

// cors replica `CorsLayer::permissive()`: cualquier origen, método y header.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Expose-Headers", "*")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Set("Access-Control-Allow-Methods", "*")
			h.Set("Access-Control-Allow-Headers", "*")
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Microsecond))
	})
}

// spa sirve los archivos de dist/ tal cual; cualquier otra ruta devuelve index.html con 200, así
// funcionan las rutas de react-router al recargar o al entrar por un link directo.
func spa(dist string) http.HandlerFunc {
	index := filepath.Join(dist, "index.html")
	return func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + r.URL.Path)
		file := filepath.Join(dist, filepath.FromSlash(clean))
		if info, err := os.Stat(file); err == nil && !info.IsDir() {
			http.ServeFile(w, r, file)
			return
		}
		// Un asset faltante es un 404 real, no una ruta de la SPA.
		if strings.HasPrefix(clean, "/assets/") {
			http.NotFound(w, r)
			return
		}
		data, err := os.ReadFile(index)
		if err != nil {
			http.Error(w, "index.html no encontrado", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	}
}

func (a *app) routes(frontendDist string) http.Handler {
	mux := http.NewServeMux()
	public := func(pattern string, fn apiFunc) { mux.HandleFunc(pattern, api(fn)) }
	protected := func(pattern string, fn apiFunc) { mux.HandleFunc(pattern, a.requireAuth(api(fn))) }

	public("GET /api/health", a.health)
	public("POST /api/auth/login", a.login)
	public("POST /api/auth/registrar", a.register)

	protected("GET /api/auth/yo", a.me)
	protected("POST /api/admin/reset", a.reset)
	// Canchas y stock
	protected("PATCH /api/canchas/{id}/estado", guard(staffOnly, a.courtStatus))
	protected("PUT /api/canchas/{id}/disponibilidad", guard(staffOnly, a.courtAvailability))
	protected("PATCH /api/stock/{id}/ajuste", guard(staffOnly, a.stockAdjust))
	// Documentos: los sube el personal; los descarga cualquier usuario logueado (reglamentos, títulos).
	protected("POST /api/documentos", guard(staffOnly, a.uploadDocument))
	mux.HandleFunc("GET /api/documentos/{id}", a.requireAuth(a.downloadDocument))
	// Configuración del club: plazos de reserva y cancelación fijados por el club (solo consulta).
	protected("GET /api/configuracion", a.getClubConfig)
	// Reservas: alta pendiente de pago -> seña -> inicio del alquiler -> saldo
	protected("GET /api/reservas", guard(membersAndStaff, a.listReservations))
	protected("GET /api/reservas/ocupacion", guard(membersAndStaff, a.reservationsOccupancy))
	protected("POST /api/reservas", guard(reservationCreate, a.createReservation))
	protected("POST /api/reservas/{id}/pagar-sena", guard(a.reservationOwner, a.payDeposit))
	protected("POST /api/reservas/{id}/iniciar", guard(staffOnly, a.startRental))
	protected("POST /api/reservas/{id}/pagar-saldo", guard(staffOnly, a.payRemaining))
	protected("POST /api/reservas/{id}/cancelar", guard(a.reservationOwner, a.cancelReservation))
	// Pagos: pasarela simulada (débito, crédito y QR de Mercado Pago con tarjetas de prueba)
	protected("POST /api/pagos", guard(membersAndStaff, a.createPayment))
	// Torneos
	protected("PUT /api/torneos/{id}/reglamento", guard(staffOnly, a.tournamentRules))
	protected("POST /api/torneos/{id}/inscripciones", guard(selfEnrollment, a.tournamentEnroll))
	protected("DELETE /api/torneos/{id}/inscripciones/{pid}", guard(staffOnly, a.tournamentUnenroll))
	protected("POST /api/torneos/{id}/enfrentamientos", guard(staffOnly, a.tournamentAddMatch))
	protected("PUT /api/torneos/{id}/enfrentamientos/{mid}", guard(staffOnly, a.tournamentUpdateMatch))
	// Ligas
	protected("PUT /api/ligas/{id}/reglamento", guard(staffOnly, a.leagueRules))
	protected("POST /api/ligas/{id}/inscripciones", guard(selfEnrollment, a.leagueEnroll))
	protected("DELETE /api/ligas/{id}/inscripciones/{pid}", guard(staffOnly, a.leagueUnenroll))
	protected("POST /api/ligas/{id}/enfrentamientos", guard(staffOnly, a.leagueAddMatch))
	protected("PUT /api/ligas/{id}/enfrentamientos/{mid}", guard(staffOnly, a.leagueUpdateMatch))
	protected("POST /api/ligas/{id}/enfrentamientos/{mid}/resultado", guard(staffOnly, a.leagueMatchScore))
	// Profesores, clases y entrenamientos
	protected("PUT /api/profesores/{id}/disponibilidad", guard(coachSelf, a.coachAvailability))
	protected("PUT /api/profesores/{id}/certificacion", guard(staffOnly, a.coachLicense))
	protected("POST /api/clases/{id}/alumnos", guard(a.classEnrollment, a.classEnroll))
	protected("DELETE /api/clases/{id}/alumnos/{sid}", guard(a.classTeacher, a.classUnenroll))
	protected("POST /api/clases/{id}/asistencia/alternar", guard(a.classTeacher, a.classToggleAttendance))
	protected("POST /api/clases/{id}/asistencia/sesion", guard(a.classTeacher, a.classBulkAttendance))
	protected("POST /api/clases/{id}/asistencia/registrar", guard(a.classTeacher, a.classRecordAttendance))
	protected("POST /api/clases-particulares/{id}/cancelar", guard(a.privateLessonOwner, a.privateCancel))
	// Caja
	protected("PUT /api/usuarios/{id}/descuentos", guard(staffOnly, a.userDiscounts))
	protected("GET /api/socios", guard(membersAndStaff, a.listMembers))
	// CRUD genérico: los permisos por colección están en guardCreate / guardUpdate / guardDelete y visible.
	// CRUD genérico
	protected("GET /api/{collection}", a.list)
	protected("POST /api/{collection}", a.create)
	protected("GET /api/{collection}/{id}", a.getOne)
	protected("PUT /api/{collection}/{id}", a.update)
	protected("DELETE /api/{collection}/{id}", a.remove)

	// Opcional: servir el build del front (SPA) desde el mismo puerto.
	if frontendDist != "" {
		mux.HandleFunc("GET /", spa(frontendDist))
	}

	return cors(logRequests(mux))
}

func main() {
	loadDotEnv(".env")

	port, err := strconv.ParseUint(envOr("PORT", "5090"), 10, 16)
	if err != nil {
		port = 5090
	}
	dbPath := envOr("DATABASE_PATH", "data/tenis_ahora.sqlite")
	seedPath := envOr("SEED_PATH", "seed/seed.json")
	jwtSecret := envOr("JWT_SECRET", "tenis-ahora-dev-secret-cambiar-en-produccion")

	store, err := openStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No se pudo abrir la base de datos %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	switch n, err := seedIfEmpty(store, seedPath); {
	case err != nil:
		log.Printf("WARN No se pudo cargar el seed: %v", err)
	case n == 0:
		log.Printf("Base de datos existente en %s (sin recarga de seed)", dbPath)
	default:
		log.Printf("Base vacía: se cargaron %d documentos desde %s. Usuarios con contraseña %q.", n, seedPath, defaultPassword)
	}

	frontendDist := envOr("FRONTEND_DIST", "../Tenis-Ahora-Frontend/dist")
	if _, err := os.Stat(filepath.Join(frontendDist, "index.html")); err == nil {
		log.Printf("Sirviendo el front desde %s", frontendDist)
	} else {
		log.Printf("Sin build del front en %s (solo API). Generarlo con `npm run build` en el front.", frontendDist)
		frontendDist = ""
	}

	a := &app{store: store, jwtSecret: jwtSecret, seedPath: seedPath, startedAt: time.Now()}

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No se pudo escuchar en %s: %v\n", addr, err)
		os.Exit(1)
	}
	log.Printf("Tenis Ahora API v%s escuchando en http://localhost:%d/api  (health: /api/health)", version, port)
	if err := http.Serve(ln, a.routes(frontendDist)); err != nil {
		fmt.Fprintf(os.Stderr, "El servidor terminó con error: %v\n", err)
	}
}
