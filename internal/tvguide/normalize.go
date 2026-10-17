package tvguide

import (
	"html"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/e12jassir/kickoff-cli/internal/model"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	reNonAlphaNum = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	reTokensFC    = regexp.MustCompile(`\b(fc|cf|f\.c\.|c\.f\.)\b`)
	reWhitespace  = regexp.MustCompile(`\s+`)
)

// accentTransformerPool hands out transformers that strip diacritical marks
// (e.g., á->a, ñ->n, ü->u, ç->c).
//
// A transform.Transformer carries mutable internal state (it resets and buffers
// per call), so a single package-level instance cannot be shared across
// goroutines. RemoveAccents is called from the TUI update loop and from
// concurrent fetch goroutines, therefore each caller checks one out of the pool
// and returns it reset.
var accentTransformerPool = sync.Pool{
	New: func() any {
		return transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	},
}

// RemoveAccents removes diacritical marks from UTF-8 strings.
func RemoveAccents(s string) string {
	t := accentTransformerPool.Get().(transform.Transformer)
	defer func() {
		t.Reset()
		accentTransformerPool.Put(t)
	}()

	res, _, err := transform.String(t, s)
	if err != nil {
		return s
	}
	return res
}

// Normalize normalizes a team name, fixture title, or TV guide program entry:
// 1. Unescapes HTML entities (&amp;, etc.)
// 2. Removes diacritical accents
// 3. Converts to lowercase
// 4. Strips dots
// 5. Strips "FC", "CF" acronyms
// 6. Replaces non-alphanumeric punctuation with space and collapses whitespace
func Normalize(s string) string {
	s = html.UnescapeString(s)
	s = RemoveAccents(s)
	s = strings.ToLower(s)
	// Strip dots directly so C.F. becomes cf, S.K. becomes sk
	s = strings.ReplaceAll(s, ".", "")
	// Strip FC / CF acronyms
	s = reTokensFC.ReplaceAllString(s, " ")
	// Replace remaining non-alphanumeric chars with space
	s = reNonAlphaNum.ReplaceAllString(s, " ")
	return strings.TrimSpace(reWhitespace.ReplaceAllString(s, " "))
}

// stopwords are structural words (prepositions, articles, dangling
// abbreviations) that carry no club identity on their own. Unlike the old list,
// this no longer drops "real", "inter", "atletico", "united", "city",
// "racing", "sporting" or "deportivo": those are handled by the
// unidentifyingTokens table below so they are preserved as tokens without being
// allowed to identify a club by themselves.
var stopwords = map[string]bool{
	"de": true, "del": true, "la": true, "el": true, "los": true, "las": true,
	"san": true, "santa": true, "van": true, "von": true, "sur": true, "der": true,
	"the": true, "and": true, "und": true, "vs": true, "en": true, "y": true,
	"cd": true, "ca": true, "ac": true, "sc": true, "fk": true, "afc": true,
	"balompie": true,
}

// unidentifyingTokens are normalized tokens that cannot, on their own, identify
// a single club. There are two kinds:
//
//   - generic club/legal qualifiers shared by many unrelated clubs after the
//     "FC"/"CF" and accent stripping (real, atletico, inter, sporting, ...);
//   - proper names or city tokens that two or more distinct clubs collapse onto
//     (barcelona -> FC Barcelona (ESP) or Barcelona SC (ECU); madrid -> Real
//     Madrid or Atletico Madrid; nacional, america, universitario, inter, ...).
//
// A team must match on at least one token OUTSIDE this set. A team whose entire
// name reduces to tokens in this set (for example "FC Barcelona" -> barcelona)
// is never treated as identified: resolving it could hand the user the wrong
// club, and an unverified channel is strictly better than a wrong one.
//
// The list is intentionally conservative and curated. Residual collisions on
// tokens that are NOT listed here are reported instead of guessed.
var unidentifyingTokens = map[string]bool{
	// Generic club / legal qualifiers.
	"real": true, "atletico": true, "deportivo": true, "sporting": true,
	"racing": true, "sport": true, "inter": true, "union": true,
	"club": true, "city": true, "united": true, "town": true,

	// Proper names / shared city tokens that two distinct clubs collapse onto.
	"barcelona":     true, // FC Barcelona (ESP) vs Barcelona SC (ECU)
	"madrid":        true, // Real Madrid vs Atletico Madrid
	"manchester":    true, // Manchester City vs Manchester United
	"munich":        true, // Bayern Munich vs 1860 Munich
	"milan":         true, // AC Milan vs Inter Milan
	"nacional":      true, // Atletico Nacional (COL) vs Nacional (URU)
	"america":       true, // America de Cali (COL) vs Club America (MEX)
	"cali":          true, // Deportivo Cali vs America de Cali
	"universitario": true, // Universitario de Deportes (PER) vs Universitario (BOL)
	"independiente": true, // Independiente (ARG) vs Independiente del Valle (ECU)
	"olimpia":       true, // Olimpia (PAR) vs Olimpia (HON)
	"rotterdam":     true, // Feyenoord vs Sparta Rotterdam
}

