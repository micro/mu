package flights

// Aircraft type designators.
//
// A transponder broadcasts "B789". Almost nobody knows that is a Boeing 787-9,
// and "Boeing 787-9" is the whole reason anyone asked what was overhead. So the
// designators get names.
//
// This table is hand-written and Go rather than generated and JSON, unlike the
// airport and airline tables beside it. Those two are filtered copies of public
// datasets and are regenerated wholesale; this one is a judgement about which
// couple of hundred types are worth naming out of the several thousand ICAO
// publishes, and a judgement belongs where it can be read. An unknown designator
// is passed through as it came, which is worse than a name and better than
// nothing.

import "strings"

var aircraftTypes = map[string]string{
	// Airbus narrowbodies
	"A19N": "Airbus A319neo", "A20N": "Airbus A320neo", "A21N": "Airbus A321neo",
	"A318": "Airbus A318", "A319": "Airbus A319", "A320": "Airbus A320", "A321": "Airbus A321",
	"BCS1": "Airbus A220-100", "BCS3": "Airbus A220-300",

	// Airbus widebodies
	"A306": "Airbus A300-600", "A310": "Airbus A310",
	"A332": "Airbus A330-200", "A333": "Airbus A330-300",
	"A338": "Airbus A330-800neo", "A339": "Airbus A330-900neo",
	"A342": "Airbus A340-200", "A343": "Airbus A340-300",
	"A345": "Airbus A340-500", "A346": "Airbus A340-600",
	"A359": "Airbus A350-900", "A35K": "Airbus A350-1000",
	"A388": "Airbus A380-800",

	// Boeing narrowbodies
	"B712": "Boeing 717-200",
	"B732": "Boeing 737-200", "B733": "Boeing 737-300", "B734": "Boeing 737-400",
	"B735": "Boeing 737-500", "B736": "Boeing 737-600", "B737": "Boeing 737-700",
	"B738": "Boeing 737-800", "B739": "Boeing 737-900",
	"B37M": "Boeing 737 MAX 7", "B38M": "Boeing 737 MAX 8",
	"B39M": "Boeing 737 MAX 9", "B3XM": "Boeing 737 MAX 10",
	"B752": "Boeing 757-200", "B753": "Boeing 757-300",

	// Boeing widebodies
	"B741": "Boeing 747-100", "B742": "Boeing 747-200", "B743": "Boeing 747-300",
	"B744": "Boeing 747-400", "B748": "Boeing 747-8", "B74S": "Boeing 747SP",
	"B762": "Boeing 767-200", "B763": "Boeing 767-300", "B764": "Boeing 767-400",
	"B772": "Boeing 777-200", "B773": "Boeing 777-300",
	"B77L": "Boeing 777-200LR", "B77W": "Boeing 777-300ER", "B778": "Boeing 777-8", "B779": "Boeing 777-9",
	"B788": "Boeing 787-8", "B789": "Boeing 787-9", "B78X": "Boeing 787-10",

	// Embraer
	"E135": "Embraer ERJ-135", "E145": "Embraer ERJ-145",
	"E170": "Embraer E170", "E175": "Embraer E175",
	"E190": "Embraer E190", "E195": "Embraer E195",
	"E75L": "Embraer E175", "E75S": "Embraer E175",
	"E290": "Embraer E190-E2", "E295": "Embraer E195-E2",

	// Bombardier and regional
	"CRJ1": "Bombardier CRJ100", "CRJ2": "Bombardier CRJ200",
	"CRJ7": "Bombardier CRJ700", "CRJ9": "Bombardier CRJ900", "CRJX": "Bombardier CRJ1000",
	"DH8A": "De Havilland Dash 8-100", "DH8B": "De Havilland Dash 8-200",
	"DH8C": "De Havilland Dash 8-300", "DH8D": "De Havilland Dash 8-400",
	"AT43": "ATR 42-300", "AT45": "ATR 42-500", "AT46": "ATR 42-600",
	"AT72": "ATR 72", "AT75": "ATR 72-500", "AT76": "ATR 72-600",
	"SF34": "Saab 340", "SB20": "Saab 2000",
	"JS41": "BAe Jetstream 41", "ATP": "BAe ATP",
	"B461": "BAe 146-100", "B462": "BAe 146-200", "B463": "BAe 146-300",
	"RJ85": "Avro RJ85", "RJ1H": "Avro RJ100",
	"F100": "Fokker 100", "F70": "Fokker 70", "F50": "Fokker 50",
	"D228": "Dornier 228", "D328": "Dornier 328",
	"SU95": "Sukhoi Superjet 100", "L410": "Let L-410",

	// McDonnell Douglas
	"MD11": "McDonnell Douglas MD-11",
	"MD81": "McDonnell Douglas MD-81", "MD82": "McDonnell Douglas MD-82",
	"MD83": "McDonnell Douglas MD-83", "MD87": "McDonnell Douglas MD-87",
	"MD88": "McDonnell Douglas MD-88", "MD90": "McDonnell Douglas MD-90",
	"DC10": "McDonnell Douglas DC-10",

	// Soviet and Russian
	"A124": "Antonov An-124", "A225": "Antonov An-225",
	"AN12": "Antonov An-12", "AN24": "Antonov An-24", "AN26": "Antonov An-26",
	"IL76": "Ilyushin Il-76", "IL96": "Ilyushin Il-96",
	"T204": "Tupolev Tu-204", "YK42": "Yakovlev Yak-42",

	// Business jets
	"GLF4": "Gulfstream IV", "GLF5": "Gulfstream V", "GLF6": "Gulfstream G650",
	"GA5C": "Gulfstream G500", "GA6C": "Gulfstream G600", "G280": "Gulfstream G280",
	"GLEX": "Bombardier Global Express", "GL5T": "Bombardier Global 5000",
	"GL7T": "Bombardier Global 7500",
	"CL30": "Bombardier Challenger 300", "CL35": "Bombardier Challenger 350",
	"CL60": "Bombardier Challenger 600",
	"F2TH": "Dassault Falcon 2000", "FA7X": "Dassault Falcon 7X",
	"FA8X": "Dassault Falcon 8X", "F900": "Dassault Falcon 900",
	"C25A": "Cessna Citation CJ2", "C25B": "Cessna Citation CJ3", "C25C": "Cessna Citation CJ4",
	"C500": "Cessna Citation I", "C550": "Cessna Citation II", "C560": "Cessna Citation V",
	"C56X": "Cessna Citation Excel", "C680": "Cessna Citation Sovereign",
	"C68A": "Cessna Citation Latitude", "C700": "Cessna Citation Longitude",
	"E50P": "Embraer Phenom 100", "E55P": "Embraer Phenom 300", "E545": "Embraer Legacy 450",
	"LJ35": "Learjet 35", "LJ45": "Learjet 45", "LJ60": "Learjet 60", "LJ75": "Learjet 75",
	"H25B": "Hawker 800", "HDJT": "Honda HA-420 HondaJet",
	"PC24": "Pilatus PC-24",

	// Turboprops and general aviation
	"PC12": "Pilatus PC-12", "TBM7": "Daher TBM 700", "TBM9": "Daher TBM 900",
	"BE20": "Beechcraft King Air 200", "B350": "Beechcraft King Air 350",
	"BE9L": "Beechcraft King Air 90", "B190": "Beechcraft 1900",
	"DHC6": "De Havilland Twin Otter", "DHC2": "De Havilland Beaver",
	"C172": "Cessna 172", "C152": "Cessna 152", "C182": "Cessna 182",
	"C206": "Cessna 206", "C208": "Cessna 208 Caravan", "C210": "Cessna 210",
	"P28A": "Piper PA-28", "PA31": "Piper Navajo", "PA34": "Piper Seneca", "PA46": "Piper Malibu",
	"SR20": "Cirrus SR20", "SR22": "Cirrus SR22", "DA40": "Diamond DA40", "DA42": "Diamond DA42",
	"AC11": "Aero Commander", "RV7": "Van's RV-7", "RV8": "Van's RV-8",
	"GLID": "glider", "BALL": "balloon", "UAV": "drone",

	// Helicopters
	"EC35": "Airbus H135", "EC45": "Airbus H145", "EC55": "Airbus H155",
	"EC75": "Airbus H175", "AS50": "Airbus AS350", "AS55": "Airbus AS355",
	"A139": "Leonardo AW139", "A169": "Leonardo AW169", "A189": "Leonardo AW189",
	"B06": "Bell 206", "B407": "Bell 407", "B412": "Bell 412", "B429": "Bell 429",
	"S76": "Sikorsky S-76", "S92": "Sikorsky S-92",
	"R22": "Robinson R22", "R44": "Robinson R44", "R66": "Robinson R66",

	// Military and government
	"C130": "Lockheed C-130 Hercules", "C30J": "Lockheed C-130J Super Hercules",
	"C17": "Boeing C-17 Globemaster", "C5M": "Lockheed C-5 Galaxy",
	"K35R": "Boeing KC-135 Stratotanker", "KC46": "Boeing KC-46 Pegasus",
	"A400": "Airbus A400M Atlas",
	"E3TF": "Boeing E-3 Sentry", "P8": "Boeing P-8 Poseidon",
	"F15": "F-15 Eagle", "F16": "F-16 Fighting Falcon", "F18": "F/A-18 Hornet",
	"F35": "F-35 Lightning II", "EUFI": "Eurofighter Typhoon", "RFAL": "Dassault Rafale",
	"H60": "Sikorsky UH-60 Black Hawk", "CH47": "Boeing CH-47 Chinook",
	"A10": "Fairchild A-10 Thunderbolt II", "HAWK": "BAe Hawk",
}

// AircraftType names a type designator, or returns the designator unchanged when
// nothing in the table claims it.
func AircraftType(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return ""
	}
	if name, ok := aircraftTypes[code]; ok {
		return name
	}
	return code
}
