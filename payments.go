package main

// Pasarela de pagos simulada (RD11: tarjeta de débito, tarjeta de crédito y billetera virtual con
// QR de Mercado Pago). No mueve dinero: solo acepta las tarjetas de prueba que publica Mercado Pago
// y resuelve el resultado con el nombre del titular, como el entorno de pruebas de Mercado Pago
// (APRO o cualquier otro nombre aprueba; OTHE, CALL, FUND, SECU, EXPI y FORM rechazan; CONT queda
// pendiente y no confirma la operación). De la tarjeta se guardan solo la marca y los últimos 4
// dígitos (RNF03).
//
// Cada pago aprobado queda en la colección `pagos` y se usa una sola vez: el recibo, la seña o el
// saldo que lo consume lo marca con `usedBy`.

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const paymentsCollection = "pagos"

const (
	methodQR     = "Mercado Pago (QR)"
	methodDebit  = "Tarjeta de Débito"
	methodCredit = "Tarjeta de Crédito"
)

type testCard struct{ number, brand, method, cvv string }

// Tarjetas de prueba de Mercado Pago Argentina (todas vencen 11/30).
var testCards = []testCard{
	{"5031755734530604", "Mastercard", methodCredit, "123"},
	{"4509953566233704", "Visa", methodCredit, "123"},
	{"371180303257522", "American Express", methodCredit, "1234"},
	{"5287338310253304", "Mastercard", methodDebit, "123"},
	{"4002768694395619", "Visa", methodDebit, "123"},
}

// Escenarios de rechazo que se fuerzan con el nombre del titular.
var holderScenarios = map[string]string{
	"OTHE": "Pago rechazado por error general.",
	"CALL": "Pago rechazado: el titular tiene que autorizarlo con el emisor de la tarjeta.",
	"FUND": "Pago rechazado por saldo insuficiente.",
	"SECU": "Pago rechazado: el código de seguridad es inválido.",
	"EXPI": "Pago rechazado por un problema con la fecha de vencimiento.",
	"FORM": "Pago rechazado: los datos de la tarjeta son inválidos.",
	"CONT": "El pago quedó pendiente de acreditación y la operación no se confirma (RD01).",
}

func paymentRejected(msg string) error { return &apiError{http.StatusPaymentRequired, msg} }

func digitsOnly(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cardExpired interpreta "MM/AA" y dice si la tarjeta ya venció (vale hasta el último día del mes).
func cardExpired(expiry string) (bool, error) {
	mm, yy, ok := strings.Cut(strings.TrimSpace(expiry), "/")
	month, errM := strconv.Atoi(strings.TrimSpace(mm))
	year, errY := strconv.Atoi(strings.TrimSpace(yy))
	if !ok || errM != nil || errY != nil || month < 1 || month > 12 {
		return false, badRequest("El vencimiento de la tarjeta debe tener el formato MM/AA.")
	}
	if year < 100 {
		year += 2000
	}
	firstOfNext := time.Date(year, time.Month(month)+1, 1, 0, 0, 0, 0, time.Local)
	return !time.Now().Before(firstOfNext), nil
}

// authorizeCard valida la tarjeta ficticia y devuelve la marca y los últimos 4 dígitos.
func authorizeCard(method string, card M) (M, error) {
	number := digitsOnly(s(card, "number"))
	var tc *testCard
	for i := range testCards {
		if testCards[i].number == number {
			tc = &testCards[i]
		}
	}
	if tc == nil {
		return nil, badRequest("Solo se aceptan tarjetas de prueba (ficticias), por ejemplo Visa crédito 4509 9535 6623 3704 o Visa débito 4002 7686 9439 5619.")
	}
	if tc.method != method {
		return nil, badRequest(fmt.Sprintf("La tarjeta ingresada es %s: elegí %s como medio de pago.", strings.ToLower(strings.TrimPrefix(tc.method, "Tarjeta ")), tc.method))
	}
	holder := strings.ToUpper(strings.TrimSpace(s(card, "holder")))
	if holder == "" {
		return nil, badRequest("Falta el nombre del titular de la tarjeta.")
	}
	expired, err := cardExpired(s(card, "expiry"))
	if err != nil {
		return nil, err
	}
	if expired {
		return nil, paymentRejected("Pago rechazado: la tarjeta está vencida.")
	}
	if digitsOnly(s(card, "cvv")) != tc.cvv {
		return nil, paymentRejected(holderScenarios["SECU"])
	}
	if msg, ok := holderScenarios[holder]; ok {
		return nil, paymentRejected(msg)
	}
	return M{"cardBrand": tc.brand, "cardLast4": number[len(number)-4:]}, nil
}

func randomDigits(n int) string {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(math.Pow10(n))))
	if err != nil {
		return fmt.Sprintf("%0*d", n, nowMillis()%int64(math.Pow10(n)))
	}
	return fmt.Sprintf("%0*d", n, v.Int64())
}

