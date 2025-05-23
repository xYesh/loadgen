package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dgryski/go-wyhash"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// spices is a list of common spices
var spices = []string{
	"allspice", "anise", "basil", "bay", "black pepper", "cardamom", "cayenne",
	"cinnamon", "cloves", "coriander", "cumin", "curry", "dill", "fennel", "fenugreek",
	"garlic", "ginger", "marjoram", "mustard", "nutmeg", "oregano", "paprika", "parsley",
	"pepper", "rosemary", "saffron", "sage", "salt", "tarragon", "thyme", "turmeric", "vanilla",
	"caraway", "chili", "masala", "lemongrass", "mint", "poppy", "sesame", "sumac", "mace",
	"nigella", "peppercorn", "wasabi",
}

// adjectives is a list of common adjectives
var adjectives = []string{
	"able", "bad", "best", "better", "big", "black", "certain", "clear", "different", "early",
	"easy", "economic", "federal", "free", "full", "good", "great", "hard", "high", "human",
	"important", "international", "large", "late", "little", "local", "long", "low", "major",
	"military", "national", "new", "old", "only", "other", "political", "possible", "public",
	"real", "recent", "right", "small", "social", "special", "strong", "sure", "true", "white",
	"whole", "young",
}

// nouns is a list of common nouns
var nouns = []string{
	"angle", "ant", "apple", "arch", "arm", "army", "baby", "bag", "ball", "band", "basin", "basket", "bath", "bed", "bee", "bell",
	"berry", "bird", "blade", "board", "boat", "bone", "book", "boot", "bottle", "box", "boy", "brain", "brake", "branch", "brick", "bridge",
	"brush", "bucket", "bulb", "button", "cake", "camera", "card", "carriage", "cart", "cat", "chain", "cheese", "chess", "chin", "church", "circle",
	"clock", "cloud", "coat", "collar", "comb", "cord", "cow", "cup", "curtain", "cushion", "dog", "door", "drain", "drawer", "dress", "drop",
	"ear", "egg", "engine", "eye", "face", "farm", "feather", "finger", "fish", "flag", "floor", "fly", "foot", "fork", "fowl", "frame",
	"garden", "girl", "glove", "goat", "gun", "hair", "hammer", "hand", "hat", "head", "heart", "hook", "horn", "horse", "hospital", "house",
	"island", "jewel", "kettle", "key", "knee", "knife", "knot", "leaf", "leg", "library", "line", "lip", "lock", "map", "match", "monkey",
	"moon", "mouth", "muscle", "nail", "neck", "needle", "nerve", "net", "nose", "nut", "office", "orange", "oven", "parcel", "pen", "pencil",
	"picture", "pig", "pin", "pipe", "plane", "plate", "plough", "pocket", "pot", "potato", "prison", "pump", "rail", "rat", "receipt", "ring",
	"rod", "roof", "root", "sail", "school", "scissors", "screw", "seed", "sheep", "shelf", "ship", "shirt", "shoe", "skin", "skirt", "snake",
	"sock", "spade", "sponge", "spoon", "spring", "square", "stamp", "star", "station", "stem", "stick", "stocking", "stomach", "store", "street", "sun",
	"table", "tail", "thread", "throat", "thumb", "ticket", "toe", "tongue", "tooth", "town", "train", "tray", "tree", "trousers", "umbrella", "wall",
	"watch", "wheel", "whip", "whistle", "window", "wing", "wire", "worm",
}

// constfield is a field that *doesn't* start with slash
var constfield = regexp.MustCompile(`^([^/].*)$`)

// genfield is used to parse generator fields by matching valid commands and numeric arguments
var genfield = regexp.MustCompile(`^/([ibfsuk][awxrgqtp]?[c]?)([0-9.-]+)?(?:,([0-9.-]+))?(?:,([0-9.-]+))?(?:,([0-9.-]+))?$`)

