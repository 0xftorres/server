package main

// Carga inicial: si la base está vacía, inserta los datos de `seed/seed.json`
// (exportado desde `src/utils/mockData.js` del front) y les asigna la contraseña por defecto
// a todos los usuarios.

import (
	"encoding/json"
	"fmt"
	"os"
)

// collections son las colecciones conocidas (también se usan para validar las rutas genéricas).
var collections = []string{
	"usuarios",
	"canchas",
	"tipos-cancha",
	"stock",
	"reservas",
	"torneos",
	"ligas",
	"tipos-competencia",
	"profesores",
	"clases",
	"clases-particulares",
	"recibos",
	"facturas",
	"reembolsos",
	"descuentos",
}

func loadSeedFile(path string) (M, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, internalError(fmt.Sprintf("No se pudo leer el seed %s: %v", path, err))
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, serializationError(err)
	}
	seed, ok := value.(M)
	if !ok {
		return nil, internalError("El seed debe ser un objeto { coleccion: [...] }")
	}
	return seed, nil
}

// applySeed inserta el seed completo. Devuelve la cantidad de documentos cargados.
func applySeed(st *Store, seed M) (int, error) {
	passwordHash := hashPassword(defaultPassword)
	total := 0
	for _, col := range collections {
		for _, item := range arr(seed, col) {
			doc, ok := item.(M)
			if !ok {
				continue
			}
			if col == "usuarios" && s(doc, "passwordHash") == "" {
				doc["passwordHash"] = passwordHash
			}
			if s(doc, "id") == "" {
				continue
			}
			if err := st.upsert(col, doc); err != nil {
				return total, err
			}
			total++
		}
	}
	return total, nil
}

func seedIfEmpty(st *Store, path string) (int, error) {
	empty, err := st.isEmpty()
	if err != nil || !empty {
		return 0, err
	}
	seed, err := loadSeedFile(path)
	if err != nil {
		return 0, err
	}
	return applySeed(st, seed)
}