// tokenAliases maps a normalized abbreviation or alias to the canonical token(s)
// used for comparison. Only club-unique, publicly known abbreviations belong
// here. Anything ambiguous must go to unidentifyingTokens instead, never here.
var tokenAliases = map[string][]string{
	"psg": {"paris", "saint", "germain"}, // Paris Saint-Germain
	"bvb": {"dortmund"},                  // Borussia Dortmund
}

// canonicalTokens splits a normalized string into tokens, expanding any alias.
func canonicalTokens(normName string) []string {
	fields := strings.Fields(normName)
	out := make([]string, 0, len(fields))
	for _, tok := range fields {
		if canon, ok := tokenAliases[tok]; ok {
			out = append(out, canon...)
			continue
		}
		out = append(out, tok)
	}
	return out
}

// clubAliases is the canonical club identity table: many published spellings
// (the ESPN API and the Gol Caracol parrilla disagree constantly) collapse onto
// one stable club id, the same idea as the canonical club id sports data
// providers use. It is the FIRST matching step and it closes two opposite
// failure modes at once:
//
//   - over-decline: a club whose name carries no distinctive token after
//     normalization ("Santa Fe" -> santa/fe, "Real Madrid" -> real/madrid,
//     "FC Barcelona" -> barcelona, "Atlético Nacional" -> atletico/nacional)
//     still resolves, because both sides collapse onto the same id;
//   - false positives: two DISTINCT clubs that share a token stay apart,
//     because distinct ids never match (FC Barcelona != Barcelona SC,
//     Bayern != Bayer, Real Madrid != Atlético Madrid, Inter != AC Milan).
//
// Keys are already-Normalize()d strings (lowercase, accents and FC/CF stripped);
// "FC"/"CF" prefixes disappear during normalization, so the keys below are the
// stripped forms. A name with no entry falls back to the distinctive-token
// logic, which still declines on ambiguity. Never add two different clubs under
// the same key: that is the bug this table exists to prevent.
var clubAliases = map[string]string{
	// --- Colombia: Liga BetPlay ---
	"santa fe":                         "santa-fe",
	"independiente santa fe":           "santa-fe",
	"atletico nacional":                "atletico-nacional",
	"atletico nacional medellin":       "atletico-nacional",
	"millonarios":                      "millonarios",
	"america de cali":                  "america-cali",
	"america cali":                     "america-cali",
	"deportivo cali":                   "deportivo-cali",
	"independiente medellin":           "independiente-medellin",
	"deportivo independiente medellin": "independiente-medellin",
	"dim":                              "independiente-medellin",
	"junior":                           "junior",
	"atletico junior":                  "junior",
	"junior de barranquilla":           "junior",
	"la equidad":                       "la-equidad",
	"equidad":                          "la-equidad",
	"aguilas doradas":                  "aguilas-doradas",
	"boyaca chico":                     "boyaca-chico",
	"once caldas":                      "once-caldas",
	"jaguares":                         "jaguares",
	"jaguares de cordoba":              "jaguares",
	"envigado":                         "envigado",
	"fortaleza ceif":                   "fortaleza-col",
	"llaneros":                         "llaneros-col",
	"deportes quindio":                 "deportes-quindio",
	"quindio":                          "deportes-quindio",
	"deportivo pereira":                "deportivo-pereira",
	"pereira":                          "deportivo-pereira",
	"deportes tolima":                  "deportes-tolima",
	"tolima":                           "deportes-tolima",
	"atletico bucaramanga":             "atletico-bucaramanga",
	"bucaramanga":                      "atletico-bucaramanga",
	"deportivo pasto":                  "deportivo-pasto",
	"pasto":                            "deportivo-pasto",
	"alianza":                          "alianza-col",
	"alianza petrolera":                "alianza-col",
	"union magdalena":                  "union-magdalena",
	"cucuta deportivo":                 "cucuta-deportivo",
	"cucuta":                           "cucuta-deportivo",
	"patriotas":                        "patriotas-boyaca",
	"patriotas boyaca":                 "patriotas-boyaca",
	"atletico huila":                   "atletico-huila",
	// Colombian clubs listed in the source that currently play in the second
	// division (they still appear in the parrilla).
	"real cartagena":           "real-cartagena",
	"leones":                   "leones-col",
	"leones de itagui":         "leones-col",
	"internacional de palmira": "internacional-palmira",
	"internacional palmira":    "internacional-palmira",

	// --- Uruguay / Paraguay / Bolivia / Chile / Peru / Ecuador / Venezuela ---
	"nacional":                  "nacional-uru",
	"nacional montevideo":       "nacional-uru",
	"club nacional de football": "nacional-uru",
	"nacional uruguay":          "nacional-uru",
	"penarol":                   "penarol",
	"danubio":                   "danubio",
	"montevideo city torque":    "montevideo-city",
	"montevideo city":           "montevideo-city",
	"liverpool montevideo":      "liverpool-montevideo",
	"cerro largo":               "cerro-largo",
	"boston river":              "boston-river",
	"racing montevideo":         "racing-montevideo",
	"club nacional asuncion":    "nacional-par",
	"nacional asuncion":         "nacional-par",
	"cerro porteno":             "cerro-porteno",
	"libertad":                  "libertad",
	"guarani":                   "guarani",
	"luqueno":                   "luqueno",
	"sportivo luqueno":          "luqueno",
	"general caballero":         "general-caballero",
	"sportivo ameliano":         "sportivo-ameliano",
	"trinidense":                "trinidense",
	"dos de mayo":               "dos-de-mayo",
	"club olimpia":              "olimpia-par",
	"olimpia paraguay":          "olimpia-par",
	"olimpia honduras":          "olimpia-hon",
	"bolivar":                   "bolivar",
	"the strongest":             "the-strongest",
	"always ready":              "always-ready",
	"blooming":                  "blooming",
	"guabira":                   "guabira",
	"nacional potosi":           "nacional-potosi",
	"universitario de vinto":    "universitario-bol",
	"real santa cruz":           "real-santa-cruz",
	"san jose bolivia":          "san-jose-bol",
	"aurora":                    "aurora",
	"oriente petrolero":         "oriente-petrolero",
	"jorge wilstermann":         "wilstermann",
	"colo colo":                 "colo-colo",
	"universidad de chile":      "universidad-de-chile",
	"universidad catolica":      "universidad-catolica",
	"huachipato":                "huachipato",
	"palestino":                 "palestino",
	"cobresal":                  "cobresal",
	"audax italiano":            "audax-italiano",
	"everton de vina del mar":   "everton-chile",
	"everton chile":             "everton-chile",
	"union la calera":           "union-la-calera",
	"o higgins":                 "o-higgins",
	"deportes iquique":          "deportes-iquique",
	"coquimbo unido":            "coquimbo-unido",
	"nublense":                  "nublense",
	"deportes la serena":        "la-serena",
	"universitario de deportes": "universitario-per",
	"universitario peru":        "universitario-per",
	"alianza lima":              "alianza-lima",
	"sporting cristal":          "sporting-cristal",
	"cienciano":                 "cienciano",
	"melgar":                    "melgar",
	"cesar vallejo":             "cesar-vallejo",
	"atletico grau":             "atletico-grau",
	"sport boys":                "sport-boys",
	"cusco":                     "cusco-fc",
	"deportivo garcilaso":       "deportivo-garcilaso",
	"adt tarma":                 "adt-tarma",
	"los chankas":               "los-chankas",
	"alianza atletico":          "alianza-atletico",
	"utc cajamarca":             "utc-cajamarca",
	"sport huancayo":            "sport-huancayo",
	"comerciantes unidos":       "comerciantes-unidos",
	"barcelona sc":              "barcelona-ecu",
	"barcelona sporting club":   "barcelona-ecu",
	"barcelona de guayaquil":    "barcelona-ecu",
	"barcelona guayaquil":       "barcelona-ecu",
	"liga de quito":             "liga-de-quito",
	"independiente del valle":   "independiente-del-valle",
	"emelec":                    "emelec",
	"aucas":                     "aucas",
	"delfin":                    "delfin",
	"el nacional":               "el-nacional-ecu",
	"caracas":                   "caracas-fc",
	"deportivo tachira":         "deportivo-tachira",
	"deportivo la guaira":       "deportivo-la-guaira",
	"metropolitanos":            "metropolitanos",
	"academia puerto cabello":   "academia-puerto-cabello",
	"estudiantes de merida":     "estudiantes-merida",
	"monagas":                   "monagas",
	"carabobo":                  "carabobo",
	"portuguesa":                "portuguesa-ven",
	"zamora":                    "zamora",
	"rayo zuliano":              "rayo-zuliano",
	"inter de barinas":          "inter-barinas",
	"ucv":                       "ucv",

	// --- Brazil: Brasileirão / Libertadores / Sudamericana ---
	"flamengo":             "flamengo",
	"palmeiras":            "palmeiras",
	"fluminense":           "fluminense",
	"botafogo":             "botafogo",
	"sao paulo":            "sao-paulo",
	"corinthians":          "corinthians",
	"internacional":        "internacional-bra",
	"gremio":               "gremio",
	"atletico mineiro":     "atletico-mineiro",
	"atletico paranaense":  "atletico-paranaense",
	"athletico paranaense": "atletico-paranaense",
	"cruzeiro":             "cruzeiro",
	"vasco da gama":        "vasco-da-gama",
	"bahia":                "bahia",
	"fortaleza ec":         "fortaleza-bra",
	"bragantino":           "bragantino",
	"rb bragantino":        "bragantino",
	"santos":               "santos",
	"juventude":            "juventude",
	"ceara":                "ceara",
	"sport recife":         "sport-recife",
	"vitoria":              "vitoria-bra",
	"mirassol":             "mirassol",
	"coritiba":             "coritiba",
	"cuiaba":               "cuiaba",
	"goias":                "goias",
	"america mineiro":      "america-mineiro",

	// --- Argentina: Liga Profesional / Libertadores / Sudamericana ---
	"river plate":               "river-plate",
	"boca juniors":              "boca-juniors",
	"racing club":               "racing-club",
	"racing club de avellaneda": "racing-club",
	"independiente":             "independiente-arg",
	"san lorenzo":               "san-lorenzo",
	"velez sarsfield":           "velez-sarsfield",
	"velez":                     "velez-sarsfield",
	"estudiantes":               "estudiantes",
	"estudiantes de la plata":   "estudiantes",
	"talleres":                  "talleres",
	"talleres de cordoba":       "talleres",
	"rosario central":           "rosario-central",
	"newell s old boys":         "newells-old-boys",
	"lanus":                     "lanus",
	"banfield":                  "banfield",
	"argentinos juniors":        "argentinos-juniors",
	"huracan":                   "huracan",
	"defensa y justicia":        "defensa-y-justicia",
	"godoy cruz":                "godoy-cruz",
	"platense":                  "platense",
	"instituto":                 "instituto",
	"central cordoba":           "central-cordoba",
	"tigre":                     "tigre-arg",
	"sarmiento":                 "sarmiento",
	"barracas central":          "barracas-central",
	"gimnasia":                  "gimnasia",
	"gimnasia la plata":         "gimnasia",
	"union santa fe":            "union-santa-fe",
	"belgrano":                  "belgrano",
	"independiente rivadavia":   "independiente-rivadavia",
	"aldosivi":                  "aldosivi",

	// --- Mexico: Liga MX ---
	"club america":       "america-mex",
	"america mexico":     "america-mex",
	"guadalajara":        "guadalajara",
	"chivas":             "guadalajara",
	"chivas guadalajara": "guadalajara",
	"cruz azul":          "cruz-azul",
	"pumas unam":         "pumas-unam",
	"pumas":              "pumas-unam",
	"unam":               "pumas-unam",
	"tigres uanl":        "tigres-uanl",
	"tigres":             "tigres-uanl",
	"monterrey":          "monterrey",
	"rayados":            "monterrey",
	"toluca":             "toluca",
	"leon":               "leon-mex",
	"pachuca":            "pachuca",
	"santos laguna":      "santos-laguna",
	"atlas":              "atlas",
	"necaxa":             "necaxa",
	"queretaro":          "queretaro",
	"puebla":             "puebla",
	"mazatlan":           "mazatlan",
	"juarez":             "juarez",
	"atletico san luis":  "atletico-san-luis",
	"tijuana":            "tijuana",
	"xolos":              "tijuana",

	// --- USA / Canada: MLS ---
	"inter miami":            "inter-miami",
	"lafc":                   "lafc",
	"los angeles":            "lafc",
	"la galaxy":              "la-galaxy",
	"los angeles galaxy":     "la-galaxy",
	"seattle sounders":       "seattle-sounders",
	"portland timbers":       "portland-timbers",
	"atlanta united":         "atlanta-united",
	"austin":                 "austin-fc",
	"charlotte":              "charlotte-fc",
	"chicago fire":           "chicago-fire",
	"cincinnati":             "fc-cincinnati",
	"colorado rapids":        "colorado-rapids",
	"columbus crew":          "columbus-crew",
	"dc united":              "dc-united",
	"dallas":                 "fc-dallas",
	"houston dynamo":         "houston-dynamo",
	"minnesota united":       "minnesota-united",
	"nashville sc":           "nashville-sc",
	"new england revolution": "new-england-revolution",
	"new york red bulls":     "new-york-red-bulls",
	"new york city":          "nycfc",
	"nycfc":                  "nycfc",
	"orlando city":           "orlando-city",
	"philadelphia union":     "philadelphia-union",
	"real salt lake":         "real-salt-lake",
	"san jose earthquakes":   "san-jose-earthquakes",
	"sporting kansas city":   "sporting-kc",
	"sporting kc":            "sporting-kc",
	"st louis city":          "st-louis-city",
	"toronto":                "toronto-fc",
	"vancouver whitecaps":    "vancouver-whitecaps",
	"montreal":               "cf-montreal",
	"san diego":              "san-diego-fc",

	// --- England: Premier League / EFL Championship ---
	"arsenal":              "arsenal",
	"aston villa":          "aston-villa",
	"bournemouth":          "bournemouth",
	"afc bournemouth":      "bournemouth",
	"brentford":            "brentford",
	"brighton":             "brighton",
	"brighton hove albion": "brighton",
	"burnley":              "burnley",
	"chelsea":              "chelsea",
	"crystal palace":       "crystal-palace",
	"everton":              "everton",
	"fulham":               "fulham",
	"leeds united":         "leeds-united",
	"leeds":                "leeds-united",
	"liverpool":            "liverpool",
	"manchester city":      "manchester-city",
	"man city":             "manchester-city",
	"manchester united":    "manchester-united",
	"man united":           "manchester-united",
	"man utd":              "manchester-united",
	"newcastle united":     "newcastle-united",
	"newcastle":            "newcastle-united",
	"nottingham forest":    "nottingham-forest",
	"nottingham":           "nottingham-forest",
	"sunderland":           "sunderland",
	"tottenham hotspur":    "tottenham",
	"tottenham":            "tottenham",
	"spurs":                "tottenham",
	"west ham":             "west-ham",
	"west ham united":      "west-ham",
	"wolverhampton":        "wolverhampton",
	"wolves":               "wolverhampton",
	"sheffield united":     "sheffield-united",
	"sheffield wednesday":  "sheffield-wednesday",
	"middlesbrough":        "middlesbrough",
	"coventry":             "coventry",
	"coventry city":        "coventry",
	"west bromwich albion": "west-brom",
	"west brom":            "west-brom",
	"norwich":              "norwich",
	"norwich city":         "norwich",
	"watford":              "watford",
	"millwall":             "millwall",
	"blackburn":            "blackburn",
	"blackburn rovers":     "blackburn",
	"bristol city":         "bristol-city",
	"swansea":              "swansea",
	"swansea city":         "swansea",
	"cardiff":              "cardiff",
	"cardiff city":         "cardiff",
	"hull city":            "hull-city",
	"stoke":                "stoke",
	"stoke city":           "stoke",
	"queens park rangers":  "qpr",
	"qpr":                  "qpr",
	"preston":              "preston",
	"preston north end":    "preston",
	"portsmouth":           "portsmouth",
	"oxford united":        "oxford-united",
	"luton":                "luton",
	"luton town":           "luton",
	"plymouth":             "plymouth",
	"plymouth argyle":      "plymouth",

	// --- Spain: LaLiga ---
	"barcelona":             "barcelona-esp",
	"real madrid":           "real-madrid",
	"atletico madrid":       "atletico-madrid",
	"atletico de madrid":    "atletico-madrid",
	"athletic club":         "athletic-bilbao",
	"athletic bilbao":       "athletic-bilbao",
	"athletic":              "athletic-bilbao",
	"real sociedad":         "real-sociedad",
	"villarreal":            "villarreal",
	"real betis":            "real-betis",
	"betis":                 "real-betis",
	"sevilla":               "sevilla",
	"valencia":              "valencia",
	"osasuna":               "osasuna",
	"celta vigo":            "celta-vigo",
	"celta":                 "celta-vigo",
	"getafe":                "getafe",
	"rayo vallecano":        "rayo-vallecano",
	"rayo":                  "rayo-vallecano",
	"mallorca":              "mallorca",
	"real mallorca":         "mallorca",
	"alaves":                "alaves",
	"deportivo alaves":      "alaves",
	"espanyol":              "espanyol",
	"rcd espanyol":          "espanyol",
	"espanyol de barcelona": "espanyol",
	"girona":                "girona",
	"levante":               "levante",
	"elche":                 "elche",
	"real oviedo":           "real-oviedo",
	"oviedo":                "real-oviedo",
	"real valladolid":       "real-valladolid",
	"valladolid":            "real-valladolid",
	"las palmas":            "las-palmas",

	// --- Italy: Serie A ---
	"inter milan":    "inter-milan",
	"inter de milan": "inter-milan",
	"internazionale": "inter-milan",
	"inter":          "inter-milan",
	"ac milan":       "ac-milan",
	"milan":          "ac-milan",
	"juventus":       "juventus",
	"juve":           "juventus",
	"napoli":         "napoli",
	"ssc napoli":     "napoli",
	"roma":           "roma",
	"as roma":        "roma",
	"lazio":          "lazio",
	"ss lazio":       "lazio",
	"atalanta":       "atalanta",
	"fiorentina":     "fiorentina",
	"acf fiorentina": "fiorentina",
	"bologna":        "bologna",
	"torino":         "torino",
	"udinese":        "udinese",
	"genoa":          "genoa",
	"genoa cfc":      "genoa",
	"cagliari":       "cagliari",
	"como":           "como",
	"lecce":          "lecce",
	"parma":          "parma",
	"verona":         "verona",
	"hellas verona":  "verona",
	"sassuolo":       "sassuolo",
	"pisa":           "pisa",
	"cremonese":      "cremonese",
	"venezia":        "venezia",

	// --- Germany: Bundesliga ---
	"bayern munich":            "bayern-munchen",
	"bayern munchen":           "bayern-munchen",
	"bayern de munich":         "bayern-munchen",
	"bayern de munchen":        "bayern-munchen",
	"bayern":                   "bayern-munchen",
	"bayer leverkusen":         "bayer-leverkusen",
	"bayer 04 leverkusen":      "bayer-leverkusen",
	"leverkusen":               "bayer-leverkusen",
	"bayer":                    "bayer-leverkusen",
	"borussia dortmund":        "borussia-dortmund",
	"dortmund":                 "borussia-dortmund",
	"bvb":                      "borussia-dortmund",
	"rb leipzig":               "rb-leipzig",
	"leipzig":                  "rb-leipzig",
	"eintracht frankfurt":      "eintracht-frankfurt",
	"frankfurt":                "eintracht-frankfurt",
	"vfb stuttgart":            "vfb-stuttgart",
	"stuttgart":                "vfb-stuttgart",
	"wolfsburg":                "wolfsburg",
	"freiburg":                 "freiburg",
	"sc freiburg":              "freiburg",
	"hoffenheim":               "hoffenheim",
	"mainz 05":                 "mainz-05",
	"mainz":                    "mainz-05",
	"augsburg":                 "augsburg",
	"werder bremen":            "werder-bremen",
	"union berlin":             "union-berlin",
	"borussia monchengladbach": "borussia-monchengladbach",
	"monchengladbach":          "borussia-monchengladbach",
	"bochum":                   "bochum",
	"heidenheim":               "heidenheim",
	"st pauli":                 "st-pauli",
	"hamburger sv":             "hamburger-sv",
	"hamburgo":                 "hamburger-sv",
	"koln":                     "koln",
	"cologne":                  "koln",

	// --- France: Ligue 1 ---
	"paris saint germain": "paris-saint-germain",
	"paris sg":            "paris-saint-germain",
	"psg":                 "paris-saint-germain",
	"olympique marseille": "olympique-marseille",
	"marseille":           "olympique-marseille",
	"monaco":              "monaco",
	"as monaco":           "monaco",
	"lille":               "lille",
	"losc lille":          "lille",
	"nice":                "nice",
	"ogc nice":            "nice",
	"lyon":                "lyon",
	"olympique lyonnais":  "lyon",
	"rennes":              "rennes",
	"stade rennais":       "rennes",
	"lens":                "lens",
	"rc lens":             "lens",
	"strasbourg":          "strasbourg",
	"nantes":              "nantes",
	"brest":               "brest",
	"toulouse":            "toulouse",
	"auxerre":             "auxerre",
	"angers":              "angers",
	"le havre":            "le-havre",
	"metz":                "metz",
	"lorient":             "lorient",
	"saint etienne":       "saint-etienne",

	// --- Netherlands: Eredivisie ---
	"ajax":                "ajax",
	"psv":                 "psv",
	"psv eindhoven":       "psv",
	"feyenoord":           "feyenoord",
	"feyenoord rotterdam": "feyenoord",
	"az alkmaar":          "az-alkmaar",
	"az":                  "az-alkmaar",
	"twente":              "twente",
	"utrecht":             "utrecht",
	"sparta rotterdam":    "sparta-rotterdam",
	"heerenveen":          "heerenveen",
	"go ahead eagles":     "go-ahead-eagles",
	"nec nijmegen":        "nec-nijmegen",
	"nec":                 "nec-nijmegen",
	"groningen":           "groningen",
	"fortuna sittard":     "fortuna-sittard",
	"heracles":            "heracles",
	"zwolle":              "zwolle",
	"willem ii":           "willem-ii",
	"rkc waalwijk":        "rkc-waalwijk",
	"almere city":         "almere-city",
	"nac breda":           "nac-breda",

	// --- Portugal: Liga Portugal ---
	"benfica":            "benfica",
	"sl benfica":         "benfica",
	"porto":              "fc-porto",
	"sporting cp":        "sporting-cp",
	"sporting lisbon":    "sporting-cp",
	"sporting de lisboa": "sporting-cp",
	"braga":              "braga",
	"sc braga":           "braga",
	"vitoria guimaraes":  "vitoria-guimaraes",
	"vitoria sc":         "vitoria-guimaraes",
	"famalicao":          "famalicao",
	"moreirense":         "moreirense",
	"arouca":             "arouca",
	"estoril":            "estoril",
	"boavista":           "boavista",
	"rio ave":            "rio-ave",
	"santa clara":        "santa-clara",
	"casa pia":           "casa-pia",
	"estrela amadora":    "estrela-amadora",
	"gil vicente":        "gil-vicente",
	"farense":            "farense",
	"tondela":            "tondela",
	"avs":                "avs",

	// --- Turkey: Süper Lig ---
	"galatasaray":         "galatasaray",
	"fenerbahce":          "fenerbahce",
	"fenerbahce sk":       "fenerbahce",
	"besiktas":            "besiktas",
	"trabzonspor":         "trabzonspor",
	"basaksehir":          "basaksehir",
	"istanbul basaksehir": "basaksehir",
	"samsunspor":          "samsunspor",
	"konyaspor":           "konyaspor",
	"kayserispor":         "kayserispor",
	"alanyaspor":          "alanyaspor",
	"antalyaspor":         "antalyaspor",
	"gaziantep":           "gaziantep",
	"rizespor":            "rizespor",
	"kasimpasa":           "kasimpasa",
	"goztepe":             "goztepe",
	"eyupspor":            "eyupspor",
	"bodrum":              "bodrum",
	"karagumruk":          "karagumruk",
	"hatayspor":           "hatayspor",
	"adana demirspor":     "adana-demirspor",

	// --- Rest of UEFA (Champions / Europa / Conference) ---
	"celtic":            "celtic",
	"rangers":           "rangers",
	"club brugge":       "club-brugge",
	"olympiacos":        "olympiacos",
	"olympiakos":        "olympiacos",
	"red star belgrade": "red-star-belgrade",
	"crvena zvezda":     "red-star-belgrade",
	"young boys":        "young-boys",
	"copenhagen":        "fc-copenhagen",
	"slavia prague":     "slavia-prague",
	"sparta prague":     "sparta-prague",
	"viktoria plzen":    "viktoria-plzen",
	"salzburg":          "salzburg",
	"shakhtar donetsk":  "shakhtar",
	"dynamo kyiv":       "dynamo-kyiv",
	"bodo glimt":        "bodo-glimt",
	"malmo":             "malmo",
	"ferencvaros":       "ferencvaros",
	"qarabag":           "qarabag",
	"slovan bratislava": "slovan-bratislava",
}

