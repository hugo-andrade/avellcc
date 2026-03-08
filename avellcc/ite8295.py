"""ITE IT8295 (829x) per-key RGB keyboard controller protocol.

Protocol confirmed via tuxedo-drivers (ite_829x.c) and live testing.
Communication is via HID feature reports on report ID 0xCC (6 bytes).

Report format: [0xCC, command, arg1, arg2, arg3, arg4]

Commands:
  0x00 - Trigger hardware animation (arg1=0x09 for random color anim)
  0x01 - Set single key color (arg1=led_id, arg2=R, arg3=G, arg4=B)
  0x09 - Set brightness (arg1=level 0-10, arg2=0x02)

LED addressing:
  led_id = (row << 5) | col
  Grid is 6 rows x 20 columns, not all positions have physical keys.
"""

from .hidraw import HidrawDevice, find_hidraw

VID = 0x048D
PID_MAIN = 0x8910
PID_SECONDARY = 0x8911

REPORT_ID = 0xCC
REPORT_SIZE = 7  # report_id + 6 bytes

GRID_ROWS = 6
GRID_COLS = 20

# Commands
CMD_SET_EFFECT = 0x00
CMD_SET_KEY_COLOR = 0x01
CMD_SET_BRIGHTNESS = 0x09

# Hardware animation: CC 00 09 00 00 00 (confirmed from tuxedo-drivers)
HW_ANIM_RANDOM_COLOR = 0x09

# Effect names map to hardware animation sub-commands or per-key software modes.
# The ITE 829x firmware only has one hardware animation (random color = 0x09).
# Other "effects" are implemented in software by setting per-key colors in a loop.
EFFECT_NAMES = {
    'random_color': HW_ANIM_RANDOM_COLOR,
    'rainbow': HW_ANIM_RANDOM_COLOR,
}

MAX_BRIGHTNESS = 10


def led_id(row: int, col: int) -> int:
    """Compute LED ID from grid coordinates."""
    return ((row & 0x07) << 5) | (col & 0x1F)


class ITE8295:
    """Driver for ITE IT8295 per-key RGB keyboard controller."""

    def __init__(self, device: HidrawDevice | None = None):
        self._dev = device
        self._owns_device = device is None

    def open(self):
        if self._dev is None:
            path = find_hidraw(VID, PID_MAIN)
            if path is None:
                raise RuntimeError(
                    f"ITE 8295 device ({VID:04x}:{PID_MAIN:04x}) not found. "
                    "Check USB connection and udev rules."
                )
            self._dev = HidrawDevice(path)
            self._owns_device = True
        self._dev.open()

    def close(self):
        if self._dev and self._owns_device:
            self._dev.close()

    def __enter__(self):
        self.open()
        return self

    def __exit__(self, *args):
        self.close()

    def _send(self, cmd: int, a1: int = 0, a2: int = 0, a3: int = 0, a4: int = 0):
        """Send a 0xCC feature report with the given command and arguments."""
        self._dev.send_feature_report([REPORT_ID, cmd, a1, a2, a3, a4, 0x00])

    def set_brightness(self, level: int):
        """Set keyboard backlight brightness (0-10)."""
        level = max(0, min(MAX_BRIGHTNESS, level))
        self._send(CMD_SET_BRIGHTNESS, level, 0x02)

    def set_key_color(self, row: int, col: int, r: int, g: int, b: int):
        """Set color of a single key by grid position."""
        self._send(CMD_SET_KEY_COLOR, led_id(row, col), r & 0xFF, g & 0xFF, b & 0xFF)

    def set_all_keys(self, r: int, g: int, b: int):
        """Set all keys to the same color."""
        for row in range(GRID_ROWS):
            for col in range(GRID_COLS):
                self.set_key_color(row, col, r, g, b)

    def set_key_map(self, color_map: dict[tuple[int, int], tuple[int, int, int]]):
        """Set colors from a map of {(row, col): (r, g, b)}."""
        for (row, col), (r, g, b) in color_map.items():
            self.set_key_color(row, col, r, g, b)

    def set_hw_animation(self, anim_id: int = HW_ANIM_RANDOM_COLOR):
        """Trigger a hardware-driven animation effect.

        The ITE 829x firmware has one known animation: random color (0x09).
        Command: CC 00 09 00 00 00
        """
        self._send(CMD_SET_EFFECT, anim_id)

    def set_effect_by_name(self, name: str, speed: int = 3):
        """Set effect by name. Hardware effects use the controller; others are software-driven."""
        effect_id = EFFECT_NAMES.get(name.lower())
        if effect_id is not None:
            self.set_hw_animation(effect_id)
            return
        from .effects import SOFTWARE_EFFECTS
        sw_name = f'sw_{name.lower()}'
        if name.lower() in SOFTWARE_EFFECTS:
            sw_name = name.lower()
        if sw_name not in SOFTWARE_EFFECTS:
            all_effects = list(EFFECT_NAMES.keys()) + list(SOFTWARE_EFFECTS.keys())
            raise ValueError(
                f"Unknown effect '{name}'. Available: {', '.join(sorted(all_effects))}"
            )
        from .effects import EffectRunner
        runner = EffectRunner(self)
        runner.start(SOFTWARE_EFFECTS[sw_name], speed=speed)
        return runner

    def off(self):
        """Turn off all keyboard LEDs."""
        self.set_brightness(0)

    def get_firmware_info(self) -> bytes:
        """Read firmware info from report 0x5A."""
        return self._dev.get_feature_report(0x5A, 17)
