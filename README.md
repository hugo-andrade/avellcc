# avellcc

Linux control center for **Avell Storm 590X** (Clevo barebone) laptops. Per-key RGB keyboard LEDs, rear lightbar, fan control, and thermal monitoring — no Windows required.

## Hardware

| Component | Details |
|---|---|
| Keyboard LED Controller | ITE IT8295, USB `048d:8910` |
| Lightbar Controller | ITE `048d:8911` (X58 protocol) |
| Fans | 2x ACPI fans via WMI/hwmon |
| WMI | Clevo WMBB (`ABBC0F6D`) for fan control |

## Install

System requirements:

- Python 3.10+
- Linux `hidraw` support
- `acpi_call-dkms` for fan speed control (optional)

```bash
# Arch Linux
sudo pacman -S --needed python python-pip git hidapi

# Debian / Ubuntu
sudo apt install python3 python3-pip python3-venv git libhidapi-hidraw0 libhidapi-dev

# Fedora
sudo dnf install python3 python3-pip git hidapi hidapi-devel
```

```bash
git clone git@github.com:hugo-andrade/avellcc.git
cd avellcc
pip install -e .

# udev rules (required for non-root access to keyboard and lightbar)
sudo cp udev/99-avell-keyboard.rules /etc/udev/rules.d/
sudo udevadm control --reload-rules && sudo udevadm trigger
```

## Usage

### Keyboard

```bash
avellcc keyboard --color red              # All keys solid color
avellcc keyboard --color "#FF6600"        # Hex color
avellcc keyboard --color 255,100,0        # RGB values

avellcc keyboard --key w --color blue     # Single key
avellcc keyboard --key space --color green

avellcc keyboard --brightness 7           # Brightness (0-10)
avellcc keyboard --effect rainbow         # Hardware animation
avellcc keyboard --effect sw_rainbow      # Software rainbow wave
avellcc keyboard --effect sw_breathing    # Software breathing

avellcc keyboard --profile gaming.json    # Load profile
avellcc keyboard --off                    # Turn off
avellcc keyboard --restore                # Restore saved state

avellcc kb -c red -b 7                    # Short alias
```

### Lightbar

```bash
avellcc lightbar                          # Show status and available effects/colors
avellcc lightbar --effect static --color blue --brightness 4
avellcc lightbar --effect static --color purple --brightness 4
avellcc lightbar --effect wave --speed 5
avellcc lightbar --effect color-wave
avellcc lightbar --effect change-color
avellcc lightbar --effect granular --color cyan
avellcc lightbar --off
avellcc lightbar --restore

avellcc lb -e static -c blue -b 4 -s 3   # Short alias
```

Available effects: `static`, `breathe`, `wave`, `change-color`, `granular`, `color-wave`

Available colors: `red`, `yellow`, `lime`, `green`, `cyan`, `blue`, `purple`

On the Storm 590X, `wave` and `color-wave` are currently confirmed as RGB
automatic modes. `static` is the grounded single-color mode.

### Fans

```bash
avellcc fan                               # Show RPM + temperatures
avellcc fan --status                      # Same as above
avellcc fan --speed 80                    # All fans 80%
avellcc fan --speed 100 --fan 1           # Fan 1 at 100%
avellcc fan --auto                        # Back to automatic
```

Fan speed control requires the `acpi_call` kernel module:

```bash
# Arch Linux
sudo pacman -S --needed linux-headers acpi_call-dkms

# Debian / Ubuntu
sudo apt install dkms acpi-call-dkms linux-headers-$(uname -r)

# Fedora
sudo dnf install dkms kernel-devel
```

```bash
sudo modprobe acpi_call
```

### Keyboard utilities

```bash
avellcc keyboard keys                     # List known key names
avellcc keyboard keys -v                  # With grid positions
avellcc keyboard calibrate                # Interactive key-to-LED calibration
avellcc keyboard firmware                 # Show keyboard firmware info
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
sudo systemctl daemon-reload
sudo systemctl enable avellcc-restore.service
```

The restore service runs `avellcc keyboard --restore`, which restores both keyboard and lightbar saved state.

## Protocol

### Keyboard (ITE IT8295)

HID feature reports on report ID `0xCC` (6 bytes) via Linux hidraw.

| Command | Format | Description |
|---|---|---|
| Set key color | `CC 01 <led_id> R G B` | Per-key RGB |
| Set brightness | `CC 09 <level> 02 00 00` | Level 0-10 |
| Hardware animation | `CC 00 09 00 00 00` | Random color effect |

LED addressing: `led_id = (row << 5) | col` on a 6x20 grid.

### Lightbar (ITE 8911)

HID feature reports on report ID `0xCD`, command `0xE2`, 64-byte frames via hidraw. Protocol reverse-engineered from the Windows `CC.Device.LightBar_X58` driver. Details in [`docs/lightbar-re.md`](docs/lightbar-re.md).

### Fans (Clevo WMI)

ACPI method `\_SB.WMI.WMBB` (GUID `ABBC0F6D`, 3 args: instance, command, data).

| Command | Function |
|---|---|
| `0x63` | Get fan 1 duty + period |
| `0x64` | Get fan 2 duty + period |
| `0x68` | Set fan duty (packed: fan1[7:0] \| fan2[15:8]) |
| `0x69` | Set auto mode (bitmask: bit0=fan1, bit1=fan2) |

## Compatibility

Built and tested on Arch Linux. Should work on any distro with Python >= 3.10 and hidraw support. Other Clevo-based laptops with ITE IT8295 (TUXEDO, Sager, etc.) should also work.

## License

GPL-3.0-or-later