// clubAliasEntry is one alias phrase (as a token slice) bound to a club id.
type clubAliasEntry struct {
	id     string
	tokens []string
}

// clubAliasIndex maps the first token of an alias to every alias phrase that
// starts with it, longest phrase first. Sorting is what makes guide scanning
// maximal-munch: at a given position the longest matching alias wins, so
// "barcelona sc" beats "barcelona" and "inter de milan" beats "inter".
var clubAliasIndex = buildClubAliasIndex()

func buildClubAliasIndex() map[string][]clubAliasEntry {
	idx := make(map[string][]clubAliasEntry, len(clubAliases))
	for phrase, id := range clubAliases {
		toks := strings.Fields(phrase)
		if len(toks) == 0 {
			continue
		}
		idx[toks[0]] = append(idx[toks[0]], clubAliasEntry{id: id, tokens: toks})
	}
	for first, entries := range idx {
		sort.SliceStable(entries, func(i, j int) bool {
			return len(entries[i].tokens) > len(entries[j].tokens)
		})
		idx[first] = entries
	}
	return idx
}

// canonicalClubID returns the canonical club id for an already-normalized team
// name, or "" when the name is not a known club.
func canonicalClubID(normName string) string {
	return clubAliases[normName]
}

// tokensEqual reports whether two token slices are identical element-wise.
func tokensEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// longestAliasAt returns the end index and club id of the longest known club
// alias matching the tokens starting at i, or ok=false when none match.
func longestAliasAt(toks []string, i int) (next int, id string, ok bool) {
	for _, e := range clubAliasIndex[toks[i]] {
		if i+len(e.tokens) > len(toks) {
			continue
		}
		if tokensEqual(toks[i:i+len(e.tokens)], e.tokens) {
			return i + len(e.tokens), e.id, true
		}
	}
	return 0, "", false
}

