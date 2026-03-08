"""Low-level hidraw interface for ITE 829x keyboard controllers."""

import os
import fcntl
import glob as _glob


def _IOC(direction, type_char, nr, size):
    return (direction << 30) | (ord(type_char) << 8) | nr | (size << 16)


def _HIDIOCSFEATURE(length):
    return _IOC(3, 'H', 0x06, length)


def _HIDIOCGFEATURE(length):
    return _IOC(3, 'H', 0x07, length)


# HIDIOCGRAWINFO = _IOC(2, 'H', 0x03, 8)
_HIDIOCGRAWINFO = 0x80084803


def find_hidraw(vendor_id: int, product_id: int) -> str | None:
    """Find the hidraw device path for a given USB VID:PID."""
    for path in sorted(_glob.glob('/sys/class/hidraw/hidraw*/device/uevent')):
        try:
            with open(path) as f:
                content = f.read()
            hid_id_line = [l for l in content.splitlines() if l.startswith('HID_ID=')]
            if not hid_id_line:
                continue
            parts = hid_id_line[0].split('=')[1].split(':')
            vid = int(parts[1], 16)
            pid = int(parts[2], 16)
            if vid == vendor_id and pid == product_id:
                hidraw_name = path.split('/sys/class/hidraw/')[1].split('/')[0]
                return f'/dev/{hidraw_name}'
        except (IOError, IndexError, ValueError):
            continue
    return None


class HidrawDevice:
    """Direct hidraw interface for sending/receiving HID feature reports."""

    def __init__(self, path: str):
        self.path = path
        self._fd = -1

    def open(self):
        self._fd = os.open(self.path, os.O_RDWR)

    def close(self):
        if self._fd >= 0:
            os.close(self._fd)
            self._fd = -1

    def __enter__(self):
        self.open()
        return self

    def __exit__(self, *args):
        self.close()

    def send_feature_report(self, data: bytes | bytearray | list[int]):
        """Send a SET_FEATURE report. First byte must be the report ID."""
        buf = bytearray(data)
        fcntl.ioctl(self._fd, _HIDIOCSFEATURE(len(buf)), buf)

    def get_feature_report(self, report_id: int, length: int) -> bytes:
        """Get a GET_FEATURE report. Returns `length` bytes including report ID."""
        buf = bytearray(length)
        buf[0] = report_id
        fcntl.ioctl(self._fd, _HIDIOCGFEATURE(length), buf)
        return bytes(buf)
