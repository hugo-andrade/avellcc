"""Experimental support for the ITE 8911 lightbar device.

The Storm 590X exposes a second HID device at 048d:8911. Reverse engineering
the newer FnKey package showed that the real lightbar backend is
`CC.Device.LightBar_X58`, which writes 64-byte HID feature frames through
`HID_Device.dll`:

  * byte 0: report ID
  * byte 1: command byte
  * byte 2+: payload
  * remaining bytes: zero padded up to 64 bytes total

This module keeps the older X170 helpers for reference, but it now also exposes
the grounded X58 transport used by the Windows app on the real 8911 device.
"""

from __future__ import annotations

import os

from .hidraw import HidrawDevice, find_hidraw

VID = 0x048D
PID = 0x8911

REPORT_ID_INFO = 0x5A
REPORT_ID_CTRL = 0xCD
DESCRIPTOR_REPORT_PAYLOAD_SIZE = 16
DESCRIPTOR_REPORT_SIZE = 1 + DESCRIPTOR_REPORT_PAYLOAD_SIZE
WINDOWS_FRAME_SIZE = 0x40

X58_COMMAND = 0xE2
X58_EFFECT_CODES = {
    "static": 0x05,
    "breathe": 0x06,
    "wave": 0x07,
    "change-color": 0x08,
    "granular": 0x09,
    "color-wave": 0x0A,
}
X58_EFFECT_NAMES = {code: name for name, code in X58_EFFECT_CODES.items()}

# Reverse-engineered from `Convert_toColorID` / `Convert_toColor` in
# `CC.Device.LightBar_X58`.
X58_COLOR_IDS = {
    "red": 0x01,
    "yellow": 0x02,
    "lime": 0x03,
    "green": 0x04,
    "cyan": 0x05,
    "blue": 0x06,
    "purple": 0x07,
}
X58_COLOR_HEX = {
    0x01: "#ff0000",
    0x02: "#ffff00",
    0x03: "#80ff00",
    0x04: "#00ff00",
    0x05: "#00ffff",
    0x06: "#0000ff",
    0x07: "#8000ff",
}
X58_COLOR_NAMES = {color_id: name for name, color_id in X58_COLOR_IDS.items()}
X58_DEFAULT_EFFECT = "static"
X58_DEFAULT_EFFECT_CODE = X58_EFFECT_CODES[X58_DEFAULT_EFFECT]
X58_DEFAULT_COLOR = "red"
X58_DEFAULT_COLOR_ID = X58_COLOR_IDS[X58_DEFAULT_COLOR]
X58_DEFAULT_BRIGHTNESS = 4
X58_DEFAULT_SPEED = 3

# Reverse-engineered from the Windows `CC.Device.LightBar_X170` class.
X170_EFFECT_COMMANDS = (0xB0, 0xB2, 0xB3, 0xB5)
X170_POWER_COMMAND = 0xBF


def _clamp_byte(value: int) -> int:
    if not 0 <= value <= 0xFF:
        raise ValueError(f"Byte value out of range: {value}")
    return value


