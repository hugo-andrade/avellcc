"""Fan and thermal control for Clevo/Avell laptops via WMI/ACPI.

Supports multiple backends:
1. tuxedo_io kernel module (preferred)
2. acpi_call kernel module (fallback, read+write)
3. hwmon sysfs (read-only)

Clevo WMI method: \\_SB.WMI.WMBB(instance=0, cmd, arg)
GUID ABBC0F6D, object ID BB.

WMI command codes:
  0x63 - Get fan 1 info (returns duty[7:0] | period_lo[15:8] | period_hi[23:16])
  0x64 - Get fan 2 info (same encoding)
  0x68 - Set fan duty (arg: fan0[7:0] | fan1[15:8], duty 0-255 each)
  0x69 - Set fan auto mode
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
        # Filter to interesting hwmon sources
        _SKIP = {'AC', 'BAT0', 'hidpp_battery_2', 'ucsi_source_psy_USBC000:001',
                 'ucsi_source_psy_USBC000:002', 'acpi_fan'}
        for hwmon in sorted(glob.glob('/sys/class/hwmon/hwmon*')):
            name_path = os.path.join(hwmon, 'name')
            if not os.path.exists(name_path):
                continue
            with open(name_path) as f:
                name = f.read().strip()
            if name in _SKIP:
                continue
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
                        label = f"temp{idx}"
                    # Prefix with source for clarity (except CPU cores)
                    if name == 'coretemp':
                        key = label  # "Package id 0", "Core 0", etc.
                    else:
                        hwmon_idx = os.path.basename(hwmon).replace('hwmon', '')
                        key = f"{name}[{hwmon_idx}]: {label}"
                    temps[key] = val / 1000.0
                except (IOError, ValueError):
                    continue
        return temps

    def set_fan_speed(self, fan_id: int, duty_percent: int):
        """Set fan speed as percentage (0-100). fan_id: 0=both, 1=fan1, 2=fan2."""
        duty = max(0, min(255, int(duty_percent * 255 / 100)))
        if self._backend == 'acpi_call':
            self._acpi_set_fan(fan_id, duty)
        elif self._backend == 'tuxedo_io':
            self._tuxedo_set_fan(fan_id, duty)
        else:
            raise RuntimeError(f"Backend '{self._backend}' does not support fan control. "
                             "Install acpi_call: sudo modprobe acpi_call")

    def set_auto(self):
        """Return fans to automatic control."""
        if self._backend == 'acpi_call':
            # arg is a bitmask: bit0=fan1, bit1=fan2, bit2=fan3, bit3=fan4
            self._acpi_call(0x69, 0x0F)
        elif self._backend == 'tuxedo_io':
            self._tuxedo_set_auto()
        else:
            raise RuntimeError(f"Backend '{self._backend}' does not support fan control.")

    # --- acpi_call backend ---

    def _acpi_call(self, method: int, arg: int) -> int:
        with open('/proc/acpi/call', 'w') as f:
            f.write(f'\\_SB.WMI.WMBB 0x00 {method:#x} {arg:#x}')
        with open('/proc/acpi/call') as f:
            result = f.read().strip().rstrip('\x00')
        if result.startswith('0x'):
            return int(result, 16)
        return 0

    def _acpi_get_fans(self) -> dict[str, int]:
        fans = {}
        for fan_num, cmd in [(1, 0x63), (2, 0x64)]:
            try:
                raw = self._acpi_call(cmd, 0)
                if raw:
                    duty = raw & 0xFF
                    fans[f'fan{fan_num}_duty'] = duty
                    fans[f'fan{fan_num}_duty_pct'] = round(duty * 100 / 255)
            except (IOError, PermissionError):
                pass
        # Supplement with hwmon RPM readings (more accurate)
        hwmon_fans = self._hwmon_get_fans()
        fans.update(hwmon_fans)
        return fans

    def _acpi_set_fan(self, fan_id: int, duty: int):
        # Clevo cmd 0x68 packs both fan duties: fan0[7:0] | fan1[15:8]
        # To set only one fan, we need to read the other fan's duty first
        # and preserve it, or set both at once.
        if fan_id == 0:
            # Set both fans to same duty
            arg = (duty & 0xFF) | ((duty & 0xFF) << 8)
        else:
            # Read current duties
            try:
                raw1 = self._acpi_call(0x63, 0)
                raw2 = self._acpi_call(0x64, 0)
                duty1 = raw1 & 0xFF if raw1 else duty
                duty2 = raw2 & 0xFF if raw2 else duty
            except (IOError, PermissionError):
                duty1 = duty2 = duty
            if fan_id == 1:
                duty1 = duty
            elif fan_id == 2:
                duty2 = duty
            arg = (duty1 & 0xFF) | ((duty2 & 0xFF) << 8)
        self._acpi_call(0x68, arg)

    # --- hwmon backend (read-only) ---

    def _hwmon_get_fans(self) -> dict[str, int]:
        fans = {}
        fan_idx = 0
        for fan_input in sorted(glob.glob('/sys/class/hwmon/hwmon*/fan*_input')):
            try:
                with open(fan_input) as f:
                    rpm = int(f.read().strip())
                fan_idx += 1
                fans[f"fan{fan_idx}_rpm"] = rpm
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
        # Group per fan
        fan_nums = sorted({k.replace('fan', '').split('_')[0]
                          for k in fans if k.startswith('fan')})
        for fn in fan_nums:
            rpm = fans.get(f'fan{fn}_rpm', '?')
            duty_pct = fans.get(f'fan{fn}_duty_pct')
            parts = [f"Fan {fn}: {rpm} RPM"]
            if duty_pct is not None:
                parts.append(f"(duty: {duty_pct}%)")
            lines.append(f"  {'  '.join(parts)}")
    else:
        lines.append("\nNo fan data available.")

    temps = fc.get_temperatures()
    if temps:
        lines.append("\nTemperatures:")
        # Summarize CPU cores (show package + max core)
        core_temps = {k: v for k, v in temps.items() if k.startswith('Core ')}
        other_temps = {k: v for k, v in temps.items() if not k.startswith('Core ')}
        for name, value in sorted(other_temps.items()):
            lines.append(f"  {name}: {value:.1f}°C")
        if core_temps:
            max_core = max(core_temps.values())
            min_core = min(core_temps.values())
            lines.append(f"  CPU Cores ({len(core_temps)}): {min_core:.0f}-{max_core:.0f}°C")

    return '\n'.join(lines)
