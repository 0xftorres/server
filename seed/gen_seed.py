# -*- coding: utf-8 -*-
"""Genera server/seed/seed.json y tenis-ahora-frontend/src/utils/mockData.js con el mismo dataset."""
import base64
import json
import random
import sys

SERVER_SEED = r"C:\Users\Administrator\Desktop\TenisAhora\server\seed\seed.json"
FRONT_MOCK = r"C:\Users\Administrator\Desktop\TenisAhora\tenis-ahora-frontend\src\utils\mockData.js"

random.seed(2026)

# ---------------------------------------------------------------- canchas
COURT_TYPES = [
    {"id": "ct-ladrillo", "name": "Ladrillo", "label": "Polvo de ladrillo", "description": "Superficie lenta de arcilla roja. Ideal para peloteo largo y juego de fondo.", "speed": "Lenta", "basePricePerHour": 4800, "maintenanceFrequency": "Diaria (riego y pasada de alfombra)", "maxStudentsPerClass": 30, "active": True},
    {"id": "ct-cemento", "name": "Cemento", "label": "Cemento", "description": "Superficie dura de pique medio-rápido. Bajo mantenimiento.", "speed": "Media-rápida", "basePricePerHour": 4200, "maintenanceFrequency": "Semanal (barrido y revisión de fisuras)", "maxStudentsPerClass": 20, "active": True},
    {"id": "ct-pasto", "name": "Pasto", "label": "Césped natural", "description": "Superficie rápida de pique bajo. No se usa en días de lluvia.", "speed": "Rápida", "basePricePerHour": 6500, "maintenanceFrequency": "Diaria (corte, riego y marcado de líneas)", "maxStudentsPerClass": 12, "active": True},
]
PRICE = {t["name"]: t["basePricePerHour"] for t in COURT_TYPES}

PHOTOS = [
    "https://images.unsplash.com/photo-1595435934249-5df7ed86e1c0?auto=format&fit=crop&w=600&q=80",
    "https://images.unsplash.com/photo-1554068865-24cecd4e34b8?auto=format&fit=crop&w=600&q=80",
    "https://images.unsplash.com/photo-1622279457486-62dcc4a431d6?auto=format&fit=crop&w=600&q=80",
    "https://images.unsplash.com/photo-1560012057-4372e14c5085?auto=format&fit=crop&w=600&q=80",
    "https://images.unsplash.com/photo-1534438327276-14e5300c3a48?auto=format&fit=crop&w=600&q=80",
]
DAYS = ["Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado", "Domingo"]


def court(n, idol, surface, description, status="disponible", maintenance="", notes="Horario corrido. Iluminación artificial desde las 19:00."):
    return {
        "id": f"cancha-{n}",
        "name": f"Cancha {n} - {idol}",
        "surfaceType": surface,
        "capacity": 4,
        "pricePerHour": PRICE[surface],
        "status": status,
        "description": description,
        "maintenanceNotes": maintenance,
        "availability": {"days": list(DAYS), "openTime": "08:00", "closeTime": "22:00", "slotMinutes": 60, "blockedRanges": [], "notes": notes},
        "photo": PHOTOS[n - 1],
    }


COURTS = [
    court(1, "Lionel Messi", "Ladrillo", "Cancha central de polvo de ladrillo, con tribuna."),
    court(2, "Dibu Martínez", "Ladrillo", "Polvo de ladrillo, ideal para singles y dobles."),
    court(3, "Ángel Di María", "Cemento", "Superficie dura de pique rápido y uniforme."),
    court(4, "Julián Álvarez", "Cemento", "Superficie dura. Actualmente en repintado de líneas.", status="mantenimiento", maintenance="Repintado de líneas y cambio de red hasta el 30/10."),
    court(5, "Rodrigo De Paul", "Pasto", "Césped natural, uso con calzado adecuado.", notes="Cerrada los días de lluvia."),
]
COURT_BY_ID = {c["id"]: c for c in COURTS}


def court_label(cid):
    c = COURT_BY_ID[cid]
    return f'{c["name"]} ({c["surfaceType"]})'


def court_short(cid):
    c = COURT_BY_ID[cid]
    return f'{c["name"].split(" - ")[0]} ({c["surfaceType"]})'