// guideClubIDs returns the set of canonical club ids mentioned in an
// already-normalized guide title. Scanning is left-to-right with maximal munch,
// so a more specific alias always wins over a shorter prefix and a consumed
// phrase is never re-examined. That is what keeps two clubs that share a token
// (FC Barcelona vs Barcelona SC, Inter Milan vs Inter Miami) from bleeding into
// each other.
func guideClubIDs(normTitle string) map[string]struct{} {
	toks := strings.Fields(normTitle)
	ids := make(map[string]struct{})
	for i := 0; i < len(toks); {
		next, id, ok := longestAliasAt(toks, i)
		if !ok {
			i++
			continue
		}
		ids[id] = struct{}{}
		i = next
	}
	return ids
}

// keywords returns the meaningful tokens of a normalized name: aliases are
// expanded, and stopwords or tokens shorter than three characters are dropped.
func keywords(normName string) []string {
	var out []string
	for _, tok := range canonicalTokens(normName) {
		if len(tok) < 3 || stopwords[tok] {
			continue
		}
		out = append(out, tok)
	}
	return out
}

// ExtractKeywords returns distinctive words from a normalized team name.
func ExtractKeywords(normName string) []string {
	return keywords(normName)
}

// distinctiveTokens keeps only the keywords that can identify a single club,
// dropping shared generic/club-qualifier tokens.
func distinctiveTokens(normName string) []string {
	var out []string
	for _, tok := range keywords(normName) {
		if unidentifyingTokens[tok] {
			continue
		}
		out = append(out, tok)
	}
	return out
}

