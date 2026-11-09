import sys
# 6000 frames of a full screen (200x60) of coloured text, each frame homed.
rows, cols = 60, 200
frames = []
for f in range(8):
    lines = []
    for y in range(rows):
        parts = []
        for x in range(0, cols, 10):
            parts.append("\x1b[38;5;%dm%s" % ((x + y + f) % 216 + 16, ("w%08d " % (x * y + f))[:10]))
        lines.append("".join(parts)[: cols * 12])
    frames.append("\x1b[H" + "\x1b[0m\r\n".join(lines))
out = sys.stdout
for i in range(6000):
    out.write(frames[i % 8])
out.flush()
