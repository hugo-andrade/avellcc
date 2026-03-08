"""Command-line interface for Avell Control Center."""

import argparse
import json
import os
import sys
import time

from . import __version__
from .ite8295 import ITE8295, EFFECT_NAMES, GRID_ROWS, GRID_COLS
from .effects import SOFTWARE_EFFECTS
from .keyboard_map import get_key_position, load_keymap, save_keymap, list_keys, DEFAULT_MAP
from .fan_control import FanController, status_report

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


def save_state(state: dict):
    os.makedirs(CONFIG_DIR, exist_ok=True)
    with open(STATE_FILE, 'w') as f:
        json.dump(state, f, indent=2)


def load_state() -> dict | None:
    if os.path.exists(STATE_FILE):
        with open(STATE_FILE) as f:
            return json.load(f)
    return None


def cmd_led(args):
    ctrl = ITE8295()
    ctrl.open()
    try:
        state = {}

        if args.off:
            ctrl.off()
            state['mode'] = 'off'
            save_state(state)
            print("Keyboard LEDs off.")
            return

        if args.restore:
            saved = load_state()
            if saved is None:
                print("No saved state found.")
                return
            _restore_state(ctrl, saved)
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
                # Software effect running in background
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
                    print(f"Unknown key: '{args.key}'. Use 'avellcc keys' to list keys.")
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
            _load_profile(ctrl, args.profile)
            state['mode'] = 'profile'
            state['profile'] = args.profile
            print(f"Profile '{args.profile}' loaded.")

        if state:
            save_state(state)
    finally:
        ctrl.close()


def _restore_state(ctrl: ITE8295, state: dict):
    mode = state.get('mode', '')
    if mode == 'off':
        ctrl.off()
    elif mode == 'effect':
        ctrl.set_effect_by_name(state['effect'], state.get('speed', 3))
    elif mode == 'static':
        r, g, b = state['color']
        ctrl.set_all_keys(r, g, b)
    elif mode == 'profile':
        _load_profile(ctrl, state['profile'])

    if 'brightness' in state:
        ctrl.set_brightness(state['brightness'])

    per_key = state.get('per_key', {})
    keymap = load_keymap()
    for key_name, (r, g, b) in per_key.items():
        pos = get_key_position(key_name, keymap)
        if pos:
            ctrl.set_key_color(pos[0], pos[1], r, g, b)


def _load_profile(ctrl: ITE8295, profile_path: str):
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
            fc.set_fan_speed(1, args.speed)
            fc.set_fan_speed(2, args.speed)
            print(f"All fans set to {args.speed}%.")
        return

    # Default: show status
    print(status_report(fc))


def cmd_keys(args):
    keymap = load_keymap()
    keys = list_keys(keymap)
    if args.verbose:
        for k in keys:
            pos = keymap[k]
            print(f"  {k:20s} -> row={pos[0]}, col={pos[1]}")
    else:
        for i in range(0, len(keys), 8):
            print('  '.join(f'{k:15s}' for k in keys[i:i+8]))


def cmd_calibrate(args):
    """Interactive calibration: light up each LED and ask user which key it is."""
    ctrl = ITE8295()
    ctrl.open()
    try:
        keymap = {}
        print("=== Keyboard LED Calibration ===")
        print("Each LED will light up RED one at a time.")
        print("Type the key name (e.g., 'esc', 'a', 'f1') or press Enter to skip.")
        print("Type 'q' to quit and save progress.\n")

        # First turn off all LEDs
        ctrl.set_all_keys(0, 0, 0)
        time.sleep(0.5)

        for row in range(GRID_ROWS):
            for col in range(GRID_COLS):
                # Light up this LED
                ctrl.set_key_color(row, col, 255, 0, 0)
                try:
                    answer = input(f"  LED ({row},{col:2d}): ").strip().lower()
                except (EOFError, KeyboardInterrupt):
                    answer = 'q'

                # Turn it off
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


def cmd_firmware(args):
    ctrl = ITE8295()
    ctrl.open()
    try:
        data = ctrl.get_firmware_info()
        print(f"Firmware report 0x5A: {' '.join(f'{b:02x}' for b in data)}")
    finally:
        ctrl.close()


def main():
    parser = argparse.ArgumentParser(
        prog='avellcc',
        description='Avell Storm 590X Control Center for Linux',
    )
    parser.add_argument('--version', action='version', version=f'%(prog)s {__version__}')
    sub = parser.add_subparsers(dest='command')

    # LED subcommand
    led_p = sub.add_parser('led', help='Control keyboard RGB LEDs')
    led_p.add_argument('--color', '-c', help='Set color (hex, name, or R,G,B)')
    led_p.add_argument('--key', '-k', help='Target a specific key')
    all_effects = sorted(set(EFFECT_NAMES) | set(SOFTWARE_EFFECTS))
    led_p.add_argument('--effect', '-e', help=f'Set effect ({", ".join(all_effects)})')
    led_p.add_argument('--speed', '-s', type=int, help='Effect speed (0-10)')
    led_p.add_argument('--brightness', '-b', type=int, help='Set brightness (0-10)')
    led_p.add_argument('--off', action='store_true', help='Turn off LEDs')
    led_p.add_argument('--restore', action='store_true', help='Restore saved state')
    led_p.add_argument('--profile', '-p', help='Load a profile JSON file')
    led_p.set_defaults(func=cmd_led)

    # Fan subcommand
    fan_p = sub.add_parser('fan', help='Control fans and view thermals')
    fan_p.add_argument('--status', action='store_true', help='Show fan and temperature status')
    fan_p.add_argument('--speed', type=int, help='Set fan speed (0-100%%)')
    fan_p.add_argument('--fan', type=int, choices=[1, 2], help='Target specific fan')
    fan_p.add_argument('--auto', action='store_true', help='Set fans to automatic mode')
    fan_p.set_defaults(func=cmd_fan)

    # Keys subcommand
    keys_p = sub.add_parser('keys', help='List known key names')
    keys_p.add_argument('-v', '--verbose', action='store_true', help='Show grid positions')
    keys_p.set_defaults(func=cmd_keys)

    # Calibrate subcommand
    cal_p = sub.add_parser('calibrate', help='Interactively calibrate key-to-LED mapping')
    cal_p.set_defaults(func=cmd_calibrate)

    # Firmware subcommand
    fw_p = sub.add_parser('firmware', help='Show firmware info')
    fw_p.set_defaults(func=cmd_firmware)

    args = parser.parse_args()
    if not args.command:
        parser.print_help()
        sys.exit(0)

    try:
        args.func(args)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