// keysplitter separates fields that look like number.name (ex: 1.myfield)
var keysplitter = regexp.MustCompile(`^([0-9]+)\.(.*$)`)

type Rng struct {
	rng *rand.Rand
}

func NewRng(seed string) Rng {
	return Rng{rand.New(rand.NewSource(int64(wyhash.Hash([]byte(seed), 2467825690))))}
}

func (r Rng) Intn(n int) int64 {
	return int64(r.rng.Intn(n))
}

// Chooses a random element from a slice of strings.
func (r Rng) Choice(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return a[r.Intn(len(a))]
}

// Chooses a random element from a slice of strings, with a quadratic bias
// towards the first elements.
func (r Rng) QuadraticChoice(a []string) string {
	sq := float64(len(a) * len(a))
	rn := r.Float(0, sq)
	choice := len(a) - int(math.Floor(math.Sqrt(rn))) - 1
	return a[choice]
}

func (r Rng) Bool() bool {
	return r.Intn(2) == 0
}

func (r Rng) Int(min, max int) int64 {
	return int64(r.rng.Intn(max-min) + min)
}

func (r Rng) Float(min, max float64) float64 {
	return r.rng.Float64()*(max-min) + min
}

func (r Rng) Gaussian(mean, stddev float64) float64 {
	return r.rng.NormFloat64()*stddev + mean
}

func (r Rng) GaussianInt(mean, stddev float64) int64 {
	return int64(r.rng.NormFloat64()*stddev + mean)
}

func (r Rng) String(len int) string {
	var b strings.Builder
	for i := 0; i < len; i++ {
		b.WriteByte(byte("abcdefghijklmnopqrstuvwxyz"[r.Int(0, 26)]))
	}
	return b.String()
}

func (r Rng) HexString(len int) string {
	var b strings.Builder
	for i := 0; i < len; i++ {
		b.WriteByte(byte("0123456789abcdef"[r.Int(0, 16)]))
	}
	return b.String()
}

func (r Rng) WordPair() string {
	return r.Choice(adjectives) + "-" + r.Choice(nouns)
}

func (r Rng) BoolWithProb(p float64) bool {
	return r.Float(0, 100) < p
}

// getProcessID returns the process ID
func getProcessID() int64 {
	return int64(os.Getpid())
}

func (r Rng) getValueGenerators() []func() any {
	return []func() any{
		func() any { return r.Intn(100) },
		func() any { return r.BoolWithProb(99) },
		func() any { return r.BoolWithProb(50) },
		func() any { return r.BoolWithProb(1) },
		func() any { return r.Int(-100, 100) },
		func() any { return r.Float(0, 1000) },
		func() any { return r.Float(0, 1) },
		func() any { return r.GaussianInt(50, 30) },
		func() any { return r.Gaussian(10000, 1000) },
		func() any { return r.Gaussian(500, 300) },
		func() any { return r.String(2) },
		func() any { return r.String(5) },
		func() any { return r.String(10) },
		func() any { return r.String(4) + "-" + r.HexString(8) + "-" + r.String(4) },
		func() any { return r.HexString(16) },
	}
}

// getWordList returns a list of words with the specified cardinality;
// if a source word list is specified and cardinality fits within it, it uses it.
func getWordList(rng Rng, cardinality int, source []string) []string {
	generator := rng.WordPair
	if source != nil && len(source) >= cardinality {
		generator = func() string { return rng.Choice(source) }
	}
	words := make([]string, cardinality)
	for i := 0; i < cardinality; i++ {
		words[i] = generator()
	}
	return words
}

type EligibilityPeriod struct {
	word  string
	start time.Duration
	end   time.Duration
}

type PeriodicEligibility struct {
	rng     Rng
	periods []EligibilityPeriod
	period  time.Duration
}

