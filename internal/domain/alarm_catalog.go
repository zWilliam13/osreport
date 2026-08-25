package domain

// AlarmInfo is the human-readable title and root-cause narrative for an
// EventType, used to render the Top N Alarmas report.
type AlarmInfo struct {
	Alarma      string
	Descripcion string
}

// alarmCatalog covers every EventType ExtractEventType can currently
// produce via knownFunctionEventTypes or its handle_io_conn/DIAM
// special-cases. EventTypes reaching here through the "unrecognized
// function name surfaces as-is" fallback (classify.go) have no entry —
// DescribeAlarm falls back to a generic description for those rather than
// fabricating one.
var alarmCatalog = map[string]AlarmInfo{
	"ASP_DOWN": {
		Alarma:      "Caida de ASP (M3UA)",
		Descripcion: "Indicacion de caida de un Application Server Process en el enlace SS7/SIGTRAN. Suele acompanarse de CONN_TIMEOUT sobre el mismo enlace.",
	},
	"ASP_UP": {
		Alarma:      "ASP recuperado (M3UA)",
		Descripcion: "Indicacion de que un Application Server Process volvio a estar disponible.",
	},
	"CONN_TIMEOUT": {
		Alarma:      "Timeout de conexion M3UA",
		Descripcion: "El enlace M3UA no responde dentro del tiempo esperado. Sintoma habitual acompanando una caida de ASP en el mismo enlace.",
	},
	"CONN_REFUSED": {
		Alarma:      "Conexion M3UA rechazada",
		Descripcion: "El peer rechazo activamente el intento de conexion M3UA.",
	},
	"CONN_RESET": {
		Alarma:      "Conexion M3UA reseteada",
		Descripcion: "La conexion M3UA fue reseteada por el peer o la red.",
	},
	"CONN_ERROR": {
		Alarma:      "Error de conexion M3UA",
		Descripcion: "Fallo de conexion M3UA no clasificado en timeout/refused/reset.",
	},
	"PEER_IO_ERROR": {
		Alarma:      "Error de I/O con peer (M3UA)",
		Descripcion: "Error de socket a nivel de I/O contra un peer M3UA - sintoma de conectividad de red distinto al timeout de conexion estandar.",
	},
	"AS_UNREACHABLE": {
		Alarma:      "AS inalcanzable (DUNA)",
		Descripcion: "Notificacion M3UA de Destino No Disponible para todo un Application Server (no solo un ASP). El volumen suele venir parejo con AS_REACHABLE - ruta que fluctua/reconecta repetidamente, no una caida sostenida.",
	},
	"AS_REACHABLE": {
		Alarma:      "AS disponible de nuevo (DAVA)",
		Descripcion: "Notificacion M3UA de Destino Disponible - el Application Server volvio a estar alcanzable tras un AS_UNREACHABLE.",
	},
	"AS_STATE_DOWN": {
		Alarma:      "AS transiciona a DOWN",
		Descripcion: "El Application Server completo paso de ACTIVE a DOWN - caida real del AS, no solo fluctuacion de ruta (DUNA/DAVA).",
	},
	"AS_STATE_RECOVERING": {
		Alarma:      "AS transiciona a INACTIVE (recuperando)",
		Descripcion: "El Application Server paso de DOWN a INACTIVE - paso intermedio de recuperacion, todavia no esta ACTIVE.",
	},
	"AS_STATE_ACTIVE": {
		Alarma:      "AS recuperado a ACTIVE",
		Descripcion: "El Application Server completo el ciclo de recuperacion y volvio a ACTIVE.",
	},
	"TCAP_CCO_EXHAUSTED": {
		Alarma:      "Fallo de reserva de state machine (CCO)",
		Descripcion: "TCAP no logra reservar slots de control (CCO) para nuevas transacciones MAP (SMS, autenticacion, ubicacion) - indica agotamiento del pool de DSM/SSM o transacciones huerfanas sin liberar.",
	},
	"MAP_DSM_16015_STUCK": {
		Alarma:      "DSM 16015 no inicializado (slot fijo)",
		Descripcion: "Siempre el mismo slot de estado (16015), no aleatorio - indica un slot posiblemente corrupto/pegado que bloquea esa capacidad de forma permanente.",
	},
	"MAP_IDH_LINK_FAIL": {
		Alarma:      "Fallo de enlace de dialogo (IDH)",
		Descripcion: "MAP no puede reasociar una respuesta con su dialogo de origen. A menudo comparte causa de fondo con el agotamiento de pool TCAP.",
	},
	"MAP_DSM0_NOT_INIT": {
		Alarma:      "DSM no inicializado (timer)",
		Descripcion: "Timer disparado sobre un DSM no inicializado - mismo patron de fondo que el agotamiento de pool TCAP/MAP.",
	},
	"MAP_ACN_UNSUPPORTED": {
		Alarma:      "ACN no soportado",
		Descripcion: "Peer externo pide un Application Context no habilitado en este nodo - desalineacion de version/config puntual.",
	},
	"MAP_ERROR_IND_UNHANDLED": {
		Alarma:      "ERROR INDICATION sin manejo (MAP)",
		Descripcion: "Un DSM recibe un ERROR INDICATION que no tiene manejador definido para ese estado - senal de un caso de error no contemplado en el flujo MAP.",
	},
	"MAP_DSM_DUMMY_LINK": {
		Alarma:      "Enlace de DSM dummy con DHA inferior",
		Descripcion: "MAP crea un enlace dummy entre un DSM y un DHA de menor jerarquia - patron recurrente que amerita revisar la logica de asignacion de DSM/DHA.",
	},
	"HSS_UNKNOWN_IMSI": {
		Alarma:      "IMSI desconocido (AuthInfo)",
		Descripcion: "HSS recibe pedido de autenticacion para SIMs no registradas - el celular no puede conectarse (falla de attach), impacto directo en el cliente si son abonados reales.",
	},
	"HSS_UNKNOWN_MSISDN": {
		Alarma:      "Suscriptor desconocido (MSISDN)",
		Descripcion: "Consulta de SMS (sriSM) para numeros no existentes en HSS - mayormente trafico externo (spam/numeros dados de baja), no necesariamente falla propia del sistema.",
	},
	"S6A_UNKNOWN_IMSI": {
		Alarma:      "AIR para IMSI desconocido",
		Descripcion: "Peer externo envia Authentication-Information-Request (S6a) para IMSIs no aprovisionados en este HSS.",
	},
	"S6A_UNKNOWN_CP": {
		Alarma:      "NOA de contact-point desconocido",
		Descripcion: "Respuesta S6a desde un contact-point no registrado - puede afectar sincronizacion de estado MME/HSS.",
	},
	"DIAM_PEER_DOWN": {
		Alarma:      "Peer Diameter caido",
		Descripcion: "Conexion Diameter hacia un peer externo reportada abajo con reconexion en progreso de forma repetida.",
	},
	"DIAM_DYNAMIC_PEER": {
		Alarma:      "Peer dinamico de alta frecuencia",
		Descripcion: "Aviso repetido de peer dinamico Diameter. Por si solo no indica falla, pero el volumen amerita confirmar si es reconexion constante del mismo peer.",
	},
	"DIAM_ORPHAN_ANSWER": {
		Alarma:      "Respuesta huerfana (hbh/e2e)",
		Descripcion: "Respuesta Diameter sin match de transaccion - llega tarde, sintoma de latencia del peer, no de falla del sistema.",
	},
	"HSS_PROFILE_RELOAD": {
		Alarma:      "Recarga de perfil por alta/baja de suscripcion",
		Descripcion: "HSS recarga el perfil de una UE tras un insert/delete de suscripcion - lectura como telemetria de aprovisionamiento normal, no como falla (por eso va como Info, no Major, a diferencia de lo que su ALERT_SEVERITY=SYS crudo sugeriria).",
	},
	"TCAP_END_UNALLOCATED": {
		Alarma:      "TCAP END sobre transaccion no asignada",
		Descripcion: "Un END de dialogo TCAP llega para una transaccion que nunca se reservo - probable mismo trasfondo que el agotamiento de pool TCAP/MAP (transacciones huerfanas), no un patron independiente.",
	},
	"DIAM_ROUTE_FAILURE": {
		Alarma:      "Fallo de ruteo Diameter (sin peer destino)",
		Descripcion: "No se encontro un peer destino para la solicitud Diameter - tabla de rutas/peer no configurado o peer caido, distinto de DIAM_PEER_DOWN (que es una reconexion en curso, no una ausencia total de ruta).",
	},
	"MAP_AUTH_CLOSE_NOT_IMPLEMENTED": {
		Alarma:      "Cierre de autenticacion MAP no implementado",
		Descripcion: "El cierre de la sesion de autenticacion HLR (Auth close) golpea una ruta marcada NOT IMPLEMENTED en el stack MAP - posible brecha real de funcionalidad, no solo ruido; candidato a escalar si el volumen se sostiene.",
	},
	"TCAP_DHA_NOT_INIT": {
		Alarma:      "DHA no inicializado (set_state / release)",
		Descripcion: "Una Dialogue Handling Association (DHA) de TCAP recibe set_state o release sin haber sido inicializada - mismo DHA en ambos casos en los ejemplos vistos, probable trasfondo compartido con el agotamiento de pool TCAP/MAP (transacciones huerfanas que nunca llegaron a reservar su DHA).",
	},
	// Distinta de TCAP_DHA_NOT_INIT: dispara desde un timer, no desde
	// set_state/release, y su conteo no coincide con el de esas dos - no
	// hay evidencia de que sea el mismo evento, asi que se deja como su
	// propio EventType (function name como Key, sin remapear) para no
	// romper su historial de tendencia ya establecido.
	"tcap_start_dha_timer": {
		Alarma:      "DHA no inicializado (timer)",
		Descripcion: "Un timer de TCAP dispara sobre una DHA que no fue inicializada - mismo sintoma de fondo que el resto de la familia DHA/DSM no inicializado, via un disparador distinto (timer en vez de set_state/release).",
	},
}

// DescribeAlarm returns the catalog entry for eventType, or a generic
// fallback for EventTypes not yet cataloged (e.g. a function name surfaced
// as-is by ExtractEventType's unrecognized-function fallback). eventType
// is "" for an event whose message matched no known shape at all (e.g. a
// document with no MESSAGE.raw/msg body) — falling through to the generic
// case for that would render a blank Alarma cell in the report, so it
// gets its own explicit label instead.
func DescribeAlarm(eventType string) AlarmInfo {
	if info, ok := alarmCatalog[eventType]; ok {
		return info
	}
	if eventType == "" {
		return AlarmInfo{
			Alarma:      "(sin tipo de evento)",
			Descripcion: "El mensaje no coincidio con ningun patron conocido — revisar el mensaje de ejemplo.",
		}
	}
	return AlarmInfo{
		Alarma:      eventType,
		Descripcion: "Patron no catalogado — revisar el mensaje de ejemplo.",
	}
}
