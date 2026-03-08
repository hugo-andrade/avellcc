"""Command-line interface for Avell Control Center."""

import argparse
import json
import os
import re
import sys
import time

from . import __version__
from .ite8295 import ITE8295, EFFECT_NAMES, GRID_ROWS, GRID_COLS
from .effects import SOFTWARE_EFFECTS
from .keyboard_map import get_key_position, load_keymap, save_keymap, list_keys, DEFAULT_MAP
from .fan_control import FanController, status_report
from .lightbar import (
    DESCRIPTOR_REPORT_SIZE,
    ITE8911,
    REPORT_ID_CTRL,
    REPORT_ID_INFO,
    WINDOWS_FRAME_SIZE,
    X58_COLOR_HEX,
    X58_COLOR_IDS,
    X58_COLOR_NAMES,
    X58_DEFAULT_BRIGHTNESS,
    X58_DEFAULT_COLOR_ID,
    X58_DEFAULT_EFFECT,
    X58_DEFAULT_EFFECT_CODE,
    X58_DEFAULT_SPEED,
    X58_EFFECT_CODES,
    X58_EFFECT_NAMES,
    X58_COMMAND,
    X170_EFFECT_COMMANDS,
    X170_POWER_COMMAND,
)

CONFIG_DIR = os.path.expanduser('~/.config/avellcc')
STATE_FILE = os.path.join(CONFIG_DIR, 'state.json')


def parse_color(color_str: str) -> tuple[int, int, int]:
    """Parse a color string into (R, G, B).

    Supported formats:
      '#FF6600', 'FF6600', 'red', 'green', 'blue', 'white', '255,100,0'
    """
    s = color_str.strip().lower()

    named = {
        'red': (255, 0, 0), 'green': (0, 255, 0), 'blue': (0, 0, 255),
        'white': (255, 255, 255), 'black': (0, 0, 0),
        'yellow': (255, 255, 0), 'cyan': (0, 255, 255), 'magenta': (255, 0, 255),
        'orange': (255, 128, 0), 'purple': (128, 0, 255), 'pink': (255, 100, 200),
    }
    if s in named:
        return named[s]

    if ',' in s:
        parts = [int(x.strip()) for x in s.split(',')]
        if len(parts) == 3:
            return (parts[0] & 0xFF, parts[1] & 0xFF, parts[2] & 0xFF)

    s = s.lstrip('#')
    if len(s) == 6:
        return (int(s[0:2], 16), int(s[2:4], 16), int(s[4:6], 16))

    raise ValueError(f"Cannot parse color: '{color_str}'")


def parse_byte(value_str: str) -> int:
    """Parse a single byte in decimal or hex notation."""
    s = value_str.strip().lower()
    base = 16 if s.startswith('0x') or any(c in 'abcdef' for c in s) else 10
    value = int(s, base)
    if not 0 <= value <= 0xFF:
        raise ValueError(f"Byte out of range: {value_str}")
    return value


def parse_bytes(spec: str | None) -> list[int]:
    """Parse space/comma-separated decimal or hex bytes."""
    if not spec:
        return []
    parts = [p for p in re.split(r'[\s,;:]+', spec.strip()) if p]
    return [parse_byte(part) for part in parts]


def _normalize_name(value: str) -> str:
    return re.sub(r'[\s_]+', '-', value.strip().lower())


def parse_lightbar_effect(value: str) -> int:
    s = _normalize_name(value)
    aliases = {
        "changecolor": "change-color",
        "colorwave": "color-wave",
    }
    s = aliases.get(s, s)
    if s in X58_EFFECT_CODES:
        return X58_EFFECT_CODES[s]
    return parse_byte(value)


def parse_lightbar_color(value: str) -> int:
    s = _normalize_name(value)
    aliases = {
        "yellow-green": "lime",
        "chartreuse": "lime",
        "violet": "purple",
    }
    s = aliases.get(s, s)
    if s in X58_COLOR_IDS:
        return X58_COLOR_IDS[s]

    hex_value = value.strip().lower().lstrip("#")
    if len(hex_value) == 8:
        hex_value = hex_value[2:]
    if len(hex_value) == 6:
        for color_id, known_hex in X58_COLOR_HEX.items():
            if known_hex.lstrip("#") == hex_value:
                return color_id

    return parse_byte(value)