// POST /api/pagos  { paymentMethod, amount, concept, card?: { number, holder, expiry, cvv } }
// Autoriza el pago con la pasarela simulada. 201 con la transacción aprobada; 402 si se rechaza.
func (a *app) createPayment(r *http.Request) (int, any, error) {
	body, err := readObject(r)
	if err != nil {
		return 0, nil, err
	}
	amount := math.Round(f(body, "amount")*100) / 100
	if amount <= 0 {
		return 0, nil, badRequest("El importe a pagar debe ser mayor a cero.")
	}
	method := s(body, "paymentMethod")
	tx := M{
		"status":        "approved",
		"paymentMethod": method,
		"amount":        amount,
		"concept":       strings.TrimSpace(s(body, "concept")),
		"createdAt":     nowISO(),
		"createdBy":     claimsOf(r).Sub,
		"usedBy":        nil,
	}
	switch method {
	case methodQR:
		// La billetera confirma el pago al escanear el QR; acá se simula esa notificación aprobada.
	case methodDebit, methodCredit:
		card, err := authorizeCard(method, asMap(body["card"]))
		if err != nil {
			return 0, nil, err
		}
		tx = merge(tx, card)
	default:
		return 0, nil, badRequest("El medio de pago debe ser Tarjeta de Débito, Tarjeta de Crédito o Mercado Pago (QR) (RD11).")
	}
	for {
		tx["id"] = "MP-" + randomDigits(8)
		taken, err := a.store.exists(paymentsCollection, s(tx, "id"))
		if err != nil {
			return 0, nil, err
		}
		if !taken {
			break
		}
	}
	if err := a.store.insert(paymentsCollection, tx); err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, tx, nil
}

// consumePayment valida que el pago exista, esté aprobado, sea de quien lo usa (el personal puede
// usar cualquiera), no se haya usado y cubra el importe; lo marca como usado por ref.
// Hay que llamarlo después de las demás validaciones, justo antes de guardar.
func (a *app) consumePayment(c Claims, txID string, amount float64, ref string) (M, error) {
	if txID == "" {
		return nil, badRequest("Falta el pago: primero hay que abonar con tarjeta o billetera virtual (RD01).")
	}
	tx, ok, err := a.store.get(paymentsCollection, txID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, badRequest(fmt.Sprintf("El pago %s no existe.", txID))
	}
	switch {
	case s(tx, "status") != "approved":
		return nil, badRequest(fmt.Sprintf("El pago %s no está aprobado.", txID))
	case !c.staff() && s(tx, "createdBy") != c.Sub:
		return nil, forbidden("El pago no es tuyo.")
	case s(tx, "usedBy") != "":
		return nil, conflict(fmt.Sprintf("El pago %s ya se usó en %s.", txID, s(tx, "usedBy")))
	case math.Abs(f(tx, "amount")-amount) >= 0.01:
		return nil, badRequest(fmt.Sprintf("El pago %s es por $%s y el importe a cobrar es $%s.", txID, numStr(f(tx, "amount")), numStr(amount)))
	}
	tx["usedBy"] = ref
	tx["usedAt"] = nowISO()
	if err := a.store.upsert(paymentsCollection, tx); err != nil {
		return nil, err
	}
	return tx, nil
}

// paymentFields son los datos del pago que se copian al recibo.
func paymentFields(tx M) M {
	out := M{"paymentMethod": s(tx, "paymentMethod"), "transactionId": s(tx, "id")}
	if last4 := s(tx, "cardLast4"); last4 != "" {
		out["cardBrand"] = s(tx, "cardBrand")
		out["cardLast4"] = last4
	}
	return out
}

// attachPayment exige y consume el pago de un recibo dado de alta por POST /api/recibos.
func (a *app) attachPayment(c Claims, receipt M) error {
	if f(receipt, "totalPaid") <= 0 {
		return badRequest("El importe del recibo debe ser mayor a cero.")
	}
	tx, err := a.consumePayment(c, s(receipt, "transactionId"), f(receipt, "totalPaid"), "recibos/"+s(receipt, "id"))
	if err != nil {
		return err
	}
	for k, v := range paymentFields(tx) {
		receipt[k] = v
	}
	return nil
}
