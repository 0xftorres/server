package main

// Endpoints con reglas de negocio (reservas, torneos, ligas, clases, caja...).
// Son el equivalente servidor de la lógica que tenía el mock del front, así el front recibe
// exactamente las mismas formas de respuesta.

import (
	"encoding/base64"
	"fmt"
	"math"
	"mime"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
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

// Índice = time.Weekday.
var weekDays = []string{"Domingo", "Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado"}

func hhmmToMinutes(v string) int {
	hh, mm, _ := strings.Cut(strings.TrimSpace(v), ":")
	h, _ := strconv.Atoi(hh)
	m, _ := strconv.Atoi(mm)
	return h*60 + m
}

// slotBlock devuelve por qué la cancha no admite el turno según su disponibilidad (RD06: días de
// atención, horario y bloqueos programados), o "" si está libre. Mismo criterio que getSlotBlock
// del front. El estado "mantenimiento" se valida aparte.
func slotBlock(court M, date, startTime string, durationHours float64) string {
	av := merge(defaultAvailability(), asMap(court["availability"]))
	day := ""
	if t, err := time.Parse("2006-01-02", date); err == nil {
		day = weekDays[t.Weekday()]
		if !slices.Contains(arr(av, "days"), any(day)) {
			return "sin atención el día " + day
		}
	}
	if startTime == "" {
		return ""
	}
	if durationHours <= 0 {
		durationHours = 1
	}
	from := hhmmToMinutes(startTime)
	to := from + int(math.Round(durationHours*60))
	if from < hhmmToMinutes(s(av, "openTime")) || to > hhmmToMinutes(s(av, "closeTime")) {
		return fmt.Sprintf("fuera de horario (%s a %s)", s(av, "openTime"), s(av, "closeTime"))
	}
	for _, raw := range arr(av, "blockedRanges") {
		bl := asMap(raw)
		if s(bl, "day") == day && from < hhmmToMinutes(s(bl, "to")) && to > hhmmToMinutes(s(bl, "from")) {
			return orDefault(s(bl, "reason"), fmt.Sprintf("bloqueada de %s a %s", s(bl, "from"), s(bl, "to")))
		}
	}
	return ""
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
	if total < f(item, "inUseStock") {
		return 0, nil, badRequest(fmt.Sprintf("El stock total no puede ser menor a lo que está prestado en canchas (%s).", numStr(f(item, "inUseStock"))))
	}
	available := math.Max(total-f(item, "inUseStock"), 0)
	item["totalStock"] = total
	item["availableStock"] = available
	item["status"] = stockStatus(available)
	if err := a.store.upsert("stock", item); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, item, nil
}

// ===========================================================================
// Configuración del club (RD02-RD05)
// ===========================================================================

// Los plazos de reserva y cancelación (RD02-RD05) y la tarifa de la clase particular los fija el
// club; el sistema solo los consulta.
const configID = "club"

func defaultClubConfig() M {
	return M{
		"freeCancellationHours":            6.0,
		"lateCancellationRetentionPercent": 50.0,
		"maxAdvanceBookingDays":            30.0,
		"privateLessonHourlyRate":          6000.0,
	}
}

func (a *app) clubConfig() (M, error) {
	cfg := defaultClubConfig()
	cfg["id"] = configID
	return cfg, nil
}

// GET /api/configuracion
func (a *app) getClubConfig(r *http.Request) (int, any, error) {
	cfg, err := a.clubConfig()
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, cfg, nil
}

// checkBookingWindow aplica RD05: no se reserva más allá de la antelación máxima del club.
func (a *app) checkBookingWindow(date string) error {
	if date == "" {
		return nil
	}
	day, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return badRequest("La fecha de la reserva no es válida (formato AAAA-MM-DD).")
	}
	cfg, err := a.clubConfig()
	if err != nil {
		return err
	}
	maxDays := int(f(cfg, "maxAdvanceBookingDays"))
	now := time.Now()
	limit := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, maxDays)
	if day.After(limit) {
		return badRequest(fmt.Sprintf("Las reservas se pueden hacer con hasta %d días de antelación (hasta el %s) (RD05).", maxDays, limit.Format("2006-01-02")))
	}
	return nil
}

// hoursUntilStart devuelve cuántas horas faltan para el inicio de la reserva (negativo si ya empezó).
func hoursUntilStart(res M) (float64, error) {
	start, err := time.ParseInLocation("2006-01-02 15:04", s(res, "date")+" "+orDefault(s(res, "startTime"), "00:00"), time.Local)
	if err != nil {
		return 0, badRequest("La reserva no tiene una fecha y hora de inicio válidas.")
	}
	return math.Round(time.Until(start).Hours()*10) / 10, nil
}

