# avellcc

Linux control center for **Avell Storm 590X** (Clevo barebone) laptops. Per-key RGB keyboard LEDs, fan monitoring, and thermal status — no Windows required.

## Hardware

| Component | Details |
|---|---|
| Keyboard LED Controller | ITE IT8295, USB `048d:8910` |
| Secondary Device | ITE `048d:8911` (X58 lightbar support) |
| Fans | 2x ACPI fans via hwmon |
| WMI | Clevo WMBB (`ABBC0F6D`) for fan control |

## Install

```bash
# Dependencies
pip install hidapi

# Install
git clone git@github.com:hugo-andrade/avellcc.git
cd avellcc
pip install -e .

# udev rules (required for non-root access)
sudo cp udev/99-avell-keyboard.rules /etc/udev/rules.d/
sudo udevadm control --reload-rules && sudo udevadm trigger
```

## Usage

### Keyboard LEDs

```bash
avellcc led --color red                # All keys solid color
avellcc led --color "#FF6600"          # Hex color
avellcc led --color 255,100,0          # RGB values

avellcc led --key w --color blue       # Single key
avellcc led --key space --color green

avellcc led --brightness 7             # Brightness (0-10)
avellcc led --effect rainbow           # Hardware animation
avellcc led --effect sw_rainbow        # Software rainbow wave
avellcc led --effect sw_breathing      # Software breathing

avellcc led --profile gaming.json      # Load profile
avellcc led --off                      # Turn off
avellcc led --restore                  # Restore saved state
```

### Fan monitoring

```bash
avellcc fan --status                   # RPM + temperatures
```

Fan speed control requires `acpi_call-dkms` or `tuxedo-drivers-dkms`:

```bash
avellcc fan --speed 80                 # All fans 80%
avellcc fan --speed 100 --fan 1        # Fan 1 at 100%
avellcc fan --auto                     # Back to automatic
```

### Lightbar

```bash
avellcc lightbar --x58-off
avellcc lightbar --restore
avellcc lightbar --x58-effect static --x58-color blue --x58-brightness 4 --x58-speed 3
avellcc lightbar --x58-effect breathe --x58-color purple --x58-brightness 4 --x58-speed 3
avellcc lightbar --x58-effect wave --x58-brightness 4 --x58-speed 3
avellcc lightbar --x58-effect color-wave --x58-brightness 4 --x58-speed 3
avellcc lightbar --x58-effect change-color --x58-brightness 4 --x58-speed 3
avellcc lightbar --x58-effect granular --x58-color purple --x58-brightness 4 --x58-speed 3

# Raw / reverse-engineering access
avellcc lightbar --descriptor
avellcc lightbar --get 0x5A
avellcc lightbar --get 0xCD --feature-size 64
avellcc lightbar --raw "bf 05"
avellcc lightbar --command 0xe2 --data "05" --feature-size 64

# Legacy X170 probing
avellcc lightbar --x170-off
avellcc lightbar --x170-brightness 5
avellcc lightbar --x170-color-cmd 0xb0 --speed 3 --color blue
```

The practical path for the Storm 590X is `LightBar_X58` on `048d:8911`. It
uses `cmd 0xE2` inside a 64-byte HID feature frame and is now integrated as a
normal project feature. `avellcc led --restore` restores both keyboard and
lightbar state. The legacy X170 helpers remain only for comparison with older
models. Details are in [`docs/lightbar-re.md`](docs/lightbar-re.md).

### Other

```bash
avellcc keys -v                        # List known keys with grid positions
avellcc calibrate                      # Interactive key-to-LED calibration
avellcc firmware                       # Show firmware info
```

## Profiles

JSON files in `~/.config/avellcc/profiles/`:

```json
{
    "brightness": 10,
    "color": "black",
    "lightbar": {
        "effect": "static",
        "color": "blue",
        "brightness": 4,
        "speed": 3
    },
    "keys": {
        "w": "#FF0000",
        "a": "#FF0000",
        "s": "#FF0000",
        "d": "#FF0000",
        "space": "#FF4400",
        "esc": "#FFFFFF"
    }
}
```

## Restore on boot

```bash
sudo cp systemd/avellcc-restore.service /etc/systemd/system/
sudo systemctl enable avellcc-restore.service
```

The restore service uses `avellcc led --restore`, which now restores both the
keyboard and the saved X58 lightbar state.

## Protocol

Communication with the ITE IT8295 is via HID feature reports on report ID `0xCC` (6 bytes), sent through the Linux hidraw interface.

| Command | Format | Description |
|---|---|---|
| Set key color | `CC 01 <led_id> R G B` | Per-key RGB |
| Set brightness | `CC 09 <level> 02 00 00` | Level 0-10 |
| Hardware animation | `CC 00 09 00 00 00` | Random color effect |

LED addressing: `led_id = (row << 5) | col` on a 6x20 grid.

Key positions confirmed via [tuxedo-drivers](https://github.com/tuxedocomputers/tuxedo-drivers) (`ite_829x.c`).

## Compatibility

Built and tested on Arch Linux. Should work on any distro with:
- Python >= 3.10
- hidraw kernel support (standard)
- USB HID device `048d:8910`

Other Clevo-based laptops with the same ITE IT8295 controller (TUXEDO, Sager, etc.) should also work.

## License

GPL-3.0-or-later
