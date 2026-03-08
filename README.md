# avellcc

Linux control center for **Avell Storm 590X** (Clevo barebone) laptops. Per-key RGB keyboard LEDs, fan monitoring, and thermal status — no Windows required.

## Hardware

| Component | Details |
|---|---|
| Keyboard LED Controller | ITE IT8295, USB `048d:8910` |
| Secondary Device | ITE `048d:8911` (lightbar/side LEDs — WIP) |
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
