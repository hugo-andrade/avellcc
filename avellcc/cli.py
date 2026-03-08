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


def _normalize_lightbar_name(value: str) -> str:
    return re.sub(r'[\s_]+', '-', value.strip().lower())


def parse_x58_effect(value: str) -> int:
    s = _normalize_lightbar_name(value)
    aliases = {
        "changecolor": "change-color",
        "colorwave": "color-wave",
    }
    s = aliases.get(s, s)
    if s in X58_EFFECT_CODES:
        return X58_EFFECT_CODES[s]
    return parse_byte(value)


def parse_x58_color_id(value: str) -> int:
    s = _normalize_lightbar_name(value)
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
        'mode': 'x58',
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


def cmd_led(args):
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
                effect_code = parse_x58_effect(str(effect))

            color = lightbar.get('color')
            color_id = lightbar.get('color_id')
            if color is not None:
                color_id = parse_x58_color_id(str(color))

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


def cmd_lightbar(args):
    if args.x58_effect is not None and args.x58_effect_code is not None:
        raise ValueError("Choose either --x58-effect or --x58-effect-code.")
    if args.x58_color is not None and args.x58_color_id is not None:
        raise ValueError("Choose either --x58-color or --x58-color-id.")
    if args.x58_static and (args.x58_effect is not None or args.x58_effect_code is not None):
        raise ValueError("--x58-static already fixes the effect to static.")

    x58_apply_requested = (
        not args.x58_static
        and any(
            value is not None
            for value in [
                args.x58_effect,
                args.x58_effect_code,
                args.x58_color,
                args.x58_color_id,
                args.x58_brightness,
                args.x58_speed,
            ]
        )
    )

    actions = []
    if args.descriptor:
        actions.append("descriptor")
    if args.restore:
        actions.append("restore")
    if args.get is not None:
        actions.append("get")
    if args.raw is not None:
        actions.append("raw")
    if args.command_id is not None:
        actions.append("command")
    if args.x170_off:
        actions.append("x170_off")
    if args.x170_brightness is not None:
        actions.append("x170_brightness")
    if args.x170_mode is not None:
        actions.append("x170_mode")
    if args.x170_color_cmd is not None:
        actions.append("x170_color")
    if args.x58_off:
        actions.append("x58_off")
    if args.x58_static:
        actions.append("x58_static")
    if x58_apply_requested:
        actions.append("x58_apply")
    if len(actions) > 1:
        raise ValueError("Choose only one lightbar action at a time.")

    feature_size = parse_byte(args.feature_size)

    ctrl = ITE8911()
    ctrl.open()
    try:
        if args.descriptor:
            data = ctrl.read_report_descriptor()
            print(f"Descriptor ({len(data)} bytes): {format_hex(data)}")
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

        if args.get is not None:
            report_id = parse_byte(args.get)
            data = ctrl.get_feature(report_id, length=feature_size)
            print(f"Feature 0x{report_id:02x}: {format_hex(data)}")
            return

        if args.raw is not None:
            report_id = parse_byte(args.report)
            payload = parse_bytes(args.raw)
            ctrl.send_feature(report_id, payload, total_size=feature_size)
            print(f"Sent report 0x{report_id:02x}: {format_hex([report_id] + payload)}")
            return

        if args.command_id is not None:
            cmd = parse_byte(args.command_id)
            payload = parse_bytes(args.data)
            ctrl.send_command(cmd, payload, total_size=feature_size)
            print(
                f"Sent command 0x{cmd:02x} "
                f"(report=0x{REPORT_ID_CTRL:02x}, feature_size={feature_size}): "
                f"{format_hex([REPORT_ID_CTRL, cmd] + payload)}"
            )
            return

        if args.x58_static:
            if args.x58_color is not None:
                color_id = parse_x58_color_id(args.x58_color)
            elif args.x58_color_id is not None:
                color_id = args.x58_color_id
            else:
                color_id = X58_DEFAULT_COLOR_ID
            brightness = X58_DEFAULT_BRIGHTNESS if args.x58_brightness is None else args.x58_brightness
            speed = X58_DEFAULT_SPEED if args.x58_speed is None else args.x58_speed
            ctrl.x58_set_static(color_id=color_id, brightness=brightness, speed=speed)
            save_lightbar_state(
                merge_lightbar_state(
                    None,
                    mode='x58',
                    effect=X58_DEFAULT_EFFECT,
                    effect_code=X58_DEFAULT_EFFECT_CODE,
                    color_id=color_id,
                    brightness=brightness,
                    speed=speed,
                )
            )
            print(
                "Sent grounded X58 static sequence "
                f"(report=0x{REPORT_ID_CTRL:02x}, cmd=0x{X58_COMMAND:02x}, "
                f"frame={WINDOWS_FRAME_SIZE}, color={X58_COLOR_NAMES.get(color_id, color_id)}, "
                f"brightness={brightness}, speed={speed})."
            )
            return

        if args.x58_off:
            ctrl.x58_off()
            save_lightbar_state({'mode': 'off'})
            print(
                "Sent grounded X58 off sequence "
                f"(report=0x{REPORT_ID_CTRL:02x}, cmd=0x{X58_COMMAND:02x}, frame={WINDOWS_FRAME_SIZE})."
            )
            return

        if x58_apply_requested:
            if args.x58_effect is not None:
                effect_code = parse_x58_effect(args.x58_effect)
            elif args.x58_effect_code is not None:
                effect_code = parse_byte(args.x58_effect_code)
            else:
                effect_code = None

            if args.x58_color is not None:
                color_id = parse_x58_color_id(args.x58_color)
            elif args.x58_color_id is not None:
                color_id = args.x58_color_id
            else:
                color_id = None

            current_state = load_state_bundle().get('lightbar')
            saved_state = merge_lightbar_state(
                current_state,
                mode='x58',
                effect_code=effect_code,
                color_id=color_id,
                brightness=args.x58_brightness,
                speed=args.x58_speed,
            )
            ctrl.x58_apply(
                effect_code=effect_code,
                color_id=color_id,
                brightness=args.x58_brightness,
                speed=args.x58_speed,
            )
            save_lightbar_state(saved_state)

            parts = [f"frame={WINDOWS_FRAME_SIZE}"]
            if effect_code is not None:
                parts.append(f"effect={X58_EFFECT_NAMES.get(effect_code, hex(effect_code))}")
            if color_id is not None:
                parts.append(f"color={X58_COLOR_NAMES.get(color_id, color_id)}")
            if args.x58_brightness is not None:
                parts.append(f"brightness={args.x58_brightness}")
            if args.x58_speed is not None:
                parts.append(f"speed={args.x58_speed}")
            print("Sent grounded X58 update (" + ", ".join(parts) + ").")
            return

        if args.x170_off:
            ctrl.x170_off()
            print(
                "Sent experimental X170-compatible off command "
                f"(report=0x{REPORT_ID_CTRL:02x}, cmd=0x{X170_POWER_COMMAND:02x})."
            )
            return

        if args.x170_brightness is not None:
            ctrl.x170_set_brightness(args.x170_brightness)
            print(
                "Sent experimental X170-compatible brightness command "
                f"(cmd=0x{X170_POWER_COMMAND:02x}, level={args.x170_brightness})."
            )
            return

        if args.x170_mode is not None:
            cmd = parse_byte(args.x170_mode)
            ctrl.x170_set_mode(cmd, args.speed)
            print(
                "Sent experimental X170-compatible mode command "
                f"(cmd=0x{cmd:02x}, speed={args.speed})."
            )
            return

        if args.x170_color_cmd is not None:
            if not args.color:
                raise ValueError("--color is required with --x170-color-cmd")
            cmd = parse_byte(args.x170_color_cmd)
            r, g, b = parse_color(args.color)
            ctrl.x170_set_color(cmd, args.speed, r, g, b)
            print(
                "Sent experimental X170-compatible color command "
                f"(cmd=0x{cmd:02x}, speed={args.speed}, rgb={r},{g},{b})."
            )
            return

        print(f"Lightbar device: {ctrl.path}")
        print(f"Linux descriptor-sized feature length: {DESCRIPTOR_REPORT_SIZE}")
        print(f"Windows X58 feature frame length: {WINDOWS_FRAME_SIZE}")
        print(
            "Known X58 effects: "
            + ", ".join(f"{name}=0x{code:02x}" for name, code in X58_EFFECT_CODES.items())
        )
        print(
            "Known X58 colors: "
            + ", ".join(
                f"{name}=id{color_id}:{X58_COLOR_HEX[color_id]}"
                for name, color_id in X58_COLOR_IDS.items()
            )
        )
        print(f"Known X170 effect commands: {', '.join(f'0x{cmd:02x}' for cmd in X170_EFFECT_COMMANDS)}")
        print(f"Known X170 power command: 0x{X170_POWER_COMMAND:02x}")
        print(f"Descriptor: {format_hex(ctrl.read_report_descriptor())}")
        for report_id in (REPORT_ID_INFO, REPORT_ID_CTRL):
            try:
                print(f"Feature 0x{report_id:02x}: {format_hex(ctrl.get_feature(report_id))}")
            except OSError as e:
                print(f"Feature 0x{report_id:02x}: read failed ({e})")
    finally:
        ctrl.close()