# ---------------------------------------------------------------- personas
# (nombre, dni, telefono, email, direccion, memberSince, descuentos, reservas)
CLIENTS = [
    ("Juan Sánchez", "35.678.912", "+54 11 5234-7781", "socio@tenisahora.com", "Mitre 1240, Quilmes", "2024-03-10", ["PAQ10"], 14),
    ("Melina Romo", "39.104.556", "+54 11 6012-3345", "melina.romo@gmail.com", "Av. Calchaquí 2350, Quilmes", "2024-05-02", ["LIGA20"], 9),
    ("Lucas Pereyra", "36.887.213", "+54 11 4477-9120", "lucas.pereyra@hotmail.com", "Lavalle 780, Bernal", "2023-08-14", ["LIGA20"], 21),
    ("Sofía Acosta", "40.223.871", "+54 11 3390-4412", "sofia.acosta@gmail.com", "Rivadavia 455, Berazategui", "2025-02-18", [], 5),
    ("Matías Giménez", "34.556.902", "+54 11 5578-2201", "matias.gimenez@gmail.com", "Belgrano 1920, Quilmes", "2023-04-03", ["PAQ10", "LIGA20"], 27),
    ("Camila Rodríguez", "41.009.338", "+54 11 2265-7730", "camila.rodriguez@outlook.com", "Alsina 310, Bernal", "2025-06-09", [], 3),
    ("Nicolás Ledesma", "37.345.610", "+54 11 6643-8815", "nicolas.ledesma@gmail.com", "San Martín 2210, Quilmes Oeste", "2023-11-27", ["LIGA20"], 18),
    ("Valentina Ríos", "42.118.405", "+54 11 3812-6604", "valentina.rios@gmail.com", "Moreno 1015, Ezpeleta", "2025-03-12", ["EST10"], 6),
    ("Facundo Molina", "33.790.221", "+54 11 4920-1187", "facundo.molina@yahoo.com.ar", "Garibaldi 640, Quilmes", "2022-10-05", ["LIGA20"], 33),
    ("Agustina Herrera", "38.902.774", "+54 11 5109-3362", "agustina.herrera@gmail.com", "Brown 1580, Bernal", "2024-08-21", ["PAQ10"], 12),
    ("Tomás Benítez", "39.667.130", "+54 11 6788-0241", "tomas.benitez@gmail.com", "Colón 925, Quilmes", "2024-09-30", ["LIGA20"], 15),
    ("Florencia Castro", "36.230.198", "+54 11 4056-7793", "florencia.castro@hotmail.com", "Sarmiento 1330, Berazategui", "2023-02-14", [], 11),
    ("Ezequiel Núñez", "35.114.887", "+54 11 5531-9906", "ezequiel.nunez@gmail.com", "Andrés Baranda 2040, Quilmes", "2023-06-20", ["LIGA20"], 24),
    ("Julieta Vega", "43.045.612", "+54 11 2974-5518", "julieta.vega@gmail.com", "Humberto Primo 715, Bernal", "2025-08-04", ["EST10"], 2),
    ("Franco Medina", "37.998.045", "+54 11 6207-4439", "franco.medina@gmail.com", "Pringles 450, Quilmes", "2024-01-16", ["LIGA20"], 19),
    ("Martina Suárez", "40.771.209", "+54 11 3345-8820", "martina.suarez@gmail.com", "Olavarría 1180, Ezpeleta", "2024-11-11", [], 7),
    ("Ignacio Torres", "34.402.566", "+54 11 4489-1073", "ignacio.torres@gmail.com", "Videla 870, Quilmes", "2023-09-08", ["PAQ10", "LIGA20"], 29),
]
ADMIN = ("Carolina Peralta", "30.512.774", "+54 11 4253-8800", "admin@tenisahora.com", "Sede Club Tenis Ahora, Quilmes", "2022-01-01")

# (id, nombre, tipo, dni, telefono, email, horarios, titulo, institucion, licencia, emision, vencimiento, archivo)
COACHES = [
    ("coach-01", "Gustavo Ferrari", "Profesor", "28.334.901", "+54 11 5566-7788", "profesor@tenisahora.com",
     "Lunes a Viernes 08:00 - 13:00 y 17:00 - 21:00",
     "Profesor Nacional de Tenis", "Asociación Argentina de Tenis (AAT)", "AAT-PRO-41287", "2014-08-20", "2028-12-31", "Titulo_Profesor_Ferrari.pdf"),
    ("coach-02", "Laura Domínguez", "Entrenador", "31.245.678", "+54 11 4433-2211", "entrenador@tenisahora.com",
     "Lunes a Jueves 14:00 - 20:00, Sábados 09:00 - 13:00",
     "Entrenadora Nacional de Tenis Nivel 1", "Asociación Argentina de Tenis (AAT)", "AAT-ENT-50932", "2018-03-12", "2029-03-31", "Certificado_Entrenadora_Dominguez.pdf"),
    ("coach-03", "Diego Paredes", "Entrenador", "29.871.340", "+54 11 6677-9922", "diego.paredes@tenisahora.com",
     "Martes a Sábados 10:00 - 18:00",
     "Entrenador Nacional de Tenis Nivel 2", "Asociación Argentina de Tenis (AAT)", "AAT-ENT-47710", "2016-09-10", "2027-12-31", "Certificado_Entrenador_Paredes.pdf"),
]

PERSON = {c[0]: c for c in CLIENTS}


def dni(name):
    return PERSON[name][1]


def phone_short(name):
    return PERSON[name][2].replace("+54 ", "")


USERS = []
for i, (name, d, ph, email, addr, since, discounts, bookings) in enumerate(CLIENTS, start=1):
    USERS.append({
        "id": f"usr-{i:02d}", "name": name, "dni": d, "phone": ph, "email": email, "address": addr,
        "role": "client", "memberNumber": f"TA-{1000 + i}", "memberSince": since,
        "activeDiscounts": discounts, "totalBookings": bookings,
    })
USERS.insert(1, {
    "id": "usr-admin", "name": ADMIN[0], "dni": ADMIN[1], "phone": ADMIN[2], "email": ADMIN[3], "address": ADMIN[4],
    "role": "admin", "memberNumber": "ADM-001", "memberSince": ADMIN[5], "activeDiscounts": [], "totalBookings": 0,
})
for n, c in enumerate(COACHES, start=1):
    USERS.append({
        "id": f"usr-{c[0]}", "name": c[1], "dni": c[3], "phone": c[4], "email": c[5], "address": "Quilmes, Buenos Aires",
        "role": "coach", "coachId": c[0], "memberNumber": f"DOC-{n:02d}", "memberSince": "2024-01-15",
        "activeDiscounts": [], "totalBookings": 0,
    })


# ---------------------------------------------------------------- documentos (PDF mínimo por profesor)
def pdf_text(lines):
    def esc(t):
        return t.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)")
    content = "BT /F1 14 Tf 72 760 Td 18 TL " + " ".join(f"({esc(l)}) Tj T*" for l in lines) + " ET"
    content_b = content.encode("cp1252")
    objs = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
        b"<< /Length " + str(len(content_b)).encode() + b" >>\nstream\n" + content_b + b"\nendstream",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
    ]
    out = b"%PDF-1.4\n"
    offsets = []
    for i, o in enumerate(objs, start=1):
        offsets.append(len(out))
        out += f"{i} 0 obj\n".encode() + o + b"\nendobj\n"
    xref = len(out)
    out += f"xref\n0 {len(objs) + 1}\n".encode() + b"0000000000 65535 f \n"
    for off in offsets:
        out += f"{off:010d} 00000 n \n".encode()
    out += f"trailer\n<< /Size {len(objs) + 1} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n".encode()
    return out


