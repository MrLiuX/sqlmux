"""ConPTY smoke test for sqlmux on Windows (specs/m18-windows-mssql/task.md F18.5 b).

Run ON the Windows host (not in tmux — there is none): python smoke.py PATH\\TO\\sqlmux.exe
It starts sqlmux on a SQLite demo file in a ConPTY (pywinpty), feeds the screen to pyte, sends C-h j j Enter (and a mouse click) gt :qa, and
compares a few fixed texts: the connection and table names in the tree, NORMAL in the status bar. It does not try to
cover anything; it shows that the keyboard and the rendering work through ConPTY. Needs: pip install pywinpty pyte.
Config / state / data go to a temp directory (APPDATA, LOCALAPPDATA), never the user's own.
"""
import os
import shutil
import sqlite3
import queue
import sys
import tempfile
import threading
import time

import pyte
from winpty import PtyProcess

sys.stdout.reconfigure(encoding="utf-8")  # the console code page may be GBK; the screen has box drawing and icons
ROWS, COLS = 45, 160
exe = os.path.abspath(sys.argv[1])
tmp = tempfile.mkdtemp(prefix="sqlmux-conpty-")
db = os.path.join(tmp, "demo.db")
con = sqlite3.connect(db)
con.executescript(
    "create table t_user (id integer primary key, name text, email text);"
    "insert into t_user values (1, 'ann', 'ann@example.com'), (2, 'bob', 'bob@example.com'), (3, 'cat', null);"
    "create table t_order (id integer primary key, user_id integer, amount real);"
    "insert into t_order values (1, 1, 9.5), (2, 2, 20.0);"
)
con.close()

env = dict(os.environ, APPDATA=os.path.join(tmp, "appdata"), LOCALAPPDATA=os.path.join(tmp, "local"), TERM="xterm-256color")
for d in ("appdata", "local"):
    os.makedirs(os.path.join(tmp, d), exist_ok=True)

screen = pyte.Screen(COLS, ROWS)
stream = pyte.Stream(screen)
proc = PtyProcess.spawn([exe, db], dimensions=(ROWS, COLS), env=env)


chunks = queue.Queue()


def reader():
    """PtyProcess.read blocks until there is output, so a thread of its own does it."""
    while True:
        try:
            chunks.put(proc.read(65536))
        except EOFError:
            chunks.put(None)
            return


threading.Thread(target=reader, daemon=True).start()


def pump(seconds):
    """Feed what sqlmux has written in the next `seconds` to the screen."""
    end = time.time() + seconds
    while time.time() < end:
        try:
            data = chunks.get(timeout=0.1)
        except queue.Empty:
            continue
        if data is None:
            return
        stream.feed(data)


def text():
    return "\n".join(line.rstrip() for line in screen.display)


passed = failed = 0


def check(name, ok, why=""):
    global passed, failed
    print(("PASS " if ok else "FAIL ") + name)
    if ok:
        passed += 1
    else:
        failed += 1
        print("  " + why)


def show(title):
    print(f"----- {title} -----")
    print("\n".join(line for line in text().split("\n") if line.strip()))


def send(keys, wait=0.8):
    proc.write(keys)
    pump(wait)


def row_of(word):
    """The 1-based screen row of the sidebar line that holds `word`."""
    for y, line in enumerate(screen.display):
        if word in line[:32]:
            return y + 1
    return 0


try:
    pump(4)
    show("started")
    check("sqlmux started: the screen is not blank", bool(text().strip()), "blank screen")
    check("the connection name is in the tree (demo)", "demo" in text(), "no 'demo' on screen")
    check("the tables are in the tree (t_order, t_user)", "t_order" in text() and "t_user" in text(), "no table names")
    check("the status bar shows NORMAL", "NORMAL" in text(), "no NORMAL on screen")
    # keyboard: C-h puts the focus in the tree (it starts in the first pane), j moves its cursor from demo past Tables (2) to t_order,
    # Enter opens that table; C-h j Enter again opens the next one (t_user)
    send("\x08")
    send("j", 0.5)
    send("j", 0.5)
    send("\r", 2)
    show("after C-h j j Enter")
    check("C-h j j Enter opens t_order: its columns (user_id, amount) and a row (9.5) are drawn", all(w in text() for w in ("user_id", "amount", "9.5")), "no t_order grid")
    send("\x08")
    send("j", 0.5)
    send("\r", 2)
    show("after C-h j Enter")
    check("C-h j Enter opens t_user: email and ann@example.com are drawn", "email" in text() and "ann@example.com" in text(), "no t_user grid")
    # mouse: a click on a table in the tree switches to it
    y = row_of("t_order")
    send(f"\x1b[<0;12;{y}M\x1b[<0;12;{y}m", 2)
    show("after a click on t_order")
    check("a mouse click on t_order in the tree shows it again: amount and 9.5 are drawn, email is not", "amount" in text() and "9.5" in text() and "email" not in text(), f"no t_order grid (clicked row {y})")
    send("gt")
    show("after gt")
    check("gt: alive, NORMAL, still drawing", "NORMAL" in text() and proc.isalive(), "dead or no NORMAL")
    send(":qa\r", 1.5)
    time.sleep(1)
    check(":qa: the process exits", not proc.isalive(), "still running after :qa")
finally:
    if proc.isalive():
        proc.terminate(force=True)
    shutil.rmtree(tmp, ignore_errors=True)
print(f"== {passed} passed, {failed} failed")
sys.exit(1 if failed else 0)