def main():
    parser = argparse.ArgumentParser(
        prog='avellcc',
        description='Avell Storm 590X Control Center for Linux',
    )
    parser.add_argument('--version', action='version', version=f'%(prog)s {__version__}')
    sub = parser.add_subparsers(dest='subcommand')

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

    # Lightbar subcommand
    lb_p = sub.add_parser('lightbar', help='Control the 048d:8911 rear lightbar')
    lb_p.add_argument('--descriptor', action='store_true', help='Dump the HID report descriptor')
    lb_p.add_argument('--restore', action='store_true', help='Restore the saved X58 lightbar state')
    lb_p.add_argument('--get', metavar='REPORT_ID', help='Read a feature report (for example 0x5A or 0xCD)')
    lb_p.add_argument(
        '--feature-size',
        default=str(DESCRIPTOR_REPORT_SIZE),
        help='Feature report size for --get/--raw/--command (default: 17, Windows X58 uses 64)',
    )
    lb_p.add_argument(
        '--raw',
        metavar='BYTES',
        help='Send raw payload bytes to the report selected by --report. The report ID byte is added automatically.',
    )
    lb_p.add_argument('--report', default='0xCD', help='Report ID used with --raw (default: 0xCD)')
    lb_p.add_argument(
        '--command',
        dest='command_id',
        metavar='CMD',
        help='Send a candidate command byte on report 0xCD. Use --data for the payload bytes.',
    )
    lb_p.add_argument('--data', metavar='BYTES', help='Payload bytes for --command')
    lb_p.add_argument('--x170-off', action='store_true', help='Send the old X170 off/power command (experimental)')
    lb_p.add_argument(
        '--x170-brightness',
        type=int,
        help='Send the old X170 brightness command (experimental, 0-255)',
    )
    lb_p.add_argument(
        '--x170-mode',
        metavar='CMD',
        help='Send an old X170 mode command (for example 0xB0, 0xB2, 0xB3, 0xB5)',
    )
    lb_p.add_argument(
        '--x170-color-cmd',
        metavar='CMD',
        help='Send an old X170 color payload (for example 0xB0, 0xB2, 0xB3, 0xB5)',
    )
    lb_p.add_argument('--speed', type=int, default=3, help='Speed byte used by the experimental X170 helpers')
    lb_p.add_argument('--color', help='Color used by --x170-color-cmd')
    lb_p.add_argument(
        '--x58-off',
        action='store_true',
        help='Send the grounded LightBar_X58 off sequence on 0xCD using 64-byte frames',
    )
    lb_p.add_argument(
        '--x58-static',
        action='store_true',
        help='Send the grounded LightBar_X58 static sequence on 0xCD using 64-byte frames',
    )
    lb_p.add_argument(
        '--x58-effect',
        metavar='NAME',
        help='Grounded LightBar_X58 effect name: static, breathe, wave, change-color, granular, color-wave',
    )
    lb_p.add_argument(
        '--x58-effect-code',
        metavar='CODE',
        help='Send a grounded LightBar_X58 effect code on report 0xCD with a 64-byte frame',
    )
    lb_p.add_argument(
        '--x58-color',
        metavar='NAME',
        help='Grounded LightBar_X58 color name or hex: red, yellow, lime, green, cyan, blue, purple',
    )
    lb_p.add_argument(
        '--x58-brightness',
        type=int,
        help='Send a grounded LightBar_X58 brightness level (Windows UI range 0-4)',
    )
    lb_p.add_argument(
        '--x58-speed',
        type=int,
        help='Send a grounded LightBar_X58 speed value with a 64-byte frame',
    )
    lb_p.add_argument(
        '--x58-color-id',
        type=int,
        help='Send a grounded LightBar_X58 color ID with a 64-byte frame',
    )
    lb_p.set_defaults(func=cmd_lightbar)

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