// ===========================================================================
// Reservas
// ===========================================================================

// Estados de una reserva: pendiente de pago (registrada, sin seña) -> confirmada (seña del 50 %
// abonada, RD01) -> en juego (alquiler iniciado, equipamiento descontado del stock) -> finalizada
// (saldo abonado, equipamiento devuelto). Se cancela mientras está pendiente o confirmada.
const (
	resPending   = "pendiente_pago"
	resConfirmed = "confirmada"
	resPlaying   = "en_juego"
	resFinished  = "finalizada"
	resCancelled = "cancelada"
)

// Cada alquiler dura como máximo dos horas (consigna).
const maxRentalHours = 2.0

// Equipamiento obligatorio de cada alquiler: una red, un tubo de pelotas y una raqueta por jugador.
var equipment = []struct{ key, category, label string }{
	{"nets", "Redes", "redes"},
	{"ballTubes", "Pelotas", "pelotas"},
	{"rackets", "Raquetas", "raquetas"},
}

func equipmentFor(playersType string) M {
	rackets := 2.0
	if playersType == "dobles" {
		rackets = 4
	}
	return M{"nets": 1.0, "ballTubes": 1.0, "rackets": rackets}
}

func setStockLevels(item M, inUse float64) {
	available := math.Max(f(item, "totalStock")-inUse, 0)
	item["inUseStock"] = inUse
	item["availableStock"] = available
	item["status"] = stockStatus(available)
}

// checkEquipment aplica la regla de la consigna: no se alquila una cancha si no hay red, pelotas y
// raquetas disponibles en stock (TC_001).
func (a *app) checkEquipment(eq M) error {
	items, err := a.store.list("stock")
	if err != nil {
		return err
	}
	missing := []string{}
	for _, e := range equipment {
		need, have := f(eq, e.key), 0.0
		for _, it := range items {
			if strings.EqualFold(s(it, "category"), e.category) {
				have += f(it, "availableStock")
			}
		}
		if have < need {
			missing = append(missing, fmt.Sprintf("%s (hay %s, se necesitan %s)", e.label, numStr(have), numStr(need)))
		}
	}
	if len(missing) > 0 {
		return conflict("No hay stock suficiente para el alquiler: faltan " + strings.Join(missing, ", ") + ".")
	}
	return nil
}

// takeEquipment descuenta del stock el equipamiento del alquiler y devuelve de qué ítems salió,
// para restituir exactamente lo mismo al finalizar (TC_002).
func (a *app) takeEquipment(eq M) ([]any, error) {
	if err := a.checkEquipment(eq); err != nil {
		return nil, err
	}
	items, err := a.store.list("stock")
	if err != nil {
		return nil, err
	}
	taken := []any{}
	for _, e := range equipment {
		need := f(eq, e.key)
		for _, it := range items {
			if need <= 0 || !strings.EqualFold(s(it, "category"), e.category) || f(it, "availableStock") <= 0 {
				continue
			}
			qty := math.Min(need, f(it, "availableStock"))
			setStockLevels(it, f(it, "inUseStock")+qty)
			if err := a.store.upsert("stock", it); err != nil {
				return nil, err
			}
			taken = append(taken, M{"itemId": s(it, "id"), "category": e.category, "quantity": qty})
			need -= qty
		}
	}
	return taken, nil
}

// returnEquipment devuelve al stock lo que se descontó al iniciar el alquiler.
func (a *app) returnEquipment(taken []any) error {
	for _, t := range taken {
		tm := asMap(t)
		item, ok, err := a.store.get("stock", s(tm, "itemId"))
		if err != nil {
			return err
		}
		if !ok {
			continue // el ítem se dio de baja mientras estaba prestado
		}
		setStockLevels(item, math.Max(f(item, "inUseStock")-f(tm, "quantity"), 0))
		if err := a.store.upsert("stock", item); err != nil {
			return err
		}
	}
	return nil
}

// endTime suma la duración al horario de inicio ("HH:MM").
func endTime(start string, hours float64) string {
	total := hhmmToMinutes(start) + int(math.Round(hours*60))
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

// checkSchedule valida fecha, horario y duración de una reserva: no en el pasado, dentro de la
// antelación máxima del club (RD05) y de hasta dos horas.
func (a *app) checkSchedule(date, start string, hours float64) error {
	day, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return badRequest("La fecha de la reserva no es válida (formato AAAA-MM-DD).")
	}
	if _, err := time.Parse("15:04", start); err != nil {
		return badRequest("El horario de inicio no es válido (formato HH:MM).")
	}
	if hours <= 0 || hours > maxRentalHours {
		return badRequest(fmt.Sprintf("La duración de un alquiler es de hasta %s horas.", numStr(maxRentalHours)))
	}
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if day.Before(todayStart) {
		return badRequest("La fecha de la reserva ya pasó.")
	}
	if begins, _ := time.ParseInLocation("2006-01-02 15:04", date+" "+start, time.Local); !begins.After(now) {
		return badRequest("El horario de la reserva ya pasó.")
	}
	return a.checkBookingWindow(date)
}

