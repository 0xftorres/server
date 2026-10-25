package main

// Autorización por rol. El token trae el rol, el DNI y, si es un profesional, su coachId; con eso
// se decide qué puede ver y modificar cada usuario sin volver a leer su ficha en cada request.
//
//   - Empleado: todo.
//   - Socio: el catálogo del club, sus reservas y recibos, inscribirse a sí mismo y pedir clases
//     particulares para sí mismo.
//   - Profesor / Entrenador: el catálogo, su ficha y sus clases. Un profesor solo da clases y un
//     entrenador solo da entrenamientos (campo `kind`).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
)

const (
	rolEmpleado   = "Empleado"
	rolSocio      = "Socio"
	rolProfesor   = "Profesor"
	rolEntrenador = "Entrenador"
)

// catalog son las colecciones que cualquier usuario logueado puede consultar.
var catalog = []string{"canchas", "tipos-cancha", "stock", "torneos", "ligas", "tipos-competencia", "profesores", "clases", "descuentos"}

// staffCollections son las que solo consulta el personal; el resto se filtra por dueño con visible.
var staffCollections = []string{"usuarios", "facturas", "reembolsos"}

func claimsOf(r *http.Request) Claims {
	c, _ := r.Context().Value(claimsKey{}).(Claims)
	return c
}

func (c Claims) staff() bool        { return c.Role == rolEmpleado }
func (c Claims) member() bool       { return c.Role == rolSocio }
func (c Claims) professional() bool { return c.Role == rolProfesor || c.Role == rolEntrenador }

// isMe compara contra el DNI del token; un usuario sin DNI cargado no es dueño de nada.
func (c Claims) isMe(dni string) bool { return c.Dni != "" && dni == c.Dni }

// kindFor devuelve el `kind` que puede dar cada tipo de profesional.
func kindFor(coachType string) string {
	if coachType == rolEntrenador {
		return "entrenamiento"
	}
	return "clase"
}

// El titular de una reserva es el primer participante; a su DNI van los recibos.
func ownsReservation(c Claims, res M) bool {
	ps := arr(res, "participants")
	return len(ps) > 0 && c.isMe(s(asMap(ps[0]), "dni"))
}

func (c Claims) teaches(d M) bool { return c.professional() && s(d, "coachId") == c.CoachID }

var errStaffOnly = forbidden("Esta acción es solo para el personal del club.")

// visible devuelve el documento tal como lo puede ver c, o nil si no lo puede ver.
func visible(c Claims, col string, d M) M {
	if c.staff() {
		return d
	}
	switch col {
	case "clases":
		if c.professional() {
			if c.teaches(d) {
				return d
			}
			return nil
		}
		// Un socio ve la oferta y el cupo, no los datos de los demás alumnos.
		out := maps.Clone(d)
		delete(out, "students")
		return out
	case "reservas":
		if c.member() && ownsReservation(c, d) {
			return d
		}
	case "recibos":
		if c.member() && c.isMe(s(d, "clientDni")) {
			return d
		}
	case "clases-particulares":
		if c.teaches(d) || c.member() && c.isMe(s(d, "studentDni")) {
			return d
		}
	default:
		if slices.Contains(catalog, col) {
			return d
		}
	}
	return nil
}

