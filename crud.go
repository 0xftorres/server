package main

// CRUD genérico por colección (`/api/{coleccion}` y `/api/{coleccion}/{id}`) con las reglas de
// alta/modificación/baja que tenía el mock del front (`src/services/api.js`).

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
)

func validateCollection(col string) error {
	if slices.Contains(collections, col) {
		return nil
	}
	return notFound(fmt.Sprintf("La colección `%s` no existe. Disponibles: %s", col, strings.Join(collections, ", ")))
}

// sanitize oculta campos sensibles antes de responder.
func sanitize(col string, doc M) M {
	if col == "usuarios" {
		delete(doc, "passwordHash")
	}
	return doc
}

// ---------------------------------------------------------------------------
// Generación de identificadores (mismos prefijos que el front)
// ---------------------------------------------------------------------------
func (a *app) nextID(col string, doc M) (string, error) {
	ms := nowMillis()
	count, err := a.store.count(col)
	if err != nil {
		return "", err
	}
	var base string
	switch col {
	case "usuarios":
		base = fmt.Sprintf("usr-%d", ms)
	case "canchas":
		base = fmt.Sprintf("cancha-%d", ms)
	case "tipos-cancha":
		base = fmt.Sprintf("ct-%d", ms)
	case "stock":
		base = fmt.Sprintf("stock-%d", ms)
	case "torneos":
		base = fmt.Sprintf("trn-%d", ms)
	case "ligas":
		base = fmt.Sprintf("liga-%d", ms)
	case "tipos-competencia":
		base = fmt.Sprintf("cmp-%d", ms)
	case "profesores":
		base = fmt.Sprintf("coach-%d", ms)
	case "clases":
		base = fmt.Sprintf("cls-%d", ms)
	case "clases-particulares":
		base = fmt.Sprintf("prv-%d", ms)
	case "descuentos":
		base = fmt.Sprintf("disc-%d", ms)
	case "reservas":
		base = fmt.Sprintf("RES-%d-%03d", year(), count+1)
	case "recibos":
		base = fmt.Sprintf("REC-%d-%d", year(), 1050+count)
	case "facturas":
		base = fmt.Sprintf("FC-%s-0001-%08d", orDefault(s(doc, "type"), "B"), 412+count+1)
	case "reembolsos":
		base = fmt.Sprintf("RMB-%d-%04d", year(), count+9)
	default:
		base = fmt.Sprintf("%s-%d", col, ms)
	}
	id := base
	for n := 1; ; {
		taken, err := a.store.exists(col, id)
		if err != nil {
			return "", err
		}
		if !taken {
			return id, nil
		}
		n++
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

func defaultStr(d M, key, value string) {
	if s(d, key) == "" {
		d[key] = value
	}
}

func defaultVal(d M, key string, value any) {
	if v, ok := d[key]; !ok || v == nil {
		d[key] = value
	}
}

// anyOther informa si algún documento de la colección (salvo exceptID) cumple match.
func (a *app) anyOther(col, exceptID string, match func(M) bool) (bool, error) {
	docs, err := a.store.list(col)
	if err != nil {
		return false, err
	}
	for _, d := range docs {
		if (exceptID == "" || s(d, "id") != exceptID) && match(d) {
			return true, nil
		}
	}
	return false, nil
}

func stockStatus(available, min float64) string {
	if available <= min {
		return "alerta_baja"
	}
	return "normal"
}

func capTo30(x float64) float64 {
	if x <= 0 {
		return 30
	}
	return math.Min(x, 30)
}

// ---------------------------------------------------------------------------
// Reglas de alta
// ---------------------------------------------------------------------------
func (a *app) applyCreateDefaults(col string, d M) (M, error) {
	if s(d, "id") == "" {
		id, err := a.nextID(col, d)
		if err != nil {
			return nil, err
		}
		d["id"] = id
	}

	switch col {
	case "usuarios":
		email := strings.ToLower(strings.TrimSpace(s(d, "email")))
		if email == "" {
			return nil, badRequest("El email es obligatorio.")
		}
		users, err := a.store.list(col)
		if err != nil {
			return nil, err
		}
		admins := 0
		for _, u := range users {
			if strings.ToLower(s(u, "email")) == email {
				return nil, conflict("Ya existe un usuario registrado con ese email.")
			}
			if s(u, "role") == "admin" {
				admins++
			}
		}
		d["email"] = email
		defaultStr(d, "role", "client")
		if s(d, "memberNumber") == "" {
			if s(d, "role") == "admin" {
				d["memberNumber"] = fmt.Sprintf("ADM-%03d", admins+1)
			} else {
				d["memberNumber"] = fmt.Sprintf("TA-%d", 1000+nowMillis()%9000)
			}
		}
		defaultStr(d, "memberSince", today())
		defaultVal(d, "activeDiscounts", []any{})
		defaultVal(d, "totalBookings", 0)
		plain := s(d, "password")
		delete(d, "password")
		if s(d, "passwordHash") == "" {
			d["passwordHash"] = hashPassword(orDefault(plain, defaultPassword))
		}
	case "canchas":
		defaultStr(d, "status", "disponible")
		defaultStr(d, "maintenanceNotes", "")
		defaultVal(d, "schedule", []any{})
		defaultVal(d, "availability", defaultAvailability())
		defaultStr(d, "photo", "https://images.unsplash.com/photo-1595435934249-5df7ed86e1c0?auto=format&fit=crop&w=600&q=80")
	case "tipos-cancha":
		name := strings.TrimSpace(s(d, "name"))
		if name == "" {
			return nil, badRequest("El nombre del tipo de cancha es obligatorio.")
		}
		taken, err := a.anyOther(col, "", func(t M) bool { return strings.EqualFold(s(t, "name"), name) })
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, conflict("Ya existe un tipo de cancha con ese nombre.")
		}
		d["name"] = name
		defaultStr(d, "label", name)
		defaultStr(d, "description", "")
		defaultStr(d, "speed", "Media")
		defaultStr(d, "maintenanceFrequency", "")
		d["basePricePerHour"] = f(d, "basePricePerHour")
		d["maxStudentsPerClass"] = capTo30(f(d, "maxStudentsPerClass"))
		defaultVal(d, "active", true)
	case "stock":
		total := f(d, "totalStock")
		inUse := f(d, "inUseStock")
		available := math.Max(total-inUse, 0)
		min := f(d, "minAlertThreshold")
		if min <= 0 {
			min = 1
		}
		defaultStr(d, "category", "Equipamiento")
		defaultStr(d, "description", "")
		defaultStr(d, "unit", "unidades")
		d["totalStock"] = total
		d["inUseStock"] = inUse
		d["availableStock"] = available
		d["minAlertThreshold"] = min
		d["status"] = stockStatus(available, min)
	case "torneos":
		defaultVal(d, "currentEnrolled", 0)
		defaultStr(d, "status", "inscripcion_abierta")
		defaultStr(d, "internalRules", "Reglamento Interno: Partidos al mejor de 3 sets con tie-break oficial.")
		defaultVal(d, "participants", []any{})
		defaultVal(d, "bracket", []any{})
	case "ligas":
		defaultVal(d, "currentEnrolled", 0)
		defaultStr(d, "status", "inscripcion_abierta")
		defaultStr(d, "internalRules", "Reglamento Interno: Sistema todos contra todos por puntos.")
		defaultVal(d, "participants", []any{})
		defaultVal(d, "standings", []any{})
		defaultVal(d, "fixtures", []any{})
	case "tipos-competencia":
		if strings.TrimSpace(s(d, "name")) == "" {
			return nil, badRequest("El nombre del tipo de competencia es obligatorio.")
		}
		defaultStr(d, "format", "")
		defaultStr(d, "appliesTo", "Torneo")
		defaultStr(d, "scoringSystem", "")
		defaultStr(d, "description", "")
		if v := f(d, "minParticipants"); v <= 0 {
			d["minParticipants"] = 2
		} else {
			d["minParticipants"] = v
		}
		if v := f(d, "maxParticipants"); v <= 0 {
			d["maxParticipants"] = 32
		} else {
			d["maxParticipants"] = v
		}
		defaultVal(d, "active", true)
	case "profesores":
		defaultVal(d, "hasOfficialLicense", true)
		lic := merge(M{
			"titleName":     "Profesor de Tenis Certificado",
			"institution":   "Asociación Argentina de Tenis (AAT)",
			"licenseNumber": fmt.Sprintf("AAT-LIC-%d", 10000+rand4()*7%90000),
			"issueDate":     today(),
			"expiryDate":    "2029-12-31",
			"status":        "habilitado",
		}, asMap(d["licenseDetails"]))
		if s(lic, "digitalDocumentUrl") == "" {
			lic["digitalDocumentUrl"] = fmt.Sprintf("Certificado_%s.pdf", s(lic, "licenseNumber"))
		}
		d["licenseDetails"] = lic
		defaultStr(d, "type", "Profesor")
		defaultStr(d, "photo", "https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?auto=format&fit=crop&w=400&q=80")
	case "clases":
		defaultStr(d, "kind", "clase")
		defaultStr(d, "type", "grupal")
		capacity := f(d, "maxCapacity")
		if capacity > 30 {
			return nil, badRequest("El cupo reglamentario máximo es de hasta 30 alumnos.")
		}
		d["maxCapacity"] = capTo30(capacity)
		defaultVal(d, "currentEnrolled", 0)
		defaultVal(d, "students", []any{})
		defaultVal(d, "sessions", []any{"02/10", "04/10", "09/10", "11/10", "16/10", "18/10", "23/10", "25/10"})
	case "clases-particulares":
		defaultStr(d, "kind", "clase")
		defaultStr(d, "status", "programada")
	case "recibos":
		defaultStr(d, "date", nowStamp())
		defaultStr(d, "status", "completado")
		defaultStr(d, "cashierName", "Administración General")
	case "facturas":
		return computeInvoice(d)
	case "reembolsos":
		return computeRefund(d)
	case "descuentos":
		code := strings.ToUpper(strings.TrimSpace(s(d, "code")))
		if code == "" {
			return nil, badRequest("El código del descuento es obligatorio.")
		}
		taken, err := a.anyOther(col, "", func(x M) bool { return s(x, "code") == code })
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, conflict("Ya existe un descuento con ese código.")
		}
		d["code"] = code
		d["discountPercent"] = math.Min(math.Max(f(d, "discountPercent"), 0), 100)
		defaultStr(d, "applicableTo", "Alquiler de Canchas")
		defaultVal(d, "active", true)
	}
	return d, nil
}