// generates a list of eligibility periods for a set of words
// each word is eligible for some period of time that is proportional to its position in the list
// this is so that all the words are not available at the same time, but eventually all of them are
func newPeriodicEligibility(rng Rng, words []string, period time.Duration) *PeriodicEligibility {
	cardinality := len(words)
	periods := make([]EligibilityPeriod, cardinality)
	for i := 0; i < cardinality; i++ {
		// calculate a period length that is proportional to the number of remaining words
		periodLength := time.Duration(float64(period) * float64(cardinality-i) / float64(cardinality))
		// startTime is a random value that ensures it will end before the next period starts
		startTime := (period - periodLength) * time.Duration(rng.Float(0, 1))
		periods[i] = EligibilityPeriod{
			word:  words[i],
			start: startTime,
			end:   startTime + periodLength,
		}
	}
	return &PeriodicEligibility{
		rng:     rng,
		periods: periods,
		period:  period,
	}
}

// gets one word from the list of eligible words based on the time since the start of the period
// This is, on average, slower than the random selection, but the random one can sometimes
// be very slow, so we use this as a fallback if we try randomly a few times and fail.
func (pe *PeriodicEligibility) getEligibleWordFallback(durationSinceStart time.Duration) string {
	tInPeriod := durationSinceStart % pe.period
	eligibleIndexes := make([]int, 0, 20)
	for i, period := range pe.periods {
		if period.start <= tInPeriod && tInPeriod < period.end {
			eligibleIndexes = append(eligibleIndexes, i)
		}
	}

	if len(eligibleIndexes) == 0 {
		// shouldn't happen, but if it does, just pick the first word
		return pe.periods[0].word
	}
	ix := eligibleIndexes[pe.rng.Intn(len(eligibleIndexes))]
	return pe.periods[ix].word
}

func (pe *PeriodicEligibility) getEligibleWord(durationSinceStart time.Duration) string {
	tInPeriod := durationSinceStart % pe.period
	// try 10 times to find an eligible word
	for i := 0; i < 5; i++ {
		ix := pe.rng.Intn(len(pe.periods))
		period := pe.periods[ix]
		if period.start <= tInPeriod && tInPeriod < period.end {
			return period.word
		}
	}
	// use the fallback
	return pe.getEligibleWordFallback(durationSinceStart)
}

