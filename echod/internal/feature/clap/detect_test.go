package clap

import (
	"math"
	"math/rand/v2"
	"testing"
)

// room is a block of a quiet room, and loud one of a clap or a voice at amplitude a.
func room(r *rand.Rand) []int16 {
	b := make([]int16, block)
	for i := range b {
		b[i] = int16(r.IntN(121) - 60)
	}
	return b
}

// thud is a block of a low sound at amplitude a: a knock, a footstep, a cup set down.
func thud(n, a int) []int16 {
	b := make([]int16, block)
	for i := range b {
		b[i] = int16(float64(a) * math.Sin(2*math.Pi*150*float64(n*block+i)/16000))
	}
	return b
}

func loud(r *rand.Rand, a int) []int16 {
	b := make([]int16, block)
	for i := range b {
		b[i] = int16(r.IntN(2*a+1) - a)
	}
	return b
}

// run plays a script to a detector after a second of the room: "." a block of room, "x" a block of clap
// loud, "q" a much quieter one, "k" a knock, "r" a block of a voice starting, and answers how many
// double claps it heard.
func run(script string) int {
	r := rand.New(rand.NewPCG(7, 8))
	d := newDetector()
	for range 100 {
		d.add(room(r))
	}
	n := 0
	for i, c := range script {
		b := room(r)
		switch c {
		case 'x':
			b = loud(r, 6000)
		case 'q':
			b = loud(r, 300)
		case 'k':
			b = thud(i, 8000)
		case 'r':
			b = loud(r, 250)
		}
		if d.add(b) {
			n++
		}
	}
	return n
}

func repeat(s string, n int) string {
	out := ""
	for range n {
		out += s
	}
	return out
}

func TestADoubleClapIsHeard(t *testing.T) {
	gap := repeat(".", 30)
	if n := run("xx" + gap + "xx" + repeat(".", 60)); n != 1 {
		t.Errorf("a double clap heard %d times", n)
	}
}

func TestWhatIsNotADoubleClap(t *testing.T) {
	for name, script := range map[string]string{
		"one clap":          "xx" + repeat(".", 120),
		"three claps":       "xx" + repeat(".", 30) + "xx" + repeat(".", 20) + "xx" + repeat(".", 80),
		"too close":         "xx" + repeat(".", 5) + "xx" + repeat(".", 80),
		"too far apart":     "xx" + repeat(".", 100) + "xx" + repeat(".", 80),
		"somebody talking":  repeat("x", 50) + repeat(".", 30) + repeat("x", 50) + repeat(".", 80),
		"applause":          repeat("xx"+repeat(".", 12), 12) + repeat(".", 80),
		"a knock, twice":    "kk" + repeat(".", 30) + "kk" + repeat(".", 80),
		"a loud and a soft": "xx" + repeat(".", 30) + "qq" + repeat(".", 80),
		"no sharp start":    "rrrxx" + repeat(".", 30) + "rrrxx" + repeat(".", 80),
		"out of speech":     repeat("r", 40) + repeat(".", 20) + "xx" + repeat(".", 30) + "xx" + repeat(".", 80),
	} {
		if n := run(script); n != 0 {
			t.Errorf("%s heard as %d double claps", name, n)
		}
	}
}

// After a reset, what came before is not half of a double clap.
func TestResetForgetsTheFirstClap(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 1))
	d := newDetector()
	for range 100 {
		d.add(room(r))
	}
	for _, c := range "xx" + repeat(".", 30) {
		b := room(r)
		if c == 'x' {
			b = loud(r, 6000)
		}
		d.add(b)
	}
	d.reset()
	heard := false
	for _, c := range "xx" + repeat(".", 60) {
		b := room(r)
		if c == 'x' {
			b = loud(r, 6000)
		}
		heard = heard || d.add(b)
	}
	if heard {
		t.Error("a clap before a reset made a double clap with one after")
	}
}

// heardIn plays blocks to a detector after a second of the room, and answers how many double claps it
// heard.
func heardIn(r *rand.Rand, blocks [][]int16) int {
	d := newDetector()
	for range 100 {
		d.add(room(r))
	}
	n := 0
	for _, b := range blocks {
		if d.add(b) {
			n++
		}
	}
	return n
}