// checkParticipants exige los datos de los 2 (singles) o 4 (dobles) jugadores, sin repetir DNI y
// sin superar la capacidad de la cancha (consigna: reservas para dos o cuatro personas).
func checkParticipants(res, court M) error {
	want := 0
	switch s(res, "playersType") {
	case "singles":
		want = 2
	case "dobles":
		want = 4
	default:
		return badRequest("La modalidad debe ser singles (2 jugadores) o dobles (4 jugadores).")
	}
	ps := arr(res, "participants")
	if len(ps) != want {
		return badRequest(fmt.Sprintf("Una reserva de %s lleva los datos de %d jugadores (se cargaron %d).", s(res, "playersType"), want, len(ps)))
	}
	seen := map[string]bool{}
	for i, p := range ps {
		pm := asMap(p)
		dni := cleanDNI(s(pm, "dni"))
		if strings.TrimSpace(s(pm, "name")) == "" || dni == "" {
			return badRequest(fmt.Sprintf("Falta el nombre o el DNI del jugador %d.", i+1))
		}
		if seen[dni] {
			return badRequest(fmt.Sprintf("El DNI %s está repetido entre los jugadores.", s(pm, "dni")))
		}
		seen[dni] = true
	}
	if capacity := f(court, "capacity"); capacity > 0 && float64(want) > capacity {
		return badRequest(fmt.Sprintf("%s admite hasta %s jugadores.", s(court, "name"), numStr(capacity)))
	}
	return nil
}

// checkReservationConflict impide reservar dos veces la misma cancha en una franja superpuesta, o
// anotar a un jugador que ya tiene otra reserva a esa hora. Misma regla que findReservationConflict
// del front.
func (a *app) checkReservationConflict(res M, excludeID string) error {
	docs, err := a.store.list("reservas")
	if err != nil {
		return err
	}
	from, to := hhmmToMinutes(s(res, "startTime")), hhmmToMinutes(s(res, "endTime"))
	dnis := map[string]bool{}
	for _, p := range arr(res, "participants") {
		if d := cleanDNI(s(asMap(p), "dni")); d != "" {
			dnis[d] = true
		}
	}
	for _, d := range docs {
		if s(d, "id") == excludeID || s(d, "status") == resCancelled || s(d, "date") != s(res, "date") {
			continue
		}
		if !(hhmmToMinutes(s(d, "startTime")) < to && from < hhmmToMinutes(s(d, "endTime"))) {
			continue
		}
		when := fmt.Sprintf("el %s de %s a %s", s(d, "date"), s(d, "startTime"), s(d, "endTime"))
		if s(d, "courtId") == s(res, "courtId") {
			return conflict(fmt.Sprintf("%s ya está reservada %s.", s(d, "courtName"), when))
		}
		for _, p := range arr(d, "participants") {
			if pm := asMap(p); dnis[cleanDNI(s(pm, "dni"))] {
				return conflict(fmt.Sprintf("%s ya tiene una reserva %s en %s.", s(pm, "name"), when, s(d, "courtName")))
			}
		}
	}
	return nil
}

// checkCourt valida que la cancha exista, no esté en mantenimiento y atienda en ese turno (RD06).
func (a *app) checkCourt(res M) (M, error) {
	if s(res, "courtId") == "" {
		return nil, badRequest("Falta elegir la cancha.")
	}
	court, ok, err := a.store.get("canchas", s(res, "courtId"))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, badRequest(fmt.Sprintf("La cancha %s no existe.", s(res, "courtId")))
	}
	if s(court, "status") == "mantenimiento" {
		return nil, conflict("La cancha está bloqueada por mantenimiento y no admite reservas (RD06).")
	}
	if reason := slotBlock(court, s(res, "date"), s(res, "startTime"), f(res, "durationHours")); reason != "" {
		return nil, conflict(fmt.Sprintf("La cancha no admite reservas en ese turno: %s (RD06).", reason))
	}
	return court, nil
}