def format_hex(data: bytes | bytearray | list[int]) -> str:
    return ' '.join(f'{int(b) & 0xFF:02x}' for b in data)


# ---------------------------------------------------------------------------
# State persistence
# ---------------------------------------------------------------------------

def save_state(state: dict):
    os.makedirs(CONFIG_DIR, exist_ok=True)
    with open(STATE_FILE, 'w') as f:
        json.dump(state, f, indent=2)


def load_state() -> dict | None:
    if os.path.exists(STATE_FILE):
        with open(STATE_FILE) as f:
            return json.load(f)
    return None


def load_state_bundle() -> dict:
    raw = load_state()
    if not isinstance(raw, dict):
        return {}
    if 'keyboard' in raw or 'lightbar' in raw:
        return raw
    # Backward compatibility with the original keyboard-only schema.
    return {'keyboard': raw}


def save_state_bundle(bundle: dict):
    state = {}
    if bundle.get('keyboard'):
        state['keyboard'] = bundle['keyboard']
    if bundle.get('lightbar'):
        state['lightbar'] = bundle['lightbar']
    save_state(state)


def default_lightbar_state() -> dict:
    return {
        'mode': 'active',
        'effect': X58_DEFAULT_EFFECT,
        'effect_code': X58_DEFAULT_EFFECT_CODE,
        'color_id': X58_DEFAULT_COLOR_ID,
        'brightness': X58_DEFAULT_BRIGHTNESS,
        'speed': X58_DEFAULT_SPEED,
    }


def merge_lightbar_state(state: dict | None, **updates) -> dict:
    merged = default_lightbar_state()
    if isinstance(state, dict):
        merged.update(state)
    merged.update({k: v for k, v in updates.items() if v is not None})
    if merged.get('effect') in X58_EFFECT_CODES:
        merged['effect_code'] = X58_EFFECT_CODES[merged['effect']]
    elif merged.get('effect_code') in X58_EFFECT_NAMES:
        merged['effect'] = X58_EFFECT_NAMES[merged['effect_code']]
    return merged


def save_lightbar_state(lightbar_state: dict | None):
    bundle = load_state_bundle()
    if lightbar_state:
        bundle['lightbar'] = lightbar_state
    else:
        bundle.pop('lightbar', None)
    save_state_bundle(bundle)


def restore_lightbar_state(lightbar_state: dict | None, ctrl: ITE8911 | None = None):
    if not lightbar_state:
        return

    state = merge_lightbar_state(lightbar_state)
    owns_ctrl = ctrl is None
    if ctrl is None:
        ctrl = ITE8911()
        ctrl.open()
    try:
        if state.get('mode') == 'off':
            ctrl.x58_off()
            return
        ctrl.x58_apply(
            effect_code=state.get('effect_code'),
            color_id=state.get('color_id'),
            brightness=state.get('brightness'),
            speed=state.get('speed'),
        )
    finally:
        if owns_ctrl:
            ctrl.close()


# ---------------------------------------------------------------------------
# Subcommand: keyboard
# ---------------------------------------------------------------------------

