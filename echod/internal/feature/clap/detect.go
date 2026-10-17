package clap

import "math"

// The detector: a pause, two claps a beat apart, and then quiet.
//
// A clap is a sharp, short, bright, unvoiced sound. Sharp: it starts in one block, or across two, from
// about the room's own level to far over it, where speech, music and a door closing rise over several.
// Short: it is down again, well under its own peak or back near the room, within a quarter of a second,
// echo included. Bright: its energy reaches high up, as a slap of two hands does, where a knock on a door,
// a footstep, a dog's bark or a cup set down are mostly low; the samples' first difference measures that,
// being large for high frequencies and small for low ones. Unvoiced: it does not repeat at a voice's
// pitch, as the vowel after a consonant does.
//
// Two of them, a beat apart, as alike in loudness and length as two claps by one person are, and nothing
// after. The first comes out of a pause, which a consonant in the middle of speech does not. The second
// may come out of the first one's echo.
//
// The numbers were measured on an Echo Dot in a living room (2026-10-17): its echo keeps a clap up for 100
// to 180 ms, the second clap of a pair is often 10 to 20 dB softer or louder than the first, a light clap
// is 14 to 16 dB over the room, a clap with the palms is low, and a car passing outside lifts the room,
// mostly in the lows, for seconds.
//
// The room's level is followed the way the leveler follows it (hardware/mic/level.go): down quickly, up
// slowly, so a clap does not raise it and a room that gets louder for a while is heard as louder. It is
// followed twice, over the whole band and in the highs alone, so a car's rumble is still the room.

const (
	// block is how much audio one look takes, in samples at 16 kHz: 10 ms.
	block = 160

	// clapOver is how far over the room a block has to be to start a first clap, in dB, and onsetFrom how
	// far over the room one of the two blocks before it may be at most: the start can fall across two
	// blocks.
	clapOver  = 15.0
	onsetFrom = 6.0

	// A second clap may start out of the first one's echo, which is still 10 to 15 dB over the room a
	// quarter of a second later: secondOver over the room and secondJump over the quieter of the two blocks
	// before it.
	secondOver = 12.0
	secondJump = 6.0

	// quietBefore is how long the room has to have been quiet before a first clap, in blocks, and calmUnder
	// how far over its level, in dB, over the whole band or in the highs, still counts as quiet.
	quietBefore = 50 // blocks: 500 ms
	calmUnder   = 8.0

	// clapDrop is how far under its own peak a clap has to fall within clapLongest blocks for it to have
	// been short, or clapBack how near the room: a soft clap cannot fall 15 dB and stay over the room's own
	// noise.
	clapDrop    = 15.0
	clapBack    = 5.0
	clapLongest = 25 // blocks: 250 ms

	// cutAfter is how long a clap has to have lasted, in blocks, before a new one can cut it short, and
	// cutJump how sharply the new one has to rise over the two blocks before it, in dB: two quick claps
	// come while the room still rings with the first.
	cutAfter = 10
	cutJump  = 8.0

	// bright is the least ratio of the first difference's RMS to the block's RMS for a clap to start: about
	// 1.4 for white noise, 0.77 for 2 kHz, 0.2 for 400 Hz, and as low as 0.36 for a clap with the palms.
	// brightAll is the least over the whole clap, its echo included: a clap's echo is about as bright as
	// the clap, where a consonant is followed by a vowel, which is mostly low.
	bright    = 0.35
	brightAll = 0.4

	// voicedMost is the most a clap may be voiced, on average over its blocks (see voicing): a vowel
	// repeats at its pitch, a clap's echo is noise and does not. pitchLow and pitchHigh are the shortest
	// and longest periods looked for, in samples: 400 Hz down to 100 Hz, the longest being a block, as far
	// back as the block before reaches.
	voicedMost = 0.5
	pitchLow   = 40
	pitchHigh  = block

	// alike is how far apart the two claps' peaks may be, in dB, and their lengths, in blocks.
	alikeDB     = 25.0
	alikeBlocks = 6

	// twoApart is the gap between the ends of the two claps, in blocks: 150 to 700 ms, a deliberate double
	// clap and not one clap's echo or a round of applause.
	twoApartMin = 15
	twoApartMax = 70

	// quietAfter is how long after the second clap nothing else clap-like may come, in blocks: 400 ms,
	// so applause and drumming are not taken for two claps.
	quietAfter = 40

	// floorDown and floorUp are how much of the way to a block's level the room's level moves, and
	// floorHold how long after a clap it is left alone, in blocks: the clap's echo is not the room.
	floorDown = 0.3
	floorUp   = 0.002
	floorHold = 40

	// leastFloor is as quiet as the room's level is taken to be, as a block's RMS: below it, the numbers
	// are the converter's own noise.
	leastFloor = 4.0
)