// setCosts calcula el costo con la tarifa de la cancha (no con lo que mande el cliente) y la seña
// del 50 %. Una reserva confirmada conserva la seña que ya pagó.
func setCosts(res, court M) {
	courtCost := f(court, "pricePerHour") * f(res, "durationHours")
	res["courtCost"] = courtCost
	res["equipmentCost"] = 0.0
	res["totalCost"] = courtCost
	deposit := f(res, "depositPaid")
	if s(res, "status") == resPending {
		deposit = math.Round(courtCost / 2)
		res["depositAmount"] = deposit
	}
	res["remainingBalance"] = math.Max(courtCost-deposit, 0)
}

// client devuelve nombre y DNI del titular (primer participante) para los recibos.
func client(res M) (string, string) {
	var lead M
	if ps := arr(res, "participants"); len(ps) > 0 {
		lead = asMap(ps[0])
	}
	return orDefault(s(lead, "name"), "Socio Tenis Ahora"), orDefault(s(lead, "dni"), "00.000.000")
}

// rentalLine describe el turno en los recibos.
func rentalLine(res M) string {
	return fmt.Sprintf("Alquiler %s hs - %s - %s de %s a %s", numStr(f(res, "durationHours")), s(res, "courtName"), s(res, "date"), s(res, "startTime"), s(res, "endTime"))
}

// newReceipt arma el recibo de un cobro de la reserva con los datos del pago.
func (a *app) newReceipt(res M, tx M, concept, cashier string, items []any, total float64) (M, error) {
	id, err := a.nextID("recibos", nil)
	if err != nil {
		return nil, err
	}
	name, dni := client(res)
	receipt := merge(M{
		"id":            id,
		"reservationId": s(res, "id"),
		"clientName":    name,
		"clientDni":     dni,
		"concept":       concept,
		"items":         items,
		"totalPaid":     total,
		"status":        "completado",
		"date":          nowStamp(),
		"cashierName":   cashier,
	}, paymentFields(tx))
	return receipt, a.store.insert("recibos", receipt)
}