// ---------------------------------------------------------------------------
// Reglas de modificación
// ---------------------------------------------------------------------------
func (a *app) applyUpdateRules(col string, existing, patch M) (M, error) {
	id := s(existing, "id")
	d := merge(existing, patch)
	d["id"] = id // el id no se cambia por PUT

	switch col {
	case "usuarios":
		plain := s(d, "password")
		delete(d, "password")
		if plain != "" {
			d["passwordHash"] = hashPassword(plain)
		}
		if s(d, "passwordHash") == "" {
			d["passwordHash"] = existing["passwordHash"]
		}
		email := strings.ToLower(strings.TrimSpace(s(d, "email")))
		taken, err := a.anyOther(col, id, func(u M) bool { return strings.ToLower(s(u, "email")) == email })
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, conflict("Ya existe otro usuario con ese email.")
		}
		d["email"] = email
	case "stock":
		total := f(d, "totalStock")
		inUse := f(d, "inUseStock")
		available := math.Max(total-inUse, 0)
		min := f(d, "minAlertThreshold")
		d["totalStock"] = total
		d["inUseStock"] = inUse
		d["availableStock"] = available
		d["minAlertThreshold"] = min
		d["status"] = stockStatus(available, min)
	case "tipos-cancha":
		name := strings.TrimSpace(s(d, "name"))
		taken, err := a.anyOther(col, id, func(t M) bool { return strings.EqualFold(s(t, "name"), name) })
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, conflict("Ya existe otro tipo de cancha con ese nombre.")
		}
		d["basePricePerHour"] = f(d, "basePricePerHour")
		d["maxStudentsPerClass"] = capTo30(f(d, "maxStudentsPerClass"))
	case "clases":
		capacity := f(d, "maxCapacity")
		if capacity > 30 {
			return nil, badRequest("El cupo reglamentario máximo es de hasta 30 alumnos.")
		}
		if capacity <= 0 {
			d["maxCapacity"] = 30
		}
	case "descuentos":
		code := strings.ToUpper(strings.TrimSpace(s(d, "code")))
		taken, err := a.anyOther(col, id, func(x M) bool { return s(x, "code") == code })
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, conflict("Ya existe otro descuento con ese código.")
		}
		d["code"] = code
		d["discountPercent"] = math.Min(math.Max(f(d, "discountPercent"), 0), 100)
	case "reservas":
		return a.recalculateReservation(existing, patch)
	}
	return d, nil
}

