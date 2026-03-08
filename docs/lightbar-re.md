# Lightbar Reverse Engineering Notes

This repo now includes direct tooling for the real Storm 590X secondary HID:

- USB VID:PID: `048d:8911`
- Linux hidraw node: detected as `ITE Tech. Inc. ITE Device(8911)`
- HID report descriptor:
  - report `0x5A`: 16-byte payload
  - report `0xCD`: 16-byte payload

Descriptor bytes captured on Linux:

```text
06 89 ff 09 12 a1 01 85 5a 09 01 15 00 26 ff 00
75 08 95 10 b1 00 c0 06 89 ff 09 cd a1 01 85 cd
09 01 15 00 26 ff 00 75 08 95 10 b1 00 c0
```

## Windows Findings

The older keyboard package contains:

- managed class: `CC.Device.LightBar_X170`
- native DLL: `LineLEDAPI.dll`
- native target inside that DLL: `vid_048d&pid_8297`

That backend is not the real transport for the Storm 590X `8911` device.
It is still useful as a legacy reference, but the newer FnKey package contains
the actual matching backend:

- managed class: `CC.Device.LightBar_X58`
- native DLL: `HID_Device.dll`
- native target: `Init_HIDdevice(0x048d, 0x8911, 0xff89, 0xcd)`

That is the first Windows path that matches the real hardware exactly.

### Real X58 transport

`HID_Device.dll` writes the `8911` through `HidD_SetFeature` with a 64-byte
frame. Static disassembly of `WriteData` and its helper shows this layout:

```text
byte 0  = report ID
byte 1  = command byte
byte 2+ = payload
rest    = 0x00 padding up to 64 bytes total
```

The important details:

- total feature frame size: `0x40` bytes
- no checksum
- no extra header beyond `report_id` and `cmd`
- `ReadData` uses the same framing, then issues `HidD_GetFeature`

For `LightBar_X58`, the grounded command family is `cmd = 0xE2`:

- speed: payload `[0x01, speed]`
- brightness: payload `[0x02, ui_level + 1]`
- color: payload `[0x03, color_id]`
- effect: payload `[effect_code]`

Grounded effect codes recovered from managed IL:

- `0x05`: static
- `0x06`: breathe
- `0x07`: wave
- `0x08`: change color
- `0x09`: granular
- `0x0A`: color wave

Grounded color IDs recovered from `Convert_toColorID` / `Convert_toColor`:

- `1`: red `#ff0000`
- `2`: yellow `#ffff00`
- `3`: lime `#80ff00`
- `4`: green `#00ff00`
- `5`: cyan `#00ffff`
- `6`: blue `#0000ff`
- `7`: purple `#8000ff`

Linux `hidraw` also accepts `0x40`-byte `SET_FEATURE` and `GET_FEATURE` calls on
the real `8911`, so the repo now exposes that Windows-sized transport directly.

### Legacy X170 command model

The native export is:

```text
SetLightBarData62(cmd, uint8_t payload[62])
```

From the managed caller we can recover the commands and the meaningful payload
bytes that were actually populated:

- `0xBF`: power/brightness
  - off: payload `[0x00]`
  - brightness: payload `[level]`
- `0xB0`, `0xB2`, `0xB3`, `0xB5`: effect groups
  - mode payload: `[0x00, speed]`
  - custom color payload: `[0x01, speed, R, G, B]`

Only the first 2-5 bytes were written; the rest of the 62-byte buffer stayed
zero. Those helpers remain in the repo as a reference for older models, but
they are no longer the primary hypothesis for the Storm 590X.

## Why `avellcc lightbar` exists

`avellcc lightbar` now gives three useful layers:

- raw access to `0x5A`/`0xCD`
- Windows-sized probing through `--debug-feature-size 64`
- grounded `LightBar_X58` helpers for the real `8911`

Useful commands:

```bash
avellcc lightbar
avellcc lightbar --debug-descriptor
avellcc lightbar --debug-get 0x5A
avellcc lightbar --debug-get 0xCD --debug-feature-size 64
avellcc lightbar --debug-raw "bf 05"
avellcc lightbar --debug-raw "e2 05" --debug-report 0xCD --debug-feature-size 64
avellcc lightbar --debug-command 0xbf --debug-data "05"
avellcc lightbar --debug-command 0xe2 --debug-data "05" --debug-feature-size 64
avellcc lightbar --off
avellcc lightbar --restore
avellcc lightbar --effect static --color blue --brightness 4 --speed 3
avellcc lightbar --effect wave --brightness 4 --speed 3
avellcc lightbar --effect color-wave --brightness 4 --speed 3
avellcc lightbar --effect change-color --brightness 4 --speed 3
avellcc lightbar --effect granular --color purple --brightness 4 --speed 3
```

## Current conclusion

The solid transport for the Storm 590X lightbar is the newer `LightBar_X58`
path:

- report ID `0xCD`
- command family `0xE2`
- total feature frame size `64`

The remaining unknown is the semantic map of every `effect_code` and
`color_id`, not the transport itself.

Confirmed hardware results on the Storm 590X:

- `--off` turns the physical rear lightbar off
- `--effect static --color blue --brightness 4 --speed 3` produces a static blue bar
- `--effect wave` and `--effect color-wave` both still behave as RGB automatic modes on this machine

The repo now also persists grounded X58 state. `avellcc keyboard --restore` restores
both keyboard and lightbar state, and profiles can carry a `lightbar` section.