// parseUserFields expects a list of fields in the form of name=constant or name=/gen.
// See README.md for more information.
func parseUserFields(rng Rng, userfields map[string]string) (map[string]func() any, error) {
	// groups                                        1                   2	         3         4
	fields := make(map[string]func() any)
	for name, value := range userfields {
		// see if it's a constant
		if constfield.MatchString(value) {
			fields[name] = getConst(value)
			continue
		}

		// see if it's a generator
		matches := genfield.FindStringSubmatch(value)
		if matches == nil {
			return nil, fmt.Errorf("unparseable user field %s=%s", name, value)
		}
		var err error
		gentype := matches[1]
		p1 := matches[2]
		p2 := matches[3]
		p3 := matches[4]
		p4 := matches[5]
		switch gentype {
		case "ip":
			fields[name], err = getIpGen(rng, p1, p2, p3, p4)
			if err != nil {
				return nil, fmt.Errorf("invalid int in user field %s=%s: %w", name, value, err)
			}
		case "i", "ir", "ig":
			fields[name], err = getIntGen(rng, gentype, p1, p2)
			if err != nil {
				return nil, fmt.Errorf("invalid int in user field %s=%s: %w", name, value, err)
			}
		case "f", "fr", "fg":
			fields[name], err = getFloatGen(rng, gentype, p1, p2)
			if err != nil {
				return nil, fmt.Errorf("invalid float in user field %s=%s: %w", name, value, err)
			}
		case "b":
			n := 50.0
			var err error
			if p1 != "" {
				n, err = strconv.ParseFloat(p1, 64)
				if err != nil || n < 0 || n > 100 {
					return nil, fmt.Errorf("invalid bool option in %s=%s", name, value)
				}
			}
			fields[name] = func() any { return rng.BoolWithProb(n) }
		case "s", "sw", "sx", "sa", "sq", "sxc":
			n := 16
			if p1 != "" {
				n, err = strconv.Atoi(p1)
				if err != nil {
					return nil, fmt.Errorf("invalid string option in %s=%s", name, value)
				}
			}
			switch gentype {
			case "sw":
				// words with specified cardinality in a rectangular distribution
				words := getWordList(rng, n, nil)
				fields[name] = func() any { return rng.Choice(words) }
			case "sq":
				// words with specified cardinality in a quadratic distribution
				words := getWordList(rng, n, nil)
				fields[name] = func() any { return rng.QuadraticChoice(words) }
			case "sx":
				fields[name] = func() any { return rng.HexString(n) }
			case "sxc":
				fields[name], err = genHexStringWithCardinality(rng, p1, p2)
				if err != nil {
					return nil, fmt.Errorf("invalid int in user field %s=%s: %w", name, value, err)
				}
			default:
				fields[name] = func() any { return rng.String(n) }
			}
		case "k":
			fields[name], err = getKeyGen(rng, p1, p2)
			if err != nil {
				return nil, fmt.Errorf("invalid key in key field %s=%s: %w", name, value, err)
			}
		case "u", "uq":
			// Generate a URL-like string with a random path and possibly a query string
			fields[name], err = getURLGen(rng, gentype, p1, p2)
			if err != nil {
				return nil, fmt.Errorf("invalid float in user field %s=%s: %w", name, value, err)
			}
		case "st":
			// Generate a semi-plausible mix of status codes; percentage of 400s and 500s can be controlled by the extra args
			twos := 95.0
			fours := 4.0
			fives := 1.0
			if p1 != "" {
				fours, err = strconv.ParseFloat(p1, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid float in user field %s=%s: %w", name, value, err)
				}
			}
			if p2 != "" {
				fives, err = strconv.ParseFloat(p2, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid float in user field %s=%s: %w", name, value, err)
				}
			}
			twos = 100 - fours - fives
			fields[name] = func() any {
				r := rng.Float(0, 100)
				if r < twos {
					return rng.QuadraticChoice([]string{"200", "200", "200", "201", "202"})
				} else if r < twos+fours {
					return rng.QuadraticChoice([]string{"404", "400", "400", "400", "402", "429", "403"})
				} else {
					return "500"
				}
			}

		default:
			return nil, fmt.Errorf("invalid generator type %s in field %s=%s", gentype, name, value)
		}
	}
	return fields, nil
}

func getConst(value string) func() any {
	var gen func() any
	if value == "true" {
		gen = func() any { return true }
	} else if value == "false" {
		gen = func() any { return false }
	} else {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			gen = func() any { return i }
		} else if f, err := strconv.ParseFloat(value, 64); err == nil {
			gen = func() any { return f }
		} else {
			gen = func() any { return value }
		}
	}
	return gen
}

func gaussianDefaults(v1, v2 float64) (float64, float64) {
	if v1 == 0 && v2 == 0 {
		v1 = 100
		v2 = 10
	} else if v2 == 0 {
		v2 = v1 / 10
	}
	return v1, v2
}

func genHexStringWithCardinality(rng Rng, p1 string, p2 string) (func() any, error) {

	var v1, length int
	var err error
	if p1 != "" {
		length, err = strconv.Atoi(p1)
		if err != nil {
			return nil, fmt.Errorf("%s is not an int", p1)
		}
		if length >= 64 {
			return nil, fmt.Errorf("sxc length can only be a max of 63")
		}
	} else {
		length = 16
	}
	if p2 != "" {
		v1, err = strconv.Atoi(p2)
		if err != nil {
			return nil, fmt.Errorf("%s is not an int", p2)
		}
	} else {
		v1 = 16
	}

	return func() any {
		var number = rng.rng.Intn(v1)

		data := fmt.Sprintf("%d:%d", number, length)
		//Can switch to non-cryptographic hashes for higher throughputs
		hash := sha256.Sum256([]byte(data))
		hashString := hex.EncodeToString(hash[:])

		// Truncate the hash to the specified length
		if length > len(hashString) {
			return hashString
		}
		return hashString[:length]
	}, nil
}