// createReservationInner registra la reserva como pendiente de pago: queda confirmada recién cuando
// se abona la seña del 50 % (RD01, POST /reservas/{id}/pagar-sena).
func (a *app) createReservationInner(body M) (M, error) {
	res := M{
		"courtId":       s(body, "courtId"),
		"date":          s(body, "date"),
		"startTime":     s(body, "startTime"),
		"durationHours": f(body, "durationHours"),
		"playersType":   s(body, "playersType"),
		"participants":  arr(body, "participants"),
		"notes":         s(body, "notes"),
	}
	if err := a.checkSchedule(s(res, "date"), s(res, "startTime"), f(res, "durationHours")); err != nil {
		return nil, err
	}
	res["endTime"] = endTime(s(res, "startTime"), f(res, "durationHours"))
	court, err := a.checkCourt(res)
	if err != nil {
		return nil, err
	}
	if err := checkParticipants(res, court); err != nil {
		return nil, err
	}
	if err := a.checkReservationConflict(res, ""); err != nil {
		return nil, err
	}
	res["equipmentAssigned"] = equipmentFor(s(res, "playersType"))
	if err := a.checkEquipment(asMap(res["equipmentAssigned"])); err != nil {
		return nil, err
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
	res["id"] = id
	res["courtName"] = fmt.Sprintf("%s (%s)", s(court, "name"), s(court, "surfaceType"))
	res["surfaceType"] = s(court, "surfaceType")
	res["status"] = resPending
	res["depositPaid"] = 0.0
	res["depositReceiptNumber"] = nil
	res["remainingPaid"] = false
	res["finalReceiptNumber"] = nil
	res["cancellationDetails"] = nil
	res["createdAt"] = nowISO()
	setCosts(res, court)
	if err := a.store.insert("reservas", res); err != nil {
		return nil, err
	}
	return M{"data": res}, nil
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
		if s(d, "status") == resCancelled {
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

// Los cambios de estado, los cobros y los costos solo los hacen los endpoints de negocio.
var reservationReadOnly = []string{
	"status", "depositAmount", "depositPaid", "depositPaymentMethod", "depositTransactionId", "depositReceiptNumber",
	"remainingPaid", "finalReceiptNumber", "finalPaymentMethod", "finalTransactionId", "cancellationDetails",
	"equipmentAssigned", "equipmentTaken", "courtName", "surfaceType", "courtCost", "equipmentCost", "totalCost",
	"remainingBalance", "endTime", "createdAt", "confirmedAt", "startedAt", "finishedAt",
}

// recalculateReservation aplica un cambio de horario, cancha o jugadores (PUT /api/reservas/{id}),
// antes de que se juegue y sujeto a disponibilidad.
func (a *app) recalculateReservation(existing, patch M) (M, error) {
	if st := s(existing, "status"); st != resPending && st != resConfirmed {
		return nil, conflict("Solo se modifican reservas pendientes de pago o confirmadas.")
	}
	for _, k := range reservationReadOnly {
		delete(patch, k)
	}
	changed := func(keys ...string) bool {
		for _, k := range keys {
			if v, ok := patch[k]; ok && !reflect.DeepEqual(v, existing[k]) {
				return true
			}
		}
		return false
	}
	next := merge(existing, patch)
	next["id"] = existing["id"]
	scheduleChanged := changed("date", "startTime", "durationHours")
	courtChanged := changed("courtId")
	playersChanged := changed("participants", "playersType")

	if scheduleChanged {
		if err := a.checkSchedule(s(next, "date"), s(next, "startTime"), f(next, "durationHours")); err != nil {
			return nil, err
		}
		next["endTime"] = endTime(s(next, "startTime"), f(next, "durationHours"))
	}
	court, ok, err := a.store.get("canchas", s(next, "courtId"))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, badRequest(fmt.Sprintf("La cancha %s no existe.", s(next, "courtId")))
	}
	if scheduleChanged || courtChanged {
		if _, err := a.checkCourt(next); err != nil {
			return nil, err
		}
	}
	if playersChanged || courtChanged {
		if err := checkParticipants(next, court); err != nil {
			return nil, err
		}
		next["equipmentAssigned"] = equipmentFor(s(next, "playersType"))
	}
	if scheduleChanged || courtChanged || playersChanged {
		if err := a.checkReservationConflict(next, s(existing, "id")); err != nil {
			return nil, err
		}
	}
	if courtChanged {
		next["courtName"] = fmt.Sprintf("%s (%s)", s(court, "name"), s(court, "surfaceType"))
		next["surfaceType"] = s(court, "surfaceType")
	}
	if courtChanged || scheduleChanged {
		setCosts(next, court)
	}
	next["updatedAt"] = nowISO()
	return next, nil
}

// POST /api/reservas/{id}/pagar-sena  { transactionId }
// Cobra la seña del 50 % con un pago aprobado, emite el recibo y confirma la reserva (RD01).
func (a *app) payDeposit(r *http.Request) (int, any, error) {
	body, err := readJSON(r)
	if err != nil {
		return 0, nil, err
	}
	id := r.PathValue("id")
	res, err := a.store.require("reservas", id)
	if err != nil {
		return 0, nil, err
	}
	switch s(res, "status") {
	case resPending:
	case resCancelled:
		return 0, nil, conflict("La reserva está cancelada.")
	default:
		return 0, nil, conflict("La seña de esta reserva ya fue abonada.")
	}
	if err := a.checkEquipment(asMap(res["equipmentAssigned"])); err != nil {
		return 0, nil, err
	}
	deposit := f(res, "depositAmount")
	tx, err := a.consumePayment(claimsOf(r), s(body, "transactionId"), deposit, "reservas/"+id)
	if err != nil {
		return 0, nil, err
	}
	receipt, err := a.newReceipt(res, tx, "Seña 50 % - "+s(res, "courtName"), "Cobro digital online", []any{
		M{"description": rentalLine(res), "amount": f(res, "courtCost")},
		M{"description": "Equipamiento incluido (red, pelotas y raquetas)", "amount": 0},
		M{"description": "Total del turno", "amount": f(res, "totalCost")},
		M{"description": "Seña 50 % para confirmar la reserva", "amount": deposit},
	}, deposit)
	if err != nil {
		return 0, nil, err
	}
	res["status"] = resConfirmed
	res["depositPaid"] = deposit
	res["depositPaymentMethod"] = s(tx, "paymentMethod")
	res["depositTransactionId"] = s(tx, "id")
	res["depositReceiptNumber"] = s(receipt, "id")
	res["confirmedAt"] = nowISO()
	if err := a.store.upsert("reservas", res); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"data": res, "receipt": receipt}, nil
}

// POST /api/reservas/{id}/iniciar
// Registra el inicio del alquiler y descuenta del stock el equipamiento asignado (TC_002). Sin
// stock no se inicia (TC_001).
func (a *app) startRental(r *http.Request) (int, any, error) {
	id := r.PathValue("id")
	res, err := a.store.require("reservas", id)
	if err != nil {
		return 0, nil, err
	}
	switch s(res, "status") {
	case resConfirmed:
	case resPending:
		return 0, nil, conflict("La reserva no tiene la seña abonada: no está confirmada (RD01).")
	default:
		return 0, nil, conflict("Solo se inicia el alquiler de una reserva confirmada.")
	}
	if court, ok, err := a.store.get("canchas", s(res, "courtId")); err != nil {
		return 0, nil, err
	} else if ok && s(court, "status") == "mantenimiento" {
		return 0, nil, conflict("La cancha está bloqueada por mantenimiento (RD06).")
	}
	taken, err := a.takeEquipment(asMap(res["equipmentAssigned"]))
	if err != nil {
		return 0, nil, err
	}
	res["status"] = resPlaying
	res["equipmentTaken"] = taken
	res["startedAt"] = nowISO()
	if err := a.store.upsert("reservas", res); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, res, nil
}

// POST /api/reservas/{id}/pagar-saldo  { transactionId }
// Al terminar el partido: cobra el 50 % restante, emite el recibo final, devuelve el equipamiento
// al stock y da por finalizado el alquiler (TC_002).
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
	switch s(res, "status") {
	case resPlaying:
	case resFinished:
		return 0, nil, conflict("El saldo de esta reserva ya fue liquidado.")
	case resCancelled:
		return 0, nil, conflict("La reserva está cancelada.")
	default:
		return 0, nil, conflict("Primero hay que registrar el inicio del alquiler.")
	}
	remaining := f(res, "remainingBalance")
	tx, err := a.consumePayment(claimsOf(r), s(body, "transactionId"), remaining, "reservas/"+id)
	if err != nil {
		return 0, nil, err
	}
	if err := a.returnEquipment(arr(res, "equipmentTaken")); err != nil {
		return 0, nil, err
	}
	receipt, err := a.newReceipt(res, tx, "Saldo 50 % - "+s(res, "courtName"), "Recepción del club", []any{
		M{"description": rentalLine(res), "amount": f(res, "totalCost")},
		M{"description": fmt.Sprintf("Seña abonada (recibo %s)", s(res, "depositReceiptNumber")), "amount": f(res, "depositPaid")},
		M{"description": "Saldo restante al finalizar el partido", "amount": remaining},
	}, remaining)
	if err != nil {
		return 0, nil, err
	}
	res["status"] = resFinished
	res["remainingPaid"] = true
	res["finalPaymentMethod"] = s(tx, "paymentMethod")
	res["finalTransactionId"] = s(tx, "id")
	res["finalReceiptNumber"] = s(receipt, "id")
	res["finishedAt"] = nowISO()
	if err := a.store.upsert("reservas", res); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, M{"data": res, "receipt": receipt}, nil
}