// clap is one heard: how loud at its peak, over the room, and how many blocks long.
type clap struct {
	peak   float64
	blocks int
}

// detector looks at the microphone a block at a time.
type detector struct {
	floor     float64 // the room's level, as RMS
	highFloor float64 // and in the highs, as the first difference's RMS
	blocks    int
	hold      int     // blocks left before the room's level follows the blocks again
	last      []int16 // the block before, for voicing

	// The two blocks before: how far over the room, over the whole band and in the highs, in dB, and how
	// many quiet blocks in a row there were up to each.
	prevOver, prev2Over float64
	prevHigh, prev2High float64
	calm, calm2         int

	// The clap being heard: blocks since it started (-1 for none), its peak, and its brightness and voicing
	// summed over its blocks.
	inClap int
	peak   float64
	bright float64
	voiced float64

	// The claps so far: the first, blocks since it ended, how many, and blocks of quiet since the last.
	first      clap
	sinceFirst int
	claps      int
	quiet      int
}

func newDetector() *detector { return &detector{floor: leastFloor, inClap: -1} }

// measure is a block's RMS and how bright it is.
// The block's mean is taken out first: the microphones carry a small DC offset, which would otherwise
// count as loudness.
func measure(b []int16) (rms, brightness float64) {
	var mean float64
	for _, v := range b {
		mean += float64(v)
	}
	mean /= float64(len(b))
	var sum, diff float64
	prev := float64(b[0])
	for _, v := range b {
		x := float64(v)
		sum += (x - mean) * (x - mean)
		d := x - prev
		diff += d * d
		prev = x
	}
	n := float64(len(b))
	rms = math.Sqrt(sum / n)
	if rms > 0 {
		brightness = math.Sqrt(diff/n) / rms
	}
	return rms, brightness
}