// guideTokenSet returns the set of canonical tokens present in a normalized
// guide title.
func guideTokenSet(normTitle string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, tok := range canonicalTokens(normTitle) {
		set[tok] = struct{}{}
	}
	return set
}

// teamIdentified reports whether the guide title identifies this team.
//
// Canonical identity comes first: when the team maps to a known club id, the
// guide must contain that SAME id. This resolves clubs whose name carries no
// distinctive token (Santa Fe, Real Madrid, Atlético Nacional) while keeping two
// distinct clubs that share a token apart (FC Barcelona never matches Barcelona
// SC, Bayern never matches Bayer).
//
// An unknown club falls back to the conservative distinctive-token logic: at
// least one of its DISTINCTIVE tokens must appear verbatim (exact equality, so
// "bayer" never matches "bayern"), and a team whose name reduces to shared
// tokens only is never identified.
func teamIdentified(normTeam string, guideIDs, guideTokens map[string]struct{}) bool {
	if id := canonicalClubID(normTeam); id != "" {
		_, ok := guideIDs[id]
		return ok
	}
	for _, tok := range distinctiveTokens(normTeam) {
		if _, ok := guideTokens[tok]; ok {
			return true
		}
	}
	return false
}

// isFixturePhrase reports whether a normalized title reads as a fixture
// ("home vs away") rather than a single club name. Only "vs"/"v" survive
// normalization as separators, so a bare club name can never be mistaken for a
// full fixture title.
func isFixturePhrase(normTitle string) bool {
	if normTitle == "" {
		return false
	}
	padded := " " + normTitle + " "
	return strings.Contains(padded, " vs ") || strings.Contains(padded, " v ")
}

