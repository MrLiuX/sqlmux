"""A TCP proxy that is slow: slowproxy.py LISTEN_PORT UPSTREAM_PORT ROUNDTRIP_MS (127.0.0.1).

Every chunk it forwards, either way, waits ROUNDTRIP_MS / 2 first, so a request and its reply cost ROUNDTRIP_MS, as on a server far away
(F18.11: a Redis whose SCAN round trips are slow). It prints `up` once it listens. e2e/redis.sh's redis_slow starts it."""
import asyncio
import sys

lport, uport, ms = int(sys.argv[1]), int(sys.argv[2]), float(sys.argv[3])


async def pipe(r, w):
    try:
        while data := await r.read(65536):
            await asyncio.sleep(ms / 2000)
            w.write(data)
            await w.drain()
    except Exception:
        pass
    finally:
        w.close()


async def handle(r, w):
    ur, uw = await asyncio.open_connection("127.0.0.1", uport)
    await asyncio.gather(pipe(r, uw), pipe(ur, w))


async def main():
    s = await asyncio.start_server(handle, "127.0.0.1", lport)
    print("up", flush=True)
    async with s:
        await s.serve_forever()


asyncio.run(main())
