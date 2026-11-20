"""Writes a synthetic DNG for the RAW tests: a 64x48 Bayer image whose
top-left quarter is bright, orientation 6, and the given JPEG as a
reduced-resolution preview in a second IFD.

Usage: python3 dng.py preview.jpg out.dng [--broken-sensor]

With --broken-sensor the sensor image has a width of 0, so LibRaw cannot
decode it and only the preview is usable.
"""

import array
import struct
import sys

WIDTH, HEIGHT = 64, 48
SHORT, LONG, ASCII, BYTE, SRATIONAL = 3, 4, 2, 1, 10
SIZES = {BYTE: 1, ASCII: 1, SHORT: 2, LONG: 4, SRATIONAL: 8}
FORMATS = {BYTE: "B", SHORT: "H", LONG: "I"}


def entry_bytes(kind, values):
    if kind == ASCII:
        return values.encode() + b"\0"
    if kind == SRATIONAL:
        return b"".join(struct.pack("<ii", n, d) for n, d in values)
    return struct.pack("<%d%s" % (len(values), FORMATS[kind]), *values)


def ifd(entries, start, next_offset):
    """Lays out an IFD at start with its out-of-line values after it."""
    entries = sorted(entries)
    data_offset = start + 2 + 12 * len(entries) + 4
    table, data = [], b""
    for tag, kind, values in entries:
        raw = entry_bytes(kind, values)
        count = len(raw) // SIZES[kind]
        if len(raw) <= 4:
            field = raw.ljust(4, b"\0")
        else:
            field = struct.pack("<I", data_offset + len(data))
            data += raw
            if len(data) % 2:
                data += b"\0"
        table.append(struct.pack("<HHI", tag, kind, count) + field)
    head = struct.pack("<H", len(entries)) + b"".join(table) + struct.pack("<I", next_offset)
    return head + data


def main():
    preview_path, out_path = sys.argv[1], sys.argv[2]
    broken = "--broken-sensor" in sys.argv[3:]
    with open(preview_path, "rb") as f:
        preview = f.read()
    preview_width, preview_height = jpeg_size(preview)

    pixels = array.array("H", (
        10000 if x < WIDTH // 2 and y < HEIGHT // 2 else 2500
        for y in range(HEIGHT) for x in range(WIDTH)
    ))
    if sys.byteorder != "little":
        pixels.byteswap()
    sensor = pixels.tobytes()

    def ifd0(strip_offset, next_offset):
        return ifd([
            (256, LONG, [0 if broken else WIDTH]),
            (257, LONG, [HEIGHT]),
            (258, SHORT, [16]),
            (259, SHORT, [1]),
            (262, SHORT, [32803]),
            (271, ASCII, "Fuchsia"),
            (272, ASCII, "Test Camera"),
            (273, LONG, [strip_offset]),
            (274, SHORT, [6]),
            (277, SHORT, [1]),
            (278, LONG, [HEIGHT]),
            (279, LONG, [len(sensor)]),
            (33421, SHORT, [2, 2]),
            (33422, BYTE, [0, 1, 1, 2]),
            (50706, BYTE, [1, 4, 0, 0]),
            (50707, BYTE, [1, 1, 0, 0]),
            (50708, ASCII, "Fuchsia Test Camera"),
            (50710, BYTE, [0, 1, 2]),
            (50711, SHORT, [1]),
            (50714, SHORT, [0]),
            (50717, LONG, [16383]),
            (50721, SRATIONAL, [(1, 1), (0, 1), (0, 1), (0, 1), (1, 1), (0, 1), (0, 1), (0, 1), (1, 1)]),
            (50728, SRATIONAL, [(1, 1)] * 3),
            (50778, SHORT, [21]),
        ], 8, next_offset)

    def ifd1(start, jpeg_offset):
        return ifd([
            (254, LONG, [1]),
            (256, LONG, [preview_width]),
            (257, LONG, [preview_height]),
            (259, SHORT, [7]),
            (262, SHORT, [6]),
            (273, LONG, [jpeg_offset]),
            (277, SHORT, [3]),
            (278, LONG, [preview_height]),
            (279, LONG, [len(preview)]),
        ], start, 0)

    # Sizes do not depend on the offsets, so lay out once to measure.
    strip_offset = 8 + len(ifd0(0, 0))
    ifd1_offset = strip_offset + len(sensor)
    jpeg_offset = ifd1_offset + len(ifd1(0, 0))
    with open(out_path, "wb") as f:
        f.write(b"II*\0" + struct.pack("<I", 8))
        f.write(ifd0(strip_offset, ifd1_offset))
        f.write(sensor)
        f.write(ifd1(ifd1_offset, jpeg_offset))
        f.write(preview)


def jpeg_size(data):
    i = 2
    while i < len(data):
        marker, length = data[i + 1], struct.unpack(">H", data[i + 2:i + 4])[0]
        if 0xC0 <= marker <= 0xCF and marker not in (0xC4, 0xC8, 0xCC):
            height, width = struct.unpack(">HH", data[i + 5:i + 9])
            return width, height
        i += 2 + length
    raise ValueError("no JPEG frame header")


if __name__ == "__main__":
    main()
