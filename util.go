package main

// Helpers para trabajar con documentos JSON libres (map[string]any), con la misma forma que usa
// el front en src/utils/mockData.js.

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type M = map[string]any

// s lee una clave como string (vacío si no existe o no es string).
func s(m M, key string) string {
	v, _ := m[key].(string)
	return v
}

// f lee una clave numérica como float64 (0 si no existe). Acepta números y strings numéricos.
func f(m M, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case string:
		x, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0
		}
		return x
	}
	return 0
}

func b(m M, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

// asMap devuelve el objeto o un mapa vacío si el valor no es un objeto.
func asMap(v any) M {
	if m, ok := v.(M); ok {
		return m
	}
	return M{}
}

func arr(m M, key string) []any {
	v, _ := m[key].([]any)
	return v
}

// merge es una mezcla superficial: las claves de patch pisan las de base (igual que `{ ...base, ...patch }`).
func merge(base, patch M) M {
	out := make(M, len(base)+len(patch))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		out[k] = v
	}
	return out
}

// cleanDNI deja solo los dígitos, para comparar DNI cargados con o sin puntos.
func cleanDNI(v string) string {
	return digitsOnly(v)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// numStr formatea un número como lo hace el front: `1` para 1.0, `1.5` para 1.5.
func numStr(x float64) string {
	return strconv.FormatFloat(x, 'f', -1, 64)
}

// marshalJSON no escapa <, > ni & (encoding/json lo hace por defecto y el front no lo espera).
func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}

// nowStamp devuelve `2026-10-23 16:46` (misma forma que el mock: ISO sin segundos, separado por espacio).
func nowStamp() string {
	return time.Now().Format("2006-01-02 15:04")
}

func today() string {
	return time.Now().Format("2006-01-02")
}

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func year() int {
	return time.Now().Year()
}

// rand4 es un pseudo-aleatorio simple para números de recibo tipo `REC-50-8321`.
func rand4() int64 {
	x := uint64(nowMillis()) ^ 0x9E3779B97F4A7C15
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	return 1000 + int64(x%9000)
}