DOCUMENTS = []
PROFESORES = []
for n, (cid, name, kind, d, ph, email, hours, title, inst, lic, issued, expires, filename) in enumerate(COACHES, start=1):
    pdf = pdf_text([
        "CLUB TENIS AHORA - DOCUMENTO DE MUESTRA",
        f"Título habilitante: {title}",
        f"Titular: {name}",
        f"Institución: {inst}",
        f"Licencia: {lic}",
        f"Vencimiento: {expires}",
    ])
    doc_id = f"doc-seed-{cid}"
    DOCUMENTS.append({"id": doc_id, "name": filename, "mimeType": "application/pdf", "size": len(pdf),
                      "data": base64.b64encode(pdf).decode(), "uploadedAt": "2026-10-01T12:00:00.000Z"})
    PROFESORES.append({
        "id": cid, "name": name, "type": kind, "dni": d, "phone": ph, "email": email,
        "hasOfficialLicense": True, "availableHours": hours,
        "licenseDetails": {
            "titleName": title, "institution": inst, "licenseNumber": lic, "issueDate": issued, "expiryDate": expires,
            "status": "habilitado",
            "document": {"id": doc_id, "name": filename, "mimeType": "application/pdf", "size": len(pdf)},
        },
    })
COACH_NAME = {c[0]: c[1] for c in COACHES}

# ---------------------------------------------------------------- stock
STOCK = [
    {"id": "item-redes", "name": "Redes de tenis", "category": "Redes", "totalStock": 6, "inUseStock": 5, "availableStock": 1, "unit": "unidades", "itemPrice": 0, "status": "disponible", "description": "Una por cancha habilitada; la restante es de repuesto."},
    {"id": "item-pelotas", "name": "Tubos de pelotas", "category": "Pelotas", "totalStock": 40, "inUseStock": 12, "availableStock": 28, "unit": "tubos", "itemPrice": 0, "status": "disponible", "description": "Tubos de 3 pelotas presurizadas. Se entrega uno por reserva."},
    {"id": "item-raquetas", "name": "Raquetas de préstamo", "category": "Raquetas", "totalStock": 20, "inUseStock": 6, "availableStock": 14, "unit": "raquetas", "itemPrice": 0, "status": "disponible", "description": "Raquetas para socios que no traen la propia."},
    {"id": "item-grips", "name": "Overgrips", "category": "Accesorios", "totalStock": 50, "inUseStock": 0, "availableStock": 50, "unit": "unidades", "itemPrice": 2500, "status": "disponible", "description": "Overgrips a la venta en recepción."},
]

# ---------------------------------------------------------------- descuentos
DISCOUNTS = [
    {"id": "disc-01", "name": "Paquete 10 horas mensuales", "discountPercent": 15, "code": "PAQ10", "applicableTo": "Alquiler de canchas", "active": True},
    {"id": "disc-02", "name": "Jugador de liga", "discountPercent": 20, "code": "LIGA20", "applicableTo": "Torneos y ligas", "active": True},
    {"id": "disc-03", "name": "Estudiantes", "discountPercent": 10, "code": "EST10", "applicableTo": "Clases y entrenamientos", "active": True},
]

# ---------------------------------------------------------------- tipos de competencia
COMPETITION_TYPES = [
    {"id": "cmp-elim", "name": "Torneo de eliminación directa", "format": "Cuadro eliminatorio", "appliesTo": "Torneo", "scoringSystem": "Al mejor de 3 sets con tie-break", "minParticipants": 8, "maxParticipants": 16, "description": "El perdedor queda eliminado. Cuadro de octavos, cuartos, semifinales y final.", "active": True},
    {"id": "cmp-liga", "name": "Liga todos contra todos", "format": "Round robin por puntos", "appliesTo": "Liga", "scoringSystem": "3 puntos por victoria, 0 por derrota. Desempate por diferencia de sets", "minParticipants": 4, "maxParticipants": 8, "description": "Cada jugador enfrenta a todos los demás a lo largo de las jornadas.", "active": True},
    {"id": "cmp-dobles", "name": "Torneo de dobles", "format": "Cuadro eliminatorio por parejas", "appliesTo": "Torneo", "scoringSystem": "Dos sets con punto de oro; match tie-break a 10 si empatan", "minParticipants": 4, "maxParticipants": 8, "description": "Eliminación directa entre parejas inscriptas.", "active": True},
]

# ---------------------------------------------------------------- reservas + recibos + facturas + reembolsos
RECEIPTS = []
_rec_seq = [1000]


def receipt(reservation_id, client, concept, items, total, method, date, cashier="Portal del socio", status="completado"):
    _rec_seq[0] += 1
    r = {
        "id": f"REC-2026-{_rec_seq[0]}", "reservationId": reservation_id, "clientName": client, "clientDni": dni(client),
        "concept": concept, "items": items, "totalPaid": total, "paymentMethod": method, "status": status, "date": date,
        "cashierName": cashier,
    }
    RECEIPTS.append(r)
    return r["id"]


def hhmm_plus(start, hours):
    h = int(start[:2]) + hours
    return f"{h:02d}:{start[3:]}"


RESERVATIONS = []