def cmd_keyboard(args):
    action = getattr(args, 'action', None)

    # --- Sub-actions ---
    if action == 'keys':
        _keyboard_keys(args)
        return
    if action == 'calibrate':
        _keyboard_calibrate()
        return
    if action == 'firmware':
        _keyboard_firmware()
        return

    # --- LED control ---
    ctrl = ITE8295()
    ctrl.open()
    try:
        bundle = load_state_bundle()
        state = {}

        if args.off:
            ctrl.off()
            state['mode'] = 'off'
            bundle['keyboard'] = state
            save_state_bundle(bundle)
            print("Keyboard LEDs off.")
            return

        if args.restore:
            if not bundle:
                print("No saved state found.")
                return
            _restore_state(ctrl, bundle)
            print("State restored.")
            return

        if args.brightness is not None:
            ctrl.set_brightness(args.brightness)
            state['brightness'] = args.brightness
            print(f"Brightness set to {args.brightness}.")

        if args.effect:
            speed = args.speed if args.speed is not None else 3
            result = ctrl.set_effect_by_name(args.effect, speed)
            state['mode'] = 'effect'
            state['effect'] = args.effect
            state['speed'] = speed
            if result is not None:
                print(f"Software effect '{args.effect}' running (speed={speed}). Press Ctrl+C to stop.")
                try:
                    import signal
                    signal.pause()
                except KeyboardInterrupt:
                    result.stop()
                    print("\nEffect stopped.")
                return
            print(f"Hardware effect '{args.effect}' activated.")

        elif args.color:
            r, g, b = parse_color(args.color)

            if args.key:
                keymap = load_keymap()
                pos = get_key_position(args.key, keymap)
                if pos is None:
                    print(f"Unknown key: '{args.key}'. Use 'avellcc keyboard keys' to list keys.")
                    sys.exit(1)
                ctrl.set_key_color(pos[0], pos[1], r, g, b)
                state.setdefault('per_key', {})[args.key.lower()] = [r, g, b]
                print(f"Key '{args.key}' set to ({r}, {g}, {b}).")
            else:
                ctrl.set_all_keys(r, g, b)
                state['mode'] = 'static'
                state['color'] = [r, g, b]
                print(f"All keys set to ({r}, {g}, {b}).")

        elif args.profile:
            profile_lightbar_state = _load_profile(ctrl, args.profile)
            state['mode'] = 'profile'
            state['profile'] = args.profile
            if profile_lightbar_state is not None:
                bundle['lightbar'] = profile_lightbar_state
            print(f"Profile '{args.profile}' loaded.")

        if state:
            bundle['keyboard'] = state
            save_state_bundle(bundle)
    finally:
        ctrl.close()


def _keyboard_keys(args):
    keymap = load_keymap()
    keys = list_keys(keymap)
    if getattr(args, 'verbose', False):
        for k in keys:
            pos = keymap[k]
            print(f"  {k:20s} -> row={pos[0]}, col={pos[1]}")
    else:
        for i in range(0, len(keys), 8):
            print('  '.join(f'{k:15s}' for k in keys[i:i+8]))


def _keyboard_calibrate():
    ctrl = ITE8295()
    ctrl.open()
    try:
        keymap = {}
        print("=== Keyboard LED Calibration ===")
        print("Each LED will light up RED one at a time.")
        print("Type the key name (e.g., 'esc', 'a', 'f1') or press Enter to skip.")
        print("Type 'q' to quit and save progress.\n")

        ctrl.set_all_keys(0, 0, 0)
        time.sleep(0.5)

        for row in range(GRID_ROWS):
            for col in range(GRID_COLS):
                ctrl.set_key_color(row, col, 255, 0, 0)
                try:
                    answer = input(f"  LED ({row},{col:2d}): ").strip().lower()
                except (EOFError, KeyboardInterrupt):
                    answer = 'q'

                ctrl.set_key_color(row, col, 0, 0, 0)

                if answer == 'q':
                    break
                if answer:
                    keymap[answer] = (row, col)
                    print(f"    -> mapped '{answer}' to ({row}, {col})")
            else:
                continue
            break

        if keymap:
            save_keymap(keymap)
            print(f"\nSaved {len(keymap)} key mappings to {os.path.expanduser('~/.config/avellcc/keymap.json')}")
        else:
            print("\nNo keys mapped.")
    finally:
        ctrl.close()


def _keyboard_firmware():
    ctrl = ITE8295()
    ctrl.open()
    try:
        data = ctrl.get_firmware_info()
        print(f"Firmware report 0x5A: {' '.join(f'{b:02x}' for b in data)}")
    finally:
        ctrl.close()


