# Contraband — balance settings (Balance Lab v15, 2026-10-22; game build 2, 2026-10-23)

## In the real game (build 2, 2026-10-23)

Dan's first playtest: the wardens never did anything once they got through. Two causes: the drain was too weak for any generator to blow, and with no ring left the wardens queued at the door mouths and blocked the one carrying the device. Both are fixed: if nobody defends, the wardens now take the device every round, 2½–4 minutes in (`TestPassivePlayerLoses`).

| Rule | Build 1 | Build 2 | What it does |
|---|---|---|---|
| drain | 0.5 | 1.0 | How fast wardens leaning on a span drain its two generators |
| sting | 0.35 | 0.17 | How fast the ring hurts a warden leaning on it (they stay longer) |
| pressure | 0.55 | 0.42 | Robots per wave = NORMAL |
| difficulty | ×0.7 / ×1 / ×1.15 | ×0.75 / ×1 / ×1.1 (build 12: ×0.55 / ×1 / ×1.1) | EASY / NORMAL / HARD multiplier on pressure (console button) |

Unchanged from build 1: fieldSecs 5, fieldFew 2, fieldCharge 0.25, leechBite 4, maxRobots 18, waveSize -0.5. The simulated player (0.9 s to react, hands-on jobs 1.3x their book time, walks 0.9 m/s) now also supercharges a span when two or more wardens lean on it.

**Dan's real room** (183 sq ft, 8–10 generators), 16 rounds per cell. Build 10: robots have power. A supercharge's blast fries within 0.25 m of a robot's body (SuperKill) and does up to 9 power/s out to 1.6 m (SuperReach, SuperDmg; ramping up the longer a robot lingers); draining the ring costs a robot 0.12 power per unit of charge (DrainCost). That made rounds far easier, so pressure went 0.42 → 0.54 (robots per wave):

| Difficulty | 8 gens | 9 gens | 10 gens |
|---|---|---|---|
| EASY | 100 | 100 | 100 |
| NORMAL | 50 | 50 | 87 |
| HARD | 31 | 37 | 31 |

Build 14 (supercharge cooldown 15 → 20 s, seven device games, the prod in a bracket on a generator): EASY 75/93/100, NORMAL 56/37/50, HARD 6/62/25 - about the same as build 12 within noise (16 rounds a cell), a little harder on EASY. Build 12 (misfires: 50% of rounds, then 20% again; the simulated player takes 40 s to fix one and now finishes the last stage before topping up generators; a misfire's pulse dazes wardens within 6 m for 15 s; EASY multiplier x0.75 → x0.55): EASY 87/87/93, NORMAL 68/37/56, HARD 18/37/31. Without misfires the same player would be EASY 100s, NORMAL 87/75/75, HARD 50/62/75 - a misfire lands at the most crowded point of the round, so it costs a lot. Build 11 (reel-only spears on 30% of loads (reelLoad 0.3), power leads to replug after a theft, stages shuffled): EASY 100/100/100, NORMAL 50/68/50, HARD 31/50/25 - the same within noise. Pressure is touchy here: at 0.5 NORMAL was 62–93% and HARD 50–81%; at 0.6 NORMAL fell to 18–31%. Build 9: EASY 100/100/100, NORMAL 75/62/81, HARD 62/50/62 (pressure 0.42). Build 8: NORMAL 62/31/68, HARD 12/18/37.

On NORMAL in Dan's room a round has 2–4 generators blown and the device grabbed about once; wins take about 5 minutes, losses come at about 8½.

All lab rooms at NORMAL (12 rounds per cell, so ±14%):

| Room | 4 | 5 | 6 | 7 | 8 | 9 | 10 |
|---|---|---|---|---|---|---|---|
| 7x7 | 33 | 50 | 91 | 91 | 100 | – | – |
| 8x10 | 83 | 66 | 41 | 58 | 33 | 50 | 33 |
| 10x10 | 75 | 41 | 50 | 50 | 0 | 16 | 41 |
| 12x10 | 50 | 41 | 66 | 75 | 25 | 41 | 25 |
| 14x12 | – | – | 91 | 91 | 75 | 41 | 66 |
| 16x16 | – | – | – | – | 91 | 66 | 91 |

Known issue: in small and mid rooms (100–120 sq ft) 8 or more generators is now much harder than 4–7 (6–10 generators blown a round). The small-base help (more field time, fewer drones) stops at 8 generators, and with the doubled drain that cliff shows. Dan's room only allows 8–10, so it doesn't affect him yet; it needs fixing before other rooms matter.

Real tuning should come from playtests: any number can be overridden with `contraband-tuning.json` beside the exe (copy `contraband-tuning.sample.json`).

## Build 1 (first playable, 2026-10-23)

The lab's 50/50 grid did not survive the full rules: small bases lost almost every round (one leech or blown corner uncovered the device) and big rooms were close to impossible (every job is a long walk). Added then: the device's backup field (fieldSecs 5, +2 s per generator below 8, 0.25 s regained per safe second), a 4 s leech bite, 18 robots in the hall at most, and smaller waves for bigger rooms (waveSize -0.5). Dan's room was EASY 87–100, NORMAL 43–56, HARD 6–31.

## Fairest setup in the lab (what the game started from)

| Control | Value |
|---|---|
| Generator | 10 pts |
| Magenta post | 10 pts |
| Beam per foot | 2 pts |
| Base points per round | 400 |
| Bonus per sq ft of floor | 0 |
| Bonus per wave survived | 50 |
| Longest span between posts | 12 ft |
| Each generator past 4 costs more | 0% |
| Green strength spreads over | Ring length |
| Ring length effect | 0.10 |
| Wave size by floor area | -0.20 (game now -0.5) |
| Scale generators to the ring | off (0) |
| Small-base help starts below | 8 generators |
| Fewer drones per missing generator | 11% |
| Tougher generators per missing generator | 0% |
| Big-room help starts at | 200 sq ft |
| Fewer drones per doubling of floor | 20% |
| Maze length before walkers burn through | 60 ft |
| Hands-on EMP work | 150 s |
| Warden drain rate | 0.5 |
| Seconds between waves | 40 |

Dan's own setup ("Load Dan's setup") is the same except scale 0.05 and no big-room rule.

## Lab win rates, fairest setup (800 fresh rounds per cell, lab model)

| Room | 4 | 5 | 6 | 7 | 8 | 9 | 10 |
|---|---|---|---|---|---|---|---|
| 7x7 | 39 | 52 | 51 | 51 | 48 | 51 | – |
| 8x10 | 51 | 56 | 51 | 52 | 50 | 52 | 56 |
| 10x10 | 52 | 57 | 51 | 52 | 51 | 48 | 54 |
| 12x10 | 51 | 58 | 53 | 53 | 49 | 51 | 55 |
| 14x12 | – | 50 | 51 | 47 | 45 | 47 | 50 |
| 16x16 | – | – | 53 | 53 | 48 | 53 | 55 |

35 of 38 cells within 50 ± 6 in the lab's model, no trend by room size or generator count.

## Rules the lab numbers depend on
- Wave sizes use random rounding (a fractional robot is a chance of one more).
- Small-base help: bases under 8 generators face 11% fewer drones per missing generator.
- Big-room help: rooms over 200 sq ft face 20% fewer drones per doubling of floor area.
- Tested and rejected in the lab: drones scaled by floor area both ways, and a help threshold that grows with room size — both overcorrected.