// script turns a script of "x" (a sound) and "." (the room) into blocks, sound(n) making the nth block.
func script(r *rand.Rand, s string, sound func(n int) []int16) [][]int16 {
	var out [][]int16
	for n, c := range s {
		if c == 'x' {
			out = append(out, sound(n))
		} else {
			out = append(out, room(r))
		}
	}
	return out
}

// ringing is a clap in a room that rings: n blocks falling 1 dB each, 15 dB down after 150 ms as in a
// living room, the first starting offset dB under the peak, as when a clap begins near a block's end.
func ringing(r *rand.Rand, peak, n int, offset float64) [][]int16 {
	var out [][]int16
	if offset > 0 {
		out = append(out, loud(r, int(float64(peak)*math.Pow(10, -offset/20))))
	}
	for i := range n {
		out = append(out, loud(r, int(float64(peak)*math.Pow(10, -float64(i)/20))))
	}
	return out
}

// Two claps in a room that rings: each takes longer than 120 ms to fall away, the second starts out of
// the first one's echo, and they are not as loud as each other.
func TestADoubleClapInARingingRoomIsHeard(t *testing.T) {
	for name, second := range map[string]struct {
		peak   int
		offset float64
	}{
		"as loud":             {6000, 0},
		"twice as loud":       {12000, 0},
		"starting across two": {6000, 18},
		"softer, across two":  {2000, 15},
	} {
		r := rand.New(rand.NewPCG(5, 6))
		blocks := append(ringing(r, 6000, 22, 0), ringing(r, second.peak, 22, second.offset)...)
		blocks = append(blocks, script(r, repeat(".", 60), nil)...)
		if n := heardIn(r, blocks); n != 1 {
			t.Errorf("%s: heard %d double claps", name, n)
		}
	}
}

// Two quick claps: the second comes 120 ms after the first, while the first is still 12 dB over its fall.
func TestAQuickDoubleClapIsHeard(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 10))
	blocks := append(ringing(r, 6000, 12, 0), ringing(r, 12000, 22, 0)...)
	blocks = append(blocks, script(r, repeat(".", 60), nil)...)
	if n := heardIn(r, blocks); n != 1 {
		t.Errorf("heard %d double claps", n)
	}
}

// palm is a block of a clap with the palms: noise smoothed over a few samples, low (brightness about
// 0.45, as measured).
func palm(r *rand.Rand, a int) []int16 {
	b := make([]int16, block)
	var y float64
	for i := range b {
		y += (float64(r.IntN(2*a+1)-a) - y) * 0.1
		b[i] = int16(y * 4)
	}
	return b
}

// buzz is the nth block of a voice: a pulse at 150 Hz, sharp and bright, but the same pulse over and over.
func buzz(n, a int) []int16 {
	b := make([]int16, block)
	for i := range b {
		if (n*block+i)%107 < 3 {
			b[i] = int16(a)
		}
	}
	return b
}

// A clap with the palms is low but noisy, and is heard; a voice as sharp and as short is not.
func TestPalmsAreHeardAndVoicesAreNot(t *testing.T) {
	pair := "xxxx" + repeat(".", 25) + "xxxx" + repeat(".", 80)
	r := rand.New(rand.NewPCG(11, 12))
	if n := heardIn(r, script(r, pair, func(int) []int16 { return palm(r, 6000) })); n != 1 {
		t.Errorf("palms: heard %d double claps", n)
	}
	r = rand.New(rand.NewPCG(11, 12))
	if n := heardIn(r, script(r, pair, func(n int) []int16 { return buzz(n, 12000) })); n != 0 {
		t.Errorf("a voice: heard %d double claps", n)
	}
}

// A car passing in the street: a low rumble 12 dB over the room for seconds, under a double clap.
func TestADoubleClapOverACarIsHeard(t *testing.T) {
	r := rand.New(rand.NewPCG(13, 14))
	blocks := script(r, repeat(".", 100)+"xx"+repeat(".", 30)+"xx"+repeat(".", 80), func(int) []int16 {
		return loud(r, 6000)
	})
	for n, b := range blocks {
		for i := range b {
			b[i] += int16(200 * math.Sin(2*math.Pi*60*float64(n*block+i)/16000))
		}
	}
	if n := heardIn(r, blocks); n != 1 {
		t.Errorf("heard %d double claps", n)
	}
}
