# killhole.py LISTEN-PORT HOST PORT BLOCKFILE LOGFILE — a TCP proxy to a database that, while BLOCKFILE exists, takes new connections and never answers them
# (the KILL QUERY's temporary connection of a MySQL cancel cannot get through, e2e F14.6); connections made before keep flowing; LOGFILE gets a line for each connection taken, held or not.
import os, socket, sys, threading
lp, host, port, block, log = int(sys.argv[1]), sys.argv[2], int(sys.argv[3]), sys.argv[4], sys.argv[5]
srv = socket.socket(); srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1); srv.bind(("127.0.0.1", lp)); srv.listen(16)
held = []
def pipe(a, b):
    try:
        while (d := a.recv(65536)):
            b.sendall(d)
    except OSError: pass
    for s in (a, b):
        try: s.shutdown(socket.SHUT_RDWR)
        except OSError: pass
        try: s.close()
        except OSError: pass
while True:
    c, _ = srv.accept()
    with open(log, "a") as f: f.write("conn\n")
    if os.path.exists(block):
        held.append(c)                                 # silence: no handshake, nothing forwarded
        continue
    u = socket.create_connection((host, port))
    threading.Thread(target=pipe, args=(c, u), daemon=True).start(); threading.Thread(target=pipe, args=(u, c), daemon=True).start()
