# commitcut.py LISTEN-PORT HOST PORT — a TCP proxy to a database that drops the connection once a COMMIT has gone through to the server:
# the COMMIT is forwarded and given time to run, its answer is never relayed, both sides are closed (e2e F13.1: "the connection went with the COMMIT").
import socket, sys, threading, time
lp, host, port = int(sys.argv[1]), sys.argv[2], int(sys.argv[3])
srv = socket.socket(); srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1); srv.bind(("127.0.0.1", lp)); srv.listen(16)
def pipe(a, b, cut, ev):
    try:
        while (d := a.recv(65536)):
            if ev.is_set():
                break                                  # after the COMMIT nothing goes back to the client
            b.sendall(d)
            if cut and b"COMMIT" in d.upper():
                ev.set()
                time.sleep(0.15)                       # the server runs it
                break
    except OSError: pass
    for s in (a, b):
        try: s.shutdown(socket.SHUT_RDWR)
        except OSError: pass
        try: s.close()
        except OSError: pass
while True:
    c, _ = srv.accept(); u = socket.create_connection((host, port)); ev = threading.Event()
    threading.Thread(target=pipe, args=(c, u, True, ev), daemon=True).start(); threading.Thread(target=pipe, args=(u, c, False, ev), daemon=True).start()
