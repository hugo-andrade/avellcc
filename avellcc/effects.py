"""Software-driven LED effects for the ITE IT8295 keyboard.

These effects run in a Python loop, updating per-key colors each frame.
For hardware-accelerated effects, use ITE8295.set_effect() instead.
"""

import colorsys
import math
import time
import threading

from .ite8295 import ITE8295, GRID_ROWS, GRID_COLS


def _hsv_to_rgb(h: float, s: float = 1.0, v: float = 1.0) -> tuple[int, int, int]:
    r, g, b = colorsys.hsv_to_rgb(h % 1.0, s, v)
    return int(r * 255), int(g * 255), int(b * 255)


class EffectRunner:
    """Runs software-driven LED effects in a background thread."""

    def __init__(self, controller: ITE8295, fps: int = 30):
        self._ctrl = controller
        self._fps = fps
        self._running = False
        self._thread: threading.Thread | None = None

    def start(self, effect_func, **kwargs):
        """Start an effect function in a background thread.

        effect_func(controller, frame, **kwargs) is called each frame.
        It should set key colors using controller.set_key_color().
        """
        self.stop()
        self._running = True
        self._thread = threading.Thread(
            target=self._run_loop,
            args=(effect_func, kwargs),
            daemon=True,
        )
        self._thread.start()

    def stop(self):
        self._running = False
        if self._thread:
            self._thread.join(timeout=2.0)
            self._thread = None

    def _run_loop(self, effect_func, kwargs):
        frame = 0
        interval = 1.0 / self._fps
        while self._running:
            t0 = time.monotonic()
            effect_func(self._ctrl, frame, **kwargs)
            frame += 1
            elapsed = time.monotonic() - t0
            if elapsed < interval:
                time.sleep(interval - elapsed)


def rainbow_wave(ctrl: ITE8295, frame: int, speed: int = 3, **_):
    """Rainbow wave effect - hue shifts across columns."""
    for row in range(GRID_ROWS):
        for col in range(GRID_COLS):
            hue = (col / GRID_COLS + frame * speed * 0.005) % 1.0
            r, g, b = _hsv_to_rgb(hue)
            ctrl.set_key_color(row, col, r, g, b)


def breathing(ctrl: ITE8295, frame: int, r: int = 255, g: int = 255, b: int = 255, speed: int = 3, **_):
    """Breathing effect - pulsing brightness."""
    t = frame * speed * 0.02
    factor = (math.sin(t) + 1.0) / 2.0
    cr = int(r * factor)
    cg = int(g * factor)
    cb = int(b * factor)
    ctrl.set_all_keys(cr, cg, cb)


def color_wave(ctrl: ITE8295, frame: int, r: int = 0, g: int = 100, b: int = 255, speed: int = 3, **_):
    """Color wave - brightness wave moves across columns."""
    for row in range(GRID_ROWS):
        for col in range(GRID_COLS):
            t = frame * speed * 0.03
            factor = (math.sin(t - col * 0.5) + 1.0) / 2.0
            ctrl.set_key_color(row, col, int(r * factor), int(g * factor), int(b * factor))


SOFTWARE_EFFECTS = {
    'sw_rainbow': rainbow_wave,
    'sw_breathing': breathing,
    'sw_wave': color_wave,
}
