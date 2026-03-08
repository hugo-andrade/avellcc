# avellcc

<p>
    <a href="https://goreportcard.com/report/github.com/hugo-andrade/avellcc"><img src="https://goreportcard.com/badge/github.com/hugo-andrade/avellcc" alt="Go Report Badge"></a>
    <a href="https://github.com/hugo-andrade/avellcc/actions/workflows/ci.yml"><img src="https://github.com/hugo-andrade/avellcc/actions/workflows/ci.yml/badge.svg" alt="CI Badge"></a>
    <a href="https://github.com/hugo-andrade/avellcc/blob/main/LICENSE"><img src="https://img.shields.io/github/license/hugo-andrade/avellcc.svg" alt="License Badge"></a>
    <a href="https://github.com/hugo-andrade/avellcc/releases"><img src="https://img.shields.io/github/v/release/hugo-andrade/avellcc" alt="Release Badge"></a>
</p>

Linux control center for **Avell Storm 590X** (Clevo barebone) laptops. Per-key RGB keyboard LEDs, rear lightbar, fan control, and thermal monitoring — no Windows required.

Single static binary, zero dependencies.

## Hardware

| Component | Details |
|---|---|
| Keyboard LED Controller | ITE IT8295, USB `048d:8910` |
| Lightbar Controller | ITE `048d:8911` (X58 protocol) |
| Fans | 2x ACPI fans via WMI/hwmon |
| WMI | Clevo WMBB (`ABBC0F6D`) for fan control |

## Install

### Quick install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/hugo-andrade/avellcc/main/install.sh | bash
```

This downloads the latest release, verifies the checksum, installs the binary to `/usr/local/bin`, sets up udev rules, and installs the systemd restore service.

You can customize the install:

```bash
# Install a specific version
VERSION=0.2.0 curl -fsSL https://raw.githubusercontent.com/hugo-andrade/avellcc/main/install.sh | bash

# Install to a custom directory
INSTALL_DIR=~/.local/bin curl -fsSL https://raw.githubusercontent.com/hugo-andrade/avellcc/main/install.sh | bash
```

### Go install

```bash
go install github.com/hugo-andrade/avellcc@latest
```

> **Note:** `go install` only installs the binary. You still need to set up udev rules manually for non-root access (see [udev rules](#udev-rules) below).

### Build from source

```bash
git clone https://github.com/hugo-andrade/avellcc.git
cd avellcc
make install
```

Or manually:

```bash
go build -o avellcc .
sudo install -m 755 avellcc /usr/local/bin/
```

### udev rules

Required for non-root access to the keyboard and lightbar HID devices:

```bash
sudo cp udev/99-avellcc.rules /etc/udev/rules.d/
sudo udevadm control --reload-rules && sudo udevadm trigger
```

### Fan speed control (optional)

Fan speed control requires the `acpi_call` kernel module:

```bash
# Arch Linux
sudo pacman -S --needed linux-headers acpi_call-dkms

# Debian / Ubuntu
sudo apt install dkms acpi-call-dkms linux-headers-$(uname -r)
```

```bash
sudo modprobe acpi_call
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

### Fans

```bash
avellcc fan                               # Live TUI dashboard (interactive terminal)
avellcc fan --status                      # Plain text output
avellcc fan --speed 80                    # All fans 80%
avellcc fan --speed 100 --fan 1           # Fan 1 at 100%
avellcc fan --auto                        # Back to automatic
```

The TUI dashboard shows live RPM sparklines, duty progress bars, and temperatures. Keyboard shortcuts: `+` max, `-` min, `a` auto, `q` quit.

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

> **Tip:** The quick install script sets this up automatically.

## Uninstall

```bash
make uninstall
```

Or manually:

```bash
sudo systemctl disable --now avellcc-restore.service
sudo rm -f /usr/local/bin/avellcc
sudo rm -f /etc/udev/rules.d/99-avellcc.rules
sudo rm -f /etc/systemd/system/avellcc-restore.service
sudo udevadm control --reload-rules
sudo systemctl daemon-reload
```

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

Built and tested on Arch Linux. Should work on any distro with hidraw support. Other Clevo-based laptops with ITE IT8295 (TUXEDO, Sager, etc.) should also work.