def _restore_state(ctrl: ITE8295, state: dict):
    bundle = state if ('keyboard' in state or 'lightbar' in state) else {'keyboard': state}
    keyboard_state = bundle.get('keyboard', {})
    mode = keyboard_state.get('mode', '')
    if mode == 'off':
        ctrl.off()
    elif mode == 'effect':
        ctrl.set_effect_by_name(keyboard_state['effect'], keyboard_state.get('speed', 3))
    elif mode == 'static':
        r, g, b = keyboard_state['color']
        ctrl.set_all_keys(r, g, b)
    elif mode == 'profile':
        _load_profile(ctrl, keyboard_state['profile'])

    if 'brightness' in keyboard_state:
        ctrl.set_brightness(keyboard_state['brightness'])

    per_key = keyboard_state.get('per_key', {})
    keymap = load_keymap()
    for key_name, (r, g, b) in per_key.items():
        pos = get_key_position(key_name, keymap)
        if pos:
            ctrl.set_key_color(pos[0], pos[1], r, g, b)

    restore_lightbar_state(bundle.get('lightbar'))


def _load_profile(ctrl: ITE8295, profile_path: str) -> dict | None:
    if not os.path.exists(profile_path):
        config_path = os.path.join(CONFIG_DIR, 'profiles', profile_path)
        if os.path.exists(config_path):
            profile_path = config_path
        else:
            print(f"Profile not found: {profile_path}")
            sys.exit(1)

    with open(profile_path) as f:
        profile = json.load(f)

    if 'brightness' in profile:
        ctrl.set_brightness(profile['brightness'])

    if 'effect' in profile:
        ctrl.set_effect_by_name(profile['effect'], profile.get('speed', 3))
    elif 'color' in profile:
        r, g, b = parse_color(profile['color']) if isinstance(profile['color'], str) else profile['color']
        ctrl.set_all_keys(r, g, b)

    keymap = load_keymap()
    for key_name, color in profile.get('keys', {}).items():
        pos = get_key_position(key_name, keymap)
        if pos:
            r, g, b = parse_color(color) if isinstance(color, str) else color
            ctrl.set_key_color(pos[0], pos[1], r, g, b)

    lightbar = profile.get('lightbar')
    if isinstance(lightbar, dict):
        if lightbar.get('mode') == 'off':
            applied_state = {'mode': 'off'}
            restore_lightbar_state(applied_state)
            return applied_state
        else:
            effect = lightbar.get('effect')
            effect_code = lightbar.get('effect_code')
            if effect is not None:
                effect_code = parse_lightbar_effect(str(effect))

            color = lightbar.get('color')
            color_id = lightbar.get('color_id')
            if color is not None:
                color_id = parse_lightbar_color(str(color))

            applied_state = merge_lightbar_state(
                None,
                effect_code=effect_code,
                color_id=color_id,
                brightness=lightbar.get('brightness'),
                speed=lightbar.get('speed'),
            )
            restore_lightbar_state(applied_state)
            return applied_state

    return None


# ---------------------------------------------------------------------------
# Subcommand: fan
# ---------------------------------------------------------------------------

def cmd_fan(args):
    fc = FanController()

    if args.status:
        print(status_report(fc))
        return

    if args.auto:
        fc.set_auto()
        print("Fans set to automatic mode.")
        return

    if args.speed is not None:
        if args.fan:
            fc.set_fan_speed(args.fan, args.speed)
            print(f"Fan {args.fan} set to {args.speed}%.")
        else:
            fc.set_fan_speed(0, args.speed)
            print(f"All fans set to {args.speed}%.")
        return

    # Default: show status
    print(status_report(fc))


# ---------------------------------------------------------------------------
# Subcommand: lightbar
# ---------------------------------------------------------------------------

LIGHTBAR_EFFECTS = sorted(X58_EFFECT_CODES.keys())
LIGHTBAR_COLORS = sorted(X58_COLOR_IDS.keys())