func getIpGen(rng Rng, p1, p2, p3, p4 string) (func() any, error) {

	var v1, v2, v3, v4 int
	var err error
	v1, err = strconv.Atoi(p1)
	if err != nil {
		return nil, fmt.Errorf("%s is not an int", p1)
	}
	v2, err = strconv.Atoi(p2)
	if err != nil {
		return nil, fmt.Errorf("%s is not an int", p2)
	}
	v3, err = strconv.Atoi(p3)
	if err != nil {
		return nil, fmt.Errorf("%s is not an int", p3)
	}
	v4, err = strconv.Atoi(p4)
	if err != nil {
		return nil, fmt.Errorf("%s is not an int", p4)
	}

	return func() any {
		return fmt.Sprintf("%d.%d.%d.%d", rng.rng.Intn(v1), rng.rng.Intn(v2), rng.rng.Intn(v3), rng.rng.Intn(v4))
	}, nil
}

func getIntGen(rng Rng, gentype, p1, p2 string) (func() any, error) {
	var v1, v2 int
	var err error
	if p1 == "" {
		v1 = 0
	} else {
		v1, err = strconv.Atoi(p1)
		if err != nil {
			return nil, fmt.Errorf("%s is not an int", p1)
		}
	}
	if p2 == "" || p2 == "," {
		v2 = v1
		v1 = 0
	} else {
		v2, err = strconv.Atoi(p2)
		if err != nil {
			return nil, fmt.Errorf("%s is not an int", p2[:1])
		}
	}
	if gentype == "ig" {
		g1, g2 := gaussianDefaults(float64(v1), float64(v2))
		return func() any { return rng.GaussianInt(g1, g2) }, nil
	} else {
		if v1 == 0 && v2 == 0 {
			v2 = 100
		}
		return func() any { return rng.Int(v1, v2) }, nil
	}
}

func getFloatGen(rng Rng, gentype, p1, p2 string) (func() any, error) {
	var v1, v2 float64
	var err error
	if p1 == "" {
		v1 = 0
	} else {
		v1, err = strconv.ParseFloat(p1, 64)
		if err != nil {
			return nil, fmt.Errorf("%s is not a number", p1)
		}
	}
	if p2 == "" || p2 == "," {
		v2 = v1
		v1 = 0
	} else {
		v2, err = strconv.ParseFloat(p2, 64)
		if err != nil {
			return nil, fmt.Errorf("%s is not a number", p2[:1])
		}
	}
	if gentype == "fg" {
		g1, g2 := gaussianDefaults(v1, v2)
		return func() any { return rng.GaussianInt(g1, g2) }, nil
	} else {
		if v1 == 0 && v2 == 0 {
			v2 = 100
		}
		return func() any { return rng.Float(v1, v2) }, nil
	}
}

func getURLGen(rng Rng, gentype, p1, p2 string) (func() any, error) {
	var c1 int = 3
	var c2 int = 10
	var err error
	if p1 != "" {
		c1, err = strconv.Atoi(p1)
		if err != nil {
			return nil, fmt.Errorf("%s is not a number", p1)
		}
	}
	if p2 != "" && p2 != "," {
		c2, err = strconv.Atoi(p2)
		if err != nil {
			return nil, fmt.Errorf("%s is not a number", p2[:1])
		}
	}
	path1words := getWordList(rng, c1, nouns)
	path1 := func() string { return rng.Choice(path1words) }
	path2 := func() string { return "" }
	if c2 != 0 {
		path2words := getWordList(rng, c2, adjectives)
		path2 = func() string { return rng.Choice(path2words) }
	}
	if gentype == "uq" {
		return func() any {
			return "https://example.com/" + path1() + "/" + path2() + "?extra=" + rng.String(10)
		}, nil
	} else {
		return func() any {
			return "https://example.com/" + path1() + "/" + path2()
		}, nil
	}
}

