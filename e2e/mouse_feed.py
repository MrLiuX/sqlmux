#!/usr/bin/env python3
"""mouse_feed.py SOCKET X YMIN YMAX SECONDS RATE — feed SGR "motion, no button" events to the tmux session t of SOCKET at RATE
events/s for SECONDS, the pointer bouncing between YMIN and YMAX (one row per event) at column X. Events go out in ticks of
50 ms against the absolute clock, so a slow tmux call does not lower the rate. Prints the number of events sent and the last row."""
import subprocess, sys, time

sock, x, ymin, ymax, secs, rate = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), int(sys.argv[4]), float(sys.argv[5]), int(sys.argv[6])
tick = 0.05
per = max(1, round(rate * tick))
span = max(1, ymax - ymin)
y, d, sent, last = ymin, 1, 0, ymin
t0 = time.monotonic()
n = int(secs / tick)
for i in range(n):
    buf = ""
    for _ in range(per):
        buf += "\x1b[<35;%d;%dM" % (x, y)
        last = y
        sent += 1
        if span > 0 and ymax > ymin:
            if y + d > ymax or y + d < ymin:
                d = -d
            y += d
    subprocess.run(["tmux", "-L", sock, "send-keys", "-t", "t", "-l", buf], check=True)
    wait = t0 + (i + 1) * tick - time.monotonic()
    if wait > 0:
        time.sleep(wait)
print(sent, last, "%.2f" % (time.monotonic() - t0))
