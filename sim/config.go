package sim

import (
	_ "embed"
	"encoding/json"
	"math"
)

// Tuning: every number the rules use. The point costs and robot numbers keep the Balance
// Lab's units (points, feet, feet per second) so the two stay comparable; the code converts.
type Tuning struct {
	Pool        float64   `json:"pool"`        // points to spend each round
	PerSqFt     float64   `json:"perSqFt"`     // bonus points per square foot of floor
	GenCost     float64   `json:"genCost"`     // points per generator
	PostCost    float64   `json:"postCost"`    // points per magenta post (one every 8 ft of maze)
	BeamPerFt   float64   `json:"beamPerFt"`   // points per foot of beam, green or magenta
	WaveBonus   float64   `json:"waveBonus"`   // points for each wave survived
	MaxSpanFt   float64   `json:"maxSpanFt"`   // longest gap allowed between generators
	MinSpanFt   float64   `json:"minSpanFt"`   // shortest gap allowed between generators
	MinGens     int       `json:"minGens"`     //
	MaxGens     int       `json:"maxGens"`     //
	Beta        float64   `json:"beta"`        // ring length effect on wall strength
	ChargeMax   float64   `json:"chargeMax"`   // generator charge when full
	RepairFrac  float64   `json:"repairFrac"`  // charge a repaired generator comes back with
	PumpStroke  float64   `json:"pumpStroke"`  // charge one stroke of the pump handle adds (fraction)
	Knockback   float64   `json:"knockback"`   // seconds wardens back off when you top a generator up
	OffRecharge float64   `json:"offRecharge"` // charge per second a switched-off generator recovers
	FirstWave   float64   `json:"firstWave"`   // seconds from the start lever to wave 1
	WaveEvery   float64   `json:"waveEvery"`   //
	WaveSize    float64   `json:"waveSize"`    // wave size by floor area exponent
	WalkersBase float64   `json:"walkersBase"` //
	WalkersPer  float64   `json:"walkersPer"`  // more walkers each wave
	HeaviesPer2 float64   `json:"heaviesPer2"` // heavies per two waves
	DronesBase  float64   `json:"dronesBase"`  //
	DronesPer   float64   `json:"dronesPer"`   // more drones each wave
	FewBelow    int       `json:"fewBelow"`    // small-base help starts below this many generators
	FewDrones   float64   `json:"fewDrones"`   // % fewer drones per missing generator
	BigFromSqFt float64   `json:"bigFromSqFt"` // big-room help starts at this floor area
	BigDrones   float64   `json:"bigDrones"`   // % fewer drones per doubling of floor above that
	WalkerSpeed float64   `json:"walkerSpeed"` // ft/s
	HeavySpeed  float64   `json:"heavySpeed"`  //
	DroneSpeed  float64   `json:"droneSpeed"`  //
	CarrySpeed  float64   `json:"carrySpeed"`  //
	Drain       float64   `json:"drain"`       // charge per second a warden drains from each end of its span
	HeavyDrain  float64   `json:"heavyDrain"`  // heavies drain this many times as fast
	Sting       float64   `json:"sting"`       // damage per second a wall does to a warden leaning on it
	WalkerHP    float64   `json:"walkerHP"`    //
	HeavyHP     float64   `json:"heavyHP"`     //
	KamikazeFt  float64   `json:"kamikazeFt"`  // extra maze walking before walkers burn straight through
	BurnMin     float64   `json:"burnMin"`     // each walker's own limit: kamikaze x (min..max)
	BurnMax     float64   `json:"burnMax"`     //
	WreckFt     float64   `json:"wreckFt"`     // maze a burning walker smashes
	EMPWork     float64   `json:"empWork"`     // seconds of hands-on work on the device (sum of modules)
	Setback     float64   `json:"setback"`     // seconds of work a failed device game costs
	TugWalker   float64   `json:"tugWalker"`   // chance a yank tears the device off a walker
	TugHeavy    float64   `json:"tugHeavy"`    // ...off a heavy
	TugCost     float64   `json:"tugCost"`     // seconds of work a successful yank knocks loose
	FryPerSec   float64   `json:"fryPerSec"`   // chance per second a carrier brushing magenta fries
	Regrab      float64   `json:"regrab"`      // seconds before a dropped device is picked up again
	LeechBite   float64   `json:"leechBite"`   // seconds a clamped leech takes to shut its generator down
	FieldSecs   float64   `json:"fieldSecs"`   // the device's backup field: seconds it holds wardens off once exposed
	FieldCharge float64   `json:"fieldCharge"` // seconds of field regained per second it's safe in your ring
	FieldFew    float64   `json:"fieldFew"`    // extra field seconds per generator below fewBelow (small bases lose the device easily)
	MaxRobots   int       `json:"maxRobots"`   // walkers and heavies in the hall at once (the rest wait behind the doors)
	Pressure    float64   `json:"pressure"`    // difficulty: multiplies how many robots every wave sends
	HarpoonAt   []float64 `json:"harpoonAt"`   // device progress fractions that arm the harpoon
	SuperSecs   float64   `json:"superSecs"`   // how long a supercharged wall stays hot
	SuperCool   float64   `json:"superCool"`   // cooldown between supercharges
	ReelLoad    float64   `json:"reelLoad"`    // chance a fresh spear comes as a pair of reel-only spears (yellow)
	Misfire     float64   `json:"misfire"`     // chance the first pull of the cord misfires (the guts need fixing)
	Misfire2    float64   `json:"misfire2"`    // chance it misfires again once that's fixed
	MisfireSecs float64   `json:"misfireSecs"` // the simulated player's time to fix a misfire (s)
	MisfireDaze float64   `json:"misfireDaze"` // a misfire's weak pulse leaves wardens near the cart reeling this long (s)
	HallMargin  float64   `json:"hallMargin"`  // metres of open floor between your room and the hall walls
	WallHeight  float64   `json:"wallHeight"`  // magenta wall height (m)
	MinWall     float64   `json:"minWall"`     // shortest magenta wall (m)
	RoomGap     float64   `json:"roomGap"`     // magenta must stay this far outside your room (m)
}

