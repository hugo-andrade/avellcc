package keyboard

import (
	"context"
	"math"
	"sync"
	"time"
)

// EffectFunc is a per-frame effect function. frame counts from 0.
type EffectFunc func(ctrl *ITE8295, frame int, opts EffectOpts)

// EffectOpts holds parameters for software effects.
type EffectOpts struct {
	Speed   int
	R, G, B byte
}

// DefaultEffectOpts returns sensible defaults.
func DefaultEffectOpts() EffectOpts {
	return EffectOpts{Speed: 3, R: 255, G: 255, B: 255}
}

// EffectRunner manages a software LED effect running in a goroutine.
type EffectRunner struct {
	ctrl   *ITE8295
	fps    int
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewEffectRunner creates a new runner for the given controller.
func NewEffectRunner(ctrl *ITE8295, fps int) *EffectRunner {
	if fps <= 0 {
		fps = 30
	}
	return &EffectRunner{ctrl: ctrl, fps: fps}
}

// Start begins running the effect in a goroutine.
func (r *EffectRunner) Start(fn EffectFunc, opts EffectOpts) {
	r.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.runLoop(ctx, fn, opts)
	}()
}

// Stop stops the running effect and waits for cleanup.
func (r *EffectRunner) Stop() {
	if r.cancel != nil {
		r.cancel()
		r.wg.Wait()
		r.cancel = nil
	}
}

func (r *EffectRunner) runLoop(ctx context.Context, fn EffectFunc, opts EffectOpts) {
	interval := time.Duration(float64(time.Second) / float64(r.fps))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn(r.ctrl, frame, opts)
			frame++
		}
	}
}

// hsvToRGB converts HSV (h in [0,1), s, v in [0,1]) to RGB bytes.
func hsvToRGB(h, s, v float64) (byte, byte, byte) {
	h -= math.Floor(h) // wrap to [0, 1)
	i := int(h * 6)
	f := h*6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)

	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	case 5:
		r, g, b = v, p, q
	}
	return byte(r * 255), byte(g * 255), byte(b * 255)
}

// RainbowWave is a rainbow wave effect — hue shifts across columns.
func RainbowWave(ctrl *ITE8295, frame int, opts EffectOpts) {
	for row := 0; row < GridRows; row++ {
		for col := 0; col < GridCols; col++ {
			hue := float64(col)/float64(GridCols) + float64(frame)*float64(opts.Speed)*0.005
			r, g, b := hsvToRGB(hue, 1.0, 1.0)
			_ = ctrl.SetKeyColor(row, col, r, g, b)
		}
	}
}

// Breathing is a pulsing brightness effect.
func Breathing(ctrl *ITE8295, frame int, opts EffectOpts) {
	t := float64(frame) * float64(opts.Speed) * 0.02
	factor := (math.Sin(t) + 1.0) / 2.0
	cr := byte(float64(opts.R) * factor)
	cg := byte(float64(opts.G) * factor)
	cb := byte(float64(opts.B) * factor)
	_ = ctrl.SetAllKeys(cr, cg, cb)
}

// ColorWave is a brightness wave that moves across columns.
func ColorWave(ctrl *ITE8295, frame int, opts EffectOpts) {
	for row := 0; row < GridRows; row++ {
		for col := 0; col < GridCols; col++ {
			t := float64(frame) * float64(opts.Speed) * 0.03
			factor := (math.Sin(t-float64(col)*0.5) + 1.0) / 2.0
			r := byte(float64(opts.R) * factor)
			g := byte(float64(opts.G) * factor)
			b := byte(float64(opts.B) * factor)
			_ = ctrl.SetKeyColor(row, col, r, g, b)
		}
	}
}

// SoftwareEffects maps effect names to their functions.
var SoftwareEffects = map[string]EffectFunc{
	"sw_rainbow":   RainbowWave,
	"sw_breathing": Breathing,
	"sw_wave":      ColorWave,
}