def reservation(n, cid, date, start, hours, lead, others, method, created, status="confirmada", cancel=None):
    c = COURT_BY_ID[cid]
    players = [lead] + others
    cost = c["pricePerHour"] * hours
    deposit = cost // 2
    rid = f"RES-2026-{n:03d}"
    label = court_label(cid)
    dep_items = [
        {"description": f"Alquiler {hours} h - {label}", "amount": cost},
        {"description": "Equipamiento incluido (red, pelotas y raquetas)", "amount": 0},
        {"description": "Seña 50 %", "amount": deposit},
    ]
    dep_rec = receipt(rid, lead, f"Seña 50 % - {label}", dep_items, deposit, method, created.replace("T", " ")[:16])
    final_rec = None
    if status == "finalizada":
        final_rec = receipt(rid, lead, f"Saldo 50 % - {label}", [{"description": f"Saldo restante alquiler {hours} h", "amount": cost - deposit}],
                            cost - deposit, method, f"{date} {hhmm_plus(start, hours)}", cashier="Recepción del Club")
    r = {
        "id": rid, "courtId": cid, "courtName": label, "surfaceType": c["surfaceType"],
        "date": date, "startTime": start, "endTime": hhmm_plus(start, hours), "durationHours": hours,
        "playersType": "dobles" if len(players) == 4 else "singles",
        "participants": [dict({"name": p, "dni": dni(p), "phone": phone_short(p)}, **({"isLead": True} if i == 0 else {})) for i, p in enumerate(players)],
        "equipmentAssigned": {"nets": 1, "ballTubes": 1, "rackets": len(players)},
        "courtCost": cost, "equipmentCost": 0, "totalCost": cost,
        "depositPaid": deposit, "depositReceiptNumber": dep_rec, "depositPaymentMethod": method,
        "remainingBalance": 0 if status == "finalizada" else cost - deposit,
        "remainingPaid": status == "finalizada", "finalReceiptNumber": final_rec,
        "status": status, "createdAt": created, "cancellationDetails": cancel,
    }
    RESERVATIONS.append(r)
    return r


MP, DEB, CRED = "Mercado Pago", "Tarjeta de Débito", "Tarjeta de Crédito"

# La primera reserva confirmada es del socio demo: el dashboard de la demo (sin filtro por usuario) muestra esa.
reservation(1, "cancha-2", "2026-10-27", "19:00", 1, "Juan Sánchez", ["Nicolás Ledesma"], MP, "2026-10-22T20:05:00Z")
reservation(2, "cancha-3", "2026-10-26", "10:00", 1, "Melina Romo", ["Sofía Acosta"], DEB, "2026-10-21T09:15:00Z")
r3 = reservation(3, "cancha-1", "2026-10-20", "18:00", 2, "Juan Sánchez", ["Franco Medina", "Ignacio Torres", "Lucas Pereyra"], MP, "2026-10-15T14:30:00Z", status="finalizada")
reservation(4, "cancha-1", "2026-10-28", "20:00", 2, "Facundo Molina", ["Matías Giménez", "Ignacio Torres", "Franco Medina"], CRED, "2026-10-22T11:40:00Z")
reservation(5, "cancha-5", "2026-10-29", "17:00", 1, "Agustina Herrera", ["Florencia Castro"], DEB, "2026-10-23T16:20:00Z")
r6 = reservation(6, "cancha-3", "2026-10-22", "09:00", 1, "Juan Sánchez", ["Tomás Benítez"], MP, "2026-10-18T12:00:00Z", status="cancelada",
                 cancel={"cancelledAt": "2026-10-21T10:00:00Z", "hoursInAdvance": 23, "policyApplied": "Cancelación dentro del plazo: reintegro total de la seña."})
reservation(7, "cancha-2", "2026-10-23", "18:00", 1, "Ezequiel Núñez", ["Franco Medina"], DEB, "2026-10-19T08:30:00Z", status="finalizada")
reservation(8, "cancha-3", "2026-10-30", "16:00", 1, "Valentina Ríos", ["Julieta Vega"], DEB, "2026-10-24T18:10:00Z")
reservation(9, "cancha-1", "2026-11-03", "19:00", 2, "Juan Sánchez", ["Lucas Pereyra", "Nicolás Ledesma", "Tomás Benítez"], MP, "2026-10-24T21:00:00Z")
reservation(10, "cancha-5", "2026-10-24", "11:00", 1, "Camila Rodríguez", ["Martina Suárez"], CRED, "2026-10-20T13:45:00Z", status="finalizada")
reservation(11, "cancha-2", "2026-10-31", "10:00", 1, "Nicolás Ledesma", ["Facundo Molina"], DEB, "2026-10-24T10:25:00Z")
r12 = reservation(12, "cancha-3", "2026-10-24", "19:00", 1, "Matías Giménez", ["Ignacio Torres"], DEB, "2026-10-20T19:00:00Z", status="cancelada",
                  cancel={"cancelledAt": "2026-10-24T17:30:00Z", "hoursInAdvance": 1.5, "policyApplied": "Cancelación fuera de plazo: retención del 50 % de la seña."})