func getKeyGen(rng Rng, p1, p2 string) (func() any, error) {
	var cardinality, period int
	var err error
	if p1 == "" {
		cardinality = 50
	} else {
		cardinality, err = strconv.Atoi(p1)
		if err != nil {
			return nil, fmt.Errorf("%s is not an int", p1)
		}
		if cardinality > len(nouns) {
			return nil, fmt.Errorf("cardinality %d cannot be more than %d", cardinality, len(nouns))
		}
	}
	if p2 == "" || p2 == "," {
		period = 60
	} else {
		period, err = strconv.Atoi(p2)
		if err != nil {
			return nil, fmt.Errorf("%s is not an int", p2[:1])
		}
	}
	ep := newPeriodicEligibility(rng, nouns[:cardinality], time.Duration(period)*time.Second)
	startTime := time.Now()
	return func() any { return ep.getEligibleWord(time.Since(startTime)) }, nil
}

type Fielder struct {
	fields              map[string]func() any
	names               []string
	keys                []string
	attributesPerSpan   int
	intrinsicAttributes int
}

// Fielder is an object that takes a name and generates a map of
// fields based on using the name as a random seed.
// It takes a set of field specifications that are used to generate the fields.
// It also takes two counts: the number of fields to generate and the number of
// service names to generate. The field names are randomly generated by
// combining an adjective and a noun and are consistent for a given fielder.
// The field values are randomly generated.
// Fielder also includes the process_id.
func NewFielder(seed string, userFields map[string]string, nextras, nservices int, attributesPerSpan int, intrinsicAttributes int) (*Fielder, error) {
	rng := NewRng(seed)
	gens := rng.getValueGenerators()
	fields, err := parseUserFields(rng, userFields)
	var keys []string
	if err != nil {
		return nil, err
	}
	for i := 0; i < nextras; i++ {
		fieldname := rng.WordPair()
		fields[fieldname] = gens[rng.Intn(len(gens))]
	}
	fields["process_id"] = func() any { return getProcessID() }
	for k, _ := range fields {
		keys = append(keys, k)
	}
	names := make([]string, nservices)
	for i := 0; i < nservices; i++ {
		names[i] = rng.Choice(spices)
	}

	var validAttributesPerSpan = int(math.Min(float64(attributesPerSpan), float64(len(fields))))
	var validIntrinsicAttributes = int(math.Min(float64(intrinsicAttributes), float64(validAttributesPerSpan)))
	return &Fielder{fields: fields, names: names, keys: keys, attributesPerSpan: validAttributesPerSpan, intrinsicAttributes: validIntrinsicAttributes}, nil
}

func (f *Fielder) GetServiceName(n int) string {
	return f.names[n%len(f.names)]
}

// Searches for a field name that includes a level marker.
// These markers look like "1.fieldname" and are used to
// indicate that the field should be included at a specific
// level in the trace, where 0 is the root.
func (f *Fielder) atLevel(name string, level int) (string, bool) {
	matches := keysplitter.FindStringSubmatch(name)
	if len(matches) == 0 {
		return name, true
	}
	keylevel, _ := strconv.Atoi(matches[1])
	if keylevel == level {
		return matches[2], true
	}
	return matches[2], false
}

func (f *Fielder) GetFields(count int64, level int) map[string]any {
	fields := make(map[string]any)
	if count != 0 {
		fields["count"] = count
	}
	for k, v := range f.fields {
		k, ok := f.atLevel(k, level)
		if !ok {
			continue
		}
		fields[k] = v()
	}
	return fields
}