func visibleAll(c Claims, col string, docs []M) []M {
	out := []M{}
	for _, d := range docs {
		if v := visible(c, col, d); v != nil {
			out = append(out, v)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// CRUD genérico
// ---------------------------------------------------------------------------

// guardCreate decide si c puede dar de alta body en col. A un profesional le fija su coachId y el
// kind que le corresponde, para que no pueda cargar clases a nombre de otro.
func guardCreate(c Claims, col string, body M) error {
	if c.staff() {
		return nil
	}
	switch col {
	case "reservas":
		if c.member() {
			if ownsReservation(c, body) {
				return nil
			}
			return forbidden("Solo podés reservar a tu nombre: el titular tiene que ser tu DNI.")
		}
	case "clases-particulares":
		if c.professional() {
			body["coachId"] = c.CoachID
			body["kind"] = kindFor(c.Role)
			return nil
		}
		if c.member() {
			if c.isMe(s(body, "studentDni")) {
				return nil
			}
			return forbidden("Solo podés pedir clases particulares para vos.")
		}
	case "recibos":
		// Un socio registra desde el portal los recibos de sus propios pagos (inscripciones, cuotas, turnos).
		if c.member() || c.professional() {
			if c.isMe(s(body, "clientDni")) {
				return nil
			}
			return forbidden("Solo podés registrar pagos a tu nombre.")
		}
	}
	return errStaffOnly
}

// guardUpdate decide si c puede modificar existing con body, y saca de body lo que no puede tocar.
func guardUpdate(c Claims, col string, existing, body M) error {
	if c.staff() {
		return nil
	}
	switch col {
	case "clases-particulares":
		if c.teaches(existing) {
			body["coachId"] = c.CoachID
			body["kind"] = kindFor(c.Role)
			return nil
		}
	case "usuarios":
		// Cualquier usuario edita su propio perfil, pero solo nombre, teléfono y foto. El rol, el
		// email, el DNI o los descuentos los cambia el personal: intentarlo es 403, no se ignora.
		if s(existing, "id") == c.Sub {
			for k := range body {
				switch k {
				case "name", "nombre", "apellido", "phone", "photo":
				default:
					return forbidden(fmt.Sprintf("No podés modificar el campo %q de tu usuario; solo nombre, teléfono y foto.", k))
				}
			}
			return nil
		}
	case "profesores":
		if c.professional() && s(existing, "id") == c.CoachID {
			// El tipo y la certificación los valida el club, no el profesional.
			for _, k := range []string{"type", "licenseDetails", "hasOfficialLicense"} {
				delete(body, k)
			}
			return nil
		}
	}
	return errStaffOnly
}

func guardDelete(c Claims, col string, existing M) error {
	if c.staff() || col == "clases-particulares" && c.teaches(existing) {
		return nil
	}
	return errStaffOnly
}

// checkCoachKind valida que una clase o clase particular respete el tipo de su profesional.
func (a *app) checkCoachKind(d M) error {
	coach, ok, err := a.store.get("profesores", s(d, "coachId"))
	if err != nil || !ok {
		return err
	}
	if want := kindFor(s(coach, "type")); s(d, "kind") != want {
		return badRequest(fmt.Sprintf("%s es %s: solo puede tener %ss.", s(coach, "name"), strings.ToLower(s(coach, "type")), want))
	}
	return nil
}

// checkCoachUser exige que un usuario profesional apunte a una ficha de profesional existente.
func (a *app) checkCoachUser(u M) error {
	if s(u, "role") != "coach" {
		delete(u, "coachId")
		return nil
	}
	if s(u, "coachId") == "" {
		return badRequest("Un usuario profesional tiene que estar vinculado a una ficha de profesional.")
	}
	_, err := a.store.require("profesores", s(u, "coachId"))
	return err
}

// ---------------------------------------------------------------------------
// Rutas de negocio
// ---------------------------------------------------------------------------

// guard antepone un chequeo de permisos al handler.
func guard(check func(r *http.Request, c Claims) error, fn apiFunc) apiFunc {
	return func(r *http.Request) (int, any, error) {
		if err := check(r, claimsOf(r)); err != nil {
			return 0, nil, err
		}
		return fn(r)
	}
}

func staffOnly(_ *http.Request, c Claims) error {
	if c.staff() {
		return nil
	}
	return errStaffOnly
}

// peekJSON lee el cuerpo sin consumirlo, para que el handler lo pueda volver a leer.
func peekJSON(r *http.Request) M {
	raw, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	var m M
	json.Unmarshal(raw, &m)
	return m
}

// selfEnrollment: un socio solo se puede inscribir a sí mismo (torneos y ligas).
func selfEnrollment(r *http.Request, c Claims) error {
	if c.staff() || c.member() && c.isMe(s(peekJSON(r), "dni")) {
		return nil
	}
	if c.member() {
		return forbidden("Solo podés inscribirte a vos mismo.")
	}
	return errStaffOnly
}

func (a *app) reservationOwner(r *http.Request, c Claims) error {
	if c.staff() {
		return nil
	}
	res, err := a.store.require("reservas", r.PathValue("id"))
	if err != nil {
		return err
	}
	if c.member() && ownsReservation(c, res) {
		return nil
	}
	return forbidden("La reserva no es tuya.")
}

// classTeacher: el personal o el profesional a cargo de la clase.
func (a *app) classTeacher(r *http.Request, c Claims) error {
	if c.staff() {
		return nil
	}
	cls, err := a.store.require("clases", r.PathValue("id"))
	if err != nil {
		return err
	}
	if c.teaches(cls) {
		return nil
	}
	return forbidden("La clase no está a tu cargo.")
}

// classEnrollment: además del personal y el profesional a cargo, un socio se puede anotar a sí mismo.
func (a *app) classEnrollment(r *http.Request, c Claims) error {
	if c.member() {
		if c.isMe(s(peekJSON(r), "dni")) {
			return nil
		}
		return forbidden("Solo podés inscribirte a vos mismo.")
	}
	return a.classTeacher(r, c)
}

func (a *app) privateLessonOwner(r *http.Request, c Claims) error {
	if c.staff() {
		return nil
	}
	p, err := a.store.require("clases-particulares", r.PathValue("id"))
	if err != nil {
		return err
	}
	if visible(c, "clases-particulares", p) != nil {
		return nil
	}
	return forbidden("La clase particular no es tuya.")
}

func coachSelf(r *http.Request, c Claims) error {
	if c.staff() || c.professional() && r.PathValue("id") == c.CoachID {
		return nil
	}
	return errStaffOnly
}

func membersAndStaff(_ *http.Request, c Claims) error {
	if c.staff() || c.member() {
		return nil
	}
	return forbidden("Esta sección es solo para socios.")
}

func reservationCreate(r *http.Request, c Claims) error {
	return guardCreate(c, "reservas", peekJSON(r))
}