// MatchScore evaluates how well a TV guide title matches a soccer match.
//
// A channel is only considered to be airing the fixture when BOTH teams are
// identified in the guide title, or when the guide title contains the full
// fixture title. A single team name is never sufficient: a lone team can name a
// different competition, a studio panel, or a namesake club, and resolving on it
// is exactly how the wrong broadcast gets picked.
//
// Returns:
//
//	matched: true only for a full fixture-title match or a genuine both-teams match
//	score:   10 (full fixture title), 5 (both teams), 0 (no match)
func MatchScore(guideTitle string, match model.Match) (bool, int) {
	gNorm := Normalize(guideTitle)
	if gNorm == "" {
		return false, 0
	}

	tNorm := Normalize(match.Title)

	// 1. Full fixture-title match (e.g. "Roma vs. Real Madrid" against
	// "AS Roma vs Real Madrid"). Both sides must read as a fixture phrase so a
	// bare club name can never masquerade as a full title.
	if isFixturePhrase(tNorm) && isFixturePhrase(gNorm) {
		if strings.Contains(gNorm, tNorm) || strings.Contains(tNorm, gNorm) {
			return true, 10
		}
	}

	// 2. Both teams must be identified. Partial evidence is not accepted.
	gTokens := guideTokenSet(gNorm)
	gIDs := guideClubIDs(gNorm)
	hIdentified := teamIdentified(Normalize(match.HomeTeam.DisplayName), gIDs, gTokens)
	aIdentified := teamIdentified(Normalize(match.AwayTeam.DisplayName), gIDs, gTokens)
	if hIdentified && aIdentified {
		return true, 5
	}

	return false, 0
}
