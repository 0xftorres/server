package main

// Endpoints con reglas de negocio (reservas, torneos, ligas, clases, caja...).
// Son el equivalente servidor de la lógica que tenía el mock del front, así el front recibe
// exactamente las mismas formas de respuesta.

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// ===========================================================================
// Canchas
// ===========================================================================
func defaultAvailability() M {
	return M{
		"days":          []any{"Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado", "Domingo"},
		"openTime":      "08:00",
		"closeTime":     "22:00",
		"slotMinutes":   60,
		"blockedRanges": []any{},
		"notes":         "",
	}
}

// PATCH /api/canchas/{id}/estado  { status, maintenanceNotes }
func (a *app) courtStatus(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	court, err := a.store.require("canchas", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	status := s(body, "status")
	if !slices.Contains([]string{"disponible", "ocupada", "mantenimiento"}, status) {
		return 0, nil, badRequest("status debe ser disponible, ocupada o mantenimiento.")
	}
	court["status"] = status
	court["maintenanceNotes"] = s(body, "maintenanceNotes")
	if err := a.store.upsert("canchas", court); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, court, nil
}

// PUT /api/canchas/{id}/disponibilidad  { days, openTime, closeTime, slotMinutes, blockedRanges, notes }
func (a *app) courtAvailability(r *http.Request) (int, any, error) {
	body, err := readObject(r)
	if err != nil {
		return 0, nil, err
	}
	court, err := a.store.require("canchas", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	next := merge(merge(defaultAvailability(), asMap(court["availability"])), body)
	if len(arr(next, "days")) == 0 {
		return 0, nil, badRequest("Debe indicar al menos un día de atención.")
	}
	if s(next, "closeTime") <= s(next, "openTime") {
		return 0, nil, badRequest("El horario de cierre debe ser posterior al de apertura.")
	}
	court["availability"] = next
	if err := a.store.upsert("canchas", court); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, court, nil
}

// ===========================================================================
// Stock
// ===========================================================================
// PATCH /api/stock/{id}/ajuste  { totalStock }
func (a *app) stockAdjust(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	item, err := a.store.require("stock", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	total := math.Max(f(body, "totalStock"), 0)
	available := math.Max(total-f(item, "inUseStock"), 0)
	item["totalStock"] = total
	item["availableStock"] = available
	item["status"] = stockStatus(available, f(item, "minAlertThreshold"))
	if err := a.store.upsert("stock", item); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, item, nil
}

// ===========================================================================
// Reservas
// ===========================================================================

// client devuelve nombre y DNI del titular (primer participante) para los recibos.
func client(res M) (string, string) {
	var lead M
	if ps := arr(res, "participants"); len(ps) > 0 {
		lead = asMap(ps[0])
	}
	return orDefault(s(lead, "name"), "Socio Tenis Ahora"), orDefault(s(lead, "dni"), "00.000.000")
}

// createReservationInner da de alta una reserva con recibo de seña (RD01: se confirma con el 50% abonado).
func (a *app) createReservationInner(body M) (M, error) {
	if courtID := s(body, "courtId"); courtID != "" {
		court, ok, err := a.store.get("canchas", courtID)
		if err != nil {
			return nil, err
		}
		if ok && s(court, "status") == "mantenimiento" {
			return nil, conflict("La cancha está bloqueada por mantenimiento y no admite reservas (RD06).")
		}
	}
	count, err := a.store.count("reservas")
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("RES-%d-%03d", year(), count+1)
	for n := 1; ; {
		taken, err := a.store.exists("reservas", id)
		if err != nil {
			return nil, err
		}
		if !taken {
			break
		}
		n++
		id = fmt.Sprintf("RES-%d-%03d-%d", year(), count+1, n)
	}
	recID := fmt.Sprintf("REC-50-%d", rand4())

	res := body
	res["id"] = id
	res["depositReceiptNumber"] = recID
	res["remainingPaid"] = false
	res["finalReceiptNumber"] = nil
	res["status"] = "confirmada"
	res["createdAt"] = nowISO()
	res["cancellationDetails"] = nil
	if _, ok := res["participants"]; !ok {
		res["participants"] = []any{}
	}
	if err := a.store.insert("reservas", res); err != nil {
		return nil, err
	}

	name, dni := client(res)
	receipt := M{
		"id":            recID,
		"reservationId": id,
		"clientName":    name,
		"clientDni":     dni,
		"concept":       "Seña 50% Obligatoria - " + s(res, "courtName"),
		"items": []any{
			M{"description": fmt.Sprintf("Alquiler %shs %s", numStr(f(res, "durationHours")), s(res, "courtName")), "amount": f(res, "courtCost")},
			M{"description": "Equipamiento incluido (Red, Pelotas, Raquetas)", "amount": 0},
			M{"description": "Total Turno", "amount": f(res, "totalCost")},
			M{"description": "Cobro de Seña 50% para Confirmación", "amount": f(res, "depositPaid")},
		},
		"totalPaid":     f(res, "depositPaid"),
		"paymentMethod": orDefault(s(res, "depositPaymentMethod"), "Mercado Pago (QR)"),
		"status":        "completado",
		"date":          nowStamp(),
		"qrData":        "https://mpago.la/pos/tenisahora/" + strings.ToLower(recID),
		"cashierName":   "Cobro Digital Online",
	}
	if err := a.store.insert("recibos", receipt); err != nil {
		return nil, err
	}
	return M{"data": res, "receipt": receipt}, nil
}

// POST /api/reservas
func (a *app) createReservation(r *http.Request) (int, any, error) {
	body, err := readObject(r)
	if err != nil {
		return 0, nil, err
	}
	res, err := a.createReservationInner(body)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, res, nil
}

// GET /api/reservas — el personal ve todas; un socio, solo las suyas.
func (a *app) listReservations(r *http.Request) (int, any, error) {
	return a.listCollection(claimsOf(r), "reservas")
}

// GET /api/reservas/ocupacion — turnos tomados de todas las reservas activas, sin datos de quién
// reservó, para que un socio vea qué horarios están libres.
func (a *app) reservationsOccupancy(r *http.Request) (int, any, error) {
	docs, err := a.store.list("reservas")
	if err != nil {
		return 0, nil, err
	}
	out := []M{}
	for _, d := range docs {
		if s(d, "status") == "cancelada" {
			continue
		}
		slot := M{}
		for _, k := range []string{"id", "courtId", "courtName", "date", "startTime", "endTime", "durationHours", "status"} {
			slot[k] = d[k]
		}
		out = append(out, slot)
	}
	return http.StatusOK, out, nil
}

// recalculateReservation recalcula hora de fin, costo y saldo al modificar una reserva (PUT /api/reservas/{id}).
func (a *app) recalculateReservation(existing, patch M) (M, error) {
	next := merge(existing, patch)
	next["id"] = existing["id"]
	dur := f(next, "durationHours")
	if dur <= 0 {
		dur = 1
	}
	_, startChanged := patch["startTime"]
	_, durationChanged := patch["durationHours"]
	_, courtChanged := patch["courtId"]

	if startChanged || durationChanged {
		hh, mm, _ := strings.Cut(s(next, "startTime"), ":")
		h, err := strconv.ParseInt(strings.TrimSpace(hh), 10, 64)
		if err != nil {
			h = 8
		}
		m, err := strconv.ParseInt(strings.TrimSpace(mm), 10, 64)
		if err != nil {
			m = 0
		}
		totalMin := h*60 + m + int64(math.Round(dur*60))
		next["endTime"] = fmt.Sprintf("%02d:%02d", totalMin/60, totalMin%60)
	}

	court, ok, err := a.store.get("canchas", s(next, "courtId"))
	if err != nil {
		return nil, err
	}
	if ok {
		if courtChanged && s(court, "status") == "mantenimiento" {
			return nil, conflict("La cancha seleccionada está bloqueada por mantenimiento (RD06).")
		}
		if courtChanged || durationChanged {
			if courtChanged {
				next["courtName"] = fmt.Sprintf("%s (%s)", s(court, "name"), s(court, "surfaceType"))
				next["surfaceType"] = s(court, "surfaceType")
			}
			courtCost := f(court, "pricePerHour") * dur
			total := courtCost + f(next, "equipmentCost")
			next["courtCost"] = courtCost
			next["totalCost"] = total
			next["remainingBalance"] = math.Max(total-f(next, "depositPaid"), 0)
		}
	}
	next["updatedAt"] = nowISO()
	return next, nil
}

// POST /api/reservas/{id}/pagar-saldo  { paymentMethod }
func (a *app) payRemaining(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	id := r.PathValue("id")
	res, err := a.store.require("reservas", id)
	if err != nil {
		return 0, nil, err
	}
	if b(res, "remainingPaid", false) {
		return 0, nil, conflict("El saldo de esta reserva ya fue liquidado.")
	}
	if s(res, "status") == "cancelada" {
		return 0, nil, conflict("La reserva está cancelada.")
	}
	finalID := fmt.Sprintf("REC-FINAL-%d", rand4())
	res["remainingPaid"] = true
	res["status"] = "finalizada"
	res["finalReceiptNumber"] = finalID
	if err := a.store.upsert("reservas", res); err != nil {
		return 0, nil, err
	}

	name, dni := client(res)
	receipt := M{
		"id":            finalID,
		"reservationId": id,
		"clientName":    name,
		"clientDni":     dni,
		"concept":       "Liquidación Final 50% Saldo - " + s(res, "courtName"),
		"items":         []any{M{"description": "Saldo restante 50% de turno finalizado", "amount": f(res, "remainingBalance")}},
		"totalPaid":     f(res, "remainingBalance"),
		"paymentMethod": orDefault(s(body, "paymentMethod"), "Efectivo"),
		"status":        "completado",
		"date":          nowStamp(),
		"qrData":        "https://mpago.la/pos/tenisahora/" + strings.ToLower(finalID),
		"cashierName":   "Recepción del Club",
	}
	if err := a.store.insert("recibos", receipt); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, res, nil
}

// POST /api/reservas/{id}/cancelar  { hoursInAdvance }
// RD02-RD04: con 6 hs o más de antelación corresponde reembolso total; con menos, se retiene
// el 50% de la seña como cargo y se reintegra el resto.
func (a *app) cancelReservation(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	id := r.PathValue("id")
	res, err := a.store.require("reservas", id)
	if err != nil {
		return 0, nil, err
	}
	if s(res, "status") == "cancelada" {
		return 0, nil, conflict("La reserva ya está cancelada.")
	}
	hours := f(body, "hoursInAdvance")
	within := hours >= 6
	policy := "Cancelación tardía (<6hs): Se aplica retención del 50% de seña por costo operativo."
	if within {
		policy = "Cancelación anticipada (>6hs): Reembolso de seña según política."
	}
	res["status"] = "cancelada"
	res["cancellationDetails"] = M{
		"cancelledAt":    nowISO(),
		"hoursInAdvance": hours,
		"policyApplied":  policy,
	}
	if err := a.store.upsert("reservas", res); err != nil {
		return 0, nil, err
	}

	name, dni := client(res)
	deposit := f(res, "depositPaid")
	refundCount, err := a.store.count("reembolsos")
	if err != nil {
		return 0, nil, err
	}
	refund := M{
		"id":                   fmt.Sprintf("RMB-%d-%04d", year(), refundCount+9),
		"clientName":           name,
		"clientDni":            dni,
		"relatedReservationId": id,
		"relatedReceiptId":     res["depositReceiptNumber"],
		"originalAmount":       deposit,
		"status":               "pendiente",
		"date":                 nowStamp(),
		"processedBy":          "Sistema de Reservas",
	}

	var penalty any
	if within {
		refund["reason"] = fmt.Sprintf("Cancelación de reserva %s con %s hs de antelación (dentro del plazo reglamentario).", id, numStr(hours))
		refund["refundType"] = "total"
		refund["refundAmount"] = deposit
		refund["refundMethod"] = "Devolución a " + orDefault(s(res, "depositPaymentMethod"), "medio de pago original")
	} else {
		retained := math.Round(deposit * 0.5)
		receipt := M{
			"id":            fmt.Sprintf("REC-CARGO-%d", rand4()),
			"reservationId": id,
			"clientName":    name,
			"clientDni":     dni,
			"concept":       "Cargo por cancelación fuera de plazo - " + s(res, "courtName"),
			"items": []any{
				M{"description": fmt.Sprintf("Seña abonada (%s)", id), "amount": deposit},
				M{"description": "Retención 50% por cancelación con menos de 6 hs", "amount": retained},
				M{"description": "Saldo reintegrado como crédito en cuenta", "amount": deposit - retained},
			},
			"totalPaid":     retained,
			"paymentMethod": "Retención sobre seña",
			"status":        "completado",
			"date":          nowStamp(),
			"cashierName":   "Sistema de Reservas",
		}
		if err := a.store.insert("recibos", receipt); err != nil {
			return 0, nil, err
		}
		penalty = receipt
		refund["reason"] = fmt.Sprintf("Cancelación fuera de plazo (%s hs). Se retiene el 50%% de la seña y se reintegra el resto.", numStr(hours))
		refund["refundType"] = "parcial"
		refund["refundAmount"] = deposit - retained
		refund["refundMethod"] = "Crédito en cuenta de socio"
	}
	if err := a.store.insert("reembolsos", refund); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"data": res, "isWithinFreeWindow": within, "refund": refund, "penaltyReceipt": penalty}, nil
}

// ===========================================================================
// Torneos y ligas
// ===========================================================================
func newParticipant(body M, prefix string) M {
	return M{
		"id":          fmt.Sprintf("%s-%d", prefix, nowMillis()),
		"name":        orDefault(orDefault(s(body, "name"), s(body, "partnerName")), "Participante Inscripto"),
		"dni":         orDefault(s(body, "dni"), "00.000.000"),
		"phone":       s(body, "phone"),
		"partnerName": s(body, "partnerName"),
		"partnerDni":  s(body, "partnerDni"),
		"enrolledAt":  today(),
	}
}

func (a *app) setRules(r *http.Request, col string) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	doc, err := a.store.require(col, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	doc["internalRules"] = s(body, "internalRules")
	if err := a.store.upsert(col, doc); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, doc, nil
}

// PUT /api/torneos/{id}/reglamento
func (a *app) tournamentRules(r *http.Request) (int, any, error) { return a.setRules(r, "torneos") }

// PUT /api/ligas/{id}/reglamento
func (a *app) leagueRules(r *http.Request) (int, any, error) { return a.setRules(r, "ligas") }

// POST /api/torneos/{id}/inscripciones
func (a *app) tournamentEnroll(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	t, err := a.store.require("torneos", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	list := arr(t, "participants")
	if max := f(t, "maxParticipants"); max > 0 && float64(len(list)) >= max {
		return 0, nil, conflict("El torneo alcanzó el cupo máximo de inscriptos.")
	}
	p := newParticipant(body, "p")
	list = append(list, p)
	t["currentEnrolled"] = len(list)
	t["participants"] = list
	if err := a.store.upsert("torneos", t); err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, M{"success": true, "participant": p}, nil
}

// DELETE /api/torneos/{id}/inscripciones/{pid}
func (a *app) tournamentUnenroll(r *http.Request) (int, any, error) {
	t, err := a.store.require("torneos", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	pid := r.PathValue("pid")
	list := []any{}
	for _, p := range arr(t, "participants") {
		if s(asMap(p), "id") != pid {
			list = append(list, p)
		}
	}
	t["currentEnrolled"] = len(list)
	t["participants"] = list
	if err := a.store.upsert("torneos", t); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"success": true}, nil
}

func newMatch(body M, prefix string) M {
	return M{
		"id":     fmt.Sprintf("%s-%d", prefix, nowMillis()),
		"p1":     strings.TrimSpace(s(body, "p1")),
		"p2":     strings.TrimSpace(s(body, "p2")),
		"score":  "Pendiente",
		"winner": nil,
		"status": "programado",
		"court":  orDefault(s(body, "court"), "Cancha 1"),
		"date":   orDefault(s(body, "date"), today()),
	}
}

func addMatchTo(doc M, roundsKey, roundName, defaultRound string, m M) {
	rounds := arr(doc, roundsKey)
	name := roundName
	if name == "" {
		name = fmt.Sprintf("%s %d", defaultRound, len(rounds)+1)
	}
	for _, ri := range rounds {
		if r, ok := ri.(M); ok && s(r, "round") == name {
			r["matches"] = append(arr(r, "matches"), m)
			return
		}
	}
	doc[roundsKey] = append(rounds, M{"round": name, "matches": []any{m}})
}

// updateMatchIn mezcla patch sobre el enfrentamiento matchID (in place) y lo devuelve.
func updateMatchIn(doc M, roundsKey, matchID string, patch M) (M, error) {
	var found M
	for _, ri := range arr(doc, roundsKey) {
		r, ok := ri.(M)
		if !ok {
			continue
		}
		matches := arr(r, "matches")
		for i, mi := range matches {
			if m, ok := mi.(M); ok && s(m, "id") == matchID {
				next := merge(m, patch)
				next["id"] = matchID
				matches[i] = next
				found = next
			}
		}
	}
	if found == nil {
		return nil, notFound(fmt.Sprintf("No existe el enfrentamiento %s.", matchID))
	}
	return found, nil
}

func validateMatchBody(body M) error {
	p1, p2 := strings.TrimSpace(s(body, "p1")), strings.TrimSpace(s(body, "p2"))
	if p1 == "" || p2 == "" {
		return badRequest("p1 y p2 son obligatorios.")
	}
	if p1 == p2 {
		return badRequest("Los dos jugadores deben ser distintos.")
	}
	return nil
}

func (a *app) addMatch(r *http.Request, col, roundsKey, defaultRound, prefix string) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	if err := validateMatchBody(body); err != nil {
		return 0, nil, err
	}
	doc, err := a.store.require(col, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	m := newMatch(body, prefix)
	addMatchTo(doc, roundsKey, s(body, "round"), defaultRound, m)
	if err := a.store.upsert(col, doc); err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, M{"success": true, "match": m}, nil
}

func (a *app) updateMatch(r *http.Request, col, roundsKey string) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	doc, err := a.store.require(col, r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	m, err := updateMatchIn(doc, roundsKey, r.PathValue("mid"), body)
	if err != nil {
		return 0, nil, err
	}
	if err := a.store.upsert(col, doc); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"success": true, "match": m}, nil
}

// POST /api/torneos/{id}/enfrentamientos  { round, p1, p2, court, date }
func (a *app) tournamentAddMatch(r *http.Request) (int, any, error) {
	return a.addMatch(r, "torneos", "bracket", "Ronda", "tm")
}

// PUT /api/torneos/{id}/enfrentamientos/{mid}
func (a *app) tournamentUpdateMatch(r *http.Request) (int, any, error) {
	return a.updateMatch(r, "torneos", "bracket")
}

// POST /api/ligas/{id}/inscripciones — agrega el participante y su fila en la tabla.
func (a *app) leagueEnroll(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	l, err := a.store.require("ligas", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	list := arr(l, "participants")
	if max := f(l, "maxParticipants"); max > 0 && float64(len(list)) >= max {
		return 0, nil, conflict("La liga alcanzó el cupo máximo de inscriptos.")
	}
	p := newParticipant(body, "lp")
	list = append(list, p)
	standings := arr(l, "standings")
	if !slices.ContainsFunc(standings, func(st any) bool { return s(asMap(st), "name") == s(p, "name") }) {
		standings = append(standings, M{"rank": len(standings) + 1, "name": s(p, "name"), "pj": 0, "pg": 0, "pp": 0, "sf": 0, "sc": 0, "dif": 0, "points": 0})
	}
	l["currentEnrolled"] = len(list)
	l["participants"] = list
	l["standings"] = standings
	if err := a.store.upsert("ligas", l); err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, M{"success": true, "participant": p}, nil
}

// rerank reasigna `rank` según el orden actual de la tabla.
func rerank(standings []any) {
	for i, st := range standings {
		if m, ok := st.(M); ok {
			m["rank"] = i + 1
		}
	}
}

// DELETE /api/ligas/{id}/inscripciones/{pid}
func (a *app) leagueUnenroll(r *http.Request) (int, any, error) {
	l, err := a.store.require("ligas", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	pid := r.PathValue("pid")
	removedName, removed := "", false
	list := []any{}
	for _, p := range arr(l, "participants") {
		if pm := asMap(p); s(pm, "id") == pid {
			if !removed {
				removedName, removed = s(pm, "name"), true
			}
		} else {
			list = append(list, p)
		}
	}
	standings := []any{}
	for _, st := range arr(l, "standings") {
		if !removed || s(asMap(st), "name") != removedName {
			standings = append(standings, st)
		}
	}
	rerank(standings)
	l["currentEnrolled"] = len(list)
	l["participants"] = list
	l["standings"] = standings
	if err := a.store.upsert("ligas", l); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"success": true}, nil
}

// POST /api/ligas/{id}/enfrentamientos  { round, p1, p2, court, date }
func (a *app) leagueAddMatch(r *http.Request) (int, any, error) {
	return a.addMatch(r, "ligas", "fixtures", "Jornada", "lm")
}

// PUT /api/ligas/{id}/enfrentamientos/{mid}
func (a *app) leagueUpdateMatch(r *http.Request) (int, any, error) {
	return a.updateMatch(r, "ligas", "fixtures")
}

// POST /api/ligas/{id}/enfrentamientos/{mid}/resultado  { score, winner }
// Marca el partido como finalizado y actualiza la tabla (3 puntos por victoria).
func (a *app) leagueMatchScore(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	l, err := a.store.require("ligas", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	mid := r.PathValue("mid")
	winner, score := s(body, "winner"), s(body, "score")
	if winner == "" {
		return 0, nil, badRequest("Debe indicar el ganador.")
	}
	var before M
	for _, ri := range arr(l, "fixtures") {
		for _, mi := range arr(asMap(ri), "matches") {
			if m := asMap(mi); before == nil && s(m, "id") == mid {
				before = m
			}
		}
	}
	if before == nil {
		return 0, nil, notFound(fmt.Sprintf("No existe el enfrentamiento %s.", mid))
	}
	if winner != s(before, "p1") && winner != s(before, "p2") {
		return 0, nil, badRequest("El ganador debe ser uno de los dos jugadores del partido.")
	}
	alreadyFinished := s(before, "status") == "finalizado"
	m, err := updateMatchIn(l, "fixtures", mid, M{"score": score, "winner": winner, "status": "finalizado"})
	if err != nil {
		return 0, nil, err
	}

	if !alreadyFinished {
		loser := s(m, "p1")
		if winner == s(m, "p1") {
			loser = s(m, "p2")
		}
		add := func(st M, key string, delta float64) { st[key] = f(st, key) + delta }
		standings := arr(l, "standings")
		for _, sti := range standings {
			st, ok := sti.(M)
			if !ok {
				continue
			}
			switch s(st, "name") {
			case winner:
				add(st, "pj", 1)
				add(st, "pg", 1)
				add(st, "points", 3)
				add(st, "sf", 2)
				add(st, "dif", 2)
			case loser:
				add(st, "pj", 1)
				add(st, "pp", 1)
				add(st, "sc", 2)
				add(st, "dif", -2)
			}
		}
		sort.SliceStable(standings, func(i, j int) bool {
			x, y := asMap(standings[i]), asMap(standings[j])
			if f(x, "points") != f(y, "points") {
				return f(x, "points") > f(y, "points")
			}
			return f(x, "dif") > f(y, "dif")
		})
		rerank(standings)
		if standings == nil {
			standings = []any{}
		}
		l["standings"] = standings
	}
	if err := a.store.upsert("ligas", l); err != nil {
		return 0, nil, err
	}
	standings, ok := l["standings"]
	if !ok {
		standings = []any{}
	}
	return http.StatusOK, M{"success": true, "match": m, "standings": standings}, nil
}

// ===========================================================================
// Profesores / entrenadores
// ===========================================================================
// PUT /api/profesores/{id}/disponibilidad  { availableHours }
func (a *app) coachAvailability(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	c, err := a.store.require("profesores", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	c["availableHours"] = s(body, "availableHours")
	if err := a.store.upsert("profesores", c); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, c, nil
}

// PUT /api/profesores/{id}/certificacion  { titleName, institution, licenseNumber, issueDate, expiryDate, status, digitalDocumentUrl }
func (a *app) coachLicense(r *http.Request) (int, any, error) {
	body, err := readObject(r)
	if err != nil {
		return 0, nil, err
	}
	c, err := a.store.require("profesores", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	c["licenseDetails"] = merge(asMap(c["licenseDetails"]), body)
	if err := a.store.upsert("profesores", c); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, c, nil
}

// ===========================================================================
// Clases y entrenamientos
// ===========================================================================
// POST /api/clases/{id}/alumnos  { name, dni, phone }  — RD07/RD10: respeta el cupo de la comisión.
func (a *app) classEnroll(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	cls, err := a.store.require("clases", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	if strings.TrimSpace(s(body, "name")) == "" {
		return 0, nil, badRequest("El nombre del alumno es obligatorio.")
	}
	students := arr(cls, "students")
	capacity := int(capTo30(f(cls, "maxCapacity")))
	if len(students) >= capacity {
		return 0, nil, conflict(fmt.Sprintf("No es posible agregar más alumnos: el cupo de %d está completo.", capacity))
	}
	attendance := make([]any, max(len(arr(cls, "sessions")), 1))
	for i := range attendance {
		attendance[i] = "P"
	}
	st := M{
		"id":         fmt.Sprintf("std-%d", nowMillis()),
		"name":       s(body, "name"),
		"dni":        s(body, "dni"),
		"phone":      s(body, "phone"),
		"enrolledAt": today(),
		"attendance": attendance,
	}
	students = append(students, st)
	cls["currentEnrolled"] = len(students)
	cls["students"] = students
	if err := a.store.upsert("clases", cls); err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, M{"success": true, "student": st}, nil
}

// DELETE /api/clases/{id}/alumnos/{sid}
func (a *app) classUnenroll(r *http.Request) (int, any, error) {
	cls, err := a.store.require("clases", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	sid := r.PathValue("sid")
	students := []any{}
	for _, st := range arr(cls, "students") {
		if s(asMap(st), "id") != sid {
			students = append(students, st)
		}
	}
	cls["currentEnrolled"] = len(students)
	cls["students"] = students
	if err := a.store.upsert("clases", cls); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"success": true}, nil
}

func (a *app) attendanceResponse(r *http.Request) (int, any, error) {
	classes, err := a.store.list("clases")
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"success": true, "classes": visibleAll(claimsOf(r), "clases", classes)}, nil
}

func padAttendance(att []any, idx int) []any {
	for len(att) <= idx {
		att = append(att, "-")
	}
	return att
}

// POST /api/clases/{id}/asistencia/alternar  { studentId, sessionIndex }  — P → A → J → P
func (a *app) classToggleAttendance(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	cls, err := a.store.require("clases", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	sid := s(body, "studentId")
	idx := int(math.Max(f(body, "sessionIndex"), 0))
	for _, sti := range arr(cls, "students") {
		st, ok := sti.(M)
		if !ok || s(st, "id") != sid {
			continue
		}
		att := padAttendance(arr(st, "attendance"), idx)
		cur, _ := att[idx].(string)
		next := map[string]string{"P": "A", "A": "J", "J": "P"}[cur]
		att[idx] = orDefault(next, "P")
		st["attendance"] = att
	}
	if err := a.store.upsert("clases", cls); err != nil {
		return 0, nil, err
	}
	return a.attendanceResponse(r)
}

// POST /api/clases/{id}/asistencia/sesion  { sessionIndex, status }
func (a *app) classBulkAttendance(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	cls, err := a.store.require("clases", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	idx := int(math.Max(f(body, "sessionIndex"), 0))
	status := orDefault(s(body, "status"), "P")
	for _, sti := range arr(cls, "students") {
		st, ok := sti.(M)
		if !ok {
			continue
		}
		att := padAttendance(arr(st, "attendance"), idx)
		att[idx] = status
		st["attendance"] = att
	}
	if err := a.store.upsert("clases", cls); err != nil {
		return 0, nil, err
	}
	return a.attendanceResponse(r)
}

// POST /api/clases/{id}/asistencia/registrar  { studentId, status } — agrega una sesión al alumno.
func (a *app) classRecordAttendance(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	cls, err := a.store.require("clases", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	sid := s(body, "studentId")
	for _, sti := range arr(cls, "students") {
		if st, ok := sti.(M); ok && s(st, "id") == sid {
			st["attendance"] = append(arr(st, "attendance"), s(body, "status"))
		}
	}
	if err := a.store.upsert("clases", cls); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"success": true}, nil
}

// POST /api/clases-particulares/{id}/cancelar
func (a *app) privateCancel(r *http.Request) (int, any, error) {
	p, err := a.store.require("clases-particulares", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	p["status"] = "cancelada"
	if err := a.store.upsert("clases-particulares", p); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"success": true, "data": p}, nil
}

// ===========================================================================
// Caja: facturas, reembolsos, descuentos por usuario
// ===========================================================================
func computeInvoice(d M) (M, error) {
	items := []any{}
	subtotal := 0.0
	for _, it := range arr(d, "items") {
		im := asMap(it)
		q := f(im, "quantity")
		if q <= 0 {
			q = 1
		}
		if strings.TrimSpace(s(im, "description")) == "" {
			return nil, badRequest("La factura debe tener al menos un ítem con descripción.")
		}
		items = append(items, M{"description": s(im, "description"), "quantity": q, "unitPrice": f(im, "unitPrice")})
		subtotal += q * f(im, "unitPrice")
	}
	if len(items) == 0 {
		return nil, badRequest("La factura debe tener al menos un ítem con descripción.")
	}
	if subtotal <= 0 {
		return nil, badRequest("El importe de la factura debe ser mayor a cero.")
	}
	taxRate := 21.0
	if _, ok := d["taxRate"]; ok {
		taxRate = f(d, "taxRate")
	}
	tax := math.Round(subtotal * taxRate / 100)
	defaultStr(d, "type", "B")
	defaultStr(d, "concept", s(items[0].(M), "description"))
	defaultStr(d, "paymentMethod", "Efectivo")
	defaultStr(d, "issuedBy", "Administración General")
	if s(d, "relatedReceiptId") == "" {
		d["relatedReceiptId"] = nil
	}
	d["items"] = items
	d["subtotal"] = subtotal
	d["taxRate"] = taxRate
	d["taxAmount"] = tax
	d["total"] = subtotal + tax
	d["status"] = "emitida"
	d["date"] = nowStamp()
	return d, nil
}

func computeRefund(d M) (M, error) {
	original := f(d, "originalAmount")
	amount := math.Min(f(d, "refundAmount"), original)
	if amount <= 0 {
		return nil, badRequest("El importe a reembolsar debe ser mayor a cero.")
	}
	d["originalAmount"] = original
	d["refundAmount"] = amount
	if amount >= original {
		d["refundType"] = "total"
	} else {
		d["refundType"] = "parcial"
	}
	defaultStr(d, "refundMethod", "Crédito en cuenta de socio")
	defaultStr(d, "status", "pendiente")
	defaultStr(d, "processedBy", "Administración General")
	for _, k := range []string{"relatedReservationId", "relatedReceiptId"} {
		if s(d, k) == "" {
			d[k] = nil
		}
	}
	d["date"] = nowStamp()
	return d, nil
}

// POST /api/facturas/{id}/anular
func (a *app) invoiceAnnul(r *http.Request) (int, any, error) {
	inv, err := a.store.require("facturas", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	inv["status"] = "anulada"
	if err := a.store.upsert("facturas", inv); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, inv, nil
}

// PATCH /api/reembolsos/{id}/estado  { status }
func (a *app) refundStatus(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	status := s(body, "status")
	if !slices.Contains([]string{"pendiente", "procesado", "rechazado"}, status) {
		return 0, nil, badRequest("status debe ser pendiente, procesado o rechazado.")
	}
	ref, err := a.store.require("reembolsos", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	ref["status"] = status
	if err := a.store.upsert("reembolsos", ref); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, ref, nil
}

// PUT /api/usuarios/{id}/descuentos  { activeDiscounts: ["PAQ10", ...] }
func (a *app) userDiscounts(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	u, err := a.store.require("usuarios", r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	catalog, err := a.store.list("descuentos")
	if err != nil {
		return 0, nil, err
	}
	known := map[string]bool{}
	for _, d := range catalog {
		known[s(d, "code")] = true
	}
	codes := arr(body, "activeDiscounts")
	for _, c := range codes {
		if code, _ := c.(string); !known[code] {
			raw, _ := marshalJSON(c)
			return 0, nil, badRequest(fmt.Sprintf("El descuento %s no existe en el catálogo.", raw))
		}
	}
	if codes == nil {
		codes = []any{}
	}
	u["activeDiscounts"] = codes
	if err := a.store.upsert("usuarios", u); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, sanitize("usuarios", u), nil
}

// ===========================================================================
// Administración de la API de prueba
// ===========================================================================
// POST /api/admin/reset — vacía la base y vuelve a cargar el seed. Solo rol Empleado.
func (a *app) reset(r *http.Request) (int, any, error) {
	if claims, _ := r.Context().Value(claimsKey{}).(Claims); claims.Role != "Empleado" {
		return 0, nil, forbidden("Solo un usuario con rol Empleado puede resetear la base.")
	}
	seed, err := loadSeedFile(a.seedPath)
	if err != nil {
		return 0, nil, err
	}
	if err := a.store.clear(); err != nil {
		return 0, nil, err
	}
	n, err := applySeed(a.store, seed)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"success": true, "documents": n}, nil
}