# Cargo por cancelación tardía (el 50 % de la seña) de la reserva 12, con el mismo formato que genera la API
penalty_rec = receipt(r12["id"], "Matías Giménez", f"Cargo por cancelación fuera de plazo - {r12['courtName']}",
                      [{"description": f"Seña abonada ({r12['id']})", "amount": r12["depositPaid"]},
                       {"description": "Retención 50 % por cancelación con menos de 6 hs", "amount": r12["depositPaid"] // 2},
                       {"description": "Saldo reintegrado como crédito en cuenta", "amount": r12["depositPaid"] // 2}],
                      r12["depositPaid"] // 2, "Retención sobre seña", "2026-10-24 17:30", cashier="Sistema de Reservas")

# Otros cobros de caja
rec_liga = receipt(None, "Facundo Molina", "Inscripción Liga Primavera Singles Caballeros", [{"description": "Inscripción a liga", "amount": 15000}], 15000, CRED, "2026-09-28 11:10", cashier="Recepción del Club")
rec_cuota = receipt(None, "Melina Romo", "Cuota octubre - Escuela de Adultos Nivel Intermedio", [{"description": "Cuota mensual clase grupal", "amount": 16000}], 16000, DEB, "2026-10-02 18:45", cashier="Recepción del Club")
rec_torneo = receipt(None, "Juan Sánchez", "Inscripción Torneo Primavera 2026 Singles Caballeros", [{"description": "Inscripción a torneo", "amount": 8000}], 8000, MP, "2026-10-09 20:30")

INVOICES = [
    {"id": "FC-0001-00000118", "clientName": "Juan Sánchez", "clientDni": dni("Juan Sánchez"), "clientAddress": PERSON["Juan Sánchez"][4],
     "concept": f"Alquiler de cancha - Reserva {r3['id']}", "items": [{"description": f"Alquiler 2 h {r3['courtName']}", "quantity": 1, "unitPrice": 9600}],
     "subtotal": 9600, "total": 9600, "paymentMethod": MP, "relatedReceiptId": r3["finalReceiptNumber"], "status": "emitida", "date": "2026-10-20 20:05", "issuedBy": "Recepción del Club"},
    {"id": "FC-0001-00000119", "clientName": "Facundo Molina", "clientDni": dni("Facundo Molina"), "clientAddress": PERSON["Facundo Molina"][4],
     "concept": "Inscripción Liga Primavera Singles Caballeros", "items": [{"description": "Inscripción a liga", "quantity": 1, "unitPrice": 15000}],
     "subtotal": 15000, "total": 15000, "paymentMethod": CRED, "relatedReceiptId": rec_liga, "status": "emitida", "date": "2026-09-28 11:15", "issuedBy": "Recepción del Club"},
    {"id": "FC-0001-00000120", "clientName": "Melina Romo", "clientDni": dni("Melina Romo"), "clientAddress": PERSON["Melina Romo"][4],
     "concept": "Cuota octubre - Escuela de Adultos Nivel Intermedio", "items": [{"description": "Cuota mensual clase grupal", "quantity": 1, "unitPrice": 16000}],
     "subtotal": 16000, "total": 16000, "paymentMethod": DEB, "relatedReceiptId": rec_cuota, "status": "emitida", "date": "2026-10-02 18:50", "issuedBy": "Recepción del Club"},
]

REFUNDS = [
    {"id": "RMB-2026-0003", "clientName": "Juan Sánchez", "clientDni": dni("Juan Sánchez"), "relatedReservationId": r6["id"], "relatedReceiptId": r6["depositReceiptNumber"],
     "reason": "Cancelación de reserva con más de 6 h de antelación.", "refundType": "total", "originalAmount": r6["depositPaid"], "refundAmount": r6["depositPaid"],
     "refundMethod": "Devolución a Mercado Pago", "date": "2026-10-21 10:05", "processedBy": "Sistema de Reservas"},
    {"id": "RMB-2026-0004", "clientName": "Matías Giménez", "clientDni": dni("Matías Giménez"), "relatedReservationId": r12["id"], "relatedReceiptId": r12["depositReceiptNumber"],
     "reason": "Cancelación con menos de 6 h de antelación. Se retiene el 50 % de la seña.", "refundType": "parcial", "originalAmount": r12["depositPaid"], "refundAmount": r12["depositPaid"] // 2,
     "refundMethod": "Devolución a Tarjeta de Débito", "date": "2026-10-24 17:35", "processedBy": "Sistema de Reservas"},
]

# ---------------------------------------------------------------- torneos
def participants(names, prefix, start_day, partners=None):
    out = []
    for i, n in enumerate(names):
        p = {"id": f"{prefix}-{i + 1}", "name": n, "dni": dni(n) if n in PERSON else f"3{random.randint(3, 9)}.{random.randint(100, 999)}.{random.randint(100, 999)}",
             "phone": phone_short(n) if n in PERSON else f"11-{random.randint(2000, 6999)}-{random.randint(1000, 9999)}", "enrolledAt": f"2026-{start_day[0]:02d}-{start_day[1] + i // 2:02d}"}
        if partners:
            p["partnerName"] = partners[i]
            p["partnerDni"] = dni(partners[i])
        out.append(p)
    return out


def match(mid, p1, p2, score=None, winner=None, date="", court=""):
    return {"id": mid, "p1": p1, "p2": p2, "score": score or "", "winner": winner,
            "status": "finalizado" if winner else ("programado" if date and not p1.startswith("Ganador") and not p2.startswith("Ganador") else "pendiente"),
            "date": date, "court": court}


T1_PLAYERS = ["Facundo Molina", "Hernán Ortiz", "Tomás Benítez", "Pablo Quiroga", "Lucas Pereyra", "Santiago Ojeda", "Ezequiel Núñez", "Martín Figueroa",
              "Ignacio Torres", "Rodrigo Aguirre", "Nicolás Ledesma", "Juan Sánchez", "Franco Medina", "Gonzalo Cabrera", "Matías Giménez", "Federico Luna"]
C1, C2 = "Cancha 1", "Cancha 2"
TRN_01 = {
    "id": "trn-01", "name": "Torneo Primavera 2026 Singles Caballeros", "category": "Masculino", "modality": "Singles", "surfaceRequired": "Ladrillo",
    "startDate": "2026-10-17", "endDate": "2026-11-01", "registrationFee": 8000, "maxParticipants": 16, "currentEnrolled": 16, "status": "en_curso",
    "description": "Torneo de eliminación directa a 16 jugadores en polvo de ladrillo. Octavos y cuartos en fines de semana consecutivos; semifinales y final el último fin de semana.",
    "internalRules": "Partidos al mejor de 3 sets con tie-break en todos los sets. Tolerancia de espera de 15 minutos; pasado ese tiempo se pierde el partido por W.O. Pelotas provistas por el club.",
    "participants": participants(T1_PLAYERS, "p", (10, 1)),
    "bracket": [
        {"round": "Octavos de final", "matches": [
            match("m1", "Facundo Molina", "Hernán Ortiz", "6-2, 6-3", "Facundo Molina", "2026-10-17 10:00", C1),
            match("m2", "Tomás Benítez", "Pablo Quiroga", "7-5, 6-4", "Tomás Benítez", "2026-10-17 10:00", C2),
            match("m3", "Lucas Pereyra", "Santiago Ojeda", "6-3, 6-2", "Lucas Pereyra", "2026-10-17 12:00", C1),
            match("m4", "Ezequiel Núñez", "Martín Figueroa", "4-6, 6-3, 6-4", "Ezequiel Núñez", "2026-10-17 12:00", C2),
            match("m5", "Ignacio Torres", "Rodrigo Aguirre", "6-1, 6-2", "Ignacio Torres", "2026-10-18 10:00", C1),
            match("m6", "Nicolás Ledesma", "Juan Sánchez", "4-6, 6-3, 5-7", "Juan Sánchez", "2026-10-18 10:00", C2),
            match("m7", "Franco Medina", "Gonzalo Cabrera", "6-4, 6-4", "Franco Medina", "2026-10-18 12:00", C1),
            match("m8", "Matías Giménez", "Federico Luna", "6-0, 6-3", "Matías Giménez", "2026-10-18 12:00", C2),
        ]},
        {"round": "Cuartos de final", "matches": [
            match("m9", "Facundo Molina", "Tomás Benítez", "6-3, 6-4", "Facundo Molina", "2026-10-24 16:00", C1),
            match("m10", "Lucas Pereyra", "Ezequiel Núñez", "7-6, 6-4", "Lucas Pereyra", "2026-10-24 18:00", C1),
            match("m11", "Ignacio Torres", "Juan Sánchez", date="2026-10-25 16:00", court=C1),
            match("m12", "Franco Medina", "Matías Giménez", date="2026-10-25 18:00", court=C2),
        ]},
        {"round": "Semifinales", "matches": [
            match("m13", "Facundo Molina", "Lucas Pereyra", date="2026-10-31 16:00", court=C1),
            match("m14", "Ganador partido 11", "Ganador partido 12", date="2026-10-31 18:00", court=C1),
        ]},
        {"round": "Final", "matches": [
            match("m15", "Ganador partido 13", "Ganador partido 14", date="2026-11-01 17:00", court=C1),
        ]},
    ],
}

T2_LEADS = ["Melina Romo", "Camila Rodríguez", "Agustina Herrera", "Julieta Vega"]
T2_PARTNERS = ["Sofía Acosta", "Valentina Ríos", "Florencia Castro", "Martina Suárez"]
TRN_02 = {
    "id": "trn-02", "name": "Torneo de Dobles Damas - Noviembre 2026", "category": "Femenino", "modality": "Pareja", "surfaceRequired": "Cemento",
    "startDate": "2026-11-14", "endDate": "2026-11-15", "registrationFee": 12000, "maxParticipants": 8, "currentEnrolled": 4, "status": "inscripcion_abierta",
    "description": "Torneo femenino por parejas en canchas de cemento. Cuadro a 8 parejas; se juega en un fin de semana.",
    "internalRules": "Partidos a dos sets con punto de oro en 40-40. En caso de empate en sets se define por match tie-break a 10 puntos.",
    "participants": participants(T2_LEADS, "p", (10, 12), partners=T2_PARTNERS),
    "bracket": [],
}
T3_PLAYERS = ["Melina Romo", "Julieta Vega", "Valentina Ríos", "Martina Suárez", "Sofía Acosta", "Camila Rodríguez", "Agustina Herrera", "Florencia Castro"]
C3 = "Cancha 3"
TRN_03 = {
    "id": "trn-03", "name": "Torneo Invierno 2026 Singles Damas", "category": "Femenino", "modality": "Singles", "surfaceRequired": "Cemento",
    "startDate": "2026-08-08", "endDate": "2026-08-16", "registrationFee": 7000, "maxParticipants": 8, "currentEnrolled": 8, "status": "finalizado",
    "description": "Torneo femenino de eliminación directa a 8 jugadoras en cemento.",
    "internalRules": "Partidos al mejor de 3 sets con tie-break en todos los sets. Tolerancia de espera de 15 minutos.",
    "participants": participants(T3_PLAYERS, "p", (7, 20)),
    "bracket": [
        {"round": "Cuartos de final", "matches": [
            match("m1", "Melina Romo", "Julieta Vega", "6-2, 6-1", "Melina Romo", "2026-08-08 10:00", C3),
            match("m2", "Valentina Ríos", "Martina Suárez", "6-4, 6-4", "Valentina Ríos", "2026-08-08 12:00", C3),
            match("m3", "Sofía Acosta", "Camila Rodríguez", "3-6, 6-2, 6-3", "Sofía Acosta", "2026-08-09 10:00", C3),
            match("m4", "Agustina Herrera", "Florencia Castro", "7-5, 6-3", "Agustina Herrera", "2026-08-09 12:00", C3),
        ]},
        {"round": "Semifinales", "matches": [
            match("m5", "Melina Romo", "Valentina Ríos", "6-3, 6-4", "Melina Romo", "2026-08-15 16:00", C3),
            match("m6", "Sofía Acosta", "Agustina Herrera", "4-6, 6-7", "Agustina Herrera", "2026-08-15 18:00", C3),
        ]},
        {"round": "Final", "matches": [
            match("m7", "Melina Romo", "Agustina Herrera", "6-4, 3-6, 6-2", "Melina Romo", "2026-08-16 17:00", C3),
        ]},
    ],
}
TOURNAMENTS = [TRN_01, TRN_02, TRN_03]

# ---------------------------------------------------------------- ligas
L1_PLAYERS = ["Facundo Molina", "Matías Giménez", "Ignacio Torres", "Lucas Pereyra", "Juan Sánchez", "Ezequiel Núñez", "Franco Medina", "Nicolás Ledesma"]
SCORES = ["6-4, 6-3", "7-5, 6-4", "6-2, 6-3", "6-4, 3-6, 6-3", "7-6, 6-4", "6-3, 6-2", "4-6, 6-4, 7-5", "6-1, 6-4", "6-4, 6-4", "3-6, 6-3, 6-4"]


def round_robin(players):
    """Fixture todos contra todos (método del círculo)."""
    ps = list(players)
    rounds = []
    for _ in range(len(ps) - 1):
        rounds.append([(ps[i], ps[-1 - i]) for i in range(len(ps) // 2)])
        ps = [ps[0]] + [ps[-1]] + ps[1:-1]
    return rounds


L1_DATES = ["2026-10-04", "2026-10-11", "2026-10-18", "2026-11-08"]
fixtures = []
mid = 0
rank = {p: i for i, p in enumerate(L1_PLAYERS)}
for j, pairs in enumerate(round_robin(L1_PLAYERS)[:4]):
    matches = []
    for k, (a, b) in enumerate(pairs):
        mid += 1
        courtname = C1 if k % 2 == 0 else C2
        hour = f"{15 + (k // 2) * 2:02d}:00"
        if j < 3:
            # gana casi siempre el mejor rankeado, con alguna sorpresa
            fav, dog = (a, b) if rank[a] < rank[b] else (b, a)
            winner = dog if random.random() < 0.25 else fav
            matches.append(match(f"lm-{mid}", a, b, SCORES[mid % len(SCORES)], winner, f"{L1_DATES[j]} {hour}", courtname))
        else:
            matches.append(match(f"lm-{mid}", a, b, date=f"{L1_DATES[j]} {hour}", court=courtname))
    fixtures.append({"round": f"Jornada {j + 1}", "matches": matches})

table = {p: {"name": p, "pj": 0, "pg": 0, "pp": 0, "sf": 0, "sc": 0, "dif": 0, "points": 0} for p in L1_PLAYERS}
for f in fixtures:
    for m in f["matches"]:
        if m["status"] != "finalizado":
            continue
        sets = [tuple(int(x) for x in s.split("-")) for s in m["score"].split(", ")]
        p1_sets = sum(1 for a, b in sets if a > b)
        p2_sets = len(sets) - p1_sets
        # el score está escrito desde el ganador
        if m["winner"] == m["p2"]:
            p1_sets, p2_sets = p2_sets, p1_sets
        for p, won, lost in ((m["p1"], p1_sets, p2_sets), (m["p2"], p2_sets, p1_sets)):
            t = table[p]
            t["pj"] += 1
            t["sf"] += won
            t["sc"] += lost
            t["dif"] = t["sf"] - t["sc"]
            if m["winner"] == p:
                t["pg"] += 1
                t["points"] += 3
            else:
                t["pp"] += 1
standings = sorted(table.values(), key=lambda t: (-t["points"], -t["dif"], t["name"]))
standings = [dict({"rank": i + 1}, **t) for i, t in enumerate(standings)]

LIGA_01 = {
    "id": "liga-01", "name": "Liga Primavera Singles Caballeros", "category": "Masculino", "modality": "Singles", "surfaceRequired": "Ladrillo",
    "startDate": "2026-10-04", "endDate": "2026-12-13", "registrationFee": 15000, "maxParticipants": 8, "currentEnrolled": 8, "status": "en_curso",
    "description": "Liga todos contra todos a una rueda, con una jornada por fin de semana en polvo de ladrillo.",
    "internalRules": "Cada partido ganado suma 3 puntos. Partidos al mejor de 3 sets. Desempate en la tabla: 1) diferencia de sets, 2) resultado entre ambos.",
    "standings": standings,
    "fixtures": fixtures,
    "participants": participants(L1_PLAYERS, "lp", (9, 20)),
}

L2_LEADS = ["Melina Romo", "Sofía Acosta", "Camila Rodríguez"]
L2_PARTNERS = ["Lucas Pereyra", "Matías Giménez", "Nicolás Ledesma"]
LIGA_02 = {
    "id": "liga-02", "name": "Liga de Dobles Mixto", "category": "Mixto", "modality": "Pareja", "surfaceRequired": "Cemento",
    "startDate": "2026-11-07", "endDate": "2026-12-19", "registrationFee": 20000, "maxParticipants": 8, "currentEnrolled": 3, "status": "inscripcion_abierta",
    "description": "Liga de dobles mixto por puntos en canchas de cemento. Una jornada por sábado.",
    "internalRules": "Partidos a dos sets con punto de oro; match tie-break a 10 si empatan. Cada pareja debe presentar su lista de buena fe antes de la primera jornada.",
    "standings": [{"rank": i + 1, "name": n, "pj": 0, "pg": 0, "pp": 0, "sf": 0, "sc": 0, "dif": 0, "points": 0} for i, n in enumerate(L2_LEADS)],
    "fixtures": [],
    "participants": participants(L2_LEADS, "lp", (10, 15), partners=L2_PARTNERS),
}
LEAGUES = [LIGA_01, LIGA_02]

# ---------------------------------------------------------------- clases
FIRST_KIDS = ["Benjamín", "Olivia", "Thiago", "Emma", "Bautista", "Isabella", "Mateo", "Catalina", "Santino", "Alma", "Lautaro", "Victoria", "Joaquín", "Mía", "Felipe",
              "Luna", "Dante", "Zoe", "Valentino", "Delfina", "Ciro", "Renata", "Lorenzo", "Guadalupe", "Francesco", "Juana", "Gael", "Antonella", "Noah", "Abril"]
LAST_KIDS = ["Ramírez", "Flores", "Sosa", "Romero", "Álvarez", "Ruiz", "Díaz", "Moreno", "Ortega", "Silva", "Aguilar", "Peralta", "Gutiérrez", "Luna", "Ponce",
             "Rojas", "Ferreyra", "Arias", "Villalba", "Bustos", "Cardozo", "Godoy", "Ríos", "Navarro", "Paz", "Mansilla", "Acuña", "Ledesma", "Coronel", "Vera"]


def attendance(n, absent_rate):
    return ["A" if random.random() < absent_rate else "P" for _ in range(n)]


def students(names, prefix, sessions, kids=False):
    out = []
    for i, n in enumerate(names):
        d = dni(n) if n in PERSON else f"5{random.randint(0, 3)}.{random.randint(100, 999)}.{random.randint(100, 999)}"
        out.append({"id": f"{prefix}-{i + 1}", "name": n, "dni": d, "attendance": attendance(len(sessions), 0.12 if not kids else 0.1)})
    return out


def klass(n, name, kind, coach, days, time, cid, cap, names, fee, sessions, kids=False):
    return {
        "id": f"cls-{n:02d}", "name": name, "type": "grupal", "kind": kind, "coachId": coach, "coachName": COACH_NAME[coach],
        "scheduleDays": days, "scheduleTime": time, "courtAssigned": court_short(cid), "maxCapacity": cap, "currentEnrolled": len(names),
        "monthlyFee": fee, "monthName": "Octubre 2026", "sessions": sessions, "students": students(names, f"std-{n:02d}", sessions, kids),
    }


MAR_JUE = ["01/10", "06/10", "08/10", "13/10", "15/10", "20/10", "22/10"]
LUN_MIE = ["05/10", "07/10", "14/10", "19/10", "21/10"]
VIE_SAB = ["02/10", "03/10", "09/10", "10/10", "16/10", "17/10", "23/10", "24/10"]
KIDS = [f"{FIRST_KIDS[i]} {LAST_KIDS[i]}" for i in range(30)]

CLASSES = [
    klass(1, "Escuela de Adultos - Nivel Intermedio", "clase", "coach-01", "Martes y Jueves", "19:00 - 20:30", "cancha-1", 12,
          ["Melina Romo", "Lucas Pereyra", "Sofía Acosta", "Camila Rodríguez", "Agustina Herrera", "Franco Medina"], 16000, MAR_JUE),
    klass(2, "Iniciación Adultos", "entrenamiento", "coach-02", "Lunes y Miércoles", "18:00 - 19:30", "cancha-3", 10,
          ["Valentina Ríos", "Julieta Vega", "Martina Suárez", "Florencia Castro"], 14000, LUN_MIE),
    klass(3, "Escuela de Menores - Competición", "clase", "coach-01", "Viernes y Sábados", "17:00 - 19:00", "cancha-2", 30, KIDS, 18000, VIE_SAB, kids=True),
    klass(4, "Entrenamiento Físico-Técnico", "entrenamiento", "coach-02", "Lunes y Miércoles", "07:00 - 08:30", "cancha-3", 15,
          ["Facundo Molina", "Matías Giménez", "Ignacio Torres", "Ezequiel Núñez"], 18000, LUN_MIE),
    klass(5, "Entrenamiento de Alto Rendimiento", "entrenamiento", "coach-03", "Martes y Jueves", "20:30 - 22:00", "cancha-2", 8,
          ["Tomás Benítez", "Nicolás Ledesma", "Lucas Pereyra"], 17000, MAR_JUE),
]


def private(n, kind, coach, student, date, time, cid, status, notes):
    return {"id": f"prv-{n:02d}", "kind": kind, "coachId": coach, "coachName": COACH_NAME[coach], "studentName": student, "studentDni": dni(student),
            "studentPhone": phone_short(student), "date": date, "time": time, "courtAssigned": court_short(cid), "hourlyRate": 6000, "status": status, "notes": notes}


PRIVATE_CLASSES = [
    private(1, "clase", "coach-01", "Juan Sánchez", "2026-10-27", "10:00 - 11:00", "cancha-1", "confirmada", "Trabajo de saque y volea."),
    private(2, "entrenamiento", "coach-02", "Melina Romo", "2026-10-28", "16:00 - 17:00", "cancha-3", "confirmada", "Preparación física para el torneo de dobles."),
    private(3, "clase", "coach-01", "Sofía Acosta", "2026-10-30", "11:00 - 12:00", "cancha-2", "programada", "Revés a dos manos."),
    private(4, "entrenamiento", "coach-03", "Facundo Molina", "2026-10-29", "08:00 - 09:00", "cancha-2", "programada", "Entrenamiento previo a la semifinal."),
    private(5, "clase", "coach-01", "Valentina Ríos", "2026-10-23", "10:00 - 11:00", "cancha-1", "cancelada", "Cancelada por lluvia."),
]

# ---------------------------------------------------------------- salida
SEED = {
    "usuarios": USERS, "canchas": COURTS, "tipos-cancha": COURT_TYPES, "stock": STOCK, "reservas": RESERVATIONS,
    "torneos": TOURNAMENTS, "ligas": LEAGUES, "tipos-competencia": COMPETITION_TYPES, "profesores": PROFESORES,
    "clases": CLASSES, "clases-particulares": PRIVATE_CLASSES, "recibos": RECEIPTS, "facturas": INVOICES,
    "reembolsos": REFUNDS, "descuentos": DISCOUNTS, "documentos": DOCUMENTS,
}

with open(SERVER_SEED, "w", encoding="utf-8") as fh:
    json.dump(SEED, fh, ensure_ascii=False, indent=2)
    fh.write("\n")

MOCK_NAMES = [
    ("INITIAL_COURTS", "canchas"), ("INITIAL_STOCK", "stock"), ("INITIAL_RESERVATIONS", "reservas"), ("INITIAL_TOURNAMENTS", "torneos"),
    ("INITIAL_LEAGUES", "ligas"), ("INITIAL_COACHES", "profesores"), ("INITIAL_DOCUMENTS", "documentos"), ("INITIAL_PRIVATE_CLASSES", "clases-particulares"),
    ("INITIAL_CLASSES", "clases"), ("INITIAL_DISCOUNTS", "descuentos"), ("INITIAL_RECEIPTS", "recibos"), ("INITIAL_USERS", "usuarios"),
    ("INITIAL_COURT_TYPES", "tipos-cancha"), ("INITIAL_COMPETITION_TYPES", "tipos-competencia"), ("INITIAL_INVOICES", "facturas"), ("INITIAL_REFUNDS", "reembolsos"),
]
with open(FRONT_MOCK, "w", encoding="utf-8") as fh:
    fh.write("/**\n * Datos de ejemplo de \"Tenis Ahora\" para el Modo Demo (sin backend).\n"
             " * Es el mismo dataset que `server/seed/seed.json`; los dos se generan juntos.\n */\n\n")
    for const, col in MOCK_NAMES:
        fh.write(f"export const {const} = {json.dumps(SEED[col], ensure_ascii=False, indent=2)};\n\n")

total = sum(len(v) for v in SEED.values())
print("documentos en el seed:", total)
for k, v in SEED.items():
    print(f"  {k}: {len(v)}")
print("tabla liga-01:")
for s in standings:
    print("  ", s)