def cmd_lightbar(args):
    # --- Resolve effect ---
    effect_code = None
    if getattr(args, 'effect', None) is not None:
        effect_code = parse_lightbar_effect(args.effect)
    elif getattr(args, 'effect_code', None) is not None:
        effect_code = parse_byte(args.effect_code)

    # --- Resolve color ---
    color_id = None
    if getattr(args, 'color', None) is not None:
        color_id = parse_lightbar_color(args.color)
    elif getattr(args, 'color_id', None) is not None:
        color_id = args.color_id

    brightness = getattr(args, 'brightness', None)
    speed = getattr(args, 'speed', None)

    has_update = any(v is not None for v in [effect_code, color_id, brightness, speed])

    ctrl = ITE8911()
    ctrl.open()
    try:
        # --- Primary actions ---

        if args.off:
            ctrl.x58_off()
            save_lightbar_state({'mode': 'off'})
            print("Lightbar off.")
            return

        if args.restore:
            bundle = load_state_bundle()
            lightbar_state = bundle.get('lightbar')
            if not lightbar_state:
                print("No saved lightbar state found.")
                return
            restore_lightbar_state(lightbar_state, ctrl=ctrl)
            print("Lightbar state restored.")
            return

        if has_update:
            current_state = load_state_bundle().get('lightbar')
            saved_state = merge_lightbar_state(
                current_state,
                mode='active',
                effect_code=effect_code,
                color_id=color_id,
                brightness=brightness,
                speed=speed,
            )
            ctrl.x58_apply(
                effect_code=effect_code,
                color_id=color_id,
                brightness=brightness,
                speed=speed,
            )
            save_lightbar_state(saved_state)

            parts = []
            if effect_code is not None:
                parts.append(f"effect={X58_EFFECT_NAMES.get(effect_code, hex(effect_code))}")
            if color_id is not None:
                parts.append(f"color={X58_COLOR_NAMES.get(color_id, color_id)}")
            if brightness is not None:
                parts.append(f"brightness={brightness}")
            if speed is not None:
                parts.append(f"speed={speed}")
            print("Lightbar updated: " + ", ".join(parts) + ".")
            return

        # --- Debug actions ---

        if getattr(args, 'debug_descriptor', False):
            data = ctrl.read_report_descriptor()
            print(f"Descriptor ({len(data)} bytes): {format_hex(data)}")
            return

        if getattr(args, 'debug_get', None) is not None:
            feature_size = parse_byte(args.debug_feature_size)
            report_id = parse_byte(args.debug_get)
            data = ctrl.get_feature(report_id, length=feature_size)
            print(f"Feature 0x{report_id:02x}: {format_hex(data)}")
            return

        if getattr(args, 'debug_raw', None) is not None:
            feature_size = parse_byte(args.debug_feature_size)
            report_id = parse_byte(args.debug_report)
            payload = parse_bytes(args.debug_raw)
            ctrl.send_feature(report_id, payload, total_size=feature_size)
            print(f"Sent report 0x{report_id:02x}: {format_hex([report_id] + payload)}")
            return

        if getattr(args, 'debug_command', None) is not None:
            feature_size = parse_byte(args.debug_feature_size)
            cmd = parse_byte(args.debug_command)
            payload = parse_bytes(getattr(args, 'debug_data', None))
            ctrl.send_command(cmd, payload, total_size=feature_size)
            print(f"Sent command 0x{cmd:02x}: {format_hex([REPORT_ID_CTRL, cmd] + payload)}")
            return

        # --- Default: show lightbar info ---
        _lightbar_status(ctrl)

    finally:
        ctrl.close()


def _lightbar_status(ctrl: ITE8911):
    """Show lightbar device info and current saved state."""
    print(f"Device: {ctrl.path}")
    bundle = load_state_bundle()
    lb_state = bundle.get('lightbar', {})
    mode = lb_state.get('mode', 'unknown')
    if mode == 'off':
        print("State: off")
    else:
        effect = lb_state.get('effect', '?')
        color_id = lb_state.get('color_id')
        color_name = X58_COLOR_NAMES.get(color_id, '?') if color_id else '?'
        brightness = lb_state.get('brightness', '?')
        speed = lb_state.get('speed', '?')
        print(f"State: effect={effect}, color={color_name}, brightness={brightness}, speed={speed}")
    print(f"\nEffects: {', '.join(LIGHTBAR_EFFECTS)}")
    print(f"Colors:  {', '.join(LIGHTBAR_COLORS)}")