// Content: the libraries the rules draw from at random, and the device's modules in order.
type Content struct {
	RepairTasks []Task      `json:"repairTasks"`
	RepairSteps []int       `json:"repairSteps"` // a work order has one of these many tasks
	EMPModules  []Task      `json:"empModules"`
	Games       []string    `json:"games"` // minigames a "game" module can be
	Drones      []DroneType `json:"drones"`
	DoorNames   []string    `json:"doors"`
}

type Task struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Secs float64 `json:"secs"` // how long it takes a practised player (the bot uses this)
}

type DroneType struct {
	ID     string  `json:"id"`
	From   int     `json:"from"` // first wave it can appear in (0 = wave 1)
	Weight float64 `json:"weight"`
}

//go:embed data/tuning.json
var tuningJSON []byte

//go:embed data/content.json
var contentJSON []byte

// DefaultTuning: the fairest setup from the Balance Lab.
func DefaultTuning() *Tuning {
	t := &Tuning{}
	if err := json.Unmarshal(tuningJSON, t); err != nil {
		panic("tuning.json: " + err.Error())
	}
	return t
}

// TuningWith: the defaults with a JSON override on top (any subset of the keys in
// data/tuning.json). A bad file is reported and ignored.
func TuningWith(override []byte) (*Tuning, error) {
	t := DefaultTuning()
	if len(override) == 0 {
		return t, nil
	}
	o := DefaultTuning()
	if err := json.Unmarshal(override, o); err != nil {
		return t, err
	}
	return o, nil
}

func DefaultContent() *Content {
	c := &Content{}
	if err := json.Unmarshal(contentJSON, c); err != nil {
		panic("content.json: " + err.Error())
	}
	return c
}

const (
	FtPerM    = 3.28084
	SqFtPerM2 = 10.7639
)

// Budget: points for a room of this floor area, before anything is bought.
func (t *Tuning) Budget(areaM2 float64) float64 { return t.Pool + t.PerSqFt*areaM2*SqFtPerM2 }

// GreenCost: what n generators plus the ring of beams around a perimeter cost.
func (t *Tuning) GreenCost(n int, perimM float64) float64 {
	return t.GenCost*float64(n) + t.BeamPerFt*perimM*FtPerM
}

// MazeCost: points per metre of magenta (beam plus a post every 8 ft).
func (t *Tuning) MazeCost() float64 { return t.BeamPerFt*FtPerM + t.PostCost*FtPerM/8 }

func (t *Tuning) maxSpan() float64 { return t.MaxSpanFt / FtPerM }
func (t *Tuning) minSpan() float64 { return t.MinSpanFt / FtPerM }

// strength: how hard the ring hits back, from the total length of beam it holds up.
func (t *Tuning) strength(ringLen float64) float64 {
	return math.Pow(12.192/math.Max(3.048, ringLen), t.Beta)
}

// waveScale: the wave size factor for a room of this floor area.
func (t *Tuning) waveScale(areaM2 float64) float64 {
	k := t.Pressure
	if k <= 0 {
		k = 1
	}
	return k * math.Pow(areaM2*SqFtPerM2/100, t.WaveSize)
}

// FieldMax: how long an exposed device's backup field holds with n generators.
func (t *Tuning) FieldMax(gens int) float64 {
	f := t.FieldSecs
	if miss := t.FewBelow - gens; miss > 0 {
		f += t.FieldFew * float64(miss)
	}
	return f
}

// droneScale: the small-base and big-room drone discounts.
func (t *Tuning) droneScale(gens int, areaM2 float64) float64 {
	k := 1.0
	if miss := t.FewBelow - gens; miss > 0 {
		k *= math.Max(0, 1-t.FewDrones/100*float64(miss))
	}
	if sq := areaM2 * SqFtPerM2; sq > t.BigFromSqFt && t.BigFromSqFt > 0 {
		k *= math.Max(0, 1-t.BigDrones/100*math.Log2(sq/t.BigFromSqFt))
	}
	return k
}