// add takes one block of samples and answers whether it ends a double clap.
func (d *detector) add(b []int16) bool {
	if len(b) == 0 {
		return false
	}
	defer func() { d.last = append(d.last[:0], b...) }()

	rms, brightness := measure(b)
	high := rms * brightness
	if d.blocks == 0 {
		// The room as first heard, rather than learned up to from nothing.
		d.floor, d.highFloor = math.Max(rms, leastFloor), math.Max(high, leastFloor)
	}
	d.blocks++
	over := 20 * math.Log10(math.Max(rms, 1)/d.floor)
	highOver := 20 * math.Log10(math.Max(high, 1)/d.highFloor)

	prevOver, prev2Over, prevHigh, prev2High := d.prevOver, d.prev2Over, d.prevHigh, d.prev2High
	calm, calm2 := d.calm, d.calm2
	d.prevOver, d.prev2Over, d.prevHigh, d.prev2High = over, prevOver, highOver, prevHigh
	d.calm2 = calm
	if over < calmUnder || highOver < calmUnder {
		d.calm++
	} else {
		d.calm = 0
	}

	var ended *clap
	switch {
	case d.inClap < 0:
		// A first clap starts from the room, after a pause; the block before may be the clap's own start,
		// and then the pause ended a block earlier. A third needs no pause: it has to be heard, to undo the
		// double. A second may instead start out of the first one's echo.
		quiet1 := math.Min(prevOver, prevHigh) < onsetFrom
		quiet2 := math.Min(prev2Over, prev2High) < onsetFrom
		paused := d.claps > 0 || quiet1 && calm >= quietBefore || quiet2 && calm2 >= quietBefore
		fromRoom := over >= clapOver && (quiet1 || quiet2) && paused
		fromEcho := d.claps == 1 && over >= secondOver && over-math.Min(prevOver, prev2Over) >= secondJump
		if (fromRoom || fromEcho) && brightness >= bright {
			d.start(over, brightness, b)
		}
	default:
		d.inClap++
		d.peak = math.Max(d.peak, over)
		d.bright += brightness
		d.voiced += voicing(d.last, b)
		cut := d.inClap > cutAfter && over-math.Min(prevOver, prev2Over) >= cutJump && brightness >= bright
		switch {
		case cut || over <= math.Max(d.peak-clapDrop, clapBack):
			if n := float64(d.inClap + 1); d.bright/n >= brightAll && d.voiced/n <= voicedMost {
				ended = &clap{peak: d.peak, blocks: d.inClap}
			} else {
				d.claps = 0 // short, but dull or voiced: a word, and nothing before it pairs with what follows
			}
			d.inClap, d.hold = -1, floorHold
			if cut {
				d.start(over, brightness, b)
			}
		case d.inClap > clapLongest:
			// Loud for too long: not a clap. Whatever was counted is not a double clap either.
			d.inClap, d.claps = -1, 0
		}
	}

	// The room's level, from blocks that are not a clap or its echo.
	if d.hold > 0 {
		d.hold--
	} else if d.inClap < 0 && ended == nil {
		d.floor = follow(d.floor, rms)
		d.highFloor = follow(d.highFloor, high)
	}

	if d.claps > 0 {
		d.sinceFirst++
	}
	if ended != nil {
		switch {
		case d.claps == 1 && d.sinceFirst >= twoApartMin && d.sinceFirst <= twoApartMax && alike(d.first, *ended):
			d.claps, d.quiet = 2, 0
		default:
			// The first, a third, one too late, too soon or unlike the first: start again from this one.
			d.claps, d.sinceFirst, d.first = 1, 0, *ended
		}
		return false
	}
	if d.claps == 1 && d.sinceFirst > twoApartMax {
		d.claps = 0
	}
	if d.claps == 2 {
		if d.inClap >= 0 {
			d.claps = 0 // something clap-like right after: applause, not a double clap
			return false
		}
		d.quiet++
		if d.quiet >= quietAfter {
			d.claps = 0
			return true
		}
	}
	return false
}

// start begins hearing a clap at block b.
func (d *detector) start(over, brightness float64, b []int16) {
	d.inClap, d.peak, d.bright, d.voiced = 0, over, brightness, voicing(d.last, b)
}

// alike is whether two claps are as alike as two by one person are.
func alike(a, b clap) bool {
	return math.Abs(a.peak-b.peak) <= alikeDB && abs(a.blocks-b.blocks) <= alikeBlocks
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// reset forgets everything but the room's level, for when the device made a sound or was listening.
func (d *detector) reset() { d.inClap, d.claps, d.sinceFirst, d.quiet = -1, 0, 0, 0 }

// follow moves a room's level toward a block's: down quickly, up slowly.
func follow(floor, level float64) float64 {
	step := floorUp
	if level < floor {
		step = floorDown
	}
	return math.Max(floor+(level-floor)*step, leastFloor)
}

// voicing is how much block b repeats at a voice's pitch: the highest normalized correlation of b with
// itself pitchLow to pitchHigh samples earlier, reaching back into the block before. 1 for a steady vowel,
// near 0 for noise; 0 when there is no block before to reach into.
func voicing(before, b []int16) float64 {
	at := func(i int) float64 { // i counts from the start of before
		if i < len(before) {
			return float64(before[i])
		}
		return float64(b[i-len(before)])
	}
	start := len(before)
	best := 0.0
	for lag := pitchLow; lag <= min(pitchHigh, start); lag++ {
		var xy, xx, yy float64
		for i := start; i < start+len(b); i++ {
			x, y := at(i), at(i-lag)
			xy += x * y
			xx += x * x
			yy += y * y
		}
		if xx > 0 && yy > 0 {
			best = math.Max(best, xy/math.Sqrt(xx*yy))
		}
	}
	return best
}