# ---------------------------------------------------------------------------
# Main parser
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        prog='avellcc',
        description='Avell Storm 590X Control Center for Linux',
    )
    parser.add_argument('--version', action='version', version=f'%(prog)s {__version__}')
    sub = parser.add_subparsers(dest='subcommand')

    # --- keyboard ---
    kb_p = sub.add_parser('keyboard', aliases=['kb'],
                           help='Control keyboard RGB LEDs')
    kb_p.add_argument('action', nargs='?', default=None,
                       choices=['keys', 'calibrate', 'firmware'],
                       help='keys: list key names | calibrate: map LEDs to keys | firmware: show firmware info')
    kb_p.add_argument('--color', '-c', help='Set color (hex, name, or R,G,B)')
    kb_p.add_argument('--key', '-k', help='Target a specific key')
    all_effects = sorted(set(EFFECT_NAMES) | set(SOFTWARE_EFFECTS))
    kb_p.add_argument('--effect', '-e', help=f'Set effect ({", ".join(all_effects)})')
    kb_p.add_argument('--speed', '-s', type=int, help='Effect speed (0-10)')
    kb_p.add_argument('--brightness', '-b', type=int, help='Set brightness (0-10)')
    kb_p.add_argument('--off', action='store_true', help='Turn off keyboard LEDs')
    kb_p.add_argument('--restore', action='store_true', help='Restore saved state')
    kb_p.add_argument('--profile', '-p', help='Load a profile JSON file')
    kb_p.add_argument('-v', '--verbose', action='store_true',
                       help='Show grid positions (with keys action)')
    kb_p.set_defaults(func=cmd_keyboard)

    # --- lightbar ---
    lb_p = sub.add_parser('lightbar', aliases=['lb'], help='Control rear lightbar')
    lb_p.add_argument('--effect', '-e', metavar='NAME',
                       help=f'Set effect ({", ".join(LIGHTBAR_EFFECTS)})')
    lb_p.add_argument('--color', '-c', metavar='NAME',
                       help=f'Set color ({", ".join(LIGHTBAR_COLORS)})')
    lb_p.add_argument('--brightness', '-b', type=int, metavar='N',
                       help='Set brightness (0-4)')
    lb_p.add_argument('--speed', '-s', type=int, metavar='N',
                       help='Set animation speed')
    lb_p.add_argument('--off', action='store_true', help='Turn off lightbar')
    lb_p.add_argument('--restore', action='store_true', help='Restore saved state')
    # Advanced (rarely needed)
    lb_p.add_argument('--effect-code', metavar='HEX',
                       help='Set effect by raw hex code')
    lb_p.add_argument('--color-id', type=int, metavar='N',
                       help='Set color by raw ID')

    # Debug group (for reverse engineering / troubleshooting)
    lb_dbg = lb_p.add_argument_group('debug', 'Low-level HID commands for troubleshooting')
    lb_dbg.add_argument('--debug-descriptor', action='store_true',
                         help='Dump the HID report descriptor')
    lb_dbg.add_argument('--debug-get', metavar='REPORT_ID',
                         help='Read a HID feature report (e.g. 0x5A)')
    lb_dbg.add_argument('--debug-raw', metavar='BYTES',
                         help='Send raw payload bytes (report ID via --debug-report)')
    lb_dbg.add_argument('--debug-report', default='0xCD',
                         help='Report ID for --debug-raw (default: 0xCD)')
    lb_dbg.add_argument('--debug-command', metavar='CMD',
                         help='Send a command byte on report 0xCD')
    lb_dbg.add_argument('--debug-data', metavar='BYTES',
                         help='Payload bytes for --debug-command')
    lb_dbg.add_argument('--debug-feature-size', default=str(DESCRIPTOR_REPORT_SIZE),
                         help=f'Feature frame size (default: {DESCRIPTOR_REPORT_SIZE})')
    lb_p.set_defaults(func=cmd_lightbar)

    # --- fan ---
    fan_p = sub.add_parser('fan', help='Control fans and view thermals')
    fan_p.add_argument('--status', action='store_true', help='Show fan and temperature status')
    fan_p.add_argument('--speed', type=int, help='Set fan speed (0-100%%)')
    fan_p.add_argument('--fan', type=int, choices=[1, 2], help='Target specific fan')
    fan_p.add_argument('--auto', action='store_true', help='Set fans to automatic mode')
    fan_p.set_defaults(func=cmd_fan)

    args = parser.parse_args()
    if not args.subcommand:
        parser.print_help()
        sys.exit(0)

    try:
        args.func(args)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
