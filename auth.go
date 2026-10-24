package main

// Autenticación: login / registro con bcrypt + JWT HS256 (firmado a mano con crypto/hmac).
// El front guarda el token y lo manda como `Bearer`.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	defaultPassword = "123456"
	tokenTTLSecs    = 8 * 60 * 60 // 8 horas
)

type Claims struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Dni     string `json:"dni,omitempty"`
	CoachID string `json:"coachId,omitempty"`
	Iat     int64  `json:"iat"`
	Exp     int64  `json:"exp"`
}

type claimsKey struct{}

// ---------------------------------------------------------------------------
// JWT
// ---------------------------------------------------------------------------
var b64 = base64.RawURLEncoding

func hmacSHA256(secret, input string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(input))
	return mac.Sum(nil)
}

func signToken(secret string, claims Claims) string {
	header := b64.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadJSON, _ := json.Marshal(claims)
	signingInput := header + "." + b64.EncodeToString(payloadJSON)
	return signingInput + "." + b64.EncodeToString(hmacSHA256(secret, signingInput))
}

func verifyToken(secret, token string) (Claims, string) {
	var claims Claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, "Formato de token inválido"
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return claims, "Firma ilegible"
	}
	if !hmac.Equal(sig, hmacSHA256(secret, parts[0]+"."+parts[1])) {
		return claims, "Firma inválida (¿token de Modo Demo?)"
	}
	payload, err := b64.DecodeString(parts[1])
	if err != nil {
		return claims, "Payload ilegible"
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, "Claims inválidos"
	}
	if claims.Exp < nowMillis()/1000 {
		return claims, "Token vencido"
	}
	return claims, ""
}

// requireAuth exige `Authorization: Bearer <jwt>` válido en todas las rutas protegidas.
func (a *app) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeError(w, unauthorized("Falta el token de autenticación (Authorization: Bearer ...)."))
			return
		}
		claims, problem := verifyToken(a.jwtSecret, token)
		if problem != "" {
			writeError(w, unauthorized("Token rechazado: "+problem))
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims)))
	}
}

// ---------------------------------------------------------------------------
// Usuarios <-> AuthResponseDto
// ---------------------------------------------------------------------------
func hashPassword(plain string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), 4)
	if err != nil {
		return ""
	}
	return string(h)
}

// backendRole traduce el rol del usuario; el de un profesional sale del tipo de su ficha.
func (a *app) backendRole(user M) (string, error) {
	switch s(user, "role") {
	case "admin":
		return rolEmpleado, nil
	case "coach":
		coach, err := a.store.require("profesores", s(user, "coachId"))
		if err != nil {
			return "", err
		}
		if s(coach, "type") == rolEntrenador {
			return rolEntrenador, nil
		}
		return rolProfesor, nil
	}
	return rolSocio, nil
}

func splitName(user M) (string, string) {
	nombre, apellido := s(user, "nombre"), s(user, "apellido")
	if nombre != "" || apellido != "" {
		return nombre, apellido
	}
	first, rest, _ := strings.Cut(s(user, "name"), " ")
	return first, rest
}

// authResponse tiene la misma forma que el `AuthResponseDto` del backend .NET original, más `dni` y `memberNumber`.
func (a *app) authResponse(user M) (M, error) {
	nombre, apellido := splitName(user)
	role, err := a.backendRole(user)
	if err != nil {
		return nil, err
	}
	now := nowMillis() / 1000
	claims := Claims{
		Sub:     s(user, "id"),
		Email:   s(user, "email"),
		Role:    role,
		Dni:     s(user, "dni"),
		CoachID: s(user, "coachId"),
		Iat:     now,
		Exp:     now + tokenTTLSecs,
	}
	return M{
		"token":          signToken(a.jwtSecret, claims),
		"id":             s(user, "id"),
		"nombre":         nombre,
		"apellido":       apellido,
		"email":          s(user, "email"),
		"numeroTelefono": s(user, "phone"),
		"direccion":      s(user, "address"),
		"rol":            role,
		"dni":            s(user, "dni"),
		"coachId":        s(user, "coachId"),
		"memberNumber":   s(user, "memberNumber"),
		"expiraEn":       claims.Exp,
	}, nil
}

// POST /api/auth/login
func (a *app) login(r *http.Request) (int, any, error) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &body); err != nil {
		return 0, nil, err
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	users, err := a.store.list("usuarios")
	if err != nil {
		return 0, nil, err
	}
	for _, u := range users {
		if strings.ToLower(s(u, "email")) != email {
			continue
		}
		hash := s(u, "passwordHash")
		if hash == "" || bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)) != nil {
			break
		}
		res, err := a.authResponse(u)
		if err != nil {
			return 0, nil, err
		}
		return http.StatusOK, res, nil
	}
	return 0, nil, unauthorized("Credenciales inválidas.")
}

// POST /api/auth/registrar — crea un usuario con rol Socio.
func (a *app) register(r *http.Request) (int, any, error) {
	var body struct {
		Nombre         string `json:"nombre"`
		Apellido       string `json:"apellido"`
		Email          string `json:"email"`
		Password       string `json:"password"`
		Direccion      string `json:"direccion"`
		NumeroTelefono string `json:"numeroTelefono"`
		Dni            string `json:"dni"`
	}
	if err := decodeBody(r, &body); err != nil {
		return 0, nil, err
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" || len(body.Password) < 4 {
		return 0, nil, badRequest("Email y contraseña (mínimo 4 caracteres) son obligatorios.")
	}
	users, err := a.store.list("usuarios")
	if err != nil {
		return 0, nil, err
	}
	for _, u := range users {
		if strings.ToLower(s(u, "email")) == email {
			return 0, nil, conflict("Ya existe un usuario registrado con ese email.")
		}
	}
	nombre, apellido := strings.TrimSpace(body.Nombre), strings.TrimSpace(body.Apellido)
	user := M{
		"id":              fmt.Sprintf("usr-%d", nowMillis()),
		"name":            strings.TrimSpace(nombre + " " + apellido),
		"nombre":          nombre,
		"apellido":        apellido,
		"dni":             body.Dni,
		"phone":           body.NumeroTelefono,
		"email":           email,
		"address":         body.Direccion,
		"role":            "client",
		"memberNumber":    fmt.Sprintf("TA-%d", 1000+nowMillis()%9000),
		"memberSince":     today(),
		"activeDiscounts": []any{},
		"totalBookings":   0,
		"passwordHash":    hashPassword(body.Password),
	}
	if err := a.store.insert("usuarios", user); err != nil {
		return 0, nil, err
	}
	res, err := a.authResponse(user)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, res, nil
}

// GET /api/auth/yo — devuelve los claims del token (útil para probar la autenticación).
func (a *app) me(r *http.Request) (int, any, error) {
	return http.StatusOK, r.Context().Value(claimsKey{}), nil
}