func (f *Fielder) AddFields(span trace.Span, count int64, level int) {
	attrs := make([]attribute.KeyValue, 0, 1+len(f.fields))

	if count != 0 {
		attrs = append(attrs, attribute.Int64("count", count))
	}

	processedKeys := make(map[string]struct{}) // To keep track of keys already added

	var numAdditionalRandomFields = f.attributesPerSpan - f.intrinsicAttributes

	//Setting intrinsic attributes here
	for i := 0; i < f.intrinsicAttributes; i++ {
		key := f.keys[i]
		if _, exists := processedKeys[key]; exists { // Should not happen if f.keys has unique elements
			continue
		}

		valFunc, fieldExists := f.fields[key]
		if !fieldExists || valFunc == nil {
			continue
		}

		processedKeyName, ok := f.atLevel(key, level)
		if !ok {
			continue
		}

		// Add to attributes and mark as processed
		switch v := valFunc().(type) {
		case int64:
			attrs = append(attrs, attribute.Int64(processedKeyName, v))
		case uint64:
			attrs = append(attrs, attribute.Int64(processedKeyName, int64(v)))
		case float64:
			attrs = append(attrs, attribute.Float64(processedKeyName, v))
		case string:
			attrs = append(attrs, attribute.String(processedKeyName, v))
		case bool:
			attrs = append(attrs, attribute.Bool(processedKeyName, v))
		default:
			panic(fmt.Sprintf("unknown type %T for %s -- implementation error in fielder.go", v, processedKeyName))
		}
		processedKeys[key] = struct{}{}
	}

	//Setting additional random attributes here.
	if numAdditionalRandomFields > 0 {
		// Create a pool of candidate keys for random selection:
		// These are keys in f.keys that were NOT processed as intrinsic.
		candidateRandomKeys := make([]string, 0, len(f.keys)-f.intrinsicAttributes)
		// A more direct way if intrinsic keys are always from the start:
		if f.intrinsicAttributes < len(f.keys) {
			candidateRandomKeys = f.keys[f.intrinsicAttributes:]
		}

		if len(candidateRandomKeys) > 0 {
			effectiveNumAdditionalRandom := numAdditionalRandomFields
			if effectiveNumAdditionalRandom > len(candidateRandomKeys) {
				effectiveNumAdditionalRandom = len(candidateRandomKeys) // Cap at available unique candidates
			}

			// Randomly select 'effectiveNumAdditionalRandom' keys from 'candidateRandomKeys'
			// Using the same random block selection logic as before
			startRandom := 0
			if len(candidateRandomKeys) > effectiveNumAdditionalRandom {
				startRandom = rand.Intn(len(candidateRandomKeys) - effectiveNumAdditionalRandom + 1)
			}

			for i := 0; i < effectiveNumAdditionalRandom; i++ {
				randomKeyIndex := startRandom + i
				key := candidateRandomKeys[randomKeyIndex]

				// This check is important if f.keys could have duplicates,
				// or if somehow a key from the random pool was already in processedKeys
				// (though with strict partitioning, this shouldn't be the case for the latter).
				if _, exists := processedKeys[key]; exists {
					continue
				}

				valFunc, fieldExists := f.fields[key]
				if !fieldExists || valFunc == nil {
					continue
				}

				processedKeyName, ok := f.atLevel(key, level)
				if !ok {
					continue
				}

				// Add to attributes and mark as processed
				switch v := valFunc().(type) {
				case int64:
					attrs = append(attrs, attribute.Int64(processedKeyName, v))
				case uint64:
					attrs = append(attrs, attribute.Int64(processedKeyName, int64(v)))
				case float64:
					attrs = append(attrs, attribute.Float64(processedKeyName, v))
				case string:
					attrs = append(attrs, attribute.String(processedKeyName, v))
				case bool:
					attrs = append(attrs, attribute.Bool(processedKeyName, v))
				default:
					panic(fmt.Sprintf("unknown type %T for %s -- implementation error in fielder.go", v, processedKeyName))
				}
				processedKeys[key] = struct{}{} // Mark this random key as processed
			}
		}
	}
	span.SetAttributes(attrs...)
}
