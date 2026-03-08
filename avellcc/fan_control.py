"""Fan and thermal control for Clevo/Avell laptops via WMI/ACPI.

Supports multiple backends:
1. tuxedo_io kernel module (preferred)
2. acpi_call kernel module (fallback)
3. hwmon sysfs (read-only)

Clevo WMI method codes (via WMBB method, GUID ABBC0F6D):
  0x63 - Get fan 1 duty cycle (0-255) and RPM
  0x64 - Get fan 2 duty cycle (0-255) and RPM
  0x67 - Get temperature (CPU=0x01, GPU=0x02 in arg)
  0x68 - Set fan duty (arg: fan_id<<16 | duty, duty 0-255)
  0x69 - Set fan auto mode
  0x79 - Performance mode (sub-command 0x19)
"""

import glob
import os
import struct


class FanController:
    """Read and control fan speeds on Clevo/Avell laptops."""

    def __init__(self):
        self._backend = self._detect_backend()

    def _detect_backend(self) -> str:
        if os.path.exists('/dev/tuxedo_io'):
            return 'tuxedo_io'
        if os.path.exists('/proc/acpi/call'):
            return 'acpi_call'
        if glob.glob('/sys/class/hwmon/hwmon*/fan*_input'):
            return 'hwmon'
        return 'none'

    @property
    def backend(self) -> str:
        return self._backend

    def get_fan_rpm(self) -> dict[str, int]:
        """Get current fan RPMs."""
        if self._backend == 'acpi_call':
            return self._acpi_get_fans()
        if self._backend == 'hwmon':
            return self._hwmon_get_fans()
        if self._backend == 'tuxedo_io':
            return self._tuxedo_get_fans()
        return {}

    def get_temperatures(self) -> dict[str, float]:
        """Get CPU and GPU temperatures."""
        temps = {}
        for hwmon in sorted(glob.glob('/sys/class/hwmon/hwmon*')):
            name_path = os.path.join(hwmon, 'name')
            if not os.path.exists(name_path):
                continue
            with open(name_path) as f:
                name = f.read().strip()
            for temp_input in sorted(glob.glob(os.path.join(hwmon, 'temp*_input'))):
                try:
                    with open(temp_input) as f:
                        val = int(f.read().strip())
                    label_path = temp_input.replace('_input', '_label')
                    if os.path.exists(label_path):
                        with open(label_path) as f:
                            label = f.read().strip()
                    else:
                        idx = os.path.basename(temp_input).replace('temp', '').replace('_input', '')
                        label = f"{name}_temp{idx}"
                    temps[label] = val / 1000.0
                except (IOError, ValueError):
                    continue
        return temps

    def set_fan_speed(self, fan_id: int, duty_percent: int):
        """Set fan speed as percentage (0-100). fan_id: 1 or 2."""
        duty = max(0, min(255, int(duty_percent * 255 / 100)))
        if self._backend == 'acpi_call':
            self._acpi_set_fan(fan_id, duty)
        elif self._backend == 'tuxedo_io':
            self._tuxedo_set_fan(fan_id, duty)
        else:
            raise RuntimeError(f"Backend '{self._backend}' does not support fan control. "
                             "Install tuxedo-drivers or acpi_call.")

    def set_auto(self):
        """Return fans to automatic control."""
        if self._backend == 'acpi_call':
            self._acpi_call(0x69, 0)
        elif self._backend == 'tuxedo_io':
            self._tuxedo_set_auto()
        else:
            raise RuntimeError(f"Backend '{self._backend}' does not support fan control.")

    # --- acpi_call backend ---

    def _acpi_call(self, method: int, arg: int) -> int:
        with open('/proc/acpi/call', 'w') as f:
            f.write(f'\\_SB.WMBB {method:#x} {arg:#x}')
        with open('/proc/acpi/call') as f:
            result = f.read().strip()
        if result.startswith('0x'):
            return int(result, 16)
        return 0

    def _acpi_get_fans(self) -> dict[str, int]:
        fans = {}
        try:
            raw1 = self._acpi_call(0x63, 0)
            fans['fan1_rpm'] = (raw1 >> 8) & 0xFFFF if raw1 else 0
            fans['fan1_duty'] = raw1 & 0xFF if raw1 else 0
        except (IOError, PermissionError):
            pass
        try:
            raw2 = self._acpi_call(0x64, 0)
            fans['fan2_rpm'] = (raw2 >> 8) & 0xFFFF if raw2 else 0
            fans['fan2_duty'] = raw2 & 0xFF if raw2 else 0
        except (IOError, PermissionError):
            pass
        return fans

    def _acpi_set_fan(self, fan_id: int, duty: int):
        # Clevo packs all fan duties into one u32: fan0[7:0] | fan1[15:8] | fan2[23:16]
        # Values < 12 are forced to 0; values 12-63 are raised to minimum threshold.
        if 0 < duty < 12:
            duty = 0
        shift = (fan_id - 1) * 8
        arg = (duty & 0xFF) << shift
        self._acpi_call(0x68, arg)

    # --- hwmon backend (read-only) ---

    def _hwmon_get_fans(self) -> dict[str, int]:
        fans = {}
        for fan_input in sorted(glob.glob('/sys/class/hwmon/hwmon*/fan*_input')):
            try:
                with open(fan_input) as f:
                    rpm = int(f.read().strip())
                name = os.path.basename(fan_input).replace('_input', '')
                hwmon_dir = os.path.dirname(fan_input)
                hwmon_name_path = os.path.join(hwmon_dir, 'name')
                if os.path.exists(hwmon_name_path):
                    with open(hwmon_name_path) as f:
                        hwmon_name = f.read().strip()
                    name = f"{hwmon_name}_{name}"
                fans[f"{name}_rpm"] = rpm
            except (IOError, ValueError):
                continue
        return fans

    # --- tuxedo_io backend ---

    def _tuxedo_get_fans(self) -> dict[str, int]:
        # tuxedo_io uses ioctl - for now fall back to hwmon
        return self._hwmon_get_fans()

    def _tuxedo_set_fan(self, fan_id: int, duty: int):
        # Requires tuxedo_io ioctl interface
        raise NotImplementedError("tuxedo_io fan set not yet implemented")

    def _tuxedo_set_auto(self):
        raise NotImplementedError("tuxedo_io auto mode not yet implemented")


def status_report(fc: FanController) -> str:
    """Generate a human-readable fan and temperature status report."""
    lines = [f"Backend: {fc.backend}"]

    fans = fc.get_fan_rpm()
    if fans:
        lines.append("\nFans:")
        for name, value in sorted(fans.items()):
            unit = "RPM" if "rpm" in name else ""
            lines.append(f"  {name}: {value} {unit}")
    else:
        lines.append("\nNo fan data available.")

    temps = fc.get_temperatures()
    if temps:
        lines.append("\nTemperatures:")
        for name, value in sorted(temps.items()):
            lines.append(f"  {name}: {value:.1f} C")

    return '\n'.join(lines)