// ---------------------------------------------------------------------------
// Reglas de baja
// ---------------------------------------------------------------------------
func (a *app) checkDelete(col string, doc M) error {
	switch col {
	case "tipos-cancha":
		name := s(doc, "name")
		used, err := a.anyOther("canchas", "", func(c M) bool { return s(c, "surfaceType") == name })
		if err != nil {
			return err
		}
		if used {
			return conflict(fmt.Sprintf("No se puede eliminar: existen canchas registradas con superficie \"%s\".", name))
		}
	case "descuentos":
		// Cascada: quitar el código de los usuarios que lo tenían asignado.
		code := s(doc, "code")
		users, err := a.store.list("usuarios")
		if err != nil {
			return err
		}
		for _, u := range users {
			codes := arr(u, "activeDiscounts")
			if !slices.Contains(codes, any(code)) {
				continue
			}
			next := []any{}
			for _, c := range codes {
				if c != code {
					next = append(next, c)
				}
			}
			u["activeDiscounts"] = next
			if err := a.store.upsert("usuarios", u); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------
func (a *app) listCollection(col string) (int, any, error) {
	if err := validateCollection(col); err != nil {
		return 0, nil, err
	}
	docs, err := a.store.list(col)
	if err != nil {
		return 0, nil, err
	}
	for _, d := range docs {
		sanitize(col, d)
	}
	return http.StatusOK, docs, nil
}

func (a *app) list(r *http.Request) (int, any, error) {
	return a.listCollection(r.PathValue("collection"))
}

func (a *app) getOne(r *http.Request) (int, any, error) {
	col := r.PathValue("collection")
	if err := validateCollection(col); err != nil {
		return 0, nil, err
	}
	doc, err := a.store.require(col, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, sanitize(col, doc), nil
}

func (a *app) create(r *http.Request) (int, any, error) {
	col := r.PathValue("collection")
	if err := validateCollection(col); err != nil {
		return 0, nil, err
	}
	body, err := readObject(r)
	if err != nil {
		return 0, nil, err
	}
	if col == "reservas" {
		// Las reservas tienen alta propia (genera recibo de seña): ver createReservationInner.
		res, err := a.createReservationInner(body)
		if err != nil {
			return 0, nil, err
		}
		return http.StatusCreated, res, nil
	}
	doc, err := a.applyCreateDefaults(col, body)
	if err != nil {
		return 0, nil, err
	}
	if err := a.store.insert(col, doc); err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, sanitize(col, doc), nil
}

func (a *app) update(r *http.Request) (int, any, error) {
	col := r.PathValue("collection")
	if err := validateCollection(col); err != nil {
		return 0, nil, err
	}
	existing, err := a.store.require(col, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	body, err := readObject(r)
	if err != nil {
		return 0, nil, err
	}
	doc, err := a.applyUpdateRules(col, existing, body)
	if err != nil {
		return 0, nil, err
	}
	if err := a.store.upsert(col, doc); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, sanitize(col, doc), nil
}

func (a *app) remove(r *http.Request) (int, any, error) {
	col, id := r.PathValue("collection"), r.PathValue("id")
	if err := validateCollection(col); err != nil {
		return 0, nil, err
	}
	doc, err := a.store.require(col, id)
	if err != nil {
		return 0, nil, err
	}
	if err := a.checkDelete(col, doc); err != nil {
		return 0, nil, err
	}
	if _, err := a.store.delete(col, id); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"success": true}, nil
}