class ITE8911:
    """Raw hidraw access for the 048d:8911 device."""

    def __init__(self, device: HidrawDevice | None = None):
        self._dev = device
        self._owns_device = device is None

    @property
    def path(self) -> str | None:
        return self._dev.path if self._dev is not None else None

    def open(self):
        if self._dev is None:
            path = find_hidraw(VID, PID)
            if path is None:
                raise RuntimeError(
                    f"ITE lightbar device ({VID:04x}:{PID:04x}) not found. "
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

    def descriptor_path(self) -> str:
        if self._dev is None:
            raise RuntimeError("Device not opened")
        return f"/sys/class/hidraw/{os.path.basename(self._dev.path)}/device/report_descriptor"

    def read_report_descriptor(self) -> bytes:
        with open(self.descriptor_path(), "rb") as f:
            return f.read()

    def get_feature(self, report_id: int, length: int = DESCRIPTOR_REPORT_SIZE) -> bytes:
        if length < 2:
            raise ValueError(f"Feature length too small: {length}")
        return self._dev.get_feature_report(_clamp_byte(report_id), int(length))

    def send_feature(
        self,
        report_id: int,
        payload: bytes | bytearray | list[int],
        total_size: int = DESCRIPTOR_REPORT_SIZE,
    ):
        payload_bytes = bytes(int(b) & 0xFF for b in payload)
        if total_size < 2:
            raise ValueError(f"Feature length too small: {total_size}")
        max_payload = int(total_size) - 1
        if len(payload_bytes) > max_payload:
            raise ValueError(
                f"Feature payload too large: {len(payload_bytes)} bytes "
                f"(max {max_payload})"
            )
        buf = bytes([_clamp_byte(report_id)]) + payload_bytes.ljust(max_payload, b"\x00")
        self._dev.send_feature_report(buf)

    def send_command(
        self,
        command: int,
        data: bytes | bytearray | list[int] = b"",
        total_size: int = DESCRIPTOR_REPORT_SIZE,
    ):
        payload = bytes([_clamp_byte(command)]) + bytes(int(b) & 0xFF for b in data)
        max_payload = int(total_size) - 1
        if len(payload) > max_payload:
            raise ValueError(
                f"Command payload too large: {len(payload)} bytes "
                f"(max {max_payload})"
            )
        self.send_feature(REPORT_ID_CTRL, payload, total_size=total_size)

    def x58_set_effect(self, effect_code: int):
        self.send_command(X58_COMMAND, [_clamp_byte(effect_code)], total_size=WINDOWS_FRAME_SIZE)

    def x58_set_speed(self, speed: int):
        self.send_command(X58_COMMAND, [0x01, _clamp_byte(speed)], total_size=WINDOWS_FRAME_SIZE)

    def x58_set_brightness(self, level: int):
        if not 0 <= level <= 4:
            raise ValueError("X58 brightness uses the Windows UI range 0-4.")
        self.send_command(
            X58_COMMAND,
            [0x02, level + 1],
            total_size=WINDOWS_FRAME_SIZE,
        )

    def x58_set_color_id(self, color_id: int):
        self.send_command(
            X58_COMMAND,
            [0x03, _clamp_byte(color_id)],
            total_size=WINDOWS_FRAME_SIZE,
        )

    def x58_apply(
        self,
        effect_code: int | None = None,
        color_id: int | None = None,
        brightness: int | None = None,
        speed: int | None = None,
    ):
        # The Windows app updates the X58 lightbar in this order.
        if effect_code is not None:
            self.x58_set_effect(effect_code)
        if color_id is not None:
            self.x58_set_color_id(color_id)
        if brightness is not None:
            self.x58_set_brightness(brightness)
        if speed is not None:
            self.x58_set_speed(speed)

    def x58_set_static(
        self,
        color_id: int = X58_DEFAULT_COLOR_ID,
        brightness: int = X58_DEFAULT_BRIGHTNESS,
        speed: int = X58_DEFAULT_SPEED,
    ):
        self.x58_apply(
            effect_code=X58_DEFAULT_EFFECT_CODE,
            color_id=color_id,
            brightness=brightness,
            speed=speed,
        )

    def x58_off(self):
        self.x58_set_static(
            color_id=X58_DEFAULT_COLOR_ID,
            brightness=0,
            speed=X58_DEFAULT_SPEED,
        )

    def x170_off(self):
        self.send_command(X170_POWER_COMMAND, [0x00])

    def x170_set_brightness(self, level: int):
        self.send_command(X170_POWER_COMMAND, [_clamp_byte(level)])

    def x170_set_mode(self, command: int, speed: int):
        self.send_command(_clamp_byte(command), [0x00, _clamp_byte(speed)])

    def x170_set_color(self, command: int, speed: int, r: int, g: int, b: int):
        self.send_command(
            _clamp_byte(command),
            [0x01, _clamp_byte(speed), _clamp_byte(r), _clamp_byte(g), _clamp_byte(b)],
        )
