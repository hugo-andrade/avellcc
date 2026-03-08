"""Keyboard key-to-LED grid mapping for Avell Storm 590X.

The ITE IT8295 uses a 6x20 LED grid. Not all positions correspond to physical
keys. This module maps key names to their (row, col) grid positions.

Confirmed positions from tuxedo-drivers TUXEDO logo mode:
  (2,4)=E, (2,6)=T, (2,8)=U, (2,10)=O, (3,4)=D, (4,4)=X

The mapping should be calibrated for each specific keyboard layout using the
calibration tool (avellcc calibrate).
"""

import json
import os

# Default mapping based on tuxedo-drivers TUXEDO logo positions.
# Row 0 = top row (Esc, F1-F12, etc.)
# Row 5 = bottom row (Ctrl, Win, Alt, Space, etc.)
# Confirmed: QWERTY keys start at col 2, with wider keys taking extra grid slots.
DEFAULT_MAP: dict[str, tuple[int, int]] = {
    # Row 0: Function row
    'esc':    (0, 0),
    'f1':     (0, 2),
    'f2':     (0, 3),
    'f3':     (0, 4),
    'f4':     (0, 5),
    'f5':     (0, 6),
    'f6':     (0, 7),
    'f7':     (0, 8),
    'f8':     (0, 9),
    'f9':     (0, 10),
    'f10':    (0, 11),
    'f11':    (0, 12),
    'f12':    (0, 13),
    'prtsc':  (0, 15),
    'scroll': (0, 16),
    'pause':  (0, 17),

    # Row 1: Number row
    'grave':  (1, 0),  # ` / ~
    '1':      (1, 2),
    '2':      (1, 3),
    '3':      (1, 4),
    '4':      (1, 5),
    '5':      (1, 6),
    '6':      (1, 7),
    '7':      (1, 8),
    '8':      (1, 9),
    '9':      (1, 10),
    '0':      (1, 11),
    'minus':  (1, 12),
    'equal':  (1, 13),
    'backspace': (1, 14),
    'insert': (1, 15),
    'home':   (1, 16),
    'pageup': (1, 17),
    'numlock':   (1, 18),
    'num_slash': (1, 19),

    # Row 2: QWERTY row (confirmed: E=col4, T=col6, U=col8, O=col10)
    'tab':    (2, 0),
    'q':      (2, 2),
    'w':      (2, 3),
    'e':      (2, 4),
    'r':      (2, 5),
    't':      (2, 6),
    'y':      (2, 7),
    'u':      (2, 8),
    'i':      (2, 9),
    'o':      (2, 10),
    'p':      (2, 11),
    'lbracket': (2, 12),
    'rbracket': (2, 13),
    'backslash': (2, 14),
    'delete':   (2, 15),
    'end':      (2, 16),
    'pagedown': (2, 17),
    'num_7':    (2, 18),
    'num_8':    (2, 19),

    # Row 3: Home row (confirmed: D=col4)
    'capslock': (3, 0),
    'a':      (3, 2),
    's':      (3, 3),
    'd':      (3, 4),
    'f':      (3, 5),
    'g':      (3, 6),
    'h':      (3, 7),
    'j':      (3, 8),
    'k':      (3, 9),
    'l':      (3, 10),
    'semicolon': (3, 11),
    'apostrophe': (3, 12),
    'enter':  (3, 14),
    'num_4':  (3, 18),
    'num_5':  (3, 19),

    # Row 4: Shift row (confirmed: X=col4)
    'lshift': (4, 0),
    'z':      (4, 3),
    'x':      (4, 4),
    'c':      (4, 5),
    'v':      (4, 6),
    'b':      (4, 7),
    'n':      (4, 8),
    'm':      (4, 9),
    'comma':  (4, 10),
    'period': (4, 11),
    'slash':  (4, 12),
    'rshift': (4, 14),
    'up':     (4, 16),
    'num_1':  (4, 18),
    'num_2':  (4, 19),

    # Row 5: Bottom row
    'lctrl':  (5, 0),
    'lmeta':  (5, 1),
    'lalt':   (5, 2),
    'space':  (5, 6),
    'ralt':   (5, 10),
    'rmeta':  (5, 11),
    'menu':   (5, 12),
    'rctrl':  (5, 14),
    'left':   (5, 15),
    'down':   (5, 16),
    'right':  (5, 17),
    'num_0':  (5, 18),
    'num_dot':   (5, 19),
}

# Common aliases
_ALIASES = {
    'escape': 'esc',
    'printscreen': 'prtsc',
    'print_screen': 'prtsc',
    'scrolllock': 'scroll',
    'scroll_lock': 'scroll',
    'backtick': 'grave',
    'tilde': 'grave',
    '-': 'minus',
    '=': 'equal',
    'bksp': 'backspace',
    'bs': 'backspace',
    'ins': 'insert',
    'pgup': 'pageup',
    'pgdn': 'pagedown',
    'del': 'delete',
    '[': 'lbracket',
    ']': 'rbracket',
    '\\': 'backslash',
    'caps': 'capslock',
    ';': 'semicolon',
    "'": 'apostrophe',
    'return': 'enter',
    ',': 'comma',
    '.': 'period',
    '/': 'slash',
    'win': 'lmeta',
    'super': 'lmeta',
    'alt': 'lalt',
    'altgr': 'ralt',
    'ctrl': 'lctrl',
    'shift': 'lshift',
    'fn': 'lmeta',
}

CONFIG_DIR = os.path.expanduser('~/.config/avellcc')
KEYMAP_FILE = os.path.join(CONFIG_DIR, 'keymap.json')


def get_key_position(key_name: str, keymap: dict[str, tuple[int, int]] | None = None) -> tuple[int, int] | None:
    """Look up the (row, col) grid position for a key name."""
    km = keymap or DEFAULT_MAP
    name = key_name.lower().strip()
    name = _ALIASES.get(name, name)
    return km.get(name)


def load_keymap() -> dict[str, tuple[int, int]]:
    """Load custom keymap from config, falling back to default."""
    if os.path.exists(KEYMAP_FILE):
        with open(KEYMAP_FILE) as f:
            raw = json.load(f)
        return {k: tuple(v) for k, v in raw.items()}
    return DEFAULT_MAP.copy()


def save_keymap(keymap: dict[str, tuple[int, int]]):
    """Save keymap to config file."""
    os.makedirs(CONFIG_DIR, exist_ok=True)
    with open(KEYMAP_FILE, 'w') as f:
        json.dump({k: list(v) for k, v in keymap.items()}, f, indent=2)


def list_keys(keymap: dict[str, tuple[int, int]] | None = None) -> list[str]:
    """Return sorted list of all known key names."""
    km = keymap or DEFAULT_MAP
    return sorted(km.keys())