// POST /api/reservas/{id}/cancelar
// RD02-RD04: con la antelación mínima del club (o más) corresponde reembolso total; con menos, se
// retiene el porcentaje configurado de la seña como cargo y se reintegra el resto. Las horas se
// calculan contra el inicio de la reserva, no las informa quien cancela. Una reserva pendiente de
// pago se anula sin cargo porque no se abonó nada.
func (a *app) cancelReservation(r *http.Request) (int, any, error) {
	id := r.PathValue("id")
	res, err := a.store.require("reservas", id)
	if err != nil {
		return 0, nil, err
	}
	status := s(res, "status")
	switch status {
	case resPending, resConfirmed:
	case resCancelled:
		return 0, nil, conflict("La reserva ya está cancelada.")
	default:
		return 0, nil, conflict("No se puede cancelar una reserva en juego o finalizada.")
	}
	hours, err := hoursUntilStart(res)
	if err != nil {
		return 0, nil, err
	}
	if status == resPending {
		res["status"] = resCancelled
		res["cancellationDetails"] = M{
			"cancelledAt":    nowISO(),
			"hoursInAdvance": hours,
			"policyApplied":  "Reserva sin seña abonada: se anula sin cargo ni reembolso.",
		}
		if err := a.store.upsert("reservas", res); err != nil {
			return 0, nil, err
		}
		return http.StatusOK, M{"data": res, "isWithinFreeWindow": true, "refund": nil, "penaltyReceipt": nil}, nil
	}
	cfg, err := a.clubConfig()
	if err != nil {
		return 0, nil, err
	}
	freeHours := f(cfg, "freeCancellationHours")
	retentionPct := f(cfg, "lateCancellationRetentionPercent")
	within := hours >= freeHours
	policy := fmt.Sprintf("Cancelación tardía (<%shs): Se aplica retención del %s%% de seña por costo operativo.", numStr(freeHours), numStr(retentionPct))
	if within {
		policy = fmt.Sprintf("Cancelación anticipada (>=%shs): Reembolso de seña según política.", numStr(freeHours))
	}
	res["status"] = resCancelled
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
	refundID, err := a.nextID("reembolsos", nil)
	if err != nil {
		return 0, nil, err
	}
	refund := M{
		"id":                   refundID,
		"clientName":           name,
		"clientDni":            dni,
		"relatedReservationId": id,
		"relatedReceiptId":     res["depositReceiptNumber"],
		"originalAmount":       deposit,
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
		retained := math.Round(deposit * retentionPct / 100)
		receiptID, err := a.nextID("recibos", nil)
		if err != nil {
			return 0, nil, err
		}
		receipt := M{
			"id":            receiptID,
			"reservationId": id,
			"clientName":    name,
			"clientDni":     dni,
			"concept":       "Cargo por cancelación fuera de plazo - " + s(res, "courtName"),
			"items": []any{
				M{"description": fmt.Sprintf("Seña abonada (%s)", id), "amount": deposit},
				M{"description": fmt.Sprintf("Retención %s%% por cancelación con menos de %s hs", numStr(retentionPct), numStr(freeHours)), "amount": retained},
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
		refund["reason"] = fmt.Sprintf("Cancelación fuera de plazo (%s hs). Se retiene el %s%% de la seña y se reintegra el resto.", numStr(hours), numStr(retentionPct))
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
// Documentos (reglamentos internos y títulos habilitantes)
// ===========================================================================

// Los archivos se guardan en base64 dentro de la misma base: es lo único que hay que respaldar.
const documentsCollection = "documentos"

const maxDocumentBytes = 5 << 20

var documentTypes = []string{"application/pdf", "image/jpeg", "image/png"}

// documentRef es lo que guardan torneos, ligas y fichas de profesores: sin el contenido.
func documentRef(doc M) M {
	return M{"id": s(doc, "id"), "name": s(doc, "name"), "mimeType": s(doc, "mimeType"), "size": f(doc, "size")}
}

// resolveDocument valida que ref apunte a un documento cargado y devuelve la referencia normalizada.
func (a *app) resolveDocument(ref any) (M, error) {
	id := s(asMap(ref), "id")
	if id == "" {
		return nil, badRequest("Falta el documento adjunto.")
	}
	doc, ok, err := a.store.get(documentsCollection, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, badRequest(fmt.Sprintf("El documento %s no existe.", id))
	}
	return documentRef(doc), nil
}

// POST /api/documentos  { name, data (base64) }
func (a *app) uploadDocument(r *http.Request) (int, any, error) {
	// El base64 ocupa 4/3 del archivo; el margen cubre el resto del JSON.
	r.Body = http.MaxBytesReader(nil, r.Body, maxDocumentBytes*4/3+64<<10)
	body, err := readObject(r)
	if err != nil {
		// decodeBody convierte el error en texto, así que se reconoce por el mensaje de MaxBytesReader.
		if strings.Contains(err.Error(), "request body too large") {
			return 0, nil, badRequest("El archivo supera el máximo de 5 MB.")
		}
		return 0, nil, err
	}
	name := strings.TrimSpace(s(body, "name"))
	if name == "" {
		return 0, nil, badRequest("El nombre del archivo es obligatorio.")
	}
	raw, err := base64.StdEncoding.DecodeString(s(body, "data"))
	if err != nil || len(raw) == 0 {
		return 0, nil, badRequest("El contenido del archivo no es base64 válido.")
	}
	if len(raw) > maxDocumentBytes {
		return 0, nil, badRequest("El archivo supera el máximo de 5 MB.")
	}
	// El tipo sale del contenido, no de lo que declara el cliente.
	mimeType := http.DetectContentType(raw)
	if !slices.Contains(documentTypes, mimeType) {
		return 0, nil, badRequest("Solo se aceptan archivos PDF, JPG o PNG.")
	}
	doc := M{
		"id":         fmt.Sprintf("doc-%d", nowMillis()),
		"name":       name,
		"mimeType":   mimeType,
		"size":       float64(len(raw)),
		"data":       s(body, "data"),
		"uploadedAt": nowISO(),
	}
	for n := 2; ; n++ {
		taken, err := a.store.exists(documentsCollection, s(doc, "id"))
		if err != nil {
			return 0, nil, err
		}
		if !taken {
			break
		}
		doc["id"] = fmt.Sprintf("doc-%d-%d", nowMillis(), n)
	}
	if err := a.store.insert(documentsCollection, doc); err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, documentRef(doc), nil
}

// GET /api/documentos/{id} — devuelve el archivo tal cual (no JSON).
func (a *app) downloadDocument(w http.ResponseWriter, r *http.Request) {
	doc, err := a.store.require(documentsCollection, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	raw, err := base64.StdEncoding.DecodeString(s(doc, "data"))
	if err != nil {
		writeError(w, internalError("El documento guardado está dañado."))
		return
	}
	w.Header().Set("Content-Type", s(doc, "mimeType"))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": s(doc, "name")}))
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.Write(raw)
}

// ===========================================================================
// Torneos y ligas
// ===========================================================================
// enrolledByDni informa si el DNI ya figura entre los inscriptos, como titular o como pareja.
func enrolledByDni(participants []any, dni string) bool {
	target := cleanDNI(dni)
	if target == "" {
		return false
	}
	for _, p := range participants {
		pm := asMap(p)
		if cleanDNI(s(pm, "dni")) == target || cleanDNI(s(pm, "partnerDni")) == target {
			return true
		}
	}
	return false
}

// checkEnrollment valida cupo e inscripción repetida de un torneo o una liga.
func checkEnrollment(doc, body M, what string) error {
	list := arr(doc, "participants")
	if enrolledByDni(list, s(body, "dni")) || enrolledByDni(list, s(body, "partnerDni")) {
		return conflict(fmt.Sprintf("Ya hay una inscripción a %s con ese DNI.", what))
	}
	if max := f(doc, "maxParticipants"); max > 0 && float64(len(list)) >= max {
		return conflict(fmt.Sprintf("%s alcanzó el cupo máximo de inscriptos.", strings.ToUpper(what[:1])+what[1:]))
	}
	return nil
}

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
	if _, has := body["internalRules"]; has {
		doc["internalRules"] = s(body, "internalRules")
	}
	// RF041 / RF048: documento del reglamento; null lo quita.
	if ref, has := body["internalRulesDocument"]; has {
		if ref == nil {
			delete(doc, "internalRulesDocument")
		} else {
			resolved, err := a.resolveDocument(ref)
			if err != nil {
				return 0, nil, err
			}
			doc["internalRulesDocument"] = resolved
		}
	}
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
	if err := checkEnrollment(t, body, "este torneo"); err != nil {
		return 0, nil, err
	}
	list := arr(t, "participants")
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
	if err := checkEnrollment(l, body, "esta liga"); err != nil {
		return 0, nil, err
	}
	list := arr(l, "participants")
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

// PUT /api/profesores/{id}/certificacion  { titleName, institution, licenseNumber, issueDate, expiryDate, status, document }
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
	if err := a.normalizeLicense(c); err != nil {
		return 0, nil, err
	}
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
	for _, st := range students {
		if dni := cleanDNI(s(body, "dni")); dni != "" && cleanDNI(s(asMap(st), "dni")) == dni {
			return 0, nil, conflict(fmt.Sprintf("Ya hay un alumno inscripto en \"%s\" con ese DNI.", s(cls, "name")))
		}
	}
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
		next := map[string]string{"P": "A", "A": "P"}[cur]
		att[idx] = orDefault(next, "P")
		st["attendance"] = att
	}
	if err := a.store.upsert("clases", cls); err != nil {
		return 0, nil, err
	}
	return a.attendanceResponse(r)
}

// La asistencia se registra como P (presente) o A (ausente).
func checkAttendanceStatus(status string) error {
	if status != "P" && status != "A" {
		return badRequest("El estado de asistencia debe ser P (presente) o A (ausente).")
	}
	return nil
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
	if err := checkAttendanceStatus(status); err != nil {
		return 0, nil, err
	}
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
	status := s(body, "status")
	if err := checkAttendanceStatus(status); err != nil {
		return 0, nil, err
	}
	for _, sti := range arr(cls, "students") {
		if st, ok := sti.(M); ok && s(st, "id") == sid {
			st["attendance"] = append(arr(st, "attendance"), status)
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
	defaultStr(d, "concept", s(items[0].(M), "description"))
	defaultStr(d, "paymentMethod", "Tarjeta de Débito")
	defaultStr(d, "issuedBy", "Administración General")
	if s(d, "relatedReceiptId") == "" {
		d["relatedReceiptId"] = nil
	}
	d["items"] = items
	d["subtotal"] = subtotal
	d["total"] = subtotal
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
	defaultStr(d, "processedBy", "Administración General")
	for _, k := range []string{"relatedReservationId", "relatedReceiptId"} {
		if s(d, k) == "" {
			d[k] = nil
		}
	}
	d["date"] = nowStamp()
	return d, nil
}

// GET /api/socios: padrón reducido para elegir compañeros al reservar. Solo lo necesario para
// registrar a un participante; teléfono, email y dirección quedan para el personal.
func (a *app) listMembers(_ *http.Request) (int, any, error) {
	users, err := a.store.list("usuarios")
	if err != nil {
		return 0, nil, err
	}
	out := []M{}
	for _, u := range users {
		if s(u, "role") != "client" {
			continue
		}
		out = append(out, M{"id": u["id"], "name": u["name"], "dni": u["dni"], "memberNumber": u["memberNumber"]})
	}
	return http.StatusOK, out, nil
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
